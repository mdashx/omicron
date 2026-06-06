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

type launchAgentRequest struct {
	AgentID   string `json:"agentId"`
	GuildID   string `json:"guildId"`
	ChannelID string `json:"channelId"`
	Command   string `json:"command"`
}

func (s *BridgeService) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/api/dashboard", s.handleDashboardData)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/join", s.handleJoin)
	mux.HandleFunc("/api/launch-agent", s.handleLaunchAgent)
	mux.HandleFunc("/agents/", s.handleAgents)
}

func (s *BridgeService) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.dashboardSnapshot())
}

func (s *BridgeService) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(renderDashboardHTML()))
}

func (s *BridgeService) handleDashboardData(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.dashboardSnapshot())
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
	if req.AgentID == "" || req.CredsRef == "" {
		http.Error(w, "agentId and credsRef are required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	requestedGuildID := req.RequestedGuildID
	requestedChannelID := req.RequestedChannelID
	if existing, ok := s.state.Bindings[req.AgentID]; ok && existing.Active {
		if requestedGuildID == "" {
			requestedGuildID = existing.GuildID
		}
		if requestedChannelID == "" {
			requestedChannelID = existing.ChannelID
		}
	}
	if requestedChannelID == "" {
		requestedGuildID, requestedChannelID = s.assignChannelLocked(requestedGuildID, req.AgentID)
		if requestedChannelID == "" {
			http.Error(w, "no assignable channel available", http.StatusConflict)
			return
		}
	}
	for agentID, binding := range s.state.Bindings {
		if agentID != req.AgentID && binding.Active && binding.ChannelID == requestedChannelID {
			http.Error(w, "channel already bound to another active agent", http.StatusConflict)
			return
		}
	}
	binding := Binding{
		AgentID:   req.AgentID,
		GuildID:   requestedGuildID,
		ChannelID: requestedChannelID,
		JoinedAt:  time.Now().UTC(),
		Active:    true,
	}
	s.state.Bindings[req.AgentID] = binding
	if err := s.saveStateLocked(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.appendAudit("agent.join", map[string]any{"agentId": req.AgentID, "channelId": requestedChannelID, "guildId": requestedGuildID, "scope": req.Scope})
	writeJSON(w, http.StatusOK, binding)
}

func (s *BridgeService) handleLaunchAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req launchAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.AgentID == "" {
		http.Error(w, "agentId is required", http.StatusBadRequest)
		return
	}
	launched, err := s.launchAgent(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, launched)
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

func (s *BridgeService) assignChannelLocked(requestedGuildID, agentID string) (string, string) {
	guildID := requestedGuildID
	if guildID == "" {
		guildID = s.cfg.DefaultGuildID
	}
	used := map[string]bool{}
	for existingAgentID, binding := range s.state.Bindings {
		if existingAgentID == agentID {
			continue
		}
		if binding.Active {
			used[binding.ChannelID] = true
		}
	}
	for _, channelID := range s.cfg.AssignableChannelIDs {
		if !used[channelID] {
			return guildID, channelID
		}
	}
	return guildID, ""
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
