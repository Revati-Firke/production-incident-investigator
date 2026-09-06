DROP TABLE IF EXISTS agent_runs;

ALTER TABLE investigations
    DROP COLUMN IF EXISTS root_cause,
    DROP COLUMN IF EXISTS confidence,
    DROP COLUMN IF EXISTS reasoning_summary;
