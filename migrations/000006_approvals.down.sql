-- Phase 7 rollback: remediation proposals and approvals

DROP INDEX IF EXISTS idx_approvals_created_at;
DROP INDEX IF EXISTS idx_approvals_proposal_id;
DROP TABLE IF EXISTS approvals;

DROP INDEX IF EXISTS idx_remediation_proposals_created_at;
DROP INDEX IF EXISTS idx_remediation_proposals_status;
DROP INDEX IF EXISTS idx_remediation_proposals_incident_id;
DROP TABLE IF EXISTS remediation_proposals;
