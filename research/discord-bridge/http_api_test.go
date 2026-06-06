package main

import (
	"strings"
	"testing"
)

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

func TestSplitDiscordMessage(t *testing.T) {
	input := strings.Repeat("word ", 900)
	chunks := splitDiscordMessage(input, 200)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	for i, chunk := range chunks {
		if len(chunk) > 200 {
			t.Fatalf("chunk %d too long: %d", i, len(chunk))
		}
	}
}
