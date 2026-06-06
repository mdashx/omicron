package bridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type State struct {
	ProcessedEventIDs map[string]int64 `json:"processedEventIds"`
}

func LoadState(path string) (State, error) {
	state := State{ProcessedEventIDs: map[string]int64{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, fmt.Errorf("read bridge state: %w", err)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("parse bridge state: %w", err)
	}
	if state.ProcessedEventIDs == nil {
		state.ProcessedEventIDs = map[string]int64{}
	}
	return state, nil
}

func SaveState(path string, state State) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}
