package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func loadState(path string) (HarnessState, error) {
	state := HarnessState{ProcessedEventIDs: map[string]bool{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read harness state: %w", err)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("parse harness state: %w", err)
	}
	if state.ProcessedEventIDs == nil {
		state.ProcessedEventIDs = map[string]bool{}
	}
	return state, nil
}

func saveState(path string, state HarnessState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
