package observability

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type Handler struct{ service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	h.call(w, r, func(id string) (any, error) { return h.service.Health(r.Context(), id) })
}
func (h *Handler) Latest(w http.ResponseWriter, r *http.Request) {
	h.call(w, r, func(id string) (any, error) { return h.service.Latest(r.Context(), id) })
}
func (h *Handler) History(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, "method not allowed", 405)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		h.error(w, "agent ID is required", 400)
		return
	}
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n <= 0 || n > 500 {
			h.error(w, "invalid limit parameter", 400)
			return
		}
		limit = n
	}
	from, to, e := parseTimes(r)
	if e != nil {
		h.error(w, e.Error(), 400)
		return
	}
	result, e := h.service.History(r.Context(), id, limit, from, to)
	h.respond(w, result, e)
}

func (h *Handler) Analytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, "method not allowed", 405)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		h.error(w, "agent ID is required", 400)
		return
	}
	timeRange := r.URL.Query().Get("range")
	if timeRange == "" {
		timeRange = "1h"
	}

	result, err := h.service.Analytics(r.Context(), id, timeRange)
	h.respond(w, result, err)
}

func parseTimes(r *http.Request) (time.Time, time.Time, error) {
	var f, t time.Time
	for name, p := range map[string]*time.Time{"from": &f, "to": &t} {
		if v := r.URL.Query().Get(name); v != "" {
			x, e := time.Parse(time.RFC3339, v)
			if e != nil {
				return f, t, errors.New("invalid " + name + " parameter format (expected RFC3339)")
			}
			*p = x
		}
	}
	return f, t, nil
}
func (h *Handler) Overview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		h.error(w, "method not allowed", 405)
		return
	}
	v, e := h.service.Overview(r.Context())
	h.respond(w, v, e)
}
func (h *Handler) call(w http.ResponseWriter, r *http.Request, fn func(string) (any, error)) {
	if r.Method != http.MethodGet {
		h.error(w, "method not allowed", 405)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		h.error(w, "agent ID is required", 400)
		return
	}
	v, e := fn(id)
	h.respond(w, v, e)
}
func (h *Handler) respond(w http.ResponseWriter, v any, e error) {
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			h.error(w, "no telemetry records found for agent", 404)
		} else {
			h.error(w, "failed to retrieve observability data", 500)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	_ = json.NewEncoder(w).Encode(v)
}
func (h *Handler) error(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{
			"message": msg,
		},
	})
}
