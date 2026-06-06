package harness

import (
	"os"
	"path/filepath"
)

type Config struct {
	AgentID    string
	Command    string
	Args       []string
	Cwd        string
	SessionDir string
	NoSession  bool
	Debug      bool
}

func DefaultConfig() Config {
	cwd, _ := os.Getwd()
	return Config{
		AgentID: "main",
		Command: "pi",
		Args:    []string{"--mode", "rpc"},
		Cwd:     cwd,
	}
}

func (c Config) Resolved() Config {
	resolved := c
	if resolved.AgentID == "" {
		resolved.AgentID = "main"
	}
	if resolved.Command == "" {
		resolved.Command = "pi"
	}
	if len(resolved.Args) == 0 {
		resolved.Args = []string{"--mode", "rpc"}
	}
	if resolved.Cwd == "" {
		resolved.Cwd, _ = os.Getwd()
	}
	if resolved.Cwd != "" {
		if abs, err := filepath.Abs(resolved.Cwd); err == nil {
			resolved.Cwd = abs
		}
	}
	return resolved
}
