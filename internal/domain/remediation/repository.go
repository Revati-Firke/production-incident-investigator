package remediation

import (
	"context"

	"github.com/google/uuid"
)

// Repository persists remediation proposals and approvals.
type Repository interface {
	CreateProposal(ctx context.Context, p *Proposal) error
	GetProposal(ctx context.Context, id uuid.UUID) (*Proposal, error)
	ListByIncidentID(ctx context.Context, incidentID uuid.UUID) ([]Proposal, error)
	UpdateProposal(ctx context.Context, p *Proposal) error
	CreateApproval(ctx context.Context, a *Approval) error
	ListApprovals(ctx context.Context, proposalID uuid.UUID) ([]Approval, error)
	ListAwaitingApproval(ctx context.Context, limit, offset int) ([]Proposal, int, error)
}
