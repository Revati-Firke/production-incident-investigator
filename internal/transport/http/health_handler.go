package http

import (
	"context"
	"net/http"
	"time"
)

// HealthChecker defines dependencies for health checks.
type HealthChecker interface {
	PingPostgres(ctx context.Context) error
	PingRedis(ctx context.Context) error
}

// HealthHandler handles health check endpoints.
type HealthHandler struct {
	checker HealthChecker
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(checker HealthChecker) *HealthHandler {
	return &HealthHandler{checker: checker}
}

// HealthResponse is the health check response body.
type HealthResponse struct {
	Status   string            `json:"status"`
	Services map[string]string `json:"services"`
}

// Liveness returns a simple liveness probe.
func (h *HealthHandler) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, HealthResponse{Status: "ok"})
}

// Readiness checks downstream dependencies.
func (h *HealthHandler) Readiness(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	services := make(map[string]string)
	healthy := true

	if err := h.checker.PingPostgres(ctx); err != nil {
		services["postgres"] = "unhealthy"
		healthy = false
	} else {
		services["postgres"] = "healthy"
	}

	if err := h.checker.PingRedis(ctx); err != nil {
		services["redis"] = "unhealthy"
		healthy = false
	} else {
		services["redis"] = "healthy"
	}

	status := "ok"
	httpStatus := http.StatusOK
	if !healthy {
		status = "degraded"
		httpStatus = http.StatusServiceUnavailable
	}

	writeSuccess(w, httpStatus, HealthResponse{
		Status:   status,
		Services: services,
	})
}
