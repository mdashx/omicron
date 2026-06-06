package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

func (s *BridgeService) handleBridgeRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.prepareBridgeRestart(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = s.appendAudit("bridge.restart.requested", map[string]any{"at": time.Now().UTC()})
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", "/")
		w.WriteHeader(http.StatusNoContent)
	} else {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(250 * time.Millisecond)
		if err := s.restartSelf(); err != nil {
			_ = s.appendAudit("bridge.restart.failed", map[string]any{"error": err.Error()})
		}
	}()
}

func (s *BridgeService) prepareBridgeRestart() error {
	agents := s.launchedAgentSnapshot()
	for _, agent := range agents {
		if err := terminateProcessGroup(agent.PID, 2*time.Second); err != nil {
			return fmt.Errorf("stop %s: %w", agent.AgentID, err)
		}
	}
	s.mu.Lock()
	s.queues = map[string][]InboundEvent{}
	s.pendingModelPickers = map[string]pendingModelPicker{}
	s.pickerModels = append([]ModelOption(nil), s.cfg.AvailableModels...)
	s.launchedAgents = map[string]LaunchedAgent{}
	s.state.ProcessedMessageIDs = map[string]int64{}
	s.state.ManagedReactions = map[string]string{}
	for agentID, agent := range s.state.ManagedAgents {
		agent.LastObservedProcess = "stopped"
		agent.LastStoppedAt = time.Now().UTC()
		agent.LastError = ""
		s.state.ManagedAgents[agentID] = agent
	}
	if err := s.saveStateLocked(); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	for _, agent := range agents {
		for _, name := range []string{"pi-sessions", "client-state"} {
			_ = os.RemoveAll(filepath.Join(s.cfg.StorageRoot, "launched-agents", agent.AgentID, name))
		}
	}
	if s.dg != nil {
		_ = s.dg.Close()
	}
	return nil
}

func (s *BridgeService) launchedAgentSnapshot() []LaunchedAgent {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]LaunchedAgent, 0, len(s.launchedAgents))
	for _, agent := range s.launchedAgents {
		result = append(result, agent)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].AgentID < result[j].AgentID })
	return result
}

func terminateProcessGroup(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processGroupExists(pid) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

func processGroupExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(-pid, 0)
	return err == nil || err == syscall.EPERM
}

func (s *BridgeService) restartSelf() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
