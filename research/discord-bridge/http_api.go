package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type joinRequest struct {
	AgentID            string   `json:"agentId"`
	CredsRef           string   `json:"credsRef"`
	RequestedGuildID   string   `json:"requestedGuildId"`
	RequestedChannelID string   `json:"requestedChannelId"`
	Scope              []string `json:"scope"`
}

type statusUpdateRequest struct {
	MessageID string `json:"messageId"`
	Reaction  string `json:"reaction"`
}

type completeRequest struct {
	MessageID     string `json:"messageId"`
	Content       string `json:"content"`
	FinalReaction string `json:"finalReaction"`
}

func (s *BridgeService) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/join", s.handleJoin)
	mux.HandleFunc("/agents/", s.handleAgents)
}

func (s *BridgeService) handleStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"envelope": s.envelope,
		"bindings": s.state.Bindings,
		"queueSizes": func() map[string]int {
			m := map[string]int{}
			for agentID, q := range s.queues {
				m[agentID] = len(q)
			}
			return m
		}(),
		"dryRun": s.cfg.DryRun,
	})
}

func (s *BridgeService) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req joinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.AgentID == "" || req.CredsRef == "" || req.RequestedChannelID == "" {
		http.Error(w, "agentId, credsRef, requestedChannelId are required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for agentID, binding := range s.state.Bindings {
		if agentID != req.AgentID && binding.Active && binding.ChannelID == req.RequestedChannelID {
			http.Error(w, "channel already bound to another active agent", http.StatusConflict)
			return
		}
	}
	binding := Binding{
		AgentID:   req.AgentID,
		GuildID:   req.RequestedGuildID,
		ChannelID: req.RequestedChannelID,
		JoinedAt:  time.Now().UTC(),
		Active:    true,
	}
	s.state.Bindings[req.AgentID] = binding
	if err := s.saveStateLocked(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.appendAudit("agent.join", map[string]any{"agentId": req.AgentID, "channelId": req.RequestedChannelID, "guildId": req.RequestedGuildID, "scope": req.Scope})
	writeJSON(w, http.StatusOK, binding)
}

func (s *BridgeService) handleAgents(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/agents/")
	parts := strings.Split(strings.Trim(trimmed, "/"), "/")
	if len(parts) != 2 {
		http.NotFound(w, r)
		return
	}
	agentID, action := parts[0], parts[1]
	switch {
	case r.Method == http.MethodGet && action == "events":
		s.handleAgentEvents(w, agentID)
	case r.Method == http.MethodPost && action == "status":
		s.handleAgentStatus(w, r, agentID)
	case r.Method == http.MethodPost && action == "complete":
		s.handleAgentComplete(w, r, agentID)
	default:
		http.NotFound(w, r)
	}
}

func (s *BridgeService) handleAgentEvents(w http.ResponseWriter, agentID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	events := append([]InboundEvent(nil), s.queues[agentID]...)
	s.queues[agentID] = nil
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *BridgeService) handleAgentStatus(w http.ResponseWriter, r *http.Request, agentID string) {
	var req statusUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	binding, err := s.activeBinding(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if req.MessageID == "" || req.Reaction == "" {
		http.Error(w, "messageId and reaction are required", http.StatusBadRequest)
		return
	}
	if err := s.setManagedReaction(binding.ChannelID, req.MessageID, req.Reaction); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = s.appendAudit("agent.status", map[string]any{"agentId": agentID, "messageId": req.MessageID, "reaction": req.Reaction})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *BridgeService) handleAgentComplete(w http.ResponseWriter, r *http.Request, agentID string) {
	var req completeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	binding, err := s.activeBinding(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if req.MessageID == "" {
		http.Error(w, "messageId is required", http.StatusBadRequest)
		return
	}
	if req.FinalReaction != "" && !contains(s.cfg.FinalReactionChoices, req.FinalReaction) {
		http.Error(w, "finalReaction not allowed by bridge palette", http.StatusBadRequest)
		return
	}
	if req.Content != "" {
		if err := s.sendChannelMessage(binding.ChannelID, req.Content, req.MessageID); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	if req.FinalReaction != "" {
		if err := s.setManagedReaction(binding.ChannelID, req.MessageID, req.FinalReaction); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	_ = s.appendAudit("agent.complete", map[string]any{"agentId": agentID, "messageId": req.MessageID, "finalReaction": req.FinalReaction})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *BridgeService) activeBinding(agentID string) (Binding, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	binding, ok := s.state.Bindings[agentID]
	if !ok || !binding.Active {
		return Binding{}, errors.New("active binding not found")
	}
	return binding, nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func contains(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

func (s *BridgeService) sendChannelMessage(channelID, content, replyTo string) error {
	if s.cfg.DryRun {
		return s.appendAudit("discord.send.dry_run", map[string]any{"channelId": channelID, "content": content, "replyTo": replyTo})
	}
	msg := &discordgo.MessageSend{Content: content}
	if replyTo != "" {
		msg.Reference = &discordgo.MessageReference{MessageID: replyTo, ChannelID: channelID}
	}
	_, err := s.dg.ChannelMessageSendComplex(channelID, msg)
	if err == nil {
		_ = s.appendAudit("discord.send", map[string]any{"channelId": channelID, "content": content, "replyTo": replyTo})
	}
	return err
}
