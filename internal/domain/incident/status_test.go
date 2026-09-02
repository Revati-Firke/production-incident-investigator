package incident_test

import (
	"testing"

	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
)

func TestCanTransition_ValidPaths(t *testing.T) {
	tests := []struct {
		from    domain.Status
		to      domain.Status
		allowed bool
	}{
		{domain.StatusReceived, domain.StatusTriaging, true},
		{domain.StatusReceived, domain.StatusInvestigating, false},
		{domain.StatusTriaging, domain.StatusInvestigating, true},
		{domain.StatusInvestigating, domain.StatusRootCauseIdentified, true},
		{domain.StatusRootCauseIdentified, domain.StatusRemediationProposed, true},
		{domain.StatusRemediationProposed, domain.StatusWaitingForApproval, true},
		{domain.StatusWaitingForApproval, domain.StatusRemediationExecuted, true},
		{domain.StatusRemediationExecuted, domain.StatusResolved, true},
		{domain.StatusResolved, domain.StatusTriaging, false},
		{domain.StatusReceived, domain.StatusCancelled, true},
		{domain.StatusFailed, domain.StatusTriaging, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"_to_"+string(tt.to), func(t *testing.T) {
			got := domain.CanTransition(tt.from, tt.to)
			if got != tt.allowed {
				t.Errorf("CanTransition(%s, %s) = %v, want %v", tt.from, tt.to, got, tt.allowed)
			}
		})
	}
}

func TestValidateSeverity(t *testing.T) {
	if err := domain.ValidateSeverity(domain.SeverityCritical); err != nil {
		t.Errorf("expected critical to be valid: %v", err)
	}
	if err := domain.ValidateSeverity("unknown"); err == nil {
		t.Error("expected unknown severity to be invalid")
	}
}

func TestValidateStatus(t *testing.T) {
	if err := domain.ValidateStatus(domain.StatusReceived); err != nil {
		t.Errorf("expected RECEIVED to be valid: %v", err)
	}
	if err := domain.ValidateStatus("INVALID"); err == nil {
		t.Error("expected INVALID status to fail validation")
	}
}
