package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type launchSpec struct {
	Command    string
	Args       []string
	WorkingDir string
}

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

	repoRoot, _ := findOmicronRoot()
	spec, err := resolveLaunchSpec(req, repoRoot)
	if err != nil {
		return LaunchedAgent{}, err
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
	cmd := exec.Command(spec.Command, spec.Args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if spec.WorkingDir != "" {
		cmd.Dir = spec.WorkingDir
	}
	piSessionDir := filepath.Join(agentDir, "pi-sessions")
	clientStateRoot := filepath.Join(agentDir, "client-state")
	cmd.Env = append(os.Environ(),
		"DISCORD_BRIDGE_AGENT_ID="+req.AgentID,
		"DISCORD_BRIDGE_CREDS_REF=local-session",
		"DISCORD_BRIDGE_URL=http://127.0.0.1:19444",
		"DISCORD_BRIDGE_GUILD_ID="+requestedGuildID,
		"DISCORD_BRIDGE_CHANNEL_ID="+requestedChannelID,
		"DISCORD_BRIDGE_CLIENT_STATE_ROOT="+clientStateRoot,
		"DISCORD_BRIDGE_PI_SESSION_ROOT="+piSessionDir,
		"PI_CODING_AGENT_SESSION_DIR="+piSessionDir,
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
		AgentID:    req.AgentID,
		GuildID:    requestedGuildID,
		ChannelID:  requestedChannelID,
		Command:    spec.Command,
		Args:       append([]string(nil), spec.Args...),
		WorkingDir: spec.WorkingDir,
		PID:        cmd.Process.Pid,
		StartedAt:  time.Now().UTC(),
		LogPath:    logPath,
		State:      "running",
	}
	s.mu.Lock()
	s.launchedAgents[req.AgentID] = launched
	managed := s.upsertManagedAgentLocked(ManagedAgent{
		AgentID:             req.AgentID,
		CredsRef:            "local-session",
		DesiredState:        "running",
		Command:             spec.Command,
		Args:                append([]string(nil), spec.Args...),
		WorkingDir:          spec.WorkingDir,
		RequestedGuildID:    requestedGuildID,
		RequestedChannelID:  requestedChannelID,
		LastLaunchedAt:      launched.StartedAt,
		LastProcessPID:      launched.PID,
		LastObservedProcess: "running",
		LastError:           "",
	})
	s.state.ManagedAgents[req.AgentID] = managed
	_ = s.saveStateLocked()
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
		managed := s.upsertManagedAgentLocked(ManagedAgent{AgentID: agentID, DesiredState: "running"})
		managed.LastObservedProcess = launched.State
		if err != nil {
			managed.LastError = err.Error()
		}
		s.state.ManagedAgents[agentID] = managed
		_ = s.saveStateLocked()
		s.mu.Unlock()
		_ = s.appendAudit("agent.launch.exit", map[string]any{"agentId": agentID, "state": launched.State})
	}(req.AgentID, cmd.Process)
	return launched, nil
}

func resolveLaunchSpec(req launchAgentRequest, repoRoot string) (launchSpec, error) {
	command := strings.TrimSpace(req.Command)
	args := compactArgs(req.Args)
	workingDir := strings.TrimSpace(req.WorkingDir)
	if command == "" {
		if repoRoot != "" {
			agentRPCRoot := filepath.Join(repoRoot, "research", "agent-rpc")
			return launchSpec{
				Command:    "go",
				Args:       []string{"run", "./cmd/agent-rpc", "--bridge"},
				WorkingDir: firstNonEmpty(workingDir, agentRPCRoot),
			}, nil
		}
		return launchSpec{Command: "agent-rpc", Args: []string{"--bridge"}, WorkingDir: workingDir}, nil
	}
	if command == "agent-rpc" && len(args) == 0 {
		args = []string{"--bridge"}
	}
	if workingDir == "" {
		workingDir = repoRootIfGoRun(command, args, repoRoot)
	}
	return launchSpec{Command: command, Args: args, WorkingDir: workingDir}, nil
}

func compactArgs(args []string) []string {
	result := make([]string, 0, len(args))
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		if arg != "" {
			result = append(result, arg)
		}
	}
	return result
}

func repoRootIfGoRun(command string, args []string, repoRoot string) string {
	if repoRoot == "" || command != "go" || len(args) < 2 {
		return ""
	}
	if args[0] == "run" && args[1] == "./cmd/agent-rpc" {
		return filepath.Join(repoRoot, "research", "agent-rpc")
	}
	return ""
}

func findOmicronRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := cwd; dir != "" && dir != "/"; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "research", "agent-rpc", "cmd", "agent-rpc", "main.go")
		if _, err := os.Stat(candidate); err == nil {
			return dir, nil
		}
		next := filepath.Dir(dir)
		if next == dir {
			break
		}
	}
	return "", errors.New("omicron repo root not found")
}

func (s *BridgeService) stopAgent(agentID string) (LaunchedAgent, error) {
	s.mu.Lock()
	launched, ok := s.launchedAgents[agentID]
	s.mu.Unlock()
	if !ok {
		return LaunchedAgent{}, errors.New("launched agent not found")
	}
	if launched.PID <= 0 {
		return LaunchedAgent{}, errors.New("launched agent has no pid")
	}
	if err := syscall.Kill(-launched.PID, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return LaunchedAgent{}, err
	}
	launched.State = "stopped"
	s.mu.Lock()
	s.launchedAgents[agentID] = launched
	managed := s.upsertManagedAgentLocked(ManagedAgent{AgentID: agentID, DesiredState: "stopped"})
	managed.LastObservedProcess = "stopped"
	managed.LastStoppedAt = time.Now().UTC()
	s.state.ManagedAgents[agentID] = managed
	_ = s.saveStateLocked()
	s.mu.Unlock()
	_ = s.appendAudit("agent.stop", map[string]any{"agentId": agentID, "pid": launched.PID})
	return launched, nil
}
