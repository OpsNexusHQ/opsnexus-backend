package retention

import (
	"strings"
	"testing"
)

func TestHourlyRollupQueryUsesTelemetryContract(t *testing.T) {
	required := []string{
		"t.metrics->'system'->'disk'->'partitions'",
		"partition->>'mountpoint' = '/'",
		"partition->>'used_percent'",
		"t.metrics->'system'->'network'->'interfaces'",
		"iface->>'bytes_recv'",
		"iface->>'bytes_sent'",
		"COALESCE(AVG(disk_usage), 0) AS disk_avg",
		"COALESCE(SUM(bytes_recv), 0)::bigint AS network_received",
		"COALESCE(SUM(bytes_sent), 0)::bigint AS network_sent",
		"disk_avg = EXCLUDED.disk_avg",
		"network_received = EXCLUDED.network_received",
		"network_sent = EXCLUDED.network_sent",
	}

	for _, want := range required {
		if !strings.Contains(hourlyRollupQuery, want) {
			t.Fatalf("hourly rollup query missing %q", want)
		}
	}
}

func TestHourlyRollupQueryUpdatesAllRollupFieldsOnConflict(t *testing.T) {
	fields := []string{
		"cpu_avg",
		"cpu_min",
		"cpu_max",
		"memory_avg",
		"memory_min",
		"memory_max",
		"disk_avg",
		"network_received",
		"network_sent",
		"sample_count",
	}

	for _, field := range fields {
		want := field + " = EXCLUDED." + field
		if !strings.Contains(hourlyRollupQuery, want) {
			t.Fatalf("ON CONFLICT clause missing %q", want)
		}
	}
}
