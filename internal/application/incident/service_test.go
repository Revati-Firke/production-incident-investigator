package incident_test

import (
	"context"
	"testing"

	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	"github.com/google/uuid"
)

type mockRepo struct {
	incidents map[uuid.UUID]*domain.Incident
	events    []domain.IncidentEvent
}

func newMockRepo() *mockRepo {
	return &mockRepo{incidents: make(map[uuid.UUID]*domain.Incident)}
}

func (m *mockRepo) CreateWithEvent(_ context.Context, inc *domain.Incident, event *domain.IncidentEvent) error {
	m.incidents[inc.ID] = inc
	m.events = append(m.events, *event)
	return nil
}

func (m *mockRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Incident, error) {
	inc, ok := m.incidents[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return inc, nil
}

func (m *mockRepo) List(_ context.Context, _ domain.ListFilter) ([]domain.Incident, int, error) {
	items := make([]domain.Incident, 0, len(m.incidents))
	for _, inc := range m.incidents {
		items = append(items, *inc)
	}
	return items, len(items), nil
}

func (m *mockRepo) TransitionWithEvent(_ context.Context, id uuid.UUID, expectedStatus, newStatus domain.Status, event *domain.IncidentEvent) error {
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

func (m *mockRepo) ListEvents(_ context.Context, incidentID uuid.UUID) ([]domain.IncidentEvent, error) {
	events := make([]domain.IncidentEvent, 0)
	for _, e := range m.events {
		if e.IncidentID == incidentID {
			events = append(events, e)
		}
	}
	return events, nil
}

func TestService_Create(t *testing.T) {
	svc := appincident.NewService(newMockRepo())

	inc, err := svc.Create(context.Background(), domain.CreateInput{
		Title:       "API latency increased",
		Description: "P95 latency above 3s",
		Severity:    domain.SeverityCritical,
		Service:     "payment-service",
		Environment: "production",
		AlertSource: "grafana",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if inc.Status != domain.StatusReceived {
		t.Errorf("status = %s, want RECEIVED", inc.Status)
	}
	if inc.ID == uuid.Nil {
		t.Error("expected non-nil incident ID")
	}
}

func TestService_Create_Validation(t *testing.T) {
	svc := appincident.NewService(newMockRepo())

	_, err := svc.Create(context.Background(), domain.CreateInput{
		Title:       "",
		Severity:    domain.SeverityHigh,
		Service:     "svc",
		Environment: "prod",
	})
	if err == nil {
		t.Fatal("expected validation error for empty title")
	}
}

func TestService_Transition(t *testing.T) {
	repo := newMockRepo()
	svc := appincident.NewService(repo)

	inc, err := svc.Create(context.Background(), domain.CreateInput{
		Title:       "Test incident",
		Severity:    domain.SeverityHigh,
		Service:     "api",
		Environment: "staging",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	updated, err := svc.Transition(context.Background(), domain.TransitionInput{
		IncidentID: inc.ID,
		ToStatus:   domain.StatusTriaging,
		Message:    "Starting triage",
	})
	if err != nil {
		t.Fatalf("Transition() error = %v", err)
	}
	if updated.Status != domain.StatusTriaging {
		t.Errorf("status = %s, want TRIAGING", updated.Status)
	}

	_, err = svc.Transition(context.Background(), domain.TransitionInput{
		IncidentID: inc.ID,
		ToStatus:   domain.StatusResolved,
		Message:    "Invalid jump",
	})
	if err == nil {
		t.Fatal("expected invalid transition error")
	}
}

func TestService_GetTimeline(t *testing.T) {
	repo := newMockRepo()
	svc := appincident.NewService(repo)

	inc, _ := svc.Create(context.Background(), domain.CreateInput{
		Title:       "Timeline test",
		Severity:    domain.SeverityLow,
		Service:     "api",
		Environment: "dev",
	})

	events, err := svc.GetTimeline(context.Background(), inc.ID)
	if err != nil {
		t.Fatalf("GetTimeline() error = %v", err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestService_Transition_ConcurrentModification(t *testing.T) {
	repo := newMockRepo()
	svc := appincident.NewService(repo)

	inc, _ := svc.Create(context.Background(), domain.CreateInput{
		Title:       "Race test",
		Severity:    domain.SeverityHigh,
		Service:     "api",
		Environment: "prod",
	})

	// Simulate concurrent modification by changing status directly.
	repo.incidents[inc.ID].Status = domain.StatusInvestigating

	_, err := svc.Transition(context.Background(), domain.TransitionInput{
		IncidentID: inc.ID,
		ToStatus:   domain.StatusTriaging,
		Message:    "Should fail",
	})
	if err == nil {
		t.Fatal("expected concurrent modification error")
	}
}
