package http

import (
	"encoding/json"
	"io"
	"net/http"

	appinvestigation "github.com/Revati-Firke/production-incident-investigator/internal/application/investigation"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations"
	"github.com/Revati-Firke/production-incident-investigator/internal/integrations/grafana"
)

// IntegrationsHandler serves integration status and inbound webhooks.
type IntegrationsHandler struct {
	status       integrations.Status
	orchestrator *appinvestigation.Orchestrator
}

// NewIntegrationsHandler creates an integrations handler.
func NewIntegrationsHandler(status integrations.Status, orchestrator *appinvestigation.Orchestrator) *IntegrationsHandler {
	return &IntegrationsHandler{status: status, orchestrator: orchestrator}
}

// Status handles GET /api/v1/integrations.
func (h *IntegrationsHandler) Status(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, h.status)
}

// GrafanaWebhook handles POST /api/v1/webhooks/grafana.
func (h *IntegrationsHandler) GrafanaWebhook(w http.ResponseWriter, r *http.Request) {
	if h.orchestrator == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Incident intake unavailable")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_BODY", "Unable to read request body")
		return
	}

	title, description, severity, service, environment, err := grafana.IncidentFromAlert(json.RawMessage(raw))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_WEBHOOK", err.Error())
		return
	}

	result, err := h.orchestrator.CreateIncident(r.Context(), domain.CreateInput{
		Title:       title,
		Description: description,
		Severity:    domain.Severity(severity),
		Service:     service,
		Environment: environment,
		AlertSource: "grafana",
	})
	if err != nil {
		mapDomainError(w, err)
		return
	}
	writeSuccess(w, http.StatusAccepted, result)
}
