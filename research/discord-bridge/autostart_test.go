package main

import "testing"

func TestEnsureManagedAgentsForEnabledChannelsCreatesOnePerChannel(t *testing.T) {
	s := &BridgeService{
		cfg: Config{
			DefaultGuildID:           "g1",
			AssignableChannelIDs:     []string{"c1", "c2"},
			AutoStartEnabledChannels: true,
			AutoStartAgentPrefix:     "room",
			StatePath:                t.TempDir() + "/state.json",
		},
		state: PersistedState{
			Bindings:            map[string]Binding{},
			ProcessedMessageIDs: map[string]int64{},
			ManagedReactions:    map[string]string{},
			ManagedAgents:       map[string]ManagedAgent{},
		},
		queues:         map[string][]InboundEvent{},
		launchedAgents: map[string]LaunchedAgent{},
	}
	ids, err := s.ensureManagedAgentsForEnabledChannels()
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 managed agents, got %d", len(ids))
	}
	for _, channelID := range []string{"c1", "c2"} {
		agentID := "room-" + channelID
		agent, ok := s.state.ManagedAgents[agentID]
		if !ok {
			t.Fatalf("missing managed agent for channel %s", channelID)
		}
		if agent.RequestedChannelID != channelID || agent.DesiredState != "running" || agent.CredsRef != "local-session" {
			t.Fatalf("unexpected managed agent: %+v", agent)
		}
	}
}
