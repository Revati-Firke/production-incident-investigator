DROP TABLE IF EXISTS investigation_jobs;
DROP TABLE IF EXISTS investigations;
ALTER TABLE incidents DROP CONSTRAINT IF EXISTS incidents_status_check;
