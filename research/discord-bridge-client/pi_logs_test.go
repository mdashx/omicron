package main

import (
	"os"
	"path/filepath"
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

func TestPiOutputMonitorResolve(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "work")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionRoot := filepath.Join(root, "sessions")
	sessionDir := filepath.Join(sessionRoot, "agent")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sessionPath := filepath.Join(sessionDir, "s.jsonl")
	if err := os.WriteFile(sessionPath, []byte(`{"type":"session","cwd":"`+cwd+`"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	launch := time.Now().UTC()
	if err := os.Chtimes(sessionPath, launch, launch); err != nil {
		t.Fatal(err)
	}
	monitor := newPiOutputMonitor(Config{AgentID: "main", Command: "pi", Cwd: cwd, PiSessionRoot: sessionRoot}, launch.Add(-time.Second))
	source := monitor.Resolve()
	if source.SessionFile != sessionPath {
		t.Fatalf("unexpected session file: %q", source.SessionFile)
	}
	if source.Active != true {
		t.Fatalf("expected active source")
	}
}
