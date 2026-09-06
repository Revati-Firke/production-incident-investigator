package memory

import (
	"encoding/json"
	"sync"

	"github.com/google/uuid"

	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

// Hypothesis is a candidate root cause tracked during investigation.
type Hypothesis struct {
	Statement  string  `json:"statement"`
	Confidence float64 `json:"confidence"`
	Status     string  `json:"status"` // active, accepted, rejected
	Reason     string  `json:"reason,omitempty"`
}

// Memory holds short-term investigation context for one agent run.
type Memory struct {
	mu sync.RWMutex

	IncidentID      uuid.UUID
	InvestigationID uuid.UUID
	Title           string
	Service         string
	Environment     string
	Severity        string
	Description     string

	ToolExecutions []domaintool.Execution
	Hypotheses     []Hypothesis
	Notes          []string
	Documents      []string
}

// New creates empty investigation memory.
func New(incidentID, investigationID uuid.UUID) *Memory {
	return &Memory{
		IncidentID:      incidentID,
		InvestigationID: investigationID,
		ToolExecutions:  make([]domaintool.Execution, 0),
		Hypotheses:      make([]Hypothesis, 0),
		Notes:           make([]string, 0),
		Documents:       make([]string, 0),
	}
}

// AddToolExecution records a tool result.
func (m *Memory) AddToolExecution(exec domaintool.Execution) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ToolExecutions = append(m.ToolExecutions, exec)
}

// AddHypothesis records a hypothesis.
func (m *Memory) AddHypothesis(h Hypothesis) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Hypotheses = append(m.Hypotheses, h)
}

// RejectHypothesis marks a hypothesis rejected with a reason.
func (m *Memory) RejectHypothesis(statement, reason string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.Hypotheses {
		if m.Hypotheses[i].Statement == statement {
			m.Hypotheses[i].Status = "rejected"
			m.Hypotheses[i].Reason = reason
			return
		}
	}
	m.Hypotheses = append(m.Hypotheses, Hypothesis{
		Statement: statement,
		Status:    "rejected",
		Reason:    reason,
	})
}

// EvidenceDigest returns a text summary of collected tool evidence for prompts.
func (m *Memory) EvidenceDigest() string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type digestItem struct {
		Tool    string          `json:"tool"`
		Status  string          `json:"status"`
		Summary json.RawMessage `json:"output,omitempty"`
		Error   string          `json:"error,omitempty"`
	}

	items := make([]digestItem, 0, len(m.ToolExecutions))
	for _, e := range m.ToolExecutions {
		items = append(items, digestItem{
			Tool:    e.ToolName,
			Status:  string(e.Status),
			Summary: e.Output,
			Error:   e.Error,
		})
	}
	data, _ := json.MarshalIndent(items, "", "  ")
	return string(data)
}

// ToolNames returns tools already executed.
func (m *Memory) ToolNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	names := make([]string, 0, len(m.ToolExecutions))
	for _, e := range m.ToolExecutions {
		names = append(names, e.ToolName)
	}
	return names
}

// SnapshotRCAHints returns rejected hypotheses currently in memory.
func (m *Memory) SnapshotRCAHints() []invdomain.RejectedHypothesis {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]invdomain.RejectedHypothesis, 0)
	for _, h := range m.Hypotheses {
		if h.Status == "rejected" {
			out = append(out, invdomain.RejectedHypothesis{
				Hypothesis: h.Statement,
				Reason:     h.Reason,
			})
		}
	}
	return out
}
