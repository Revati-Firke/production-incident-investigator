# OpsPilot — Understanding Guide

A mental model for reading and navigating this codebase. For setup, APIs, and env vars, use the root [README](../README.md). For phase design notes, see [architecture/](architecture/).

---

## 1. What this system does (one sentence)

**OpsPilot takes a production alert, gathers evidence with tools, asks an AI agent for a structured root cause, proposes remediation, waits for a human approve/reject, then executes approved actions — with everything audited in Postgres.**

Default local mode uses **mock** LLM, embeddings, and integrations so the full loop works without external API keys.

---

## 2. Core mental model

Think in **three runtimes** and **one shared domain**:

| Runtime | Binary | Job |
|---------|--------|-----|
| API | `cmd/server` | HTTP: create incidents, query state, approve remediations, webhooks |
| Worker | `cmd/worker` | Claim investigation jobs, run tools + agent, auto-propose remediation |
| Ingest | `cmd/ingest` | Chunk/embed knowledge docs into pgvector (optional; API also seeds) |
| Dashboard | `web/` | Operator UI (nginx proxies `/api` → API) |

Shared backing store:

- **PostgreSQL (+ pgvector)** — source of truth (incidents, jobs, evidence, RAG, approvals)
- **Redis** — job *hint* queue (`LPUSH`/`BRPOP`); worker also polls DB if Redis is quiet

```text
Alert / UI / curl
       │
       ▼
   cmd/server  ──writes──►  Postgres
       │                      ▲
       └──notify Redis──►  cmd/worker ──tools/agent/RCA──┘
                              │
                              └── auto ProposeFromRCA → WAITING_FOR_APPROVAL
                                       │
                              Human approve (API/UI) → execute tools → RESOLVED
```

---

## 3. Incident lifecycle (status machine)

Statuses live in `internal/domain/incident/status.go`. The happy path is:

```text
RECEIVED
  → TRIAGING
  → INVESTIGATING          (worker running tools + agent)
  → ROOT_CAUSE_IDENTIFIED
  → REMEDIATION_PROPOSED
  → WAITING_FOR_APPROVAL   (human gate)
  → REMEDIATION_EXECUTED
  → RESOLVED
```

Reject → `CANCELLED`. Failures can go to `FAILED` / `ESCALATED` where the transition table allows.

Every meaningful change should leave a trail in `incident_events` (timeline) plus specialized tables (`tool_executions`, `agent_runs`, `approvals`).

---

## 4. How to read the code (layered layout)

Go code under `internal/` follows a clean-ish hexagonal layout:

```text
transport/http     → HTTP handlers, middleware, router (thin)
application/*      → use-cases / orchestration (incident, investigation, tool, rag, remediation)
domain/*           → entities, status rules, repository interfaces
infrastructure/*   → Postgres + Redis implementations
agent/*            → LLM, investigation agent, tool registry/executor, RAG, wiring
integrations/*     → Loki / Prometheus / Grafana / GitHub / Slack clients (mock or real)
config, pkg        → env config, logging, OpenTelemetry helpers
```

**Rule of thumb when chasing a bug or feature:**

1. Start at the HTTP route in `internal/transport/http/router.go`
2. Follow the handler into `internal/application/...`
3. Domain rules / types in `internal/domain/...`
4. SQL and persistence in `internal/infrastructure/postgres/...`
5. Agent/tool behavior in `internal/agent/...` (wired in `internal/agent/wiring/tools.go`)

### Entry points worth opening first

| File | Why |
|------|-----|
| `cmd/server/main.go` | What the API wires together |
| `cmd/worker/main.go` | What the worker wires together |
| `internal/application/investigation/orchestrator.go` | Create incident + enqueue job (atomic intake) |
| `internal/application/investigation/service.go` | Worker processor: tools → agent → propose |
| `internal/agent/agents/investigation.go` | Agent loop + RCA |
| `internal/application/remediation/service.go` | Propose / approve / reject / execute |
| `internal/agent/wiring/tools.go` | Registers tools + providers (mock vs real) |

---

## 5. End-to-end path (follow once)

### A. Incident intake

1. `POST /api/v1/incidents` or Grafana webhook `POST /api/v1/webhooks/grafana`
2. Orchestrator creates **incident + investigation + job** in one transaction
3. Redis gets a job notification; worker claims the job (`FOR UPDATE SKIP LOCKED`)

### B. Automated investigation (worker)

1. Transition incident toward investigating
2. Run default **read-only** tools (see `defaultInvestigationTools` in investigation service):
   - `search_logs`, `query_metrics`, `get_service_health`, `get_recent_deployments`, `search_runbooks`
3. Investigation agent uses LLM (mock by default) + memory + optional further tools
4. Persist structured RCA on the investigation; status → `ROOT_CAUSE_IDENTIFIED`
5. Auto `ProposeFromRCA` → remediation proposal → `WAITING_FOR_APPROVAL`
6. Slack notify (mock or real) that approval is needed

### C. Human approval

1. Dashboard (`http://localhost:8088`) or API approve/reject
2. **Approve** → run write tools (e.g. `create_github_issue`, `send_slack_notification` with approval) → `REMEDIATION_EXECUTED` → `RESOLVED`
3. **Reject** → `CANCELLED`

### D. Evidence & audit

| Question | Look at |
|----------|---------|
| What happened to the incident? | `GET /api/v1/incidents/{id}` + timeline |
| What did tools return? | `GET /api/v1/incidents/{id}/evidence` |
| What did the agent conclude? | investigation + `GET .../agent-runs` |
| What is waiting on humans? | `GET /api/v1/remediations/awaiting` |

---

## 6. Tools, permissions, and integrations

Tools implement a common interface (`internal/agent/tools`). Permissions:

| Permission | Meaning |
|------------|---------|
| `READ_ONLY` | Safe during auto-investigation |
| `REQUIRES_APPROVAL` | Must not run until remediation is approved |
| `AUTONOMOUS` | Allowed for notifications / low-risk messaging |

**Integrations** (`internal/integrations/*`) are separate from tools: clients for Loki, Prometheus, Grafana, GitHub, Slack. Tools call those clients. Provider env vars default to `mock` (see `.env.example`).

`GITHUB_WRITE_ENABLED=false` by default even when using the real GitHub provider — write actions stay gated.

---

## 7. RAG (knowledge)

- Docs live in `data/knowledge/` (runbooks, playbooks, postmortems)
- Chunk + embed → pgvector (`migrations/000005_rag`)
- Tools `search_runbooks` / `search_previous_incidents` query that store
- Seed on start (`RAG_SEED_ON_START`) or `go run ./cmd/ingest -dir data/knowledge`

Design detail: [architecture/rag.md](architecture/rag.md).

---

## 8. Dashboard & hardening

- **UI:** `web/` — Vite/React; Compose service `opspilot-web` on `:8088`
- **Auth:** optional `API_KEY` on API (health/metrics exempt)
- **Rate limit:** in-memory per `RATE_LIMIT_PER_MINUTE`
- **Grafana webhook:** optional HMAC via `GRAFANA_WEBHOOK_SECRET`
- **OTel:** set `OTEL_EXPORTER_OTLP_ENDPOINT` or tracing is a no-op
- **CI:** `.github/workflows/ci.yml`

---

## 9. Database schema map

Migrations are the schema changelog:

| Migration | Adds |
|-----------|------|
| `000001` | Incidents + events |
| `000002` | Investigations + jobs |
| `000003` | Tool executions |
| `000004` | Agent runs / RCA fields |
| `000005` | RAG documents + embeddings (pgvector) |
| `000006` | Remediation proposals + approvals |

---

## 10. Suggested learning path (60–90 minutes)

1. Skim README architecture diagram + roadmap table  
2. Read this guide (§2–§5)  
3. Trace **create incident** from `incident_handler` → orchestrator → Postgres intake  
4. Trace **worker** from `cmd/worker` → `Processor.Run` → tools → agent → `ProposeFromRCA`  
5. Trace **approve** in `remediation/service.go`  
6. Open `web/src/pages/IncidentDetail.tsx` and map buttons to API calls in `web/src/api.ts`  
7. Run compose, create an incident, watch statuses flip, approve in the UI  

---

## 11. Local demo checklist

```bash
cp .env.example .env
docker compose up --build
# API :8080  |  UI :8088

# Create an incident (or use the UI), wait for WAITING_FOR_APPROVAL, approve.
curl -s http://localhost:8080/api/v1/health
```

Tests:

```bash
go test ./...
go test -tags=integration ./tests/integration/... -v   # stack must be up
```

---

## 12. Glossary

| Term | Meaning |
|------|---------|
| **Incident** | Top-level case (service, severity, status) |
| **Investigation** | Worker-driven analysis record for one incident |
| **Job** | Queue unit claimed by the worker |
| **Tool** | Discrete action (search logs, open issue, …) with permissions |
| **Evidence** | Persisted tool execution I/O |
| **RCA** | Structured root-cause analysis from the agent |
| **Proposal** | Remediation plan awaiting human decision |
| **Mock provider** | Deterministic stand-in (LLM / embed / Loki / …) for local/dev |

---

## Related docs

| Doc | Use when |
|-----|----------|
| [README](../README.md) | Run it, call APIs, configure env |
| [architecture/rag.md](architecture/rag.md) | Knowledge / embeddings |
| [architecture/integrations.md](architecture/integrations.md) | External adapters |
| [architecture/approvals.md](architecture/approvals.md) | Propose / approve / execute |
| [architecture/observability.md](architecture/observability.md) | OTel |
| [architecture/hardening.md](architecture/hardening.md) | Auth, limits, CI |
