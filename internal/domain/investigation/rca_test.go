package investigation_test

import (
	"testing"

	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
)

func TestRootCauseAnalysis_Validate(t *testing.T) {
	valid := &invdomain.RootCauseAnalysis{
		Summary:    "DB pool exhaustion",
		RootCause:  "connection leak",
		Confidence: 0.9,
		Evidence: []invdomain.EvidenceItem{
			{ID: "E-1", Source: "logs", Finding: "timeouts"},
		},
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid RCA: %v", err)
	}

	invalid := &invdomain.RootCauseAnalysis{Summary: "x", RootCause: "y", Confidence: 1.5}
	if err := invalid.Validate(); err == nil {
		t.Fatal("expected confidence validation error")
	}
}

func TestUnmarshalRootCause(t *testing.T) {
	raw := []byte(`{
		"summary":"s",
		"root_cause":"r",
		"confidence":0.8,
		"severity":"critical",
		"evidence":[{"id":"E-1","source":"logs","finding":"x"}],
		"rejected_hypotheses":[],
		"recommended_actions":["a"],
		"reasoning_summary":"because evidence"
	}`)
	rca, err := invdomain.UnmarshalRootCause(raw)
	if err != nil {
		t.Fatalf("UnmarshalRootCause: %v", err)
	}
	if rca.Confidence != 0.8 {
		t.Errorf("confidence = %v", rca.Confidence)
	}
}
