package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	appremediation "github.com/Revati-Firke/production-incident-investigator/internal/application/remediation"
	domainrem "github.com/Revati-Firke/production-incident-investigator/internal/domain/remediation"
)

// RemediationHandler handles remediation proposal APIs.
type RemediationHandler struct {
	svc *appremediation.Service
}

// NewRemediationHandler creates a remediation handler.
func NewRemediationHandler(svc *appremediation.Service) *RemediationHandler {
	return &RemediationHandler{svc: svc}
}

type proposeRequest struct {
	Summary   string             `json:"summary"`
	RiskLevel string             `json:"risk_level"`
	Actions   []domainrem.Action `json:"actions"`
}

type decideRequest struct {
	Actor   string `json:"actor"`
	Comment string `json:"comment"`
}

// Propose handles POST /api/v1/incidents/{id}/remediations/propose
func (h *RemediationHandler) Propose(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Remediation service unavailable")
		return
	}
	incidentID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid incident ID")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	var req proposeRequest
	if r.ContentLength > 0 {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
			return
		}
	}
	p, err := h.svc.Propose(r.Context(), domainrem.ProposeInput{
		IncidentID: incidentID,
		Summary:    req.Summary,
		Actions:    req.Actions,
		RiskLevel:  domainrem.RiskLevel(req.RiskLevel),
	})
	if err != nil {
		mapRemediationError(w, err)
		return
	}
	writeSuccess(w, http.StatusCreated, proposalResponse(p))
}

// List handles GET /api/v1/incidents/{id}/remediations
func (h *RemediationHandler) List(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Remediation service unavailable")
		return
	}
	incidentID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid incident ID")
		return
	}
	items, err := h.svc.List(r.Context(), incidentID)
	if err != nil {
		mapRemediationError(w, err)
		return
	}
	out := make([]any, 0, len(items))
	for i := range items {
		out = append(out, proposalResponse(&items[i]))
	}
	writeSuccess(w, http.StatusOK, out)
}

// ListAwaiting handles GET /api/v1/remediations/awaiting
func (h *RemediationHandler) ListAwaiting(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Remediation service unavailable")
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	items, total, err := h.svc.ListAwaiting(r.Context(), limit, offset)
	if err != nil {
		mapRemediationError(w, err)
		return
	}
	out := make([]any, 0, len(items))
	for i := range items {
		out = append(out, proposalResponse(&items[i]))
	}
	if limit <= 0 {
		limit = 50
	}
	writeSuccess(w, http.StatusOK, PaginatedData{Items: out, Total: total, Limit: limit, Offset: offset})
}

// Approve handles POST /api/v1/incidents/{id}/remediations/{proposalId}/approve
func (h *RemediationHandler) Approve(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, true)
}

// Reject handles POST /api/v1/incidents/{id}/remediations/{proposalId}/reject
func (h *RemediationHandler) Reject(w http.ResponseWriter, r *http.Request) {
	h.decide(w, r, false)
}

func (h *RemediationHandler) decide(w http.ResponseWriter, r *http.Request, approve bool) {
	if h.svc == nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Remediation service unavailable")
		return
	}
	incidentID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid incident ID")
		return
	}
	proposalID, err := parseUUID(chi.URLParam(r, "proposalId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid proposal ID")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	var req decideRequest
	if r.ContentLength > 0 {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
			return
		}
	}
	input := domainrem.DecideInput{
		ProposalID: proposalID,
		IncidentID: incidentID,
		Actor:      req.Actor,
		Comment:    req.Comment,
	}
	var p *domainrem.Proposal
	if approve {
		p, err = h.svc.Approve(r.Context(), input)
	} else {
		p, err = h.svc.Reject(r.Context(), input)
	}
	if err != nil {
		mapRemediationError(w, err)
		return
	}
	writeSuccess(w, http.StatusOK, proposalResponse(p))
}

func proposalResponse(p *domainrem.Proposal) map[string]any {
	var invID any
	if p.InvestigationID != nil {
		invID = p.InvestigationID.String()
	}
	return map[string]any{
		"id":               p.ID.String(),
		"incident_id":      p.IncidentID.String(),
		"investigation_id": invID,
		"summary":          p.Summary,
		"actions":          p.Actions,
		"risk_level":       string(p.RiskLevel),
		"status":           string(p.Status),
		"created_at":       p.CreatedAt,
		"updated_at":       p.UpdatedAt,
	}
}

func mapRemediationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, domainrem.ErrNotFound):
		writeError(w, http.StatusNotFound, "NOT_FOUND", "Remediation proposal not found")
	case errors.Is(err, domainrem.ErrAlreadyDecided), errors.Is(err, domainrem.ErrInvalidState):
		writeError(w, http.StatusConflict, "INVALID_STATE", err.Error())
	default:
		mapDomainError(w, err)
	}
}
