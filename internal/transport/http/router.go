package http

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	appinvestigation "github.com/Revati-Firke/production-incident-investigator/internal/application/investigation"
)

// HealthDeps provides health check dependencies.
type HealthDeps struct {
	Postgres Pinger
	Redis    Pinger
}

// Pinger can be pinged for health checks.
type Pinger interface {
	Ping(ctx context.Context) error
}

type healthChecker struct {
	deps HealthDeps
}

func (h healthChecker) PingPostgres(ctx context.Context) error {
	return h.deps.Postgres.Ping(ctx)
}

func (h healthChecker) PingRedis(ctx context.Context) error {
	return h.deps.Redis.Ping(ctx)
}

// RouterDeps holds dependencies for the HTTP router.
type RouterDeps struct {
	Log          *slog.Logger
	Incidents    *appincident.Service
	Orchestrator *appinvestigation.Orchestrator
	Health       HealthDeps
}

// NewRouter creates the HTTP router with all routes registered.
func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(securityHeadersMiddleware)
	r.Use(recoveryMiddleware)
	r.Use(loggingMiddleware(deps.Log))
	r.Use(middleware.Timeout(60 * time.Second))

	healthHandler := NewHealthHandler(healthChecker{deps: deps.Health})
	incidentHandler := NewIncidentHandler(deps.Orchestrator, deps.Incidents)

	r.Get("/api/v1/health", healthHandler.Readiness)
	r.Get("/api/v1/health/live", healthHandler.Liveness)
	r.Get("/api/v1/metrics", promhttp.Handler().ServeHTTP)

	r.Route("/api/v1/incidents", func(r chi.Router) {
		r.Post("/", incidentHandler.Create)
		r.Get("/", incidentHandler.List)
		r.Get("/{id}/timeline", incidentHandler.Timeline)
		r.Get("/{id}/investigation", incidentHandler.Investigation)
		r.Get("/{id}", incidentHandler.Get)
	})

	return r
}
