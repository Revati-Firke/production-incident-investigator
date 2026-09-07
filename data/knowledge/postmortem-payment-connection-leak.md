# INC-2041 Payment API Latency from Connection Leak

## Summary
On 2025-11-12, payment-service latency spiked after deploy `payment-service@2.14.3`.
Root cause was a connection acquired in `internal/repository/payment.go` that was not released on an error path.

## Impact
- 18 minutes elevated latency
- ~3% checkout error rate

## Timeline
- 14:02 Alert fired (P95 > 3s)
- 14:08 Deploy correlated
- 14:15 Connection pool saturation confirmed
- 14:20 Pods restarted; latency recovered
- 14:40 Hotfix deployed

## Lessons
- Add pool wait alerts
- Add lint/test for connection release on error paths
- Prefer feature flags for repository refactors
