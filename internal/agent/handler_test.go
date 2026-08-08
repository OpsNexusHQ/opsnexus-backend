package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/OpsNexusHQ/opsnexus-common/models"
	"github.com/jackc/pgx/v5"
)

type mockAgentStore struct {
	registerFunc func(ctx context.Context, registration models.AgentRegistration) (models.Agent, error)
	listFunc     func(ctx context.Context) ([]models.Agent, error)
	getByIDFunc  func(ctx context.Context, id string) (models.Agent, error)
}

func (m *mockAgentStore) Register(ctx context.Context, registration models.AgentRegistration) (models.Agent, error) {
	return m.registerFunc(ctx, registration)
}

func (m *mockAgentStore) List(ctx context.Context) ([]models.Agent, error) {
	return m.listFunc(ctx)
}

func (m *mockAgentStore) GetByID(ctx context.Context, id string) (models.Agent, error) {
	return m.getByIDFunc(ctx, id)
}

func TestHandlerList(t *testing.T) {
	store := &mockAgentStore{
		listFunc: func(ctx context.Context) ([]models.Agent, error) {
			return []models.Agent{
				{
					ID:        "agent-1",
					Name:      "Agent One",
					Hostname:  "host-1",
					OS:        "linux",
					Arch:      "amd64",
					Version:   "1.0.0",
					Status:    "online",
					LastSeen:  time.Unix(1000000, 0),
					CreatedAt: time.Unix(1000000, 0),
				},
			}, nil
		},
	}

	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	rr := httptest.NewRecorder()

	handler.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}

	var resp map[string][]models.Agent
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	agents := resp["agents"]
	if len(agents) != 1 || agents[0].ID != "agent-1" {
		t.Errorf("unexpected agents response: %v", agents)
	}
}

func TestHandlerGet(t *testing.T) {
	store := &mockAgentStore{
		getByIDFunc: func(ctx context.Context, id string) (models.Agent, error) {
			if id == "agent-1" {
				return models.Agent{
					ID:       "agent-1",
					Name:     "Agent One",
					Hostname: "host-1",
				}, nil
			}
			return models.Agent{}, pgx.ErrNoRows
		},
	}

	handler := NewHandler(store)

	t.Run("found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.Get(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}

		var agent models.Agent
		if err := json.NewDecoder(rr.Body).Decode(&agent); err != nil {
			t.Fatalf("failed to decode agent: %v", err)
		}
		if agent.ID != "agent-1" {
			t.Errorf("expected ID 'agent-1', got %s", agent.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-2", nil)
		req.SetPathValue("id", "agent-2")
		rr := httptest.NewRecorder()

		handler.Get(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rr.Code)
		}

		var errResp map[string]string
		if err := json.NewDecoder(rr.Body).Decode(&errResp); err != nil {
			t.Fatalf("failed to decode error: %v", err)
		}
		if errResp["error"] != "agent not found" {
			t.Errorf("unexpected error message: %q", errResp["error"])
		}
	})

	t.Run("internal server error", func(t *testing.T) {
		store.getByIDFunc = func(ctx context.Context, id string) (models.Agent, error) {
			return models.Agent{}, errors.New("db error")
		}

		req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/agent-1", nil)
		req.SetPathValue("id", "agent-1")
		rr := httptest.NewRecorder()

		handler.Get(rr, req)

		if rr.Code != http.StatusInternalServerError {
			t.Errorf("expected status 500, got %d", rr.Code)
		}
	})
}
