# OpsPilot

**AI-powered autonomous production incident response platform.**

OpsPilot receives production alerts, investigates incidents using AI agents and observability tools, determines probable root causes, and coordinates human-approved remediation — with a full audit trail.

> **Status:** Phase 4 (AI Agent) complete. LLM abstraction, investigation agent, structured RCA, and agent-run audit trail are operational.

## Project Overview

OpsPilot is designed for SRE and platform engineering teams who need:

- Autonomous incident triage and investigation
- Evidence-based root cause analysis (not chatbot responses)
- Human-in-the-loop approval for high-risk actions
- Full auditability of agent and tool executions

## Architecture

```mermaid
flowchart LR
    Alert[Grafana / API] --> API[OpsPilot API]
    API --> PG[(PostgreSQL)]
    API --> Redis[(Redis)]
    API --> Worker[Investigation Worker]
    Worker --> Agent[AI Agent]
    Agent --> Tools[Tool Framework]
    Tools --> Obs[Logs / Metrics / Health]
    Tools --> GH[GitHub]
    Agent --> RAG[RAG / Runbooks]
    Agent --> RCA[Root Cause Analysis]
    RCA --> Approval[Human Approval]
    Approval --> Action[Remediation]
```

## Phase 1 — Foundation

### Implemented

| Component | Description |
|-----------|-------------|
| Go HTTP server | Chi router, graceful shutdown, structured JSON logging |
| Configuration | Environment-based config with validation |
| PostgreSQL | Incident and incident_events tables via golang-migrate |
| Redis | Connection pool with health checks |
| Incident API | Create, list, get, timeline endpoints |
| State machine | Incident lifecycle with validated transitions |
| Health checks | `/api/v1/health`, `/api/v1/health/live` |
| Metrics | `/api/v1/metrics` (Prometheus) |
| Docker Compose | postgres, redis, migrate, opspilot-api |

## Phase 2 — Incident Engine

### Implemented

| Component | Description |
|-----------|-------------|
| Investigation model | `investigations` + `investigation_jobs` tables |
| Atomic intake | Single transaction for incident + investigation + job |
| Background worker | `cmd/worker` with `FOR UPDATE SKIP LOCKED` job claiming |
| Redis job notifications | LPUSH/BRPOP hint queue with DB polling fallback |
| Orchestrator | Coordinates incident creation and investigation enqueue |
| Investigation API | `GET /api/v1/incidents/{id}/investigation` |
| Optimistic locking | Status transitions use `WHERE status = expected` |
| Worker graceful shutdown | Waits for in-flight jobs before exit |

## Phase 3 — Tool Framework

### Implemented

| Component | Description |
|-----------|-------------|
| Tool interface | Generic `Tool` with `Execute`, `InputSchema`, permissions |
| Tool registry | Thread-safe registration and lookup |
| Tool executor | Timeouts, permission checks, audit persistence |
| Permission model | `READ_ONLY`, `REQUIRES_APPROVAL`, `AUTONOMOUS` |
| Mock tools | 13 tools across observability, DB, deployment, GitHub, knowledge, Slack |
| Worker integration | Investigation worker runs 5 read-only tools per incident |
| Tool API | `GET /api/v1/tools`, `POST /api/v1/incidents/{id}/tools/{name}/execute` |
| Evidence API | `GET /api/v1/incidents/{id}/evidence` (tool execution audit trail) |
| Persistence | `tool_executions` table with input/output/duration |

### Mock Tools

| Tool | Permission | Category |
|------|------------|----------|
| `search_logs` | READ_ONLY | Observability |
| `query_metrics` | READ_ONLY | Observability |
| `get_service_health` | READ_ONLY | Observability |
| `query_database` | READ_ONLY | Database |
| `get_database_connections` | READ_ONLY | Database |
| `get_recent_deployments` | READ_ONLY | Deployment |
| `search_github_commits` | READ_ONLY | GitHub |
| `inspect_code` | READ_ONLY | GitHub |
| `create_github_issue` | REQUIRES_APPROVAL | GitHub |
| `create_pull_request` | REQUIRES_APPROVAL | GitHub |
| `search_runbooks` | READ_ONLY | Knowledge |
| `search_previous_incidents` | READ_ONLY | Knowledge |
| `send_slack_notification` | AUTONOMOUS | Communication |

## Phase 4 — AI Agent (Current)

### Implemented

| Component | Description |
|-----------|-------------|
| LLM abstraction | Replaceable `llm.Provider` interface |
| Mock LLM | Deterministic evidence-based RCA (default, no API key) |
| OpenAI provider | OpenAI-compatible Chat Completions adapter |
| Investigation agent | Tool-calling loop + structured RCA validation |
| Short-term memory | Incident context, tool results, hypotheses |
| RCA persistence | `investigations.root_cause`, `confidence`, `reasoning_summary` |
| Agent runs | `agent_runs` audit table with tokens/duration/result |
| Worker integration | Tools → agent → `ROOT_CAUSE_IDENTIFIED` |
| Graceful degradation | LLM failure → `waiting_for_ai` without crashing worker |
| Agent API | `GET /api/v1/incidents/{id}/agent-runs` |

### AI Investigation Flow

```text
Incident job claimed
  → triage + collect evidence via tools
  → investigation agent (LLM)
       → optional follow-up READ_ONLY tool calls
       → structured RCA JSON (validated)
  → persist agent_run + RCA
  → incident → ROOT_CAUSE_IDENTIFIED
  → investigation → completed
```

### RCA Output Shape

```json
{
  "summary": "...",
  "root_cause": "...",
  "confidence": 0.91,
  "severity": "critical",
  "evidence": [{"id":"E-001","source":"logs","finding":"..."}],
  "rejected_hypotheses": [{"hypothesis":"...","reason":"..."}],
  "recommended_actions": ["..."],
  "reasoning_summary": "..."
}
```

### Directory Structure

```
production-incident-investigator/
├── cmd/
│   ├── server/              # API entrypoint
│   └── worker/              # Investigation worker
├── internal/
│   ├── config/              # Environment configuration
│   ├── domain/
│   │   ├── incident/        # Incident models, state machine
│   │   ├── investigation/   # Investigation and job models
│   │   └── tool/            # Tool execution domain
│   ├── application/
│   │   ├── incident/        # Incident business logic
│   │   ├── investigation/   # Orchestrator, processor, intake
│   │   └── tool/            # Tool listing and execution
│   ├── agent/
│   │   ├── agents/          # Investigation agent
│   │   ├── llm/             # LLM provider interface + openai/mock
│   │   ├── memory/          # Short-term investigation memory
│   │   ├── tools/           # Tool interface, registry, executor
│   │   │   └── mocks/       # Mock tool implementations
│   │   └── wiring/          # Dependency wiring
│   ├── infrastructure/
│   │   ├── postgres/        # PostgreSQL repositories
│   │   └── redis/           # Redis client and job queue
│   ├── transport/http/      # HTTP server, router, middleware
│   └── pkg/logger/          # Structured logging
├── migrations/              # SQL migrations
├── docker-compose.yml
├── Dockerfile
└── .env.example
```

## Tech Stack

- **Go 1.23** with modules
- **Chi** HTTP router
- **pgx** PostgreSQL driver
- **go-redis** Redis client
- **Prometheus** metrics endpoint
- **golang-migrate** database migrations
- **slog** structured JSON logging

## Database Schema (Phase 1)

### `incidents`

| Column | Type | Description |
|--------|------|-------------|
| id | UUID | Primary key |
| title | TEXT | Incident title |
| description | TEXT | Detailed description |
| severity | TEXT | critical, high, medium, low |
| service | TEXT | Affected service |
| environment | TEXT | production, staging, etc. |
| alert_source | TEXT | grafana, api, demo |
| status | TEXT | Lifecycle state |
| created_at | TIMESTAMPTZ | Creation time |
| updated_at | TIMESTAMPTZ | Last update |

### `incident_events`

| Column | Type | Description |
|--------|------|-------------|
| id | UUID | Primary key |
| incident_id | UUID | FK to incidents |
| event_type | TEXT | e.g. incident.created |
| from_status | TEXT | Previous status (nullable) |
| to_status | TEXT | New status (nullable) |
| message | TEXT | Human-readable message |
| metadata | JSONB | Additional context |
| created_at | TIMESTAMPTZ | Event time |

## Incident Lifecycle

```
RECEIVED → TRIAGING → INVESTIGATING → ROOT_CAUSE_IDENTIFIED
    → REMEDIATION_PROPOSED → WAITING_FOR_APPROVAL → REMEDIATION_EXECUTED → RESOLVED

Also: FAILED, CANCELLED, ESCALATED
```

Invalid transitions are rejected by the domain layer.

## API

### Create Incident

```bash
curl -X POST http://localhost:8080/api/v1/incidents \
  -H "Content-Type: application/json" \
  -d '{
    "title": "API latency increased",
    "severity": "critical",
    "service": "payment-service",
    "environment": "production",
    "description": "P95 latency increased above 3 seconds",
    "alert_source": "grafana"
  }'
```

Returns `202 Accepted` with the created incident.

### List Incidents

```bash
curl "http://localhost:8080/api/v1/incidents?status=RECEIVED&limit=20"
```

### Get Incident

```bash
curl http://localhost:8080/api/v1/incidents/{id}
```

### Incident Timeline

```bash
curl http://localhost:8080/api/v1/incidents/{id}/timeline
```

### List Tools

```bash
curl http://localhost:8080/api/v1/tools
```

### Execute Tool (incident context)

```bash
# Read-only tool (runs immediately)
curl -X POST http://localhost:8080/api/v1/incidents/{id}/tools/search_logs/execute \
  -H "Content-Type: application/json" \
  -d '{}'

# Approval-required tool (blocked without approved=true)
curl -X POST http://localhost:8080/api/v1/incidents/{id}/tools/create_github_issue/execute \
  -H "Content-Type: application/json" \
  -d '{"input":{"title":"Follow-up","body":"details"},"approved":true}'
```

### Get Evidence (tool executions)

```bash
curl http://localhost:8080/api/v1/incidents/{id}/evidence
```

### Get Investigation (includes RCA)

```bash
curl http://localhost:8080/api/v1/incidents/{id}/investigation
```

### Get Agent Runs

```bash
curl http://localhost:8080/api/v1/incidents/{id}/agent-runs
```

### Health

```bash
curl http://localhost:8080/api/v1/health
curl http://localhost:8080/api/v1/health/live
```

### Response Format

```json
{
  "success": true,
  "data": {},
  "error": null
}
```

## Local Setup

### Prerequisites

- Go 1.23+
- Docker and Docker Compose

### Quick Start (Docker)

```bash
cp .env.example .env
docker compose up --build
```

API available at `http://localhost:8080`.

### Local Development (without Docker for API)

```bash
# Start dependencies
docker compose up postgres redis migrate -d

# Run API
cp .env.example .env
go run ./cmd/server
```

### Run Migrations Manually

```bash
migrate -path migrations -database "$DATABASE_URL" up
```

## Environment Variables

See [`.env.example`](.env.example) for all variables.

| Variable | Required | Description |
|----------|----------|-------------|
| DATABASE_URL | Yes | PostgreSQL connection string |
| REDIS_URL | Yes | Redis connection string |
| HTTP_PORT | No | Default: 8080 |
| APP_ENV | No | Default: development |
| LOG_LEVEL | No | debug, info, warn, error |
| LLM_PROVIDER | No | `mock` (default) or `openai` |
| LLM_MODEL | No | Model name / mock model id |
| LLM_API_KEY | For openai | API key for OpenAI-compatible providers |
| LLM_BASE_URL | No | Default: `https://api.openai.com/v1` |
| LLM_TIMEOUT | No | Default: 60s |

## Testing

```bash
gofmt -w .
go test ./...
go vet ./...

# Integration tests (requires docker compose up)
go test -tags=integration ./tests/integration/... -v
```

## Roadmap

| Phase | Status | Scope |
|-------|--------|-------|
| 1 — Foundation | ✅ Complete | HTTP, PostgreSQL, Redis, incident API |
| 2 — Incident Engine | ✅ Complete | Worker, job queue, investigation model |
| 3 — Tool Framework | ✅ Complete | Tool registry, permissions, mocks, evidence API |
| 4 — AI Agent | ✅ Complete | LLM abstraction, investigation agent, structured RCA |
| 5 — RAG | Planned | Document ingestion, pgvector |
| 6 — Integrations | Planned | GitHub, Slack, Grafana, Prometheus, Loki |
| 7 — Human Approval | Planned | Approval workflow, action execution |
| 8 — Dashboard | Planned | React frontend |
| 9 — Observability | Planned | OpenTelemetry, Grafana dashboards |
| 10 — Hardening | Planned | Auth, rate limiting, integration tests |

## License

MIT
