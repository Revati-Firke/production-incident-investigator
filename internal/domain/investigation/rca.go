package investigation

import (
	"encoding/json"
	"fmt"
)

// EvidenceItem is a cited finding from a tool or data source.
type EvidenceItem struct {
	ID      string `json:"id"`
	Source  string `json:"source"`
	Finding string `json:"finding"`
}

// RejectedHypothesis explains why an alternative cause was discarded.
type RejectedHypothesis struct {
	Hypothesis string `json:"hypothesis"`
	Reason     string `json:"reason"`
}

// RootCauseAnalysis is the structured RCA output required from the AI agent.
type RootCauseAnalysis struct {
	Summary            string               `json:"summary"`
	RootCause          string               `json:"root_cause"`
	Confidence         float64              `json:"confidence"`
	Severity           string               `json:"severity"`
	Evidence           []EvidenceItem       `json:"evidence"`
	RejectedHypotheses []RejectedHypothesis `json:"rejected_hypotheses"`
	RecommendedActions []string             `json:"recommended_actions"`
	ReasoningSummary   string               `json:"reasoning_summary"`
}

// Validate checks that RCA fields are present and confidence is in range.
func (r *RootCauseAnalysis) Validate() error {
	if r == nil {
		return fmt.Errorf("root cause analysis is nil")
	}
	if r.Summary == "" {
		return fmt.Errorf("summary is required")
	}
	if r.RootCause == "" {
		return fmt.Errorf("root_cause is required")
	}
	if r.Confidence < 0 || r.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0 and 1")
	}
	if len(r.Evidence) == 0 {
		return fmt.Errorf("at least one evidence item is required")
	}
	return nil
}

// UnmarshalRootCause parses and validates RCA JSON.
func UnmarshalRootCause(data []byte) (*RootCauseAnalysis, error) {
	var rca RootCauseAnalysis
	if err := json.Unmarshal(data, &rca); err != nil {
		return nil, fmt.Errorf("parse root cause: %w", err)
	}
	if err := rca.Validate(); err != nil {
		return nil, err
	}
	return &rca, nil
}
