# Phase 7 — Human Approval & Remediation

Design reference for OpsPilot's remediation proposal and human approval loop.

## Goals

- Turn RCA `recommended_actions` into a durable **remediation proposal**
- Require a human **approve/reject** decision before write tools run
- Keep a full audit trail (`approvals` + incident timeline)
- Notify Slack when proposals await decision or complete

## Flow

```text
Investigation completes RCA
  → ROOT_CAUSE_IDENTIFIED
  → auto ProposeFromRCA
  → REMEDIATION_PROPOSED → WAITING_FOR_APPROVAL
  → Slack notify (awaiting approval)

Human POST .../approve
  → approvals row (approve)
  → execute tool actions (create_github_issue + send_slack_notification)
  → REMEDIATION_EXECUTED → RESOLVED

Human POST .../reject
  → approvals row (reject)
  → CANCELLED
```

## Schema

Migration `000006_approvals`:

- `remediation_proposals` — summary, actions JSONB, risk_level, status
- `approvals` — decision, actor, comment

Proposal statuses: `proposed | approved | rejected | executed | failed`.

## HTTP API

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/remediations/awaiting` | Approval queue |
| `GET` | `/api/v1/incidents/{id}/remediations` | List proposals |
| `POST` | `/api/v1/incidents/{id}/remediations/propose` | Manual propose |
| `POST` | `/api/v1/incidents/{id}/remediations/{proposalId}/approve` | Approve + execute |
| `POST` | `/api/v1/incidents/{id}/remediations/{proposalId}/reject` | Reject + cancel |

### Approve body

```json
{ "actor": "oncall@example.com", "comment": "Looks good" }
```

## Packages

| Path | Role |
|------|------|
| `internal/domain/remediation` | Models + repository port |
| `internal/application/remediation` | Propose / approve / reject / execute |
| `internal/infrastructure/postgres/remediation_repository.go` | Persistence |
| `internal/transport/http/remediation_handler.go` | HTTP |

## Execution policy

- On approve, only actions with a `tool` field are executed.
- Default RCA-derived actions include `create_github_issue` (REQUIRES_APPROVAL) and `send_slack_notification` (AUTONOMOUS).
- Tool executor still requires `Approved=true` for GitHub writes; remediation service sets that after an approval record exists.
- Real GitHub writes still need `GITHUB_WRITE_ENABLED=true` when `GITHUB_PROVIDER=github`.

## Future

- Slack interactive approve/reject buttons (Phase 8 UI / Block Kit)
- Multi-approver / quorum
- Selective action approval (approve subset of tools)
