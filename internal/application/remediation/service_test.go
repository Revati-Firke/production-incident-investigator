package remediation_test

import (
	"context"
	"testing"
	"time"

	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	appremediation "github.com/Revati-Firke/production-incident-investigator/internal/application/remediation"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
	domainrem "github.com/Revati-Firke/production-incident-investigator/internal/domain/remediation"
	"github.com/google/uuid"
)

type memRemRepo struct {
	proposals map[uuid.UUID]*domainrem.Proposal
	approvals []domainrem.Approval
}

func newMemRemRepo() *memRemRepo {
	return &memRemRepo{proposals: map[uuid.UUID]*domainrem.Proposal{}}
}

func (m *memRemRepo) CreateProposal(_ context.Context, p *domainrem.Proposal) error {
	cp := *p
	m.proposals[p.ID] = &cp
	return nil
}
func (m *memRemRepo) GetProposal(_ context.Context, id uuid.UUID) (*domainrem.Proposal, error) {
	p, ok := m.proposals[id]
	if !ok {
		return nil, domainrem.ErrNotFound
	}
	cp := *p
	return &cp, nil
}
func (m *memRemRepo) ListByIncidentID(_ context.Context, incidentID uuid.UUID) ([]domainrem.Proposal, error) {
	out := []domainrem.Proposal{}
	for _, p := range m.proposals {
		if p.IncidentID == incidentID {
			out = append(out, *p)
		}
	}
	return out, nil
}
func (m *memRemRepo) UpdateProposal(_ context.Context, p *domainrem.Proposal) error {
	m.proposals[p.ID] = p
	return nil
}
func (m *memRemRepo) CreateApproval(_ context.Context, a *domainrem.Approval) error {
	m.approvals = append(m.approvals, *a)
	return nil
}
func (m *memRemRepo) ListApprovals(_ context.Context, _ uuid.UUID) ([]domainrem.Approval, error) {
	return m.approvals, nil
}
func (m *memRemRepo) ListAwaitingApproval(_ context.Context, _, _ int) ([]domainrem.Proposal, int, error) {
	out := []domainrem.Proposal{}
	for _, p := range m.proposals {
		if p.Status == domainrem.StatusProposed {
			out = append(out, *p)
		}
	}
	return out, len(out), nil
}

type memIncRepo struct {
	inc *domain.Incident
	ev  []domain.IncidentEvent
}

func (m *memIncRepo) CreateWithEvent(_ context.Context, inc *domain.Incident, event *domain.IncidentEvent) error {
	m.inc = inc
	m.ev = append(m.ev, *event)
	return nil
}
func (m *memIncRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Incident, error) {
	if m.inc == nil || m.inc.ID != id {
		return nil, domain.ErrNotFound
	}
	cp := *m.inc
	return &cp, nil
}
func (m *memIncRepo) List(_ context.Context, _ domain.ListFilter) ([]domain.Incident, int, error) {
	return nil, 0, nil
}
func (m *memIncRepo) TransitionWithEvent(_ context.Context, id uuid.UUID, expected, next domain.Status, event *domain.IncidentEvent) error {
	if m.inc == nil || m.inc.ID != id {
		return domain.ErrNotFound
	}
	if m.inc.Status != expected {
		return domain.ErrConcurrentModification
	}
	m.inc.Status = next
	m.ev = append(m.ev, *event)
	return nil
}
func (m *memIncRepo) ListEvents(_ context.Context, _ uuid.UUID) ([]domain.IncidentEvent, error) {
	return m.ev, nil
}

func TestProposeFromRCA_TransitionsToWaiting(t *testing.T) {
	now := time.Now().UTC()
	incID := uuid.New()
	invID := uuid.New()
	incRepo := &memIncRepo{inc: &domain.Incident{
		ID: incID, Title: "t", Severity: domain.SeverityHigh, Service: "payment-service",
		Environment: "production", Status: domain.StatusRootCauseIdentified, CreatedAt: now, UpdatedAt: now,
	}}
	remRepo := newMemRemRepo()
	incSvc := appincident.NewService(incRepo)
	svc := appremediation.NewService(remRepo, incSvc, nil, "http://localhost:8080")

	rca := &invdomain.RootCauseAnalysis{
		Summary: "pool exhaustion", RootCause: "leak", Confidence: 0.9, Severity: "critical",
		Evidence:           []invdomain.EvidenceItem{{ID: "E1", Source: "logs", Finding: "pool"}},
		RecommendedActions: []string{"Restart pods"},
	}
	p, err := svc.ProposeFromRCA(context.Background(), incID, invID, rca)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != domainrem.StatusProposed {
		t.Fatalf("status=%s", p.Status)
	}
	if incRepo.inc.Status != domain.StatusWaitingForApproval {
		t.Fatalf("incident status=%s", incRepo.inc.Status)
	}
	if len(p.Actions) < 2 {
		t.Fatalf("expected tool-backed actions, got %d", len(p.Actions))
	}
}
