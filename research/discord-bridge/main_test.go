package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestConfigDefaults(t *testing.T) {
	t.Setenv("DISCORD_BOT_TOKEN", "token")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BridgeID == "" || cfg.StorageRoot == "" || cfg.AckReaction == "" {
		t.Fatalf("missing defaults: %+v", cfg)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	state := PersistedState{
		Bindings: map[string]Binding{
			"a": {AgentID: "a", ChannelID: "c", Active: true},
		},
		ProcessedMessageIDs: map[string]int64{"m": 1},
		ManagedReactions:    map[string]string{"m": "✅"},
	}
	raw, err := jsonMarshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Bindings["a"].ChannelID != "c" || loaded.ManagedReactions["m"] != "✅" {
		t.Fatalf("bad state round trip: %+v", loaded)
	}
}

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}
