package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

type EventType string

const (
	EventTelemetryUpdated  EventType = "telemetry.updated"
	EventAgentRegistered   EventType = "agent.registered"
	EventAgentStatus       EventType = "agent.status_changed"
	EventAlertFiring       EventType = "alert.firing"
	EventAlertAcknowledged EventType = "alert.acknowledged"
	EventAlertResolved     EventType = "alert.resolved"
	EventAlertCommentAdded EventType = "alert.comment_added"
)

type Event struct {
	Type      EventType      `json:"type"`
	AgentID   string         `json:"agent_id,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
	Metrics   map[string]any `json:"metrics,omitempty"`
	Alert     any            `json:"alert,omitempty"`
	Status    string         `json:"status,omitempty"`
	Data      any            `json:"data,omitempty"`
}

type Client struct {
	ID   string
	Send chan Event
}

type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]bool
	register   chan *Client
	unregister chan *Client
	broadcast  chan Event
	logger     *slog.Logger
}

func NewHub(logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client, 64),
		unregister: make(chan *Client, 64),
		broadcast:  make(chan Event, 256),
		logger:     logger,
	}
}

func (h *Hub) Run(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.mu.Lock()
			for client := range h.clients {
				close(client.Send)
				delete(h.clients, client)
			}
			h.mu.Unlock()
			return

		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			h.logger.Debug("SSE client connected", slog.String("client_id", client.ID))

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.Send)
				h.logger.Debug("SSE client disconnected", slog.String("client_id", client.ID))
			}
			h.mu.Unlock()

		case event := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- event:
				default:
					// Slow client channel buffer full; drop event to avoid blocking ingestion
					h.logger.Warn("SSE client channel buffer full, dropping event", slog.String("client_id", client.ID))
				}
			}
			h.mu.RUnlock()

		case <-ticker.C:
			// Periodic keep-alive heartbeat ping
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.Send <- Event{Type: "ping", Timestamp: time.Now()}:
				default:
				}
			}
			h.mu.RUnlock()
		}
	}
}

func (h *Hub) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	select {
	case h.broadcast <- event:
	default:
		h.logger.Warn("SSE hub broadcast buffer full, event dropped", slog.String("event_type", string(event.Type)))
	}
}

func (h *Hub) Subscribe(clientID string) (*Client, func()) {
	client := &Client{
		ID:   clientID,
		Send: make(chan Event, 64),
	}
	h.register <- client

	unsubscribe := func() {
		h.unregister <- client
	}

	return client, unsubscribe
}

func FormatSSEData(event Event) ([]byte, error) {
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}
	return []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", event.Type, data)), nil
}
