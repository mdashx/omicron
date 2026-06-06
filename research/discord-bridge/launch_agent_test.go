package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveLaunchSpecDefaultsToAgentRPCGoRunWhenRepoRootFound(t *testing.T) {
	dir := t.TempDir()
	repoRoot := filepath.Join(dir, "omicron")
	if err := os.MkdirAll(filepath.Join(repoRoot, "research", "agent-rpc", "cmd", "agent-rpc"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "research", "agent-rpc", "cmd", "agent-rpc", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := resolveLaunchSpec(launchAgentRequest{}, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command != "go" {
		t.Fatalf("expected go command, got %q", spec.Command)
	}
	if len(spec.Args) != 3 || spec.Args[0] != "run" || spec.Args[2] != "--bridge" {
		t.Fatalf("unexpected args: %#v", spec.Args)
	}
	if spec.WorkingDir != repoRoot {
		t.Fatalf("unexpected working dir: %q", spec.WorkingDir)
	}
}

func TestResolveLaunchSpecUsesExplicitCommandAndArgs(t *testing.T) {
	spec, err := resolveLaunchSpec(launchAgentRequest{Command: "agent-rpc", Args: []string{"--bridge", "--debug"}}, "")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Command != "agent-rpc" {
		t.Fatalf("unexpected command: %q", spec.Command)
	}
	if len(spec.Args) != 2 || spec.Args[1] != "--debug" {
		t.Fatalf("unexpected args: %#v", spec.Args)
	}
}
