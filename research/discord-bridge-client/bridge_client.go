package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type BridgeClient struct {
	baseURL string
	http    *http.Client
}

func NewBridgeClient(baseURL string) *BridgeClient {
	return &BridgeClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *BridgeClient) Join(req JoinRequest) error {
	return c.post("/join", req, nil)
}

func (c *BridgeClient) PollEvents(agentID string) ([]InboundEvent, error) {
	resp, err := c.http.Get(c.baseURL + "/agents/" + agentID + "/events")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, decodeHTTPError(resp)
	}
	var payload EventPollResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Events, nil
}

func (c *BridgeClient) SetStatus(agentID string, req StatusUpdateRequest) error {
	return c.post("/agents/"+agentID+"/status", req, nil)
}

func (c *BridgeClient) Complete(agentID string, req CompleteRequest) error {
	return c.post("/agents/"+agentID+"/complete", req, nil)
}

func (c *BridgeClient) post(path string, requestBody any, responseTarget any) error {
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return decodeHTTPError(resp)
	}
	if responseTarget != nil {
		return json.NewDecoder(resp.Body).Decode(responseTarget)
	}
	return nil
}

func decodeHTTPError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("bridge http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
