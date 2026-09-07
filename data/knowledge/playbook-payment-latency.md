# Payment Service Latency Spike

## Symptoms
- Grafana alert: payment-service P95 latency above threshold
- Checkout failures or timeouts in production
- Downstream dependency timeouts optional

## Investigation Steps
1. Query metrics for payment-service latency and error rate.
2. Search logs for connection pool, timeout, and 5xx patterns.
3. Review recent deployments for payment-service.
4. Search runbooks for database connection pool exhaustion.
5. Inspect code paths that talk to Postgres.

## Common Root Causes
- Database connection leaks after deploy
- Missing indexes on hot payment queries
- Upstream processor timeouts cascading into retries

## Escalation
Escalate to payments on-call if revenue impact exceeds 5 minutes or error rate > 2%.
