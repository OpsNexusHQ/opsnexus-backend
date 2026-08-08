package config

import "os"

type Config struct {
	Host        string
	Port        string
	DatabaseURL string
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

	return Config{
		Host:        host,
		Port:        port,
		DatabaseURL: databaseURL,
	}
}
