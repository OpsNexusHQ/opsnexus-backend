package alerting

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/OpsNexusHQ/opsnexus-backend/internal/events"
	"github.com/jackc/pgx/v5"
)

type Handler struct {
	repo *Repository
	hub  *events.Hub
}

func NewHandler(repo *Repository) *Handler {
	return &Handler{repo: repo}
}

func (h *Handler) WithEvents(hub *events.Hub) *Handler {
	h.hub = hub
	return h
}

func (h *Handler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := r.URL.Query().Get("status")
	agentID := r.URL.Query().Get("agent_id")

	alerts, err := h.repo.ListAlerts(r.Context(), status, agentID)
	if err != nil {
		h.sendError(w, "failed to list alerts", http.StatusInternalServerError)
		return
	}

	h.sendJSON(w, http.StatusOK, map[string]any{"alerts": alerts})
}

func (h *Handler) GetAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "alert ID is required", http.StatusBadRequest)
		return
	}

	alert, err := h.repo.GetAlertByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			h.sendError(w, "alert not found", http.StatusNotFound)
			return
		}
		h.sendError(w, "failed to retrieve alert", http.StatusInternalServerError)
		return
	}

	h.sendJSON(w, http.StatusOK, alert)
}

type ackRequest struct {
	User    string `json:"user"`
	Comment string `json:"comment"`
}

func (h *Handler) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "alert ID is required", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	var body ackRequest
	_ = json.NewDecoder(r.Body).Decode(&body)

	user := body.User
	if user == "" {
		user = "operator"
	}

	ackedAlert, err := h.repo.AcknowledgeAlert(r.Context(), id, user, body.Comment)
	if err != nil {
		h.sendError(w, "failed to acknowledge alert", http.StatusInternalServerError)
		return
	}

	if h.hub != nil {
		h.hub.Publish(events.Event{
			Type:      events.EventAlertAcknowledged,
			AgentID:   ackedAlert.AgentID,
			Timestamp: time.Now(),
			Alert:     ackedAlert,
		})
	}

	h.sendJSON(w, http.StatusOK, ackedAlert)
}

func (h *Handler) ListComments(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "alert ID is required", http.StatusBadRequest)
		return
	}

	comments, err := h.repo.ListComments(r.Context(), id)
	if err != nil {
		h.sendError(w, "failed to list comments", http.StatusInternalServerError)
		return
	}

	h.sendJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

type commentRequest struct {
	AuthorID string `json:"author_id"`
	Comment  string `json:"comment"`
}

func (h *Handler) AddComment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "alert ID is required", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	var body commentRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || strings.TrimSpace(body.Comment) == "" {
		h.sendError(w, "comment content is required", http.StatusBadRequest)
		return
	}

	author := body.AuthorID
	if author == "" {
		author = "operator"
	}

	c := AlertComment{
		ID:       fmt.Sprintf("cmt-%d", time.Now().UnixNano()),
		AlertID:  id,
		AuthorID: author,
		Comment:  body.Comment,
	}

	created, err := h.repo.AddComment(r.Context(), c)
	if err != nil {
		h.sendError(w, "failed to add comment", http.StatusInternalServerError)
		return
	}

	if h.hub != nil {
		h.hub.Publish(events.Event{
			Type:      events.EventAlertCommentAdded,
			Timestamp: time.Now(),
			Data:      created,
		})
	}

	h.sendJSON(w, http.StatusCreated, created)
}

func (h *Handler) ListRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rules, err := h.repo.ListRules(r.Context())
	if err != nil {
		h.sendError(w, "failed to list alert rules", http.StatusInternalServerError)
		return
	}

	h.sendJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (h *Handler) CreateRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()
	var rule AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		h.sendError(w, "invalid alert rule payload", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(rule.Name) == "" || strings.TrimSpace(rule.Metric) == "" || strings.TrimSpace(rule.Operator) == "" {
		h.sendError(w, "name, metric, and operator are required", http.StatusBadRequest)
		return
	}

	if rule.ID == "" {
		rule.ID = fmt.Sprintf("rule-%s-%d", rule.Metric, time.Now().UnixNano())
	}

	created, err := h.repo.CreateRule(r.Context(), rule)
	if err != nil {
		h.sendError(w, "failed to create alert rule", http.StatusInternalServerError)
		return
	}

	h.sendJSON(w, http.StatusCreated, created)
}

func (h *Handler) UpdateRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "alert rule ID is required", http.StatusBadRequest)
		return
	}

	defer r.Body.Close()
	var rule AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		h.sendError(w, "invalid alert rule payload", http.StatusBadRequest)
		return
	}

	rule.ID = id
	updated, err := h.repo.UpdateRule(r.Context(), rule)
	if err != nil {
		h.sendError(w, "failed to update alert rule", http.StatusInternalServerError)
		return
	}

	h.sendJSON(w, http.StatusOK, updated)
}

func (h *Handler) DeleteRule(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "alert rule ID is required", http.StatusBadRequest)
		return
	}

	if err := h.repo.DeleteRule(r.Context(), id); err != nil {
		h.sendError(w, "failed to delete alert rule", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) sendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
