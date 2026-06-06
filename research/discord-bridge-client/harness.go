package main

import (
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

type Harness struct {
	cfg    Config
	client *BridgeClient
	agent  *PTYAgent
	output *piOutputMonitor
	mu     sync.Mutex
	state  HarnessState
}

func NewHarness(cfg Config) (*Harness, error) {
	if err := os.MkdirAll(cfg.StateRoot, 0o755); err != nil {
		return nil, err
	}
	state, err := loadState(cfg.StatePath)
	if err != nil {
		return nil, err
	}
	return &Harness{cfg: cfg, client: NewBridgeClient(cfg.BridgeURL), state: state}, nil
}

func (h *Harness) Run(ctx context.Context) error {
	binding, err := h.client.Join(JoinRequest{
		AgentID:            h.cfg.AgentID,
		CredsRef:           h.cfg.CredsRef,
		RequestedGuildID:   h.cfg.GuildID,
		RequestedChannelID: h.cfg.ChannelID,
		Scope:              []string{"read.logs", "read.downloads", "channel.listen", "channel.reply"},
	})
	if err != nil {
		return err
	}
	if h.cfg.GuildID == "" {
		h.cfg.GuildID = binding.GuildID
	}
	if h.cfg.ChannelID == "" {
		h.cfg.ChannelID = binding.ChannelID
	}
	h.mu.Lock()
	h.state.LastJoinAt = time.Now().UTC()
	if err := saveState(h.cfg.StatePath, h.state); err != nil {
		h.mu.Unlock()
		return err
	}
	h.mu.Unlock()
	agent, err := StartPTYAgent(h.cfg)
	if err != nil {
		return err
	}
	h.agent = agent
	h.output = newPiOutputMonitor(h.cfg, agent.StartedAt())
	if h.output.Enabled() {
		source := h.output.Resolve()
		h.mu.Lock()
		h.state.OutputSource = source
		if err := saveState(h.cfg.StatePath, h.state); err != nil {
			h.mu.Unlock()
			return err
		}
		h.mu.Unlock()
		log.Printf("pi output source agent=%s archive=%s session=%s", h.cfg.AgentID, source.ArchiveFile, source.SessionFile)
	}
	defer h.agent.Close()
	log.Printf("bridge client joined bridge=%s agent=%s channel=%s cmd=%s", h.cfg.BridgeURL, h.cfg.AgentID, h.cfg.ChannelID, h.cfg.Command)
	ticker := time.NewTicker(h.cfg.PollInterval)
	defer ticker.Stop()
	for {
		if err := h.processOnce(); err != nil {
			log.Printf("process loop error: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (h *Harness) processOnce() error {
	events, err := h.client.PollEvents(h.cfg.AgentID)
	if err != nil {
		return err
	}
	for _, evt := range events {
		if h.isProcessed(evt.EventID) {
			continue
		}
		if err := h.handleEvent(evt); err != nil {
			return err
		}
	}
	return nil
}

func (h *Harness) handleEvent(evt InboundEvent) error {
	if err := h.client.SetStatus(h.cfg.AgentID, StatusUpdateRequest{MessageID: evt.MessageID, Reaction: h.cfg.QueueReaction}); err != nil {
		return err
	}
	if err := h.client.SetStatus(h.cfg.AgentID, StatusUpdateRequest{MessageID: evt.MessageID, Reaction: h.cfg.ThinkingReaction}); err != nil {
		return err
	}
	var cursor piOutputCursor
	useStructuredOutput := h.output != nil && h.output.Enabled()
	if useStructuredOutput {
		cursor = h.output.Snapshot()
	}
	if err := h.agent.Inject(RenderBridgePrompt(evt)); err != nil {
		return err
	}
	result, err := h.waitForCompletion(cursor, useStructuredOutput)
	if err != nil {
		return err
	}
	if result.Text == "" {
		result.Text = "[discord-bridge-client] agent completed with no visible output"
	}
	if err := h.client.Complete(h.cfg.AgentID, CompleteRequest{MessageID: evt.MessageID, Content: result.Text, FinalReaction: result.FinalReaction}); err != nil {
		return err
	}
	return h.markProcessed(evt.EventID, evt.MessageID)
}

func (h *Harness) waitForCompletion(cursor piOutputCursor, useStructuredOutput bool) (CompletionResult, error) {
	if useStructuredOutput {
		h.syncOutputSource()
		if text, ok := h.output.WaitForReply(cursor, h.cfg.IdleCompleteWait); ok && strings.TrimSpace(text) != "" {
			h.syncOutputSource()
			return CompletionResult{Text: strings.TrimSpace(text), FinalReaction: h.cfg.FinalReaction}, nil
		}
	}
	return h.agent.WaitForTurnResult(h.cfg.IdleCompleteWait)
}

func (h *Harness) syncOutputSource() {
	if h.output == nil || !h.output.Enabled() {
		return
	}
	source := h.output.Resolve()
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state.OutputSource == source {
		return
	}
	h.state.OutputSource = source
	_ = saveState(h.cfg.StatePath, h.state)
}

func (h *Harness) isProcessed(eventID string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.state.ProcessedEventIDs[eventID]
}

func (h *Harness) markProcessed(eventID, messageID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.state.ProcessedEventIDs == nil {
		h.state.ProcessedEventIDs = map[string]bool{}
	}
	h.state.ProcessedEventIDs[eventID] = true
	h.state.LastMessageID = messageID
	return saveState(h.cfg.StatePath, h.state)
}
