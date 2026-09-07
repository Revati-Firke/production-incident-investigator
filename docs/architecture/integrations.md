# Phase 6 — External Integrations

Design reference for OpsPilot connectors to GitHub, Slack, Grafana, Prometheus, and Loki.
Keep this updated when changing providers, auth, or tool wiring.

## Goals

- Replace Phase 3 hardcoded observability/GitHub/Slack tools with **swappable adapters**
- Keep **tool names, schemas, and permissions stable** so the worker and agent do not change
- Default every provider to **`mock`** so local Docker works without credentials
- Ingest Grafana alerts via webhook into the existing incident intake path

## Architecture

```text
config (*_PROVIDER=mock|real)
        │
        ▼
 adapters (HTTP clients + mocks)
 internal/integrations/{loki,prometheus,grafana,github,slack}
        │
        ▼
 tool wrappers (same names)
 internal/agent/tools/integrations
        │
        ▼
 wiring.NewToolBundle → Executor → worker batch / agent loop
```

Integrations are **not** a separate agent. They plug into the tool framework
(same pattern as Phase 5 RAG).

## Provider switches

| Integration | Env provider | Real value | Required for real mode |
|-------------|--------------|------------|------------------------|
| Loki | `LOKI_PROVIDER` | `loki` | `LOKI_URL` (+ optional `LOKI_TOKEN`) |
| Prometheus | `PROMETHEUS_PROVIDER` | `prometheus` | `PROMETHEUS_URL` |
| Grafana | `GRAFANA_PROVIDER` | `grafana` | `GRAFANA_URL` (+ optional token) |
| GitHub | `GITHUB_PROVIDER` | `github` | `GITHUB_TOKEN`, `GITHUB_REPOSITORY=owner/repo` |
| Slack | `SLACK_PROVIDER` | `slack` | `SLACK_BOT_TOKEN` |

Shared: `INTEGRATION_TIMEOUT` (default `15s`).

GitHub writes (`create_github_issue`, `create_pull_request`) additionally require
`GITHUB_WRITE_ENABLED=true`. Read tools work without that flag.

## Tool mapping

| Tool | Adapter | Permission |
|------|---------|------------|
| `search_logs` | Loki | READ_ONLY |
| `query_metrics` | Prometheus | READ_ONLY |
| `get_service_health` | Grafana | READ_ONLY |
| `search_github_commits` | GitHub | READ_ONLY |
| `inspect_code` | GitHub | READ_ONLY |
| `create_github_issue` | GitHub | REQUIRES_APPROVAL |
| `create_pull_request` | GitHub | REQUIRES_APPROVAL |
| `send_slack_notification` | Slack | AUTONOMOUS |

DB / deployment tools remain local mocks (`RegisterLocal`).

## HTTP surfaces

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/v1/integrations` | Provider status for operators |
| `POST` | `/api/v1/webhooks/grafana` | Map Grafana alert JSON → incident intake |

Existing tool execute API continues to work unchanged.

### Example Grafana webhook

```bash
curl -X POST http://localhost:8080/api/v1/webhooks/grafana \
  -H 'Content-Type: application/json' \
  -d '{
    "title": "HighLatency",
    "status": "firing",
    "commonLabels": {
      "alertname": "HighLatency",
      "service": "payment-service",
      "environment": "production",
      "severity": "critical"
    },
    "alerts": [{"annotations": {"summary": "P95 > 3s"}}]
  }'
```

## Packages

| Path | Responsibility |
|------|----------------|
| `internal/integrations` | Shared types + status model |
| `internal/integrations/loki` | LogQL query_range client + mock |
| `internal/integrations/prometheus` | Instant query client + mock |
| `internal/integrations/grafana` | Health API + webhook → incident mapping |
| `internal/integrations/github` | Commits / contents / issues / PRs |
| `internal/integrations/slack` | `chat.postMessage` |
| `internal/agent/tools/integrations` | Tool wrappers |
| `internal/agent/wiring` | Provider selection |

## Operational notes

1. Compose defaults all providers to `mock` — no external deps required.
2. Tool results include a `provider` field (`mock` / `loki` / …) for auditability.
3. Real Loki LogQL assumes labels `service` and optionally `environment`; adjust
   `buildLogQL` if your label schema differs.
4. Prometheus default PromQL uses common `service` label conventions; override via
   tool `metric` input for a raw query.
5. Approval-required GitHub write tools are never auto-invoked by the LLM agent.

## Future extensions

- Alertmanager webhook (in addition to Grafana)
- PagerDuty / Opsgenie paging tools
- Per-service repository mapping (not a single `GITHUB_REPOSITORY`)
- OAuth / GitHub App auth instead of PAT
- Slack Block Kit rich incident cards (Phase 7/8 UI alignment)
- Circuit breakers + retry budgets per adapter

## Verification checklist

```bash
gofmt -w .
go test ./...
docker-compose up --build -d
go test -tags=integration ./tests/integration/... -v
curl -s http://localhost:8080/api/v1/integrations | jq .
curl -s -X POST http://localhost:8080/api/v1/webhooks/grafana \
  -H 'Content-Type: application/json' \
  -d '{"title":"t","status":"firing","commonLabels":{"service":"payment-service","severity":"high"}}' | jq .
```
