package harness

import "encoding/json"

type State struct {
	SessionID       string
	SessionFile     string
	SessionName     string
	ThinkingLevel   string
	IsStreaming     bool
	PendingMessages int
	AutoCompaction  bool
	RawModel        json.RawMessage
	Raw             json.RawMessage
}

type PromptResult struct {
	RequestID string
	Text      string
}

type StatusEvent struct {
	Phase   string
	Message string
}

type EventSummary struct {
	Type      string
	ToolName  string
	DeltaType string
}
