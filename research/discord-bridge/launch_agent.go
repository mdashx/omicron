package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func (s *BridgeService) launchAgent(req launchAgentRequest) (LaunchedAgent, error) {
	s.mu.Lock()
	if existing, ok := s.launchedAgents[req.AgentID]; ok && existing.State == "running" {
		s.mu.Unlock()
		return LaunchedAgent{}, fmt.Errorf("agent %q already launched", req.AgentID)
	}
	requestedGuildID := strings.TrimSpace(req.GuildID)
	requestedChannelID := strings.TrimSpace(req.ChannelID)
	if existing, ok := s.state.Bindings[req.AgentID]; ok && existing.Active {
		if requestedGuildID == "" {
			requestedGuildID = existing.GuildID
		}
		if requestedChannelID == "" {
			requestedChannelID = existing.ChannelID
		}
	}
	if requestedChannelID == "" {
		requestedGuildID, requestedChannelID = s.assignChannelLocked(requestedGuildID, req.AgentID)
		if requestedChannelID == "" {
			s.mu.Unlock()
			return LaunchedAgent{}, fmt.Errorf("no assignable channel available")
		}
	}
	s.mu.Unlock()

	command := strings.TrimSpace(req.Command)
	if command == "" {
		command = "discoagent"
	}
	agentDir := filepath.Join(s.cfg.StorageRoot, "launched-agents", req.AgentID)
	if err := os.MkdirAll(agentDir, 0o755); err != nil {
		return LaunchedAgent{}, err
	}
	logPath := filepath.Join(agentDir, "stdout.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return LaunchedAgent{}, err
	}
	cmd := exec.Command(command)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"DISCORD_BRIDGE_AGENT_ID="+req.AgentID,
		"DISCORD_BRIDGE_CREDS_REF=local-session",
		"DISCORD_BRIDGE_URL=http://127.0.0.1:19444",
		"DISCORD_BRIDGE_GUILD_ID="+requestedGuildID,
		"DISCORD_BRIDGE_CHANNEL_ID="+requestedChannelID,
		"DISCORD_BRIDGE_PTY_INPUT_LOG="+filepath.Join(agentDir, "pty-input.log"),
		"DISCORD_BRIDGE_PTY_OUTPUT_LOG="+filepath.Join(agentDir, "pty-output.log"),
	)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return LaunchedAgent{}, err
	}
	_ = logFile.Close()
	launched := LaunchedAgent{
		AgentID:   req.AgentID,
		GuildID:   requestedGuildID,
		ChannelID: requestedChannelID,
		Command:   command,
		PID:       cmd.Process.Pid,
		StartedAt: time.Now().UTC(),
		LogPath:   logPath,
		State:     "running",
	}
	s.mu.Lock()
	s.launchedAgents[req.AgentID] = launched
	s.mu.Unlock()
	_ = s.appendAudit("agent.launch", launched)
	go func(agentID string, process *os.Process) {
		state, err := process.Wait()
		s.mu.Lock()
		launched := s.launchedAgents[agentID]
		if err != nil {
			launched.State = "error"
		} else if state.Success() {
			launched.State = "exited"
		} else {
			launched.State = fmt.Sprintf("exited:%d", state.ExitCode())
		}
		s.launchedAgents[agentID] = launched
		s.mu.Unlock()
		_ = s.appendAudit("agent.launch.exit", map[string]any{"agentId": agentID, "state": launched.State})
	}(req.AgentID, cmd.Process)
	return launched, nil
}
