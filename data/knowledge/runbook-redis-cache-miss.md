# Redis Cache Miss Storm

## Symptoms
- Sudden CPU increase on Redis
- Origin databases overloaded
- Latency across multiple services

## Diagnosis
1. Check Redis command rate and hit ratio.
2. Confirm recent cache key schema changes or TTL misconfiguration.
3. Look for thundering-herd retries.

## Mitigation
- Temporarily raise cache TTLs for hot keys
- Enable request coalescing / singleflight
- Rate-limit origin fallbacks
