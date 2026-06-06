package main

import "time"

type JoinRequest struct {
	AgentID            string   `json:"agentId"`
	CredsRef           string   `json:"credsRef"`
	RequestedGuildID   string   `json:"requestedGuildId,omitempty"`
	RequestedChannelID string   `json:"requestedChannelId,omitempty"`
	Scope              []string `json:"scope,omitempty"`
}

type Binding struct {
	AgentID   string    `json:"agentId"`
	GuildID   string    `json:"guildId"`
	ChannelID string    `json:"channelId"`
	JoinedAt  time.Time `json:"joinedAt"`
	Active    bool      `json:"active"`
}

type InboundEvent struct {
	EventID     string             `json:"eventId"`
	MessageID   string             `json:"messageId"`
	AgentID     string             `json:"agentId"`
	GuildID     string             `json:"guildId,omitempty"`
	ChannelID   string             `json:"channelId"`
	AuthorID    string             `json:"authorId"`
	AuthorName  string             `json:"authorName"`
	Content     string             `json:"content"`
	Timestamp   time.Time          `json:"timestamp"`
	Attachments []AttachmentRecord `json:"attachments,omitempty"`
	ReplyToID   string             `json:"replyToId,omitempty"`
}

type AttachmentRecord struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	URL         string `json:"url"`
	LocalPath   string `json:"localPath,omitempty"`
	Size        int    `json:"size"`
	ContentType string `json:"contentType,omitempty"`
}

type EventPollResponse struct {
	Events []InboundEvent `json:"events"`
}

type StatusUpdateRequest struct {
	MessageID string `json:"messageId"`
	Reaction  string `json:"reaction"`
}

type CompleteRequest struct {
	MessageID     string `json:"messageId"`
	Content       string `json:"content"`
	FinalReaction string `json:"finalReaction,omitempty"`
}

type PiStructuredOutputSource struct {
	AgentID      string    `json:"agentId"`
	Mode         string    `json:"mode"`
	SessionFile  string    `json:"sessionFile,omitempty"`
	RegisteredAt time.Time `json:"registeredAt,omitempty"`
	Active       bool      `json:"active"`
}

type HarnessState struct {
	ProcessedEventIDs map[string]bool          `json:"processedEventIds"`
	LastJoinAt        time.Time                `json:"lastJoinAt,omitempty"`
	LastMessageID     string                   `json:"lastMessageId,omitempty"`
	OutputSource      PiStructuredOutputSource `json:"outputSource,omitempty"`
}

type CompletionResult struct {
	Text          string
	FinalReaction string
}
