package server

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/OpsNexusHQ/opsnexus-backend/internal/agent"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/alerting"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/auth"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/config"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/events"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/health"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/middleware"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/notifications"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/observability"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/retention"
	"github.com/OpsNexusHQ/opsnexus-backend/internal/telemetry"
	"github.com/jackc/pgx/v5/pgxpool"
)

// New creates the OpsNexus HTTP server.
func New(cfg config.Config, db *pgxpool.Pool) *http.Server {
	logger := slog.Default()
	mux := http.NewServeMux()

	// 1. Initialize SSE Event Hub
	hub := events.NewHub(logger)
	go hub.Run(context.Background())

	eventsHandler := events.NewHandler(hub)

	// 2. Initialize Notification Queue & Worker Pool
	notifRepo := notifications.NewRepository(db)
	notifQueue := notifications.NewQueue(notifRepo, 4, 256, 3, 10*time.Second, logger)
	notifQueue.Start(context.Background())

	notifHandler := notifications.NewHandler(notifRepo, notifQueue)

	// 3. Initialize Retention Worker
	retentionWorker := retention.NewWorker(db, cfg.TelemetryRetentionDays, 1*time.Hour, logger)
	go retentionWorker.Start(context.Background())

	// 4. Initialize Alerting Repository and Engine
	alertRepository := alerting.NewRepository(db)
	alertEngine := alerting.NewEngine(alertRepository, hub, notifQueue, logger)

	// 5. Initialize Handlers
	agentRepository := agent.NewRepository(db)
	telemetryRepository := telemetry.NewRepository(db)

	agentHandler := agent.NewHandler(agentRepository).WithEvents(hub)
	telemetryHandler := telemetry.NewHandler(telemetryRepository).WithEvents(hub).WithAlerting(alertEngine)
	alertHandler := alerting.NewHandler(alertRepository).WithEvents(hub)

	obsService := observability.NewService(agentRepository, telemetryRepository, cfg.HealthyThreshold, cfg.StaleThreshold)
	observabilityHandler := observability.NewHandler(obsService)

	authenticator := auth.NewAuthenticator(db, cfg.APIAuthEnabled)
	tokenHandler := auth.NewTokenHandler(db)

	// 6. Rate limiter for sensitive endpoints
	limiter := middleware.NewRateLimiter(cfg.RateLimitRequestsPerMin, 1*time.Minute)

	// Base endpoints
	mux.HandleFunc("/health", health.Handler(db))
	mux.Handle("GET /api/v1/events", eventsHandler)

	// Agent & Telemetry write APIs
	mux.Handle("POST /api/v1/agents/register", limiter.Limit(http.HandlerFunc(agentHandler.Register)))
	mux.Handle("POST /api/v1/telemetry", limiter.Limit(http.HandlerFunc(telemetryHandler.Ingest)))
	mux.Handle("POST /api/v1/agents/{id}/telemetry", limiter.Limit(http.HandlerFunc(telemetryHandler.IngestForAgent)))

	// Observability & Telemetry read APIs
	mux.HandleFunc("GET /api/v1/agents", agentHandler.List)
	mux.HandleFunc("GET /api/v1/agents/{id}", agentHandler.Get)
	mux.HandleFunc("GET /api/v1/agents/{id}/telemetry/latest", telemetryHandler.GetLatest)
	mux.HandleFunc("GET /api/v1/agents/{id}/telemetry", telemetryHandler.GetHistory)
	mux.HandleFunc("GET /api/v1/agents/{id}/health", observabilityHandler.Health)
	mux.HandleFunc("GET /api/v1/agents/{id}/metrics", observabilityHandler.Latest)
	mux.HandleFunc("GET /api/v1/agents/{id}/metrics/history", observabilityHandler.History)
	mux.HandleFunc("GET /api/v1/agents/{id}/analytics", observabilityHandler.Analytics)
	mux.HandleFunc("GET /api/v1/overview", observabilityHandler.Overview)

	// Alert & Alert Rule APIs
	mux.HandleFunc("GET /api/v1/alerts", alertHandler.ListAlerts)
	mux.HandleFunc("GET /api/v1/alerts/{id}", alertHandler.GetAlert)
	mux.Handle("POST /api/v1/alerts/{id}/acknowledge", limiter.Limit(auth.RequireRole(auth.RoleOperator)(http.HandlerFunc(alertHandler.AcknowledgeAlert))))
	mux.HandleFunc("GET /api/v1/alerts/{id}/comments", alertHandler.ListComments)
	mux.Handle("POST /api/v1/alerts/{id}/comments", limiter.Limit(auth.RequireRole(auth.RoleOperator)(http.HandlerFunc(alertHandler.AddComment))))
	mux.HandleFunc("GET /api/v1/alert-rules", alertHandler.ListRules)
	mux.Handle("POST /api/v1/alert-rules", limiter.Limit(auth.RequireRole(auth.RoleOperator)(http.HandlerFunc(alertHandler.CreateRule))))
	mux.Handle("PUT /api/v1/alert-rules/{id}", limiter.Limit(auth.RequireRole(auth.RoleOperator)(http.HandlerFunc(alertHandler.UpdateRule))))
	mux.Handle("DELETE /api/v1/alert-rules/{id}", limiter.Limit(auth.RequireRole(auth.RoleOperator)(http.HandlerFunc(alertHandler.DeleteRule))))

	// Notification Channels & Delivery APIs
	mux.HandleFunc("GET /api/v1/notification-channels", notifHandler.ListChannels)
	mux.Handle("POST /api/v1/notification-channels", limiter.Limit(auth.RequireRole(auth.RoleOperator)(http.HandlerFunc(notifHandler.CreateChannel))))
	mux.Handle("DELETE /api/v1/notification-channels/{id}", limiter.Limit(auth.RequireRole(auth.RoleOperator)(http.HandlerFunc(notifHandler.DeleteChannel))))
	mux.Handle("POST /api/v1/notification-channels/{id}/test", limiter.Limit(auth.RequireRole(auth.RoleOperator)(http.HandlerFunc(notifHandler.TestChannel))))
	mux.Handle("GET /api/v1/notification-deliveries", auth.RequireRole(auth.RoleOperator)(http.HandlerFunc(notifHandler.ListDeliveries)))

	// API Token Management (Admin)
	mux.Handle("GET /api/v1/tokens", auth.RequireRole(auth.RoleAdmin)(http.HandlerFunc(tokenHandler.ListTokens)))
	mux.Handle("POST /api/v1/tokens", limiter.Limit(auth.RequireRole(auth.RoleAdmin)(http.HandlerFunc(tokenHandler.CreateToken))))
	mux.Handle("DELETE /api/v1/tokens/{id}", limiter.Limit(auth.RequireRole(auth.RoleAdmin)(http.HandlerFunc(tokenHandler.DeleteToken))))

	// 7. Apply Global Middleware (CORS, RequestID, Logging, Authentication)
	handler := middleware.CORS(cfg.CORSOrigins)(
		middleware.RequestID(
			middleware.Logging(logger)(
				authenticator.Middleware(mux),
			),
		),
	)

	return &http.Server{
		Addr:    cfg.Host + ":" + cfg.Port,
		Handler: handler,
	}
}
