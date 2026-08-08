package agent

import (
	"context"
	"fmt"

	"github.com/OpsNexusHQ/opsnexus-common/models"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository provides database operations for agents.
type Repository struct {
	db *pgxpool.Pool
}

// NewRepository creates an Agent repository.
func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Register creates or updates an agent registration.
func (r *Repository) Register(ctx context.Context, registration models.AgentRegistration) (models.Agent, error) {
	const query = `
		INSERT INTO agents (
			id,
			name,
			hostname,
			os,
			arch,
			version,
			status,
			last_seen
		)
		VALUES ($1, $2, $3, $4, $5, $6, 'online', NOW())
		ON CONFLICT (id)
		DO UPDATE SET
			name = EXCLUDED.name,
			hostname = EXCLUDED.hostname,
			os = EXCLUDED.os,
			arch = EXCLUDED.arch,
			version = EXCLUDED.version,
			status = 'online',
			last_seen = NOW()
		RETURNING
			id,
			name,
			hostname,
			os,
			arch,
			version,
			status,
			last_seen,
			created_at
	`

	var agent models.Agent

	err := r.db.QueryRow(
		ctx,
		query,
		registration.ID,
		registration.Name,
		registration.Hostname,
		registration.OS,
		registration.Arch,
		registration.Version,
	).Scan(
		&agent.ID,
		&agent.Name,
		&agent.Hostname,
		&agent.OS,
		&agent.Arch,
		&agent.Version,
		&agent.Status,
		&agent.LastSeen,
		&agent.CreatedAt,
	)
	if err != nil {
		return models.Agent{}, fmt.Errorf("register agent: %w", err)
	}

	return agent, nil
}

// List retrieves all registered agents.
func (r *Repository) List(ctx context.Context) ([]models.Agent, error) {
	const query = `
		SELECT id, name, hostname, os, arch, version, status, last_seen, created_at
		FROM agents
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []models.Agent
	for rows.Next() {
		var agent models.Agent
		err := rows.Scan(
			&agent.ID,
			&agent.Name,
			&agent.Hostname,
			&agent.OS,
			&agent.Arch,
			&agent.Version,
			&agent.Status,
			&agent.LastSeen,
			&agent.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan agent: %w", err)
		}
		agents = append(agents, agent)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	// Always return an empty slice instead of nil to keep API output clean
	if agents == nil {
		agents = []models.Agent{}
	}

	return agents, nil
}

// GetByID retrieves a single agent by its ID.
func (r *Repository) GetByID(ctx context.Context, id string) (models.Agent, error) {
	const query = `
		SELECT id, name, hostname, os, arch, version, status, last_seen, created_at
		FROM agents
		WHERE id = $1
	`
	var agent models.Agent
	err := r.db.QueryRow(ctx, query, id).Scan(
		&agent.ID,
		&agent.Name,
		&agent.Hostname,
		&agent.OS,
		&agent.Arch,
		&agent.Version,
		&agent.Status,
		&agent.LastSeen,
		&agent.CreatedAt,
	)
	if err != nil {
		return models.Agent{}, fmt.Errorf("get agent by id %q: %w", id, err)
	}
	return agent, nil
}
