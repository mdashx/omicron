package main

import "testing"

func TestAssignChannelLocked(t *testing.T) {
	s := &BridgeService{
		cfg: Config{DefaultGuildID: "g1", AssignableChannelIDs: []string{"c1", "c2"}},
		state: PersistedState{Bindings: map[string]Binding{
			"other": {AgentID: "other", GuildID: "g1", ChannelID: "c1", Active: true},
		}},
	}
	guildID, channelID := s.assignChannelLocked("", "main")
	if guildID != "g1" || channelID != "c2" {
		t.Fatalf("unexpected assignment: guild=%s channel=%s", guildID, channelID)
	}
}
