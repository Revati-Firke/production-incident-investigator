-- Phase 7: remediation proposals and human approvals

CREATE TABLE IF NOT EXISTS remediation_proposals (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id      UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    investigation_id UUID REFERENCES investigations (id) ON DELETE SET NULL,
    summary          TEXT NOT NULL DEFAULT '',
    actions          JSONB NOT NULL DEFAULT '[]'::jsonb,
    risk_level       TEXT NOT NULL DEFAULT 'medium' CHECK (risk_level IN ('low', 'medium', 'high', 'critical')),
    status           TEXT NOT NULL DEFAULT 'proposed' CHECK (status IN (
        'proposed', 'approved', 'rejected', 'executed', 'failed'
    )),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_remediation_proposals_incident_id ON remediation_proposals (incident_id);
CREATE INDEX IF NOT EXISTS idx_remediation_proposals_status ON remediation_proposals (status);
CREATE INDEX IF NOT EXISTS idx_remediation_proposals_created_at ON remediation_proposals (created_at DESC);

CREATE TABLE IF NOT EXISTS approvals (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    proposal_id UUID NOT NULL REFERENCES remediation_proposals (id) ON DELETE CASCADE,
    decision    TEXT NOT NULL CHECK (decision IN ('approve', 'reject')),
    actor       TEXT NOT NULL DEFAULT '',
    comment     TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_approvals_proposal_id ON approvals (proposal_id);
CREATE INDEX IF NOT EXISTS idx_approvals_created_at ON approvals (created_at DESC);
