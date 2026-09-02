package incident

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
)

// Service implements incident business logic.
type Service struct {
	repo domain.Repository
}

// NewService creates a new incident service.
func NewService(repo domain.Repository) *Service {
	return &Service{repo: repo}
}

// PrepareCreate validates input and builds incident entities without persisting.
func (s *Service) PrepareCreate(input domain.CreateInput) (*domain.Incident, *domain.IncidentEvent, error) {
	if err := ValidateCreateInput(input); err != nil {
		return nil, nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}

	now := time.Now().UTC()
	inc := &domain.Incident{
		ID:          uuid.New(),
		Title:       strings.TrimSpace(input.Title),
		Description: strings.TrimSpace(input.Description),
		Severity:    input.Severity,
		Service:     strings.TrimSpace(input.Service),
		Environment: strings.TrimSpace(input.Environment),
		AlertSource: strings.TrimSpace(input.AlertSource),
		Status:      domain.StatusReceived,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	status := domain.StatusReceived
	event := &domain.IncidentEvent{
		ID:         uuid.New(),
		IncidentID: inc.ID,
		EventType:  "incident.created",
		ToStatus:   &status,
		Message:    "Incident received",
		CreatedAt:  now,
	}

	return inc, event, nil
}

// Create validates and persists a new incident.
func (s *Service) Create(ctx context.Context, input domain.CreateInput) (*domain.Incident, error) {
	inc, event, err := s.PrepareCreate(input)
	if err != nil {
		return nil, err
	}

	if err := s.repo.CreateWithEvent(ctx, inc, event); err != nil {
		return nil, fmt.Errorf("create incident: %w", err)
	}

	return inc, nil
}

// GetByID returns an incident by ID.
func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*domain.Incident, error) {
	return s.repo.GetByID(ctx, id)
}

// List returns incidents matching the filter.
func (s *Service) List(ctx context.Context, filter domain.ListFilter) ([]domain.Incident, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 20
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	return s.repo.List(ctx, filter)
}

// Transition updates incident status if the transition is valid.
func (s *Service) Transition(ctx context.Context, input domain.TransitionInput) (*domain.Incident, error) {
	if err := domain.ValidateStatus(input.ToStatus); err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrInvalidInput, err)
	}

	inc, err := s.repo.GetByID(ctx, input.IncidentID)
	if err != nil {
		return nil, err
	}

	if !domain.CanTransition(inc.Status, input.ToStatus) {
		return nil, fmt.Errorf("%w: cannot transition from %s to %s", domain.ErrInvalidTransition, inc.Status, input.ToStatus)
	}

	fromStatus := inc.Status
	now := time.Now().UTC()
	event := &domain.IncidentEvent{
		ID:         uuid.New(),
		IncidentID: inc.ID,
		EventType:  "incident.status_changed",
		FromStatus: &fromStatus,
		ToStatus:   &input.ToStatus,
		Message:    input.Message,
		CreatedAt:  now,
	}

	if err := s.repo.TransitionWithEvent(ctx, inc.ID, fromStatus, input.ToStatus, event); err != nil {
		return nil, err
	}

	inc.Status = input.ToStatus
	inc.UpdatedAt = now
	return inc, nil
}

// GetTimeline returns events for an incident.
func (s *Service) GetTimeline(ctx context.Context, incidentID uuid.UUID) ([]domain.IncidentEvent, error) {
	if _, err := s.repo.GetByID(ctx, incidentID); err != nil {
		return nil, err
	}
	return s.repo.ListEvents(ctx, incidentID)
}

// ValidateCreateInput validates incident creation input.
func ValidateCreateInput(input domain.CreateInput) error {
	title := strings.TrimSpace(input.Title)
	if title == "" {
		return fmt.Errorf("title is required")
	}
	if len(title) > domain.MaxTitleLength {
		return fmt.Errorf("title exceeds maximum length of %d", domain.MaxTitleLength)
	}
	if len(strings.TrimSpace(input.Description)) > domain.MaxDescriptionLength {
		return fmt.Errorf("description exceeds maximum length of %d", domain.MaxDescriptionLength)
	}
	service := strings.TrimSpace(input.Service)
	if service == "" {
		return fmt.Errorf("service is required")
	}
	if len(service) > domain.MaxServiceLength {
		return fmt.Errorf("service exceeds maximum length of %d", domain.MaxServiceLength)
	}
	environment := strings.TrimSpace(input.Environment)
	if environment == "" {
		return fmt.Errorf("environment is required")
	}
	if len(environment) > domain.MaxEnvironmentLength {
		return fmt.Errorf("environment exceeds maximum length of %d", domain.MaxEnvironmentLength)
	}
	if len(strings.TrimSpace(input.AlertSource)) > domain.MaxAlertSourceLength {
		return fmt.Errorf("alert_source exceeds maximum length of %d", domain.MaxAlertSourceLength)
	}
	if err := domain.ValidateSeverity(input.Severity); err != nil {
		return err
	}
	return nil
}
