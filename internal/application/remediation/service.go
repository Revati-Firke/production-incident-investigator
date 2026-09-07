package remediation

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"

	appincident "github.com/Revati-Firke/production-incident-investigator/internal/application/incident"
	apptool "github.com/Revati-Firke/production-incident-investigator/internal/application/tool"
	domain "github.com/Revati-Firke/production-incident-investigator/internal/domain/incident"
	invdomain "github.com/Revati-Firke/production-incident-investigator/internal/domain/investigation"
	domainrem "github.com/Revati-Firke/production-incident-investigator/internal/domain/remediation"
	domaintool "github.com/Revati-Firke/production-incident-investigator/internal/domain/tool"
)

// Service orchestrates remediation proposals and human approvals.
type Service struct {
	repo      domainrem.Repository
	incidents *appincident.Service
	tools     *apptool.Service
	publicURL string
}

// NewService creates a remediation service.
func NewService(repo domainrem.Repository, incidents *appincident.Service, tools *apptool.Service, publicURL string) *Service {
	return &Service{repo: repo, incidents: incidents, tools: tools, publicURL: strings.TrimRight(publicURL, "/")}
}

// ProposeFromRCA builds a proposal from RCA recommended actions and moves incident to waiting for approval.
func (s *Service) ProposeFromRCA(ctx context.Context, incidentID, investigationID uuid.UUID, rca *invdomain.RootCauseAnalysis) (*domainrem.Proposal, error) {
	if rca == nil {
		return nil, fmt.Errorf("rca is required")
	}
	actions := actionsFromRCA(rca)
	summary := rca.Summary
	if summary == "" {
		summary = rca.RootCause
	}
	risk := riskFromSeverity(rca.Severity)
	return s.Propose(ctx, domainrem.ProposeInput{
		IncidentID:      incidentID,
		InvestigationID: &investigationID,
		Summary:         summary,
		Actions:         actions,
		RiskLevel:       risk,
	})
}

// Propose creates a proposal and transitions incident toward WAITING_FOR_APPROVAL.
func (s *Service) Propose(ctx context.Context, input domainrem.ProposeInput) (*domainrem.Proposal, error) {
	if _, err := s.incidents.GetByID(ctx, input.IncidentID); err != nil {
		return nil, err
	}
	if len(input.Actions) == 0 {
		input.Actions = []domainrem.Action{{
			Title:   "Review RCA and decide next steps",
			Details: "No automated remediation tools selected; human follow-up required.",
		}}
	}
	if input.RiskLevel == "" {
		input.RiskLevel = domainrem.RiskMedium
	}
	now := time.Now().UTC()
	p := &domainrem.Proposal{
		ID:              uuid.New(),
		IncidentID:      input.IncidentID,
		InvestigationID: input.InvestigationID,
		Summary:         input.Summary,
		Actions:         input.Actions,
		RiskLevel:       input.RiskLevel,
		Status:          domainrem.StatusProposed,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.repo.CreateProposal(ctx, p); err != nil {
		return nil, err
	}

	// Advance lifecycle: ROOT_CAUSE_IDENTIFIED → REMEDIATION_PROPOSED → WAITING_FOR_APPROVAL
	inc, err := s.incidents.GetByID(ctx, input.IncidentID)
	if err != nil {
		return nil, err
	}
	if inc.Status == domain.StatusRootCauseIdentified {
		if _, err := s.incidents.Transition(ctx, domain.TransitionInput{
			IncidentID: input.IncidentID,
			ToStatus:   domain.StatusRemediationProposed,
			Message:    "Remediation proposal created from RCA",
		}); err != nil {
			slog.Warn("transition to REMEDIATION_PROPOSED failed", "error", err)
		}
	}
	inc, _ = s.incidents.GetByID(ctx, input.IncidentID)
	if inc != nil && (inc.Status == domain.StatusRemediationProposed || inc.Status == domain.StatusRootCauseIdentified) {
		if _, err := s.incidents.Transition(ctx, domain.TransitionInput{
			IncidentID: input.IncidentID,
			ToStatus:   domain.StatusWaitingForApproval,
			Message:    "Awaiting human approval for remediation",
		}); err != nil {
			slog.Warn("transition to WAITING_FOR_APPROVAL failed", "error", err)
		}
	}

	s.notifySlack(ctx, input.IncidentID, p, "Remediation proposal awaiting approval")
	return p, nil
}

// List returns proposals for an incident.
func (s *Service) List(ctx context.Context, incidentID uuid.UUID) ([]domainrem.Proposal, error) {
	if _, err := s.incidents.GetByID(ctx, incidentID); err != nil {
		return nil, err
	}
	return s.repo.ListByIncidentID(ctx, incidentID)
}

// ListAwaiting returns proposals still proposed.
func (s *Service) ListAwaiting(ctx context.Context, limit, offset int) ([]domainrem.Proposal, int, error) {
	return s.repo.ListAwaitingApproval(ctx, limit, offset)
}

// Get returns one proposal.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*domainrem.Proposal, error) {
	return s.repo.GetProposal(ctx, id)
}

// Approve records approval and executes remediation tools.
func (s *Service) Approve(ctx context.Context, input domainrem.DecideInput) (*domainrem.Proposal, error) {
	p, err := s.repo.GetProposal(ctx, input.ProposalID)
	if err != nil {
		return nil, err
	}
	if p.IncidentID != input.IncidentID {
		return nil, domainrem.ErrNotFound
	}
	if p.Status != domainrem.StatusProposed {
		return nil, domainrem.ErrAlreadyDecided
	}

	actor := strings.TrimSpace(input.Actor)
	if actor == "" {
		actor = "api"
	}
	approval := &domainrem.Approval{
		ID:         uuid.New(),
		ProposalID: p.ID,
		Decision:   domainrem.DecisionApprove,
		Actor:      actor,
		Comment:    input.Comment,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.repo.CreateApproval(ctx, approval); err != nil {
		return nil, err
	}

	p.Status = domainrem.StatusApproved
	p.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateProposal(ctx, p); err != nil {
		return nil, err
	}

	if err := s.executeActions(ctx, p); err != nil {
		p.Status = domainrem.StatusFailed
		_ = s.repo.UpdateProposal(ctx, p)
		_, _ = s.incidents.Transition(ctx, domain.TransitionInput{
			IncidentID: p.IncidentID,
			ToStatus:   domain.StatusFailed,
			Message:    "Remediation execution failed: " + err.Error(),
		})
		return p, fmt.Errorf("execute remediation: %w", err)
	}

	p.Status = domainrem.StatusExecuted
	p.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateProposal(ctx, p); err != nil {
		return nil, err
	}

	if _, err := s.incidents.Transition(ctx, domain.TransitionInput{
		IncidentID: p.IncidentID,
		ToStatus:   domain.StatusRemediationExecuted,
		Message:    fmt.Sprintf("Remediation approved by %s and executed", actor),
	}); err != nil {
		slog.Warn("transition to REMEDIATION_EXECUTED failed", "error", err)
	}
	if _, err := s.incidents.Transition(ctx, domain.TransitionInput{
		IncidentID: p.IncidentID,
		ToStatus:   domain.StatusResolved,
		Message:    "Incident resolved after approved remediation",
	}); err != nil {
		slog.Warn("transition to RESOLVED failed", "error", err)
	}

	s.notifySlack(ctx, p.IncidentID, p, "Remediation approved and executed")
	return p, nil
}

// Reject records rejection and cancels the incident (re-propose via Propose later).
func (s *Service) Reject(ctx context.Context, input domainrem.DecideInput) (*domainrem.Proposal, error) {
	p, err := s.repo.GetProposal(ctx, input.ProposalID)
	if err != nil {
		return nil, err
	}
	if p.IncidentID != input.IncidentID {
		return nil, domainrem.ErrNotFound
	}
	if p.Status != domainrem.StatusProposed {
		return nil, domainrem.ErrAlreadyDecided
	}
	actor := strings.TrimSpace(input.Actor)
	if actor == "" {
		actor = "api"
	}
	approval := &domainrem.Approval{
		ID:         uuid.New(),
		ProposalID: p.ID,
		Decision:   domainrem.DecisionReject,
		Actor:      actor,
		Comment:    input.Comment,
		CreatedAt:  time.Now().UTC(),
	}
	if err := s.repo.CreateApproval(ctx, approval); err != nil {
		return nil, err
	}
	p.Status = domainrem.StatusRejected
	p.UpdatedAt = time.Now().UTC()
	if err := s.repo.UpdateProposal(ctx, p); err != nil {
		return nil, err
	}

	// Prefer return to ROOT_CAUSE_IDENTIFIED for re-propose: cancel waiting path via CANCELLED is allowed from WAITING_FOR_APPROVAL.
	// Plan: reject → stay auditable; allow CANCELLED or return to ROOT_CAUSE_IDENTIFIED. We cancel waiting and leave status CANCELLED
	// only if operator intends to stop; for re-propose we transition via Cancel is harsh.
	// Use CANCELLED as documented "prefer cancel-or-repropose" — reject marks cancelled with comment; operator can create new incident or we allow re-propose from cancelled?
	// Better: from WAITING_FOR_APPROVAL, reject → stay WAITING is wrong. Transition to CANCELLED is allowed.
	// For re-propose path: after reject, transition isn't to ROOT_CAUSE_IDENTIFIED from WAITING (not in map).
	// validTransitions: WAITING_FOR_APPROVAL → REMEDIATION_EXECUTED, CANCELLED, FAILED, ESCALATED
	// So reject → CANCELLED. Manual re-propose endpoint can still create a new proposal if we also allow propose when CANCELLED...
	// Simpler: reject → CANCELLED. Document that a new incident or escalate is needed for rework.
	// Actually plan said "prefer cancel-or-repropose path". I'll reject → CANCELLED and allow Propose to work when status is CANCELLED or ROOT_CAUSE by transitioning appropriately.
	if _, err := s.incidents.Transition(ctx, domain.TransitionInput{
		IncidentID: p.IncidentID,
		ToStatus:   domain.StatusCancelled,
		Message:    fmt.Sprintf("Remediation rejected by %s", actor),
	}); err != nil {
		slog.Warn("transition to CANCELLED failed", "error", err)
	}
	s.notifySlack(ctx, p.IncidentID, p, "Remediation rejected")
	return p, nil
}

func (s *Service) executeActions(ctx context.Context, p *domainrem.Proposal) error {
	if s.tools == nil {
		return nil
	}
	for _, action := range p.Actions {
		toolName := strings.TrimSpace(action.Tool)
		if toolName == "" {
			continue
		}
		input := action.Input
		if len(input) == 0 {
			input = defaultToolInput(toolName, p)
		}
		_, err := s.tools.Execute(ctx, domaintool.ExecuteInput{
			IncidentID:      p.IncidentID,
			InvestigationID: p.InvestigationID,
			ToolName:        toolName,
			Input:           input,
			Approved:        true,
			Agent:           "remediation-service",
		})
		if err != nil {
			return fmt.Errorf("%s: %w", toolName, err)
		}
	}
	return nil
}

func (s *Service) notifySlack(ctx context.Context, incidentID uuid.UUID, p *domainrem.Proposal, headline string) {
	if s.tools == nil {
		return
	}
	link := ""
	if s.publicURL != "" {
		link = fmt.Sprintf(" %s/incidents/%s", s.publicURL, incidentID)
	}
	msg := fmt.Sprintf("%s — %s (risk=%s)%s", headline, p.Summary, p.RiskLevel, link)
	payload, _ := json.Marshal(map[string]string{
		"message": msg,
		"channel": "#incidents",
	})
	_, err := s.tools.Execute(ctx, domaintool.ExecuteInput{
		IncidentID:      incidentID,
		InvestigationID: p.InvestigationID,
		ToolName:        "send_slack_notification",
		Input:           payload,
		Agent:           "remediation-service",
	})
	if err != nil {
		slog.Warn("slack notify failed", "error", err)
	}
}

func actionsFromRCA(rca *invdomain.RootCauseAnalysis) []domainrem.Action {
	actions := make([]domainrem.Action, 0, len(rca.RecommendedActions)+2)
	for _, a := range rca.RecommendedActions {
		actions = append(actions, domainrem.Action{
			Title:   a,
			Details: a,
		})
	}
	// Always include autonomous Slack status + approval-gated GitHub issue.
	title := "Incident follow-up: " + rca.RootCause
	if len(title) > 120 {
		title = title[:117] + "..."
	}
	body := fmt.Sprintf("Summary: %s\n\nRoot cause: %s\n\nRecommended actions:\n- %s",
		rca.Summary, rca.RootCause, strings.Join(rca.RecommendedActions, "\n- "))
	issueInput, _ := json.Marshal(map[string]string{"title": title, "body": body})
	actions = append(actions, domainrem.Action{
		Tool:    "create_github_issue",
		Title:   "Create GitHub follow-up issue",
		Details: title,
		Input:   issueInput,
	})
	slackInput, _ := json.Marshal(map[string]string{
		"message": "Remediation approved for: " + rca.Summary,
		"channel": "#incidents",
	})
	actions = append(actions, domainrem.Action{
		Tool:    "send_slack_notification",
		Title:   "Notify Slack of remediation progress",
		Details: "Post status update to #incidents",
		Input:   slackInput,
	})
	return actions
}

func riskFromSeverity(sev string) domainrem.RiskLevel {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical":
		return domainrem.RiskCritical
	case "high":
		return domainrem.RiskHigh
	case "low":
		return domainrem.RiskLow
	default:
		return domainrem.RiskMedium
	}
}

func defaultToolInput(toolName string, p *domainrem.Proposal) json.RawMessage {
	switch toolName {
	case "create_github_issue":
		b, _ := json.Marshal(map[string]string{
			"title": "OpsPilot remediation: " + p.Summary,
			"body":  p.Summary,
		})
		return b
	case "send_slack_notification":
		b, _ := json.Marshal(map[string]string{
			"message": "Remediation update: " + p.Summary,
			"channel": "#incidents",
		})
		return b
	default:
		return json.RawMessage(`{}`)
	}
}
