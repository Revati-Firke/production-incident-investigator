package tool

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound         = errors.New("tool execution not found")
	ErrToolNotFound     = errors.New("tool not found")
	ErrPermissionDenied = errors.New("tool permission denied")
	ErrApprovalRequired = errors.New("tool requires human approval")
	ErrInvalidInput     = errors.New("invalid tool input")
	ErrExecutionFailed  = errors.New("tool execution failed")
)

// PermissionLevel defines how a tool may be invoked.
type PermissionLevel string

const (
	PermissionReadOnly         PermissionLevel = "READ_ONLY"
	PermissionRequiresApproval PermissionLevel = "REQUIRES_APPROVAL"
	PermissionAutonomous       PermissionLevel = "AUTONOMOUS"
)

// ExecutionStatus represents tool execution lifecycle.
type ExecutionStatus string

const (
	StatusPending          ExecutionStatus = "pending"
	StatusRunning          ExecutionStatus = "running"
	StatusCompleted        ExecutionStatus = "completed"
	StatusFailed           ExecutionStatus = "failed"
	StatusAwaitingApproval ExecutionStatus = "awaiting_approval"
)

// Execution records a tool invocation for auditing.
type Execution struct {
	ID              uuid.UUID       `json:"id"`
	IncidentID      uuid.UUID       `json:"incident_id"`
	InvestigationID *uuid.UUID      `json:"investigation_id,omitempty"`
	ToolName        string          `json:"tool_name"`
	PermissionLevel PermissionLevel `json:"permission_level"`
	Status          ExecutionStatus `json:"status"`
	Agent           string          `json:"agent"`
	Input           json.RawMessage `json:"input"`
	Output          json.RawMessage `json:"output,omitempty"`
	Error           string          `json:"error,omitempty"`
	DurationMS      *int            `json:"duration_ms,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
}

// ToolInfo describes a registered tool for API listing.
type ToolInfo struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	PermissionLevel PermissionLevel `json:"permission_level"`
	InputSchema     any             `json:"input_schema"`
}

// ExecuteInput holds parameters for tool execution.
type ExecuteInput struct {
	IncidentID      uuid.UUID
	InvestigationID *uuid.UUID
	ToolName        string
	Input           json.RawMessage
	Agent           string
	Approved        bool
	Service         string
	Environment     string
}
