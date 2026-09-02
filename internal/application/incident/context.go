package incident

import (
	"context"

	"github.com/google/uuid"

	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
)

// IncidentContext provides incident metadata for other packages.
type IncidentContext struct {
	repo domain.Repository
}

// NewIncidentContext creates an incident context lookup.
func NewIncidentContext(repo domain.Repository) *IncidentContext {
	return &IncidentContext{repo: repo}
}

// GetServiceEnvironment returns service and environment for an incident.
func (c *IncidentContext) GetServiceEnvironment(ctx context.Context, incidentID uuid.UUID) (string, string, error) {
	inc, err := c.repo.GetByID(ctx, incidentID)
	if err != nil {
		return "", "", err
	}
	return inc.Service, inc.Environment, nil
}
