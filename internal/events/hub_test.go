package events

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSSEHubBroadcastAndDisconnect(t *testing.T) {
	hub := NewHub(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)

	client, unsubscribe := hub.Subscribe("test-client-1")
	defer unsubscribe()

	// Wait briefly for hub loop to process registration
	time.Sleep(50 * time.Millisecond)

	event := Event{
		Type:    EventTelemetryUpdated,
		AgentID: "agent-test",
		Metrics: map[string]any{"cpu": 42.0},
	}

	hub.Publish(event)

	select {
	case received := <-client.Send:
		if received.Type != EventTelemetryUpdated {
			t.Errorf("expected event type %s, got %s", EventTelemetryUpdated, received.Type)
		}
		if received.AgentID != "agent-test" {
			t.Errorf("expected agent_id 'agent-test', got %s", received.AgentID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestSSEHandlerHeadersAndStream(t *testing.T) {
	hub := NewHub(nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go hub.Run(ctx)

	handler := NewHandler(hub)

	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %s", resp.Header.Get("Content-Type"))
	}

	buf := make([]byte, 256)
	n, err := resp.Body.Read(buf)
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("failed to read stream: %v", err)
	}

	out := string(buf[:n])
	if !strings.Contains(out, ": connected") {
		t.Errorf("expected initial connection comment, got: %s", out)
	}
}
