package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

type PTYAgent struct {
	cfg        Config
	cmd        *exec.Cmd
	ptyFile    *os.File
	startedAt  time.Time
	mu         sync.Mutex
	outputBuf  bytes.Buffer
	lastOutput time.Time
	closed     bool
}

func StartPTYAgent(cfg Config) (*PTYAgent, error) {
	cmd := exec.Command(cfg.Command, cfg.CommandArgs...)
	cmd.Dir = cfg.Cwd
	cmd.Env = os.Environ()
	f, err := pty.StartWithSize(cmd, &pty.Winsize{Cols: cfg.Cols, Rows: cfg.Rows})
	if err != nil {
		return nil, fmt.Errorf("start pty: %w", err)
	}
	startedAt := time.Now().UTC()
	a := &PTYAgent{cfg: cfg, cmd: cmd, ptyFile: f, startedAt: startedAt, lastOutput: startedAt}
	go a.captureOutput()
	return a, nil
}

func (a *PTYAgent) captureOutput() {
	reader := bufio.NewReader(a.ptyFile)
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			a.mu.Lock()
			a.outputBuf.Write(buf[:n])
			a.lastOutput = time.Now()
			a.mu.Unlock()
		}
		if err != nil {
			if err != io.EOF {
				// ignore; process completion handled elsewhere
			}
			return
		}
	}
}

func (a *PTYAgent) Inject(input string) error {
	a.mu.Lock()
	a.outputBuf.Reset()
	a.lastOutput = time.Now()
	a.mu.Unlock()
	_, err := io.WriteString(a.ptyFile, input+"\n")
	return err
}

func (a *PTYAgent) WaitForTurnResult(idle time.Duration) (CompletionResult, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for range ticker.C {
		a.mu.Lock()
		idleFor := time.Since(a.lastOutput)
		output := a.outputBuf.String()
		closed := a.closed
		a.mu.Unlock()
		if closed || idleFor >= idle {
			text := strings.TrimSpace(cleanPTYOutput(output))
			return CompletionResult{Text: text, FinalReaction: a.cfg.FinalReaction}, nil
		}
	}
	return CompletionResult{}, nil
}

func (a *PTYAgent) StartedAt() time.Time {
	return a.startedAt
}

func (a *PTYAgent) Close() error {
	a.mu.Lock()
	if a.closed {
		a.mu.Unlock()
		return nil
	}
	a.closed = true
	a.mu.Unlock()
	if a.ptyFile != nil {
		_ = a.ptyFile.Close()
	}
	if a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
		_, _ = a.cmd.Process.Wait()
	}
	return nil
}

func cleanPTYOutput(raw string) string {
	raw = strings.ReplaceAll(raw, "\r", "")
	lines := strings.Split(raw, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}
