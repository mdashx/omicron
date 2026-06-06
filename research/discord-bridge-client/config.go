package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Config struct {
	Enabled          bool          `json:"enabled"`
	BridgeURL        string        `json:"bridgeUrl"`
	AgentID          string        `json:"agentId"`
	CredsRef         string        `json:"credsRef"`
	GuildID          string        `json:"guildId,omitempty"`
	ChannelID        string        `json:"channelId"`
	Command          string        `json:"command"`
	CommandArgs      []string      `json:"commandArgs,omitempty"`
	Cwd              string        `json:"cwd"`
	PollInterval     time.Duration `json:"pollIntervalMs"`
	IdleCompleteWait time.Duration `json:"idleCompleteMs"`
	Cols             uint16        `json:"cols"`
	Rows             uint16        `json:"rows"`
	StateRoot        string        `json:"stateRoot"`
	StatePath        string        `json:"statePath"`
	QueueReaction    string        `json:"queueReaction"`
	ThinkingReaction string        `json:"thinkingReaction"`
	FinalReaction    string        `json:"finalReaction"`
}

func LoadConfig() (Config, error) {
	root := expandPath(envOr("DISCORD_BRIDGE_CLIENT_STATE_ROOT", "~/.pi/discord-bridge-client"))
	cwd, _ := os.Getwd()
	cfg := Config{
		Enabled:          true,
		BridgeURL:        envOr("DISCORD_BRIDGE_URL", "http://127.0.0.1:19444"),
		AgentID:          strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_AGENT_ID")),
		CredsRef:         envOr("DISCORD_BRIDGE_CREDS_REF", "local-session"),
		GuildID:          strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_GUILD_ID")),
		ChannelID:        strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_CHANNEL_ID")),
		Command:          envOr("DISCORD_BRIDGE_COMMAND", "pi"),
		CommandArgs:      splitArgs(os.Getenv("DISCORD_BRIDGE_COMMAND_ARGS")),
		Cwd:              expandPath(envOr("DISCORD_BRIDGE_CWD", cwd)),
		PollInterval:     time.Duration(envOrInt("DISCORD_BRIDGE_POLL_INTERVAL_MS", 1500)) * time.Millisecond,
		IdleCompleteWait: time.Duration(envOrInt("DISCORD_BRIDGE_IDLE_COMPLETE_MS", 2500)) * time.Millisecond,
		Cols:             uint16(envOrInt("DISCORD_BRIDGE_COLS", 120)),
		Rows:             uint16(envOrInt("DISCORD_BRIDGE_ROWS", 40)),
		StateRoot:        root,
		StatePath:        filepath.Join(root, "state.json"),
		QueueReaction:    envOr("DISCORD_BRIDGE_QUEUE_REACTION", "⏳"),
		ThinkingReaction: envOr("DISCORD_BRIDGE_THINKING_REACTION", "💭"),
		FinalReaction:    envOr("DISCORD_BRIDGE_FINAL_REACTION", "✅"),
	}
	if path := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_CLIENT_CONFIG")); path != "" {
		raw, err := os.ReadFile(expandPath(path))
		if err != nil {
			return cfg, fmt.Errorf("read client config: %w", err)
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return cfg, fmt.Errorf("parse client config: %w", err)
		}
	}
	cfg.StateRoot = expandPath(cfg.StateRoot)
	cfg.StatePath = expandPath(cfg.StatePath)
	cfg.Cwd = expandPath(cfg.Cwd)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if !c.Enabled {
		return errors.New("client harness must remain enabled for this prototype")
	}
	if c.BridgeURL == "" || c.AgentID == "" || c.CredsRef == "" || c.ChannelID == "" {
		return errors.New("bridgeUrl, agentId, credsRef, and channelId are required")
	}
	if c.Command == "" || c.Cwd == "" {
		return errors.New("command and cwd are required")
	}
	if c.PollInterval <= 0 || c.IdleCompleteWait <= 0 {
		return errors.New("poll and idle completion timings must be > 0")
	}
	if c.Cols == 0 || c.Rows == 0 {
		return errors.New("pty cols and rows must be > 0")
	}
	return nil
}

func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envOrInt(key string, fallback int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func splitArgs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	return strings.Fields(raw)
}
