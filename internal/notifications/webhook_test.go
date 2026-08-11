package notifications

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWebhookProviderHMACSignature(t *testing.T) {
	var receivedSig string
	var bodyBytes []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-OpsNexus-Signature")
		bodyBytes, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	provider := NewWebhookProvider(5 * time.Second)
	notif := Notification{
		ID:        "notif-1",
		Event:     EventAlertFiring,
		Timestamp: time.Now(),
		Alert:     map[string]any{"id": "alert-1"},
	}

	secret := "super-secret-key"
	status, err := provider.Send(context.Background(), server.URL, secret, notif)
	if err != nil {
		t.Fatalf("send failed: %v", err)
	}

	if status != http.StatusOK {
		t.Errorf("expected status 200, got %d", status)
	}

	if receivedSig == "" {
		t.Error("expected X-OpsNexus-Signature header, got empty")
	}

	if len(bodyBytes) == 0 {
		t.Error("expected non-empty request body")
	}
}
