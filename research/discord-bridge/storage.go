package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
)

type auditRecord struct {
	Type      string    `json:"type"`
	Timestamp time.Time `json:"timestamp"`
	Payload   any       `json:"payload"`
}

type chatLogRecord struct {
	MessageID    string             `json:"messageId"`
	GuildID      string             `json:"guildId,omitempty"`
	ChannelID    string             `json:"channelId"`
	AuthorID     string             `json:"authorId,omitempty"`
	AuthorName   string             `json:"authorName,omitempty"`
	Content      string             `json:"content"`
	Timestamp    time.Time          `json:"timestamp"`
	Attachments  []AttachmentRecord `json:"attachments,omitempty"`
	Direction    string             `json:"direction"`
	BoundChannel bool               `json:"boundChannel"`
}

func (s *BridgeService) appendAudit(kind string, payload any) error {
	return appendJSONL(s.cfg.AuditPath, auditRecord{Type: kind, Timestamp: time.Now().UTC(), Payload: payload})
}

func (s *BridgeService) appendChatLog(msg *discordgo.Message, attachments []AttachmentRecord, bound bool) error {
	path := filepath.Join(s.cfg.LogsRoot, "chats", safeID(msg.GuildID), fmt.Sprintf("%s.jsonl", safeID(msg.ChannelID)))
	return appendJSONL(path, chatLogRecord{
		MessageID:    msg.ID,
		GuildID:      msg.GuildID,
		ChannelID:    msg.ChannelID,
		AuthorID:     valueOr(msg.Author != nil, func() string { return msg.Author.ID }),
		AuthorName:   valueOr(msg.Author != nil, func() string { return msg.Author.Username }),
		Content:      msg.Content,
		Timestamp:    msg.Timestamp,
		Attachments:  attachments,
		Direction:    "inbound",
		BoundChannel: bound,
	})
}

func appendJSONL(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(value)
}

func (s *BridgeService) persistAttachments(msg *discordgo.Message) []AttachmentRecord {
	if len(msg.Attachments) == 0 {
		return nil
	}
	result := make([]AttachmentRecord, 0, len(msg.Attachments))
	for _, attachment := range msg.Attachments {
		rec := AttachmentRecord{
			ID:          attachment.ID,
			Filename:    attachment.Filename,
			URL:         attachment.URL,
			Size:        attachment.Size,
			ContentType: attachment.ContentType,
		}
		localPath, err := s.downloadAttachment(msg.ChannelID, msg.ID, attachment)
		if err == nil {
			rec.LocalPath = localPath
		} else {
			_ = s.appendAudit("attachment.download_error", map[string]any{"messageId": msg.ID, "attachmentId": attachment.ID, "error": err.Error()})
		}
		result = append(result, rec)
	}
	return result
}

func (s *BridgeService) downloadAttachment(channelID, messageID string, attachment *discordgo.MessageAttachment) (string, error) {
	path := filepath.Join(s.cfg.DownloadsRoot, safeID(channelID), fmt.Sprintf("%s_%s", safeID(messageID), sanitizeFilename(attachment.Filename)))
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	resp, err := http.Get(attachment.URL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("attachment download status %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return "", err
	}
	return path, nil
}

func (s *BridgeService) setManagedReaction(channelID, messageID, emoji string) error {
	if emoji == "" {
		return nil
	}
	s.mu.Lock()
	previous := s.state.ManagedReactions[messageID]
	s.mu.Unlock()
	if previous == emoji {
		return nil
	}
	if s.cfg.DryRun {
		s.mu.Lock()
		s.state.ManagedReactions[messageID] = emoji
		err := s.saveStateLocked()
		s.mu.Unlock()
		if err != nil {
			return err
		}
		return s.appendAudit("discord.reaction.dry_run", map[string]any{"channelId": channelID, "messageId": messageID, "emoji": emoji, "previous": previous})
	}
	if previous != "" {
		_ = s.dg.MessageReactionRemove(channelID, messageID, previous, "@me")
	}
	if err := s.dg.MessageReactionAdd(channelID, messageID, emoji); err != nil {
		return err
	}
	s.mu.Lock()
	s.state.ManagedReactions[messageID] = emoji
	err := s.saveStateLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	return s.appendAudit("discord.reaction", map[string]any{"channelId": channelID, "messageId": messageID, "emoji": emoji, "previous": previous})
}

func safeID(id string) string {
	if strings.TrimSpace(id) == "" {
		return "unknown"
	}
	return id
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", " ", "_")
	clean := replacer.Replace(strings.TrimSpace(name))
	if clean == "" {
		return "attachment.bin"
	}
	return clean
}

func valueOr[T any](ok bool, fn func() T) T {
	var zero T
	if !ok {
		return zero
	}
	return fn()
}
