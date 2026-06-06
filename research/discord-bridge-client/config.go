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
	Enabled              bool          `json:"enabled"`
	BridgeURL            string        `json:"bridgeUrl"`
	AgentID              string        `json:"agentId"`
	CredsRef             string        `json:"credsRef"`
	GuildID              string        `json:"guildId,omitempty"`
	ChannelID            string        `json:"channelId"`
	Command              string        `json:"command"`
	CommandArgs          []string      `json:"commandArgs,omitempty"`
	Cwd                  string        `json:"cwd"`
	PollInterval         time.Duration `json:"pollIntervalMs"`
	IdleCompleteWait     time.Duration `json:"idleCompleteMs"`
	Cols                 uint16        `json:"cols"`
	Rows                 uint16        `json:"rows"`
	StateRoot            string        `json:"stateRoot"`
	StatePath            string        `json:"statePath"`
	QueueReaction        string        `json:"queueReaction"`
	ThinkingReaction     string        `json:"thinkingReaction"`
	FinalReaction        string        `json:"finalReaction"`
	OutputMode           string        `json:"outputMode"`
	PiSessionRoot        string        `json:"piSessionRoot"`
	PiSessionArchiveRoot string        `json:"piSessionArchiveRoot"`
	PiLogPreference      string        `json:"piLogPreference"`
	PTYInputLogPath      string        `json:"ptyInputLogPath"`
	PTYOutputLogPath     string        `json:"ptyOutputLogPath"`
}

func LoadConfig() (Config, error) {
	root := expandPath("~/.pi/discord-bridge-client")
	cwd, _ := os.Getwd()
	cfg := Config{
		Enabled:              true,
		BridgeURL:            "http://127.0.0.1:19444",
		CredsRef:             "local-session",
		Command:              "pi",
		Cwd:                  cwd,
		PollInterval:         1500 * time.Millisecond,
		IdleCompleteWait:     2500 * time.Millisecond,
		Cols:                 120,
		Rows:                 40,
		StateRoot:            root,
		StatePath:            filepath.Join(root, "state.json"),
		QueueReaction:        "⏳",
		ThinkingReaction:     "💭",
		FinalReaction:        "✅",
		OutputMode:           "pi-jsonl",
		PiSessionRoot:        expandPath("~/.pi/agent/sessions"),
		PiSessionArchiveRoot: expandPath("~/.pi/agent/session-archive"),
		PiLogPreference:      "session",
		PTYInputLogPath:      filepath.Join(root, "pty-input.log"),
		PTYOutputLogPath:     filepath.Join(root, "pty-output.log"),
	}
	configPath := expandPath(envOr("DISCORD_BRIDGE_CLIENT_CONFIG", "~/.pi/discord-bridge-client/config.json"))
	if raw, err := os.ReadFile(configPath); err == nil {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return cfg, fmt.Errorf("parse client config: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return cfg, fmt.Errorf("read client config: %w", err)
	}
	cfg.applyEnvOverrides(cwd)
	if cfg.StateRoot == "" {
		cfg.StateRoot = root
	}
	if cfg.StatePath == "" {
		cfg.StatePath = filepath.Join(cfg.StateRoot, "state.json")
	}
	cfg.StateRoot = expandPath(cfg.StateRoot)
	cfg.StatePath = expandPath(cfg.StatePath)
	cfg.Cwd = expandPath(cfg.Cwd)
	cfg.PiSessionRoot = expandPath(cfg.PiSessionRoot)
	cfg.PiSessionArchiveRoot = expandPath(cfg.PiSessionArchiveRoot)
	cfg.PTYInputLogPath = expandPath(cfg.PTYInputLogPath)
	cfg.PTYOutputLogPath = expandPath(cfg.PTYOutputLogPath)
	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if !c.Enabled {
		return errors.New("client harness must remain enabled for this prototype")
	}
	if c.BridgeURL == "" || c.AgentID == "" || c.CredsRef == "" {
		return errors.New("bridgeUrl, agentId, and credsRef are required")
	}
	if c.Command == "" || c.Cwd == "" {
		return errors.New("command and cwd are required")
	}
	if c.PollInterval <= 0 || c.IdleCompleteWait <= 0 {
		return errors.New("poll and idle completion timings must be > 0")
	}
	if c.OutputMode == "" {
		return errors.New("outputMode is required")
	}
	if c.PTYInputLogPath == "" || c.PTYOutputLogPath == "" {
		return errors.New("pty transcript log paths are required")
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

func (c *Config) applyEnvOverrides(cwd string) {
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_URL")); value != "" {
		c.BridgeURL = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_AGENT_ID")); value != "" {
		c.AgentID = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_CREDS_REF")); value != "" {
		c.CredsRef = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_GUILD_ID")); value != "" {
		c.GuildID = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_CHANNEL_ID")); value != "" {
		c.ChannelID = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_COMMAND")); value != "" {
		c.Command = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_COMMAND_ARGS")); value != "" {
		c.CommandArgs = splitArgs(value)
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_CWD")); value != "" {
		c.Cwd = value
	} else if c.Cwd == "" {
		c.Cwd = cwd
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_CLIENT_STATE_ROOT")); value != "" {
		c.StateRoot = value
		c.StatePath = filepath.Join(expandPath(value), "state.json")
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_POLL_INTERVAL_MS")); value != "" {
		c.PollInterval = time.Duration(envOrInt("DISCORD_BRIDGE_POLL_INTERVAL_MS", 1500)) * time.Millisecond
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_IDLE_COMPLETE_MS")); value != "" {
		c.IdleCompleteWait = time.Duration(envOrInt("DISCORD_BRIDGE_IDLE_COMPLETE_MS", 2500)) * time.Millisecond
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_COLS")); value != "" {
		c.Cols = uint16(envOrInt("DISCORD_BRIDGE_COLS", 120))
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_ROWS")); value != "" {
		c.Rows = uint16(envOrInt("DISCORD_BRIDGE_ROWS", 40))
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_QUEUE_REACTION")); value != "" {
		c.QueueReaction = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_THINKING_REACTION")); value != "" {
		c.ThinkingReaction = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_FINAL_REACTION")); value != "" {
		c.FinalReaction = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_OUTPUT_MODE")); value != "" {
		c.OutputMode = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_PI_SESSION_ROOT")); value != "" {
		c.PiSessionRoot = expandPath(value)
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_PI_SESSION_ARCHIVE_ROOT")); value != "" {
		c.PiSessionArchiveRoot = expandPath(value)
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_PI_LOG_PREFERENCE")); value != "" {
		c.PiLogPreference = value
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_PTY_INPUT_LOG")); value != "" {
		c.PTYInputLogPath = expandPath(value)
	}
	if value := strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_PTY_OUTPUT_LOG")); value != "" {
		c.PTYOutputLogPath = expandPath(value)
	}
}
