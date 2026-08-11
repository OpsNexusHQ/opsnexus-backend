package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Host                    string
	Port                    string
	DatabaseURL             string
	HealthyThreshold        time.Duration
	StaleThreshold          time.Duration
	CORSOrigins             string
	RateLimitRequestsPerMin int
	TelemetryRetentionDays  int
	APIAuthEnabled          bool
}

func Load() Config {
	host := os.Getenv("OPSNEXUS_HOST")
	if host == "" {
		host = "0.0.0.0"
	}

	port := os.Getenv("OPSNEXUS_PORT")
	if port == "" {
		port = "8080"
	}

	databaseURL := os.Getenv("OPSNEXUS_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}

	corsOrigins := os.Getenv("OPSNEXUS_CORS_ORIGINS")
	if corsOrigins == "" {
		corsOrigins = "*"
	}

	return Config{
		Host:                    host,
		Port:                    port,
		DatabaseURL:             databaseURL,
		HealthyThreshold:        loadDuration("OPSNEXUS_HEALTHY_THRESHOLD", 30*time.Second),
		StaleThreshold:          loadDuration("OPSNEXUS_STALE_THRESHOLD", 2*time.Minute),
		CORSOrigins:             corsOrigins,
		RateLimitRequestsPerMin: loadInt("OPSNEXUS_RATE_LIMIT", 120),
		TelemetryRetentionDays:  loadInt("OPSNEXUS_TELEMETRY_RETENTION", 30),
		APIAuthEnabled:          os.Getenv("OPSNEXUS_API_AUTH_ENABLED") == "true",
	}
}

func loadDuration(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fallback
	}
	return duration
}

func loadInt(name string, fallback int) int {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	v, err := strconv.Atoi(value)
	if err != nil || v < 0 {
		return fallback
	}
	return v
}
