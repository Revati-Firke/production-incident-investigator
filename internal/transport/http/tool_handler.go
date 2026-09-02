package http

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	apptool "github.com/Revati-Firke/production-incident-investigator/internal/application/tool"
	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

// ToolHandler handles tool HTTP endpoints.
type ToolHandler struct {
	tools *apptool.Service
}

// NewToolHandler creates a new tool handler.
func NewToolHandler(tools *apptool.Service) *ToolHandler {
	return &ToolHandler{tools: tools}
}

// List handles GET /api/v1/tools.
func (h *ToolHandler) List(w http.ResponseWriter, _ *http.Request) {
	writeSuccess(w, http.StatusOK, h.tools.ListTools())
}

// ExecuteRequest is the body for tool execution.
type ExecuteRequest struct {
	Input    json.RawMessage `json:"input"`
	Approved bool            `json:"approved"`
	Agent    string          `json:"agent"`
}

// Execute handles POST /api/v1/incidents/{id}/tools/{name}/execute.
func (h *ToolHandler) Execute(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

	incidentID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid incident ID")
		return
	}

	toolName := chi.URLParam(r, "name")
	if toolName == "" {
		writeError(w, http.StatusBadRequest, "INVALID_TOOL", "Tool name is required")
		return
	}

	var req ExecuteRequest
	if r.ContentLength > 0 {
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "INVALID_JSON", "Invalid request body")
			return
		}
	}

	agent := req.Agent
	if agent == "" {
		agent = "api"
	}

	exec, err := h.tools.Execute(r.Context(), domaintool.ExecuteInput{
		IncidentID: incidentID,
		ToolName:   toolName,
		Input:      req.Input,
		Approved:   req.Approved,
		Agent:      agent,
	})
	if err != nil {
		mapToolError(w, err, exec)
		return
	}

	writeSuccess(w, http.StatusOK, exec)
}

// Evidence handles GET /api/v1/incidents/{id}/evidence.
func (h *ToolHandler) Evidence(w http.ResponseWriter, r *http.Request) {
	incidentID, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_ID", "Invalid incident ID")
		return
	}

	evidence, err := h.tools.ListEvidence(r.Context(), incidentID)
	if err != nil {
		mapDomainError(w, err)
		return
	}

	writeSuccess(w, http.StatusOK, evidence)
}

func mapToolError(w http.ResponseWriter, err error, exec *domaintool.Execution) {
	switch {
	case errors.Is(err, domaintool.ErrToolNotFound):
		writeError(w, http.StatusNotFound, "TOOL_NOT_FOUND", "Tool not found")
	case errors.Is(err, domaintool.ErrApprovalRequired):
		writeSuccess(w, http.StatusAccepted, exec)
	case errors.Is(err, domaintool.ErrPermissionDenied):
		writeError(w, http.StatusForbidden, "PERMISSION_DENIED", "Tool execution not permitted")
	case errors.Is(err, domaintool.ErrInvalidInput):
		writeError(w, http.StatusBadRequest, "INVALID_INPUT", "Invalid tool input")
	case errors.Is(err, domaintool.ErrExecutionFailed):
		writeSuccess(w, http.StatusOK, exec)
	default:
		slog.Error("tool execution error", "error", err)
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "An internal error occurred")
	}
}
