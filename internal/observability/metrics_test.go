package observability

import (
	"testing"
	"time"

	"github.com/OpsNexusHQ/opsnexus-backend/internal/telemetry"
)

func TestAggregate(t *testing.T) {
	records := []telemetry.TelemetryRecord{{Metrics: map[string]any{"system": map[string]any{
		"cpu":       map[string]any{"usage_percent": 10.0},
		"memory":    map[string]any{"used_percent": 40.0},
		"processes": map[string]any{"running_count": 20.0},
		"network":   map[string]any{"interfaces": []any{map[string]any{"bytes_recv": 100.0, "bytes_sent": 50.0}}},
	}}}, {Timestamp: time.Now(), Metrics: map[string]any{"system": map[string]any{
		"cpu": map[string]any{"usage_percent": 30.0}, "memory": map[string]any{"used_percent": 60.0}, "processes": map[string]any{"running_count": 40.0},
	}}}}
	result := Aggregate(records)
	if result["cpu"].(map[string]any)["average"] != 20.0 {
		t.Fatalf("unexpected CPU aggregate: %#v", result["cpu"])
	}
	if result["network"].(map[string]any)["total_bytes_received"] != uint64(100) {
		t.Fatalf("unexpected network aggregate: %#v", result["network"])
	}
}
