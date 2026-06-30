package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
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

// attachmentClient bounds every stage of an attachment download so a stalled
// Discord CDN connection can never hang the message handler indefinitely. The
// overall Timeout is generous enough for large (boosted-guild) uploads while
// still guaranteeing the request eventually returns.
var attachmentClient = &http.Client{
	Timeout: 10 * time.Minute,
	Transport: &http.Transport{
		DialContext:           (&net.Dialer{Timeout: 15 * time.Second}).DialContext,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		IdleConnTimeout:       90 * time.Second,
	},
}

const attachmentDownloadAttempts = 3

func (s *BridgeService) downloadAttachment(channelID, messageID string, attachment *discordgo.MessageAttachment) (string, error) {
	path := filepath.Join(s.cfg.DownloadsRoot, safeID(channelID), fmt.Sprintf("%s_%s", safeID(messageID), sanitizeFilename(attachment.Filename)))
	// Only a fully-written file ever lands at `path` (see the atomic rename
	// below), so an existing file here is safe to treat as already downloaded.
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}

	var lastErr error
	for attempt := 1; attempt <= attachmentDownloadAttempts; attempt++ {
		retryable, err := fetchToFile(attachment.URL, path)
		if err == nil {
			return path, nil
		}
		lastErr = err
		if !retryable || attempt == attachmentDownloadAttempts {
			break
		}
		// Linear backoff; the signed CDN URL stays valid well beyond this window.
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	return "", lastErr
}

// fetchToFile downloads url into a temp file alongside dest and atomically
// renames it into place on success, so dest never holds a partial download.
// The boolean reports whether the failure is worth retrying.
func fetchToFile(url, dest string) (retryable bool, err error) {
	resp, err := attachmentClient.Get(url)
	if err != nil {
		return true, err // network/timeout errors are transient
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Retry on rate limiting and server errors; treat other 4xx as permanent.
		retry := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		return retry, fmt.Errorf("attachment download status %d", resp.StatusCode)
	}

	tmp, err := os.CreateTemp(filepath.Dir(dest), ".dl-*.part")
	if err != nil {
		return false, err
	}
	tmpName := tmp.Name()
	// Clean up the temp file unless the rename below succeeds.
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := io.Copy(tmp, resp.Body); err != nil {
		return true, err // interrupted body read is transient
	}
	if err := tmp.Close(); err != nil {
		return true, err
	}
	if err := os.Rename(tmpName, dest); err != nil {
		return false, err
	}
	cleanup = false
	return false, nil
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
