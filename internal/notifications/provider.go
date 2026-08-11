package notifications

import (
	"context"
	"time"
)

type NotificationEvent string

const (
	EventAlertFiring       NotificationEvent = "alert.firing"
	EventAlertAcknowledged NotificationEvent = "alert.acknowledged"
	EventAlertResolved     NotificationEvent = "alert.resolved"
)

type Notification struct {
	ID        string            `json:"id"`
	Event     NotificationEvent `json:"event"`
	Timestamp time.Time         `json:"timestamp"`
	Alert     any               `json:"alert"`
	Agent     any               `json:"agent,omitempty"`
}

type Provider interface {
	Send(ctx context.Context, channelURL, secret string, notification Notification) (int, error)
}
