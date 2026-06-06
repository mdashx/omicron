package harness

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

type rpcEnvelope struct {
	ID      string          `json:"id,omitempty"`
	Type    string          `json:"type"`
	Command string          `json:"command,omitempty"`
	Success bool            `json:"success,omitempty"`
	Error   string          `json:"error,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

type harnessEvent struct {
	Type                  string          `json:"type"`
	Message               json.RawMessage `json:"message,omitempty"`
	Messages              json.RawMessage `json:"messages,omitempty"`
	AssistantMessageEvent json.RawMessage `json:"assistantMessageEvent,omitempty"`
	ToolName              string          `json:"toolName,omitempty"`
}

type statePayload struct {
	Model               json.RawMessage `json:"model,omitempty"`
	ThinkingLevel       string          `json:"thinkingLevel,omitempty"`
	IsStreaming         bool            `json:"isStreaming,omitempty"`
	AutoCompaction      bool            `json:"autoCompactionEnabled,omitempty"`
	SessionFile         string          `json:"sessionFile,omitempty"`
	SessionID           string          `json:"sessionId,omitempty"`
	SessionName         string          `json:"sessionName,omitempty"`
	PendingMessageCount int             `json:"pendingMessageCount,omitempty"`
}

type lastAssistantPayload struct {
	Text *string `json:"text"`
}

type promptCommand struct {
	ID                string `json:"id,omitempty"`
	Type              string `json:"type"`
	Message           string `json:"message,omitempty"`
	StreamingBehavior string `json:"streamingBehavior,omitempty"`
}

type requestTracker struct {
	requestID      string
	agentDone      chan struct{}
	thinkingSeen   bool
	toolEventsSeen int
}

type Harness struct {
	cfg    Config
	logger *log.Logger

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	mu          sync.Mutex
	started     bool
	closed      bool
	state       State
	pending     map[string]chan rpcEnvelope
	activeReq   *requestTracker
	processed   map[string]struct{}
	requestSeq  uint64
	readerDone  chan error
	processDone chan error
	requestMu   sync.Mutex
}

func NewHarness(cfg Config) *Harness {
	cfg = cfg.Resolved()
	return &Harness{
		cfg:         cfg,
		logger:      log.New(os.Stderr, "[agent-rpc] ", log.LstdFlags),
		pending:     map[string]chan rpcEnvelope{},
		processed:   map[string]struct{}{},
		readerDone:  make(chan error, 1),
		processDone: make(chan error, 1),
	}
}

func (h *Harness) Start() error {
	h.mu.Lock()
	if h.started {
		h.mu.Unlock()
		return nil
	}
	cmd := exec.Command(h.cfg.Command, h.cfg.Args...)
	cmd.Dir = h.cfg.Cwd
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		h.mu.Unlock()
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		h.mu.Unlock()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		h.mu.Unlock()
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		h.mu.Unlock()
		return fmt.Errorf("start upstream: %w", err)
	}
	h.cmd, h.stdin, h.stdout, h.stderr = cmd, stdin, stdout, stderr
	h.started = true
	h.mu.Unlock()
	go h.readStdout()
	go h.readStderr()
	go func() { h.processDone <- cmd.Wait() }()
	if _, err := h.GetState(context.Background()); err != nil {
		return err
	}
	return nil
}

func (h *Harness) Close() error {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	stdin := h.stdin
	cmd := h.cmd
	h.mu.Unlock()
	if stdin != nil {
		_ = stdin.Close()
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	return nil
}

func (h *Harness) GetState(ctx context.Context) (State, error) {
	resp, err := h.sendCommand(ctx, map[string]any{"type": "get_state"})
	if err != nil {
		return State{}, err
	}
	if !resp.Success {
		return State{}, errors.New(resp.Error)
	}
	var payload statePayload
	if err := json.Unmarshal(resp.Data, &payload); err != nil {
		return State{}, fmt.Errorf("parse get_state: %w", err)
	}
	state := State{
		SessionID:       payload.SessionID,
		SessionFile:     payload.SessionFile,
		SessionName:     payload.SessionName,
		ThinkingLevel:   payload.ThinkingLevel,
		IsStreaming:     payload.IsStreaming,
		PendingMessages: payload.PendingMessageCount,
		AutoCompaction:  payload.AutoCompaction,
		RawModel:        payload.Model,
		Raw:             resp.Data,
	}
	h.mu.Lock()
	h.state = state
	h.mu.Unlock()
	return state, nil
}

func (h *Harness) NewSession(ctx context.Context) error {
	resp, err := h.sendCommand(ctx, map[string]any{"type": "new_session"})
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Error)
	}
	_, err = h.GetState(ctx)
	return err
}

func (h *Harness) SetModel(ctx context.Context, model string) error {
	parts := strings.SplitN(model, "/", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return fmt.Errorf("model must be provider/modelId: %s", model)
	}
	resp, err := h.sendCommand(ctx, map[string]any{"type": "set_model", "provider": parts[0], "modelId": parts[1]})
	if err != nil {
		return err
	}
	if !resp.Success {
		return errors.New(resp.Error)
	}
	_, err = h.GetState(ctx)
	return err
}

func (h *Harness) Prompt(ctx context.Context, message string) (PromptResult, error) {
	h.requestMu.Lock()
	defer h.requestMu.Unlock()
	if err := h.Start(); err != nil {
		return PromptResult{}, err
	}
	state, err := h.GetState(ctx)
	if err != nil {
		return PromptResult{}, err
	}
	requestID := h.nextID("req")
	tracker := &requestTracker{requestID: requestID, agentDone: make(chan struct{})}
	h.mu.Lock()
	h.activeReq = tracker
	h.mu.Unlock()
	cmd := promptCommand{ID: requestID, Type: "prompt", Message: message}
	if state.IsStreaming {
		cmd.StreamingBehavior = "steer"
	}
	h.status("started", "accepted request")
	resp, err := h.sendCommand(ctx, cmd)
	if err != nil {
		h.clearActiveRequest(requestID)
		return PromptResult{}, err
	}
	if !resp.Success {
		h.clearActiveRequest(requestID)
		return PromptResult{}, errors.New(resp.Error)
	}
	h.status("working", "waiting for upstream completion")
	select {
	case <-tracker.agentDone:
	case err := <-h.readerDone:
		h.clearActiveRequest(requestID)
		return PromptResult{}, fmt.Errorf("reader failed: %w", err)
	case err := <-h.processDone:
		h.clearActiveRequest(requestID)
		return PromptResult{}, fmt.Errorf("upstream exited: %w", err)
	case <-ctx.Done():
		return PromptResult{}, ctx.Err()
	}
	text, err := h.getLastAssistantText(ctx)
	if err != nil {
		h.clearActiveRequest(requestID)
		return PromptResult{}, err
	}
	if text == "" {
		text = "[agent-rpc] upstream completed without extractable final assistant text"
	}
	h.mu.Lock()
	h.processed[requestID] = struct{}{}
	h.mu.Unlock()
	h.clearActiveRequest(requestID)
	h.status("completed", "received final assistant reply")
	return PromptResult{RequestID: requestID, Text: text}, nil
}

func (h *Harness) sendCommand(ctx context.Context, payload any) (rpcEnvelope, error) {
	if err := h.Start(); err != nil {
		return rpcEnvelope{}, err
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return rpcEnvelope{}, err
	}
	var decoded map[string]any
	if err := json.Unmarshal(blob, &decoded); err != nil {
		return rpcEnvelope{}, err
	}
	id, _ := decoded["id"].(string)
	if id == "" {
		id = h.nextID("cmd")
		decoded["id"] = id
		blob, err = json.Marshal(decoded)
		if err != nil {
			return rpcEnvelope{}, err
		}
	}
	respCh := make(chan rpcEnvelope, 1)
	h.mu.Lock()
	h.pending[id] = respCh
	stdin := h.stdin
	h.mu.Unlock()
	if h.cfg.Debug {
		h.logger.Printf("-> %s", string(blob))
	}
	if _, err := stdin.Write(append(blob, '\n')); err != nil {
		return rpcEnvelope{}, fmt.Errorf("write command: %w", err)
	}
	select {
	case resp := <-respCh:
		return resp, nil
	case err := <-h.readerDone:
		return rpcEnvelope{}, fmt.Errorf("reader failed: %w", err)
	case err := <-h.processDone:
		return rpcEnvelope{}, fmt.Errorf("upstream exited: %w", err)
	case <-ctx.Done():
		return rpcEnvelope{}, ctx.Err()
	}
}

func (h *Harness) getLastAssistantText(ctx context.Context) (string, error) {
	resp, err := h.sendCommand(ctx, map[string]any{"type": "get_last_assistant_text"})
	if err != nil {
		return "", err
	}
	if !resp.Success {
		return "", errors.New(resp.Error)
	}
	var payload lastAssistantPayload
	if err := json.Unmarshal(resp.Data, &payload); err != nil {
		return "", fmt.Errorf("parse get_last_assistant_text: %w", err)
	}
	if payload.Text == nil {
		return "", nil
	}
	return *payload.Text, nil
}

func (h *Harness) readStdout() {
	reader := bufio.NewReader(h.stdout)
	for {
		line, err := readJSONLLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				h.readerDone <- err
				return
			}
			h.readerDone <- err
			return
		}
		if len(line) == 0 {
			continue
		}
		if h.cfg.Debug {
			h.logger.Printf("<- %s", string(line))
		}
		var envelope rpcEnvelope
		if err := json.Unmarshal(line, &envelope); err == nil && envelope.Type == "response" {
			h.mu.Lock()
			respCh := h.pending[envelope.ID]
			delete(h.pending, envelope.ID)
			if envelope.Command == "get_state" && len(envelope.Data) > 0 {
				var payload statePayload
				if json.Unmarshal(envelope.Data, &payload) == nil {
					h.state.SessionID = payload.SessionID
					h.state.SessionFile = payload.SessionFile
					h.state.SessionName = payload.SessionName
					h.state.ThinkingLevel = payload.ThinkingLevel
					h.state.IsStreaming = payload.IsStreaming
					h.state.PendingMessages = payload.PendingMessageCount
				}
			}
			h.mu.Unlock()
			if respCh != nil {
				respCh <- envelope
			}
			continue
		}
		var event harnessEvent
		if err := json.Unmarshal(line, &event); err != nil {
			h.status("error", fmt.Sprintf("parse event: %v", err))
			continue
		}
		h.handleEvent(event)
	}
}

func (h *Harness) handleEvent(event harnessEvent) {
	h.mu.Lock()
	tracker := h.activeReq
	if event.Type == "agent_start" {
		h.state.IsStreaming = true
	}
	if event.Type == "agent_end" {
		h.state.IsStreaming = false
	}
	h.mu.Unlock()
	switch event.Type {
	case "agent_start":
		h.status("working", "agent started")
	case "agent_end":
		if tracker != nil {
			close(tracker.agentDone)
		}
	case "message_update":
		h.captureMessageUpdate(tracker, event)
	case "tool_execution_start":
		if tracker != nil {
			tracker.toolEventsSeen++
		}
	case "queue_update":
		// intentionally retained as internal state only in v1
	case "extension_error":
		h.status("error", "extension error reported by upstream")
	}
}

func (h *Harness) captureMessageUpdate(tracker *requestTracker, event harnessEvent) {
	if tracker == nil || len(event.AssistantMessageEvent) == 0 {
		return
	}
	var delta struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(event.AssistantMessageEvent, &delta) != nil {
		return
	}
	if delta.Type == "thinking_start" || delta.Type == "thinking_delta" || delta.Type == "thinking_end" {
		tracker.thinkingSeen = true
	}
}

func (h *Harness) readStderr() {
	reader := bufio.NewReader(h.stderr)
	for {
		line, err := reader.ReadString('\n')
		if line != "" && h.cfg.Debug {
			h.logger.Printf("[stderr] %s", line)
		}
		if err != nil {
			return
		}
	}
}

func (h *Harness) clearActiveRequest(requestID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.activeReq != nil && h.activeReq.requestID == requestID {
		h.activeReq = nil
	}
}

func (h *Harness) nextID(prefix string) string {
	n := atomic.AddUint64(&h.requestSeq, 1)
	return fmt.Sprintf("%s-%06d", prefix, n)
}

func (h *Harness) status(phase, message string) {
	if h.cfg.Debug {
		h.logger.Printf("[%s] %s", phase, message)
	}
}
