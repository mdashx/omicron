package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("DISCORD_BRIDGE_AGENT_ID", "main")
	t.Setenv("DISCORD_BRIDGE_CHANNEL_ID", "123")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BridgeURL == "" || cfg.Command == "" || cfg.StateRoot == "" {
		t.Fatalf("missing defaults: %+v", cfg)
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	state := HarnessState{ProcessedEventIDs: map[string]bool{"evt_1": true}}
	if err := saveState(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.ProcessedEventIDs["evt_1"] {
		t.Fatalf("state did not round-trip: %+v", loaded)
	}
}

func TestRenderBridgePrompt(t *testing.T) {
	prompt := RenderBridgePrompt(InboundEvent{EventID: "evt_1", AuthorName: "easter", AuthorID: "u1", ChannelID: "c1", Content: "hello"})
	if len(prompt) == 0 || !strings.Contains(prompt, "discord-bridge") || !strings.Contains(prompt, "hello") {
		t.Fatalf("unexpected prompt: %q", prompt)
	}
}

func TestLoadStateMissing(t *testing.T) {
	loaded, err := loadState(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.ProcessedEventIDs) != 0 {
		t.Fatalf("expected empty state, got %+v", loaded)
	}
}

func TestConfigExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got := expandPath("~/example")
	want := filepath.Join(home, "example")
	if got != want {
		t.Fatalf("expandPath mismatch: got %q want %q", got, want)
	}
}
