package alerting

import (
	"testing"
)

func TestEvaluateCondition(t *testing.T) {
	tests := []struct {
		val       float64
		op        string
		threshold float64
		expected  bool
	}{
		{85.0, ">", 80.0, true},
		{75.0, ">", 80.0, false},
		{80.0, ">=", 80.0, true},
		{10.0, "<", 20.0, true},
		{1.0, "==", 1.0, true},
		{0.0, "==", 1.0, false},
	}

	for _, tt := range tests {
		res := evaluateCondition(tt.val, tt.op, tt.threshold)
		if res != tt.expected {
			t.Errorf("evaluateCondition(%f %s %f) = %v; expected %v", tt.val, tt.op, tt.threshold, res, tt.expected)
		}
	}
}

func TestExtractMetricValue(t *testing.T) {
	data := map[string]any{
		"usage_percent": 45.5,
	}

	val, ok := extractMetricValue(data, "usage_percent")
	if !ok || val != 45.5 {
		t.Errorf("expected 45.5, got %f (ok=%v)", val, ok)
	}

	_, okMissing := extractMetricValue(data, "missing")
	if okMissing {
		t.Error("expected false for missing key")
	}
}
