package investigation

import (
	"context"

	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
)

// IntakeStore atomically persists incident intake and investigation job records.
type IntakeStore interface {
	Persist(
		ctx context.Context,
		inc *domain.Incident,
		event *domain.IncidentEvent,
		inv *invdomain.Investigation,
		job *invdomain.Job,
	) error
}
