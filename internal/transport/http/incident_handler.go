package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	appinvestigation "github.com/Revati-Firke/production-incident-investigator/internal/application/investigation"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
)

const maxRequestBodyBytes = 1 << 20 // 1 MiB

// IncidentHandler handles incident HTTP endpoints.
type IncidentHandler struct {
	orchestrator *appinvestigation.Orchestrator
	incidents    *appincident.Service
}

// NewIncidentHandler creates a new incident handler.
func NewIncidentHandler(orchestrator *appinvestigation.Orchestrator, incidents *appincident.Service) *IncidentHandler {
	return &IncidentHandler{orchestrator: orchestrator, incidents: incidents}
}

// CreateIncidentRequest is the request body for creating an incident.
type CreateIncidentRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Service     string `json:"service"`
	Environment string `json:"environment"`
	AlertSource string `json:"alert_source"`
}

// Create handles POST /api/v1/incidents.
func (h *IncidentHandler) Create(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	var req CreateIncidentRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
		return
	}

	if err := domain.ValidateSeverity(domain.Severity(req.Severity)); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_SEVERITY", "severity must be one of: critical, high, medium, low")
		return
	}

	result, err := h.orchestrator.CreateIncident(r.Context(), domain.CreateInput{
		Title:       req.Title,
		Description: req.Description,
		Severity:    domain.Severity(req.Severity),
		Service:     req.Service,
		Environment: req.Environment,
		AlertSource: req.AlertSource,
	})
	if err != nil {
		mapDomainError(w, err)
		return
	}

	writeSuccess(w, http.StatusAccepted, result)
}

// Get handles GET /api/v1/incidents/{id}.
func (h *IncidentHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid incident ID")
		return
	}

	inc, err := h.incidents.GetByID(r.Context(), id)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, inc)
}

// List handles GET /api/v1/incidents.
func (h *IncidentHandler) List(w http.ResponseWriter, r *http.Request) {
	filter := domain.ListFilter{
		Service:     r.URL.Query().Get("service"),
		Environment: r.URL.Query().Get("environment"),
		Limit:       parseIntQuery(r.URL.Query().Get("limit"), 20, 1, 100),
		Offset:      parseIntQuery(r.URL.Query().Get("offset"), 0, 0, 0),
	}

	if statusStr := r.URL.Query().Get("status"); statusStr != "" {
		s := domain.Status(statusStr)
		if err := domain.ValidateStatus(s); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_STATUS", "Invalid incident status filter")
			return
		}
		filter.Status = &s
	}

	incidents, total, err := h.incidents.List(r.Context(), filter)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, PaginatedData{
		Items:  incidents,
		Total:  total,
		Limit:  filter.Limit,
		Offset: filter.Offset,
	})
}

// Timeline handles GET /api/v1/incidents/{id}/timeline.
func (h *IncidentHandler) Timeline(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid incident ID")
		return
	}

	events, err := h.incidents.GetTimeline(r.Context(), id)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, events)
}

// Investigation handles GET /api/v1/incidents/{id}/investigation.
func (h *IncidentHandler) Investigation(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid incident ID")
		return
	}

	inv, err := h.orchestrator.GetInvestigation(r.Context(), id)
	if err != nil {
		mapInvestigationError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, inv)
}

// AgentRuns handles GET /api/v1/incidents/{id}/agent-runs.
func (h *IncidentHandler) AgentRuns(w http.ResponseWriter, r *http.Request) {
	id, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid incident ID")
		return
	}

	runs, err := h.orchestrator.ListAgentRuns(r.Context(), id)
	if err != nil {
		mapInvestigationError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, runs)
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

// parseIntQuery parses an integer query param, falling back to def and clamping to [min, max].
// When max is 0, no upper bound is applied.
func parseIntQuery(raw string, def, min, max int) int {
	if raw == "" {
		return clampInt(def, min, max)
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return clampInt(def, min, max)
	}
	return clampInt(v, min, max)
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if max > 0 && v > max {
		return max
	}
	return v
}

func mapDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		writeError(w, http.StatusNotFound, "INCIDENT_NOT_FOUND", "Incident not found")
	case errors.Is(err, domain.ErrInvalidInput):
		slog.Debug("invalid input", "error", err)
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", clientSafeMessage(err, "Invalid incident input"))
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "INVALID_TRANSITION", "Invalid incident status transition")
	case errors.Is(err, domain.ErrConcurrentModification):
		writeError(w, http.StatusConflict, "CONCURRENT_MODIFICATION", "Incident was modified by another process")
	default:
		slog.Error("internal error", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred")
	}
}

func clientSafeMessage(err error, fallback string) string {
	if errors.Is(err, domain.ErrInvalidInput) {
		// Return validation detail without internal wrapping prefixes.
		msg := err.Error()
		const prefix = "invalid incident input: "
		if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
			return msg[len(prefix):]
		}
	}
	return fallback
}
