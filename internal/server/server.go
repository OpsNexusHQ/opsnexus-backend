package server

import (
	"net/http"

	"github.com/OpsNexusHQ/opsnexus-backend/internal/agent"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/config"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/health"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/telemetry"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates the OpsNexus HTTP server.
func New(cfg config.Config, db *pgxpool.Pool) *http.Server {
	mux := http.NewServeMux()

	agentRepository := agent.NewRepository(db)
	agentHandler := agent.NewHandler(agentRepository)

	telemetryRepository := telemetry.NewRepository(db)
	telemetryHandler := telemetry.NewHandler(telemetryRepository)

	mux.HandleFunc("/health", health.Handler(db))
	mux.HandleFunc("POST /api/v1/agents/register", agentHandler.Register)
	mux.HandleFunc("POST /api/v1/telemetry", telemetryHandler.Ingest)

	// New Observability APIs
	mux.HandleFunc("GET /api/v1/agents", agentHandler.List)
	mux.HandleFunc("GET /api/v1/agents/{id}", agentHandler.Get)
	mux.HandleFunc("GET /api/v1/agents/{id}/telemetry/latest", telemetryHandler.GetLatest)
	mux.HandleFunc("GET /api/v1/agents/{id}/telemetry", telemetryHandler.GetHistory)
	mux.HandleFunc("POST /api/v1/agents/{id}/telemetry", telemetryHandler.IngestForAgent)

	return &http.Server{
		Addr:    cfg.Host + ":" + cfg.Port,
		Handler: mux,
	}
}
