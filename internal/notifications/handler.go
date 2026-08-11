package notifications

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type createChannelRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	URL     string `json:"url"`
	Secret  string `json:"secret,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
}

func (r *createChannelRequest) normalize() (*Channel, error) {
	if strings.TrimSpace(r.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(r.URL) == "" {
		return nil, fmt.Errorf("url is required")
	}

	parsedURL, err := url.Parse(strings.TrimSpace(r.URL))
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("url must be a valid http or https URL")
	}
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("url must use http or https")
	}

	typeValue := strings.TrimSpace(r.Type)
	if typeValue == "" {
		typeValue = "webhook"
	}
	if typeValue != "webhook" && typeValue != "slack_webhook" {
		return nil, fmt.Errorf("type must be either webhook or slack_webhook")
	}

	enabled := true
	if r.Enabled != nil {
		enabled = *r.Enabled
	}

	return &Channel{
		Name:    strings.TrimSpace(r.Name),
		Type:    typeValue,
		URL:     parsedURL.String(),
		Secret:  strings.TrimSpace(r.Secret),
		Enabled: enabled,
	}, nil
}

type Handler struct {
	repo  *Repository
	queue *Queue
}

func NewHandler(repo *Repository, queue *Queue) *Handler {
	return &Handler{
		repo:  repo,
		queue: queue,
	}
}

func (h *Handler) ListChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	channels, err := h.repo.ListChannels(r.Context())
	if err != nil {
		h.sendError(w, "failed to list notification channels", http.StatusInternalServerError)
		return
	}

	h.sendJSON(w, http.StatusOK, map[string]any{"channels": channels})
}

func (h *Handler) CreateChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	defer r.Body.Close()
	var req createChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "invalid notification channel payload", http.StatusBadRequest)
		return
	}

	channel, err := req.normalize()
	if err != nil {
		h.sendError(w, err.Error(), http.StatusBadRequest)
		return
	}

	if channel.ID == "" {
		channel.ID = fmt.Sprintf("chan-%d", time.Now().UnixNano())
	}

	created, err := h.repo.CreateChannel(r.Context(), *channel)
	if err != nil {
		h.sendError(w, "failed to create channel", http.StatusInternalServerError)
		return
	}

	h.sendJSON(w, http.StatusCreated, created)
}

func (h *Handler) DeleteChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	if err := h.repo.DeleteChannel(r.Context(), id); err != nil {
		h.sendError(w, "failed to delete channel", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) TestChannel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.sendError(w, "channel ID is required", http.StatusBadRequest)
		return
	}

	ch, err := h.repo.GetChannelByID(r.Context(), id)
	if err != nil {
		h.sendError(w, "notification channel not found", http.StatusNotFound)
		return
	}

	testNotif := Notification{
		ID:        fmt.Sprintf("test-%d", time.Now().UnixNano()),
		Event:     "alert.firing",
		Timestamp: time.Now(),
		Alert: map[string]any{
			"id":        "test-alert",
			"rule_name": "Test Webhook Alert",
			"status":    "firing",
			"message":   "This is a test notification from OpsNexus.",
		},
	}

	var prov Provider
	if ch.Type == "slack_webhook" {
		prov = NewSlackWebhookProvider(10 * time.Second)
	} else {
		prov = NewWebhookProvider(10 * time.Second)
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	statusCode, err := prov.Send(ctx, ch.URL, ch.Secret, testNotif)
	if err != nil {
		h.sendJSON(w, http.StatusOK, map[string]any{
			"status":          "failed",
			"response_status": statusCode,
			"error":           err.Error(),
		})
		return
	}

	h.sendJSON(w, http.StatusOK, map[string]any{
		"status":          "delivered",
		"response_status": statusCode,
	})
}

func (h *Handler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	deliveries, err := h.repo.ListDeliveries(r.Context(), 50)
	if err != nil {
		h.sendError(w, "failed to list deliveries", http.StatusInternalServerError)
		return
	}

	h.sendJSON(w, http.StatusOK, map[string]any{"deliveries": deliveries})
}

func (h *Handler) sendJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) sendError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"message": message},
	})
}
