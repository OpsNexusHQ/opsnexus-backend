package observability

import "github.com/OpsNexusHQ/opsnexus-backend/internal/telemetry"

// Aggregate calculates dashboard-friendly statistics from the JSON-shaped snapshots.
func Aggregate(records []telemetry.TelemetryRecord) map[string]any {
	result := map[string]any{}
	var cpu, mem, proc []float64
	var recv, sent uint64
	var disk []float64
	for _, record := range records {
		system, ok := record.Metrics["system"].(map[string]any)
		if !ok {
			continue
		}
		if value, ok := number(system["cpu"], "usage_percent"); ok {
			cpu = append(cpu, value)
		}
		if value, ok := number(system["memory"], "used_percent"); ok {
			mem = append(mem, value)
		}
		if value, ok := number(system["processes"], "running_count"); ok {
			proc = append(proc, value)
		}
		if network, ok := system["network"].(map[string]any); ok {
			if interfaces, ok := network["interfaces"].([]any); ok {
				for _, item := range interfaces {
					if iface, ok := item.(map[string]any); ok {
						recv += uint64(numberValue(iface["bytes_recv"]))
						sent += uint64(numberValue(iface["bytes_sent"]))
					}
				}
			}
		}
		if diskMetrics, ok := system["disk"].(map[string]any); ok {
			if value, ok := number(diskMetrics, "used_percent"); ok {
				disk = append(disk, value)
			}
			if partitions, ok := diskMetrics["partitions"].([]any); ok {
				for _, item := range partitions {
					if partition, ok := item.(map[string]any); ok {
						if value, ok := numberValueOK(partition["used_percent"]); ok {
							disk = append(disk, value)
						}
					}
				}
			}
		}
	}
	if len(cpu) > 0 {
		result["cpu"] = stats(cpu)
	}
	if len(mem) > 0 {
		result["memory"] = stats(mem)
	}
	if len(proc) > 0 {
		result["processes"] = stats(proc)
	}
	if len(disk) > 0 {
		result["disk"] = map[string]any{"current_utilization": disk[0]}
	}
	result["network"] = map[string]any{"total_bytes_received": recv, "total_bytes_sent": sent}
	return result
}
func number(value any, key string) (float64, bool) {
	m, ok := value.(map[string]any)
	if !ok {
		return 0, false
	}
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	return numberValue(v), true
}
func numberValue(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case uint64:
		return float64(n)
	}
	return 0
}
func numberValueOK(v any) (float64, bool) {
	switch v.(type) {
	case float64, float32, int, int64, uint64:
		return numberValue(v), true
	}
	return 0, false
}
func stats(values []float64) map[string]any {
	min, max, sum := values[0], values[0], 0.0
	for _, v := range values {
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
		sum += v
	}
	return map[string]any{"average": sum / float64(len(values)), "minimum": min, "maximum": max}
}
