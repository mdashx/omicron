package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type piOutputMonitor struct {
	cfg        Config
	launchTime time.Time
	cwd        string

	mu     sync.Mutex
	source PiStructuredOutputSource
}

type piOutputCursor struct {
	archivePath   string
	archiveOffset int64
	sessionPath   string
	sessionOffset int64
}

type piLogCandidate struct {
	path    string
	modTime time.Time
}

func newPiOutputMonitor(cfg Config, launchTime time.Time) *piOutputMonitor {
	return &piOutputMonitor{cfg: cfg, launchTime: launchTime.UTC(), cwd: cfg.Cwd}
}

func (m *piOutputMonitor) Enabled() bool {
	if strings.TrimSpace(m.cfg.OutputMode) != "pi-jsonl" {
		return false
	}
	base := filepath.Base(strings.TrimSpace(m.cfg.Command))
	return base == "pi"
}

func (m *piOutputMonitor) Resolve() PiStructuredOutputSource {
	source := PiStructuredOutputSource{
		AgentID:      m.cfg.AgentID,
		Mode:         "pi-jsonl",
		RegisteredAt: time.Now().UTC(),
	}
	if session := m.findLatestSessionFile(); session != "" {
		source.SessionFile = session
	}
	if archive := m.findLatestArchiveFile(); archive != "" {
		source.ArchiveFile = archive
	}
	source.Active = source.SessionFile != "" || source.ArchiveFile != ""

	m.mu.Lock()
	m.source = source
	m.mu.Unlock()
	return source
}

func (m *piOutputMonitor) Snapshot() piOutputCursor {
	source := m.Resolve()
	cursor := piOutputCursor{archivePath: source.ArchiveFile, sessionPath: source.SessionFile}
	if source.SessionFile != "" {
		cursor.sessionOffset = fileSize(source.SessionFile)
	}
	if source.ArchiveFile != "" {
		cursor.archiveOffset = fileSize(source.ArchiveFile)
	}
	return cursor
}

func (m *piOutputMonitor) WaitForReply(cursor piOutputCursor, idle time.Duration) (string, bool) {
	waitBudget := 4 * idle
	if waitBudget < 15*time.Second {
		waitBudget = 15 * time.Second
	}
	deadline := time.Now().Add(waitBudget)
	poll := time.NewTicker(250 * time.Millisecond)
	defer poll.Stop()
	lastActivity := time.Now()
	archiveText := ""
	sessionText := ""
	sawAny := false
	for range poll.C {
		if cursor.archivePath == "" || cursor.sessionPath == "" {
			source := m.Resolve()
			if cursor.archivePath == "" && source.ArchiveFile != "" {
				cursor.archivePath = source.ArchiveFile
				cursor.archiveOffset = 0
			}
			if cursor.sessionPath == "" && source.SessionFile != "" {
				cursor.sessionPath = source.SessionFile
				cursor.sessionOffset = 0
			}
		}
		if cursor.archivePath != "" {
			if text, next, changed := readPiReplyDelta(cursor.archivePath, cursor.archiveOffset); changed {
				cursor.archiveOffset = next
				if text != "" {
					archiveText = text
					lastActivity = time.Now()
					sawAny = true
				}
			}
		}
		if cursor.sessionPath != "" {
			if text, next, changed := readPiReplyDelta(cursor.sessionPath, cursor.sessionOffset); changed {
				cursor.sessionOffset = next
				if text != "" {
					sessionText = text
					lastActivity = time.Now()
					sawAny = true
				}
			}
		}
		if sawAny && time.Since(lastActivity) >= idle {
			if sessionText != "" {
				return sessionText, true
			}
			if archiveText != "" {
				return archiveText, true
			}
		}
		if time.Now().After(deadline) {
			break
		}
	}
	if sessionText != "" {
		return sessionText, true
	}
	if archiveText != "" {
		return archiveText, true
	}
	return "", false
}

func (m *piOutputMonitor) findLatestArchiveFile() string {
	return m.findLatestMatchingFile(m.cfg.PiSessionArchiveRoot, isPiArchiveForCwd)
}

func (m *piOutputMonitor) findLatestSessionFile() string {
	return m.findLatestMatchingFile(m.cfg.PiSessionRoot, isPiSessionForCwd)
}

func (m *piOutputMonitor) findLatestMatchingFile(root string, match func(string, string) bool) string {
	root = strings.TrimSpace(root)
	if root == "" {
		return ""
	}
	var candidates []piLogCandidate
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil || d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil {
			return nil
		}
		if info.ModTime().Before(m.launchTime.Add(-2 * time.Minute)) {
			return nil
		}
		if !match(path, m.cwd) {
			return nil
		}
		candidates = append(candidates, piLogCandidate{path: path, modTime: info.ModTime()})
		return nil
	})
	if len(candidates) == 0 {
		return ""
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].modTime.After(candidates[j].modTime)
	})
	return candidates[0].path
}

func isPiArchiveForCwd(path, cwd string) bool {
	line, err := readFirstLine(path)
	if err != nil || strings.TrimSpace(line) == "" {
		return false
	}
	var evt struct {
		Role      string `json:"role"`
		EventType string `json:"eventType"`
		Content   string `json:"content"`
	}
	if json.Unmarshal([]byte(line), &evt) != nil {
		return false
	}
	if evt.Role != "system" || evt.EventType != "session_start" || evt.Content == "" {
		return false
	}
	var envelope struct {
		Cwd string `json:"cwd"`
	}
	if json.Unmarshal([]byte(evt.Content), &envelope) != nil {
		return false
	}
	return filepath.Clean(envelope.Cwd) == filepath.Clean(cwd)
}

func isPiSessionForCwd(path, cwd string) bool {
	line, err := readFirstLine(path)
	if err != nil || strings.TrimSpace(line) == "" {
		return false
	}
	var header struct {
		Type string `json:"type"`
		Cwd  string `json:"cwd"`
	}
	if json.Unmarshal([]byte(line), &header) != nil {
		return false
	}
	return header.Type == "session" && filepath.Clean(header.Cwd) == filepath.Clean(cwd)
}

func readPiReplyDelta(path string, offset int64) (string, int64, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", offset, false
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", offset, false
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return "", offset, false
	}
	if len(data) == 0 {
		return "", offset, false
	}
	text := extractPiAssistantReply(data)
	return text, offset + int64(len(data)), true
}

func extractPiAssistantReply(data []byte) string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	best := ""
	for scanner.Scan() {
		if text := parsePiAssistantLine(scanner.Bytes()); text != "" {
			best = text
		}
	}
	return strings.TrimSpace(best)
}

func parsePiAssistantLine(line []byte) string {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return ""
	}
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(line, &envelope); err != nil {
		return ""
	}
	if role := rawString(envelope["role"]); role == "assistant" {
		if text := rawString(envelope["content"]); text != "" {
			return strings.TrimSpace(text)
		}
	}
	if rawString(envelope["type"]) != "message" {
		return ""
	}
	var payload struct {
		Message struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal(line, &payload); err != nil {
		return ""
	}
	if payload.Message.Role != "assistant" {
		return ""
	}
	parts := make([]string, 0, len(payload.Message.Content))
	for _, item := range payload.Message.Content {
		if item.Type == "text" && strings.TrimSpace(item.Text) != "" {
			parts = append(parts, strings.TrimSpace(item.Text))
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func rawString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err == nil {
		return value
	}
	return ""
}

func readFirstLine(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
