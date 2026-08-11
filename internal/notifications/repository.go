package notifications

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Channel struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"` // 'webhook', 'slack_webhook'
	URL       string    `json:"url"`
	Secret    string    `json:"secret,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Delivery struct {
	ID             string     `json:"id"`
	ChannelID      string     `json:"channel_id"`
	AlertID        string     `json:"alert_id"`
	EventType      string     `json:"event_type"`
	Status         string     `json:"status"` // 'pending', 'delivered', 'failed'
	Attempts       int        `json:"attempts"`
	ResponseStatus int        `json:"response_status"`
	ErrorMessage   string     `json:"error_message"`
	CreatedAt      time.Time  `json:"created_at"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListChannels(ctx context.Context) ([]Channel, error) {
	const query = `
		SELECT id, name, type, url, secret, enabled, created_at, updated_at
		FROM notification_channels
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list channels: %w", err)
	}
	defer rows.Close()

	var channels []Channel
	for rows.Next() {
		var c Channel
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.URL, &c.Secret, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan channel: %w", err)
		}
		channels = append(channels, c)
	}
	if channels == nil {
		channels = []Channel{}
	}
	return channels, nil
}

func (r *Repository) GetChannelByID(ctx context.Context, id string) (Channel, error) {
	const query = `
		SELECT id, name, type, url, secret, enabled, created_at, updated_at
		FROM notification_channels
		WHERE id = $1
	`
	var c Channel
	err := r.db.QueryRow(ctx, query, id).Scan(&c.ID, &c.Name, &c.Type, &c.URL, &c.Secret, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return Channel{}, fmt.Errorf("get channel %s: %w", id, err)
	}
	return c, nil
}

func (r *Repository) CreateChannel(ctx context.Context, c Channel) (Channel, error) {
	const query = `
		INSERT INTO notification_channels (id, name, type, url, secret, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())
		RETURNING id, name, type, url, secret, enabled, created_at, updated_at
	`
	var created Channel
	err := r.db.QueryRow(ctx, query, c.ID, c.Name, c.Type, c.URL, c.Secret, c.Enabled).
		Scan(&created.ID, &created.Name, &created.Type, &created.URL, &created.Secret, &created.Enabled, &created.CreatedAt, &created.UpdatedAt)
	if err != nil {
		return Channel{}, fmt.Errorf("create channel: %w", err)
	}
	return created, nil
}

func (r *Repository) DeleteChannel(ctx context.Context, id string) error {
	const query = `DELETE FROM notification_channels WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete channel %s: %w", id, err)
	}
	return nil
}

func (r *Repository) CreateDelivery(ctx context.Context, d Delivery) (Delivery, error) {
	const query = `
		INSERT INTO notification_deliveries (id, channel_id, alert_id, event_type, status, attempts, response_status, error_message, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
		RETURNING id, channel_id, alert_id, event_type, status, attempts, response_status, error_message, created_at, sent_at
	`
	var created Delivery
	err := r.db.QueryRow(ctx, query, d.ID, d.ChannelID, d.AlertID, d.EventType, d.Status, d.Attempts, d.ResponseStatus, d.ErrorMessage).
		Scan(&created.ID, &created.ChannelID, &created.AlertID, &created.EventType, &created.Status, &created.Attempts, &created.ResponseStatus, &created.ErrorMessage, &created.CreatedAt, &created.SentAt)
	if err != nil {
		return Delivery{}, fmt.Errorf("create delivery: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateDelivery(ctx context.Context, id, status string, attempts, responseStatus int, errorMsg string) error {
	const query = `
		UPDATE notification_deliveries
		SET status = $2, attempts = $3, response_status = $4, error_message = $5, sent_at = CASE WHEN $2 = 'delivered' THEN NOW() ELSE sent_at END
		WHERE id = $1
	`
	_, err := r.db.Exec(ctx, query, id, status, attempts, responseStatus, errorMsg)
	if err != nil {
		return fmt.Errorf("update delivery %s: %w", id, err)
	}
	return nil
}

func (r *Repository) ListDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	if limit <= 0 {
		limit = 50
	}
	const query = `
		SELECT id, channel_id, alert_id, event_type, status, attempts, response_status, error_message, created_at, sent_at
		FROM notification_deliveries
		ORDER BY created_at DESC
		LIMIT $1
	`
	rows, err := r.db.Query(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("list deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []Delivery
	for rows.Next() {
		var d Delivery
		if err := rows.Scan(&d.ID, &d.ChannelID, &d.AlertID, &d.EventType, &d.Status, &d.Attempts, &d.ResponseStatus, &d.ErrorMessage, &d.CreatedAt, &d.SentAt); err != nil {
			return nil, fmt.Errorf("scan delivery: %w", err)
		}
		deliveries = append(deliveries, d)
	}
	if deliveries == nil {
		deliveries = []Delivery{}
	}
	return deliveries, nil
}
