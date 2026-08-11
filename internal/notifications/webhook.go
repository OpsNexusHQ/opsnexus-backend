package notifications

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type WebhookProvider struct {
	client *http.Client
}

func NewWebhookProvider(timeout time.Duration) *WebhookProvider {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &WebhookProvider{
		client: &http.Client{Timeout: timeout},
	}
}

func (p *WebhookProvider) Send(ctx context.Context, channelURL, secret string, notification Notification) (int, error) {
	payloadBytes, err := json.Marshal(notification)
	if err != nil {
		return 0, fmt.Errorf("marshal notification payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, channelURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return 0, fmt.Errorf("create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "OpsNexus-NotificationService/1.0")

	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(payloadBytes)
		signature := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-OpsNexus-Signature", "sha256="+signature)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("dispatch webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("webhook responded with HTTP %d", resp.StatusCode)
	}

	return resp.StatusCode, nil
}
