package config

import (
	"testing"
)

func TestLoadUsesFallbackDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://fallback:5432/opsnexus?sslmode=disable")
	t.Setenv("OPSNEXUS_DATABASE_URL", "")

	cfg := Load()

	if cfg.DatabaseURL != "postgres://fallback:5432/opsnexus?sslmode=disable" {
		t.Fatalf("expected database URL fallback from DATABASE_URL, got %q", cfg.DatabaseURL)
	}
}
