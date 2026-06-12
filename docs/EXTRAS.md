# Extras & Brilliant Ideas Implemented

Beyond the 130-prompt spec, the following production-readiness and "smart"
features were added. Each is wired in and verified (services build + test green).

## Security hardening (no endpoint left open)
- **orchestrator** `/admin/active-tests` → constant-time `X-Admin-Key` guard.
- **telemetry-ingester** `/v1/metrics/*` and `/v1/analysis/*` → constant-time
  `X-Internal-Token` guard (service-to-service); `/v1/health` stays public.
- **submission-api** `SecurityHeaders` middleware (CSP, nosniff, frame-deny, etc.)
  plus a `security` package: filename/language/test-param validation and a
  zip-bomb / zip-slip detector used by both submission-api and build-worker.
- Constant-time comparisons (`crypto/subtle`) for every admin/internal key.

## Real bug fixes found during the audit
- **bot-fleet client**: `http.Client.Timeout` was not detected as a timeout, so
  slow contestants were mislabeled "unreachable" instead of `timed_out`. Fixed via
  a `net.Error.Timeout()` check.
- **bot-fleet client**: 5xx responses from a contestant were swallowed as
  successful `REJECTED` fills, so the circuit breaker never tripped. Now surfaced
  as transport errors. Both covered by `chaos` tests.

## Observability (production-grade)
- Every HTTP service exposes **`/metrics`** (Prometheus), **`/healthz`**, **`/readyz`**:
  - submission-api: request count + duration histogram by route.
  - orchestrator: `tests_started/completed/failed`, `orphans_recovered`,
    `active_tests` gauge.
  - telemetry-ingester: `events_processed/correct`, `kafka_consumer_lag` gauge
    (fed by the lag monitor).
  - leaderboard-api: `websocket_connections` gauge.
- Grafana dashboards (`infra/grafana/dashboards/*.json`), ServiceMonitors and
  PrometheusRule alerts (`infra/k8s/monitoring`), Jaeger + OTel collector
  (`infra/k8s/tracing`), Fluent Bit/ES/Kibana values (`infra/helm/logging`).

## Advanced features
- **ML anomaly detection** (`anomaly/ml_detector.go`): Welford online z-score per
  contestant flags latency outliers vs the contestant's own baseline; a behaviour
  classifier (`CACHING`/`INCONSISTENT`/`CONSISTENT_HIGH_PERFORMER`) is published to
  Redis each second.
- **Historical analysis API**: `/v1/analysis/latency-distribution` (histogram
  buckets) and `/v1/analysis/head-to-head` (percentile comparison).
- **Live commentary ticker** (`commentary/generator.go`): rank-change /
  milestone / close-race callouts published over the same WebSocket fan-out as a
  `ticker_event` message and rendered as a scrolling ticker in the UI.
- **Contestant insights** (`/v1/contestants/{id}/insights`): rank, weakest
  dimension vs peers, and auto-generated improvement tips.
- **Score prediction** (`/v1/contestants/{id}/prediction`): least-squares trend
  projection of the final score with a confidence based on sample count.
- **Admin ops console** (`/admin/v1/system/status` + freeze/unfreeze/disqualify),
  surfaced in a frontend Operations Console page.
- **Webhooks** (P97): contestants register HMAC-SHA256-signed callbacks
  (`build_complete`/`test_complete`/`score_update`/`anomaly`) with bounded retries.
- **FIX 4.2 bot** (`bots/fix_bot.go`): self-contained message builder with correct
  BodyLength/checksum/sequence numbers (unit-tested).
- **Kafka ops**: parallel consumer group, lag monitor, offset tracker
  (`telemetry-ingester/kafka`).
- **Container resource monitor + Trivy image scanner** in build-worker; CRITICAL
  CVEs block a sandbox from starting.

## Frontend (world-class)
- Recharts components (LatencyChart with reference bands, SVG CorrectnessGauge,
  Sparkline), animated rank-change row flashes, scrolling commentary ticker,
  leader hero stats, pulsing connection-status indicator.
- Pages: Leaderboard, Submit, Results (with gauge + score breakdown), Progress
  (insights + live prediction), Operations Console (admin), Login.
- PWA manifest + theme color, React ErrorBoundary, mobile-responsive table,
  Vitest component tests.

## Dev-ex / prod hardening
- `.dockerignore` per service (smaller, faster image builds).
- `.golangci.yml` (errcheck, govet, staticcheck, unused, ineffassign, gosimple).
- All Go modules pinned to **go 1.22**; graceful shutdown on SIGTERM everywhere.

## gRPC / GraphQL contracts
- `proto/services.proto` (gRPC service definitions) and `proto/schema.graphql`
  define the low-latency/streaming contracts; generate stubs with `make proto`
  (requires protoc / gqlgen). The durable Kafka path remains the source of truth.
