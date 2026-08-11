package alerting

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) ListRules(ctx context.Context) ([]AlertRule, error) {
	const query = `
		SELECT id, name, metric, operator, threshold, duration_seconds, for_duration_seconds, cooldown_seconds, severity, enabled, created_at, updated_at
		FROM alert_rules
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer rows.Close()

	var rules []AlertRule
	for rows.Next() {
		var rule AlertRule
		err := rows.Scan(
			&rule.ID,
			&rule.Name,
			&rule.Metric,
			&rule.Operator,
			&rule.Threshold,
			&rule.DurationSeconds,
			&rule.ForDurationSeconds,
			&rule.CooldownSeconds,
			&rule.Severity,
			&rule.Enabled,
			&rule.CreatedAt,
			&rule.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan alert rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if rules == nil {
		rules = []AlertRule{}
	}
	return rules, nil
}

func (r *Repository) GetRuleByID(ctx context.Context, id string) (AlertRule, error) {
	const query = `
		SELECT id, name, metric, operator, threshold, duration_seconds, for_duration_seconds, cooldown_seconds, severity, enabled, created_at, updated_at
		FROM alert_rules
		WHERE id = $1
	`
	var rule AlertRule
	err := r.db.QueryRow(ctx, query, id).Scan(
		&rule.ID,
		&rule.Name,
		&rule.Metric,
		&rule.Operator,
		&rule.Threshold,
		&rule.DurationSeconds,
		&rule.ForDurationSeconds,
		&rule.CooldownSeconds,
		&rule.Severity,
		&rule.Enabled,
		&rule.CreatedAt,
		&rule.UpdatedAt,
	)
	if err != nil {
		return AlertRule{}, fmt.Errorf("get alert rule %s: %w", id, err)
	}
	return rule, nil
}

func (r *Repository) CreateRule(ctx context.Context, rule AlertRule) (AlertRule, error) {
	if rule.Severity == "" {
		rule.Severity = "warning"
	}
	const query = `
		INSERT INTO alert_rules (id, name, metric, operator, threshold, duration_seconds, for_duration_seconds, cooldown_seconds, severity, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW(), NOW())
		RETURNING id, name, metric, operator, threshold, duration_seconds, for_duration_seconds, cooldown_seconds, severity, enabled, created_at, updated_at
	`
	var created AlertRule
	err := r.db.QueryRow(
		ctx,
		query,
		rule.ID,
		rule.Name,
		rule.Metric,
		rule.Operator,
		rule.Threshold,
		rule.DurationSeconds,
		rule.ForDurationSeconds,
		rule.CooldownSeconds,
		rule.Severity,
		rule.Enabled,
	).Scan(
		&created.ID,
		&created.Name,
		&created.Metric,
		&created.Operator,
		&created.Threshold,
		&created.DurationSeconds,
		&created.ForDurationSeconds,
		&created.CooldownSeconds,
		&created.Severity,
		&created.Enabled,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return AlertRule{}, fmt.Errorf("create alert rule: %w", err)
	}
	return created, nil
}

func (r *Repository) UpdateRule(ctx context.Context, rule AlertRule) (AlertRule, error) {
	if rule.Severity == "" {
		rule.Severity = "warning"
	}
	const query = `
		UPDATE alert_rules
		SET name = $2, metric = $3, operator = $4, threshold = $5, duration_seconds = $6, for_duration_seconds = $7, cooldown_seconds = $8, severity = $9, enabled = $10, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, metric, operator, threshold, duration_seconds, for_duration_seconds, cooldown_seconds, severity, enabled, created_at, updated_at
	`
	var updated AlertRule
	err := r.db.QueryRow(
		ctx,
		query,
		rule.ID,
		rule.Name,
		rule.Metric,
		rule.Operator,
		rule.Threshold,
		rule.DurationSeconds,
		rule.ForDurationSeconds,
		rule.CooldownSeconds,
		rule.Severity,
		rule.Enabled,
	).Scan(
		&updated.ID,
		&updated.Name,
		&updated.Metric,
		&updated.Operator,
		&updated.Threshold,
		&updated.DurationSeconds,
		&updated.ForDurationSeconds,
		&updated.CooldownSeconds,
		&updated.Severity,
		&updated.Enabled,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if err != nil {
		return AlertRule{}, fmt.Errorf("update alert rule %s: %w", rule.ID, err)
	}
	return updated, nil
}

func (r *Repository) DeleteRule(ctx context.Context, id string) error {
	const query = `DELETE FROM alert_rules WHERE id = $1`
	_, err := r.db.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("delete alert rule %s: %w", id, err)
	}
	return nil
}

func (r *Repository) ListAlerts(ctx context.Context, status, agentID string) ([]Alert, error) {
	query := `
		SELECT a.id, a.rule_id, a.agent_id, r.name, r.severity, a.status, a.started_at, a.acknowledged_at, COALESCE(a.acknowledged_by, ''), COALESCE(a.acknowledged_comment, ''), a.resolved_at, a.last_value, a.message, a.created_at, a.updated_at
		FROM alerts a
		JOIN alert_rules r ON a.rule_id = r.id
		WHERE 1=1
	`
	args := []any{}
	idx := 1

	if status != "" {
		query += fmt.Sprintf(" AND a.status = $%d", idx)
		args = append(args, status)
		idx++
	}
	if agentID != "" {
		query += fmt.Sprintf(" AND a.agent_id = $%d", idx)
		args = append(args, agentID)
		idx++
	}

	query += " ORDER BY a.updated_at DESC"

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []Alert
	for rows.Next() {
		var a Alert
		err := rows.Scan(
			&a.ID,
			&a.RuleID,
			&a.AgentID,
			&a.RuleName,
			&a.Severity,
			&a.Status,
			&a.StartedAt,
			&a.AcknowledgedAt,
			&a.AcknowledgedBy,
			&a.AcknowledgedComment,
			&a.ResolvedAt,
			&a.LastValue,
			&a.Message,
			&a.CreatedAt,
			&a.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	if alerts == nil {
		alerts = []Alert{}
	}
	return alerts, nil
}

func (r *Repository) GetAlertByID(ctx context.Context, id string) (Alert, error) {
	const query = `
		SELECT a.id, a.rule_id, a.agent_id, r.name, r.severity, a.status, a.started_at, a.acknowledged_at, COALESCE(a.acknowledged_by, ''), COALESCE(a.acknowledged_comment, ''), a.resolved_at, a.last_value, a.message, a.created_at, a.updated_at
		FROM alerts a
		JOIN alert_rules r ON a.rule_id = r.id
		WHERE a.id = $1
	`
	var a Alert
	err := r.db.QueryRow(ctx, query, id).Scan(
		&a.ID,
		&a.RuleID,
		&a.AgentID,
		&a.RuleName,
		&a.Severity,
		&a.Status,
		&a.StartedAt,
		&a.AcknowledgedAt,
		&a.AcknowledgedBy,
		&a.AcknowledgedComment,
		&a.ResolvedAt,
		&a.LastValue,
		&a.Message,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		return Alert{}, fmt.Errorf("get alert %s: %w", id, err)
	}
	return a, nil
}

func (r *Repository) GetActiveAlert(ctx context.Context, ruleID, agentID string) (*Alert, error) {
	const query = `
		SELECT a.id, a.rule_id, a.agent_id, r.name, r.severity, a.status, a.started_at, a.acknowledged_at, COALESCE(a.acknowledged_by, ''), COALESCE(a.acknowledged_comment, ''), a.resolved_at, a.last_value, a.message, a.created_at, a.updated_at
		FROM alerts a
		JOIN alert_rules r ON a.rule_id = r.id
		WHERE a.rule_id = $1 AND a.agent_id = $2 AND a.status IN ('firing', 'acknowledged')
		LIMIT 1
	`
	var a Alert
	err := r.db.QueryRow(ctx, query, ruleID, agentID).Scan(
		&a.ID,
		&a.RuleID,
		&a.AgentID,
		&a.RuleName,
		&a.Severity,
		&a.Status,
		&a.StartedAt,
		&a.AcknowledgedAt,
		&a.AcknowledgedBy,
		&a.AcknowledgedComment,
		&a.ResolvedAt,
		&a.LastValue,
		&a.Message,
		&a.CreatedAt,
		&a.UpdatedAt,
	)
	if err != nil {
		return nil, nil
	}
	return &a, nil
}

func (r *Repository) CreateAlert(ctx context.Context, alert Alert) (Alert, error) {
	const query = `
		INSERT INTO alerts (id, rule_id, agent_id, status, started_at, last_value, message, created_at, updated_at)
		VALUES ($1, $2, $3, 'firing', NOW(), $4, $5, NOW(), NOW())
		RETURNING id, rule_id, agent_id, status, started_at, last_value, message, created_at, updated_at
	`
	var created Alert
	err := r.db.QueryRow(
		ctx,
		query,
		alert.ID,
		alert.RuleID,
		alert.AgentID,
		alert.LastValue,
		alert.Message,
	).Scan(
		&created.ID,
		&created.RuleID,
		&created.AgentID,
		&created.Status,
		&created.StartedAt,
		&created.LastValue,
		&created.Message,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return Alert{}, fmt.Errorf("create alert: %w", err)
	}
	return created, nil
}

func (r *Repository) AcknowledgeAlert(ctx context.Context, alertID, user, comment string) (Alert, error) {
	const query = `
		UPDATE alerts
		SET status = 'acknowledged', acknowledged_at = NOW(), acknowledged_by = $2, acknowledged_comment = $3, updated_at = NOW()
		WHERE id = $1 AND status = 'firing'
		RETURNING id, rule_id, agent_id, status, started_at, acknowledged_at, COALESCE(acknowledged_by, ''), COALESCE(acknowledged_comment, ''), resolved_at, last_value, message, created_at, updated_at
	`
	var acked Alert
	err := r.db.QueryRow(ctx, query, alertID, user, comment).Scan(
		&acked.ID,
		&acked.RuleID,
		&acked.AgentID,
		&acked.Status,
		&acked.StartedAt,
		&acked.AcknowledgedAt,
		&acked.AcknowledgedBy,
		&acked.AcknowledgedComment,
		&acked.ResolvedAt,
		&acked.LastValue,
		&acked.Message,
		&acked.CreatedAt,
		&acked.UpdatedAt,
	)
	if err != nil {
		return Alert{}, fmt.Errorf("acknowledge alert %s: %w", alertID, err)
	}
	return acked, nil
}

func (r *Repository) ResolveAlert(ctx context.Context, alertID string, lastValue float64) (Alert, error) {
	const query = `
		UPDATE alerts
		SET status = 'resolved', resolved_at = NOW(), last_value = $2, updated_at = NOW()
		WHERE id = $1 AND status IN ('firing', 'acknowledged')
		RETURNING id, rule_id, agent_id, status, started_at, acknowledged_at, COALESCE(acknowledged_by, ''), COALESCE(acknowledged_comment, ''), resolved_at, last_value, message, created_at, updated_at
	`
	var resolved Alert
	err := r.db.QueryRow(ctx, query, alertID, lastValue).Scan(
		&resolved.ID,
		&resolved.RuleID,
		&resolved.AgentID,
		&resolved.Status,
		&resolved.StartedAt,
		&resolved.AcknowledgedAt,
		&resolved.AcknowledgedBy,
		&resolved.AcknowledgedComment,
		&resolved.ResolvedAt,
		&resolved.LastValue,
		&resolved.Message,
		&resolved.CreatedAt,
		&resolved.UpdatedAt,
	)
	if err != nil {
		return Alert{}, fmt.Errorf("resolve alert %s: %w", alertID, err)
	}
	return resolved, nil
}

func (r *Repository) AddComment(ctx context.Context, c AlertComment) (AlertComment, error) {
	const query = `
		INSERT INTO alert_comments (id, alert_id, author_id, comment, created_at)
		VALUES ($1, $2, $3, $4, NOW())
		RETURNING id, alert_id, author_id, comment, created_at
	`
	var created AlertComment
	err := r.db.QueryRow(ctx, query, c.ID, c.AlertID, c.AuthorID, c.Comment).
		Scan(&created.ID, &created.AlertID, &created.AuthorID, &created.Comment, &created.CreatedAt)
	if err != nil {
		return AlertComment{}, fmt.Errorf("add comment: %w", err)
	}
	return created, nil
}

func (r *Repository) ListComments(ctx context.Context, alertID string) ([]AlertComment, error) {
	const query = `
		SELECT id, alert_id, author_id, comment, created_at
		FROM alert_comments
		WHERE alert_id = $1
		ORDER BY created_at ASC
	`
	rows, err := r.db.Query(ctx, query, alertID)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	var comments []AlertComment
	for rows.Next() {
		var c AlertComment
		if err := rows.Scan(&c.ID, &c.AlertID, &c.AuthorID, &c.Comment, &c.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan comment: %w", err)
		}
		comments = append(comments, c)
	}
	if comments == nil {
		comments = []AlertComment{}
	}
	return comments, nil
}
