package remediation

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	ErrNotFound       = errors.New("remediation proposal not found")
	ErrInvalidState   = errors.New("remediation proposal is not in a valid state for this action")
	ErrAlreadyDecided = errors.New("remediation proposal already decided")
)

// Status is the lifecycle of a remediation proposal.
type Status string

const (
	StatusProposed Status = "proposed"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
	StatusExecuted Status = "executed"
	StatusFailed   Status = "failed"
)

// RiskLevel classifies remediation risk.
type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

// Decision is an approval outcome.
type Decision string

const (
	DecisionApprove Decision = "approve"
	DecisionReject  Decision = "reject"
)

// Action is a planned remediation step (tool-backed or advisory).
type Action struct {
	Tool    string          `json:"tool,omitempty"`
	Title   string          `json:"title"`
	Details string          `json:"details,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
}

// Proposal is a remediation plan awaiting human decision.
type Proposal struct {
	ID              uuid.UUID
	IncidentID      uuid.UUID
	InvestigationID *uuid.UUID
	Summary         string
	Actions         []Action
	RiskLevel       RiskLevel
	Status          Status
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Approval records a human decision on a proposal.
type Approval struct {
	ID         uuid.UUID
	ProposalID uuid.UUID
	Decision   Decision
	Actor      string
	Comment    string
	CreatedAt  time.Time
}

// ProposeInput creates a new proposal.
type ProposeInput struct {
	IncidentID      uuid.UUID
	InvestigationID *uuid.UUID
	Summary         string
	Actions         []Action
	RiskLevel       RiskLevel
}

// DecideInput is approve/reject input.
type DecideInput struct {
	ProposalID uuid.UUID
	IncidentID uuid.UUID
	Actor      string
	Comment    string
}
