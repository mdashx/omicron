package bridge

import (
	"strings"
	"testing"
	"time"
)

func TestFormatBridgeMessageIncludesUsefulFields(t *testing.T) {
	event := InboundEvent{
		EventID:    "evt_1",
		MessageID:  "msg_1",
		ChannelID:  "chan_1",
		AuthorName: "easter",
		Content:    "hello world",
		Timestamp:  time.Date(2026, 6, 6, 5, 0, 0, 0, time.UTC),
		ReplyToID:  "msg_0",
		Attachments: []AttachmentRecord{{
			Filename:  "note.txt",
			LocalPath: "/tmp/note.txt",
		}},
	}
	text := formatBridgeMessage(event)
	for _, needle := range []string{"[discord-bridge]", "Author: easter", "Channel: chan_1", "ReplyTo: msg_0", "note.txt", "hello world"} {
		if !strings.Contains(text, needle) {
			t.Fatalf("expected %q in formatted message: %s", needle, text)
		}
	}
}
