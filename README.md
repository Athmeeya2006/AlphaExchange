# AlphaExchange — Trading Infrastructure Evaluation Platform

A production-grade platform for evaluating contestant order-book implementations
under realistic market simulation. Contestants upload code; the platform builds it
into a hardened sandbox, hammers it with thousands of simulated trading bots,
validates correctness against a reference matching engine, and ranks everyone on a
live leaderboard.

## Architecture

```
Contestant -> Submission API -> MinIO -> Build Worker -> Docker Sandbox
                                              |
                                       Orchestrator <- Postgres
                                              |  (state machine + crash recovery)
                                              v
                                Bot Fleet (load generators)
                                              |  bot-telemetry (Kafka, 16 partitions)
                                              v
                          Telemetry Ingester (validate -> TimescaleDB + Redis)
                                              |
                                      Leaderboard API (Redis + WebSocket)
                                              v
                                   React Frontend (live rankings)
```

| Service | Language | Role |
|---|---|---|
| `submission-api` | Go | Receives uploads, queues build jobs, serves REST + leaderboard |
| `build-worker` | Go | Compiles code into hardened Docker sandboxes |
| `orchestrator` | Go | Test lifecycle state machine, crash recovery, scoring |
| `bot-fleet` | Go | Spawns market-maker / taker / spammer / whale bots |
| `telemetry-ingester` | Go | Validates fills, HDR-histogram percentiles, persists metrics |
| `leaderboard-api` | Go | Scoring + WebSocket fan-out |
| `shadow-orderbook` | Go (lib) | Reference price-time-priority matching engine |
| `frontend` | React + TS | Live leaderboard, submission, results |

## Quick start (local)

Prerequisites: Docker 24+, Docker Compose v2, Go 1.22+, Node 20+.

```bash
make up            # full stack (infra + all services + frontend)
make smoke-test    # end-to-end check once builds are ready
```

Access:
- Frontend: http://localhost:3000
- Submission API: http://localhost:8080
- Leaderboard API / WS: http://localhost:8084
- MinIO console: http://localhost:9001

Infra-only (run services with `go run`):

```bash
make dev
make migrate && make seed
```

## Common commands

```bash
make test                # unit tests for every Go module + frontend
make build               # build all Docker images
make lint                # go vet (+ golangci-lint if installed)
make proto               # regenerate protobuf Go code
make template LANG=cpp   # generate a contestant starter
```

## For contestants

- API contract: docs/contestant-api.md
- Scoring: docs/scoring-formula.md
- A correct reference implementation lives in testdata/sample-orderbook/.

## Design notes

- docs/architecture.md - full system design
- docs/NORMAL_VS_SMART.md - key engineering decisions and why
- docs/SECURITY.md - the 7-layer sandbox isolation model
- docs/CONTEST_DAY_RUNBOOK.md - operations runbook

## Scoring

```
composite_score = 0.40*norm(tps) + 0.40*(1 - norm(p99)) + 0.20*correctness_rate
```

Normalized min-max across contestants, scaled to 0-100. Tiebreak: correctness, then p99.
