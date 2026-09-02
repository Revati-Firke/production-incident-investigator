-- Phase 2: investigation engine, status constraints, audit improvements

ALTER TABLE incidents
    ADD CONSTRAINT incidents_status_check CHECK (status IN (
        'RECEIVED', 'TRIAGING', 'INVESTIGATING', 'ROOT_CAUSE_IDENTIFIED',
        'REMEDIATION_PROPOSED', 'WAITING_FOR_APPROVAL', 'REMEDIATION_EXECUTED',
        'RESOLVED', 'FAILED', 'CANCELLED', 'ESCALATED'
    ));

CREATE TABLE investigations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL UNIQUE REFERENCES incidents (id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending', 'running', 'completed', 'failed', 'waiting_for_ai'
    )),
    started_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    error       TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_investigations_status ON investigations (status);
CREATE INDEX idx_investigations_incident_id ON investigations (incident_id);

CREATE TABLE investigation_jobs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    investigation_id UUID NOT NULL REFERENCES investigations (id) ON DELETE CASCADE,
    status      TEXT NOT NULL DEFAULT 'pending' CHECK (status IN (
        'pending', 'processing', 'completed', 'failed', 'dead_letter'
    )),
    attempts    INT NOT NULL DEFAULT 0,
    max_attempts INT NOT NULL DEFAULT 3,
    error       TEXT,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at  TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_investigation_jobs_status_scheduled
    ON investigation_jobs (status, scheduled_at)
    WHERE status = 'pending';

CREATE INDEX idx_investigation_jobs_incident_id ON investigation_jobs (incident_id);
