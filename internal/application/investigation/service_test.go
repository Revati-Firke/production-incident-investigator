package investigation_test

import (
	"context"
	"errors"
	"testing"
	"time"

	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	appinvestigation "github.com/Revati-Firke/production-incident-investigator/internal/application/investigation"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
	"github.com/google/uuid"
)

type mockIncidentRepo struct {
	incidents map[uuid.UUID]*domain.Incident
	events    []domain.IncidentEvent
}

func newMockIncidentRepo() *mockIncidentRepo {
	return &mockIncidentRepo{incidents: make(map[uuid.UUID]*domain.Incident)}
}

func (m *mockIncidentRepo) CreateWithEvent(_ context.Context, inc *domain.Incident, event *domain.IncidentEvent) error {
	m.incidents[inc.ID] = inc
	m.events = append(m.events, *event)
	return nil
}

func (m *mockIncidentRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Incident, error) {
	inc, ok := m.incidents[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return inc, nil
}

func (m *mockIncidentRepo) List(_ context.Context, _ domain.ListFilter) ([]domain.Incident, int, error) {
	return nil, 0, nil
}

func (m *mockIncidentRepo) TransitionWithEvent(_ context.Context, id uuid.UUID, expectedStatus, newStatus domain.Status, event *domain.IncidentEvent) error {
	inc, ok := m.incidents[id]
	if !ok {
		return domain.ErrNotFound
	}
	if inc.Status != expectedStatus {
		return domain.ErrConcurrentModification
	}
	inc.Status = newStatus
	m.events = append(m.events, *event)
	return nil
}

func (m *mockIncidentRepo) ListEvents(_ context.Context, incidentID uuid.UUID) ([]domain.IncidentEvent, error) {
	return nil, nil
}

type mockInvestigationRepo struct {
	investigations map[uuid.UUID]*invdomain.Investigation
	jobs           map[uuid.UUID]*invdomain.Job
	byIncident     map[uuid.UUID]uuid.UUID
}

func newMockInvestigationRepo() *mockInvestigationRepo {
	return &mockInvestigationRepo{
		investigations: make(map[uuid.UUID]*invdomain.Investigation),
		jobs:           make(map[uuid.UUID]*invdomain.Job),
		byIncident:     make(map[uuid.UUID]uuid.UUID),
	}
}

func (m *mockInvestigationRepo) CreateWithJob(_ context.Context, inv *invdomain.Investigation, job *invdomain.Job) error {
	m.investigations[inv.ID] = inv
	m.jobs[job.ID] = job
	m.byIncident[inv.IncidentID] = inv.ID
	return nil
}

func (m *mockInvestigationRepo) GetByIncidentID(_ context.Context, incidentID uuid.UUID) (*invdomain.Investigation, error) {
	invID, ok := m.byIncident[incidentID]
	if !ok {
		return nil, invdomain.ErrNotFound
	}
	return m.investigations[invID], nil
}

func (m *mockInvestigationRepo) UpdateInvestigation(_ context.Context, inv *invdomain.Investigation) error {
	m.investigations[inv.ID] = inv
	return nil
}

func (m *mockInvestigationRepo) ClaimNextJob(_ context.Context) (*invdomain.Job, error) {
	for _, job := range m.jobs {
		if job.Status == invdomain.JobPending {
			job.Status = invdomain.JobProcessing
			job.Attempts++
			now := time.Now().UTC()
			job.StartedAt = &now
			return job, nil
		}
	}
	return nil, nil
}

func (m *mockInvestigationRepo) CompleteJob(_ context.Context, jobID uuid.UUID) error {
	job, ok := m.jobs[jobID]
	if !ok {
		return invdomain.ErrNotFound
	}
	job.Status = invdomain.JobCompleted
	return nil
}

func (m *mockInvestigationRepo) FailJob(_ context.Context, jobID uuid.UUID, jobErr string, retry bool) error {
	job, ok := m.jobs[jobID]
	if !ok {
		return invdomain.ErrNotFound
	}
	job.Error = jobErr
	if retry {
		job.Status = invdomain.JobPending
	} else {
		job.Status = invdomain.JobDeadLetter
	}
	return nil
}

type mockIntakeStore struct {
	incRepo *mockIncidentRepo
	invRepo *mockInvestigationRepo
	err     error
}

func (m *mockIntakeStore) Persist(
	ctx context.Context,
	inc *domain.Incident,
	event *domain.IncidentEvent,
	inv *invdomain.Investigation,
	job *invdomain.Job,
) error {
	if m.err != nil {
		return m.err
	}
	if err := m.incRepo.CreateWithEvent(ctx, inc, event); err != nil {
		return err
	}
	return m.invRepo.CreateWithJob(ctx, inv, job)
}

type mockNotifier struct {
	notified []uuid.UUID
}

func (m *mockNotifier) Notify(_ context.Context, jobID uuid.UUID) error {
	m.notified = append(m.notified, jobID)
	return nil
}

func (m *mockNotifier) WaitForJob(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestProcessor_ProcessNext(t *testing.T) {
	incRepo := newMockIncidentRepo()
	invRepo := newMockInvestigationRepo()
	notifier := &mockNotifier{}

	incSvc := appincident.NewService(incRepo)
	invSvc := appinvestigation.NewService(invRepo, notifier)
	processor := appinvestigation.NewProcessor(invRepo, incSvc, notifier, nil)

	inc, err := incSvc.Create(context.Background(), domain.CreateInput{
		Title:       "Worker test",
		Severity:    domain.SeverityHigh,
		Service:     "api",
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	if _, _, err := invSvc.Enqueue(context.Background(), inc.ID); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}

	if err := processor.ProcessNext(context.Background()); err != nil {
		t.Fatalf("ProcessNext() error = %v", err)
	}

	updated, err := incSvc.GetByID(context.Background(), inc.ID)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if updated.Status != domain.StatusInvestigating {
		t.Errorf("status = %s, want INVESTIGATING", updated.Status)
	}

	inv, err := invRepo.GetByIncidentID(context.Background(), inc.ID)
	if err != nil {
		t.Fatalf("GetByIncidentID() error = %v", err)
	}
	if inv.Status != invdomain.StatusCompleted {
		t.Errorf("investigation status = %s, want completed", inv.Status)
	}
}

func TestOrchestrator_CreateIncident(t *testing.T) {
	incRepo := newMockIncidentRepo()
	invRepo := newMockInvestigationRepo()
	notifier := &mockNotifier{}
	intake := &mockIntakeStore{incRepo: incRepo, invRepo: invRepo}

	incSvc := appincident.NewService(incRepo)
	invSvc := appinvestigation.NewService(invRepo, notifier)
	orch := appinvestigation.NewOrchestrator(incSvc, invSvc, intake)

	result, err := orch.CreateIncident(context.Background(), domain.CreateInput{
		Title:       "Orchestrated incident",
		Severity:    domain.SeverityCritical,
		Service:     "payments",
		Environment: "production",
	})
	if err != nil {
		t.Fatalf("CreateIncident() error = %v", err)
	}
	if result.Investigation == nil {
		t.Fatal("expected investigation in result")
	}
	if result.JobID == uuid.Nil {
		t.Error("expected job ID")
	}
	if len(notifier.notified) != 1 {
		t.Errorf("expected 1 notification, got %d", len(notifier.notified))
	}
}

func TestOrchestrator_CreateIncident_IntakeFailure(t *testing.T) {
	incRepo := newMockIncidentRepo()
	invRepo := newMockInvestigationRepo()
	notifier := &mockNotifier{}
	intake := &mockIntakeStore{incRepo: incRepo, invRepo: invRepo, err: errors.New("database unavailable")}

	incSvc := appincident.NewService(incRepo)
	invSvc := appinvestigation.NewService(invRepo, notifier)
	orch := appinvestigation.NewOrchestrator(incSvc, invSvc, intake)

	_, err := orch.CreateIncident(context.Background(), domain.CreateInput{
		Title:       "Should not persist",
		Severity:    domain.SeverityHigh,
		Service:     "api",
		Environment: "production",
	})
	if err == nil {
		t.Fatal("expected intake failure")
	}
	if len(incRepo.incidents) != 0 {
		t.Errorf("expected no incidents after intake failure, got %d", len(incRepo.incidents))
	}
	if len(notifier.notified) != 0 {
		t.Error("expected no worker notification after intake failure")
	}
}
