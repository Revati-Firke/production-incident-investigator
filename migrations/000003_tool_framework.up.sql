-- Phase 3: tool execution audit trail

CREATE TABLE tool_executions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id      UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    investigation_id UUID REFERENCES investigations (id) ON DELETE SET NULL,
    tool_name        TEXT NOT NULL,
    permission_level TEXT NOT NULL CHECK (permission_level IN ('READ_ONLY', 'REQUIRES_APPROVAL', 'AUTONOMOUS')),
    status           TEXT NOT NULL DEFAULT 'running' CHECK (status IN (
        'pending', 'running', 'completed', 'failed', 'awaiting_approval'
    )),
    agent            TEXT NOT NULL DEFAULT '',
    input            JSONB NOT NULL DEFAULT '{}',
    output           JSONB,
    error            TEXT,
    duration_ms      INT,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at     TIMESTAMPTZ
);

CREATE INDEX idx_tool_executions_incident_id ON tool_executions (incident_id);
CREATE INDEX idx_tool_executions_investigation_id ON tool_executions (investigation_id);
CREATE INDEX idx_tool_executions_tool_name ON tool_executions (tool_name);
CREATE INDEX idx_tool_executions_created_at ON tool_executions (created_at DESC);
