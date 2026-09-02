-- Initial schema for OpsPilot Phase 1

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE incidents (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    severity    TEXT NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    service     TEXT NOT NULL,
    environment TEXT NOT NULL,
    alert_source TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'RECEIVED',
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_incidents_status ON incidents (status);
CREATE INDEX idx_incidents_service ON incidents (service);
CREATE INDEX idx_incidents_environment ON incidents (environment);
CREATE INDEX idx_incidents_created_at ON incidents (created_at DESC);

CREATE TABLE incident_events (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    incident_id UUID NOT NULL REFERENCES incidents (id) ON DELETE CASCADE,
    event_type  TEXT NOT NULL,
    from_status TEXT,
    to_status   TEXT,
    message     TEXT NOT NULL DEFAULT '',
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_incident_events_incident_id ON incident_events (incident_id);
CREATE INDEX idx_incident_events_created_at ON incident_events (created_at DESC);
