package bridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientJoinAndPollEvents(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/join", func(w http.ResponseWriter, r *http.Request) {
		var req JoinRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatal(err)
		}
		if req.AgentID != "main" {
			t.Fatalf("unexpected agent id: %s", req.AgentID)
		}
		_ = json.NewEncoder(w).Encode(Binding{AgentID: req.AgentID, ChannelID: "c1", Active: true})
	})
	mux.HandleFunc("/agents/main/events", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(PollEventsResponse{Events: []InboundEvent{{EventID: "evt_1", MessageID: "msg_1", Content: "hello"}}})
	})
	server := httptest.NewServer(mux)
	defer server.Close()
	client := NewClient(server.URL)
	binding, err := client.Join(context.Background(), JoinRequest{AgentID: "main", CredsRef: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if binding.AgentID != "main" {
		t.Fatalf("unexpected binding: %+v", binding)
	}
	events, err := client.PollEvents(context.Background(), "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "evt_1" {
		t.Fatalf("unexpected events: %+v", events)
	}
}
