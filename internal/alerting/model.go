package alerting

import "time"

type AlertRule struct {
	ID                 string    `json:"id"`
	Name               string    `json:"name"`
	Metric             string    `json:"metric"`
	Operator           string    `json:"operator"`
	Threshold          float64   `json:"threshold"`
	DurationSeconds    int       `json:"duration_seconds"`
	ForDurationSeconds int       `json:"for_duration_seconds"`
	CooldownSeconds    int       `json:"cooldown_seconds"`
	Severity           string    `json:"severity"` // "info" | "warning" | "critical"
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type Alert struct {
	ID                  string     `json:"id"`
	RuleID              string     `json:"rule_id"`
	AgentID             string     `json:"agent_id"`
	RuleName            string     `json:"rule_name,omitempty"`
	Severity            string     `json:"severity,omitempty"`
	Status              string     `json:"status"` // "firing" | "acknowledged" | "resolved"
	StartedAt           time.Time  `json:"started_at"`
	AcknowledgedAt      *time.Time `json:"acknowledged_at,omitempty"`
	AcknowledgedBy      string     `json:"acknowledged_by,omitempty"`
	AcknowledgedComment string     `json:"acknowledged_comment,omitempty"`
	ResolvedAt          *time.Time `json:"resolved_at,omitempty"`
	LastValue           float64    `json:"last_value"`
	Message             string     `json:"message"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type AlertComment struct {
	ID        string    `json:"id"`
	AlertID   string    `json:"alert_id"`
	AuthorID  string    `json:"author_id"`
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
}
