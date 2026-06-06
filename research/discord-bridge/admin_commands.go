package main

import (
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type adminCommand struct {
	Raw            string
	Name           string
	Args           []string
	ResolvedFrom   string
	IsPassthrough  bool
	PassthroughCmd string
}

func isSlashCommand(content string) bool {
	content = strings.TrimSpace(content)
	return strings.HasPrefix(content, "/") && len(content) > 1
}

func parseAdminCommand(content string) (adminCommand, error) {
	content = strings.TrimSpace(content)
	if !isSlashCommand(content) {
		return adminCommand{}, fmt.Errorf("not a slash command")
	}
	parts := strings.Fields(strings.TrimPrefix(content, "/"))
	if len(parts) == 0 {
		return adminCommand{}, fmt.Errorf("empty command")
	}
	cmd := adminCommand{Raw: content, Name: strings.ToLower(parts[0]), Args: parts[1:]}
	switch cmd.Name {
	case "health":
		cmd.ResolvedFrom = cmd.Name
		cmd.Name = "status"
	case "rooms":
		cmd.ResolvedFrom = cmd.Name
		cmd.Name = "agents"
	case "new", "state", "compact", "model":
		cmd.IsPassthrough = true
		cmd.PassthroughCmd = content
	}
	return cmd, nil
}

func (s *BridgeService) handleSlashCommand(m *discordgo.MessageCreate, hasBinding bool, binding Binding) {
	cmd, err := parseAdminCommand(m.Content)
	if err != nil {
		return
	}
	_ = s.appendAudit("bridge.admin.received", map[string]any{
		"authorId":   m.Author.ID,
		"authorName": m.Author.Username,
		"channelId":  m.ChannelID,
		"guildId":    m.GuildID,
		"command":    m.Content,
	})
	_ = s.setManagedReaction(m.ChannelID, m.ID, s.cfg.AckReaction)
	if !s.isAdminAuthorized(m.Author.ID) {
		_ = s.appendAudit("bridge.admin.denied", map[string]any{
			"authorId":  m.Author.ID,
			"channelId": m.ChannelID,
			"command":   m.Content,
			"reason":    "unauthorized",
		})
		_ = s.setManagedReaction(m.ChannelID, m.ID, failureReaction(s.cfg))
		_ = s.sendChannelMessage(m.ChannelID, bridgeResponse("Command denied.", "You are not authorized to use bridge slash commands here."), m.ID)
		return
	}
	if cmd.ResolvedFrom != "" {
		_ = s.appendAudit("bridge.admin.alias_resolved", map[string]any{
			"authorId":  m.Author.ID,
			"channelId": m.ChannelID,
			"from":      cmd.ResolvedFrom,
			"to":        cmd.Name,
		})
	}
	text, execErr := s.executeAdminCommand(cmd, hasBinding, binding)
	if execErr != nil {
		_ = s.appendAudit("bridge.admin.failed", map[string]any{
			"authorId":  m.Author.ID,
			"channelId": m.ChannelID,
			"command":   m.Content,
			"error":     execErr.Error(),
		})
		_ = s.setManagedReaction(m.ChannelID, m.ID, failureReaction(s.cfg))
		_ = s.sendChannelMessage(m.ChannelID, bridgeResponse("Command failed.", execErr.Error()), m.ID)
		return
	}
	kind := "bridge.admin.executed"
	if cmd.IsPassthrough {
		kind = "bridge.admin.forwarded"
	}
	_ = s.appendAudit(kind, map[string]any{
		"authorId":   m.Author.ID,
		"channelId":  m.ChannelID,
		"command":    m.Content,
		"normalized": cmd.Name,
	})
	_ = s.setManagedReaction(m.ChannelID, m.ID, successReaction(s.cfg))
	_ = s.sendChannelMessage(m.ChannelID, text, m.ID)
}

func (s *BridgeService) executeAdminCommand(cmd adminCommand, hasBinding bool, binding Binding) (string, error) {
	if cmd.IsPassthrough {
		return bridgeResponse("Slash command recognized.", fmt.Sprintf("Passthrough candidate: %s\nBridge policy has not enabled live Pi passthrough yet.", cmd.PassthroughCmd)), nil
	}
	switch cmd.Name {
	case "help":
		return bridgeResponse("Available bridge slash commands",
			"/help\n/status\n/agents\n/bindings\n/activity\n/agent <id> status\n/agent <id> start\n/agent <id> stop\n/agent <id> restart\n/health (alias for /status)\n/rooms (alias for /agents)\n/new, /state, /compact, /model (recognized passthrough candidates)"), nil
	case "status":
		stats := s.overviewStats()
		return bridgeResponse("Bridge status",
			fmt.Sprintf("Discord: %s\nUptime: %s\nManaged agents: %d\nHealthy joined: %d\nQueued events: %d\nNeeds attention: %d\nAssignable channels: %d",
				stats.DiscordStatus, stats.BridgeUptime, stats.ManagedAgents, stats.HealthyJoined, stats.QueuedEvents, stats.NeedsAttention, stats.AssignableChannels)), nil
	case "agents":
		views := s.managedAgentViews()
		if len(views) == 0 {
			return bridgeResponse("Managed agents", "No managed agents registered."), nil
		}
		lines := make([]string, 0, len(views))
		for _, view := range views {
			lines = append(lines, fmt.Sprintf("%s — desired=%s process=%s bridge=%s work=%s channel=%s queue=%d", view.AgentID, view.DesiredState, view.ProcessState, view.BridgeState, view.WorkState, defaultText(view.ChannelID, "unassigned"), view.QueueDepth))
		}
		return bridgeResponse("Managed agents", strings.Join(lines, "\n")), nil
	case "bindings":
		bindings := s.bindingViews()
		if len(bindings) == 0 {
			return bridgeResponse("Bindings", "No active or desired bindings."), nil
		}
		lines := make([]string, 0, len(bindings))
		for _, view := range bindings {
			lines = append(lines, fmt.Sprintf("%s — %s — %s", view.AgentID, view.State, defaultText(view.ChannelLabel, view.ChannelID)))
		}
		return bridgeResponse("Bindings", strings.Join(lines, "\n")), nil
	case "activity":
		items := s.activityItems(10)
		if len(items) == 0 {
			return bridgeResponse("Activity", "No recent activity."), nil
		}
		lines := make([]string, 0, len(items))
		for _, item := range items {
			lines = append(lines, fmt.Sprintf("%s — %s — %s", item.Timestamp.Local().Format("15:04:05"), item.Type, item.Summary))
		}
		return bridgeResponse("Recent activity", strings.Join(lines, "\n")), nil
	case "agent":
		return s.executeAgentSubcommand(cmd.Args, hasBinding, binding)
	default:
		return "", fmt.Errorf("unknown bridge command: /%s", cmd.Name)
	}
}

func (s *BridgeService) executeAgentSubcommand(args []string, hasBinding bool, binding Binding) (string, error) {
	if len(args) == 0 {
		if hasBinding {
			args = []string{binding.AgentID, "status"}
		} else {
			return "", fmt.Errorf("usage: /agent <id> <status|start|stop|restart>")
		}
	}
	agentID := args[0]
	action := "status"
	if len(args) > 1 {
		action = strings.ToLower(args[1])
	}
	if agentID == "status" && hasBinding {
		agentID = binding.AgentID
		action = "status"
	}
	switch action {
	case "status":
		view, ok := s.findManagedAgentView(agentID)
		if !ok {
			return "", fmt.Errorf("managed agent not found: %s", agentID)
		}
		return bridgeResponse("Agent status", fmt.Sprintf("Agent: %s\nDesired: %s\nProcess: %s\nBridge: %s\nWork: %s\nChannel: %s\nQueue: %d\nPID: %d\nLast activity: %s\nLast error: %s", view.AgentID, view.DesiredState, view.ProcessState, view.BridgeState, view.WorkState, defaultText(view.ChannelID, "unassigned"), view.QueueDepth, view.PID, relativeTimeText(view.LastActivityAt), defaultText(view.LastError, "none"))), nil
	case "start":
		_, err := s.launchManagedAgent(agentID)
		if err != nil {
			return "", err
		}
		return bridgeResponse("Agent started", agentID), nil
	case "stop":
		if _, err := s.stopAgent(agentID); err != nil {
			return "", err
		}
		return bridgeResponse("Agent stopped", agentID), nil
	case "restart":
		_, _ = s.stopAgent(agentID)
		_, err := s.launchManagedAgent(agentID)
		if err != nil {
			return "", err
		}
		return bridgeResponse("Agent restarted", agentID), nil
	default:
		return "", fmt.Errorf("unknown agent action: %s", action)
	}
}

func (s *BridgeService) launchManagedAgent(agentID string) (LaunchedAgent, error) {
	s.mu.Lock()
	agent, ok := s.state.ManagedAgents[agentID]
	s.mu.Unlock()
	if !ok {
		return LaunchedAgent{}, fmt.Errorf("managed agent not found: %s", agentID)
	}
	return s.launchAgent(launchAgentRequest{
		AgentID:    agentID,
		GuildID:    agent.RequestedGuildID,
		ChannelID:  agent.RequestedChannelID,
		Command:    agent.Command,
		Args:       append([]string(nil), agent.Args...),
		WorkingDir: agent.WorkingDir,
	})
}

func (s *BridgeService) isAdminAuthorized(userID string) bool {
	s.mu.Lock()
	allowed := append([]string(nil), s.cfg.AdminUserIDs...)
	s.mu.Unlock()
	if len(allowed) == 0 {
		return false
	}
	for _, id := range allowed {
		if strings.TrimSpace(id) == userID {
			return true
		}
	}
	return false
}

func bridgeResponse(title, body string) string {
	if strings.TrimSpace(body) == "" {
		return "[discord-bridge]\n" + title
	}
	return fmt.Sprintf("[discord-bridge]\n%s\n\n%s", title, body)
}

func defaultText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func relativeTimeText(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return time.Since(t).Round(time.Second).String() + " ago"
}

func successReaction(cfg Config) string {
	for _, reaction := range cfg.FinalReactionChoices {
		if strings.TrimSpace(reaction) != "" {
			return reaction
		}
	}
	return "✅"
}

func failureReaction(cfg Config) string {
	for _, reaction := range cfg.StatusReactions {
		if reaction == "⚠️" {
			return reaction
		}
	}
	return "⚠️"
}
