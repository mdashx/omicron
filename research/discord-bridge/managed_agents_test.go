package main

import (
	"testing"
	"time"
)

func TestManagedAgentViewsCombineManagedLaunchAndBindingState(t *testing.T) {
	s := &BridgeService{
		cfg:     Config{AssignableChannelIDs: []string{"c1"}},
		started: time.Now().Add(-5 * time.Minute),
		state: PersistedState{
			Bindings: map[string]Binding{
				"main": {AgentID: "main", GuildID: "g1", ChannelID: "c1", Active: true, JoinedAt: time.Now().Add(-2 * time.Minute)},
			},
			ManagedAgents: map[string]ManagedAgent{
				"main": {AgentID: "main", DesiredState: "running", LastJoinAt: time.Now().Add(-2 * time.Minute)},
			},
		},
		queues: map[string][]InboundEvent{
			"main": {{EventID: "evt_1"}},
		},
		launchedAgents: map[string]LaunchedAgent{
			"main": {AgentID: "main", ChannelID: "c1", PID: 123, State: "running", StartedAt: time.Now().Add(-3 * time.Minute)},
		},
	}
	views := s.managedAgentViews()
	if len(views) != 1 {
		t.Fatalf("expected 1 view, got %d", len(views))
	}
	view := views[0]
	if view.ProcessState != "running" || view.BridgeState != "bound" || view.WorkState != "queued" {
		t.Fatalf("unexpected view state: %+v", view)
	}
	if !view.CanStop || view.CanStart {
		t.Fatalf("unexpected actions: %+v", view)
	}
}
