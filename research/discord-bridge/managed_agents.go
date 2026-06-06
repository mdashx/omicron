package main

import (
	"sort"
	"strings"
	"time"
)

type ManagedAgentView struct {
	AgentID          string
	DesiredState     string
	ProcessState     string
	BridgeState      string
	WorkState        string
	GuildID          string
	ChannelID        string
	QueueDepth       int
	PID              int
	CommandLabel     string
	LastActivityAt   time.Time
	LastCompletionAt time.Time
	LastJoinAt       time.Time
	LastError        string
	NeedsAttention   bool
	BindingActive    bool
	CanStart         bool
	CanStop          bool
	CanRestart       bool
	Summary          string
	LogPath          string
}

type OverviewStats struct {
	DiscordStatus      string
	BridgeUptime       string
	ManagedAgents      int
	HealthyJoined      int
	QueuedEvents       int
	NeedsAttention     int
	AssignableChannels int
}

func (s *BridgeService) upsertManagedAgentLocked(agent ManagedAgent) ManagedAgent {
	existing := s.state.ManagedAgents[agent.AgentID]
	if agent.AgentID == "" {
		agent.AgentID = existing.AgentID
	}
	if agent.CredsRef == "" {
		agent.CredsRef = existing.CredsRef
	}
	if agent.DesiredState == "" {
		agent.DesiredState = existing.DesiredState
	}
	if agent.Command == "" {
		agent.Command = existing.Command
	}
	if len(agent.Args) == 0 {
		agent.Args = append([]string(nil), existing.Args...)
	}
	if agent.WorkingDir == "" {
		agent.WorkingDir = existing.WorkingDir
	}
	if agent.RequestedGuildID == "" {
		agent.RequestedGuildID = existing.RequestedGuildID
	}
	if agent.RequestedChannelID == "" {
		agent.RequestedChannelID = existing.RequestedChannelID
	}
	if !agent.AutoLaunch {
		agent.AutoLaunch = existing.AutoLaunch
	}
	if agent.LastJoinAt.IsZero() {
		agent.LastJoinAt = existing.LastJoinAt
	}
	if agent.LastQueuedAt.IsZero() {
		agent.LastQueuedAt = existing.LastQueuedAt
	}
	if agent.LastCompletionAt.IsZero() {
		agent.LastCompletionAt = existing.LastCompletionAt
	}
	if agent.LastLaunchedAt.IsZero() {
		agent.LastLaunchedAt = existing.LastLaunchedAt
	}
	if agent.LastStoppedAt.IsZero() {
		agent.LastStoppedAt = existing.LastStoppedAt
	}
	if agent.LastProcessPID == 0 {
		agent.LastProcessPID = existing.LastProcessPID
	}
	if agent.LastObservedProcess == "" {
		agent.LastObservedProcess = existing.LastObservedProcess
	}
	if agent.LastError == "" {
		agent.LastError = existing.LastError
	}
	if agent.DesiredState == "" {
		agent.DesiredState = "stopped"
	}
	s.state.ManagedAgents[agent.AgentID] = agent
	return agent
}

func (s *BridgeService) managedAgentViews() []ManagedAgentView {
	s.mu.Lock()
	managed := make(map[string]ManagedAgent, len(s.state.ManagedAgents))
	for k, v := range s.state.ManagedAgents {
		managed[k] = v
	}
	bindings := make(map[string]Binding, len(s.state.Bindings))
	for k, v := range s.state.Bindings {
		bindings[k] = v
	}
	queues := make(map[string]int, len(s.queues))
	for k, v := range s.queues {
		queues[k] = len(v)
	}
	launched := make(map[string]LaunchedAgent, len(s.launchedAgents))
	for k, v := range s.launchedAgents {
		launched[k] = v
	}
	s.mu.Unlock()

	seen := map[string]bool{}
	for k := range bindings {
		seen[k] = true
		if _, ok := managed[k]; !ok {
			managed[k] = ManagedAgent{AgentID: k, DesiredState: "running"}
		}
	}
	for k := range launched {
		seen[k] = true
		if _, ok := managed[k]; !ok {
			managed[k] = ManagedAgent{AgentID: k, DesiredState: "running"}
		}
	}
	views := make([]ManagedAgentView, 0, len(managed))
	for agentID, agent := range managed {
		binding, hasBinding := bindings[agentID]
		launch, hasLaunch := launched[agentID]
		queueDepth := queues[agentID]
		processState := normalizeProcessState(agent, launch, hasLaunch)
		bridgeState := normalizeBridgeState(agent, binding, hasBinding, processState)
		workState := normalizeWorkState(queueDepth, processState)
		lastActivity := maxTime(agent.LastQueuedAt, agent.LastCompletionAt, agent.LastJoinAt, agent.LastLaunchedAt, launch.StartedAt)
		cmdLabel := strings.TrimSpace(strings.Join(append([]string{firstNonEmpty(launch.Command, agent.Command)}, firstNonEmptySlice(launch.Args, agent.Args)...), " "))
		if cmdLabel == "" {
			cmdLabel = "agent-rpc --bridge"
		}
		needsAttention := agent.LastError != "" || (agent.DesiredState == "running" && (processState == "failed" || processState == "exited" || bridgeState == "stale"))
		views = append(views, ManagedAgentView{
			AgentID:          agentID,
			DesiredState:     firstNonEmpty(agent.DesiredState, "stopped"),
			ProcessState:     processState,
			BridgeState:      bridgeState,
			WorkState:        workState,
			GuildID:          firstNonEmpty(binding.GuildID, launch.GuildID, agent.RequestedGuildID),
			ChannelID:        firstNonEmpty(binding.ChannelID, launch.ChannelID, agent.RequestedChannelID),
			QueueDepth:       queueDepth,
			PID:              firstNonZero(launch.PID, agent.LastProcessPID),
			CommandLabel:     cmdLabel,
			LastActivityAt:   lastActivity,
			LastCompletionAt: agent.LastCompletionAt,
			LastJoinAt:       agent.LastJoinAt,
			LastError:        agent.LastError,
			NeedsAttention:   needsAttention,
			BindingActive:    hasBinding && binding.Active,
			CanStart:         processState != "running" && agent.DesiredState != "disabled",
			CanStop:          processState == "running" || processState == "starting",
			CanRestart:       agent.DesiredState != "disabled",
			Summary:          summarizeManagedAgent(agentID, processState, bridgeState, queueDepth),
			LogPath:          launch.LogPath,
		})
	}
	sort.Slice(views, func(i, j int) bool {
		if views[i].NeedsAttention != views[j].NeedsAttention {
			return views[i].NeedsAttention
		}
		return views[i].AgentID < views[j].AgentID
	})
	return views
}

func (s *BridgeService) overviewStats() OverviewStats {
	views := s.managedAgentViews()
	healthy := 0
	queued := 0
	attention := 0
	for _, view := range views {
		queued += view.QueueDepth
		if view.BridgeState == "bound" && view.ProcessState == "running" {
			healthy++
		}
		if view.NeedsAttention {
			attention++
		}
	}
	discordStatus := "disconnected"
	s.mu.Lock()
	if s.dg != nil {
		discordStatus = "connected"
	}
	assignable := len(s.cfg.AssignableChannelIDs)
	started := s.started
	s.mu.Unlock()
	return OverviewStats{
		DiscordStatus:      discordStatus,
		BridgeUptime:       time.Since(started).Round(time.Second).String(),
		ManagedAgents:      len(views),
		HealthyJoined:      healthy,
		QueuedEvents:       queued,
		NeedsAttention:     attention,
		AssignableChannels: assignable,
	}
}

func normalizeProcessState(agent ManagedAgent, launch LaunchedAgent, hasLaunch bool) string {
	state := strings.TrimSpace(agent.LastObservedProcess)
	if hasLaunch {
		state = launch.State
	}
	switch {
	case state == "running":
		return "running"
	case state == "stopped":
		return "not_started"
	case state == "stopping":
		return "stopping"
	case strings.HasPrefix(state, "exited"):
		return "exited"
	case state == "error", state == "failed":
		return "failed"
	case state == "starting":
		return "starting"
	case state == "":
		return "not_started"
	default:
		return "unknown"
	}
}

func normalizeBridgeState(agent ManagedAgent, binding Binding, hasBinding bool, processState string) string {
	if hasBinding && binding.Active && processState == "running" {
		return "bound"
	}
	if hasBinding && binding.Active {
		return "stale"
	}
	if !agent.LastJoinAt.IsZero() && processState == "running" {
		return "joined"
	}
	if processState == "running" {
		return "joining"
	}
	if !agent.LastJoinAt.IsZero() {
		return "stale"
	}
	return "never_joined"
}

func normalizeWorkState(queueDepth int, processState string) string {
	if queueDepth > 0 {
		return "queued"
	}
	if processState == "running" {
		return "idle"
	}
	if processState == "failed" {
		return "errored"
	}
	return "idle"
}

func summarizeManagedAgent(agentID, processState, bridgeState string, queueDepth int) string {
	parts := []string{agentID, processState, bridgeState}
	if queueDepth > 0 {
		parts = append(parts, "queued")
	}
	return strings.Join(parts, " · ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptySlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func maxTime(values ...time.Time) time.Time {
	var max time.Time
	for _, value := range values {
		if value.After(max) {
			max = value
		}
	}
	return max
}
