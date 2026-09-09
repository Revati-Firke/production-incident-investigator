# OpsPilot — Step-by-step test plan

Use this when the stack is running (`docker compose up`).  
**API:** http://localhost:8080 · **UI:** http://localhost:8088

If `API_KEY` is set in `.env`, add `-H "X-API-Key: $API_KEY"` to curl (or the header your deploy expects).

---

## Step 0 — Stack health (1 min)

```bash
cd production-incident-investigator   # repo root with docker-compose.yml

docker compose ps
# Expect: postgres, redis, opspilot-api, opspilot-worker, opspilot-web = Up
# migrate = Exit 0

curl -s http://localhost:8080/api/v1/health | jq .
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8088/
```

**Pass:** health JSON `"success": true` (or healthy deps), web returns `200`.

---

## Step 1 — Create an incident (API)

```bash
INC=$(curl -s -X POST http://localhost:8080/api/v1/incidents \
  -H "Content-Type: application/json" \
  -d '{
    "title": "Payment API high latency",
    "description": "P95 latency above 3s on checkout",
    "severity": "critical",
    "service": "payment-service",
    "environment": "production",
    "alert_source": "manual-test"
  }')

echo "$INC" | jq .
# Create returns 202 with data.incident
ID=$(echo "$INC" | jq -r '.data.incident.id // .data.id // empty')
echo "INCIDENT_ID=$ID"
```

**Pass:** `201`/`200`, incident `id` returned, status starts at `RECEIVED` (or quickly moves).

---

## Step 2 — Wait for investigation (worker)

Poll until status reaches `WAITING_FOR_APPROVAL` (usually ~10–60s with mock LLM):

```bash
for i in $(seq 1 30); do
  curl -s "http://localhost:8080/api/v1/incidents/$ID" | jq -r '.data.status'
  sleep 2
done
```

Also check:

```bash
curl -s "http://localhost:8080/api/v1/incidents/$ID/investigation" | jq .
curl -s "http://localhost:8080/api/v1/incidents/$ID/evidence" | jq .
curl -s "http://localhost:8080/api/v1/incidents/$ID/agent-runs" | jq .
```

**Pass:**
- Status: `WAITING_FOR_APPROVAL` (via `ROOT_CAUSE_IDENTIFIED` → proposed)
- Investigation has RCA / confidence
- Evidence has tool runs (logs, metrics, health, deployments, runbooks)
- At least one agent-run

**If stuck:** `docker compose logs -f opspilot-worker`

---

## Step 3 — Remediations queue

```bash
curl -s http://localhost:8080/api/v1/remediations/awaiting | jq .
curl -s "http://localhost:8080/api/v1/incidents/$ID/remediations" | jq .
PROP=$(curl -s "http://localhost:8080/api/v1/incidents/$ID/remediations" | jq -r '.data[0].id // empty')
echo "PROPOSAL_ID=$PROP"
```

**Pass:** proposal listed, status `proposed` (or equivalent awaiting approval).

---

## Step 4A — Approve (happy path)

```bash
curl -s -X POST "http://localhost:8080/api/v1/incidents/$ID/remediations/$PROP/approve" \
  -H "Content-Type: application/json" \
  -d '{"actor":"oncall-tester","comment":"approved in step-by-step test"}' | jq .

curl -s "http://localhost:8080/api/v1/incidents/$ID" | jq -r '.data.status'
```

**Pass:** status becomes `RESOLVED` (via `REMEDIATION_EXECUTED`). Evidence/timeline shows GitHub issue + Slack notify tools (mock).

---

## Step 4B — Reject path (optional second incident)

Create another incident (Step 1), wait for approval (Step 2–3), then:

```bash
curl -s -X POST "http://localhost:8080/api/v1/incidents/$ID/remediations/$PROP/reject" \
  -H "Content-Type: application/json" \
  -d '{"actor":"oncall-tester","comment":"false alarm"}' | jq .
```

**Pass:** incident → `CANCELLED`.

---

## Step 5 — Dashboard UI

1. Open http://localhost:8088  
2. See incident list (your test incident)  
3. Open detail: timeline, evidence, RCA, remediation  
4. On a *new* awaiting incident: click **Approve** or **Reject**  
5. Pages: Awaiting, Knowledge, Integrations (if present in nav)

**Pass:** UI loads, detail matches API, approve/reject updates status.

---

## Step 6 — Grafana webhook intake

```bash
curl -s -X POST http://localhost:8080/api/v1/webhooks/grafana \
  -H "Content-Type: application/json" \
  -d '{
    "title": "HighLatency",
    "status": "firing",
    "commonLabels": {
      "alertname": "HighLatency",
      "service": "payment-service",
      "environment": "production",
      "severity": "critical"
    },
    "alerts": [{"annotations": {"summary": "P95 latency above 3s"}}]
  }' | jq .
```

**Pass:** incident created; worker investigates like Step 2.

If `GRAFANA_WEBHOOK_SECRET` is set, unsigned requests should fail (401/403).

---

## Step 7 — RAG

```bash
curl -s -X POST http://localhost:8080/api/v1/rag/search \
  -H "Content-Type: application/json" \
  -d '{"query":"database connection pool","top_k":5}' | jq .
```

**Pass:** hits from seeded `data/knowledge/` (or empty only if seed failed — check worker/API logs for RAG seed).

Re-seed if needed:

```bash
docker compose exec opspilot-api /app/opspilot-ingest -dir /app/data/knowledge -force
# or from host with Go:
# go run ./cmd/ingest -dir data/knowledge -force
```

---

## Step 8 — Integrations status

```bash
curl -s http://localhost:8080/api/v1/integrations | jq .
curl -s http://localhost:8080/api/v1/tools | jq .
```

**Pass:** providers show `mock` (default); tools list includes observability + remediation tools.

---

## Step 9 — Metrics & liveness

```bash
curl -s http://localhost:8080/api/v1/health/live
curl -s http://localhost:8080/api/v1/metrics | head -20
```

**Pass:** live OK; Prometheus text metrics returned.

---

## Step 10 — Automated tests

```bash
go test ./...
go test -tags=integration ./tests/integration/... -v
```

**Pass:** unit + integration green (compose must be up for integration).

---

## Quick failure guide

| Symptom | Check |
|---------|--------|
| API down | `docker compose logs opspilot-api` |
| Never leaves RECEIVED | worker logs; redis/postgres healthy |
| No RAG hits | seed; `pgvector` image; migration `000005` |
| Approve fails | proposal id wrong; status not awaiting |
| UI blank / API errors | CORS / proxy; open browser network tab |
| Compose recreate weirdness | `docker compose down` then `up --build` (avoid partial recreate on old compose) |

---

## Suggested order (full day-of test)

1. Step 0 health  
2. Step 1–4A API happy path  
3. Step 5 UI approve on a second incident  
4. Step 4B reject once  
5. Steps 6–9 extras  
6. Step 10 `go test`
