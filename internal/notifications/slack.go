package notifications

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type SlackWebhookProvider struct {
	client *http.Client
}

func NewSlackWebhookProvider(timeout time.Duration) *SlackWebhookProvider {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &SlackWebhookProvider{
		client: &http.Client{Timeout: timeout},
	}
}

type slackPayload struct {
	Text string `json:"text"`
}

func (p *SlackWebhookProvider) Send(ctx context.Context, channelURL, secret string, notification Notification) (int, error) {
	emoji := "🚨"
	if notification.Event == EventAlertResolved {
		emoji = "✅"
	} else if notification.Event == EventAlertAcknowledged {
		emoji = "👀"
	}

	msg := fmt.Sprintf("%s *OpsNexus Notification: %s*\nEvent: `%s` | Time: `%s`",
		emoji, notification.Event, notification.Event, notification.Timestamp.Format(time.RFC3339))

	payloadBytes, err := json.Marshal(slackPayload{Text: msg})
	if err != nil {
		return 0, fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, channelURL, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return 0, fmt.Errorf("create slack webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("dispatch slack webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("slack webhook responded with HTTP %d", resp.StatusCode)
	}

	return resp.StatusCode, nil
}
