package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBridgeClientJoinAndPoll(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/join", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		_ = json.NewEncoder(w).Encode(Binding{AgentID: "main", ChannelID: "123", Active: true})
	})
	mux.HandleFunc("/agents/main/events", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(EventPollResponse{Events: []InboundEvent{{EventID: "evt_1", MessageID: "msg_1"}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := NewBridgeClient(server.URL)
	binding, err := client.Join(JoinRequest{AgentID: "main", CredsRef: "local", RequestedChannelID: "123"})
	if err != nil {
		t.Fatal(err)
	}
	if binding.ChannelID != "123" {
		t.Fatalf("unexpected binding: %+v", binding)
	}
	events, err := client.PollEvents("main")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "evt_1" {
		t.Fatalf("unexpected events: %+v", events)
	}
}
