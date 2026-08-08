package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/OpsNexusHQ/opsnexus-common/models"
	"github.com/jackc/pgx/v5"
)

type mockTelemetryStore struct {
	storeFunc       func(ctx context.Context, telemetry models.AgentTelemetry) error
	agentExistsFunc func(ctx context.Context, agentID string) (bool, error)
	getLatestFunc   func(ctx context.Context, agentID string) (TelemetryRecord, error)
	getHistoryFunc  func(ctx context.Context, agentID string, limit int, from, to time.Time) ([]TelemetryRecord, error)
}

func (m *mockTelemetryStore) Store(ctx context.Context, telemetry models.AgentTelemetry) error {
	return m.storeFunc(ctx, telemetry)
}

func (m *mockTelemetryStore) AgentExists(ctx context.Context, agentID string) (bool, error) {
	return m.agentExistsFunc(ctx, agentID)
}

func (m *mockTelemetryStore) GetLatest(ctx context.Context, agentID string) (TelemetryRecord, error) {
	return m.getLatestFunc(ctx, agentID)
}

func (m *mockTelemetryStore) GetHistory(ctx context.Context, agentID string, limit int, from, to time.Time) ([]TelemetryRecord, error) {
	return m.getHistoryFunc(ctx, agentID, limit, from, to)
}

func TestTelemetryHandlerGetLatest(t *testing.T) {
	store := &mockTelemetryStore{
		agentExistsFunc: func(ctx context.Context, agentID string) (bool, error) {
			return agentID == "agent-1", nil
		},
		getLatestFunc: func(ctx context.Context, agentID string) (TelemetryRecord, error) {
			if agentID == "agent-1" {
				return TelemetryRecord{
					AgentID:   "agent-1",
					Timestamp: time.Unix(1000000, 0),
					Metrics:   map[string]any{"cpu": 15.4},
					CreatedAt: time.Unix(1000000, 0),
				}, nil
			}
			return TelemetryRecord{}, pgx.ErrNoRows
		},
	}

	handler := NewHandler(store)

	t.Run("found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/telemetry/latest", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.GetLatest(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}

		var resp TelemetryRecord
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp.AgentID != "agent-1" || resp.Metrics["cpu"] != 15.4 {
			t.Errorf("unexpected telemetry output: %+v", resp)
		}
	})

	t.Run("agent not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-2/telemetry/latest", nil)
		req.SetPathValue("id", "agent-2")
		rr := httptest.NewRecorder()

		handler.GetLatest(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rr.Code)
		}
	})

	t.Run("agent exists but no telemetry", func(t *testing.T) {
		store.getLatestFunc = func(ctx context.Context, agentID string) (TelemetryRecord, error) {
			return TelemetryRecord{}, pgx.ErrNoRows
		}
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/telemetry/latest", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.GetLatest(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rr.Code)
		}
	})
}

func TestTelemetryHandlerGetHistory(t *testing.T) {
	store := &mockTelemetryStore{
		agentExistsFunc: func(ctx context.Context, agentID string) (bool, error) {
			return agentID == "agent-1", nil
		},
		getHistoryFunc: func(ctx context.Context, agentID string, limit int, from, to time.Time) ([]TelemetryRecord, error) {
			return []TelemetryRecord{
				{
					AgentID:   "agent-1",
					Timestamp: time.Unix(1000000, 0),
					Metrics:   map[string]any{"cpu": 15.4},
					CreatedAt: time.Unix(1000000, 0),
				},
			}, nil
		},
	}

	handler := NewHandler(store)

	t.Run("default limit and successful history fetch", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/telemetry", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.GetHistory(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}

		var resp map[string]any
		if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp["agent_id"] != "agent-1" {
			t.Errorf("expected agent_id 'agent-1', got %v", resp["agent_id"])
		}

		telemetry, ok := resp["telemetry"].([]any)
		if !ok || len(telemetry) != 1 {
			t.Errorf("unexpected telemetry items: %v", resp["telemetry"])
		}
	})

	t.Run("invalid limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/telemetry?limit=abc", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.GetHistory(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rr.Code)
		}
	})

	t.Run("limit greater than 500", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/telemetry?limit=501", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.GetHistory(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rr.Code)
		}
	})

	t.Run("invalid from time format", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/telemetry?from=2026-08-08", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.GetHistory(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rr.Code)
		}
	})

	t.Run("valid timestamps time range", func(t *testing.T) {
		called := false
		store.getHistoryFunc = func(ctx context.Context, agentID string, limit int, from, to time.Time) ([]TelemetryRecord, error) {
			called = true
			if from.Format(time.RFC3339) != "2026-08-08T18:00:00Z" || to.Format(time.RFC3339) != "2026-08-08T19:00:00Z" {
				t.Errorf("unexpected time range received: from=%s to=%s", from.Format(time.RFC3339), to.Format(time.RFC3339))
			}
			return []TelemetryRecord{}, nil
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/telemetry?from=2026-08-08T18:00:00Z&to=2026-08-08T19:00:00Z", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.GetHistory(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		if !called {
			t.Error("expected repository GetHistory to be called")
		}
	})

	t.Run("database error", func(t *testing.T) {
		store.getHistoryFunc = func(ctx context.Context, agentID string, limit int, from, to time.Time) ([]TelemetryRecord, error) {
			return nil, errors.New("db error")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1/telemetry", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.GetHistory(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", rr.Code)
		}
	})
}

func TestTelemetryHandlerIngestForAgent(t *testing.T) {
	store := &mockTelemetryStore{
		agentExistsFunc: func(ctx context.Context, agentID string) (bool, error) {
			return agentID == "agent-1", nil
		},
		storeFunc: func(ctx context.Context, telemetry models.AgentTelemetry) error {
			return nil
		},
	}

	handler := NewHandler(store)

	t.Run("success", func(t *testing.T) {
		payload := `{"agent_id":"agent-1","timestamp":"2026-08-08T18:00:00Z","metrics":{"cpu":10.5}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-1/telemetry", strings.NewReader(payload))
		req.SetPathValue("id", "agent-1")
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.IngestForAgent(rr, req)

		if rr.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d", rr.Code)
		}
	})

		t.Run("accepts full network and processes payload", func(t *testing.T) {
			called := false
			store.storeFunc = func(ctx context.Context, telemetry models.AgentTelemetry) error {
				called = true
				net, ok := telemetry.Metrics["system"].(map[string]any)["network"]
				if !ok {
					t.Fatal("expected network data to be present in metrics")
				}
				proc, ok := telemetry.Metrics["system"].(map[string]any)["processes"]
				if !ok {
					t.Fatal("expected processes data to be present in metrics")
				}
				if net == nil {
					t.Fatal("expected non-nil network payload")
				}
				if proc == nil {
					t.Fatal("expected non-nil processes payload")
				}
				return nil
			}

			payload := `{"agent_id":"agent-1","timestamp":"2026-08-08T18:00:00Z","metrics":{"system":{"network":{"interfaces":[{"name":"eth0","bytes_sent":100}]},"processes":{"running_count":200}}}}`
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-1/telemetry", strings.NewReader(payload))
			req.SetPathValue("id", "agent-1")
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.IngestForAgent(rr, req)

			if rr.Code != http.StatusCreated {
				t.Errorf("expected status 201, got %d", rr.Code)
			}
			if !called {
				t.Fatal("expected store func to be called")
			}
		})

		t.Run("invalid path agent_id", func(t *testing.T) {
		payload := `{"agent_id":"agent-1","timestamp":"2026-08-08T18:00:00Z","metrics":{"cpu":10.5}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-2/telemetry", strings.NewReader(payload))
		req.SetPathValue("id", "agent-2")
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.IngestForAgent(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", rr.Code)
		}
	})

	t.Run("agent not found", func(t *testing.T) {
		payload := `{"agent_id":"agent-3","timestamp":"2026-08-08T18:00:00Z","metrics":{"cpu":10.5}}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/agent-3/telemetry", strings.NewReader(payload))
		req.SetPathValue("id", "agent-3")
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		handler.IngestForAgent(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rr.Code)
		}
	})
}
