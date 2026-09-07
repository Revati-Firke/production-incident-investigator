# Phase 9 — Observability

OpsPilot instruments API and worker with OpenTelemetry.

## Config

| Variable | Default | Purpose |
|----------|---------|---------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | empty (disabled) | OTLP HTTP endpoint host, e.g. `localhost:4318` |
| `OTEL_SERVICE_NAME` | `opspilot` | Base service name (`-api` / `-worker` suffix applied) |

When the endpoint is unset, tracing is a no-op.

## What is traced

- Process bootstrap (API / worker)
- Investigation jobs and tool/LLM spans can be added around hot paths via `otelx.Tracer("opspilot")`

## Local collector (optional)

```yaml
# docker-compose.otel.yml snippet
otel-collector:
  image: otel/opentelemetry-collector:0.116.0
  command: ["--config=/etc/otel-collector-config.yaml"]
  volumes:
    - ./deploy/otel/collector.yaml:/etc/otel-collector-config.yaml
  ports:
    - "4318:4318"
```

Sample Grafana dashboard JSON: [`deploy/grafana/opspilot-overview.json`](../../deploy/grafana/opspilot-overview.json).

## Runbooks

| Symptom | Check |
|---------|-------|
| Investigation stuck `waiting_for_ai` | Worker logs + agent_runs.error; LLM provider |
| Approval backlog | `GET /api/v1/remediations/awaiting` |
| Tool failures | `GET /api/v1/incidents/{id}/evidence` statuses |
