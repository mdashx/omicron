package main

import "testing"

func TestParseAdminCommandAliasAndPassthrough(t *testing.T) {
	cmd, err := parseAdminCommand("/health")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "status" || cmd.ResolvedFrom != "health" {
		t.Fatalf("unexpected alias resolution: %+v", cmd)
	}

	cmd, err = parseAdminCommand("/new")
	if err != nil {
		t.Fatal(err)
	}
	if !cmd.IsPassthrough || cmd.PassthroughCmd != "/new" {
		t.Fatalf("unexpected passthrough parse: %+v", cmd)
	}
}

func TestIsSlashCommand(t *testing.T) {
	if !isSlashCommand("/status") {
		t.Fatal("expected /status to be recognized")
	}
	if isSlashCommand("hello") {
		t.Fatal("did not expect plain text to be recognized")
	}
}
