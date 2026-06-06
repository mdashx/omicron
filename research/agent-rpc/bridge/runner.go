package bridge

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mdashx/omicron/research/agent-rpc/harness"
)

type Runner struct {
	cfg         Config
	client      *Client
	harness     *harness.Harness
	statePath   string
	state       State
	currentBind Binding
}

func NewRunner(cfg Config, h *harness.Harness) (*Runner, error) {
	cfg = cfg.Resolved()
	if cfg.CredsRef == "" {
		return nil, fmt.Errorf("bridge credsRef is required")
	}
	statePath := filepath.Join(cfg.StateRoot, fmt.Sprintf("%s.bridge-state.json", cfg.AgentID))
	state, err := LoadState(statePath)
	if err != nil {
		return nil, err
	}
	return &Runner{
		cfg:       cfg,
		client:    NewClient(cfg.BridgeURL),
		harness:   h,
		statePath: statePath,
		state:     state,
	}, nil
}

func (r *Runner) Start(ctx context.Context) error {
	binding, err := r.client.Join(ctx, JoinRequest{
		AgentID:            r.cfg.AgentID,
		CredsRef:           r.cfg.CredsRef,
		RequestedGuildID:   r.cfg.GuildID,
		RequestedChannelID: r.cfg.ChannelID,
		Scope:              r.cfg.Scope,
	})
	if err != nil {
		return err
	}
	r.currentBind = binding
	if err := r.harness.Start(); err != nil {
		return err
	}
	_, err = r.harness.GetState(ctx)
	return err
}

func (r *Runner) Run(ctx context.Context) error {
	if err := r.Start(ctx); err != nil {
		return err
	}
	ticker := time.NewTicker(time.Duration(r.cfg.PollIntervalMs) * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := r.pollOnce(ctx); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) pollOnce(ctx context.Context) error {
	events, err := r.client.PollEvents(ctx, r.cfg.AgentID)
	if err != nil {
		return err
	}
	for _, event := range events {
		if _, seen := r.state.ProcessedEventIDs[event.EventID]; seen {
			continue
		}
		if err := r.handleEvent(ctx, event); err != nil {
			_ = r.client.UpdateStatus(ctx, r.cfg.AgentID, StatusRequest{MessageID: event.MessageID, Reaction: r.cfg.FailureReaction})
			_ = r.client.Complete(ctx, r.cfg.AgentID, CompleteRequest{
				MessageID:     event.MessageID,
				Content:       fmt.Sprintf("[agent-rpc] request failed: %v", err),
				FinalReaction: r.cfg.FailureReaction,
			})
		}
		r.state.ProcessedEventIDs[event.EventID] = time.Now().Unix()
		if saveErr := SaveState(r.statePath, r.state); saveErr != nil {
			return saveErr
		}
	}
	return nil
}

func (r *Runner) handleEvent(ctx context.Context, event InboundEvent) error {
	if event.MessageID != "" {
		_ = r.client.UpdateStatus(ctx, r.cfg.AgentID, StatusRequest{MessageID: event.MessageID, Reaction: r.cfg.InProgressReaction})
	}
	if event.Kind == "slash_passthrough" {
		return r.handleSlashPassthrough(ctx, event)
	}
	if event.Kind == "set_model" {
		return r.handleSetModel(ctx, event)
	}
	message := formatBridgeMessage(event)
	result, err := r.harness.Prompt(ctx, message)
	if err != nil {
		return err
	}
	if event.MessageID == "" {
		return nil
	}
	if err := r.client.Complete(ctx, r.cfg.AgentID, CompleteRequest{
		MessageID:     event.MessageID,
		Content:       result.Text,
		FinalReaction: r.cfg.SuccessReaction,
	}); err != nil {
		return err
	}
	return nil
}

func (r *Runner) handleSetModel(ctx context.Context, event InboundEvent) error {
	if err := r.harness.SetModel(ctx, event.Content); err != nil {
		return err
	}
	if event.MessageID == "" {
		return nil
	}
	return r.client.Complete(ctx, r.cfg.AgentID, CompleteRequest{
		MessageID:     event.MessageID,
		Content:       fmt.Sprintf("[discord-bridge]\nModel changed to: %s", event.Content),
		FinalReaction: r.cfg.SuccessReaction,
	})
}

func (r *Runner) handleSlashPassthrough(ctx context.Context, event InboundEvent) error {
	var content string
	switch event.CommandName {
	case "new":
		if err := r.harness.NewSession(ctx); err != nil {
			return err
		}
		content = "[discord-bridge]\nForwarded to Pi: /new\n\nA new Pi session was created for this room agent."
	case "state":
		state, err := r.harness.GetState(ctx)
		if err != nil {
			return err
		}
		content = fmt.Sprintf("[discord-bridge]\nForwarded to Pi: /state\n\nSession: %s\nFile: %s\nStreaming: %v\nPending messages: %d\nThinking: %s", state.SessionID, state.SessionFile, state.IsStreaming, state.PendingMessages, state.ThinkingLevel)
	default:
		content = fmt.Sprintf("[discord-bridge]\nPassthrough not implemented for: %s", event.Content)
	}
	if event.MessageID == "" {
		return nil
	}
	return r.client.Complete(ctx, r.cfg.AgentID, CompleteRequest{
		MessageID:     event.MessageID,
		Content:       content,
		FinalReaction: r.cfg.SuccessReaction,
	})
}

func formatBridgeMessage(event InboundEvent) string {
	var b strings.Builder
	b.WriteString("[discord-bridge]\n")
	if event.AuthorName != "" {
		b.WriteString("Author: ")
		b.WriteString(event.AuthorName)
		b.WriteString("\n")
	}
	if event.ChannelID != "" {
		b.WriteString("Channel: ")
		b.WriteString(event.ChannelID)
		b.WriteString("\n")
	}
	if !event.Timestamp.IsZero() {
		b.WriteString("Timestamp: ")
		b.WriteString(event.Timestamp.UTC().Format(time.RFC3339))
		b.WriteString("\n")
	}
	if event.ReplyToID != "" {
		b.WriteString("ReplyTo: ")
		b.WriteString(event.ReplyToID)
		b.WriteString("\n")
	}
	if len(event.Attachments) > 0 {
		b.WriteString("Attachments:\n")
		for _, att := range event.Attachments {
			b.WriteString("- ")
			b.WriteString(att.Filename)
			if att.LocalPath != "" {
				b.WriteString(" (")
				b.WriteString(att.LocalPath)
				b.WriteString(")")
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("Message: ")
	b.WriteString(event.Content)
	return strings.TrimSpace(b.String())
}
