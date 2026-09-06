-- Phase 4: AI agent RCA persistence and agent runs

ALTER TABLE investigations
    ADD COLUMN IF NOT EXISTS root_cause JSONB,
    ADD COLUMN IF NOT EXISTS confidence DOUBLE PRECISION,
    ADD COLUMN IF NOT EXISTS reasoning_summary TEXT;

CREATE TABLE agent_runs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    investigation_id UUID NOT NULL REFERENCES investigations (id) ON DELETE CASCADE,
    incident_id      UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    status           TEXT NOT NULL DEFAULT 'running' CHECK (status IN (
        'running', 'completed', 'failed', 'waiting_for_ai'
    )),
    provider         TEXT NOT NULL DEFAULT '',
    model            TEXT NOT NULL DEFAULT '',
    input_tokens     INT NOT NULL DEFAULT 0,
    output_tokens    INT NOT NULL DEFAULT 0,
    duration_ms      INT,
    error            TEXT,
    result           JSONB,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ
);

CREATE INDEX idx_agent_runs_investigation_id ON agent_runs (investigation_id);
CREATE INDEX idx_agent_runs_incident_id ON agent_runs (incident_id);
CREATE INDEX idx_agent_runs_created_at ON agent_runs (created_at DESC);
