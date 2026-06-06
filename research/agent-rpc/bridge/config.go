package bridge

import (
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	Enabled            bool
	BridgeURL          string
	AgentID            string
	CredsRef           string
	GuildID            string
	ChannelID          string
	PollIntervalMs     int
	StateRoot          string
	InProgressReaction string
	SuccessReaction    string
	FailureReaction    string
	Scope              []string
}

func DefaultConfig() Config {
	return Config{
		Enabled:            false,
		BridgeURL:          envOr("DISCORD_BRIDGE_URL", "http://127.0.0.1:19444"),
		AgentID:            envOr("DISCORD_BRIDGE_AGENT_ID", "main"),
		CredsRef:           strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_CREDS_REF")),
		GuildID:            strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_GUILD_ID")),
		ChannelID:          strings.TrimSpace(os.Getenv("DISCORD_BRIDGE_CHANNEL_ID")),
		PollIntervalMs:     envOrInt("DISCORD_BRIDGE_POLL_INTERVAL_MS", 1500),
		StateRoot:          expandPath(envOr("DISCORD_BRIDGE_CLIENT_STATE_ROOT", "~/.pi/agent-rpc")),
		InProgressReaction: envOr("DISCORD_BRIDGE_IN_PROGRESS_REACTION", "💭"),
		SuccessReaction:    envOr("DISCORD_BRIDGE_SUCCESS_REACTION", "✅"),
		FailureReaction:    envOr("DISCORD_BRIDGE_FAILURE_REACTION", "⚠️"),
		Scope:              []string{"channel.listen", "channel.reply"},
	}
}

func (c Config) Resolved() Config {
	resolved := c
	if resolved.BridgeURL == "" {
		resolved.BridgeURL = "http://127.0.0.1:19444"
	}
	if resolved.AgentID == "" {
		resolved.AgentID = "main"
	}
	if resolved.PollIntervalMs <= 0 {
		resolved.PollIntervalMs = 1500
	}
	if resolved.StateRoot == "" {
		resolved.StateRoot = expandPath("~/.pi/agent-rpc")
	}
	resolved.StateRoot = expandPath(resolved.StateRoot)
	return resolved
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
		if _, err := fmtSscanf(value, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
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

var fmtSscanf = func(str string, format string, a ...any) (int, error) {
	return fmtSscanfImpl(str, format, a...)
}
