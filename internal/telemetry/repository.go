package telemetry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/OpsNexusHQ/opsnexus-common/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TelemetryRecord represents a database record of agent telemetry with CreatedAt field.
type TelemetryRecord struct {
	AgentID   string         `json:"agent_id"`
	Timestamp time.Time      `json:"timestamp"`
	Metrics   map[string]any `json:"metrics"`
	CreatedAt time.Time      `json:"created_at"`
}

// Repository persists agent telemetry.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates a telemetry repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Store persists a telemetry snapshot.
func (r *Repository) Store(ctx context.Context, telemetry models.AgentTelemetry) error {
	metrics, err := json.Marshal(telemetry.Metrics)
	if err != nil {
		return fmt.Errorf("marshal telemetry metrics: %w", err)
	}

	_, err = r.db.Exec(
		ctx,
		`
		INSERT INTO telemetry (agent_id, timestamp, metrics)
		VALUES ($1, $2, $3)
		`,
		telemetry.AgentID,
		telemetry.Timestamp,
		metrics,
	)
	if err != nil {
		return fmt.Errorf("insert telemetry: %w", err)
	}

	return nil
}

// AgentExists checks if an agent exists in the agents table.
func (r *Repository) AgentExists(ctx context.Context, agentID string) (bool, error) {
	const query = `SELECT EXISTS(SELECT 1 FROM agents WHERE id = $1)`
	var exists bool
	err := r.db.QueryRow(ctx, query, agentID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check agent existence: %w", err)
	}
	return exists, nil
}

// GetLatest retrieves the newest telemetry record for the agent.
func (r *Repository) GetLatest(ctx context.Context, agentID string) (TelemetryRecord, error) {
	const query = `
		SELECT agent_id, timestamp, metrics, created_at
		FROM telemetry
		WHERE agent_id = $1
		ORDER BY timestamp DESC
		LIMIT 1
	`
	var t TelemetryRecord
	var metricsBytes []byte
	err := r.db.QueryRow(ctx, query, agentID).Scan(
		&t.AgentID,
		&t.Timestamp,
		&metricsBytes,
		&t.CreatedAt,
	)
	if err != nil {
		return TelemetryRecord{}, fmt.Errorf("get latest telemetry: %w", err)
	}

	if err := json.Unmarshal(metricsBytes, &t.Metrics); err != nil {
		return TelemetryRecord{}, fmt.Errorf("unmarshal telemetry metrics: %w", err)
	}

	return t, nil
}

// GetHistory retrieves historical telemetry records with query parameter filtering.
func (r *Repository) GetHistory(ctx context.Context, agentID string, limit int, from, to time.Time) ([]TelemetryRecord, error) {
	var query = `
		SELECT agent_id, timestamp, metrics, created_at
		FROM telemetry
		WHERE agent_id = $1
	`
	var args = []any{agentID}
	placeholderIdx := 2

	if !from.IsZero() {
		query += fmt.Sprintf(" AND timestamp >= $%d", placeholderIdx)
		args = append(args, from)
		placeholderIdx++
	}
	if !to.IsZero() {
		query += fmt.Sprintf(" AND timestamp <= $%d", placeholderIdx)
		args = append(args, to)
		placeholderIdx++
	}

	query += fmt.Sprintf(" ORDER BY timestamp DESC LIMIT $%d", placeholderIdx)
	args = append(args, limit)

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	defer rows.Close()

	var history []TelemetryRecord
	for rows.Next() {
		var t TelemetryRecord
		var metricsBytes []byte
		err := rows.Scan(
			&t.AgentID,
			&t.Timestamp,
			&metricsBytes,
			&t.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan telemetry: %w", err)
		}

		if err := json.Unmarshal(metricsBytes, &t.Metrics); err != nil {
			return nil, fmt.Errorf("unmarshal telemetry metrics: %w", err)
		}
		history = append(history, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	if history == nil {
		history = []TelemetryRecord{}
	}

	return history, nil
}
