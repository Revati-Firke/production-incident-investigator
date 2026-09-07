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
	apprag "github.com/Revati-Firke/production-incident-investigator/internal/application/rag"
	appremediation "github.com/Revati-Firke/production-incident-investigator/internal/application/remediation"
	apptool "github.com/Revati-Firke/production-incident-investigator/internal/application/tool"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations"
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
	Log                  *slog.Logger
	Incidents            *appincident.Service
	Orchestrator         *appinvestigation.Orchestrator
	Tools                *apptool.Service
	RAG                  *apprag.Service
	Remediations         *appremediation.Service
	Integrations         integrations.Status
	Health               HealthDeps
	CORSOrigins          []string
	APIKey               string
	RateLimiter          RateLimiter
	GrafanaWebhookSecret string
}

// NewRouter creates the HTTP router with all routes registered.
func NewRouter(deps RouterDeps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(securityHeadersMiddleware)
	r.Use(corsMiddleware(deps.CORSOrigins))
	r.Use(recoveryMiddleware)
	r.Use(loggingMiddleware(deps.Log))
	r.Use(apiKeyMiddleware(deps.APIKey))
	r.Use(middleware.Timeout(60 * time.Second))

	healthHandler := NewHealthHandler(healthChecker{deps: deps.Health})
	incidentHandler := NewIncidentHandler(deps.Orchestrator, deps.Incidents)
	toolHandler := NewToolHandler(deps.Tools)
	ragHandler := NewRAGHandler(deps.RAG)
	integrationsHandler := NewIntegrationsHandler(deps.Integrations, deps.Orchestrator)
	remediationHandler := NewRemediationHandler(deps.Remediations)

	r.Get("/api/v1/health", healthHandler.Readiness)
	r.Get("/api/v1/health/live", healthHandler.Liveness)
	r.Get("/api/v1/metrics", promhttp.Handler().ServeHTTP)

	r.Get("/api/v1/tools", toolHandler.List)
	r.Get("/api/v1/integrations", integrationsHandler.Status)
	r.With(rateLimitMiddleware(deps.RateLimiter, "webhook", 30)).
		With(grafanaWebhookAuthMiddleware(deps.GrafanaWebhookSecret)).
		Post("/api/v1/webhooks/grafana", integrationsHandler.GrafanaWebhook)

	r.Get("/api/v1/remediations/awaiting", remediationHandler.ListAwaiting)

	r.Route("/api/v1/documents", func(r chi.Router) {
		r.Post("/", ragHandler.Ingest)
		r.Get("/", ragHandler.List)
		r.Get("/{id}", ragHandler.Get)
		r.Delete("/{id}", ragHandler.Delete)
	})
	r.Post("/api/v1/rag/search", ragHandler.Search)

	r.Route("/api/v1/incidents", func(r chi.Router) {
		r.With(rateLimitMiddleware(deps.RateLimiter, "create_incident", 60)).Post("/", incidentHandler.Create)
		r.Get("/", incidentHandler.List)
		r.Get("/{id}/timeline", incidentHandler.Timeline)
		r.Get("/{id}/investigation", incidentHandler.Investigation)
		r.Get("/{id}/agent-runs", incidentHandler.AgentRuns)
		r.Get("/{id}/evidence", toolHandler.Evidence)
		r.Post("/{id}/tools/{name}/execute", toolHandler.Execute)
		r.Get("/{id}/remediations", remediationHandler.List)
		r.Post("/{id}/remediations/propose", remediationHandler.Propose)
		r.Post("/{id}/remediations/{proposalId}/approve", remediationHandler.Approve)
		r.Post("/{id}/remediations/{proposalId}/reject", remediationHandler.Reject)
		r.Get("/{id}", incidentHandler.Get)
	})

	return r
}
