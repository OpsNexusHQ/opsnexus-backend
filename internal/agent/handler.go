package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/OpsNexusHQ/opsnexus-backend/internal/events"
	"github.com/OpsNexusHQ/opsnexus-common/models"
	"github.com/jackc/pgx/v5"
)

// AgentStore defines database operations needed by the agent Handler.
type AgentStore interface {
	Register(ctx context.Context, registration models.AgentRegistration) (models.Agent, error)
	List(ctx context.Context) ([]models.Agent, error)
	GetByID(ctx context.Context, id string) (models.Agent, error)
}

// Handler handles agent HTTP requests.
type Handler struct {
	repository AgentStore
	hub        *events.Hub
}

// NewHandler creates an agent HTTP handler.
func NewHandler(repository AgentStore) *Handler {
	return &Handler{
		repository: repository,
	}
}

func (h *Handler) WithEvents(hub *events.Hub) *Handler {
	h.hub = hub
	return h
}

// Register handles POST /api/v1/agents/register.
func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()

	var registration models.AgentRegistration

	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&registration); err != nil {
		h.sendError(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if registration.ID == "" ||
		registration.Name == "" ||
		registration.Hostname == "" ||
		registration.OS == "" ||
		registration.Arch == "" ||
		registration.Version == "" {
		h.sendError(w, "missing required agent fields", http.StatusBadRequest)
		return
	}

	agent, err := h.repository.Register(r.Context(), registration)
	if err != nil {
		h.sendError(w, "failed to register agent", http.StatusInternalServerError)
		return
	}

	if h.hub != nil {
		h.hub.Publish(events.Event{
			Type:      events.EventAgentRegistered,
			AgentID:   agent.ID,
			Timestamp: time.Now(),
			Data:      agent,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(agent)
}

// List handles GET /api/v1/agents.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agents, err := h.repository.List(r.Context())
	if err != nil {
		h.sendError(w, "failed to list agents", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"agents": agents,
	})
}

// Get handles GET /api/v1/agents/{id}.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "agent ID is required", http.StatusBadRequest)
		return
	}

	agent, err := h.repository.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.sendError(w, "agent not found", http.StatusNotFound)
			return
		}
		h.sendError(w, "failed to retrieve agent", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(agent)
}

func (h *Handler) sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
