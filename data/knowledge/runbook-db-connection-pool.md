# Database Connection Pool Exhaustion

## Symptoms
- Elevated P95/P99 API latency
- `too many connections` or pool wait timeouts in application logs
- Healthy pods reporting degraded readiness under load

## Diagnosis
1. Check database connection count vs max_connections / pool size.
2. Inspect repository error paths for connections acquired but not released.
3. Correlate with recent deploys that touched data-access code.

## Mitigation
- Restart affected pods as a temporary mitigation to drain leaked connections.
- Reduce pool max size temporarily if the database is saturated.
- Roll back the suspect deploy if connection leak is confirmed.

## Permanent Fix
- Ensure every `Acquire`/`Begin` path releases connections on error and success.
- Add connection pool metrics and alerts on wait time and active connections.
- Add integration tests that force error paths in the repository layer.
