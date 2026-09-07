package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	domainrem "github.com/Revati-Firke/production-incident-investigator/internal/domain/remediation"
)

// RemediationRepository persists remediation proposals and approvals.
type RemediationRepository struct {
	pool *Pool
}

// NewRemediationRepository creates a remediation repository.
func NewRemediationRepository(pool *Pool) *RemediationRepository {
	return &RemediationRepository{pool: pool}
}

func (r *RemediationRepository) CreateProposal(ctx context.Context, p *domainrem.Proposal) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	now := time.Now().UTC()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = now
	}
	if p.Status == "" {
		p.Status = domainrem.StatusProposed
	}
	if p.RiskLevel == "" {
		p.RiskLevel = domainrem.RiskMedium
	}
	actions, err := json.Marshal(p.Actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO remediation_proposals (
			id, incident_id, investigation_id, summary, actions, risk_level, status, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		p.ID, p.IncidentID, p.InvestigationID, p.Summary, actions, string(p.RiskLevel), string(p.Status), p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert remediation proposal: %w", err)
	}
	return nil
}

func (r *RemediationRepository) GetProposal(ctx context.Context, id uuid.UUID) (*domainrem.Proposal, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT id, incident_id, investigation_id, summary, actions, risk_level, status, created_at, updated_at
		FROM remediation_proposals WHERE id = $1`, id)
	p, err := scanProposal(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainrem.ErrNotFound
		}
		return nil, err
	}
	return p, nil
}

func (r *RemediationRepository) ListByIncidentID(ctx context.Context, incidentID uuid.UUID) ([]domainrem.Proposal, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, incident_id, investigation_id, summary, actions, risk_level, status, created_at, updated_at
		FROM remediation_proposals WHERE incident_id = $1 ORDER BY created_at DESC`, incidentID)
	if err != nil {
		return nil, fmt.Errorf("list proposals: %w", err)
	}
	defer rows.Close()
	out := make([]domainrem.Proposal, 0)
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *RemediationRepository) UpdateProposal(ctx context.Context, p *domainrem.Proposal) error {
	actions, err := json.Marshal(p.Actions)
	if err != nil {
		return err
	}
	p.UpdatedAt = time.Now().UTC()
	tag, err := r.pool.Exec(ctx, `
		UPDATE remediation_proposals
		SET summary = $1, actions = $2, risk_level = $3, status = $4, updated_at = $5
		WHERE id = $6`,
		p.Summary, actions, string(p.RiskLevel), string(p.Status), p.UpdatedAt, p.ID,
	)
	if err != nil {
		return fmt.Errorf("update proposal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domainrem.ErrNotFound
	}
	return nil
}

func (r *RemediationRepository) CreateApproval(ctx context.Context, a *domainrem.Approval) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	if a.CreatedAt.IsZero() {
		a.CreatedAt = time.Now().UTC()
	}
	_, err := r.pool.Exec(ctx, `
		INSERT INTO approvals (id, proposal_id, decision, actor, comment, created_at)
		VALUES ($1,$2,$3,$4,$5,$6)`,
		a.ID, a.ProposalID, string(a.Decision), a.Actor, a.Comment, a.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("insert approval: %w", err)
	}
	return nil
}

func (r *RemediationRepository) ListApprovals(ctx context.Context, proposalID uuid.UUID) ([]domainrem.Approval, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, proposal_id, decision, actor, comment, created_at
		FROM approvals WHERE proposal_id = $1 ORDER BY created_at ASC`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domainrem.Approval, 0)
	for rows.Next() {
		var a domainrem.Approval
		var decision string
		if err := rows.Scan(&a.ID, &a.ProposalID, &decision, &a.Actor, &a.Comment, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Decision = domainrem.Decision(decision)
		out = append(out, a)
	}
	return out, rows.Err()
}

func (r *RemediationRepository) ListAwaitingApproval(ctx context.Context, limit, offset int) ([]domainrem.Proposal, int, error) {
	if limit <= 0 {
		limit = 50
	}
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM remediation_proposals WHERE status = 'proposed'`).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.pool.Query(ctx, `
		SELECT id, incident_id, investigation_id, summary, actions, risk_level, status, created_at, updated_at
		FROM remediation_proposals WHERE status = 'proposed'
		ORDER BY created_at ASC LIMIT $1 OFFSET $2`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := make([]domainrem.Proposal, 0)
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *p)
	}
	return out, total, rows.Err()
}

func scanProposal(row pgx.Row) (*domainrem.Proposal, error) {
	var p domainrem.Proposal
	var actions []byte
	var risk, status string
	err := row.Scan(
		&p.ID, &p.IncidentID, &p.InvestigationID, &p.Summary, &actions, &risk, &status, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	p.RiskLevel = domainrem.RiskLevel(risk)
	p.Status = domainrem.Status(status)
	if len(actions) > 0 {
		_ = json.Unmarshal(actions, &p.Actions)
	}
	if p.Actions == nil {
		p.Actions = []domainrem.Action{}
	}
	return &p, nil
}
