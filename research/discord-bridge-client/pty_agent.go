package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/creack/pty"
)

type PTYAgent struct {
	cfg           Config
	cmd           *exec.Cmd
	ptyFile       *os.File
	startedAt     time.Time
	inputLogFile  *os.File
	outputLogFile *os.File
	mu            sync.Mutex
	outputBuf     bytes.Buffer
	lastOutput    time.Time
	closed        bool
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
	inputLogFile, err := openAppendFile(cfg.PTYInputLogPath)
	if err != nil {
		_ = f.Close()
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("open pty input log: %w", err)
	}
	outputLogFile, err := openAppendFile(cfg.PTYOutputLogPath)
	if err != nil {
		_ = inputLogFile.Close()
		_ = f.Close()
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("open pty output log: %w", err)
	}
	a := &PTYAgent{cfg: cfg, cmd: cmd, ptyFile: f, startedAt: startedAt, inputLogFile: inputLogFile, outputLogFile: outputLogFile, lastOutput: startedAt}
	go a.captureOutput()
	return a, nil
}

func (a *PTYAgent) captureOutput() {
	reader := bufio.NewReader(a.ptyFile)
	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunk := buf[:n]
			a.mu.Lock()
			a.outputBuf.Write(chunk)
			a.lastOutput = time.Now()
			a.mu.Unlock()
			a.appendOutputTranscript(chunk)
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
	a.appendInputTranscript(input)
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
	if a.inputLogFile != nil {
		_ = a.inputLogFile.Close()
	}
	if a.outputLogFile != nil {
		_ = a.outputLogFile.Close()
	}
	if a.cmd != nil && a.cmd.Process != nil {
		_ = a.cmd.Process.Kill()
		_, _ = a.cmd.Process.Wait()
	}
	return nil
}

func cleanPTYOutput(raw string) string {
	raw = cleanPTYTranscriptChunk(raw)
	lines := strings.Split(raw, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return strings.Join(filtered, "\n")
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b\][^\a]*(\a|\x1b\\)`)

func cleanPTYTranscriptChunk(raw string) string {
	raw = strings.ReplaceAll(raw, "\r", "")
	raw = ansiPattern.ReplaceAllString(raw, "")
	return raw
}

func openAppendFile(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
}

func (a *PTYAgent) appendInputTranscript(input string) {
	if a.inputLogFile == nil {
		return
	}
	_, _ = io.WriteString(a.inputLogFile, "\n--- "+time.Now().UTC().Format(time.RFC3339)+" INPUT ---\n"+input+"\n")
}

func (a *PTYAgent) appendOutputTranscript(chunk []byte) {
	if a.outputLogFile == nil {
		return
	}
	cleaned := cleanPTYTranscriptChunk(string(chunk))
	if strings.TrimSpace(cleaned) == "" {
		return
	}
	_, _ = io.WriteString(a.outputLogFile, cleaned)
}
