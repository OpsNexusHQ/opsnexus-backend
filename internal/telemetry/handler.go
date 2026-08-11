package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/OpsNexusHQ/opsnexus-backend/internal/alerting"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/events"
	"github.com/OpsNexusHQ/opsnexus-common/models"
	"github.com/jackc/pgx/v5"
)

// TelemetryStore defines database operations needed by the telemetry Handler.
type TelemetryStore interface {
	Store(ctx context.Context, telemetry models.AgentTelemetry) error
	AgentExists(ctx context.Context, agentID string) (bool, error)
	GetLatest(ctx context.Context, agentID string) (TelemetryRecord, error)
	GetHistory(ctx context.Context, agentID string, limit int, from, to time.Time) ([]TelemetryRecord, error)
}

// Handler handles telemetry HTTP requests.
type Handler struct {
	repository TelemetryStore
	hub        *events.Hub
	engine     *alerting.Engine
}

// NewHandler creates a telemetry handler.
func NewHandler(repository TelemetryStore) *Handler {
	return &Handler{
		repository: repository,
	}
}

func (h *Handler) WithEvents(hub *events.Hub) *Handler {
	h.hub = hub
	return h
}

func (h *Handler) WithAlerting(engine *alerting.Engine) *Handler {
	h.engine = engine
	return h
}

// Ingest accepts telemetry reported by an OpsNexus agent.
func (h *Handler) decodeTelemetryPayload(r *http.Request) (models.AgentTelemetry, error) {
	var payload models.AgentTelemetry

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&payload); err != nil {
		return payload, err
	}

	if strings.TrimSpace(payload.AgentID) == "" {
		return payload, errors.New("agent_id is required")
	}

	if payload.Timestamp.IsZero() {
		return payload, errors.New("timestamp is required")
	}

	if payload.Metrics == nil {
		return payload, errors.New("metrics is required")
	}

	return payload, nil
}

func (h *Handler) persistTelemetry(w http.ResponseWriter, r *http.Request, payload models.AgentTelemetry) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	if err := h.repository.Store(ctx, payload); err != nil {
		h.sendError(w, "failed to store telemetry", http.StatusInternalServerError)
		return
	}

	// Async non-blocking SSE broadcast and alert evaluation
	if h.hub != nil {
		h.hub.Publish(events.Event{
			Type:      events.EventTelemetryUpdated,
			AgentID:   payload.AgentID,
			Timestamp: payload.Timestamp,
			Metrics:   payload.Metrics,
		})
	}

	if h.engine != nil {
		go h.engine.EvaluateTelemetry(context.Background(), payload)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	payload, err := h.decodeTelemetryPayload(r)
	if err != nil {
		h.sendError(w, "invalid telemetry payload", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	exists, err := h.repository.AgentExists(ctx, payload.AgentID)
	if err != nil {
		h.sendError(w, "failed to check agent existence", http.StatusInternalServerError)
		return
	}
	if !exists {
		h.sendError(w, "agent not found", http.StatusNotFound)
		return
	}

	h.persistTelemetry(w, r, payload)
}

// GetLatest handles GET /api/v1/agents/{id}/telemetry/latest.
func (h *Handler) GetLatest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "agent ID is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Check if agent exists first
	exists, err := h.repository.AgentExists(ctx, id)
	if err != nil {
		h.sendError(w, "failed to check agent existence", http.StatusInternalServerError)
		return
	}
	if !exists {
		h.sendError(w, "agent not found", http.StatusNotFound)
		return
	}

	// Fetch latest telemetry
	record, err := h.repository.GetLatest(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.sendError(w, "no telemetry records found for agent", http.StatusNotFound)
			return
		}
		h.sendError(w, "failed to retrieve latest telemetry", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(record)
}

// HistoryRecord represents a historical telemetry entry in the response list.
type HistoryRecord struct {
	Timestamp time.Time      `json:"timestamp"`
	Metrics   map[string]any `json:"metrics"`
	CreatedAt time.Time      `json:"created_at"`
}

// GetHistory handles GET /api/v1/agents/{id}/telemetry.
func (h *Handler) GetHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "agent ID is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	// Check if agent exists first
	exists, err := h.repository.AgentExists(ctx, id)
	if err != nil {
		h.sendError(w, "failed to check agent existence", http.StatusInternalServerError)
		return
	}
	if !exists {
		h.sendError(w, "agent not found", http.StatusNotFound)
		return
	}

	// Parse query params
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed <= 0 {
			h.sendError(w, "invalid limit parameter", http.StatusBadRequest)
			return
		}
		if parsed > 500 {
			h.sendError(w, "limit cannot exceed 500", http.StatusBadRequest)
			return
		}
		limit = parsed
	}

	var fromTime time.Time
	fromStr := r.URL.Query().Get("from")
	if fromStr != "" {
		parsed, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			h.sendError(w, "invalid from parameter format (expected RFC3339)", http.StatusBadRequest)
			return
		}
		fromTime = parsed
	}

	var toTime time.Time
	toStr := r.URL.Query().Get("to")
	if toStr != "" {
		parsed, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			h.sendError(w, "invalid to parameter format (expected RFC3339)", http.StatusBadRequest)
			return
		}
		toTime = parsed
	}

	records, err := h.repository.GetHistory(ctx, id, limit, fromTime, toTime)
	if err != nil {
		h.sendError(w, "failed to retrieve historical telemetry", http.StatusInternalServerError)
		return
	}

	history := make([]HistoryRecord, 0, len(records))
	for _, r := range records {
		history = append(history, HistoryRecord{
			Timestamp: r.Timestamp,
			Metrics:   r.Metrics,
			CreatedAt: r.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"agent_id":  id,
		"telemetry": history,
	})
}

// IngestForAgent handles POST /api/v1/agents/{id}/telemetry.
func (h *Handler) IngestForAgent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "agent ID path parameter is required", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()

	payload, err := h.decodeTelemetryPayload(r)
	if err != nil {
		h.sendError(w, "invalid telemetry payload", http.StatusBadRequest)
		return
	}

	if payload.AgentID != id {
		h.sendError(w, "agent_id in payload does not match path parameter", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	exists, err := h.repository.AgentExists(ctx, id)
	if err != nil {
		h.sendError(w, "failed to check agent existence", http.StatusInternalServerError)
		return
	}
	if !exists {
		h.sendError(w, "agent not found", http.StatusNotFound)
		return
	}

	h.persistTelemetry(w, r, payload)
}

func (h *Handler) sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"message": message,
		},
	})
}
