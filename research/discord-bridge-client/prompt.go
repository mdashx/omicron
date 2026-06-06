package main

import (
	"fmt"
	"strings"
)

func RenderBridgePrompt(evt InboundEvent) string {
	var b strings.Builder
	b.WriteString("[discord-bridge]\n")
	b.WriteString(fmt.Sprintf("EventID: %s\n", evt.EventID))
	b.WriteString(fmt.Sprintf("Author: %s (%s)\n", evt.AuthorName, evt.AuthorID))
	b.WriteString(fmt.Sprintf("Channel: %s\n", evt.ChannelID))
	if evt.GuildID != "" {
		b.WriteString(fmt.Sprintf("Guild: %s\n", evt.GuildID))
	}
	b.WriteString(fmt.Sprintf("Timestamp: %s\n", evt.Timestamp.Format("2006-01-02T15:04:05Z07:00")))
	if evt.ReplyToID != "" {
		b.WriteString(fmt.Sprintf("ReplyTo: %s\n", evt.ReplyToID))
	}
	if len(evt.Attachments) > 0 {
		b.WriteString("Attachments:\n")
		for _, att := range evt.Attachments {
			b.WriteString(fmt.Sprintf("- %s", att.Filename))
			if att.LocalPath != "" {
				b.WriteString(fmt.Sprintf(" (local: %s)", att.LocalPath))
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("\nUserMessage:\n")
	b.WriteString(evt.Content)
	b.WriteString("\n")
	return b.String()
}
