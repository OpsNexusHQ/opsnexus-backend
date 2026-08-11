package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHasPermission(t *testing.T) {
	tests := []struct {
		userRole Role
		required Role
		expected bool
	}{
		{RoleAdmin, RoleAdmin, true},
		{RoleAdmin, RoleOperator, true},
		{RoleAdmin, RoleViewer, true},
		{RoleOperator, RoleAdmin, false},
		{RoleOperator, RoleOperator, true},
		{RoleOperator, RoleViewer, true},
		{RoleViewer, RoleAdmin, false},
		{RoleViewer, RoleOperator, false},
		{RoleViewer, RoleViewer, true},
	}

	for _, tt := range tests {
		got := hasPermission(tt.userRole, tt.required)
		if got != tt.expected {
			t.Errorf("hasPermission(%s, %s) = %v; expected %v", tt.userRole, tt.required, got, tt.expected)
		}
	}
}

func TestDisabledAuthMiddleware(t *testing.T) {
	authenticator := NewAuthenticator(nil, false)
	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role, ok := r.Context().Value(UserRoleContextKey).(Role)
		if !ok || role != RoleAdmin {
			t.Errorf("expected RoleAdmin in context when auth disabled, got %v", role)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}

func TestUnauthorizedResponseIncludesAuthenticateHeader(t *testing.T) {
	authenticator := NewAuthenticator(nil, true)
	handler := authenticator.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/agents", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	if got := rec.Header().Get("WWW-Authenticate"); got != `Bearer realm="OpsNexus"` {
		t.Fatalf("expected WWW-Authenticate header, got %q", got)
	}
}

func TestRequireRoleMiddleware(t *testing.T) {
	handler := RequireRole(RoleAdmin)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserRoleContextKey, RoleOperator))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req = req.WithContext(context.WithValue(req.Context(), UserRoleContextKey, RoleAdmin))
	rec = httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestValidRoleValues(t *testing.T) {
	tests := []struct {
		role     Role
		expected bool
	}{
		{RoleViewer, true},
		{RoleOperator, true},
		{RoleAdmin, true},
		{"invalid", false},
	}

	for _, tt := range tests {
		if got := tt.role.IsValid(); got != tt.expected {
			t.Errorf("expected IsValid() for role %q to be %v, got %v", tt.role, tt.expected, got)
		}
	}
}
