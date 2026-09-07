# Phase 10 — Hardening

## AuthN

- Set `API_KEY` to require `X-API-Key` or `Authorization: Bearer` on `/api/v1/*`
- Exempt: `/api/v1/health`, `/api/v1/health/live`, `/api/v1/metrics`
- Dashboard stores key in `localStorage.opspilot_api_key` (dev convenience)

## Rate limiting

- In-memory fixed window on incident create and Grafana webhook
- `RATE_LIMIT_PER_MINUTE` (default 120) used as guidance; routes use tighter buckets (60 create / 30 webhook)

## Webhook HMAC

- `GRAFANA_WEBHOOK_SECRET` enables `X-Grafana-Signature` (hex HMAC-SHA256 of body)
- When unset, webhooks are open (local/dev)

## CORS

- `CORS_ORIGINS` CSV (default `http://localhost:5173,http://localhost:3000`)

## CI

GitHub Actions workflow `.github/workflows/ci.yml`:

1. `gofmt` / `go vet` / `go test`
2. Compose up + integration tests

## Secrets hygiene

- Never log `LLM_API_KEY`, `GITHUB_TOKEN`, `SLACK_BOT_TOKEN`, `API_KEY`
- Prefer mock providers in compose defaults
