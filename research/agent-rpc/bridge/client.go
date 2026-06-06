package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

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

type AttachmentRecord struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	URL         string `json:"url"`
	LocalPath   string `json:"localPath,omitempty"`
	Size        int    `json:"size"`
	ContentType string `json:"contentType,omitempty"`
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

type PollEventsResponse struct {
	Events []InboundEvent `json:"events"`
}

type StatusRequest struct {
	MessageID string `json:"messageId"`
	Reaction  string `json:"reaction"`
}

type CompleteRequest struct {
	MessageID     string `json:"messageId"`
	Content       string `json:"content,omitempty"`
	FinalReaction string `json:"finalReaction,omitempty"`
}

func (c *Client) Join(ctx context.Context, req JoinRequest) (Binding, error) {
	var binding Binding
	if err := c.doJSON(ctx, http.MethodPost, "/join", req, &binding); err != nil {
		return Binding{}, err
	}
	return binding, nil
}

func (c *Client) PollEvents(ctx context.Context, agentID string) ([]InboundEvent, error) {
	var payload PollEventsResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/agents/%s/events", agentID), nil, &payload); err != nil {
		return nil, err
	}
	return payload.Events, nil
}

func (c *Client) UpdateStatus(ctx context.Context, agentID string, req StatusRequest) error {
	return c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/agents/%s/status", agentID), req, nil)
}

func (c *Client) Complete(ctx context.Context, agentID string, req CompleteRequest) error {
	return c.doJSON(ctx, http.MethodPost, fmt.Sprintf("/agents/%s/complete", agentID), req, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var req *http.Request
	var err error
	if body != nil {
		blob, err := json.Marshal(body)
		if err != nil {
			return err
		}
		req, err = http.NewRequestWithContext(ctx, method, c.baseURL+path, strings.NewReader(string(blob)))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
		if err != nil {
			return err
		}
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("bridge %s %s: status %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
