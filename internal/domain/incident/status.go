package incident

import "fmt"

// Status represents the lifecycle state of an incident.
type Status string

const (
	StatusReceived            Status = "RECEIVED"
	StatusTriaging            Status = "TRIAGING"
	StatusInvestigating       Status = "INVESTIGATING"
	StatusRootCauseIdentified Status = "ROOT_CAUSE_IDENTIFIED"
	StatusRemediationProposed Status = "REMEDIATION_PROPOSED"
	StatusWaitingForApproval  Status = "WAITING_FOR_APPROVAL"
	StatusRemediationExecuted Status = "REMEDIATION_EXECUTED"
	StatusResolved            Status = "RESOLVED"
	StatusFailed              Status = "FAILED"
	StatusCancelled           Status = "CANCELLED"
	StatusEscalated           Status = "ESCALATED"
)

// Severity represents incident severity.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityHigh     Severity = "high"
	SeverityMedium   Severity = "medium"
	SeverityLow      Severity = "low"
)

var validTransitions = map[Status][]Status{
	StatusReceived:            {StatusTriaging, StatusCancelled, StatusEscalated},
	StatusTriaging:            {StatusInvestigating, StatusFailed, StatusCancelled, StatusEscalated},
	StatusInvestigating:       {StatusRootCauseIdentified, StatusFailed, StatusCancelled, StatusEscalated},
	StatusRootCauseIdentified: {StatusRemediationProposed, StatusResolved, StatusFailed, StatusEscalated},
	StatusRemediationProposed: {StatusWaitingForApproval, StatusResolved, StatusFailed, StatusCancelled},
	StatusWaitingForApproval:  {StatusRemediationExecuted, StatusCancelled, StatusFailed, StatusEscalated},
	StatusRemediationExecuted: {StatusResolved, StatusFailed},
	StatusResolved:            {},
	StatusFailed:              {StatusTriaging, StatusEscalated},
	StatusCancelled:           {},
	StatusEscalated:           {StatusInvestigating, StatusResolved, StatusFailed},
}

// CanTransition reports whether a transition from current to next is allowed.
func CanTransition(current, next Status) bool {
	allowed, ok := validTransitions[current]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == next {
			return true
		}
	}
	return false
}

// ValidateStatus checks that a status value is known.
func ValidateStatus(s Status) error {
	switch s {
	case StatusReceived, StatusTriaging, StatusInvestigating,
		StatusRootCauseIdentified, StatusRemediationProposed,
		StatusWaitingForApproval, StatusRemediationExecuted,
		StatusResolved, StatusFailed, StatusCancelled, StatusEscalated:
		return nil
	default:
		return fmt.Errorf("invalid incident status: %s", s)
	}
}

// ValidateSeverity checks that a severity value is known.
func ValidateSeverity(s Severity) error {
	switch s {
	case SeverityCritical, SeverityHigh, SeverityMedium, SeverityLow:
		return nil
	default:
		return fmt.Errorf("invalid incident severity: %s", s)
	}
}
