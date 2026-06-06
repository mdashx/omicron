package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParsePiAssistantLineFromNativeSession(t *testing.T) {
	line := `{"type":"message","message":{"role":"assistant","content":[{"type":"thinking","thinking":"x"},{"type":"text","text":"hello world"}]}}`
	got := parsePiAssistantLine([]byte(line))
	if got != "hello world" {
		t.Fatalf("unexpected assistant text: %q", got)
	}
}

func TestParsePiAssistantLineFromArchive(t *testing.T) {
	line := `{"role":"assistant","eventType":"message","content":"archive hello"}`
	got := parsePiAssistantLine([]byte(line))
	if got != "archive hello" {
		t.Fatalf("unexpected archive assistant text: %q", got)
	}
}

func TestPiOutputMonitorResolve(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "work")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions")
	archiveRoot := filepath.Join(root, "archive")
	sessionDir := filepath.Join(sessionRoot, "agent")
	archiveDir := filepath.Join(archiveRoot, "2026", "06", "06")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(sessionDir, "s.jsonl")
	archivePath := filepath.Join(archiveDir, "a.jsonl")
	if err := os.WriteFile(sessionPath, []byte(`{"type":"session","cwd":"`+cwd+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	archiveHeader := `{"role":"system","eventType":"session_start","content":"{\"cwd\":\"` + strings.ReplaceAll(cwd, `\`, `\\`) + `\"}"}` + "\n"
	if err := os.WriteFile(archivePath, []byte(archiveHeader), 0o644); err != nil {
		t.Fatal(err)
	}
	launch := time.Now().UTC()
	if err := os.Chtimes(sessionPath, launch, launch); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(archivePath, launch.Add(time.Second), launch.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	monitor := newPiOutputMonitor(Config{AgentID: "main", Command: "pi", OutputMode: "pi-jsonl", Cwd: cwd, PiSessionRoot: sessionRoot, PiSessionArchiveRoot: archiveRoot}, launch.Add(-time.Second))
	source := monitor.Resolve()
	if source.SessionFile != sessionPath {
		t.Fatalf("unexpected session file: %q", source.SessionFile)
	}
	if source.ArchiveFile != archivePath {
		t.Fatalf("unexpected archive file: %q", source.ArchiveFile)
	}
}
