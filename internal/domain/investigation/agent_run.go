package investigation

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// AgentRunStatus is the lifecycle of an agent run.
type AgentRunStatus string

const (
	AgentRunRunning      AgentRunStatus = "running"
	AgentRunCompleted    AgentRunStatus = "completed"
	AgentRunFailed       AgentRunStatus = "failed"
	AgentRunWaitingForAI AgentRunStatus = "waiting_for_ai"
)

// AgentRun records one AI investigation attempt.
type AgentRun struct {
	ID              uuid.UUID       `json:"id"`
	InvestigationID uuid.UUID       `json:"investigation_id"`
	IncidentID      uuid.UUID       `json:"incident_id"`
	Status          AgentRunStatus  `json:"status"`
	Provider        string          `json:"provider"`
	Model           string          `json:"model"`
	InputTokens     int             `json:"input_tokens"`
	OutputTokens    int             `json:"output_tokens"`
	DurationMS      *int            `json:"duration_ms,omitempty"`
	Error           string          `json:"error,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	CompletedAt     *time.Time      `json:"completed_at,omitempty"`
}
