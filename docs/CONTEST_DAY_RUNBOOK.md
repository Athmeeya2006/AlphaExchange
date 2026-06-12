# Contest Day Runbook

## T-24h — pre-contest
- [ ] `make test` green across all modules + frontend
- [ ] `make build` succeeds for every image
- [ ] `docker compose config` validates
- [ ] Seed contestants (`make seed`) and distribute API keys
- [ ] Confirm Kafka topic partition counts (`infra/kafka/topics.sh`)
- [ ] Smoke test on staging (`make smoke-test`)

## T-1h — systems check
- [ ] All pods Running: `kubectl get pods -n trade-eval`
- [ ] Kafka consumer lag ~0: `make check-kafka-lag`
- [ ] TimescaleDB + Redis reachable
- [ ] bot-fleet scaled to 0 (KEDA): `kubectl get hpa bot-fleet -n trade-eval`

## T-0 — start
1. Announce submissions open.
2. Watch first builds drain (< 60s each).
3. `bash scripts/verify-platform.sh`

## During — every 15 min
- [ ] Kafka consumer lag < 1000
- [ ] No OOMKilled events
- [ ] Leaderboard `updated_at` is fresh
- [ ] No ERROR logs in submission-api / orchestrator

## Incident response
- **Leaderboard stale**: check `leaderboard-api` logs + Redis `leaderboard:cached`;
  `kubectl rollout restart deployment/leaderboard-api`.
- **Build stuck > 5 min**: check `build-worker` logs and the Docker daemon.
- **Kafka lag growing**: `kubectl scale deployment telemetry-ingester --replicas=6`.
- **Orchestrator crash**: k8s restarts it; orphan detection recovers running tests
  within ~60s. Verify none stuck `running` via `/admin/active-tests`.

## Close
1. Wait for active tests to finish.
2. Freeze: `POST /admin/v1/leaderboard/freeze` (X-Admin-Key).
3. Export results, announce winners.
4. `kubectl scale deployment bot-fleet --replicas=0`.
