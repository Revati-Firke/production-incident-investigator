package tools

import (
	"context"
	"encoding/json"
)

// Tool is the contract for agent-callable tools.
type Tool interface {
	Name() string
	Description() string
	Permission() PermissionLevel
	InputSchema() any
	Execute(ctx context.Context, input json.RawMessage, ctxData Context) (Result, error)
}

// Context carries incident-scoped data into tool execution.
type Context struct {
	IncidentID  string
	Service     string
	Environment string
}

// Result is structured tool output.
type Result struct {
	Data    any    `json:"data"`
	Summary string `json:"summary,omitempty"`
}

// PermissionLevel mirrors domain permission levels.
type PermissionLevel string

const (
	PermissionReadOnly         PermissionLevel = "READ_ONLY"
	PermissionRequiresApproval PermissionLevel = "REQUIRES_APPROVAL"
	PermissionAutonomous       PermissionLevel = "AUTONOMOUS"
)
