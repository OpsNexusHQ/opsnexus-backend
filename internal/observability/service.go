package observability

import (
	"context"
	"errors"
	"time"

	"github.com/OpsNexusHQ/opsnexus-backend/internal/telemetry"
	"github.com/OpsNexusHQ/opsnexus-common/models"
	"github.com/jackc/pgx/v5"
)

type AgentSource interface {
	List(context.Context) ([]models.Agent, error)
}
type TelemetrySource interface {
	GetLatest(context.Context, string) (telemetry.TelemetryRecord, error)
	GetHistory(context.Context, string, int, time.Time, time.Time) ([]telemetry.TelemetryRecord, error)
}

type Service struct {
	agents         AgentSource
	telemetry      TelemetrySource
	healthy, stale time.Duration
	now            func() time.Time
}

func NewService(agents AgentSource, source TelemetrySource, healthy, stale time.Duration) *Service {
	if healthy <= 0 {
		healthy = 30 * time.Second
	}
	if stale <= healthy {
		stale = 2 * time.Minute
	}
	return &Service{agents: agents, telemetry: source, healthy: healthy, stale: stale, now: time.Now}
}

type Health struct {
	AgentID              string     `json:"agent_id"`
	Status               string     `json:"status"`
	LastSeen             *time.Time `json:"last_seen,omitempty"`
	SecondsSinceLastSeen *int64     `json:"seconds_since_last_seen,omitempty"`
}

func (s *Service) Health(ctx context.Context, id string) (Health, error) {
	record, err := s.telemetry.GetLatest(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Health{AgentID: id, Status: "offline"}, nil
		}
		return Health{}, err
	}
	seconds := int64(s.now().Sub(record.Timestamp).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	status := "offline"
	age := time.Duration(seconds) * time.Second
	if age <= s.healthy {
		status = "healthy"
	} else if age < s.stale {
		status = "stale"
	}
	last := record.Timestamp
	return Health{AgentID: id, Status: status, LastSeen: &last, SecondsSinceLastSeen: &seconds}, nil
}

type MetricsResponse struct {
	AgentID   string         `json:"agent_id"`
	Timestamp time.Time      `json:"timestamp"`
	Metrics   map[string]any `json:"metrics"`
}

func recordResponse(id string, r telemetry.TelemetryRecord) MetricsResponse {
	return MetricsResponse{AgentID: id, Timestamp: r.Timestamp, Metrics: r.Metrics}
}

func (s *Service) Latest(ctx context.Context, id string) (MetricsResponse, error) {
	r, e := s.telemetry.GetLatest(ctx, id)
	if e != nil {
		return MetricsResponse{}, e
	}
	return recordResponse(id, r), nil
}

func (s *Service) History(ctx context.Context, id string, limit int, from, to time.Time) ([]MetricsResponse, error) {
	rs, e := s.telemetry.GetHistory(ctx, id, limit, from, to)
	if e != nil {
		return nil, e
	}
	out := make([]MetricsResponse, 0, len(rs))
	for _, r := range rs {
		out = append(out, recordResponse(id, r))
	}
	return out, nil
}

type TimeSeriesPoint struct {
	Timestamp        time.Time `json:"timestamp"`
	CPU              float64   `json:"cpu"`
	Memory           float64   `json:"memory"`
	Disk             float64   `json:"disk"`
	BytesReceived    uint64    `json:"bytes_received"`
	BytesSent        uint64    `json:"bytes_sent"`
	RunningProcesses int       `json:"running_processes"`
}

type AnalyticsResponse struct {
	AgentID string            `json:"agent_id"`
	Range   string            `json:"range"`
	Points  []TimeSeriesPoint `json:"points"`
}

func (s *Service) Analytics(ctx context.Context, id string, timeRange string) (AnalyticsResponse, error) {
	duration := 1 * time.Hour
	switch timeRange {
	case "15m":
		duration = 15 * time.Minute
	case "1h":
		duration = 1 * time.Hour
	case "6h":
		duration = 6 * time.Hour
	case "24h":
		duration = 24 * time.Hour
	case "7d":
		duration = 7 * 24 * time.Hour
	}

	from := s.now().Add(-duration)
	to := s.now()

	records, err := s.telemetry.GetHistory(ctx, id, 500, from, to)
	if err != nil {
		return AnalyticsResponse{}, err
	}

	points := make([]TimeSeriesPoint, 0, len(records))
	// Convert reverse chronological records to chronological points for charting
	for i := len(records) - 1; i >= 0; i-- {
		r := records[i]
		pt := TimeSeriesPoint{Timestamp: r.Timestamp}

		if system, ok := r.Metrics["system"].(map[string]any); ok {
			if cpu, ok := system["cpu"].(map[string]any); ok {
				pt.CPU = numberVal(cpu["usage_percent"])
			}
			if mem, ok := system["memory"].(map[string]any); ok {
				pt.Memory = numberVal(mem["used_percent"])
			}
			if disk, ok := system["disk"].(map[string]any); ok {
				pt.Disk = numberVal(disk["used_percent"])
			}
			if proc, ok := system["processes"].(map[string]any); ok {
				pt.RunningProcesses = int(numberVal(proc["running_count"]))
			}
			if net, ok := system["network"].(map[string]any); ok {
				if ifaces, ok := net["interfaces"].([]any); ok {
					for _, item := range ifaces {
						if iface, ok := item.(map[string]any); ok {
							pt.BytesReceived += uint64(numberVal(iface["bytes_recv"]))
							pt.BytesSent += uint64(numberVal(iface["bytes_sent"]))
						}
					}
				}
			}
		}
		points = append(points, pt)
	}

	return AnalyticsResponse{
		AgentID: id,
		Range:   timeRange,
		Points:  points,
	}, nil
}

func numberVal(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	}
	return 0
}

type Overview struct {
	Total           int        `json:"total"`
	Healthy         int        `json:"healthy"`
	Stale           int        `json:"stale"`
	Offline         int        `json:"offline"`
	LatestTelemetry *time.Time `json:"latest_telemetry,omitempty"`
}

func (s *Service) Overview(ctx context.Context) (Overview, error) {
	as, e := s.agents.List(ctx)
	if e != nil {
		return Overview{}, e
	}
	o := Overview{Total: len(as)}
	for _, a := range as {
		h, e := s.Health(ctx, a.ID)
		if e != nil {
			return Overview{}, e
		}
		switch h.Status {
		case "healthy":
			o.Healthy++
		case "stale":
			o.Stale++
		default:
			o.Offline++
		}
		if h.LastSeen != nil && (o.LatestTelemetry == nil || h.LastSeen.After(*o.LatestTelemetry)) {
			v := *h.LastSeen
			o.LatestTelemetry = &v
		}
	}
	return o, nil
}
