package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowedOriginSetsVaryHeader(t *testing.T) {
	handler := CORS("https://app.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/agents", nil)
	req.Header.Set("Origin", "https://app.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	res := rec.Result()
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("expected Access-Control-Allow-Origin to be allowed origin, got %q", got)
	}
	if got := res.Header.Get("Vary"); got != "Origin" {
		t.Fatalf("expected Vary header to be Origin, got %q", got)
	}
}

func TestCORSDisallowedOriginReturnsForbidden(t *testing.T) {
	handler := CORS("https://app.example.com")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "http://example.com/api/v1/agents", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, res.StatusCode)
	}
	if got := res.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("expected no Access-Control-Allow-Origin header for disallowed origin, got %q", got)
	}
}
