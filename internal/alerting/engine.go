package alerting

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/OpsNexusHQ/opsnexus-backend/internal/events"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/notifications"
	"github.com/OpsNexusHQ/opsnexus-common/models"
)

type Engine struct {
	repo       *Repository
	hub        *events.Hub
	notifQueue *notifications.Queue
	logger     *slog.Logger
	mu         sync.Mutex
	sustained  map[string]time.Time
}

func NewEngine(repo *Repository, hub *events.Hub, notifQueue *notifications.Queue, logger *slog.Logger) *Engine {
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{
		repo:       repo,
		hub:        hub,
		notifQueue: notifQueue,
		logger:     logger,
		sustained:  make(map[string]time.Time),
	}
}

func (e *Engine) EvaluateTelemetry(ctx context.Context, telemetry models.AgentTelemetry) {
	rules, err := e.repo.ListRules(ctx)
	if err != nil {
		e.logger.Error("failed to load alert rules", slog.Any("error", err))
		return
	}

	system, ok := telemetry.Metrics["system"].(map[string]any)
	if !ok {
		return
	}

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		var val float64
		var valFound bool

		switch rule.Metric {
		case "cpu_usage":
			val, valFound = extractMetricValue(system["cpu"], "usage_percent")
		case "memory_usage":
			val, valFound = extractMetricValue(system["memory"], "used_percent")
		case "disk_usage":
			val, valFound = extractMetricValue(system["disk"], "used_percent")
		}

		if !valFound {
			continue
		}

		triggered := evaluateCondition(val, rule.Operator, rule.Threshold)
		e.processSustainedAndState(ctx, rule, telemetry.AgentID, val, triggered)
	}
}

func (e *Engine) EvaluateAgentOffline(ctx context.Context, agentID string, isOffline bool) {
	rules, err := e.repo.ListRules(ctx)
	if err != nil {
		return
	}

	for _, rule := range rules {
		if !rule.Enabled || rule.Metric != "agent_offline" {
			continue
		}

		var val float64
		if isOffline {
			val = 1.0
		}
		triggered := isOffline
		e.processSustainedAndState(ctx, rule, agentID, val, triggered)
	}
}

func (e *Engine) processSustainedAndState(ctx context.Context, rule AlertRule, agentID string, currentVal float64, triggered bool) {
	key := fmt.Sprintf("%s:%s", rule.ID, agentID)

	e.mu.Lock()
	if triggered {
		if _, exists := e.sustained[key]; !exists {
			e.sustained[key] = time.Now()
		}
		firstSeen := e.sustained[key]
		e.mu.Unlock()

		// Check for_duration condition
		if rule.ForDurationSeconds > 0 && time.Since(firstSeen) < time.Duration(rule.ForDurationSeconds)*time.Second {
			// Duration threshold not met yet
			return
		}

		e.processAlertState(ctx, rule, agentID, currentVal, true)
	} else {
		delete(e.sustained, key)
		e.mu.Unlock()

		e.processAlertState(ctx, rule, agentID, currentVal, false)
	}
}

func (e *Engine) processAlertState(ctx context.Context, rule AlertRule, agentID string, currentVal float64, triggered bool) {
	activeAlert, err := e.repo.GetActiveAlert(ctx, rule.ID, agentID)
	if err != nil {
		e.logger.Error("error checking active alert state", slog.Any("error", err))
		return
	}

	if triggered {
		if activeAlert == nil {
			// Create new firing alert
			newAlert := Alert{
				ID:        fmt.Sprintf("alert-%d", time.Now().UnixNano()),
				RuleID:    rule.ID,
				AgentID:   agentID,
				LastValue: currentVal,
				Message:   fmt.Sprintf("[%s] %s: %.1f %s %.1f", rule.Severity, rule.Name, currentVal, rule.Operator, rule.Threshold),
			}

			created, err := e.repo.CreateAlert(ctx, newAlert)
			if err != nil {
				e.logger.Error("failed to create firing alert", slog.Any("error", err))
				return
			}

			created.RuleName = rule.Name
			created.Severity = rule.Severity

			e.logger.Info("alert firing", slog.String("rule", rule.Name), slog.String("agent", agentID), slog.String("severity", rule.Severity))

			if e.hub != nil {
				e.hub.Publish(events.Event{
					Type:      events.EventAlertFiring,
					AgentID:   agentID,
					Timestamp: time.Now(),
					Alert:     created,
				})
			}

			if e.notifQueue != nil {
				e.notifQueue.Enqueue(ctx, notifications.EventAlertFiring, created.ID, created)
			}
		}
	} else {
		if activeAlert != nil {
			// Resolve existing firing/acknowledged alert
			resolved, err := e.repo.ResolveAlert(ctx, activeAlert.ID, currentVal)
			if err != nil {
				e.logger.Error("failed to resolve alert", slog.Any("error", err))
				return
			}

			e.logger.Info("alert resolved", slog.String("rule", rule.Name), slog.String("agent", agentID))

			if e.hub != nil {
				e.hub.Publish(events.Event{
					Type:      events.EventAlertResolved,
					AgentID:   agentID,
					Timestamp: time.Now(),
					Alert:     resolved,
				})
			}

			if e.notifQueue != nil {
				e.notifQueue.Enqueue(ctx, notifications.EventAlertResolved, resolved.ID, resolved)
			}
		}
	}
}

func extractMetricValue(data any, key string) (float64, bool) {
	m, ok := data.(map[string]any)
	if !ok {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func evaluateCondition(val float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return val > threshold
	case ">=":
		return val >= threshold
	case "<":
		return val < threshold
	case "<=":
		return val <= threshold
	case "==":
		return val == threshold
	}
	return false
}
