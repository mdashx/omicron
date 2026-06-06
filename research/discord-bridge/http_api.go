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
	AgentID    string   `json:"agentId"`
	GuildID    string   `json:"guildId"`
	ChannelID  string   `json:"channelId"`
	Command    string   `json:"command"`
	Args       []string `json:"args,omitempty"`
	WorkingDir string   `json:"workingDir,omitempty"`
}

type stopAgentRequest struct {
	AgentID string `json:"agentId"`
}

func (s *BridgeService) registerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", s.handleHomePage)
	mux.HandleFunc("/managed-agents", s.handleManagedAgentsPage)
	mux.HandleFunc("/managed-agents/", s.handleManagedAgentsUI)
	mux.HandleFunc("/bindings", s.handleBindingsPage)
	mux.HandleFunc("/activity", s.handleActivityPage)
	mux.HandleFunc("/partials/overview", s.handleOverviewPartial)
	mux.HandleFunc("/partials/managed-agents-table", s.handleManagedAgentsTablePartial)
	mux.HandleFunc("/partials/bindings-table", s.handleBindingsTablePartial)
	mux.HandleFunc("/partials/activity-feed", s.handleActivityFeedPartial)
	mux.HandleFunc("/api/dashboard", s.handleDashboardData)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/join", s.handleJoin)
	mux.HandleFunc("/api/launch-agent", s.handleLaunchAgent)
	mux.HandleFunc("/api/stop-agent", s.handleStopAgent)
	mux.HandleFunc("/agents/", s.handleAgents)
}

func (s *BridgeService) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.dashboardSnapshot())
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
	managed := s.upsertManagedAgentLocked(ManagedAgent{
		AgentID:            req.AgentID,
		CredsRef:           req.CredsRef,
		DesiredState:       "running",
		RequestedGuildID:   requestedGuildID,
		RequestedChannelID: requestedChannelID,
		LastJoinAt:         binding.JoinedAt,
	})
	s.state.ManagedAgents[req.AgentID] = managed
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

func (s *BridgeService) handleStopAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req stopAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.AgentID == "" {
		http.Error(w, "agentId is required", http.StatusBadRequest)
		return
	}
	launched, err := s.stopAgent(req.AgentID)
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
	s.mu.Lock()
	managed := s.upsertManagedAgentLocked(ManagedAgent{AgentID: agentID, DesiredState: "running"})
	managed.LastCompletionAt = time.Now().UTC()
	managed.LastError = ""
	s.state.ManagedAgents[agentID] = managed
	_ = s.saveStateLocked()
	s.mu.Unlock()
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
	chunks := splitDiscordMessage(content, 1800)
	if len(chunks) == 0 {
		chunks = []string{"[discord-bridge] empty reply"}
	}
	if s.cfg.DryRun {
		return s.appendAudit("discord.send.dry_run", map[string]any{"channelId": channelID, "chunkCount": len(chunks), "content": content, "replyTo": replyTo})
	}
	for i, chunk := range chunks {
		msg := &discordgo.MessageSend{Content: chunk}
		if i == 0 && replyTo != "" {
			msg.Reference = &discordgo.MessageReference{MessageID: replyTo, ChannelID: channelID}
		}
		if _, err := s.dg.ChannelMessageSendComplex(channelID, msg); err != nil {
			return err
		}
	}
	_ = s.appendAudit("discord.send", map[string]any{"channelId": channelID, "chunkCount": len(chunks), "contentLength": len(content), "replyTo": replyTo})
	return nil
}

func splitDiscordMessage(content string, maxLen int) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	if maxLen <= 0 || len(content) <= maxLen {
		return []string{content}
	}
	paragraphs := strings.Split(content, "\n")
	chunks := make([]string, 0, len(paragraphs))
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(current.String()))
			current.Reset()
		}
	}
	for _, paragraph := range paragraphs {
		paragraph = strings.TrimRight(paragraph, "\r")
		if paragraph == "" {
			if current.Len()+1 > maxLen {
				flush()
			}
			if current.Len() > 0 {
				current.WriteString("\n")
			}
			continue
		}
		for len(paragraph) > maxLen {
			if current.Len() > 0 {
				flush()
			}
			splitAt := maxLen
			if idx := strings.LastIndex(paragraph[:maxLen], " "); idx > maxLen/2 {
				splitAt = idx
			}
			chunks = append(chunks, strings.TrimSpace(paragraph[:splitAt]))
			paragraph = strings.TrimSpace(paragraph[splitAt:])
		}
		candidate := paragraph
		if current.Len() > 0 {
			candidate = current.String() + "\n" + paragraph
		}
		if len(candidate) > maxLen {
			flush()
		}
		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(paragraph)
	}
	flush()
	return chunks
}
