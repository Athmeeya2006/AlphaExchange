
 # MASTER PROMPTS: Trading Infrastructure Evaluation Platform
## Full Implementation Guide — 130 Prompts

> **How to use this file**: Feed each prompt to your AI agent in ORDER. Each prompt is fully self-contained with all context. Prompts are sized to what one agent call can do without mistakes. Never skip a prompt — each one produces artifacts the next one depends on. Smart alternatives are flagged with ��.

---

## PHASE 0 — PROJECT SCAFFOLD & ARCHITECTURE (Prompts 1–6)

---

### PROMPT 1 — Monorepo Scaffold

```
Create the full monorepo directory structure for a trading infrastructure evaluation platform.
The repo is called `trade-eval-platform`. Use the following EXACT structure:

trade-eval-platform/
├── services/
│   ├── submission-api/          # Go service: receives uploads
│   ├── build-worker/            # Go service: compiles + sandboxes code
│   ├── orchestrator/            # Go service: test lifecycle coordinator
│   ├── bot-fleet/               # Go service: load generator bots
│   ├── telemetry-ingester/      # Go service: Kafka consumer + metrics
│   ├── leaderboard-api/         # Go service: scoring + WebSocket
│   └── shadow-orderbook/        # Go library: reference matching engine
├── frontend/                    # React + TypeScript + Vite
├── infra/
│   ├── terraform/               # Cloud provisioning
│   ├── helm/                    # Kubernetes Helm charts per service
│   ├── docker/
│   │   ├── contestant/          # Hardened sandbox Dockerfile base
│   │   └── build-worker/        # Kaniko-based build Dockerfile
│   └── k8s/
│       ├── namespaces/
│       ├── network-policies/
│       └── rbac/
├── proto/                       # Protobuf definitions (shared types)
├── scripts/
│   ├── local-dev/               # Docker Compose for local dev
│   └── load-test/               # k6 scripts for infrastructure testing
├── docs/
│   ├── architecture.md
│   ├── api-spec.yaml            # OpenAPI 3.0
│   └── scoring-formula.md
├── .github/
│   └── workflows/               # CI/CD pipelines
├── docker-compose.yml           # Full local stack
├── docker-compose.dev.yml       # Lightweight dev stack
├── Makefile                     # Dev commands
└── README.md

For EACH service directory, create:
- main.go
- go.mod (module: github.com/trade-eval/<service-name>, go 1.22)
- Dockerfile
- .env.example
- README.md (one paragraph describing the service)

For the frontend directory, create a Vite + React + TypeScript scaffold:
- package.json with dependencies: react, react-dom, recharts, @tanstack/react-query, zustand, tailwindcss
- tsconfig.json
- vite.config.ts
- src/main.tsx
- src/App.tsx

Create the root Makefile with these targets:
- `make dev` — starts docker-compose.dev.yml
- `make up` — starts docker-compose.yml (full stack)
- `make down` — stops all
- `make proto` — regenerates protobuf files
- `make test` — runs all service tests
- `make lint` — runs golangci-lint on all services

Create docker-compose.yml with ALL infrastructure dependencies:
- kafka (confluentinc/cp-kafka:7.6.0) with KRAFT mode (no Zookeeper)
  - topics auto-created: submissions, bot-telemetry, orchestrator-events, build-jobs
  - ports: 9092
- redis (redis:7.2-alpine) with persistence, ports: 6379
- minio (minio/minio:latest) with default bucket `submissions`, ports: 9000, 9001
- timescaledb (timescale/timescaledb:latest-pg16) with db `tradeeval`, ports: 5432
- postgres (postgres:16-alpine) — separate DB for orchestrator state, ports: 5433

Output every file with full content, no placeholders.
```

---

### PROMPT 2 — Protobuf & Shared Types

```
In the `proto/` directory of the trade-eval-platform monorepo, create the following
Protobuf 3 definition files that will be the shared contract between all services.

FILE: proto/submission.proto
Define messages:
- Submission { string id, string contestant_id, string language (enum: CPP/RUST/GO/PYTHON), 
  string s3_key, int64 submitted_at, string status (enum: PENDING/BUILDING/READY/FAILED/OOM_KILLED/TIMEOUT) }
- BuildJob { string submission_id, string s3_key, string language }
- BuildResult { string submission_id, string status, string error_log, string container_ip, int32 container_port }

FILE: proto/test_events.proto
Define messages:
- StartTest { string test_id, string contestant_id, string target_ip, int32 target_port,
  int32 duration_seconds, int32 bot_count, repeated string bot_personas }
- StopTest { string test_id, string reason }
- TestHeartbeat { string test_id, int64 timestamp_ns, int32 active_bots }

FILE: proto/telemetry.proto
Define messages:
- OrderEvent {
    string contestant_id, string test_id, string bot_id, string bot_persona,
    string order_id, int64 sent_at_ns, int64 acked_at_ns, int64 latency_us,
    string order_type (enum: LIMIT_BUY/LIMIT_SELL/MARKET_BUY/MARKET_SELL/CANCEL),
    double price, double quantity,
    Fill expected_fill, Fill actual_fill,
    bool correct, bool timed_out, bool bot_error,
    int64 sequence_number
  }
- Fill { double price, double quantity, string status (enum: FILLED/PARTIAL/REJECTED/PENDING) }
- LatencyWindow { string contestant_id, int64 window_start_ns, int64 window_end_ns,
    int64 p50_us, int64 p90_us, int64 p99_us, double tps, double correctness_rate }

FILE: proto/leaderboard.proto
Define messages:
- LeaderboardEntry { int32 rank, string contestant_id, string contestant_name,
    double score, int64 p50_us, int64 p90_us, int64 p99_us,
    double tps, double correctness_rate, string status, int64 last_updated_ns }
- LeaderboardUpdate { int64 timestamp, repeated LeaderboardEntry entries }

Also create:
FILE: proto/generate.sh
A shell script that runs `protoc` to generate Go code for all .proto files into
`proto/gen/go/` using `--go_out` and `--go-grpc_out` flags.

FILE: proto/README.md
Document all messages and their fields briefly.

Output all files with complete content.
```

---

### PROMPT 3 — Docker Compose Full Stack + Kafka Topics Bootstrap

```
In the trade-eval-platform repo, create a production-quality docker-compose.yml
that runs the ENTIRE platform locally. This is the authoritative local dev environment.

Requirements:
1. All infrastructure services (Kafka, Redis, MinIO, TimescaleDB, Postgres)
2. All application services (submission-api, build-worker, orchestrator, bot-fleet,
   telemetry-ingester, leaderboard-api, frontend)
3. A kafka-init service that creates all required topics with correct partition counts
4. A minio-init service that creates the `submissions` bucket
5. A db-migrate service that runs SQL migrations on both databases at startup
6. Health checks on every service
7. Named volumes for all stateful services
8. A dedicated Docker network `trade-eval-net` (bridge mode)
9. Resource limits on contestant sandbox containers (applied via labels)

Kafka topics to create (via kafka-init using kafka-topics.sh):
- submissions: 4 partitions, replication-factor 1
- build-jobs: 4 partitions, replication-factor 1
- orchestrator-events: 2 partitions, replication-factor 1
- bot-telemetry: 16 partitions, replication-factor 1  ← MOST IMPORTANT: high throughput
- leaderboard-updates: 2 partitions, replication-factor 1

Environment variables for all services should be loaded from a root `.env` file.
Create that `.env` file with ALL required env vars pre-filled with local dev defaults:
- KAFKA_BROKERS=kafka:9092
- REDIS_URL=redis:6379
- MINIO_ENDPOINT=minio:9000
- MINIO_ACCESS_KEY=minioadmin
- MINIO_SECRET_KEY=minioadmin
- MINIO_BUCKET=submissions
- TIMESCALE_DSN=postgres://postgres:postgres@timescaledb:5432/tradeeval?sslmode=disable
- ORCHESTRATOR_DB_DSN=postgres://postgres:postgres@postgres:5433/orchestrator?sslmode=disable
- BOT_TELEMETRY_TOPIC=bot-telemetry
- ORCHESTRATOR_EVENTS_TOPIC=orchestrator-events
- BUILD_JOBS_TOPIC=build-jobs
- SANDBOX_MEMORY_LIMIT=512m
- SANDBOX_CPU_CORES=2,3
- TEST_DEFAULT_DURATION_SECONDS=300
- TEST_DEFAULT_BOT_COUNT=500
- LEADERBOARD_UPDATE_INTERVAL_MS=500
- CONTESTANT_NETWORK=contestant-isolated
- LOG_LEVEL=info
- ENVIRONMENT=development

Also create docker-compose.dev.yml (lightweight: only infra, no app services, useful
when running services locally with `go run`).

Include a `scripts/local-dev/wait-for-it.sh` that all services use in their entrypoints
to wait for dependencies before starting.

Output all files with full content.
```

---

### PROMPT 4 — SQL Migrations: TimescaleDB + Orchestrator Postgres

```
Create all SQL migration files for the trade-eval-platform.

LOCATION: services/telemetry-ingester/migrations/
These run against TimescaleDB (postgres with timescaledb extension).

FILE: 001_enable_timescaledb.sql
Enable timescaledb extension.

FILE: 002_create_latency_samples.sql
Create table latency_samples:
  - time TIMESTAMPTZ NOT NULL
  - contestant_id TEXT NOT NULL
  - test_id TEXT NOT NULL
  - bot_id TEXT NOT NULL
  - bot_persona TEXT NOT NULL
  - latency_us BIGINT NOT NULL
  - order_type TEXT NOT NULL
  - correct BOOLEAN NOT NULL
  - timed_out BOOLEAN NOT NULL DEFAULT false

Then call SELECT create_hypertable('latency_samples', 'time') to make it a hypertable.
Create indexes:
  - (time, contestant_id) for per-contestant time-range queries
  - (test_id) for per-test queries
  - (contestant_id, bot_persona) for persona-breakdown analysis

FILE: 003_create_tps_samples.sql
Create table tps_samples:
  - time TIMESTAMPTZ NOT NULL
  - contestant_id TEXT NOT NULL
  - test_id TEXT NOT NULL
  - orders_per_second DOUBLE PRECISION NOT NULL
  - window_size_ms INT NOT NULL DEFAULT 1000

Make it a hypertable on `time`.
Add index on (time, contestant_id).

FILE: 004_create_test_summaries.sql
Create table test_summaries:
  - test_id TEXT PRIMARY KEY
  - contestant_id TEXT NOT NULL
  - started_at TIMESTAMPTZ NOT NULL
  - ended_at TIMESTAMPTZ
  - p50_latency_us BIGINT
  - p90_latency_us BIGINT
  - p99_latency_us BIGINT
  - peak_tps DOUBLE PRECISION
  - avg_tps DOUBLE PRECISION
  - total_orders BIGINT DEFAULT 0
  - correct_orders BIGINT DEFAULT 0
  - timed_out_orders BIGINT DEFAULT 0
  - correctness_rate DOUBLE PRECISION
  - composite_score DOUBLE PRECISION
  - status TEXT NOT NULL DEFAULT 'running'

LOCATION: services/orchestrator/migrations/
These run against the separate Postgres instance.

FILE: 001_create_submissions.sql
Table: submissions
  - id TEXT PRIMARY KEY
  - contestant_id TEXT NOT NULL
  - contestant_name TEXT NOT NULL
  - language TEXT NOT NULL
  - s3_key TEXT NOT NULL
  - status TEXT NOT NULL DEFAULT 'pending'
  - container_ip TEXT
  - container_port INT
  - container_id TEXT
  - error_log TEXT
  - created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  - updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()

FILE: 002_create_tests.sql
Table: tests
  - id TEXT PRIMARY KEY
  - submission_id TEXT NOT NULL REFERENCES submissions(id)
  - contestant_id TEXT NOT NULL
  - status TEXT NOT NULL DEFAULT 'pending'
    (values: pending, running, stopping, completed, failed)
  - started_at TIMESTAMPTZ
  - ended_at TIMESTAMPTZ
  - final_score DOUBLE PRECISION
  - failure_reason TEXT
  - orchestrator_instance_id TEXT  ← for crash recovery
  - last_heartbeat_at TIMESTAMPTZ  ← for orphan detection
  - created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
  - updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()

FILE: 003_create_contestants.sql
Table: contestants
  - id TEXT PRIMARY KEY
  - name TEXT NOT NULL
  - email TEXT UNIQUE NOT NULL
  - api_key TEXT UNIQUE NOT NULL
  - created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()

FILE: 004_triggers.sql
Create an updated_at trigger function that sets updated_at = NOW() on UPDATE.
Apply it to all three tables above.

Also create a seed file scripts/local-dev/seed.sql that inserts 5 test contestants
with UUIDs as IDs and random API keys for local development.

Output every file with complete SQL content.
```

---

### PROMPT 5 — OpenAPI Spec + Architecture Doc

```
Create the following documentation files for trade-eval-platform.

FILE: docs/api-spec.yaml
Full OpenAPI 3.0 specification for the submission-api service.

Paths:
POST /v1/submissions
  - Summary: Upload contestant code
  - Request: multipart/form-data with fields: file (binary), language (string enum),
    contestant_id (string)
  - Response 202: { submission_id, status: "pending", message }
  - Response 400: { error, details }
  - Response 413: { error: "File too large" } (max 50MB)
  - Auth: Bearer token (X-API-Key header)

GET /v1/submissions/{submission_id}
  - Summary: Get submission status
  - Response 200: Submission object (id, contestant_id, status, container_ip,
    container_port, error_log, created_at, updated_at)
  - Response 404: { error: "not found" }

GET /v1/submissions/{submission_id}/logs
  - Summary: Get build logs for a submission
  - Response 200: { logs: string }
  - Response 404

POST /v1/tests
  - Summary: Trigger a test for a submission
  - Request: { submission_id, duration_seconds?, bot_count?, bot_personas? }
  - Response 202: { test_id, status: "pending" }

GET /v1/tests/{test_id}
  - Summary: Get test status and results
  - Response 200: Full test object including scores

GET /v1/leaderboard
  - Summary: Get current leaderboard
  - Response 200: { updated_at, entries: [ LeaderboardEntry ] }

GET /v1/health
  - Response 200: { status: "ok", version, uptime_seconds }

Include proper schemas, authentication, and error response models.

FILE: docs/architecture.md
Write a detailed architecture document covering:
1. System overview with ASCII art pipeline diagram
2. Technology choices and WHY (Go for services, Kafka for event streaming,
   TimescaleDB for time-series, Redis for cache, MinIO for object storage)
3. Data flow walkthrough (submission to leaderboard)
4. Security model (sandbox isolation, network policies)
5. Scoring formula with weights explanation
6. Known limitations and future improvements

FILE: docs/scoring-formula.md
Document the exact scoring formula:
- composite_score = 0.40 * normalized_tps + 0.40 * normalized_inverse_p99 + 0.20 * correctness_rate
- Normalization formula: (value - min) / (max - min) across all contestants
- normalized_inverse_p99 = 1 - normalized_p99 (lower latency = higher score)
- Tiebreaker: correctness_rate, then p99_latency
- Edge cases: < 100 valid orders = disqualified
- Correctness rate definition: correct_orders / total_orders (excluding timed_out)

Output all files with complete content.
```

---

### PROMPT 6 — CI/CD GitHub Actions Pipelines

```
Create GitHub Actions workflow files for trade-eval-platform in .github/workflows/.

FILE: .github/workflows/ci.yml
Name: "CI — Build, Lint, Test"
Triggers: push to any branch, pull_request to main

Jobs:
1. lint-and-vet (runs on ubuntu-22.04):
   - Checkout code
   - Setup Go 1.22
   - Run `go vet ./...` in each service directory
   - Run golangci-lint with config: errcheck, govet, staticcheck, unused
   
2. test-services (runs on ubuntu-22.04, matrix: each service dir):
   - Checkout code
   - Setup Go 1.22
   - Start docker-compose.dev.yml (infra only)
   - Wait for all services healthy (60s timeout)
   - Run `go test -race -coverprofile=coverage.out ./...` in each service
   - Upload coverage to Codecov

3. test-frontend (runs on ubuntu-22.04):
   - Checkout code
   - Setup Node 20
   - Run `npm ci` in frontend/
   - Run `npm run type-check`
   - Run `npm test -- --run` (Vitest)
   - Run `npm run build` to verify build succeeds

4. build-images (runs after lint and test pass, on main branch only):
   - Build Docker image for each service
   - Tag with git SHA
   - Push to GitHub Container Registry (ghcr.io)

FILE: .github/workflows/integration.yml
Name: "Integration Tests"
Triggers: push to main, manual workflow_dispatch

Jobs:
1. integration-test:
   - Start full docker-compose.yml stack
   - Wait for all services healthy (120s)
   - Run scripts/integration-test.sh which:
     a. Creates a test contestant via API
     b. Uploads a sample C++ order book binary (stored in testdata/)
     c. Polls until submission status = READY (timeout 120s)
     d. Triggers a test with 10 bots, 30 seconds
     e. Polls until test status = completed (timeout 120s)
     f. Fetches leaderboard and asserts contestant appears
     g. Asserts composite_score > 0
   - Tear down stack

FILE: .github/workflows/security.yml
Name: "Security Scan"
Triggers: push to main, schedule (weekly)

Jobs:
1. trivy-scan: Scan all Docker images for HIGH/CRITICAL CVEs using trivy
2. gosec-scan: Run gosec static analysis on all Go services
3. dependency-audit: Run `go mod verify` and `npm audit --audit-level=high`

Create the scripts/integration-test.sh script with all the steps above using curl and jq.
Create testdata/sample-orderbook.cpp — a simple correct C++ order book implementation
(FIFO matching, price-time priority) that serves as the reference test binary.

Output all files with complete content.
```

---

## PHASE 1 — SUBMISSION API SERVICE (Prompts 7–14)

---

### PROMPT 7 — Submission API: Main Server + Router

```
Implement the main.go and HTTP router for services/submission-api/ in Go.

Stack: Go 1.22, chi router (github.com/go-chi/chi/v5), structured logging (log/slog).

FILE: services/submission-api/main.go
- Read all config from environment variables using a Config struct with struct tags
  Config fields:
    Port string (default "8080")
    KafkaBrokers string
    RedisURL string
    MinIOEndpoint, MinIOAccessKey, MinIOSecretKey, MinIOBucket string
    MaxUploadSizeMB int64 (default 50)
    OrchestratorDBDSN string
    LogLevel string (default "info")
    Environment string
  
- Initialize structured logger (slog with JSON handler in production, text in dev)
- Initialize connections: MinIO client, Redis client, Postgres pool (pgx/v5)
- Run database migrations on startup (use golang-migrate/migrate/v4)
- Build the router
- Start HTTP server with graceful shutdown (listen for SIGTERM/SIGINT, 30s drain)

FILE: services/submission-api/router.go
Build chi router with:
- Middleware: RequestID, RealIP, Logger (using slog), Recoverer, Timeout(30s),
  CORS (allow all origins in dev, specific in prod)
- Route group /v1 with auth middleware (validateAPIKey)
- Routes:
  POST   /v1/submissions           -> handlers.CreateSubmission
  GET    /v1/submissions/{id}      -> handlers.GetSubmission
  GET    /v1/submissions/{id}/logs -> handlers.GetSubmissionLogs
  POST   /v1/tests                 -> handlers.CreateTest
  GET    /v1/tests/{id}           -> handlers.GetTest
  GET    /v1/leaderboard          -> handlers.GetLeaderboard (no auth needed)
  GET    /v1/health               -> handlers.Health (no auth needed)

FILE: services/submission-api/middleware/auth.go
validateAPIKey middleware:
- Reads X-API-Key header
- Looks up key in Postgres `contestants` table
- If not found: 401 JSON response
- If found: stores contestant record in context (context.WithValue)
- Includes rate limiting: 10 submissions per hour per contestant using Redis
  (key: rate_limit:submission:{contestant_id}, INCR + EXPIRE 3600)

FILE: services/submission-api/go.mod
With all required dependencies pinned to specific versions:
- github.com/go-chi/chi/v5 v5.1.0
- github.com/minio/minio-go/v7 v7.0.70
- github.com/redis/go-redis/v9 v9.5.1
- github.com/jackc/pgx/v5 v5.5.5
- github.com/golang-migrate/migrate/v4 v4.17.1

Output complete, compilable Go code. No placeholder comments like "// TODO implement".
Every function must be fully implemented.
```

---

### PROMPT 8 — Submission API: Upload Handler

```
Implement the CreateSubmission handler for services/submission-api.

FILE: services/submission-api/handlers/submission.go

Implement CreateSubmission(w http.ResponseWriter, r *http.Request):

STEP 1 — Parse multipart form:
- Call r.ParseMultipartForm(cfg.MaxUploadSizeMB * 1024 * 1024)
- Return 400 if parse fails
- Extract `language` field, validate it's one of: cpp, rust, go, python
- Extract `file` field, validate extension matches language
  (cpp → .cpp or .zip, rust → .zip, go → .zip, python → .zip)
- If file > 50MB: return 413

STEP 2 — Generate IDs:
- Generate submission_id = "sub_" + UUID v7 (use github.com/google/uuid)
- Get contestant from context (set by auth middleware)

STEP 3 — Upload to MinIO:
- s3_key = "submissions/{contestant_id}/{submission_id}/{filename}"
- Stream file directly from multipart.File to MinIO using PutObject
- Do NOT buffer entire file in memory — use streaming
- Set ContentType based on file type
- If upload fails: return 500 with structured error

STEP 4 — Insert to Postgres:
- INSERT INTO submissions (id, contestant_id, contestant_name, language, s3_key, status)
- Return 500 if fails

STEP 5 — Publish to Kafka build-jobs topic:
- Produce a BuildJob message (JSON): { submission_id, s3_key, language, contestant_id }
- Use Kafka producer from services/submission-api/kafka/producer.go
  (implement this file too using segmentio/kafka-go)
- If Kafka publish fails: log error but still return 202 
  (build-worker will poll for PENDING submissions as fallback)

STEP 6 — Return 202:
{ "submission_id": "sub_xxx", "status": "pending", "message": "Build queued" }

Also implement GetSubmission and GetSubmissionLogs handlers:
- GetSubmission: SELECT from submissions WHERE id = $1, return full record as JSON, 404 if not found
- GetSubmissionLogs: SELECT error_log FROM submissions WHERE id = $1,
  return { "logs": "..." }, support streaming large logs via chunked transfer

FILE: services/submission-api/kafka/producer.go
Initialize segmentio/kafka-go producer with:
- Async: false (synchronous for reliability)
- RequiredAcks: RequireOne
- Compression: snappy
- WriteTimeout: 5 * time.Second
- MaxAttempts: 3
- Expose a Produce(topic, key, value []byte) error method

Output complete, compilable Go code.
```

---

### PROMPT 9 — Submission API: Test Handler + Leaderboard Handler

```
Implement remaining handlers for services/submission-api.

FILE: services/submission-api/handlers/test.go

Implement CreateTest(w http.ResponseWriter, r *http.Request):

Request body:
{
  "submission_id": "sub_xxx",
  "duration_seconds": 300,    // optional, default 300
  "bot_count": 500,           // optional, default 500
  "bot_personas": ["market_maker", "aggressive_taker"]  // optional
}

Logic:
1. Validate submission exists and belongs to requesting contestant
2. Check submission.status == "ready" — if not, return 409 with reason
3. Check no test currently RUNNING for this contestant (SELECT from tests WHERE
   contestant_id = $1 AND status IN ('pending','running','stopping'))
   If one exists: return 409 { "error": "test already running", "test_id": "..." }
4. Generate test_id = "test_" + UUID v7
5. INSERT into tests table with status=pending
6. Publish StartTest event to orchestrator-events Kafka topic:
   {
     "event": "START_TEST",
     "test_id": "test_xxx",
     "contestant_id": "...",
     "target_ip": submission.container_ip,
     "target_port": submission.container_port,
     "duration_seconds": 300,
     "bot_count": 500,
     "bot_personas": ["market_maker", "aggressive_taker", "spammer", "whale"]
   }
7. Return 202 { "test_id": "test_xxx", "status": "pending" }

Implement GetTest(w http.ResponseWriter, r *http.Request):
- Fetch from tests JOIN submissions WHERE tests.id = $1
- Also fetch latest metrics from Redis:
  GET metrics:{contestant_id}:p99_latency
  GET metrics:{contestant_id}:tps
  GET metrics:{contestant_id}:correctness_rate
- Merge into response: return test record + live_metrics object
- 404 if not found

FILE: services/submission-api/handlers/leaderboard.go

Implement GetLeaderboard(w http.ResponseWriter, r *http.Request):
- No auth required
- Read all contestant IDs from Redis SET `leaderboard:active_contestants`
- For each contestant, read from Redis HGETALL `metrics:{contestant_id}`
  which contains: p50, p90, p99, tps, correctness_rate, contestant_name
- Compute composite scores
- Sort by score descending
- Assign ranks (handle ties: same score = same rank, next rank skips)
- Return: { "updated_at": epoch_ms, "entries": [ ...LeaderboardEntry ] }
- Cache result in Redis for 500ms (key: leaderboard:cached, short TTL)
  so concurrent requests don't all recompute

FILE: services/submission-api/handlers/health.go
Return: { "status": "ok", "version": "1.0.0", "environment": "dev",
          "uptime_seconds": 123, "kafka": "ok", "redis": "ok", "db": "ok" }
Actually ping Kafka, Redis, and Postgres in parallel (goroutines + WaitGroup).
If any dependency is unhealthy: return 503 with the failed component named.

Output complete, compilable Go code.
```

---

### PROMPT 10 — Submission API: Repository Layer + Error Handling

```
Implement the data access layer and error handling for services/submission-api.

FILE: services/submission-api/repository/submission_repo.go

Define interface SubmissionRepository with methods:
- Create(ctx, *Submission) error
- GetByID(ctx, id string) (*Submission, error)
- GetByContestantID(ctx, contestantID string) ([]*Submission, error)
- UpdateStatus(ctx, id, status, errorLog string) error
- UpdateContainerInfo(ctx, id, containerIP string, containerPort int, containerID string) error

Implement PostgresSubmissionRepository using pgx/v5 connection pool.
Use prepared statements for all queries.
Implement proper error wrapping: wrap pgx.ErrNoRows as ErrNotFound,
other errors with fmt.Errorf("submissionRepo.Create: %w", err).
Use pgx batch queries for GetByContestantID if fetching multiple.

FILE: services/submission-api/repository/test_repo.go

Define interface TestRepository with methods:
- Create(ctx, *Test) error
- GetByID(ctx, id string) (*Test, error)
- GetActiveByContestantID(ctx, contestantID string) (*Test, error)
- UpdateStatus(ctx, id, status string) error
- UpdateFinalScore(ctx, id string, score float64, endedAt time.Time) error
- SetHeartbeat(ctx, id string) error

Implement PostgresTestRepository.

FILE: services/submission-api/repository/contestant_repo.go

Define interface ContestantRepository:
- GetByAPIKey(ctx, apiKey string) (*Contestant, error)
- GetByID(ctx, id string) (*Contestant, error)

Implement with pgx. Cache results in Redis for 60 seconds
(key: contestant_cache:{api_key}) to avoid DB hit on every request.

FILE: services/submission-api/apierrors/errors.go

Define custom error types:
- ErrNotFound { Message string }
- ErrConflict { Message string }
- ErrValidation { Field, Message string }
- ErrUnauthorized { Message string }
- ErrRateLimit { RetryAfter int }

Write a WriteError(w, err) function that maps these to correct HTTP status codes
and writes JSON error responses in a consistent format:
{ "error": { "code": "NOT_FOUND", "message": "...", "details": {} } }

�� SMART: Use pgx's built-in row scanning with pgx.RowToStructByName for all
SELECT queries — eliminates manual column mapping entirely, reduces bugs by ~90%
in the data layer.

Output complete, compilable Go code.
```

---

### PROMPT 11 — Submission API: Dockerfile + Integration Test

```
Create deployment and testing files for services/submission-api.

FILE: services/submission-api/Dockerfile
Multi-stage build:
Stage 1 (builder): golang:1.22-alpine
- Install: git, ca-certificates
- Copy go.mod, go.sum, download dependencies
- Copy source, build with CGO_DISABLED=0 GOOS=linux -ldflags="-s -w"
  Output binary: /app/submission-api

Stage 2 (runtime): gcr.io/distroless/static:nonroot
- Copy binary from builder
- Copy migrations/ directory
- EXPOSE 8080
- USER nonroot:nonroot
- ENTRYPOINT ["/app/submission-api"]

Resulting image should be < 20MB.

FILE: services/submission-api/submission_api_test.go
Integration tests using Go's testing package + testcontainers-go.

Test setup (TestMain):
- Start Postgres container (postgres:16-alpine)
- Start Redis container (redis:7.2-alpine)  
- Start MinIO container (minio/minio:latest)
- Start mock Kafka (using testcontainers redpanda image — faster than real Kafka)
- Run migrations
- Seed test contestant

Test cases:
1. TestCreateSubmission_Success
   - POST multipart with valid .zip + language=cpp
   - Assert 202 response
   - Assert submission in DB with status=pending
   - Assert MinIO object exists
   - Assert Kafka has message on build-jobs topic

2. TestCreateSubmission_FileTooLarge
   - POST with 51MB file
   - Assert 413 response
   - Assert no DB record created

3. TestCreateSubmission_InvalidLanguage
   - POST with language=java
   - Assert 400 response with validation error

4. TestRateLimit
   - POST 11 submissions in rapid succession
   - Assert first 10 succeed (202)
   - Assert 11th gets 429 Too Many Requests

5. TestGetSubmission_NotFound
   - GET /v1/submissions/nonexistent
   - Assert 404 with proper error JSON

6. TestGetSubmission_WrongContestant
   - Contestant A creates submission
   - Contestant B tries to GET it
   - Assert 404 (not 403 — don't leak existence)

7. TestHealth_AllHealthy
   - GET /v1/health
   - Assert 200 with all components ok

Use table-driven tests where possible. Add subtests with t.Run().
Output complete, compilable Go test code.
```

---

### PROMPT 12 — Build Worker: Main + Kafka Consumer

```
Implement the build-worker service in services/build-worker/.

This service watches for new build jobs and compiles contestant code into sandboxed containers.

FILE: services/build-worker/main.go
- Config struct: KafkaBrokers, BuildJobsTopic, ConsumerGroupID (default "build-workers"),
  OrchestratorDBDSN, MinIOEndpoint/Key/Secret/Bucket, DockerHost (default "unix:///var/run/docker.sock"),
  WorkDir (default "/tmp/builds"), MaxConcurrentBuilds int (default 3)
- Initialize: Kafka consumer group, Postgres pool, MinIO client, Docker client
- Start a semaphore-controlled worker pool (MaxConcurrentBuilds parallel builds)
- Graceful shutdown on SIGTERM

FILE: services/build-worker/consumer.go
Kafka consumer using segmentio/kafka-go ConsumerGroup:
- Consumer group ID: "build-workers" (multiple replicas = automatic partition assignment)
- On each message from build-jobs topic:
  1. Unmarshal BuildJob JSON
  2. Acquire semaphore slot (channel-based semaphore for MaxConcurrentBuilds limit)
  3. Launch goroutine: go worker.ProcessBuild(ctx, job)
  4. Commit offset only after ProcessBuild returns (manual commit for at-least-once delivery)
  
�� SMART: Use Kafka consumer group with manual commit (NOT auto-commit).
If the build-worker crashes mid-build, the job gets reprocessed by another worker instance.
Auto-commit would mark the job as done before the build actually completes, silently dropping builds.

FILE: services/build-worker/worker.go
ProcessBuild(ctx, job BuildJob) does the full build pipeline:

STEP 1: Update submission status to "building" in Postgres

STEP 2: Create temp directory: {WorkDir}/{submission_id}/
  - Defer cleanup: os.RemoveAll on completion

STEP 3: Download from MinIO:
  - Stream to: {WorkDir}/{submission_id}/source.zip
  - Unzip into {WorkDir}/{submission_id}/src/

STEP 4: Build (with 120s timeout context):
  Call buildContainer(ctx, submission_id, language, srcDir) → (imageName, error)
  This is the critical function — implement it in build_strategies.go

STEP 5: On build failure:
  - Read build logs from the build container's stdout/stderr (max 10KB)
  - Update submission: status=failed, error_log=logs
  - Commit Kafka offset and return

STEP 6: Launch sandbox container (implement in sandbox.go)
  Call launchSandbox(ctx, imageName, submissionID) → (containerID, ip, port, error)

STEP 7: Wait for container health check (30s timeout, probe every 2s):
  GET http://{container_ip}:{container_port}/health → 200 expected
  If not healthy in 30s: kill container, status=failed, error_log="container did not become healthy"

STEP 8: Update submission: status=ready, container_ip, container_port, container_id
  Register in Redis: HSET container:{submission_id} ip {ip} port {port} status ready

STEP 9: Publish ContainerReady event to orchestrator-events topic

Output complete, compilable Go code for all three files.
```

---

### PROMPT 13 — Build Worker: Build Strategies + Sandbox Launcher

```
Implement the actual Docker build and sandbox logic for services/build-worker.

FILE: services/build-worker/build_strategies.go

buildContainer(ctx, submissionID, language, srcDir string) (imageName string, buildLogs string, err error)

This function builds a Docker image from the contestant's source code.
Use Docker SDK (github.com/docker/docker/client) to communicate with Docker daemon.

The approach: write a Dockerfile dynamically based on language, then call Docker BuildKit.

For each language, generate a Dockerfile string:

CPP Dockerfile:
```dockerfile
FROM gcc:13-alpine AS builder
WORKDIR /src
COPY src/ .
RUN timeout 120 g++ -O2 -std=c++17 -o /orderbook main.cpp 2>&1

FROM alpine:3.19
RUN adduser -D -u 1001 contestant
WORKDIR /app
COPY --from=builder /orderbook /app/orderbook
COPY --chown=contestant:contestant entrypoint.sh /app/
RUN chmod +x /app/entrypoint.sh
USER contestant
EXPOSE 8080
HEALTHCHECK --interval=2s --timeout=3s --retries=15 CMD wget -q -O- http://localhost:8080/health || exit 1
ENTRYPOINT ["/app/entrypoint.sh"]
```

RUST Dockerfile:
```dockerfile
FROM rust:1.77-alpine AS builder
RUN apk add --no-cache musl-dev
WORKDIR /src
COPY src/ .
RUN timeout 180 cargo build --release 2>&1
RUN cp target/release/$(ls target/release | grep -v '\.' | head -1) /orderbook

FROM alpine:3.19
RUN adduser -D -u 1001 contestant
WORKDIR /app
COPY --from=builder /orderbook /app/orderbook
USER contestant
EXPOSE 8080
HEALTHCHECK --interval=2s --timeout=3s --retries=15 CMD wget -q -O- http://localhost:8080/health || exit 1
ENTRYPOINT ["/app/orderbook"]
```

GO Dockerfile (similar pattern, `go build -o /orderbook ./...`)

PYTHON Dockerfile (uses python:3.12-alpine, installs from requirements.txt if present,
runs with `python main.py`)

Write the Dockerfile to srcDir/Dockerfile, then:
- Use Docker BuildKit API to build image
- Tag as: trade-eval-contestant:{submissionID}
- Stream build logs in real time
- Return logs and image name on success

FILE: services/build-worker/sandbox.go

launchSandbox(ctx, imageName, submissionID string) (containerID, ip string, port int, err error)

Use Docker SDK to create and start a container with these EXACT security settings:

ContainerConfig:
  - Image: imageName
  - ExposedPorts: { "8080/tcp": {} }
  - Labels: { "trade-eval": "contestant", "submission-id": submissionID }

HostConfig:
  - Memory: 512 * 1024 * 1024 (512MB hard limit)
  - MemorySwap: 512 * 1024 * 1024 (no swap)
  - CPUSetCPUs: "2,3" (pin to dedicated cores, read from env SANDBOX_CPU_CORES)
  - CPUQuota: 100000 (1 CPU, 100ms per 100ms period)
  - NetworkMode: "contestant-isolated" (pre-created isolated network)
  - ReadonlyRootfs: true
  - Tmpfs: { "/tmp": "rw,noexec,nosuid,size=64m" }
  - SecurityOpt:
    - "no-new-privileges:true"
    - "seccomp=./seccomp/contestant-profile.json"  ← reference to seccomp file
  - CapDrop: ["ALL"]  ← drop ALL Linux capabilities
  - CapAdd: []        ← add NONE back
  - PidMode: (leave default but set PidsLimit: 50)
  - AutoRemove: false (we manage lifecycle)
  - RestartPolicy: { Name: "no" }
  - PortBindings: { "8080/tcp": [{ HostPort: "" }] } ← OS assigns port

After container starts:
- Inspect container to get assigned host port (from PortBindings)
- Inspect container to get container IP on contestant-isolated network
- Return containerID, IP, port

FILE: infra/docker/contestant/seccomp/contestant-profile.json
Create a seccomp profile that:
- Sets defaultAction: SCMP_ACT_ERRNO (block by default)
- Allows ONLY these syscalls (the minimal set needed for a network server):
  read, write, open, close, stat, fstat, lstat, poll, lseek, mmap, mprotect,
  munmap, brk, rt_sigaction, rt_sigprocmask, rt_sigreturn, ioctl, access,
  pipe, select, sched_yield, mremap, msync, mincore, madvise, shmget, shmat,
  shmctl, dup, dup2, pause, nanosleep, getitimer, alarm, setitimer, getpid,
  sendfile, socket, connect, accept, sendto, recvfrom, sendmsg, recvmsg,
  shutdown, bind, listen, getsockname, getpeername, socketpair, setsockopt,
  getsockopt, clone, execve, exit, wait4, kill, uname, fcntl, flock, fsync,
  fdatasync, truncate, ftruncate, getdents, getcwd, rename, mkdir, rmdir,
  unlink, readlink, chmod, fchmod, chown, fchown, umask, gettimeofday,
  getrlimit, getrusage, sysinfo, times, getuid, getgid, getgroups,
  geteuid, getegid, setuid, setgid, sigaltstack, utime, futex,
  sched_getaffinity, set_thread_area, get_thread_area, epoll_create,
  epoll_ctl, epoll_wait, set_tid_address, restart_syscall, clock_gettime,
  clock_getres, clock_nanosleep, exit_group, epoll_wait, tgkill, utimes,
  openat, getdents64, newfstatat, pread64, pwrite64

Output complete code for all files. The seccomp profile must be valid JSON.
```

---

### PROMPT 14 — Build Worker: Container Lifecycle Management + Cleanup

```
Implement container lifecycle management for services/build-worker.

FILE: services/build-worker/container_manager.go

ContainerManager struct manages the lifecycle of all contestant sandbox containers.
It should:
1. Track all running containers in a map[submissionID]ContainerInfo
   (protected by sync.RWMutex)
2. Start a background goroutine that checks container health every 30s
3. If a container is unhealthy (health check fails 3 times in a row): mark it dead
   in Redis, update submission status in Postgres, log event
4. Provide StopContainer(submissionID) that:
   - Sends SIGTERM to container, waits 10s for graceful exit
   - Then SIGKILL if still running
   - Removes the container
   - Removes the image (to free disk space)
   - Updates Redis: DEL container:{submissionID}
   - Updates Postgres: submission status = "stopped"
5. Provide StopAll() for graceful shutdown — stops all containers, blocks until done

FILE: services/build-worker/cleanup_job.go
Background cleanup goroutine that runs every 5 minutes:
- Lists all Docker containers with label "trade-eval=contestant"
- For each container, checks if it's in the ContainerManager's active map
- If a container exists in Docker but NOT in the manager (orphan): stop and remove it
  This handles the case where the build-worker crashed and restarted without cleaning up
- Log how many orphaned containers were cleaned up
- Also: check disk usage of Docker images. If > 10GB of contestant images, remove
  the oldest ones (by created timestamp) until < 5GB (prune strategy)

FILE: services/build-worker/build_worker_test.go
Unit tests:

TestBuildContainer_CPP_Success:
- Create a temp dir with a minimal valid C++ HTTP server (main.cpp that responds to /health and /order)
- Call buildContainer with language=cpp
- Assert no error
- Assert returned image name follows naming convention
- Clean up image after test

TestBuildContainer_CPP_CompileError:
- Create temp dir with invalid C++ (syntax error)
- Call buildContainer
- Assert error returned
- Assert error message contains compiler output

TestLaunchSandbox_Security:
- Launch a sandbox container from a known test image
- Inspect the launched container
- Assert ReadonlyRootfs is true
- Assert CapDrop includes ALL
- Assert NetworkMode is contestant-isolated
- Assert Memory <= 512MB
- Stop container after assertions

�� SMART: The orphan cleanup in cleanup_job.go is critical. Without it, if the
build-worker restarts repeatedly (e.g., OOMKilled by Kubernetes), you accumulate
ghost containers eating CPU and RAM. This is a production-breaker that most
implementations miss entirely. The 5-minute cleanup cycle catches this.

Output complete, compilable Go code.
```

---

## PHASE 2 — ORCHESTRATOR SERVICE (Prompts 15–20)

---

### PROMPT 15 — Orchestrator: Core State Machine

```
Implement the core of services/orchestrator/.

The Orchestrator is the brain: it manages the test lifecycle state machine.

FILE: services/orchestrator/main.go
Config: KafkaBrokers, OrchestratorEventsTopic, OrchestratorConsumerGroup,
OrchestratorDBDSN, RedisURL, OrchestratorInstanceID (auto-generated UUID on startup),
HeartbeatIntervalSeconds (default 10), OrphanDetectionIntervalSeconds (default 60)

Initialize: Kafka consumer + producer, Postgres, Redis
Start goroutines:
1. Kafka consumer loop (consumeEvents)
2. Heartbeat writer (writeHeartbeats)
3. Orphan detector (detectOrphanedTests)

FILE: services/orchestrator/state_machine.go

TestState is a string enum: pending, running, stopping, completed, failed

TestStateMachine struct:
- db: TestRepository
- redis: RedisClient
- kafka: KafkaProducer
- instanceID: string (this orchestrator's ID for crash recovery)

Methods:
1. TransitionTo(ctx, testID, newState string, reason string) error
   - Validates transition is legal:
     Legal transitions: pending→running, running→stopping, stopping→completed,
     running→failed, pending→failed, stopping→failed
   - Updates DB with: UPDATE tests SET status=$1, updated_at=NOW(), 
     orchestrator_instance_id=$2 WHERE id=$3 AND status=$4 (optimistic lock)
   - If DB UPDATE affects 0 rows: return ErrConcurrentModification
     (another orchestrator instance beat us — this test was orphaned+recovered)
   - Updates Redis: HSET test:{testID} status {state}
   - Emits state change event to Kafka

2. StartTest(ctx, event StartTestEvent) error
   - Acquire distributed lock: SET lock:test:{contestantID} {testID} NX EX 330
     (330 = test duration 300s + 30s buffer)
   - If lock not acquired: another test is running for this contestant
   - TransitionTo(pending → running)
   - Write to Redis: HSET test:{testID} started_at {ns} target_ip {ip} target_port {port}
   - Publish START_TEST to bot-fleet workers via Kafka
   - Schedule STOP_TEST after duration: use time.AfterFunc(duration, func() { StopTest(...) })
     �� SMART: Store the timer in a map[testID]*time.Timer so it can be cancelled
     if the test fails early. This prevents double-stop events.
   - Write heartbeat: SETEX orchestrator_heartbeat:{testID} 30 {instanceID}

3. StopTest(ctx, testID, reason string) error
   - Cancel any pending timer for this test
   - TransitionTo(running → stopping)
   - Publish STOP_TEST to orchestrator-events topic
   - Wait 10s for bots to drain (give telemetry a chance to flush)
   - Collect final metrics from Redis: HGETALL metrics:{contestantID}
   - Compute composite score
   - TransitionTo(stopping → completed)
   - Write final score to Postgres test_summaries
   - Release distributed lock: DEL lock:test:{contestantID}

4. FailTest(ctx, testID, reason string) error
   - Cancel any pending timer
   - TransitionTo(* → failed)
   - Set failure_reason in DB
   - Release lock
   - Publish STOP_TEST so bots know to stop

Output complete, compilable Go code.
```

---

### PROMPT 16 — Orchestrator: Event Consumer + Crash Recovery

```
Implement the Kafka consumer and crash recovery for services/orchestrator.

FILE: services/orchestrator/consumer.go

consumeEvents(ctx) — Kafka consumer loop reading from orchestrator-events topic.
Consumer group: "orchestrators" (only ONE orchestrator processes each event).

Handle these event types (dispatch by "event" field in JSON):

"CONTAINER_READY":
  - Parse BuildResult event from build-worker
  - Check if auto-test is enabled (env AUTO_TRIGGER_TESTS=true)
  - If yes: automatically call state_machine.StartTest with default parameters
  - If no: just log "container ready, waiting for manual trigger"

"START_TEST" (from submission-api):
  - Parse StartTestEvent
  - Call state_machine.StartTest(ctx, event)
  - On ErrConcurrentModification: log warning, do nothing (another instance handled it)
  - On other error: publish FAIL_TEST event back

"STOP_TEST" (manual or from timer expiry):
  - Call state_machine.StopTest(ctx, testID, reason)

"CONTAINER_CRASHED" (from build-worker health monitor):
  - If test is running for this contestant: call state_machine.FailTest
  - Update submission status in DB via submission-api OR direct Postgres write

"HEARTBEAT_CHECK" (self-published every 60s for orphan detection):
  - Handled by detectOrphanedTests instead

FILE: services/orchestrator/crash_recovery.go

detectOrphanedTests(ctx) — runs every 60 seconds.

Logic:
1. Query Postgres: SELECT * FROM tests WHERE status IN ('running','stopping')
   AND last_heartbeat_at < NOW() - INTERVAL '60 seconds'
   (tests whose orchestrator stopped sending heartbeats — the orchestrator crashed)

2. For each orphaned test:
   a. Try to acquire lock: SET lock:recover:{testID} {instanceID} NX EX 30
      If fails: another orchestrator is already recovering this test
   b. Log "recovering orphaned test {testID} (last heartbeat: {time})"
   c. Check if contestant container is still healthy (HTTP probe)
   d. If container healthy: re-register the test timer with remaining duration
      (compute: remaining = started_at + duration_seconds - NOW())
      If remaining > 0: restart test timer
      If remaining <= 0: immediately stop the test
   e. If container dead: FailTest with reason "container_unavailable_after_recovery"
   f. Update orchestrator_instance_id = current instance
   g. Release recovery lock

writeHeartbeats(ctx) — runs every 10 seconds:
  For each test in our local running_tests map:
    UPDATE tests SET last_heartbeat_at = NOW() WHERE id = $1 AND orchestrator_instance_id = $2
    Also: SETEX orchestrator_heartbeat:{testID} 30 {instanceID}

FILE: services/orchestrator/scorer.go

computeCompositeScore(metrics LatencyWindow, allContestantMetrics []LatencyWindow) float64:

1. Collect all p99 values and tps values across contestants
2. Normalize current contestant's tps: normalized_tps = (tps - min_tps) / (max_tps - min_tps)
   Handle edge case: if max == min, normalized = 1.0 (everyone tied)
3. Normalize p99 inversely: normalized_inv_p99 = 1 - ((p99 - min_p99) / (max_p99 - min_p99))
   Lower p99 = higher score
4. Correctness is already 0-1
5. score = 0.40*normalized_tps + 0.40*normalized_inv_p99 + 0.20*correctness_rate
6. Clamp to [0.0, 1.0] * 100 (percentage)
7. Round to 2 decimal places

�� SMART: The crash recovery system turns a potentially catastrophic failure
(orchestrator pod restarts in Kubernetes during a contest) into a seamless 60-second
blip. Without it, every orchestrator crash permanently corrupts the test state for
all currently-running tests. This is the feature that separates a reliable platform
from a fragile one.

Output complete, compilable Go code.
```

---

### PROMPT 17 — Orchestrator: Scoring Publisher + Metrics Writer

```
Implement the metrics writing and scoring components for services/orchestrator.

FILE: services/orchestrator/metrics_writer.go

MetricsWriter struct — responsible for writing final test metrics to Redis for leaderboard.

Methods:

1. PublishLiveMetrics(ctx, contestantID string, window LatencyWindow) error
   Called by orchestrator every 5 seconds during a test to publish intermediate results.
   Writes to Redis using HSET (atomic multi-field set):
   HSET metrics:{contestantID}
     p50_latency_us   {value}
     p90_latency_us   {value}
     p99_latency_us   {value}
     tps              {value}
     correctness_rate {value}
     last_updated_ns  {nanos}
     contestant_name  {name}
     test_status      running
   Also: SADD leaderboard:active_contestants {contestantID}

2. PublishFinalScore(ctx, contestantID, testID string, score float64, summary TestSummary) error
   Write final metrics same as above, but set test_status = completed and add composite_score.
   Also write to TimescaleDB test_summaries table.

3. ClearMetrics(ctx, contestantID string) error
   Called when a new test starts to clear stale metrics from previous run.
   DEL metrics:{contestantID} — forces fresh start.
   Note: Do NOT remove from leaderboard:active_contestants yet (remove only after
   successful test completion or permanent disqualification).

FILE: services/orchestrator/orchestrator_test.go

TestStateMachine_HappyPath:
- Create TestStateMachine with in-memory fakes (fake DB, fake Redis, fake Kafka producer)
- Call StartTest with a valid event
- Verify DB updated to "running"
- Verify Redis lock acquired
- Verify Kafka START_TEST published to bot-fleet topic
- Wait for timer (use shortened 2s duration in test)
- Verify StopTest called automatically
- Verify DB updated to "completed"
- Verify Redis lock released

TestStateMachine_ConcurrentStartFails:
- StartTest for contestant A
- Immediately StartTest again for same contestant
- Assert second call returns ErrConcurrentModification

TestCrashRecovery_OrphanedTest:
- Insert a test with status=running, last_heartbeat_at = 90 seconds ago (past 60s threshold)
- Call detectOrphanedTests
- Assert test is either re-registered or failed based on container health

TestCompositeScore_Normalization:
- Create 5 contestant metrics with varying TPS and p99 values
- Assert the highest-TPS contestant gets normalized_tps close to 1.0
- Assert the lowest-p99 contestant gets normalized_inv_p99 close to 1.0
- Assert scores sum/distribution makes sense

Use table-driven tests. Output complete, compilable Go code.
```

---

### PROMPT 18 — Orchestrator: Kubernetes Deployment Config

```
Create Kubernetes deployment manifests for the orchestrator service.

FILE: infra/k8s/orchestrator/deployment.yaml
apiVersion: apps/v1, kind: Deployment
name: orchestrator
namespace: trade-eval

Spec:
- replicas: 2 (for high availability — crash recovery handles the 2-instance case)
- strategy: RollingUpdate (maxUnavailable: 0, maxSurge: 1)
  Reason for maxUnavailable=0: we never want ZERO orchestrators running
- selector: matchLabels: app=orchestrator
- Template spec:
  Container:
    - name: orchestrator
    - image: ghcr.io/trade-eval/orchestrator:latest
    - resources:
        requests: { cpu: "100m", memory: "128Mi" }
        limits: { cpu: "500m", memory: "256Mi" }
    - env: (read from ConfigMap and Secrets)
    - livenessProbe: httpGet /healthz every 10s, failureThreshold 3
    - readinessProbe: httpGet /readyz every 5s, failureThreshold 2
    - lifecycle: preStop: exec sleep 15 (drain in-flight work before termination)

FILE: infra/k8s/orchestrator/service.yaml
ClusterIP service, port 8081, for internal health check access.

FILE: infra/k8s/orchestrator/configmap.yaml
All non-secret configuration as ConfigMap.

FILE: infra/k8s/orchestrator/hpa.yaml
HorizontalPodAutoscaler:
- minReplicas: 2, maxReplicas: 5
- Scale on: custom metric "active_tests_count" from Redis (via custom metrics adapter)
  OR: CPU utilization > 70%

FILE: infra/k8s/orchestrator/pdb.yaml
PodDisruptionBudget:
- minAvailable: 1
This ensures at least 1 orchestrator survives node drains/upgrades.

FILE: infra/k8s/network-policies/orchestrator-netpol.yaml
NetworkPolicy for orchestrator:
- ALLOW ingress: from namespace trade-eval (internal services)
- ALLOW egress: to kafka:9092, redis:6379, postgres:5433
- DENY all other egress (especially to contestant-isolated network)
  Reason: orchestrator MUST NOT be able to reach contestant containers directly

Also create infra/k8s/network-policies/contestant-netpol.yaml:
NetworkPolicy for contestant containers:
- ALLOW ingress: ONLY from bot-fleet namespace on port 8080
- DENY ALL other ingress
- DENY ALL egress (contestant code cannot call external services)
  This is the critical security isolation policy.

�� SMART: The NetworkPolicy on contestant containers (deny all egress) is the
final layer of defense against a clever contestant who exploits the running container
to exfiltrate your Kafka/Redis credentials. Even if seccomp and AppArmor fail,
the container literally cannot make outbound network calls.

Output complete Kubernetes YAML. No placeholder values — use real field names.
```

---

### PROMPT 19 — Orchestrator: Helm Chart

```
Create a Helm chart for the orchestrator service.

FILE: infra/helm/orchestrator/Chart.yaml
name: orchestrator
description: Trade evaluation platform orchestrator
version: 0.1.0
appVersion: "1.0.0"
dependencies:
  - none (infra services are managed separately)

FILE: infra/helm/orchestrator/values.yaml
Full default values:
replicaCount: 2
image:
  repository: ghcr.io/trade-eval/orchestrator
  pullPolicy: IfNotPresent
  tag: "latest"
service:
  type: ClusterIP
  port: 8081
resources:
  limits: { cpu: 500m, memory: 256Mi }
  requests: { cpu: 100m, memory: 128Mi }
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 5
  targetCPUUtilizationPercentage: 70
config:
  heartbeatIntervalSeconds: 10
  orphanDetectionIntervalSeconds: 60
  autoTriggerTests: false
  testDefaultDurationSeconds: 300
  testDefaultBotCount: 500
kafka:
  brokers: "kafka:9092"
  orchestratorEventsTopic: "orchestrator-events"
  botFleetTopic: "bot-fleet-commands"
redis:
  url: "redis:6379"
postgresql:
  host: postgres
  port: 5433
  database: orchestrator
  existingSecret: orchestrator-db-credentials

FILE: infra/helm/orchestrator/templates/deployment.yaml
Full deployment template using .Values references.
Include proper {{ include "orchestrator.fullname" . }} helper usage.

FILE: infra/helm/orchestrator/templates/hpa.yaml
HPA template, only rendered if .Values.autoscaling.enabled is true.

FILE: infra/helm/orchestrator/templates/configmap.yaml
ConfigMap template.

FILE: infra/helm/orchestrator/templates/service.yaml
Service template.

FILE: infra/helm/orchestrator/templates/_helpers.tpl
Standard Helm helpers: fullname, name, labels, selectorLabels, serviceAccountName.

Also create infra/helm/orchestrator/templates/NOTES.txt with useful post-install info:
- How to check orchestrator logs
- How to manually trigger a test via kubectl exec

Output complete, valid Helm chart YAML and templates.
```

---

### PROMPT 20 — Orchestrator: Integration + E2E Test

```
Create comprehensive integration tests for the orchestrator service.

FILE: services/orchestrator/integration_test.go

Use testcontainers-go to spin up real infrastructure.

TestFullTestLifecycle_Integration (requires docker):
Setup:
- Start Postgres container, run migrations
- Start Redis container
- Start Kafka container (redpanda image — starts in < 3s vs Kafka's 30s)
- Initialize Orchestrator with real dependencies
- Insert test contestant and submission (status=ready) into DB

Test:
1. Publish START_TEST event to Kafka
2. Start orchestrator consumer in a goroutine
3. Wait max 5 seconds for orchestrator to pick up START_TEST
4. Assert: test status in DB = "running"
5. Assert: Redis lock acquired (GET lock:test:{contestantID} returns non-nil)
6. Assert: Kafka has START_TEST message on bot-fleet-commands topic
7. Wait for test timer to fire (use 3s duration in test)
8. Assert: test status in DB = "completed"
9. Assert: Redis lock released
10. Assert: Redis metrics:{contestantID} has composite_score

TestCrashRecovery_Integration:
1. Insert running test with last_heartbeat_at = 2 minutes ago
2. Start orchestrator (it should detect orphan within 60s via detectOrphanedTests)
3. Mock the container health check to return healthy
4. Assert: test is re-registered (timer running, heartbeats updating)
5. OR if container mock returns unhealthy: assert test.status = "failed"

TestConcurrentTests_SameContestant:
1. Insert submission with status=ready for contestant A
2. Start orchestrator
3. Publish TWO START_TEST events for contestant A simultaneously
4. Wait 3 seconds
5. Assert: exactly ONE test has status=running
6. Assert: the second START_TEST was rejected with conflict

TestScoring_MultipleContestants:
1. Insert 5 contestants with mock metrics in Redis
2. Call computeCompositeScore for each
3. Assert scores are in [0, 100] range
4. Assert contestant with best TPS+latency has highest score
5. Assert scores sum and relative ordering is correct

FILE: scripts/local-dev/smoke-test.sh
A shell script for manual smoke testing on a running stack:
```bash
#!/bin/bash
set -e
BASE_URL=${BASE_URL:-http://localhost:8080}

echo "=== Trade Eval Platform Smoke Test ==="

# 1. Health check
echo "→ Health check..."
curl -sf $BASE_URL/v1/health | jq .

# 2. Create contestant (direct DB insert for smoke test)
CONTESTANT_ID=$(uuidgen | tr '[:upper:]' '[:lower:]')
API_KEY="test-key-$(openssl rand -hex 8)"
echo "→ Using contestant: $CONTESTANT_ID"

# 3. Upload sample C++ order book
echo "→ Uploading submission..."
RESULT=$(curl -sf -X POST $BASE_URL/v1/submissions \
  -H "X-API-Key: $API_KEY" \
  -F "file=@testdata/sample-orderbook.zip" \
  -F "language=cpp")
echo $RESULT | jq .
SUB_ID=$(echo $RESULT | jq -r .submission_id)

# 4. Poll for READY
echo "→ Waiting for build (60s timeout)..."
for i in $(seq 1 30); do
  STATUS=$(curl -sf $BASE_URL/v1/submissions/$SUB_ID -H "X-API-Key: $API_KEY" | jq -r .status)
  echo "  Status: $STATUS"
  [ "$STATUS" = "ready" ] && break
  [ "$STATUS" = "failed" ] && echo "BUILD FAILED" && exit 1
  sleep 2
done

# 5. Trigger test
echo "→ Starting test..."
TEST_RESULT=$(curl -sf -X POST $BASE_URL/v1/tests \
  -H "X-API-Key: $API_KEY" \
  -H "Content-Type: application/json" \
  -d "{\"submission_id\": \"$SUB_ID\", \"duration_seconds\": 30, \"bot_count\": 10}")
TEST_ID=$(echo $TEST_RESULT | jq -r .test_id)

# 6. Poll for completion
echo "→ Waiting for test completion (90s timeout)..."
for i in $(seq 1 45); do
  STATUS=$(curl -sf $BASE_URL/v1/tests/$TEST_ID -H "X-API-Key: $API_KEY" | jq -r .status)
  echo "  Status: $STATUS"
  [ "$STATUS" = "completed" ] && break
  [ "$STATUS" = "failed" ] && echo "TEST FAILED" && exit 1
  sleep 2
done

# 7. Check leaderboard
echo "→ Checking leaderboard..."
curl -sf $BASE_URL/v1/leaderboard | jq .

echo "=== SMOKE TEST PASSED ==="
```

Output all files with complete content.
```

---

## PHASE 3 — BOT FLEET SERVICE (Prompts 21–30)

---

### PROMPT 21 — Bot Fleet: Architecture + Main Service

```
Implement the core of services/bot-fleet/ — the distributed load generator.

This service spawns thousands of concurrent simulated trading bots.

FILE: services/bot-fleet/main.go
Config:
  KafkaBrokers, OrchestratorEventsTopic, TelemetryTopic, ConsumerGroupID (bot-fleet-workers),
  MaxBotsPerPod int (default 200), BotRequestTimeoutMs int (default 5000),
  BotHTTPPoolSize int (default 20, connections per bot's HTTP client),
  LogLevel, Environment

Initialize:
- Kafka consumer (reads START_TEST, STOP_TEST from orchestrator-events)
- Kafka producer (writes telemetry events to bot-telemetry topic)
- Test registry: map[testID]*TestRunner (protected by sync.RWMutex)
- HTTP client pool factory

FILE: services/bot-fleet/test_runner.go

TestRunner manages the lifecycle of one running test.

Fields:
  testID, contestantID, targetURL string
  botCount int
  personas []string
  duration time.Duration
  cancel context.CancelFunc  ← cancels all bot goroutines
  wg sync.WaitGroup          ← waits for all bots to finish
  producer KafkaProducer
  sequenceCounter atomic.Int64  ← global sequence number for ordering

Methods:
Start(ctx):
  - Create child context with cancel
  - Spawn bots: distribute personas evenly across botCount
    Example: 500 bots, personas=[market_maker, aggressive_taker, spammer, whale]
    → ~200 market_maker, ~150 aggressive_taker, ~100 spammer, ~50 whale
    (ratio defined in BotPersonaRatios config)
  - For each bot: go bot.Run(ctx, botID, persona, targetURL, producer)
  - Set timer for test duration: after duration, call Stop()

Stop(reason string):
  - Call cancel() to signal all bots to stop
  - wg.Wait() with 30s timeout (bots should stop within 10s normally)
  - Log how many bots completed, how many timed out
  - Emit BOTS_STOPPED event to Kafka with stats

FILE: services/bot-fleet/consumer.go
Kafka consumer for bot-fleet reading from orchestrator-events:

On START_TEST:
  - Parse event
  - If testID already in registry: ignore (duplicate event from Kafka)
  - Create TestRunner, call Start()
  - Register in test registry

On STOP_TEST:
  - Find TestRunner by testID in registry
  - If not found: ignore (not our test or already stopped)
  - Call Stop(reason)
  - Remove from registry

On CONTAINER_CRASHED:
  - Find all tests for this contestantID
  - Stop them with reason "container_crashed"

�� SMART: Each bot-fleet pod handles MaxBotsPerPod=200 bots. For 10,000 bots,
you need 50 pods. Kubernetes HPA auto-scales based on the custom metric
"active_bot_count" exposed via a Prometheus /metrics endpoint.
This is vastly more efficient than pre-allocating 50 pods — scale up on test start,
scale down when tests finish.

Output complete, compilable Go code.
```

---

### PROMPT 22 — Bot Fleet: Market Maker Bot

```
Implement the Market Maker bot persona for services/bot-fleet/bots/.

FILE: services/bot-fleet/bots/market_maker.go

MarketMakerBot simulates a professional market maker:
- Continuously posts bid/ask limit orders near the mid-price
- Cancels and re-posts when price moves or orders fill
- Realistic behavior: manages a two-sided quote

Implement Run(ctx, botID, targetURL string, producer KafkaProducer, wg *sync.WaitGroup):

State:
  activeBuyOrderID  string  (empty if no active buy)
  activeSellOrderID string
  currentMidPrice   float64 (start at 100.00, drifts over time)
  spreadBPS         float64 (basis points, start at 10 = 0.1%)
  orderSize         float64 (random 1-20)

Main loop (runs until ctx.Done()):

CYCLE (every 10ms ± 3ms jitter):

1. Drift mid-price: add small random walk (±0.05%)

2. Compute bid and ask:
   bid = midPrice * (1 - spreadBPS/20000)  // half-spread below mid
   ask = midPrice * (1 + spreadBPS/20000)  // half-spread above mid
   Round to 2 decimal places

3. If no active buy order: send LIMIT_BUY at bid for orderSize
   Record t1 = time.Now().UnixNano()
   POST {
     "order_id": generate UUID,
     "type": "LIMIT_BUY",
     "price": bid,
     "quantity": orderSize
   }
   Record t2 = time.Now().UnixNano() after response
   Emit telemetry event

4. If no active sell order: send LIMIT_SELL at ask for orderSize
   Same measurement pattern

5. If active buy has been outstanding > 200ms: send CANCEL for that order_id
   Measure cancel latency
   Clear activeBuyOrderID

6. If active sell has been outstanding > 200ms: send CANCEL for that order_id

7. Parse fill responses: if fill.status == "FILLED" or "PARTIAL":
   Update active order state accordingly

8. On timeout (no response within 5s): record as timed_out=true in telemetry

Telemetry emission: buildTelemetryEvent(botID, "market_maker", orderType, sentNs, ackedNs, 
  order, expectedFill, actualFill, correct) emits to Kafka bot-telemetry topic.
Serializes to JSON and uses contestantID as Kafka partition key (all events for one contestant
go to same partition, ensuring ordering in the telemetry ingester).

FILE: services/bot-fleet/bots/http_client.go
SharedHTTPClient:
- Uses http.Transport with:
  MaxIdleConns: 100
  MaxIdleConnsPerHost: 20
  IdleConnTimeout: 90s
  DisableCompression: true  (latency, not bandwidth, is what we measure)
  TLSHandshakeTimeout: 2s
- The SAME client is shared across all bots targeting the same contestant container
  (HTTP connection pool reuse is critical at 10,000 concurrent bots)
- DO NOT create a new http.Client per bot — this exhausts file descriptors and adds
  TCP handshake latency to measurements

Output complete, compilable Go code.
```

---

### PROMPT 23 — Bot Fleet: Aggressive Taker + Spammer + Whale Bots

```
Implement the remaining three bot personas for services/bot-fleet/bots/.

FILE: services/bot-fleet/bots/aggressive_taker.go

AggressiveTakerBot sends market orders that immediately cross the book:
- Market orders should always fill instantly if liquidity exists
- Tests the matching engine's speed on the critical path (match + fill)

Run loop (every 50ms ± 10ms jitter):
1. Alternate between MARKET_BUY and MARKET_SELL
2. Generate random quantity (10-100 shares)
3. Send: POST { "type": "MARKET_BUY", "quantity": qty }
   Note: NO price field — market orders take best available price
4. Expect response: fill with status FILLED immediately
5. If fill.status == PENDING or REJECTED: record as incorrect
   (market orders with available liquidity should never be PENDING)
6. Measure latency, emit telemetry

Key correctness check for aggressive taker:
The expected fill price = whatever the best ask/bid was AT TIME OF ORDER SEND.
Since we don't know the exact state of the order book, we validate CONSISTENCY instead:
  - fill.price must be a valid price (> 0)
  - fill.quantity must == requested quantity (full fill expected for reasonable sizes)
  - If fill.status == FILLED and fill.quantity < requested_quantity: partial fill
    (allowed but unexpected — record as "partial_fill_anomaly" not a hard failure)

FILE: services/bot-fleet/bots/spammer.go

SpammerBot sends limit orders then immediately cancels them (cancel latency tester):

Run loop (every 1ms ± 0.5ms — this is the FASTEST bot):
1. Generate a limit order far from mid-price (price = mid * 0.5, unlikely to fill)
2. Send LIMIT_BUY, record latency of the ACK
3. Immediately (< 1ms later) send CANCEL for that order_id
4. Record cancel latency
5. Emit TWO telemetry events: one for the order, one for the cancel

Correctness for cancel: response must include { "status": "cancelled", "order_id": "..." }
If the cancel response says status=FILLED (the order somehow filled in < 1ms):
  - Record as "racing_fill" — not an error, but flag it in telemetry
  - This can happen legitimately: a market order might have consumed our limit

FILE: services/bot-fleet/bots/whale.go

WhaleBob sends one massive order per cycle to test edge cases:

Run loop (every 500ms):
1. Send a LIMIT_BUY for 10,000 shares at a realistic price
   OR MARKET_BUY for 10,000 shares (alternating every 5 cycles)
2. For the limit version: expect either PARTIAL fill or PENDING (resting in book)
3. For the market version: expect PARTIAL fill (book rarely has 10,000 shares)
4. Validate partial fills: fill.quantity must be <= requested quantity AND >= 0
5. Check if remaining_quantity in response is correct: remaining = requested - filled
6. Emit telemetry with order_subtype: "whale_limit" or "whale_market"

FILE: services/bot-fleet/bots/base_bot.go
Shared bot utilities:
- generateOrderID() string: "ord_" + 8 random hex chars
- measureAndEmit(botID, persona, orderType, t1, t2 int64, order, fill, correct bool, producer)
  Builds and emits the telemetry JSON event
- jitter(baseDuration time.Duration, pct float64) time.Duration:
  Returns baseDuration ± pct% random jitter
  Usage: time.Sleep(jitter(10*time.Millisecond, 0.3))

�� SMART: The SpammerBot running at 1ms intervals is the most stressful persona.
1,000 spammer bots × 1000 events/sec = 1M events/sec to the telemetry topic alone.
This is realistic — real HFT cancel rates are 95%+ of order volume. Your Kafka setup
MUST have bot-telemetry configured with 16+ partitions to handle this.

Output complete, compilable Go code for all four files.
```

---

### PROMPT 24 — Bot Fleet: Shadow Order Book (Client-Side State)

```
Implement the client-side shadow order book in services/bot-fleet/bots/shadow_book.go.

This is NOT the full reference validator (that's in the telemetry-ingester).
This is a lightweight LOCAL order book state tracker used by each bot to:
1. Know what fills to EXPECT from the contestant container
2. Detect obvious correctness violations immediately

FILE: services/bot-fleet/bots/shadow_book.go

ShadowBook struct — maintains a simplified local order book state per bot.

Fields:
  bids  []PriceLevel  // sorted descending (best bid first)
  asks  []PriceLevel  // sorted ascending (best ask first)
  mu    sync.Mutex    // protect concurrent access

PriceLevel struct:
  Price    float64
  Quantity float64
  OrderID  string
  Time     time.Time

Methods:

AddOrder(order Order) ExpectedFill:
  Determines what fill SHOULD happen when this order is submitted.
  
  For MARKET_BUY:
    Walk asks from cheapest to most expensive
    Fill as many as possible up to order quantity
    Return ExpectedFill with matched price levels
    
  For MARKET_SELL:
    Walk bids from highest to lowest
    Fill against bids
    
  For LIMIT_BUY at price P:
    Check if any ask.Price <= P
    If yes: fill against those asks (price-time priority)
    If no: order rests in book, ExpectedFill.Status = PENDING
    
  For LIMIT_SELL at price P:
    Check if any bid.Price >= P
    If yes: fill against those bids
    If no: rests, ExpectedFill.Status = PENDING
    
  For CANCEL at orderID:
    Find and remove order from bids or asks
    ExpectedFill.Status = CANCELLED

ValidateFill(expected, actual Fill) (correct bool, reason string):
  - If expected.Status != actual.Status: incorrect, return reason
  - If expected.Status == FILLED and actual.Price != expected.Price: incorrect
    (price must match for limit orders — market orders can vary)
  - If actual.Quantity > expected.Quantity: incorrect (overfill impossible)
  - If actual.Quantity < 0: incorrect
  - All else: correct

IMPORTANT: The shadow book in the bot is NOT the authoritative correctness validator.
It only catches OBVIOUS bugs (overfills, wrong prices on limit orders). The Telemetry
Ingester has the authoritative shadow book with full cross-bot ordering and sequence
number reconciliation. The bot-side book uses single-bot state only.

FILE: services/bot-fleet/bots/shadow_book_test.go
Tests:
- TestLimitBuyFilledByAsk: add a sell at $100, then buy limit at $100 → expect FILLED at $100
- TestLimitBuyRests: add a buy at $99 with no matching sell → expect PENDING
- TestMarketBuyFullFill: adds asks at 100,101,102 for 10 shares each, MARKET_BUY 25 → expect fill 10+10+5 at weighted average
- TestCancel: add order, cancel it, verify it's removed from book
- TestPriceTimePriority: two sells at same price, buy fills the earlier one first

Output complete, compilable Go code.
```

---

### PROMPT 25 — Bot Fleet: Telemetry Kafka Producer + Batching

```
Implement the high-performance Kafka telemetry producer for services/bot-fleet.

FILE: services/bot-fleet/telemetry/producer.go

This producer handles up to 1,000,000 events/second. Performance is critical.

TelemetryProducer struct with:
  writer    *kafka.Writer
  batchCh   chan TelemetryEvent  (buffered, size 10,000)
  flushTick *time.Ticker (every 100ms)
  mu        sync.Mutex
  dropped   atomic.Int64  (track dropped events for observability)

Initialize with kafka.Writer:
  Addr: kafka.TCP(brokers...),
  Topic: telemetryTopic,
  Balancer: &kafka.Hash{},  ← partition by key (contestant_id ensures ordering)
  BatchSize: 1000,           ← collect up to 1000 messages per batch
  BatchTimeout: 10ms,        ← or flush after 10ms, whichever comes first
  Compression: kafka.Snappy, ← Snappy: fast compression, ~40% size reduction
  Async: true,               ← NON-BLOCKING: bots should never block on Kafka
  ErrorLogger: ...,
  RequiredAcks: kafka.RequireOne,  ← only leader needs to ack (fast)
  WriteTimeout: 5 * time.Second,
  ReadTimeout: 5 * time.Second,

Emit(event TelemetryEvent) bool:
  Marshal event to JSON
  Try to send to batchCh channel:
    select {
    case batchCh <- event: return true
    default:
      atomic.Add(&dropped, 1)
      return false  ← NEVER BLOCK: drop event if channel full
    }
  Reason: if Kafka is slow, bots must continue running. Data quality degrades
  gracefully rather than the entire test halting.

Background writer goroutine:
  Reads up to 1000 events from batchCh per iteration
  Writes as kafka.Message batch with Key=[]byte(event.ContestantID)
  Logs Kafka write errors but doesn't panic

DroppedCount() int64: returns atomic.Load(&dropped)
Expose /metrics Prometheus endpoint with:
  bot_fleet_telemetry_events_dropped_total (counter)
  bot_fleet_telemetry_events_sent_total (counter)
  bot_fleet_active_bots (gauge)
  bot_fleet_requests_per_second (gauge, updated every 5s)

FILE: services/bot-fleet/telemetry/event.go
TelemetryEvent struct matching the Protobuf definition from proto/telemetry.proto,
but as a pure Go struct with JSON tags.
Include a toJSON() []byte method with a reusable json.Encoder (pool it with sync.Pool
to avoid repeated allocations at 1M events/sec).

�� SMART: sync.Pool for JSON encoders is a significant win at 1M events/sec.
Without it: 1M allocations/sec → 1M GC pressure events. With sync.Pool:
most encoders are reused, GC pressure drops ~80%. This is the difference between
a service that runs clean and one that GC-pauses every few seconds under load.

Output complete, compilable Go code.
```

---

### PROMPT 26 — Bot Fleet: HTTP Endpoints for Contestant Containers

```
Implement the HTTP client logic for calling contestant containers from services/bot-fleet.

FILE: services/bot-fleet/client/orderbook_client.go

OrderBookClient is the interface between bots and the contestant's HTTP server.

Define the API contract that contestant containers MUST implement:
(also document this in docs/contestant-api.md)

POST /order
Request: { "order_id": "ord_xxx", "type": "LIMIT_BUY"|"LIMIT_SELL"|"MARKET_BUY"|"MARKET_SELL", "price": 100.50, "quantity": 10 }
Response: { "order_id": "ord_xxx", "status": "FILLED"|"PARTIAL"|"PENDING"|"REJECTED", "filled_price": 100.50, "filled_quantity": 10, "remaining_quantity": 0 }

POST /cancel
Request: { "order_id": "ord_xxx" }
Response: { "order_id": "ord_xxx", "status": "CANCELLED"|"NOT_FOUND"|"ALREADY_FILLED" }

GET /health
Response: { "status": "ok" }

GET /orderbook (optional for debugging)
Response: { "bids": [ {"price": 100.0, "quantity": 10} ], "asks": [ ... ] }

Implement OrderBookClient struct:
  baseURL    string
  httpClient *http.Client  (shared, from http_client.go pool)
  timeout    time.Duration

SubmitOrder(ctx, order OrderRequest) (OrderResponse, time.Duration, error):
  t1 := time.Now()
  Make HTTP POST to /order with JSON body
  Read response body (max 4096 bytes)
  t2 := time.Now()
  latency := t2.Sub(t1)
  
  Handle errors:
  - context.DeadlineExceeded: return timed_out=true, latency=timeout
  - connection refused: return error "container_unreachable"
  - non-200 status: parse error body if possible, return as OrderResponse with status="REJECTED"
  - JSON unmarshal error: return error "invalid_response_format"
  
  Return (response, latency, nil) on success

CancelOrder(ctx, orderID string) (CancelResponse, time.Duration, error):
  Similar to SubmitOrder, POST to /cancel

HealthCheck(ctx) bool:
  GET /health with 1s timeout
  Return true only on 200 response

FILE: docs/contestant-api.md
Document the complete API that contestants must implement:
- Both endpoints with request/response schemas
- The /health endpoint requirement (must return 200 within 3s on startup)
- Timing SLA expectations
- Error response format
- Example implementations in C++ (using a simple HTTP library like cpp-httplib)

FILE: services/bot-fleet/client/circuit_breaker.go
CircuitBreaker per target endpoint:
States: CLOSED (normal), OPEN (failing, reject fast), HALF_OPEN (testing recovery)

Logic:
- Track: total requests, failed requests in last 10s window
- CLOSED → OPEN: if failure rate > 50% in 10s window AND total requests > 20
  (need minimum traffic before tripping to avoid false positives at test start)
- OPEN → HALF_OPEN: after 5s
- HALF_OPEN → CLOSED: if next 3 requests succeed
- HALF_OPEN → OPEN: if any request fails

In OPEN state: return immediate error without calling the container.
Log circuit breaker state transitions.
Expose state as a Prometheus metric.

�� SMART: The circuit breaker's minimum-traffic guard (20 requests before tripping)
prevents false-positives during test startup when the first few HTTP connections are
being established. Without it, a slow container taking 2 seconds to warm up trips the
circuit breaker before the test even properly starts.

Output complete, compilable Go code.
```

---

### PROMPT 27 — Bot Fleet: Kubernetes HPA + Scaling Config

```
Create Kubernetes deployment and autoscaling configs for the bot-fleet service.

FILE: infra/k8s/bot-fleet/deployment.yaml
apiVersion: apps/v1, kind: Deployment
name: bot-fleet
namespace: trade-eval

Key points:
- replicas: 1 (starts small, HPA scales up)
- terminationGracePeriodSeconds: 40 (bots need ~30s to drain gracefully)
- containers:
  - name: bot-fleet
  - resources: requests { cpu: 500m, memory: 256Mi } limits { cpu: 2000m, memory: 512Mi }
    Reason for high CPU limit: 200 bots per pod × HTTP calls is CPU intensive
  - env: read KAFKA_BROKERS, MAX_BOTS_PER_POD, BOT_REQUEST_TIMEOUT_MS from ConfigMap
  - livenessProbe: GET /healthz every 10s
  - readinessProbe: GET /readyz every 5s (not ready until Kafka consumer connected)
  - lifecycle.preStop: exec ["sh", "-c", "sleep 5"] 
    (give 5s for Kubernetes to drain traffic before SIGTERM)
  - ports: 8082 (metrics/health)

FILE: infra/k8s/bot-fleet/hpa.yaml
HorizontalPodAutoscaler:
- minReplicas: 0  ← SCALE TO ZERO when no tests running (cost savings!)
- maxReplicas: 100
- metrics:
  - type: External
    external:
      metric:
        name: kafka_consumer_group_lag
        selector:
          matchLabels:
            topic: orchestrator-events
            group: bot-fleet-workers
      target:
        type: Value
        value: "0"  ← scale up if any messages pending
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 60

This means: if there's a START_TEST event waiting in Kafka AND bots aren't consuming fast enough,
scale up. When all tests finish and no messages pending, scale down to 0 after cooldownPeriod.

FILE: infra/k8s/bot-fleet/configmap.yaml
ConfigMap with all non-secret bot-fleet configuration.

FILE: infra/k8s/bot-fleet/keda-scaledobject.yaml
Alternative to native HPA: use KEDA (Kubernetes Event-Driven Autoscaling) for better Kafka-based scaling.
ScaledObject that scales bot-fleet pods based on:
  - kafka lag on orchestrator-events topic
  - pollingInterval: 5s
  - cooldownPeriod: 60s (don't scale down too fast after test ends)
  - minReplicaCount: 0
  - maxReplicaCount: 100
  - Advanced: restoreToOriginalReplicaCount: true (reset to 0 when idle)

�� SMART: Scale-to-zero for bot-fleet pods saves significant cost.
In a 24-hour competition with 5-minute tests and 20 contestants:
20 × 5min = 100 minutes of active testing.
At 50 pods × $0.02/pod/hour: active cost = $0.83, idle cost = $23.20.
Scale-to-zero cuts the idle cost entirely. KEDA enables this more reliably than
native HPA because it has native Kafka consumer group lag as a scaling metric.

Output complete Kubernetes YAML.
```

---

### PROMPT 28 — Bot Fleet: Test Harness + Performance Benchmarks

```
Create performance benchmarks and testing for services/bot-fleet.

FILE: services/bot-fleet/bots/bot_bench_test.go

Benchmarks using Go's testing.B:

BenchmarkMarketMakerBot_HTTPCall:
- Start a mock HTTP server (httptest.NewServer) that responds to /order instantly
- Run market maker bot in benchmark loop
- Measure: ns/op (should be < 500µs per operation including goroutine overhead)

BenchmarkTelemetryProducer_Emit:
- Create TelemetryProducer with mock Kafka writer
- Benchmark Emit() with 100,000 events
- Measure: events/second (should be > 500,000/sec on modern hardware)
- Measure: memory allocations per emit (should be near 0 with sync.Pool)

BenchmarkShadowBook_AddOrder:
- Pre-populate shadow book with 1000 orders
- Benchmark AddOrder with mixed order types
- Measure: ns/op (should be < 10µs per order)

BenchmarkHTTPClientReuse:
- Compare: new http.Client per request vs shared connection pool
- Show the difference in ns/op and TCP connections created

FILE: services/bot-fleet/load_test.go
A load test that runs the entire bot fleet against a local mock server:

TestBotFleet_FullLoad:
- Start mock order book server (handles /order, /cancel, /health) in goroutine
  Mock server: accept all orders, random 50-500µs sleep to simulate latency, return FILLED
- Launch 100 bots of each persona against mock server
- Run for 10 seconds
- Collect telemetry events
- Assert:
  - Total events > 50,000 (throughput check)
  - Average latency < 1ms (the mock adds 50-500µs, so < 1ms is realistic)
  - No crashes or panics in any bot goroutine
  - Kafka producer dropped < 1% of events
  - Circuit breaker never tripped (mock server is healthy)

FILE: services/bot-fleet/bots/bot_test.go
Unit tests for each bot persona:

TestMarketMakerBot_CancelsStaleOrders:
- Create mock server that never fills orders (always PENDING)
- Run market maker for 500ms
- Assert: cancel events appear in telemetry after ~200ms (the cancel-stale-order threshold)

TestSpammerBot_CancelLatencyMeasured:
- Create mock server, verify SpammerBot records two separate telemetry events per cycle
  (one for the order, one for the cancel)

TestWhalBot_PartialFillHandled:
- Mock server returns partial fill (fills 1000 of 10,000 requested)
- Assert bot does NOT mark this as incorrect (partial fills are expected for whale orders)

TestCircuitBreaker_OpensOnHighFailureRate:
- Create mock server that returns 500 errors 60% of the time
- Run 30 requests through CircuitBreaker
- Assert circuit breaker transitions to OPEN state

Output all benchmark and test code.
```

---

### PROMPT 29 — Bot Fleet: FIX Protocol Bot (Advanced)

```
Implement an optional FIX 4.2 protocol bot for services/bot-fleet/bots/.

This is the "advanced" mode for contestants who want to test FIX protocol support.

FILE: services/bot-fleet/bots/fix_bot.go

FIX (Financial Information eXchange) Protocol 4.2 implementation.

Note: Only a subset of FIX needed for order submission.

FIX message format (pipe-separated for clarity, actual uses SOH char \x01):
8=FIX.4.2|9=<length>|35=<msgtype>|49=<sender>|56=<target>|34=<seqnum>|52=<timestamp>|<body>|10=<checksum>

FIX message types used:
- 35=A (Logon) — establish session
- 35=D (NewOrderSingle) — submit new order
- 35=F (OrderCancelRequest) — cancel order
- 35=8 (ExecutionReport) — order fill response (from contestant server)
- 35=9 (OrderCancelReject) — cancel rejected

FIXBot struct:
  conn        net.Conn
  targetURL   string  (host:port of contestant's FIX server)
  senderCompID, targetCompID string
  seqNum      int64  (auto-increment)
  mu          sync.Mutex

Connect():
  1. TCP dial to targetURL with 5s timeout
  2. Send Logon message (35=A): HeartBtInt=30, ResetOnLogon=Y
  3. Wait for Logon response (35=A) within 5s
  4. On success: start heartbeat goroutine (sends Heartbeat 35=0 every 30s)

SubmitOrder(order) (ExecutionReport, time.Duration, error):
  Build NewOrderSingle (35=D) message:
  Tags:
    11=order_id (ClOrdID)
    54=1 (Side: 1=Buy, 2=Sell)
    38=quantity (OrderQty)
    40=2 (OrdType: 2=Limit, 1=Market)
    44=price (Price, omit for market orders)
    60=timestamp (TransactTime, format: YYYYMMDD-HH:MM:SS.mmm)
    21=1 (HandlInst: 1=AutoExecute)
  Send message, measure latency, wait for ExecutionReport (35=8)
  Parse fill fields: 39=OrdStatus, 32=LastQty, 31=LastPx, 14=CumQty

buildFIXMessage(msgType, body string) string:
  Construct full FIX message with correct:
  - BeginString (8=FIX.4.2)
  - BodyLength (9=calculated)
  - MsgType (35)
  - SenderCompID (49), TargetCompID (56)
  - MsgSeqNum (34, auto-increment, thread-safe)
  - SendingTime (52, UTC)
  - Body fields
  - Checksum (10=sum of all bytes mod 256, 3-digit padded)

parseFIXMessage(raw string) map[string]string:
  Split on SOH (\x01), parse tag=value pairs into map

FILE: docs/contestant-fix-api.md
Document FIX 4.2 API that contestants can optionally implement:
- Session establishment
- NewOrderSingle format
- Expected ExecutionReport fields
- Sample FIX messages with explanation of each tag

�� SMART: FIX protocol support is a differentiator for contestants with real
trading system experience. Most trading infrastructure runs FIX — adding FIX
support lets contestants use production-grade order routing code instead of
writing a custom HTTP layer. Score FIX submissions separately (different scoring
category) to avoid unfair comparison with HTTP submissions.

Output complete, compilable Go code.
```

---

### PROMPT 30 — Bot Fleet: Bot Fleet Observability

```
Implement comprehensive observability for services/bot-fleet.

FILE: services/bot-fleet/metrics/prometheus.go

Expose Prometheus metrics on :8082/metrics.

Define these metrics:

Counters:
- bot_fleet_orders_sent_total {persona, order_type, contestant_id}
- bot_fleet_orders_correct_total {persona, contestant_id}
- bot_fleet_orders_incorrect_total {persona, contestant_id, reason}
- bot_fleet_orders_timedout_total {persona, contestant_id}
- bot_fleet_kafka_events_emitted_total
- bot_fleet_kafka_events_dropped_total

Gauges:
- bot_fleet_active_bots {test_id, contestant_id}
- bot_fleet_active_tests
- bot_fleet_circuit_breaker_state {contestant_id} (0=closed, 1=open, 2=half_open)

Histograms (with buckets):
- bot_fleet_order_latency_microseconds {persona, contestant_id}
  Buckets: [50, 100, 250, 500, 1000, 2500, 5000, 10000, 50000, 100000, 500000, 1000000]
  The bucket boundaries are in microseconds. This lets you see the full latency
  distribution without TimescaleDB: bucket at 500µs shows % of orders under 500µs.

FILE: services/bot-fleet/health/health.go
Health check endpoints:
GET /healthz: returns 200 if process is alive (liveness probe)
GET /readyz: returns 200 only if:
  - Kafka consumer is connected and consuming
  - At least 0 active tests (always true — just verifying Kafka connectivity)
  Returns 503 if Kafka consumer is not connected

FILE: services/bot-fleet/metrics/dashboard.json
A Grafana dashboard JSON file for the bot fleet.
Define panels:
1. Active Bots (gauge panel: bot_fleet_active_bots)
2. Orders per Second (rate: bot_fleet_orders_sent_total)
3. Latency Heatmap (heatmap panel from bot_fleet_order_latency_microseconds histogram)
4. Correctness Rate (% correct = correct / (correct + incorrect) per contestant)
5. Kafka Drop Rate (rate: bot_fleet_kafka_events_dropped_total)
6. Circuit Breaker States (state panel per contestant)

�� SMART: The latency histogram with microsecond buckets gives you approximate p50/p90/p99
WITHOUT querying TimescaleDB. During active tests you can see "50% of orders are under 
500µs" directly in Grafana just from Prometheus metrics. This is 100x faster to query
than TimescaleDB for real-time debugging.

Output complete Go code and the Grafana JSON.
```

---

## PHASE 4 — TELEMETRY INGESTER SERVICE (Prompts 31–40)

---

### PROMPT 31 — Telemetry Ingester: Main + Consumer Architecture

```
Implement the core of services/telemetry-ingester/.

This service reads the firehose of bot events from Kafka, validates correctness,
computes real-time metrics, and writes to TimescaleDB + Redis.

FILE: services/telemetry-ingester/main.go
Config:
  KafkaBrokers, TelemetryTopic, ConsumerGroupID (telemetry-ingesters),
  TimescaleDSN, RedisURL,
  WindowSizeSeconds int (default 30, sliding window for p99 computation),
  TelemetryBatchSize int (default 500, TimescaleDB batch insert size),
  TelemetryFlushIntervalMs int (default 1000),
  MaxConsumerLag int (default 100000, alert if Kafka lag exceeds this),
  ReorderBufferMs int (default 100, hold events for 100ms before processing for ordering)

Initialize: Kafka consumer group, TimescaleDB pool, Redis client
Start goroutines:
1. Kafka consumer loop (N parallel consumers = N Kafka partitions)
2. Reorder buffer flusher
3. TimescaleDB batch writer
4. Redis metrics publisher (every 1s writes rolling metrics to Redis)
5. Lag monitor (checks Kafka consumer group lag every 30s, alerts if high)

FILE: services/telemetry-ingester/consumer.go

Kafka consumer group with 16 goroutines (matching 16 Kafka partitions for bot-telemetry).
Each goroutine independently reads from its assigned partitions.

For each TelemetryEvent from Kafka:
1. Unmarshal JSON
2. Send to reorder buffer channel: reorderCh <- event

FILE: services/telemetry-ingester/reorder_buffer.go

ReorderBuffer: holds events for ReorderBufferMs before sorting and processing.
This corrects for out-of-order delivery from distributed bots.

Design:
  - Ring buffer of time windows (each window = 10ms)
  - Events arrive and go into the bucket for their sent_at_ns timestamp
  - Every 10ms, the oldest bucket is "sealed" and events sorted by sequence_number
  - Sealed events are sent to the processing pipeline in order

Implementation:
  type TimeWindow struct {
    events      []TelemetryEvent
    windowStart int64  // nanoseconds
    sealed      bool
  }
  
  windows: [10]*TimeWindow  // circular array of 10 windows (100ms total)
  
  Add(event TelemetryEvent):
    windowIdx = (event.sent_at_ns / 10ms) % 10
    windows[windowIdx].events = append(windows[windowIdx].events, event)
  
  Flush() []TelemetryEvent:
    Seals the oldest window, sorts by sequence_number, returns events
    
  Why sequence numbers? Without them, if bot A on pod 1 and bot B on pod 2 both send
  at nanosecond X, Kafka arrival order could put B before A. The shadow order book
  processes orders in the ORDER BOTS SENT THEM. Sequence numbers are assigned by the
  TestRunner (atomic counter), so they represent true send order across all bots.

�� SMART: The reorder buffer is the key to correct shadow order book validation.
Without it, the shadow book processes orders in Kafka arrival order ≠ true send order,
causing false correctness failures. With it, orders are processed in the correct
sequence even when events arrive out of order due to network jitter between pods.

Output complete, compilable Go code.
```

---

### PROMPT 32 — Telemetry Ingester: HDR Histogram + Latency Computation

```
Implement latency percentile computation for services/telemetry-ingester.

FILE: services/telemetry-ingester/latency/hdr_histogram.go

Implement an HDR (High Dynamic Range) Histogram for computing latency percentiles.
DO NOT use a third-party HDR histogram library — implement a simplified version
sufficient for our 1µs to 10,000,000µs range.

HDRHistogram struct:
  buckets      [100000]int64  // bucket[i] = count of events with latency i µs
  totalCount   int64
  totalSum     int64
  maxObserved  int64
  minObserved  int64

Methods:

RecordValue(latencyUs int64):
  if latencyUs < 0: latencyUs = 0
  if latencyUs > 9999999: latencyUs = 9999999  // cap at 10s
  atomic.Add(&buckets[latencyUs], 1)
  atomic.Add(&totalCount, 1)
  atomic.Add(&totalSum, latencyUs)
  // update min/max with CAS loop

Percentile(pct float64) int64:
  // pct: 0.50, 0.90, 0.99, 0.999
  target := int64(float64(totalCount) * pct)
  cumulative := int64(0)
  for i, count := range buckets {
    cumulative += count
    if cumulative >= target {
      return int64(i)
    }
  }
  return maxObserved

Mean() float64: float64(totalSum) / float64(totalCount)
Count() int64: return totalCount

Reset():
  Zero out all buckets (use memset equivalent)
  Reset totalCount, totalSum, min, max

FILE: services/telemetry-ingester/latency/sliding_window.go

SlidingWindowHistogram manages a rolling window of HDR histograms.

Design: Instead of one histogram, maintain N time-bucketed histograms (30 × 1s buckets
for a 30-second window). Every second, advance the window by resetting the oldest bucket.

Fields:
  buckets     [30]*HDRHistogram  // one per second
  currentIdx  int                 // which bucket is "current"
  mu          sync.Mutex

Methods:

Record(latencyUs int64):
  mu.Lock()
  buckets[currentIdx].RecordValue(latencyUs)
  mu.Unlock()

Advance():  // called every second by a ticker
  mu.Lock()
  currentIdx = (currentIdx + 1) % 30
  buckets[currentIdx].Reset()  // reset the bucket we're now using
  mu.Unlock()

GetPercentiles() (p50, p90, p99 int64):
  mu.Lock(); defer mu.Unlock()
  // Merge all non-current buckets (last 29 seconds of data)
  merged := NewHDRHistogram()
  for i, b := range buckets {
    if i != currentIdx {
      merged.MergeFrom(b)
    }
  }
  return merged.Percentile(0.50), merged.Percentile(0.90), merged.Percentile(0.99)

MergeFrom(other *HDRHistogram):
  for i, count := range other.buckets {
    buckets[i] += count  // NOT atomic here — protected by outer mutex
  }

FILE: services/telemetry-ingester/latency/hdr_histogram_test.go

TestPercentile_KnownValues:
  Insert 1000 values: 500 at 100µs, 400 at 500µs, 99 at 1000µs, 1 at 10000µs
  Assert p50 = 100µs
  Assert p90 = ~500µs (90th percentile = 900th value = in the 500µs bucket)
  Assert p99 = 1000µs (99th value is in the 1000µs bucket)
  Assert p99.9 = 10000µs

TestSlidingWindow_OldDataExpires:
  Record 1000 values at 100µs
  Advance window 30 times (simulate 30 seconds passing)
  Assert all percentiles return 0 (old data expired)

�� SMART: The HDR histogram's O(1) record and O(buckets) percentile is the key.
At 1M events/sec, a sort-based p99 would take O(n log n) per second = 20M operations.
The HDR approach: 1M × O(1) record + 1 × O(100000) scan = 1.1M operations.
It's a 10x performance difference that matters a lot at this scale.

Output complete, compilable Go code.
```

---

### PROMPT 33 — Telemetry Ingester: Shadow Order Book (Authoritative)

```
Implement the authoritative shadow order book in services/telemetry-ingester/shadowbook/.

This is the reference implementation that validates correctness of contestant fills.
It's more complex than the bot-side book because it processes ALL bots' orders in sequence.

FILE: services/telemetry-ingester/shadowbook/order_book.go

OrderBook — the authoritative price-time priority matching engine.
This is the reference implementation your scoring is based on.

Data structures:
  type PriceLevel struct {
    Price  float64
    Orders []Order  // FIFO queue at each price level
  }
  
  type Order struct {
    ID           string
    ContestantID string
    Type         string  // LIMIT_BUY, LIMIT_SELL, MARKET_BUY, MARKET_SELL
    Price        float64
    Quantity     float64
    RemainingQty float64
    SubmittedAt  int64   // sequence number, NOT wall clock time
  }
  
  OrderBook struct:
    bids    []PriceLevel  // sorted DESCENDING by price (best bid first)
    asks    []PriceLevel  // sorted ASCENDING by price (best ask first)
    orders  map[string]*Order  // lookup by order ID
    mu      sync.RWMutex

Methods:

ProcessOrder(order Order) ExpectedFill:
  Acquires write lock.
  
  switch order.Type {
  case MARKET_BUY:
    return matchMarket(order, &asks)  // consumes from asks
  case MARKET_SELL:
    return matchMarket(order, &bids)  // consumes from bids
  case LIMIT_BUY:
    fill := matchLimit(order, &asks)  // try to match against asks
    if order.RemainingQty > 0 {
      insertBid(order)  // rest of order goes to bids
    }
    return fill
  case LIMIT_SELL:
    fill := matchLimit(order, &bids)
    if order.RemainingQty > 0 {
      insertAsk(order)
    }
    return fill
  }

matchMarket(order, levels) ExpectedFill:
  fill := ExpectedFill{Status: FILLED, FilledQty: 0}
  for len(levels) > 0 && order.RemainingQty > 0 {
    best := levels[0]
    for len(best.Orders) > 0 && order.RemainingQty > 0 {
      matched := min(order.RemainingQty, best.Orders[0].RemainingQty)
      fill.FilledQty += matched
      fill.FilledPrice = best.Price  // use PRICE-TIME priority price
      order.RemainingQty -= matched
      best.Orders[0].RemainingQty -= matched
      if best.Orders[0].RemainingQty == 0 {
        delete(orders, best.Orders[0].ID)
        best.Orders = best.Orders[1:]  // dequeue
      }
    }
    if len(best.Orders) == 0 {
      levels = levels[1:]  // remove empty price level
    }
  }
  if order.RemainingQty > 0 {
    fill.Status = PARTIAL
    fill.RemainingQty = order.RemainingQty
  }
  return fill

CancelOrder(orderID string) (cancelled bool):
  Find order in orders map, remove from its price level's FIFO queue.
  Return true if found and cancelled, false if not found (order might have already filled).

ProcessOrderInSequence(events []TelemetryEvent) []ExpectedFill:
  Processes all events in sequence_number order (pre-sorted by reorder buffer).
  Returns expected fills aligned to the input events.
  
FILE: services/telemetry-ingester/shadowbook/correctness_validator.go

CorrectnessValidator:
  shadowBook  *OrderBook
  stats       map[string]*ContestantStats  // keyed by contestant_id

ValidateBatch(events []TelemetryEvent) []ValidationResult:
  1. Sort events by sequence_number (should already be sorted by reorder buffer, but verify)
  2. For each event: expectedFill = shadowBook.ProcessOrder(event.order)
  3. Compare expectedFill vs event.actual_fill:
     - FILLED vs PENDING: INCORRECT (contestant didn't execute a matching order)
     - FILLED at price X vs FILLED at price Y: INCORRECT (wrong price)
     - FILLED qty 10 vs FILLED qty 8: INCORRECT (wrong quantity)
     - PENDING vs PENDING: CORRECT
     - CANCELLED vs CANCELLED: CORRECT
     - PARTIAL (correct qty range): CORRECT
  4. Record result in stats
  5. Return ValidationResult slice

FILE: services/telemetry-ingester/shadowbook/order_book_test.go
Comprehensive tests:
- TestPriceTimePriority: two LIMIT_SELLs at same price, verify FIFO fill order
- TestMarketBuyConsumesMultipleLevels: asks at 100/101/102, market buy drains them
- TestLimitBuyRests: no matching sell, verify order in bids
- TestCancelPartiallyFilled: order partially filled then cancelled
- TestShadowBookVsContestant_ExampleCorrectFill
- TestShadowBookVsContestant_WrongPrice

Output complete, compilable Go code.
```

---

### PROMPT 34 — Telemetry Ingester: TimescaleDB Batch Writer

```
Implement the high-performance batch writer for services/telemetry-ingester.

FILE: services/telemetry-ingester/storage/timescale_writer.go

TimescaleWriter manages high-throughput writes to TimescaleDB.

The key challenge: at 1M events/sec, we cannot INSERT one row at a time.
We must batch inserts using PostgreSQL's COPY protocol.

Fields:
  pool         *pgxpool.Pool
  batchCh      chan LatencySample  (buffered: 50,000)
  tpsBatchCh   chan TPSSample      (buffered: 10,000)
  flushTicker  *time.Ticker        (every 1 second)
  batchSize    int                 (default 500)

LatencySample struct:
  time         time.Time
  contestantID string
  testID       string
  botID        string
  botPersona   string
  latencyUs    int64
  orderType    string
  correct      bool
  timedOut     bool

TPSSample struct:
  time              time.Time
  contestantID      string
  testID            string
  ordersPerSecond   float64

WriteSample(sample LatencySample) bool:
  Tries to send to batchCh:
  select {
  case batchCh <- sample: return true
  default: return false  // batch channel full, drop (same pattern as Kafka producer)
  }

Background flush goroutine (flushLoop):
  Accumulates from batchCh until batch is full (batchSize) OR flushTicker fires.
  Then calls bulkInsert(batch).

bulkInsert(samples []LatencySample) error:
  Use pgx CopyFrom (PostgreSQL COPY protocol) — MUCH faster than bulk INSERT:
  
  rows := pgx.CopyFromSlice(len(samples), func(i int) ([]any, error) {
    s := samples[i]
    return []any{s.time, s.contestantID, s.testID, s.botID, s.botPersona,
                 s.latencyUs, s.orderType, s.correct, s.timedOut}, nil
  })
  _, err := pool.CopyFrom(ctx, pgx.Identifier{"latency_samples"},
    []string{"time", "contestant_id", "test_id", "bot_id", "bot_persona",
             "latency_us", "order_type", "correct", "timed_out"}, rows)

  Log error if any, increment dropped counter. DO NOT retry indefinitely —
  if TimescaleDB is slow, continue dropping rather than growing unbounded in memory.

bulkInsertTPS(samples []TPSSample) error:
  Same pattern using CopyFrom for tps_samples table.

FILE: services/telemetry-ingester/storage/redis_metrics_writer.go

RedisMetricsWriter publishes rolling metrics to Redis for the leaderboard.

Every 1 second, for each active contestant_id, computes from the SlidingWindowHistogram
and writes to Redis:
  pipe := redis.Pipeline()
  pipe.HSet(ctx, "metrics:"+contestantID,
    "p50_latency_us", p50,
    "p90_latency_us", p90,
    "p99_latency_us", p99,
    "tps", currentTPS,
    "correctness_rate", correctnessRate,
    "total_orders", totalOrders,
    "correct_orders", correctOrders,
    "last_updated_ns", time.Now().UnixNano(),
  )
  pipe.SAdd(ctx, "leaderboard:active_contestants", contestantID)
  pipe.Exec(ctx)

Uses Redis pipeline to send all metrics in one network round trip (not N round trips).

�� SMART: PostgreSQL COPY protocol is 10-50x faster than INSERT for bulk data.
At 1M events/sec → 1M rows/sec needed → COPY can do this.
INSERT (even batched) would struggle above 100K rows/sec. This is not premature
optimization — it's the ONLY viable approach at this event rate.

Output complete, compilable Go code.
```

---

### PROMPT 35 — Telemetry Ingester: TPS Counter + Metrics Aggregator

```
Implement TPS counting and metrics aggregation for services/telemetry-ingester.

FILE: services/telemetry-ingester/metrics/tps_counter.go

TPSCounter tracks orders-per-second per contestant using a sliding window approach.

Design: Token bucket in reverse — we count tokens (orders) arriving per window.

Fields:
  windows     map[string]*[60]int64  // contestant_id → 60 one-second buckets
  currentSec  int64                   // current second (unix timestamp)
  mu          sync.Mutex

Record(contestantID string):
  sec := time.Now().Unix()
  windowIdx := sec % 60
  
  mu.Lock()
  if windows[contestantID] == nil {
    windows[contestantID] = new([60]int64)
  }
  if sec != currentSec {
    // New second: reset the bucket we're about to use
    windows[contestantID][windowIdx] = 0
    currentSec = sec
  }
  windows[contestantID][windowIdx]++
  mu.Unlock()

GetCurrentTPS(contestantID string) float64:
  // Average of last 5 seconds (smoothed)
  sec := time.Now().Unix()
  sum := int64(0)
  for i := int64(1); i <= 5; i++ {
    idx := (sec - i) % 60
    sum += windows[contestantID][idx]
  }
  return float64(sum) / 5.0

GetPeakTPS(contestantID string) float64:
  max := int64(0)
  for _, count := range windows[contestantID] {
    if count > max { max = count }
  }
  return float64(max)

FILE: services/telemetry-ingester/metrics/aggregator.go

MetricsAggregator combines all metrics for a contestant.

Fields (per contestant):
  latencyHistogram   *SlidingWindowHistogram
  tpsCounter         *TPSCounter
  totalOrders        atomic.Int64
  correctOrders      atomic.Int64
  timedOutOrders     atomic.Int64
  incorrectOrders    atomic.Int64

ProcessEvent(event TelemetryEvent, validationResult ValidationResult):
  totalOrders.Add(1)
  tpsCounter.Record(event.ContestantID)
  latencyHistogram.Record(event.LatencyUs)
  
  if event.TimedOut {
    timedOutOrders.Add(1)
  } else if validationResult.Correct {
    correctOrders.Add(1)
  } else {
    incorrectOrders.Add(1)
  }

GetSnapshot(contestantID string) MetricsSnapshot:
  p50, p90, p99 := latencyHistogram.GetPercentiles()
  validTotal := totalOrders.Load() - timedOutOrders.Load()
  correctnessRate := float64(0)
  if validTotal > 0 {
    correctnessRate = float64(correctOrders.Load()) / float64(validTotal)
  }
  return MetricsSnapshot{
    ContestantID:    contestantID,
    P50LatencyUs:    p50,
    P90LatencyUs:    p90,
    P99LatencyUs:    p99,
    CurrentTPS:      tpsCounter.GetCurrentTPS(contestantID),
    PeakTPS:         tpsCounter.GetPeakTPS(contestantID),
    TotalOrders:     totalOrders.Load(),
    CorrectOrders:   correctOrders.Load(),
    CorrectnessRate: correctnessRate,
  }

FILE: services/telemetry-ingester/metrics/aggregator_test.go
Tests:
- TestCorrectnessRate_AllCorrect: 1000 correct events → rate = 1.0
- TestCorrectnessRate_HalfCorrect: 500 correct + 500 incorrect → rate = 0.5
- TestTPSCounter_AccurateCount: record 1000 events in 1 second → GetCurrentTPS ≈ 1000
- TestTPSCounter_OldDataNotCounted: record 1000 events, advance 10 seconds → GetCurrentTPS ≈ 0

Output complete, compilable Go code.
```

---

### PROMPT 36 — Telemetry Ingester: Complete Processing Pipeline

```
Wire everything together in services/telemetry-ingester/pipeline.go.

FILE: services/telemetry-ingester/pipeline.go

ProcessingPipeline orchestrates the full event processing flow.

Fields:
  reorderBuffer       *ReorderBuffer
  shadowBook          *CorrectnessValidator
  aggregators         map[string]*MetricsAggregator  // per contestant
  timescaleWriter     *TimescaleWriter
  redisWriter         *RedisMetricsWriter
  tpsCounter          *TPSCounter
  mu                  sync.RWMutex  // protects aggregators map

ProcessBatch(events []TelemetryEvent):
  // Called by reorder buffer after sorting events by sequence_number
  
  // Group events by contestant for shadow book processing
  byContestant := make(map[string][]TelemetryEvent)
  for _, e := range events {
    byContestant[e.ContestantID] = append(byContestant[e.ContestantID], e)
  }
  
  for contestantID, contestantEvents := range byContestant {
    // Get or create aggregator for this contestant
    agg := getOrCreateAggregator(contestantID)
    
    // Validate correctness using shadow book
    validationResults := shadowBook.ValidateBatch(contestantEvents)
    
    // Process each event
    for i, event := range contestantEvents {
      agg.ProcessEvent(event, validationResults[i])
      
      // Queue for TimescaleDB write
      timescaleWriter.WriteSample(LatencySample{
        time:         time.Unix(0, event.SentAtNs),
        contestantID: event.ContestantID,
        testID:       event.TestID,
        botID:        event.BotID,
        botPersona:   event.BotPersona,
        latencyUs:    event.LatencyUs,
        orderType:    event.OrderType,
        correct:      validationResults[i].Correct,
        timedOut:     event.TimedOut,
      })
    }
  }

StartMetricsPublisher(ctx):
  // Every 1 second: read all aggregators and push to Redis
  ticker := time.NewTicker(1 * time.Second)
  for {
    select {
    case <-ticker.C:
      for contestantID, agg := range aggregators {
        snapshot := agg.GetSnapshot(contestantID)
        redisWriter.PublishSnapshot(snapshot)
      }
    case <-ctx.Done():
      return
    }
  }

FILE: services/telemetry-ingester/telemetry_ingester_test.go

Full integration test with testcontainers:

TestFullPipeline_CorrectOrderBook:
- Start Kafka, TimescaleDB, Redis
- Initialize pipeline
- Publish 10,000 synthetic telemetry events (all marked correct, latencies 100-1000µs)
- Wait for pipeline to process
- Assert: TimescaleDB has 10,000 rows in latency_samples
- Assert: Redis metrics:{contestantID}:p99 is in expected range (< 2000µs)
- Assert: Redis metrics:{contestantID}:correctness_rate == 1.0

TestFullPipeline_IncorrectOrderBook:
- Publish 1,000 events with actual_fill.price != expected_fill.price
- Assert: correctness_rate in Redis < 0.1 (shadow book should catch these)

TestFullPipeline_KafkaLag:
- Simulate consumer lag by publishing 100,000 events rapidly
- Assert: pipeline processes all events within 5 seconds
- Assert: no events lost (all 100,000 in TimescaleDB)

Output complete, compilable Go code.
```

---

### PROMPT 37 — Telemetry Ingester: Kafka Consumer Group + Lag Monitoring

```
Implement Kafka consumer group management and lag monitoring.

FILE: services/telemetry-ingester/kafka/consumer_group.go

ParallelConsumerGroup: manages N parallel Kafka consumers for bot-telemetry topic.

N = number of partitions for the topic (16 in our setup).
Each consumer reads from its assigned partitions independently.

Design using segmentio/kafka-go:
  - Create a kafka.Reader per partition (not a group reader) for maximum control
  - Actually use kafka.NewReader with GroupID for automatic partition assignment
  - Start 8 goroutines (8 = a good default; Kafka will assign partitions round-robin)
  - Each goroutine runs an independent read loop
  - Messages go to a shared processingCh channel (buffered: 100,000)
  - Main pipeline reads from processingCh

FILE: services/telemetry-ingester/kafka/lag_monitor.go

LagMonitor: alerts on high Kafka consumer group lag.

Uses Kafka Admin API to fetch consumer group offsets and topic end offsets.
Lag = end_offset - committed_offset (per partition).

Every 30 seconds:
  For each partition of bot-telemetry topic:
    lag = endOffset - committedOffset
    totalLag += lag

If totalLag > MaxConsumerLag (100,000 by default):
  - Log ERROR "KAFKA LAG HIGH: {lag} events behind"
  - Increment prometheus metric: telemetry_kafka_consumer_lag
  - Optionally: send alert to Slack/PagerDuty via webhook (if configured)

If lag is growing (current > previous * 1.2):
  Log WARNING "Kafka lag growing — consider adding consumer replicas"
  Export metric: telemetry_kafka_lag_growing = 1

FILE: services/telemetry-ingester/kafka/offset_tracker.go

OffsetTracker: manages manual offset commits for at-least-once delivery.

Key insight: commit offsets ONLY AFTER successfully writing to TimescaleDB.
If TimescaleDB is unavailable and we commit offsets, we lose those events forever.

Design:
  pendingOffsets map[int32]int64  // partition → last processed offset
  
  MarkProcessed(partition int32, offset int64):
    pendingOffsets[partition] = offset
  
  CommitAll(ctx) error:
    Calls kafka.CommitMessages for all pending offsets
    Clears pendingOffsets on success
    On failure: logs error, retains pending (will retry next commit cycle)

FlushOnShutdown(ctx):
  On SIGTERM: wait up to 30s for all in-flight events to be processed,
  then commit all pending offsets before exiting.
  If we crash before committing: events re-processed after restart (at-least-once delivery).

�� SMART: Committing offsets after TimescaleDB write (not after Kafka read) gives
at-least-once semantics. At-least-once = some events processed twice on crash recovery.
Your TimescaleDB insert should be IDEMPOTENT: use INSERT ON CONFLICT DO NOTHING with
a unique constraint on (sent_at_ns, bot_id, order_id). Then duplicate events during
recovery are silently ignored, giving effectively-exactly-once semantics.

Output complete, compilable Go code.
```

---

### PROMPT 38 — Telemetry Ingester: Historical Queries API

```
Add a REST API to the telemetry ingester for historical metric queries.

FILE: services/telemetry-ingester/api/server.go

Expose a read-only HTTP API on port 8083 for historical queries.
Routes:
  GET /v1/metrics/{contestant_id}/latency?start=&end=&resolution=
  GET /v1/metrics/{contestant_id}/tps?start=&end=
  GET /v1/metrics/{contestant_id}/correctness?start=&end=
  GET /v1/metrics/{contestant_id}/summary
  GET /v1/metrics/comparison?test_ids=
  GET /v1/health

FILE: services/telemetry-ingester/api/handlers.go

GetLatencyHistory(w, r):
  Parse query params: start (unix ms), end (unix ms), resolution (1s/1m/5m/1h)
  TimescaleDB query using time_bucket (TimescaleDB's built-in time aggregation):
  
  SELECT 
    time_bucket('1 second', time) AS bucket,
    percentile_cont(0.50) WITHIN GROUP (ORDER BY latency_us) AS p50,
    percentile_cont(0.90) WITHIN GROUP (ORDER BY latency_us) AS p90,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_us) AS p99,
    COUNT(*) AS order_count
  FROM latency_samples
  WHERE contestant_id = $1 AND time >= $2 AND time <= $3
  GROUP BY bucket
  ORDER BY bucket ASC
  
  Return as JSON array of time-series points.

GetTestSummary(w, r):
  Returns the test_summaries record for the given contestant's latest completed test.
  If test still running: returns live metrics from Redis instead.
  Merge both: { "historical": {...}, "live": {...}, "source": "running|completed" }

GetLeaderboardComparison(w, r):
  Query params: test_ids (comma-separated list of test IDs to compare)
  Returns side-by-side comparison of latency distributions for multiple tests.
  Useful for: "how did contestant X improve between test run 1 and test run 5?"
  
  For each test: fetch percentile breakdown, TPS, correctness from TimescaleDB.
  Return as:
  {
    "tests": [
      { "test_id": "test_abc", "contestant": "Alice", "p99": 450, "tps": 9200, ... },
      { "test_id": "test_def", "contestant": "Bob",   "p99": 380, "tps": 11000, ... }
    ]
  }

FILE: services/telemetry-ingester/api/queries.go
Prepared SQL queries as named constants:
- QUERY_LATENCY_HISTORY (uses time_bucket)
- QUERY_TPS_HISTORY (uses time_bucket, COUNT per bucket / bucket_size)
- QUERY_CORRECTNESS_HISTORY (correct_orders / total_orders per bucket)
- QUERY_TEST_SUMMARY
Use pgx.PrepareCache to cache prepared statement execution plans.

Output complete, compilable Go code.
```

---

### PROMPT 39 — Telemetry Ingester: Dockerfile + Resource Tuning

```
Create deployment configuration for the telemetry-ingester service.

FILE: services/telemetry-ingester/Dockerfile
Multi-stage build similar to submission-api.
Final image: distroless, non-root, < 30MB.

FILE: infra/k8s/telemetry-ingester/deployment.yaml
Key resource settings for telemetry-ingester (this is the CPU-heaviest service):

resources:
  requests:
    cpu: "1000m"    # 1 full core — shadow book + HDR histogram are CPU-bound
    memory: "512Mi"  # HDR histogram uses ~400MB (100K buckets × 8B × 50 contestants)
  limits:
    cpu: "2000m"
    memory: "1Gi"

replicas: 3 (for throughput, multiple consumers share 16 Kafka partitions)

PodAntiAffinity: spread across nodes to avoid single-node bottleneck.

FILE: infra/k8s/telemetry-ingester/hpa.yaml
Scale on: kafka_consumer_group_lag metric from KEDA
minReplicas: 2 (always have 2 for redundancy)
maxReplicas: 8 (8 × 2 consumer goroutines = 16 = matches partition count)

FILE: services/telemetry-ingester/TUNING.md
Document performance tuning decisions:
1. HDR histogram bucket count (100,000) and memory usage
2. Reorder buffer window size (100ms) tradeoff (larger = more correct, more latency)
3. TimescaleDB batch size (500) tuning guide
4. Kafka consumer parallelism = partition count rule
5. TimescaleDB chunk_time_interval: SET to 1 hour for this workload
   (each test is 5 min, 1 hour chunks = ~12 tests per chunk, efficient compression)
6. TimescaleDB compression policy: compress chunks older than 7 days
   (ORDER BY: contestant_id, time — matches our query pattern perfectly)

Add the TimescaleDB tuning commands as SQL:
SELECT add_compression_policy('latency_samples', INTERVAL '7 days');
SELECT set_chunk_time_interval('latency_samples', INTERVAL '1 hour');
ALTER TABLE latency_samples SET (
  timescaledb.compress,
  timescaledb.compress_orderby = 'contestant_id, time DESC',
  timescaledb.compress_segmentby = 'test_id'
);

�� SMART: TimescaleDB columnar compression (the compress settings above) reduces
storage by 90%+ for time-series data. A test generating 1M events takes ~1GB uncompressed.
After 7 days (post-contest) it compresses to ~100MB. Over a 7-day contest with 100 contestants
running multiple tests: 50GB → 5GB. This is not cosmetic — it keeps your database server
viable without expensive storage.

Output all files with complete content.
```

---

### PROMPT 40 — Telemetry Ingester: Anomaly Detection

```
Add anomaly detection to the telemetry ingester to catch cheating or bugs.

FILE: services/telemetry-ingester/anomaly/detector.go

AnomalyDetector identifies suspicious patterns in contestant performance.

Anomalies detected:

1. LATENCY_TOO_LOW:
   If p99 latency < 1µs (1 microsecond): SUSPICIOUS.
   A real order book cannot respond in < 1µs — this indicates the contestant
   is not actually processing orders (returning cached responses or no-ops).
   Rule: if p99_latency < 1µs AND total_orders > 100: flag as LATENCY_TOO_LOW.

2. PERFECT_CORRECTNESS_WITH_HIGH_SPEED:
   If correctness_rate == 1.000 AND p99 < 10µs: SUSPICIOUS.
   A real matching engine that's 10x faster than industry leaders AND perfectly correct
   could indicate precomputed responses. Log for manual review.

3. LATENCY_SPIKE_ANOMALY:
   If current p99 > previous p99 × 10 (10x spike in a single 1-second window):
   Likely a GC pause or system hiccup. Flag but don't penalize — it's informational.

4. FILL_PRICE_DRIFT:
   Compare fill prices reported by contestant vs shadow book expected prices.
   If average price deviation > 0.1%: flag as PRICE_MANIPULATION.
   A correct implementation should match the shadow book's prices exactly.

5. ORDER_REORDERING:
   If contestant fills order B before order A when both are at the same price level
   (detected via sequence numbers): INCORRECT TIME PRIORITY.
   This is a hard correctness failure, not just suspicious.

6. THROUGHPUT_DROP:
   If TPS drops > 90% in a 5-second window: container may be struggling/crashed.
   Signal orchestrator to check container health.

AnomalyReport struct:
  ContestantID string
  TestID       string
  Type         string  // LATENCY_TOO_LOW, PERFECT_CORRECTNESS_WITH_HIGH_SPEED, etc.
  Severity     string  // INFO, WARNING, CRITICAL
  Details      string
  DetectedAt   time.Time

Publish anomaly reports to Kafka topic: telemetry-anomalies
Also write to Redis: LPUSH anomalies:{contestantID} {json} (list, max 100 entries via LTRIM)
So the leaderboard can display anomaly warnings next to scores.

FILE: services/telemetry-ingester/anomaly/detector_test.go
Tests for each anomaly type with sample data that should and shouldn't trigger.

Output complete, compilable Go code.
```

---

## PHASE 5 — LEADERBOARD API + WEBSOCKET (Prompts 41–47)

---

### PROMPT 41 — Leaderboard API: Core Service + WebSocket Hub

```
Implement services/leaderboard-api/ — the real-time leaderboard backend.

FILE: services/leaderboard-api/main.go
Config:
  Port (default 8084), RedisURL, TimescaleDSN,
  UpdateIntervalMs (default 500), WebSocketPingIntervalSec (default 30),
  MaxWebSocketConnections (default 10000), OrchestratorDBDSN

Start:
  1. Redis client
  2. Postgres pool
  3. WebSocket Hub (manages all connections)
  4. Scorer goroutine (reads Redis, computes scores, pushes to Hub)
  5. HTTP server

FILE: services/leaderboard-api/hub/websocket_hub.go

WebSocketHub manages all connected browser clients.

The pub/sub fan-out design:
  - One "publisher" goroutine pushes LeaderboardUpdate messages
  - Hub delivers to ALL connected clients simultaneously

Fields:
  clients       map[*Client]bool  // all connected WebSocket clients
  register      chan *Client       // new clients send themselves here
  unregister    chan *Client       // disconnecting clients
  broadcast     chan []byte        // messages to send to all clients
  mu            sync.RWMutex      // protect clients map reads
  maxClients    int

Client struct:
  hub     *WebSocketHub
  conn    *websocket.Conn
  send    chan []byte  // buffered channel: 256 messages
  id      string      // UUID for logging

Hub.Run():
  for {
    select {
    case client := <-register:
      // Add to clients map
      // If len(clients) > maxClients: reject with 503 message, unregister immediately
    case client := <-unregister:
      // Remove from clients map
      // Close client.send channel (signals writePump to exit)
    case message := <-broadcast:
      // Send to ALL clients
      for client := range clients {
        select {
        case client.send <- message:
          // delivered
        default:
          // Client's send buffer full — they're too slow
          // Close and remove (don't block the broadcast for one slow client)
          close(client.send)
          delete(clients, client)
        }
      }
    }
  }

Client.readPump():
  Read loop: handles ping/pong, disconnection detection.
  Set read deadline: if no ping received in 60s, close connection.
  On close: send to hub.unregister

Client.writePump():
  Write loop: reads from client.send channel, writes to WebSocket.
  Send ping every 30s (WebSocket heartbeat).
  On error: return (readPump will unregister).

FILE: services/leaderboard-api/hub/websocket_handler.go

ServeWS(hub *WebSocketHub, w, r):
  Upgrade HTTP to WebSocket using gorilla/websocket.
  Create Client, register with hub.
  Start readPump and writePump goroutines.
  Immediately send current leaderboard state as first message (sync on connect).

�� SMART: The "default: close and remove slow client" logic in Hub.Run() is critical.
Without it, one slow client with a congested network causes the entire broadcast to
block, starving all other clients. Dropping slow clients is the right call — they'll
reconnect and get fresh data.

Output complete, compilable Go code.
```

---

### PROMPT 42 — Leaderboard API: Scorer + Redis Publisher

```
Implement the scoring engine for services/leaderboard-api.

FILE: services/leaderboard-api/scorer/scorer.go

Scorer reads metrics from Redis every 500ms and computes the ranked leaderboard.

Fields:
  redis       *redis.Client
  hub         *WebSocketHub
  ticker      *time.Ticker
  cachedBoard []LeaderboardEntry
  cacheTTL    time.Time
  mu          sync.RWMutex

Run(ctx):
  ticker := time.NewTicker(500 * time.Millisecond)
  for {
    select {
    case <-ticker.C:
      board, err := computeLeaderboard(ctx)
      if err != nil {
        log.Error("scorer error", "err", err)
        continue  // use cached board on error
      }
      data, _ := json.Marshal(LeaderboardUpdate{
        Timestamp: time.Now().UnixMilli(),
        Entries:   board,
      })
      hub.broadcast <- data  // push to all WebSocket clients
      
      // Also cache in Redis for REST API fallback
      redis.Set(ctx, "leaderboard:cached", data, 2*time.Second)
    case <-ctx.Done():
      return
    }
  }

computeLeaderboard(ctx) ([]LeaderboardEntry, error):
  
  Step 1: Get all active contestant IDs
  members, err := redis.SMembers(ctx, "leaderboard:active_contestants")
  
  Step 2: Fetch metrics for all contestants in ONE Redis round-trip using pipeline
  pipe := redis.Pipeline()
  cmds := make([]*redis.MapStringStringCmd, len(members))
  for i, id := range members {
    cmds[i] = pipe.HGetAll(ctx, "metrics:"+id)
  }
  pipe.Exec(ctx)
  
  Step 3: Parse metrics for each contestant
  contestants := make([]ContestantMetrics, 0, len(members))
  for i, cmd := range cmds {
    data := cmd.Val()
    if len(data) == 0 { continue }  // contestant has no metrics yet
    contestants = append(contestants, parseMetrics(members[i], data))
  }
  
  Step 4: Normalize scores (see scoring-formula.md)
  allTPS := extract TPS values
  allP99 := extract P99 values
  minTPS, maxTPS := min/max of allTPS
  minP99, maxP99 := min/max of allP99
  
  for each contestant:
    normalizedTPS = normalize(contestant.TPS, minTPS, maxTPS)
    normalizedInvP99 = 1.0 - normalize(contestant.P99, minP99, maxP99)
    score = 0.40*normalizedTPS + 0.40*normalizedInvP99 + 0.20*contestant.CorrectnessRate
    score = clamp(score * 100, 0, 100)  // convert to 0-100 scale
  
  Step 5: Sort by score desc, assign ranks (handle ties)
  sort.Slice(contestants, func(i, j int) bool {
    if contestants[i].Score != contestants[j].Score {
      return contestants[i].Score > contestants[j].Score
    }
    // Tiebreaker 1: correctness rate
    if contestants[i].CorrectnessRate != contestants[j].CorrectnessRate {
      return contestants[i].CorrectnessRate > contestants[j].CorrectnessRate
    }
    // Tiebreaker 2: p99 latency (lower is better)
    return contestants[i].P99 < contestants[j].P99
  })
  
  rank := 1
  for i, c := range contestants {
    if i > 0 && contestants[i-1].Score != c.Score {
      rank = i + 1  // skip ranks for ties
    }
    contestants[i].Rank = rank
  }
  
  return contestants, nil

Output complete, compilable Go code.
```

---

### PROMPT 43 — Leaderboard API: REST Endpoints + Redis Sub

```
Implement REST endpoints and Redis pub/sub for services/leaderboard-api.

FILE: services/leaderboard-api/api/handlers.go

GetLeaderboard(w, r):
  Try: read "leaderboard:cached" from Redis (set by scorer every 500ms).
  If found and fresh: return immediately.
  If not found or stale: call scorer.computeLeaderboard() directly, return.
  Response: { "updated_at": ms, "entries": [...] }

GetContestantHistory(w, r):
  Path param: contestant_id
  Query params: hours (default 24), resolution (1m/5m/1h)
  
  Fetches from TimescaleDB:
  SELECT time_bucket($resolution, time) AS bucket,
    percentile_cont(0.99) WITHIN GROUP (ORDER BY latency_us) AS p99,
    COUNT(*) / EXTRACT(EPOCH FROM $resolution) AS tps
  FROM latency_samples
  WHERE contestant_id = $1 AND time > NOW() - INTERVAL '$hours hours'
  GROUP BY bucket ORDER BY bucket
  
  Return: { "contestant_id": "...", "history": [ {time, p99, tps} ] }

GetAnomalies(w, r):
  Path param: contestant_id
  Read from Redis: LRANGE anomalies:{contestant_id} 0 99
  Parse and return list of anomaly reports.

FILE: services/leaderboard-api/pubsub/redis_subscriber.go

Multi-instance leaderboard deployment needs all instances to deliver updates.

When deploying 3 leaderboard-api pods (for 10K+ clients):
- The scorer runs on one pod, but needs to reach ALL pods' WebSocket hubs.
- Solution: Redis Pub/Sub as the fan-out mechanism.

RedisSubscriber subscribes to "leaderboard:updates" channel.
When the scorer publishes an update to this channel (instead of directly to hub),
ALL leaderboard-api pods receive it and broadcast to their own clients.

Implement:
1. Publisher: score.computeLeaderboard() publishes JSON to Redis channel
   redis.Publish(ctx, "leaderboard:updates", data)
   
2. Subscriber (runs in each pod): 
   sub := redis.Subscribe(ctx, "leaderboard:updates")
   for msg := range sub.Channel() {
     hub.broadcast <- []byte(msg.Payload)
   }

FILE: services/leaderboard-api/api/websocket_upgrade.go
ServeWS with proper origin checking:
- In production: only allow upgrades from known frontend domain
- In development: allow all origins
- Set proper WebSocket headers (Upgrade, Connection, Sec-WebSocket-Accept)

�� SMART: The Redis Pub/Sub fan-out is what makes the leaderboard horizontally
scalable to 10,000+ concurrent viewers. Without it, only one pod can broadcast
(the pod running the scorer). With Redis Pub/Sub: add more leaderboard-api pods
behind a load balancer, each subscribes independently, all receive every update.
This is the exact architecture Discord uses for message fan-out at scale.

Output complete, compilable Go code.
```

---

### PROMPT 44 — Leaderboard API: Full Test Suite

```
Create comprehensive tests for services/leaderboard-api.

FILE: services/leaderboard-api/scorer_test.go

TestComputeLeaderboard_Rankings:
Setup: write mock metrics to a test Redis instance for 5 contestants:
  Alice: TPS=9000, P99=450µs, Correctness=0.999
  Bob:   TPS=7500, P99=380µs, Correctness=0.998
  Carol: TPS=11000, P99=620µs, Correctness=0.995
  Dave:  TPS=8000, P99=900µs, Correctness=0.990
  Eve:   TPS=5000, P99=1200µs, Correctness=0.999

Assertions:
- Carol should rank #1 or #2 (highest TPS despite higher latency)
- Bob should score well (low latency compensates for lower TPS)
- Eve should rank last (lowest TPS AND high latency)
- All scores in [0, 100] range
- No two entries have same rank (no ties in this dataset)

TestComputeLeaderboard_TieBreaker:
  Two contestants: same TPS, same P99, Alice=100% correct, Bob=99% correct
  Assert Alice ranks higher (correctness tiebreaker)

TestComputeLeaderboard_SingleContestant:
  Only one contestant → normalized score should be meaningful
  When min==max for TPS, normalized_tps = 1.0 (give benefit of doubt)

FILE: services/leaderboard-api/websocket_test.go

TestWebSocketHub_BroadcastToMultipleClients:
  Create mock WebSocket connections (using gorilla/websocket test client)
  Register 100 clients
  Broadcast one update
  Assert all 100 clients receive the update within 1 second

TestWebSocketHub_SlowClientDropped:
  Register client with very small send buffer (1 message)
  Flood hub with 100 broadcasts
  Assert: slow client is eventually disconnected (not blocking other clients)
  Assert: fast clients receive all 100 messages

TestWebSocketHub_Reconnect:
  Connect client
  Disconnect (simulate network drop)
  Reconnect
  Assert: client receives full current state on reconnect (not a delta)

TestWebSocketHub_MaxConnections:
  Register clients up to MaxWebSocketConnections
  Try to register one more
  Assert: 503 response for the extra client

FILE: services/leaderboard-api/api_test.go

TestGetLeaderboard_REST:
  Write data to Redis, GET /v1/leaderboard
  Assert: correct JSON structure, entries sorted by score

TestGetLeaderboard_Caching:
  Call GET /v1/leaderboard twice in quick succession
  Assert: second call is faster (served from Redis cache)
  Assert: second call returns identical data (not recomputed)

Output complete, compilable Go test code.
```

---

### PROMPT 45 — Leaderboard API: Kubernetes + Redis Scaling

```
Create production deployment configuration for leaderboard-api.

FILE: infra/k8s/leaderboard-api/deployment.yaml
replicas: 3
strategy: RollingUpdate (maxUnavailable: 1)
resources:
  requests: { cpu: "200m", memory: "128Mi" }
  limits: { cpu: "1000m", memory: "256Mi" }

Important: PodAntiAffinity to spread across nodes.
If all 3 pods are on one node and that node dies, all 10K WebSocket clients disconnect.

FILE: infra/k8s/leaderboard-api/service.yaml
Service type: LoadBalancer (for external access by browsers)
Port: 80 → 8084 (HTTP/WebSocket)
Annotations: service.beta.kubernetes.io/aws-load-balancer-type: "nlb"
  (NLB instead of ALB — NLB handles WebSocket connections better, no idle timeout)

FILE: infra/k8s/leaderboard-api/ingress.yaml
Ingress with WebSocket support:
nginx annotations:
  nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"  ← allow 1hr WS connections
  nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"
  nginx.ingress.kubernetes.io/use-regex: "true"
Rules:
  - host: leaderboard.trade-eval.com
  - path: /ws → leaderboard-api service (WebSocket upgrade)
  - path: /v1/* → leaderboard-api service (REST)
  - path: /* → frontend service (static assets)

FILE: infra/k8s/leaderboard-api/pdb.yaml
PodDisruptionBudget:
  minAvailable: 2
Ensures at most 1 pod can be down during node drains.
With 3 pods and minAvailable=2: Kubernetes guarantees 2 pods always serve traffic.

FILE: infra/k8s/leaderboard-api/hpa.yaml
Scale on: 
  - CPU utilization > 60% (each WebSocket connection = CPU usage for ping/pong)
  - Custom metric: websocket_active_connections > 3000 per pod
    (scale before hitting 10K limit per pod)
minReplicas: 3
maxReplicas: 20

�� SMART: NLB (Network Load Balancer) vs ALB (Application Load Balancer) for WebSocket:
ALBs have a 60s idle timeout that drops WebSocket connections with no activity.
NLBs are Layer 4 (TCP) — they don't interpret HTTP, so no idle timeout problem.
For a leaderboard with 30s ping intervals, ALB drops connections; NLB doesn't.
This is a real production gotcha that kills many WebSocket deployments.

Output complete Kubernetes YAML.
```

---

### PROMPT 46 — Leaderboard API: Rate Limiting + DDoS Protection

```
Add rate limiting and DDoS protection to the leaderboard API.

FILE: services/leaderboard-api/middleware/ratelimit.go

Implement rate limiting using Redis token bucket algorithm.

RateLimiter middleware for REST endpoints:
- Key: IP address (X-Forwarded-For header, or remote addr)
- Limit: 60 requests per minute per IP for REST endpoints
- Limit: 10 WebSocket connections per IP (prevent connection exhaustion)
- Algorithm: sliding window counter in Redis
  Key: ratelimit:{endpoint}:{ip}:{minute_bucket}
  INCR the key, EXPIRE 120s
  If count > limit: return 429 Too Many Requests with Retry-After header

WebSocket connection limit per IP:
INCR ws_connections:{ip}
If > 10: refuse upgrade with 429
On disconnect: DECR ws_connections:{ip} (use defer in handler)

FILE: services/leaderboard-api/middleware/cors.go
Proper CORS handling:
- Allowed origins: configurable list (default: frontend URL)
- Allow credentials: false (leaderboard is public)
- Cache preflight: Access-Control-Max-Age: 86400 (1 day)

FILE: services/leaderboard-api/middleware/auth.go (optional)
If REQUIRE_AUTH=true in config:
  - Require X-Contestant-Token for accessing own historical data
  - Leaderboard itself is always public (no auth)

FILE: infra/k8s/leaderboard-api/network-policy.yaml
NetworkPolicy for leaderboard-api:
- ALLOW ingress: from ingress-nginx namespace (external traffic)
- ALLOW ingress: from trade-eval namespace (internal service calls)
- ALLOW egress: to Redis (6379)
- ALLOW egress: to TimescaleDB (5432)
- DENY all other traffic

FILE: infra/docker/nginx/nginx.conf
Nginx config for rate limiting at the load balancer level (before reaching the app):
  limit_req_zone $binary_remote_addr zone=api:10m rate=100r/m;
  limit_req_zone $binary_remote_addr zone=ws:10m rate=10r/m;
  
  location /ws {
    limit_req zone=ws burst=5 nodelay;
    proxy_pass http://leaderboard-api;
    proxy_http_version 1.1;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection "upgrade";
    proxy_read_timeout 3600s;
  }
  
  location /v1 {
    limit_req zone=api burst=20 nodelay;
    proxy_pass http://leaderboard-api;
  }

Output complete Go code and configuration files.
```

---

### PROMPT 47 — Leaderboard API: Admin Panel Endpoints

```
Add admin-only endpoints to services/leaderboard-api for contest management.

FILE: services/leaderboard-api/admin/handlers.go

These endpoints require ADMIN_API_KEY header (a separate, more privileged key).

POST /admin/v1/tests/cancel
Request: { "test_id": "..." }
- Look up test in orchestrator DB
- Publish STOP_TEST event to Kafka
- Return 200 if event published

POST /admin/v1/contestants/disqualify
Request: { "contestant_id": "...", "reason": "..." }
- Set Redis key: disqualified:{contestant_id} = reason
- Remove from leaderboard:active_contestants
- Update all clients via WebSocket broadcast (special DQ_UPDATE event)
- Log to Postgres audit table

POST /admin/v1/leaderboard/freeze
Request: {} (no body)
- Set Redis key: leaderboard:frozen = 1 EX 86400
- Scorer checks this flag: if frozen, stop computing new scores, serve last cached
- Use case: freeze leaderboard at competition end while calculating final results

POST /admin/v1/leaderboard/unfreeze
- DEL Redis key: leaderboard:frozen

GET /admin/v1/system/status
Returns:
{
  "kafka": { "lag": 12345, "healthy": true },
  "redis": { "memory_mb": 234, "connected_clients": 10432 },
  "timescaledb": { "disk_usage_gb": 45.2, "connections": 8 },
  "active_tests": 3,
  "active_containers": 8,
  "total_events_processed": 45230000,
  "websocket_clients": 10432
}
Reads from each service's metrics endpoints and aggregates.

FILE: services/leaderboard-api/admin/middleware.go
AdminAuth middleware:
- Reads X-Admin-Key header
- Compares against ADMIN_API_KEY env var using constant-time comparison (crypto/subtle)
- Returns 401 if invalid
- Logs all admin actions with IP, timestamp, action, actor key prefix (last 4 chars)

FILE: services/leaderboard-api/admin/admin_test.go
TestAdminAuth_ValidKey: valid key → 200
TestAdminAuth_InvalidKey: invalid key → 401
TestAdminAuth_TimingAttack: verify constant-time comparison (use timing test)
TestFreezeLeaderboard: freeze → scorer stops updating → unfreeze → updates resume

Output complete, compilable Go code.
```

---

## PHASE 6 — REACT FRONTEND (Prompts 48–57)

---

### PROMPT 48 — Frontend: Project Structure + Design System

```
Set up the React + TypeScript frontend for trade-eval-platform.

The frontend is a real-time competition leaderboard with a trading/financial aesthetic.
Design direction: dark mode, monospace numbers, Bloomberg Terminal inspired but modern.

FILE: frontend/package.json
Dependencies:
{
  "react": "^18.3.0",
  "react-dom": "^18.3.0",
  "react-router-dom": "^6.23.0",
  "@tanstack/react-query": "^5.40.0",
  "zustand": "^4.5.0",
  "recharts": "^2.12.0",
  "clsx": "^2.1.1",
  "date-fns": "^3.6.0",
  "react-hot-toast": "^2.4.1"
}
devDependencies:
{
  "typescript": "^5.4.0",
  "vite": "^5.2.0",
  "@vitejs/plugin-react": "^4.3.0",
  "tailwindcss": "^3.4.0",
  "autoprefixer": "^10.4.0",
  "postcss": "^8.4.0",
  "vitest": "^1.6.0",
  "@testing-library/react": "^16.0.0"
}

FILE: frontend/tailwind.config.ts
Extend theme with trading-specific design tokens:
colors:
  surface: {
    primary: '#0a0a0a',      // near-black background
    secondary: '#111111',    // card backgrounds
    tertiary: '#1a1a1a',     // borders
    hover: '#222222',        // hover states
  }
  text:
    primary: '#e8e8e8',      // main text
    secondary: '#888888',    // muted text
    muted: '#555555',        // very muted
  accent:
    green: '#00d1a0',        // gains, positive (trading green)
    red: '#ff4d6a',          // losses, negative (trading red)
    yellow: '#fbbf24',       // warnings
    blue: '#3b82f6',         // info, links
    purple: '#a78bfa',       // rankings, scores
  mono: '#00d1a0'            // monospace text color

fontFamily:
  mono: ['JetBrains Mono', 'Fira Code', 'monospace']  // for numbers
  sans: ['Inter', 'system-ui', 'sans-serif']

FILE: frontend/src/styles/globals.css
Import JetBrains Mono from Google Fonts.
Set body background to surface.primary.
Define CSS custom properties for animation durations.
Scrollbar styling: thin, dark track, green thumb.

FILE: frontend/src/lib/websocket.ts
WebSocket manager with reconnection:
- Connects to ws://localhost:8084/ws (or env VITE_WS_URL)
- Exponential backoff reconnection: 1s, 2s, 4s, 8s, 16s, 30s max
- On connect: emit 'connected' event
- On disconnect: emit 'disconnected', start reconnect timer
- On message: emit 'message' with parsed JSON
- Expose: connect(), disconnect(), onMessage(handler), status ('connecting'|'connected'|'disconnected')
- Implement as a singleton (one connection for the whole app)

FILE: frontend/src/store/leaderboard.ts
Zustand store for leaderboard state:
- entries: LeaderboardEntry[]
- lastUpdated: number (timestamp)
- connectionStatus: 'connecting' | 'connected' | 'disconnected'
- previousEntries: LeaderboardEntry[] (for rank change animation)
- setEntries(entries): compare with previousEntries, set rankChange per entry
- updateConnectionStatus(status)
- actions: fetchLeaderboard() (REST fallback), connectWebSocket()

Output all files with complete, production-quality code.
```

---

### PROMPT 49 — Frontend: Leaderboard Table Component

```
Implement the main leaderboard table component for the frontend.

FILE: frontend/src/components/LeaderboardTable/LeaderboardTable.tsx

The main leaderboard showing all contestants ranked live.

Design: Bloomberg Terminal meets GitHub contributions.
- Dark background (#0a0a0a)
- JetBrains Mono for all numbers
- Green for gains, red for performance drops
- Rank position changes animated with CSS transitions

Types (in frontend/src/types/leaderboard.ts):
interface LeaderboardEntry {
  rank: number;
  previousRank?: number;
  contestantId: string;
  contestantName: string;
  score: number;           // 0-100
  p50Us: number;           // microseconds
  p90Us: number;
  p99Us: number;
  tps: number;             // orders per second
  correctnessRate: number; // 0-1
  status: 'running' | 'completed' | 'failed' | 'idle';
  rankChange?: number;     // positive = improved, negative = dropped
}

Component structure:

<LeaderboardTable>
  <TableHeader> — sticky header with column labels and sort controls
  <TableBody>
    {entries.map(entry => <LeaderboardRow key={entry.contestantId} entry={entry} />)}
  </TableBody>
</LeaderboardTable>

LeaderboardRow component:
- Entire row slides up/down when rank changes (CSS transition: transform 0.4s ease)
- RankBadge: shows rank number + arrow indicator (↑ if improved, ↓ if dropped)
- ScoreBar: a horizontal bar chart inline showing the score as a filled bar
  (like a mini progress bar, filled in accent.purple)
- LatencyCell: shows p50/p90/p99 stacked vertically in monospace
  Color coding: green if < 500µs, yellow if < 2ms, red if > 2ms
- TPSCell: shows current TPS with trend sparkline (tiny SVG)
- CorrectnessCell: shows percentage, red if < 99%
- StatusBadge: pill showing running/completed/failed with appropriate color

Animations:
- On mount: rows fade in from bottom with staggered delay (row[0] at 0ms, row[1] at 50ms, etc.)
- On rank change: row smoothly moves to new position using CSS translate
  (use FLIP animation technique: First, Last, Invert, Play)
- On new entry appearing: flash green border for 1 second

FILE: frontend/src/components/LeaderboardTable/LeaderboardRow.tsx
Individual row component. Accepts entry: LeaderboardEntry.
Use React.memo to prevent re-renders when entry hasn't changed.
Track previous rank with useRef to compute animation direction.

FILE: frontend/src/components/LeaderboardTable/columns.ts
Column definitions (width, label, alignment, sortable):
- Rank: 60px, center, sortable
- Contestant: 200px, left, sortable
- Score: 120px, center, sortable (default sort)
- P99 Latency: 100px, right, sortable
- TPS: 100px, right, sortable
- Correctness: 100px, right, sortable
- Status: 80px, center
- Trend: 120px (sparkline chart)

Output complete, production-quality TypeScript + React code.
```

---

### PROMPT 50 — Frontend: Live Charts Components

```
Implement real-time chart components for the frontend.

FILE: frontend/src/components/Charts/LatencyChart.tsx

P50/P90/P99 latency bar chart per contestant (using Recharts).

Props: { contestantId: string, width?: number, height?: number }

Fetches data from /v1/metrics/{contestantId}/latency via @tanstack/react-query.
Refetches every 2 seconds for live updates.

Chart type: Grouped bar chart with 3 bars per time bucket (p50, p90, p99).

X-axis: time (last 5 minutes, 10-second buckets)
Y-axis: latency in microseconds
Colors:
  p50: accent.green (#00d1a0)
  p90: accent.yellow (#fbbf24)
  p99: accent.red (#ff4d6a)

Custom tooltip showing exact values and percentage.
Reference lines: 500µs, 1000µs, 5000µs (industry benchmarks).
Animate bar entry: isAnimationActive={true} animationDuration={300}

FILE: frontend/src/components/Charts/TPSChart.tsx

TPS (throughput) over time line chart.

Single line showing orders/second.
X-axis: time (last 5 minutes)
Y-axis: TPS
Color: accent.green, filled area below line (AreaChart variant)
Smooth line: type="monotone" in Recharts

Show peak TPS annotation: dotted line at peak, label "Peak: {n} OPS"

FILE: frontend/src/components/Charts/CorrectnessGauge.tsx

Circular gauge showing correctness percentage.

Implement using a custom SVG arc (NOT a third-party gauge library).

SVG arc from 0° to 360° representing 0% to 100%.
Filled portion: accent.green for > 99%, accent.yellow for 95-99%, accent.red for < 95%.
Center text: "{value}%" in large monospace font.
Animate: stroke-dashoffset transition when value changes.

FILE: frontend/src/components/Charts/LatencyHeatmap.tsx

A heatmap showing latency distribution over time.
X-axis: time (1-minute buckets over last hour)
Y-axis: latency buckets (0-100µs, 100-500µs, 500-1000µs, 1-5ms, 5-10ms, 10ms+)
Color intensity: green (low count) → yellow → red (high count)

Implementation: Custom SVG grid (Recharts doesn't have a good heatmap).
Grid cells colored by count using d3-scale-chromatic color interpolation:
  import { interpolateRdYlGn } from 'd3-scale-chromatic'

FILE: frontend/src/components/Charts/SparklineChart.tsx
A tiny inline trend chart used in the leaderboard table rows.
Props: { data: number[], width: 120, height: 30, color: string }
Uses Recharts LineChart with no axes, no grid, no labels.
Just the line, filled area, no padding.

Output complete, production-quality TypeScript + React code.
```

---

### PROMPT 51 — Frontend: Submission Upload Page

```
Implement the submission upload page for contestants.

FILE: frontend/src/pages/Submit/SubmitPage.tsx

A full submission workflow page with:
1. File upload with drag-and-drop
2. Language selector
3. Real-time build status polling
4. Build log viewer
5. Test trigger button

Steps:
STEP 1 — Upload Form:
  - Drag and drop zone (dashed border, turns solid green on drag-over)
  - File input (hidden, triggered by clicking the drop zone)
  - Accept: .zip, .cpp, .rs, .go files
  - Language radio buttons: C++, Rust, Go, Python
    Auto-detect from file extension if possible
  - "Upload" button → disabled until file selected
  - Show file name and size after selection
  - Progress indicator during upload (track with XMLHttpRequest upload.progress)

STEP 2 — Build Progress (shown after upload):
  - Status badge: PENDING → BUILDING → READY/FAILED
  - Animated spinner while BUILDING
  - Build log accordion: collapses by default, expands to show raw compiler output
    (log is fetched from GET /v1/submissions/{id}/logs every 3s while building)
  - On READY: show green check, enable "Run Test" button
  - On FAILED: show red X, display error log prominently, "Try Again" button

STEP 3 — Test Configuration (shown after READY):
  - Duration selector: 30s (dev), 2m, 5m (competition)
  - Bot count: 10, 100, 500 (competition default)
  - Bot personas: checkboxes for each (market_maker, aggressive_taker, spammer, whale)
  - "Start Test" button

STEP 4 — Test Progress (shown after test starts):
  - Real-time metrics updating every 2s (polling GET /v1/tests/{id})
  - Mini latency chart and TPS number updating live
  - Progress bar showing test duration elapsed
  - On completion: "View Full Results" button → navigate to results page

FILE: frontend/src/components/FileUpload/DropZone.tsx
Reusable drag-and-drop file upload component.
Props: { onFile: (file: File) => void, accept: string, maxSizeMB: number }
Visual states: idle, dragover, uploading, success, error.

FILE: frontend/src/hooks/useSubmissionStatus.ts
Custom hook that polls submission status:
- useQuery with refetchInterval: 2000 when status is 'pending' or 'building'
- Stops polling when status is 'ready' or 'failed'

Output complete, production-quality TypeScript + React code.
```

---

### PROMPT 52 — Frontend: Test Results Dashboard

```
Implement the detailed test results dashboard.

FILE: frontend/src/pages/Results/ResultsPage.tsx

Shows comprehensive results for a specific test.
Route: /results/:testId

Layout: two-column grid on desktop, single column on mobile.

Left column:
  - Score breakdown card: composite_score, with visual breakdown of its components
    (three bars: TPS contribution, Latency contribution, Correctness contribution)
  - Rank and comparison: "Your rank: #3 of 8 contestants"
  - Key metrics cards: P50, P90, P99, Peak TPS, Avg TPS, Correctness %

Right column:
  - Latency distribution chart (LatencyChart for this test's duration)
  - TPS chart (throughput over time)
  - Correctness chart (correct orders % over time)

Bottom section:
  - Per-persona breakdown table:
    Columns: Persona, Orders Sent, Avg Latency, P99, Correctness %
    Rows: one per bot persona used in the test

  - Anomalies panel (shown only if anomalies detected):
    List of anomaly reports with type, severity, description
    Severity color coding: INFO=blue, WARNING=yellow, CRITICAL=red

  - Test metadata: start time, end time, duration, bot count, container info

FILE: frontend/src/pages/Results/ScoreBreakdown.tsx
Visual score breakdown showing how the composite score was computed:

  Composite Score: 73.4
  ┌─────────────────────────────────────────────────────┐
  │ TPS Score       ████████░░░░  40% weight  Score: 68│
  │ Latency Score   █████████░░░  40% weight  Score: 71│
  │ Correctness     ████████████  20% weight  Score: 99│
  └─────────────────────────────────────────────────────┘

Use inline SVG for the bars (not a library).
Show rank context: "Your p99 (450µs) was 3rd best. Best was 280µs."

FILE: frontend/src/pages/Results/PersonaBreakdown.tsx
Table showing per-persona performance.
Sortable by any column.
Row highlighting: worst-performing persona in red.

Output complete, production-quality TypeScript + React code.
```

---

### PROMPT 53 — Frontend: Live Leaderboard Page

```
Implement the main live leaderboard page — the showpiece of the frontend.

FILE: frontend/src/pages/Leaderboard/LeaderboardPage.tsx

This is the screen shown on the contest projector for the audience.
Full-screen, auto-updating, visually impressive.

Layout:
  - Top bar: "�� Live Leaderboard" title + connection status indicator + 
    last-updated timestamp ("Updated 0.5s ago")
  - Contest timer (countdown if contest has an end time, or elapsed timer)
  - Main LeaderboardTable (full width, fills remaining height)
  - Bottom ticker: scrolling banner of recent events 
    ("Alice just hit 9,200 TPS!", "Bob achieved 99.9% correctness!")

Connection status indicator:
  - Green dot (pulsing) = connected
  - Yellow dot = reconnecting
  - Red dot = disconnected
  On disconnect: toast notification "Connection lost — reconnecting..."
  On reconnect: toast notification "Reconnected ✓"

Rank change animations (CSS keyframes):
@keyframes rankUp {
  0% { background: rgba(0, 209, 160, 0.2) }
  100% { background: transparent }
}
@keyframes rankDown {
  0% { background: rgba(255, 77, 106, 0.2) }
  100% { background: transparent }
}

FILE: frontend/src/pages/Leaderboard/ContestTimer.tsx
Shows elapsed time or countdown.
Props: { startTime: number, endTime?: number, mode: 'elapsed'|'countdown' }
Updates every second using setInterval.
Displays as: HH:MM:SS in large monospace font.
Changes color: green when time remaining > 10min, yellow < 10min, red < 1min.

FILE: frontend/src/pages/Leaderboard/EventTicker.tsx
Scrolling news ticker at the bottom showing significant events.

Events to display:
- Rank changes: "▲ Alice moved to #1!"
- Milestones: "Bob just hit 10,000 TPS!"
- New submissions: "Carol submitted a new version"
- Test completions: "Dave's test completed — score: 84.2"
- Anomalies: "⚠ Suspicious pattern detected for Eve"

Events are pushed via WebSocket (special event_type: "TICKER_EVENT" from leaderboard-api).
Display: scroll left like a stock ticker.
CSS animation: @keyframes ticker-scroll { from { transform: translateX(100%) } to { transform: translateX(-100%) } }

FILE: frontend/src/hooks/useWebSocket.ts
Custom hook wrapping the WebSocket singleton.
Subscribes to messages, returns { status, lastMessage, reconnect }.
Properly cleans up subscriptions on unmount.

Output complete, production-quality TypeScript + React code.
```

---

### PROMPT 54 — Frontend: Authentication + API Client

```
Implement authentication and API client for the frontend.

FILE: frontend/src/lib/api.ts
Typed API client using fetch (no additional HTTP library needed).

Base URL from: import.meta.env.VITE_API_URL || 'http://localhost:8080'

API functions (all return typed responses, throw on non-2xx):

createSubmission(file: File, language: string, apiKey: string): Promise<CreateSubmissionResponse>
  - Uses FormData
  - Sets X-API-Key header
  - Returns { submission_id, status }

getSubmission(id: string, apiKey: string): Promise<Submission>
getSubmissionLogs(id: string, apiKey: string): Promise<{ logs: string }>

createTest(submissionId: string, config: TestConfig, apiKey: string): Promise<{ test_id: string }>
getTest(id: string, apiKey: string): Promise<Test>

getLeaderboard(): Promise<LeaderboardResponse>  (no auth, public)
getContestantHistory(contestantId: string): Promise<HistoryResponse>

All functions: include proper TypeScript return types, handle fetch errors,
parse error bodies into typed ApiError objects.

FILE: frontend/src/lib/auth.ts
Simple API key authentication (no OAuth, no JWT — this is a hackathon).

Storage: localStorage key "trade_eval_api_key" (with clear warning: don't use for production)
Functions:
  getApiKey(): string | null
  setApiKey(key: string): void
  clearApiKey(): void
  isAuthenticated(): boolean

FILE: frontend/src/pages/Auth/LoginPage.tsx
Simple login page: just an API key input field.
Label: "Enter your contestant API key"
Input: password type (hidden)
Submit: validate the key by calling GET /v1/health (liveness) and then
  GET /v1/submissions?limit=1 to verify the key is valid.
On valid: save to auth.setApiKey(), redirect to /submit

FILE: frontend/src/components/Layout/AppLayout.tsx
Main app layout with:
- Sidebar: navigation links (Submit, My Tests, Leaderboard, My Results)
- API key indicator in header (last 4 chars of key, click to change)
- ConnectionStatus indicator (WebSocket status)
- Toast notification container (react-hot-toast)

FILE: frontend/src/App.tsx
Router setup with react-router-dom v6:
Routes:
  / → redirect to /leaderboard
  /leaderboard → LeaderboardPage (public, no auth)
  /submit → SubmitPage (requires auth)
  /results/:testId → ResultsPage (requires auth)
  /login → LoginPage

ProtectedRoute component: redirects to /login if not authenticated.

Output complete, production-quality TypeScript + React code.
```

---

### PROMPT 55 — Frontend: Mobile Responsive + PWA

```
Add mobile responsiveness and PWA support to the frontend.

FILE: frontend/src/styles/responsive.css
Media queries for mobile (< 768px), tablet (768-1024px), desktop (> 1024px).

Mobile layout changes:
- LeaderboardTable: hide P50/P90 columns, keep only Rank, Name, Score, P99, Status
- Charts: full width, reduced height (200px instead of 300px)
- AppLayout: bottom navigation bar instead of sidebar
  (icons: Trophy=Leaderboard, Upload=Submit, Chart=My Results)
- Submit page: steps become vertical accordion

Tablet layout:
- AppLayout: collapsed sidebar (icons only, expand on hover)
- Charts: 2-column grid

FILE: frontend/vite.config.ts
Add vite-plugin-pwa for Progressive Web App support:
- manifest: {
    name: "Trade Eval Platform",
    short_name: "TradeEval",
    theme_color: "#0a0a0a",
    background_color: "#0a0a0a",
    display: "standalone",
    icons: [{ src: "/icon-192.png", sizes: "192x192" }]
  }
- workbox strategy: NetworkFirst for API calls, CacheFirst for static assets

FILE: frontend/public/manifest.json
PWA manifest with all required fields.

FILE: frontend/src/components/UI/index.ts
Export barrel for all UI components:
- Button: variants (primary, secondary, danger, ghost)
- Badge: variants (success, warning, danger, info, neutral)
- Card: base card with optional header
- Spinner: loading indicator
- Tooltip: hover tooltip
- EmptyState: when no data to show

FILE: frontend/src/components/UI/Button.tsx
Accessible button component with:
- Variants: primary (green filled), secondary (bordered), ghost (no border)
- Sizes: sm, md, lg
- States: loading (shows spinner, disables click), disabled
- Keyboard navigation: proper focus styles

FILE: frontend/src/components/ErrorBoundary.tsx
React error boundary that catches component errors:
- Shows a friendly error message instead of white screen
- "Try Again" button that resets the error boundary
- Logs errors to console in dev, would send to error tracker in prod

Output complete, production-quality TypeScript + React code.
```

---

### PROMPT 56 — Frontend: E2E Tests + Vitest Setup

```
Create tests for the React frontend.

FILE: frontend/src/test/setup.ts
Vitest + Testing Library setup:
- Import @testing-library/jest-dom for custom matchers
- Mock WebSocket API (since jsdom doesn't have it)
- Mock fetch (msw — Mock Service Worker for API mocking)
- Set up mock API handlers for all endpoints

FILE: frontend/src/test/mocks/handlers.ts
MSW request handlers for all API endpoints:
- GET /v1/leaderboard → returns sample 5-contestant leaderboard
- POST /v1/submissions → returns { submission_id: "sub_test", status: "pending" }
- GET /v1/submissions/:id → cycles through status updates
- GET /v1/tests/:id → returns completed test with metrics

FILE: frontend/src/components/LeaderboardTable/__tests__/LeaderboardTable.test.tsx
Tests:
- Renders all contestant entries
- Sorts by score by default (highest first)
- Shows correct rank numbers
- Displays green for latency < 500µs, red for > 2ms
- Rank change animation triggers on rank position change
- Mobile: P50/P90 columns hidden on narrow viewport

FILE: frontend/src/pages/Submit/__tests__/SubmitPage.test.tsx
Tests:
- File drop zone accepts valid files, rejects oversized ones
- Language auto-detected from file extension
- Upload button disabled until file selected
- Build progress shows spinner while status=building
- Test trigger button appears after status=ready
- Error message shown when build fails

FILE: frontend/src/lib/__tests__/api.test.ts
Tests:
- createSubmission sends correct FormData
- getLeaderboard handles empty leaderboard gracefully
- API errors thrown as typed ApiError objects
- 429 rate limit error has retryAfter field

FILE: frontend/vitest.config.ts
Configure Vitest:
- environment: jsdom
- setupFiles: [./src/test/setup.ts]
- coverage: v8, 70% minimum threshold
- globals: true

Output complete test code.
```

---

### PROMPT 57 — Frontend: Dockerfile + Nginx + Static Build

```
Create the production build configuration for the frontend.

FILE: frontend/Dockerfile
Multi-stage:
Stage 1 (builder): node:20-alpine
- Copy package.json, npm ci
- Copy source
- Run npm run build → output in /app/dist

Stage 2 (server): nginx:alpine
- Copy dist/ from builder to /usr/share/nginx/html
- Copy nginx.conf
- EXPOSE 80
- Non-root user: use nginx user (already configured in nginx:alpine)

Final image: ~30MB

FILE: frontend/nginx.conf
Nginx config for SPA (Single Page Application):
  server {
    listen 80;
    root /usr/share/nginx/html;
    
    # Gzip all text assets
    gzip on;
    gzip_types text/css application/javascript application/json;
    
    # Cache static assets aggressively (Vite content-hashes filenames)
    location /assets {
      expires 1y;
      add_header Cache-Control "public, immutable";
    }
    
    # Cache-bust the main index.html (never cache it)
    location / {
      try_files $uri $uri/ /index.html;
      add_header Cache-Control "no-cache";
    }
    
    # Proxy API calls to backend (for same-origin deployment)
    location /v1/ {
      proxy_pass http://submission-api:8080;
      proxy_set_header X-Real-IP $remote_addr;
    }
    
    # Proxy WebSocket calls
    location /ws {
      proxy_pass http://leaderboard-api:8084;
      proxy_http_version 1.1;
      proxy_set_header Upgrade $http_upgrade;
      proxy_set_header Connection "upgrade";
    }
  }

FILE: infra/k8s/frontend/deployment.yaml
Deployment: 2 replicas, resources minimal (static file serving is cheap):
  requests: { cpu: "50m", memory: "32Mi" }
  limits: { cpu: "200m", memory: "64Mi" }
ReadinessProbe: GET / every 5s (nginx healthcheck)

FILE: infra/k8s/frontend/service.yaml
ClusterIP service on port 80.

Output all files with complete content.
```

---

## PHASE 7 — TERRAFORM + HELM INFRASTRUCTURE (Prompts 58–65)

---

### PROMPT 58 — Terraform: Cloud Infrastructure (AWS)

```
Create Terraform configuration for deploying trade-eval-platform to AWS.

FILE: infra/terraform/main.tf
Terraform block: required_version = "~> 1.8", providers: aws ~> 5.0

Data sources:
- aws_availability_zones (use 2 AZs for cost control)
- aws_caller_identity

FILE: infra/terraform/vpc.tf
VPC:
- CIDR: 10.0.0.0/16
- 2 public subnets: 10.0.1.0/24, 10.0.2.0/24 (for load balancers)
- 2 private subnets: 10.0.11.0/24, 10.0.12.0/24 (for EKS nodes)
- NAT Gateway (1 for cost savings, not HA)
- Internet Gateway
- Route tables for public/private subnets

FILE: infra/terraform/eks.tf
EKS cluster:
- Version: 1.30
- Two node groups:
  1. "system" (t3.medium × 2): runs platform services (submission-api, orchestrator, etc.)
     min=2, max=4
  2. "bot-fleet" (c6i.2xlarge × 0-10): runs bot-fleet pods (CPU-optimized, scale to 0)
     min=0, max=10, instance type chosen for high CPU-to-RAM ratio
     Taint: dedicated=bot-fleet:NoSchedule (ensures only bot-fleet pods scheduled here)
- OIDC provider for IRSA (IAM Roles for Service Accounts)
- Addons: coredns, kube-proxy, vpc-cni, aws-ebs-csi-driver

FILE: infra/terraform/databases.tf
TimescaleDB: RDS instance (db.t3.medium, postgres 16, Multi-AZ=false for cost)
  Storage: 100GB gp3, encrypt at rest.
  Parameter group: shared_preload_libraries = 'timescaledb'
  Security group: only allow from EKS node CIDR

Orchestrator Postgres: same pattern, db.t3.micro

ElastiCache Redis: cluster.t3.micro, 1 shard, no cluster mode
  Security group: only from EKS

MSK (Managed Kafka): kafka.t3.small, 3 brokers, 3 AZs
  Encryption in transit: TLS
  Auto-create topics: false (we create them via bootstrap script)
  Broker storage: 50GB

FILE: infra/terraform/s3.tf
MinIO replacement: S3 bucket "trade-eval-submissions-{account_id}"
  Versioning: disabled (we don't need versions)
  Lifecycle rule: delete objects older than 7 days
  Block all public access: true
  IAM role for submission-api with s3:PutObject, s3:GetObject on this bucket

FILE: infra/terraform/outputs.tf
Output: eks_cluster_endpoint, rds_timescale_endpoint, redis_endpoint,
msk_bootstrap_brokers, s3_bucket_name, ecr_registry_url

FILE: infra/terraform/variables.tf
Variables: aws_region (default us-east-1), environment (dev/staging/prod),
  eks_cluster_name, desired_node_count, enable_nat_gateway

Output complete Terraform HCL. All resources properly tagged with:
  Environment, Project="trade-eval", ManagedBy="terraform"
```

---

### PROMPT 59 — Terraform: Security + IAM

```
Create security and IAM configuration for trade-eval-platform.

FILE: infra/terraform/iam.tf

IAM roles using IRSA (IAM Roles for Service Accounts):
This lets Kubernetes pods assume IAM roles without long-lived credentials.

1. submission-api-role:
   - Trust policy: EKS OIDC provider, service account: trade-eval/submission-api
   - Permissions:
     - s3:PutObject, s3:GetObject, s3:DeleteObject on submissions bucket
     - secretsmanager:GetSecretValue on "trade-eval/*" secrets

2. build-worker-role:
   - s3:GetObject on submissions bucket
   - ecr:GetAuthorizationToken, ecr:BatchGetImage, ecr:InitiateLayerUpload (for Kaniko)
   - ecr:PutImage to contestant-images ECR repo

3. telemetry-ingester-role:
   - No AWS permissions needed (reads from Kafka, writes to RDS via network)

4. leaderboard-api-role:
   - No AWS permissions needed

FILE: infra/terraform/security_groups.tf
Security groups:
- eks-nodes: outbound all, inbound from ALB + same SG
- rds-postgres: inbound TCP 5432 from eks-nodes only
- redis: inbound TCP 6379 from eks-nodes only
- kafka: inbound TCP 9092 and 9094 from eks-nodes only
- alb: inbound TCP 80/443 from 0.0.0.0/0

FILE: infra/terraform/secrets.tf
AWS Secrets Manager entries (created empty, must be populated manually):
- trade-eval/timescale-db-password
- trade-eval/orchestrator-db-password
- trade-eval/admin-api-key
- trade-eval/msk-credentials (if MSK uses SASL)

Use External Secrets Operator in Kubernetes to sync these to k8s Secrets.

FILE: infra/terraform/ecr.tf
ECR repositories for each service:
- trade-eval/submission-api
- trade-eval/build-worker
- trade-eval/orchestrator
- trade-eval/bot-fleet
- trade-eval/telemetry-ingester
- trade-eval/leaderboard-api
- trade-eval/frontend
- trade-eval/contestant-images  ← where built contestant Docker images go

Lifecycle policy on contestant-images: keep only last 10 images per contestant
(prevents ECR storage from ballooning during a contest with many submissions)

FILE: infra/terraform/monitoring.tf
CloudWatch:
- Log groups for each service (30-day retention)
- CloudWatch Container Insights for EKS (CPU/memory metrics)
- Alarm: EKS node CPU > 90% → SNS → email notification
- Alarm: RDS storage < 20% free → SNS notification

Output complete Terraform HCL.
```

---

### PROMPT 60 — Helm: Umbrella Chart + Values

```
Create an umbrella Helm chart that deploys the entire platform.

FILE: infra/helm/trade-eval-platform/Chart.yaml
name: trade-eval-platform
description: Umbrella chart for the complete trade evaluation platform
version: 0.1.0
appVersion: "1.0.0"
dependencies:
  - name: submission-api
    version: "0.1.0"
    repository: "file://../submission-api"
  - name: build-worker
    version: "0.1.0"
    repository: "file://../build-worker"
  - name: orchestrator
    version: "0.1.0"
    repository: "file://../orchestrator"
  - name: bot-fleet
    version: "0.1.0"
    repository: "file://../bot-fleet"
  - name: telemetry-ingester
    version: "0.1.0"
    repository: "file://../telemetry-ingester"
  - name: leaderboard-api
    version: "0.1.0"
    repository: "file://../leaderboard-api"
  - name: frontend
    version: "0.1.0"
    repository: "file://../frontend"
  # Infrastructure dependencies
  - name: kafka
    version: "29.3.3"
    repository: "https://charts.bitnami.com/bitnami"
    condition: kafka.enabled
  - name: redis
    version: "20.0.1"
    repository: "https://charts.bitnami.com/bitnami"
    condition: redis.enabled
  - name: keda
    version: "2.14.0"
    repository: "https://kedacore.github.io/charts"
    condition: keda.enabled

FILE: infra/helm/trade-eval-platform/values.yaml
Global defaults:
global:
  imageRegistry: ""
  imagePullSecrets: []
  environment: production

Kafka (using Bitnami chart):
kafka:
  enabled: true  # false if using AWS MSK
  replicaCount: 3
  persistence:
    size: 50Gi
  config: |
    auto.create.topics.enable=false
    default.replication.factor=3
    min.insync.replicas=2
  zookeeper:
    enabled: false  # use KRaft mode
  kraft:
    enabled: true

Redis:
redis:
  enabled: true  # false if using AWS ElastiCache
  architecture: standalone  # not cluster mode for simplicity
  auth:
    enabled: false  # use network isolation instead for local dev
  persistence:
    size: 10Gi

KEDA:
keda:
  enabled: true

Per-service values: (sample for submission-api)
submission-api:
  replicaCount: 2
  image:
    tag: latest
  config:
    maxUploadSizeMB: 50
    logLevel: info

FILE: infra/helm/trade-eval-platform/templates/namespace.yaml
Creates the trade-eval namespace with labels.

FILE: scripts/deploy.sh
Deployment script:
1. helm dependency update infra/helm/trade-eval-platform
2. helm upgrade --install trade-eval-platform infra/helm/trade-eval-platform \
   --namespace trade-eval \
   --create-namespace \
   --values infra/helm/trade-eval-platform/values.yaml \
   --values infra/helm/trade-eval-platform/values-prod.yaml \
   --set global.imageTag=$(git rev-parse --short HEAD) \
   --wait --timeout 10m

Output all files with complete content.
```

---

### PROMPT 61 — Kafka: Topic Configuration + Consumer Group Tuning

```
Create Kafka configuration files and tuning guides.

FILE: infra/kafka/topics.sh
Script to create all Kafka topics with production-optimized settings:

kafka-topics.sh --create --topic submissions \
  --partitions 4 \
  --replication-factor 3 \
  --config retention.ms=604800000 \  (7 days)
  --config min.insync.replicas=2

kafka-topics.sh --create --topic build-jobs \
  --partitions 4 \
  --replication-factor 3 \
  --config retention.ms=3600000 \  (1 hour — jobs are short-lived)
  --config min.insync.replicas=2

kafka-topics.sh --create --topic orchestrator-events \
  --partitions 4 \
  --replication-factor 3 \
  --config retention.ms=86400000 \  (1 day)
  --config min.insync.replicas=2

kafka-topics.sh --create --topic bot-telemetry \
  --partitions 16 \              ← CRITICAL: must match telemetry-ingester consumer count
  --replication-factor 3 \
  --config retention.ms=3600000 \ (1 hour — process and discard)
  --config min.insync.replicas=2 \
  --config compression.type=snappy \  (pre-compress at Kafka level too)
  --config segment.bytes=536870912   (500MB segments for fast compaction)

kafka-topics.sh --create --topic telemetry-anomalies \
  --partitions 2 \
  --replication-factor 3

FILE: infra/kafka/KAFKA_TUNING.md
Document all Kafka tuning decisions:

Producer tuning (bot-fleet):
- batch.size=1000: collect 1000 messages before sending (reduces network calls)
- linger.ms=10: wait 10ms for batch to fill (tradeoff: 10ms added latency for batching efficiency)
- compression.type=snappy: fast compression (lz4 is also good)
- acks=1: only leader ack needed (fast; we can afford some data loss on broker crash)
- max.in.flight.requests.per.connection=5: pipelining for throughput
  Note: set to 1 if ordering is critical (it is for our use case per partition)

Consumer tuning (telemetry-ingester):
- fetch.min.bytes=1048576: wait until 1MB available before returning (batching)
- fetch.max.wait.ms=500: OR wait 500ms, whichever comes first
- max.poll.records=1000: up to 1000 records per poll call
- auto.offset.reset=earliest: on new consumer group, start from beginning
- enable.auto.commit=false: manual commit (at-least-once delivery)

Partition key strategy:
- bot-telemetry: key = contestant_id
  This ensures all events for contestant X go to the same partition,
  guaranteeing order for the shadow order book validator.
  With 16 partitions and 20 contestants, ~1.25 contestants per partition — acceptable.

FILE: infra/kafka/monitoring-queries.sh
Useful Kafka monitoring commands:
# Check consumer group lag
kafka-consumer-groups.sh --describe --group telemetry-ingesters

# Check topic partition details
kafka-topics.sh --describe --topic bot-telemetry

# Watch consumer group lag in real time
watch -n 2 'kafka-consumer-groups.sh --describe --group telemetry-ingesters'

Output all files with complete content.
```

---

### PROMPT 62 — Monitoring Stack: Prometheus + Grafana

```
Create monitoring configuration for trade-eval-platform.

FILE: infra/helm/monitoring/values.yaml
Deploy kube-prometheus-stack:
prometheus:
  retention: 30d
  storageSpec:
    volumeClaimTemplate:
      spec:
        storageClassName: gp3
        resources:
          requests:
            storage: 50Gi

grafana:
  adminPassword: "changeme"  # override in prod
  persistence:
    enabled: true
    size: 10Gi
  dashboardProviders:
    dashboardproviders.yaml:
      apiVersion: 1
      providers:
        - name: trade-eval
          folder: Trade Eval Platform
          type: file
          options:
            path: /var/lib/grafana/dashboards/trade-eval

FILE: infra/grafana/dashboards/platform-overview.json
Grafana dashboard JSON for the overall platform health:

Panels:
1. Active Tests (stat panel)
2. Active Containers (stat panel)
3. Total Events Processed/sec (graph: rate(telemetry_events_total[1m]))
4. Kafka Consumer Lag — bot-telemetry (graph: kafka_consumer_lag per partition)
5. API Request Rate (graph: rate(http_requests_total[1m]) per service)
6. API Error Rate (graph: rate(http_requests_total{status=~"5.."}[1m]))
7. API Latency p99 (graph: histogram_quantile(0.99, http_request_duration_seconds))
8. Bot Fleet Active Bots (gauge)
9. TimescaleDB Query Latency (histogram)
10. Redis Memory Usage (gauge)

FILE: infra/grafana/dashboards/leaderboard-live.json
Live competition dashboard:

Panels:
1. Leaderboard table (Grafana table panel sourcing from TimescaleDB via plugin)
2. P99 Latency by contestant (multi-series line chart)
3. TPS by contestant (multi-series line chart)
4. Correctness Rate by contestant (bar gauge panel)
5. Active Bots per Test (stacked area chart)
6. Kafka Events/sec (single stat)

FILE: infra/k8s/monitoring/servicemonitor-all.yaml
ServiceMonitor resources (Prometheus Operator) for each service:
- submission-api: scrape /metrics every 15s
- bot-fleet: scrape /metrics every 15s (high cardinality — more frequent useful)
- telemetry-ingester: scrape /metrics every 15s
- leaderboard-api: scrape /metrics every 30s

FILE: infra/k8s/monitoring/alerts.yaml
PrometheusRule for critical alerts:
- KafkaConsumerLagHigh: lag > 100000 for 5 minutes → page on-call
- BotFleetBotsDropping: events_dropped_total increases → warning
- ContainerOOMKilled: container OOM kills → warning
- TestStuck: test in status=running for > 30 minutes → alert
- TimescaleDBSlowQueries: query latency p99 > 5s → warning

Output all files with complete content.
```

---

### PROMPT 63 — Logging: Structured Logging + ELK Stack

```
Set up centralized logging for trade-eval-platform.

FILE: infra/helm/logging/fluentbit-values.yaml
Deploy Fluent Bit as DaemonSet to collect all pod logs:
config:
  inputs: |
    [INPUT]
        Name              tail
        Path              /var/log/containers/*trade-eval*.log
        Parser            docker
        Tag               trade-eval.*
        Mem_Buf_Limit     50MB
  
  filters: |
    [FILTER]
        Name                kubernetes
        Match               trade-eval.*
        Merge_Log           On
        Keep_Log            Off
        K8S-Logging.Parser  On
  
  outputs: |
    [OUTPUT]
        Name            es
        Match           trade-eval.*
        Host            elasticsearch.logging
        Port            9200
        Logstash_Format On
        Logstash_Prefix trade-eval

FILE: infra/helm/logging/elasticsearch-values.yaml
Elasticsearch single-node (for development, 3-node in production):
  replicas: 1
  resources:
    requests: { memory: 2Gi }
    limits: { memory: 4Gi }
  persistence:
    size: 50Gi

FILE: infra/helm/logging/kibana-values.yaml
Kibana:
  service.type: ClusterIP
  ingress:
    enabled: true
    hosts: [ "kibana.trade-eval.local" ]

FILE: docs/LOGGING.md
Logging standards for all services:
1. All logs in JSON format (using slog with JSONHandler)
2. Required fields: timestamp, level, service, version, trace_id, span_id
3. For each API request: request_id, method, path, status_code, duration_ms, contestant_id
4. For each Kafka message: topic, partition, offset, lag
5. For each build: submission_id, language, duration_ms, status
6. For each test: test_id, contestant_id, bot_count, active_bots
7. NEVER log: API keys, passwords, PII

FILE: services/submission-api/middleware/logging.go
Structured request logging middleware:
- Log: method, path, status, duration, request_id, contestant_id (from context)
- Add trace_id header to response (for correlating client-side errors)
- Log request body for POST/PUT at DEBUG level only (never in production)
- Log at ERROR for 5xx, WARN for 4xx, INFO for 2xx

�� SMART: Structured JSON logging (vs plain text) makes the difference between
"searching logs for 30 minutes" and "Kibana query gives you the answer in 30 seconds."
A structured query like { service: "bot-fleet", contest_id: "abc123", level: "error" }
finds all errors for one test run instantly across 50 pods. 

Output all files with complete content.
```

---

### PROMPT 64 — Terraform: Terraform State + CD Pipeline

```
Create Terraform remote state and CD pipeline configuration.

FILE: infra/terraform/backend.tf
Remote state in S3 + DynamoDB locking:
terraform {
  backend "s3" {
    bucket         = "trade-eval-terraform-state"
    key            = "production/terraform.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "trade-eval-terraform-locks"
  }
}

FILE: infra/terraform/bootstrap/main.tf
Bootstrap script: creates the S3 bucket and DynamoDB table for remote state.
This must be applied manually BEFORE the main Terraform configuration.

S3 bucket:
  - Versioning enabled (keep 30 state file versions)
  - Server-side encryption: AES-256
  - Block all public access

DynamoDB table:
  - Hash key: LockID (string)
  - Billing mode: PAY_PER_REQUEST

FILE: .github/workflows/terraform.yml
GitHub Actions workflow for Terraform:

Triggers: PRs touching infra/terraform/**, manual dispatch for apply

Jobs:
1. terraform-plan (on PR):
   - Configure AWS credentials (using GitHub OIDC — no long-lived keys)
   - terraform init
   - terraform validate
   - terraform plan -out=plan.tfplan
   - Post plan output as PR comment (using hashicorp/setup-terraform action)
   - Store plan artifact

2. terraform-apply (manual dispatch, requires approval):
   - Download plan artifact
   - terraform apply plan.tfplan
   - Post apply output summary

Security: use GitHub OIDC for AWS authentication:
  - No AWS access keys in GitHub secrets
  - IAM role trusted by github.com/trade-eval-platform/* OIDC

FILE: infra/terraform/terragrunt.hcl (optional, for multi-environment)
Terragrunt config for DRY multi-env management:
- dev, staging, prod environments share same terraform modules
- Environment-specific values in env/*/terragrunt.hcl

FILE: Makefile (additions)
tf-init: cd infra/terraform && terraform init
tf-plan: cd infra/terraform && terraform plan -var-file=vars/$(ENV).tfvars
tf-apply: cd infra/terraform && terraform apply -var-file=vars/$(ENV).tfvars -auto-approve
tf-destroy: cd infra/terraform && terraform destroy -var-file=vars/$(ENV).tfvars

Output all files with complete content.
```

---

### PROMPT 65 — Helm: Per-Service Chart Templates

```
Create Helm chart templates for the remaining services not yet covered.

For each service (submission-api, build-worker, telemetry-ingester, leaderboard-api, frontend),
create a Helm chart following the same structure as the orchestrator chart (Prompt 19).

FILE STRUCTURE for each service (example for submission-api):
infra/helm/submission-api/
├── Chart.yaml
├── values.yaml
├── templates/
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── configmap.yaml
│   ├── hpa.yaml
│   ├── pdb.yaml
│   ├── servicemonitor.yaml  ← for Prometheus scraping
│   └── _helpers.tpl

KEY DIFFERENCES per service:

submission-api:
  - HPA: scale on CPU > 70%, minReplicas=2, maxReplicas=10
  - Service type: ClusterIP (behind ingress)
  - Extra: Ingress resource with TLS (cert-manager annotation)

build-worker:
  - Requires special service account with Docker socket access
  - HPA: scale on custom metric "pending_build_jobs" (from Redis)
  - Volume: mount Docker socket (or Kaniko doesn't need it)
  - SecurityContext: if using Kaniko, needs runAsNonRoot
  - minReplicas=1 (always need one worker), maxReplicas=10

telemetry-ingester:
  - HPA: scale on Kafka consumer lag metric (via KEDA ScaledObject)
  - High resource requests (shadow order book needs CPU/memory)
  - PodAntiAffinity (spread across nodes for redundancy)

bot-fleet:
  - Scale to zero (minReplicas=0 with KEDA)
  - NodeAffinity: prefer bot-fleet node group (c6i.2xlarge)
  - Tolerations: match bot-fleet node taint

leaderboard-api:
  - Service type: LoadBalancer (external access)
  - WebSocket-specific annotations on Service
  - PodDisruptionBudget: minAvailable=2

Create ALL these chart files with complete, production-ready YAML.
Use consistent naming conventions and Helm best practices throughout.
Include NOTES.txt in each chart with post-install verification steps.

Also create infra/helm/Makefile:
lint-all:
  for chart in infra/helm/*/; do helm lint $$chart; done

package-all:
  for chart in infra/helm/*/; do helm package $$chart -d infra/helm/packages/; done

Output complete YAML for all chart files.
```

---

## PHASE 8 — SECURITY HARDENING (Prompts 66–70)

---

### PROMPT 66 — Security: Contestant Container Hardening

```
Implement comprehensive security hardening for contestant containers.

FILE: infra/docker/contestant/seccomp/README.md
Document the seccomp profile strategy.

FILE: infra/docker/contestant/apparmor/contestant-profile
AppArmor profile for contestant containers:
#include <tunables/global>
profile trade-eval-contestant flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  
  # Allow reading essential files
  /proc/cpuinfo r,
  /proc/meminfo r,
  /etc/hosts r,
  /etc/resolv.conf r,
  /tmp/** rw,
  
  # Allow network (the only exposed port)
  network inet stream,
  network inet6 stream,
  
  # DENY filesystem writes (except /tmp)
  deny /proc/** w,
  deny /sys/** w,
  deny /dev/mem rw,
  
  # DENY ptrace/proc operations
  deny ptrace (trace),
  deny signal (send) peer=unconfined,
  
  # Allow only specific syscalls profile + execute own binary
  /app/orderbook rix,
  /usr/bin/python3 rix,  # if Python
}

FILE: services/build-worker/security/resource_monitor.go
ResourceMonitor watches contestant container resource usage and kills if exceeded:

Runs every 5 seconds per container:
1. GET /containers/{id}/stats from Docker daemon (non-streaming)
2. Parse: cpu_usage, memory_usage, network_io
3. If memory > soft_limit (400MB): WARN + emit metric
4. If memory > hard_limit (512MB): Docker handles kill (cgroup enforcement)
5. If CPU has been pinned at 100% for > 30 seconds: log (potential infinite loop)
6. If network_io > 0 bytes OUT on non-8080 port: CRITICAL SECURITY ALERT
   (contestant container should have zero outbound network traffic except on 8080)

FILE: infra/k8s/security/pod-security-policy.yaml (or PodSecurity admission):
PodSecurity namespace labels for trade-eval namespace:
  pod-security.kubernetes.io/enforce: restricted
  pod-security.kubernetes.io/warn: restricted

This enforces: no privileged containers, no host namespaces, run as non-root,
read-only root filesystem, drop all capabilities.

FILE: infra/k8s/security/network-policies/deny-all-default.yaml
Default deny-all network policy for the trade-eval namespace.
All inter-pod traffic requires explicit NetworkPolicy allow rules.

FILE: services/build-worker/security/image_scanner.go
ImageScanner: scans built contestant images for known vulnerabilities before running them.

Uses Trivy CLI (installed in build-worker Docker image):
  trivy image --severity HIGH,CRITICAL --exit-code 1 {imageName}

If scan finds CRITICAL vulnerabilities:
  - Block the container from starting
  - Mark submission as failed with reason "security_scan_failed"
  - Log detailed vulnerability report

If scan finds only HIGH vulnerabilities (but no CRITICAL):
  - Warn but allow (HIGH is typically container OS vulnerabilities, not exploits)
  - Log the warnings

�� SMART: Scanning contestant images with Trivy prevents a scenario where a contestant
submits code that exploits a known vulnerability in the base image to escalate privileges.
Combined with seccomp + AppArmor + network isolation, this creates defense-in-depth:
even if one layer is bypassed, the others still hold.

Output complete code and configuration.
```

---

### PROMPT 67 — Security: API Security + Input Validation

```
Implement comprehensive API security for all services.

FILE: services/submission-api/security/validator.go

Input validation for all API endpoints:

ValidateSubmissionRequest:
- file extension must match language (no .exe claiming to be Go code)
- file size: 1KB min (sanity check), 50MB max
- language must be in allowed list
- contestant_id in path must match authenticated contestant
- Sanitize all string fields: trim whitespace, max length, no null bytes
- Reject files with suspicious names: ../../../etc/passwd, etc.

ValidateTestRequest:
- duration_seconds: 30 ≤ x ≤ 600 (don't allow 24-hour tests)
- bot_count: 1 ≤ x ≤ 1000 (cap max bots per test for resource control)
- bot_personas: must be subset of allowed personas

FILE: services/submission-api/security/zip_bomb.go
ZipBomb detector — prevents zip files that unzip to TB of data:

CheckZipBomb(zipPath string) error:
  Open zip file, iterate entries without extracting:
  1. Count total uncompressed size: sum of entry.UncompressedSize64
  2. If > 500MB: return ErrZipBomb
  3. If ratio (uncompressed / compressed) > 100: return ErrZipBomb
  4. If file count > 10,000: return ErrZipBomb (zip slip defense)
  5. Check each filename for path traversal: strings.Contains(name, "..")
     or strings.HasPrefix(name, "/")
     Return ErrPathTraversal if found

FILE: services/submission-api/security/rate_limiter.go
Advanced rate limiting beyond the simple counter:
- Global rate limit: 100 submissions per hour across ALL contestants
  (prevents one contestant from flooding the build queue)
- Per-contestant: 10 submissions per hour
- Per-IP: 5 submissions per hour (prevents one person using multiple accounts)
- Build queue depth limit: if > 50 pending builds, reject new submissions with 503
  "Build queue at capacity, please try again later"

FILE: services/submission-api/middleware/security_headers.go
Security HTTP headers:
  X-Content-Type-Options: nosniff
  X-Frame-Options: DENY
  X-XSS-Protection: 1; mode=block
  Content-Security-Policy: default-src 'none'; script-src 'self'
  Referrer-Policy: strict-origin-when-cross-origin
  Permissions-Policy: geolocation=(), microphone=(), camera=()
  Strict-Transport-Security: max-age=31536000; includeSubDomains (HTTPS only)

FILE: docs/SECURITY.md
Security architecture document:
1. Threat model: who is the adversary? (contestants trying to cheat, external attackers)
2. Defense-in-depth layers: seccomp → AppArmor → Docker constraints → network policy
3. Scoring integrity: how correctness validation prevents fake metrics
4. API security: rate limiting, input validation, authentication
5. Infrastructure security: IRSA, no long-lived keys, Terraform state encryption

Output complete Go code and documentation.
```

---

### PROMPT 68 — Security: Secrets Management

```
Implement secrets management for trade-eval-platform.

FILE: infra/k8s/secrets/external-secrets-operator.yaml
Deploy External Secrets Operator (ESO) to sync AWS Secrets Manager → k8s Secrets:

ClusterSecretStore:
  name: aws-secrets-manager
  provider:
    aws:
      service: SecretsManager
      region: us-east-1
      auth:
        jwt:
          serviceAccountRef:
            name: external-secrets-sa  (has IRSA role for secretsmanager:GetSecretValue)

ExternalSecret for each service:
  name: submission-api-secrets
  refreshInterval: 1h (rotate secrets hourly if they change)
  secretStoreRef: aws-secrets-manager
  target:
    name: submission-api-secrets  (creates this k8s Secret)
  data:
    - secretKey: DB_PASSWORD
      remoteRef: { key: trade-eval/timescale-db-password }
    - secretKey: ADMIN_API_KEY
      remoteRef: { key: trade-eval/admin-api-key }

FILE: infra/k8s/secrets/sealed-secrets.yaml (alternative approach)
Use Bitnami Sealed Secrets for GitOps-friendly secret management:

SealedSecret that can be safely committed to Git:
  The sealed secret is encrypted with the cluster's public key.
  Only the cluster can decrypt it with its private key.
  
Example: sealed submission-api-secrets with all required env vars.
Include instructions: "to seal a new secret, run: kubeseal --format yaml < secret.yaml"

FILE: services/submission-api/config/config.go (update)
Add secret rotation support:
- On startup: load all secrets
- Background goroutine: reload secrets from k8s Secret every hour
  (ESO updates the k8s Secret, app picks up changes without restart)
- Use atomic.Value to store secrets (lock-free hot rotation)

FILE: scripts/rotate-secrets.sh
Script to rotate all secrets:
1. Generate new password (openssl rand -base64 32)
2. Update in AWS Secrets Manager
3. Wait for ESO to sync (60s)
4. Verify new secret in k8s
5. Force pod restart: kubectl rollout restart deployment/submission-api

Output complete YAML and script files.
```

---

### PROMPT 69 — Security: Penetration Testing + Audit

```
Create security testing and audit tooling for trade-eval-platform.

FILE: scripts/security/pentest.sh
Automated security test suite that runs pre-deployment:

Section 1: API Security Tests (using curl):
test_sql_injection() {
  # Try SQL injection in submission ID path
  RESP=$(curl -sf -X GET $BASE_URL/v1/submissions/"'; DROP TABLE contestants;--")
  assert_status_code 400 "$RESP" "SQL injection should return 400"
}

test_path_traversal() {
  # Try path traversal in file upload
  echo "test" > /tmp/test.cpp
  RESP=$(curl -sf -X POST $BASE_URL/v1/submissions \
    -F "file=@/tmp/test.cpp;filename=../../etc/passwd" \
    -F "language=cpp" \
    -H "X-API-Key: $TEST_API_KEY")
  assert_contains "INVALID_FILENAME" "$RESP" "Path traversal should be rejected"
}

test_zip_bomb() {
  # Create a zip bomb (small but highly compressed)
  python3 -c "
  import zipfile, io
  data = b'A' * 1024 * 1024 * 100  # 100MB of As
  buf = io.BytesIO()
  with zipfile.ZipFile(buf, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as zf:
    zf.writestr('main.cpp', data)
  open('/tmp/bomb.zip', 'wb').write(buf.getvalue())
  "
  RESP=$(curl -sf -X POST $BASE_URL/v1/submissions \
    -F "file=@/tmp/bomb.zip" \
    -F "language=cpp" \
    -H "X-API-Key: $TEST_API_KEY")
  assert_contains "ZIP_BOMB" "$RESP" "Zip bomb should be rejected"
}

test_rate_limit() {
  # Send 20 requests rapidly and check for 429
  for i in $(seq 1 20); do
    curl -s -X GET $BASE_URL/v1/health &
  done
  wait
  # At least some should have been rate limited
}

test_auth_bypass() {
  # Try accessing authenticated endpoint without API key
  RESP=$(curl -s -o /dev/null -w "%{http_code}" $BASE_URL/v1/submissions)
  assert_equals 401 "$RESP" "No API key should return 401"
  
  # Try with invalid API key
  RESP=$(curl -s -o /dev/null -w "%{http_code}" $BASE_URL/v1/submissions \
    -H "X-API-Key: invalid_key_xxx")
  assert_equals 401 "$RESP" "Invalid API key should return 401"
}

FILE: scripts/security/container_escape_test.sh
Tests that contestant containers are properly isolated:

test_no_internet_access() {
  # Start a test container
  CONTAINER_ID=$(docker run -d --network contestant-isolated trade-eval-contestant:test)
  
  # Try to reach external internet from within container
  RESULT=$(docker exec $CONTAINER_ID wget -T 5 -q -O- http://google.com 2>&1 || echo "BLOCKED")
  assert_contains "BLOCKED" "$RESULT" "Container should not reach internet"
  
  docker rm -f $CONTAINER_ID
}

test_no_host_filesystem() {
  CONTAINER_ID=$(docker run -d --read-only --network contestant-isolated trade-eval-contestant:test)
  
  # Try to write to root filesystem
  RESULT=$(docker exec $CONTAINER_ID sh -c "echo test > /etc/test 2>&1" || echo "BLOCKED")
  assert_contains "BLOCKED" "$RESULT" "Container should have read-only filesystem"
  
  docker rm -f $CONTAINER_ID
}

FILE: docs/AUDIT_LOG.md
Template for security audit log.
Document all security decisions and their rationale.
Include: date, decision, rationale, reviewer, risk level.

Output complete shell scripts and documentation.
```

---

### PROMPT 70 — Security: DDoS + Chaos Engineering

```
Implement DDoS protection and chaos engineering for trade-eval-platform.

FILE: infra/k8s/security/ddos-protection.yaml
Configure AWS Shield Standard (automatic) and WAF rules:

WAF WebACL rules (via Terraform resource aws_wafv2_web_acl):
1. AWS Managed Core Rule Set (OWASP top 10)
2. AWS Managed Known Bad Inputs
3. Custom rate rule: 1000 requests/5min per IP → block 30 minutes
4. Custom rule: block IPs hitting /v1/submissions more than 5 times/hour
5. Custom rule: block requests with suspicious User-Agents (automated scanners)

FILE: scripts/chaos/chaos-experiments.sh
Chaos Engineering experiments using Chaos Mesh or manual kubectl tricks:

experiment_kafka_broker_death() {
  echo "=== CHAOS: Kill one Kafka broker ==="
  kubectl delete pod kafka-2 -n trade-eval
  echo "Waiting 30s for recovery..."
  sleep 30
  echo "Checking consumer group lag..."
  kubectl exec -it kafka-0 -- kafka-consumer-groups.sh --describe --group telemetry-ingesters
  echo "Checking for data loss..."
  # Verify: lag should be 0 or near-0 (no messages lost, broker restarted)
}

experiment_orchestrator_crash_during_test() {
  echo "=== CHAOS: Kill orchestrator during active test ==="
  # First: start a test
  curl -X POST $API_URL/v1/tests -d '{"submission_id":"..."}' -H "X-API-Key: ..."
  sleep 5  # Let test start
  # Kill the orchestrator
  kubectl delete pod -l app=orchestrator -n trade-eval
  echo "Orchestrator killed. Waiting for replacement..."
  sleep 70  # Orphan detection runs at 60s
  echo "Checking if test was recovered or failed cleanly..."
  curl $API_URL/v1/tests/$TEST_ID | jq .status
  # Expected: either "running" (recovered) or "failed" (clean failure), never stuck in "running" forever
}

experiment_timescaledb_slow() {
  echo "=== CHAOS: Add latency to TimescaleDB ==="
  # Use tc (traffic control) to add 500ms latency to DB pod
  kubectl exec -it timescaledb-0 -- tc qdisc add dev eth0 root netem delay 500ms
  sleep 60  # Run with slow DB for 60 seconds
  echo "Checking telemetry ingester — should not crash, should buffer writes..."
  kubectl logs -l app=telemetry-ingester | grep "timescaledb" | tail -20
  # Expected: ingester logs "write queued" errors but continues processing Kafka
  # Remove chaos
  kubectl exec -it timescaledb-0 -- tc qdisc del dev eth0 root
}

experiment_redis_partition() {
  echo "=== CHAOS: Block Redis from leaderboard-api ==="
  kubectl exec -it $(kubectl get pod -l app=leaderboard-api -o name | head -1) -- \
    iptables -A OUTPUT -d redis -j DROP
  sleep 10
  echo "Checking leaderboard — should serve stale data, not 500 errors..."
  curl $API_URL/v1/leaderboard | jq .updated_at  # Should return old timestamp
  # Remove chaos
  kubectl exec -it $(kubectl get pod -l app=leaderboard-api -o name | head -1) -- \
    iptables -D OUTPUT -d redis -j DROP
}

FILE: docs/RUNBOOK.md
Operations runbook with procedures for:
1. Handling a compromised contestant container
2. Restarting a service without downtime (rolling restart)
3. Manually triggering orphan detection
4. Resetting a stuck test
5. Emergency freeze of leaderboard
6. Scaling bot fleet manually
7. Disaster recovery: restoring TimescaleDB from backup

Output all files with complete content.
```

---

## PHASE 9 — TESTING STRATEGY (Prompts 71–80)

---

### PROMPT 71 — Integration Testing: Full Stack

```
Create comprehensive integration tests that test the full system end-to-end.

FILE: tests/integration/full_pipeline_test.go

Full pipeline integration test that starts all services and verifies the complete flow.

Use testcontainers-go to start:
- Kafka (redpanda/redpanda image — starts in 3s)
- Redis (redis:7.2-alpine)
- TimescaleDB (timescale/timescaledb:latest-pg16)
- Postgres (postgres:16-alpine)
- MinIO (minio/minio:latest)
- A mock contestant container (simple Go HTTP server that correctly implements
  the order book API — this is your reference implementation double)

TestCase: ContestantSubmitsAndGetsScore
1. Start all services
2. Insert test contestant
3. POST /v1/submissions with sample C++ orderbook zip
4. Wait for build to complete (mock build-worker for test speed)
5. Mock the sandbox container start (bypass actual Docker build)
6. POST /v1/tests to trigger evaluation
7. Connect WebSocket client to leaderboard-api
8. Wait for WebSocket message with contestant appearing in leaderboard
9. Assert: WebSocket message has valid score > 0
10. Assert: TimescaleDB has rows in latency_samples
11. Assert: Redis has metrics for contestant
12. GET /v1/leaderboard and verify contestant appears
13. Assert: composite_score > 0 and < 100

TestCase: ContestantSubmitsBrokenOrderBook
1. Start all services
2. Mock contestant container that returns WRONG fills (always returns 0 fill quantity)
3. Trigger test with 50 bots for 10 seconds
4. Wait for completion
5. Assert: correctness_rate < 0.05 (almost all fills wrong)
6. Assert: composite_score < 20 (correctness failure tanks the score)

TestCase: ContestantContainerCrashes
1. Start test
2. After 5 seconds: kill the mock contestant container
3. Assert: test ends within 30 seconds (not stuck forever)
4. Assert: test status = "failed" with reason "container_crashed"
5. Assert: Redis lock is released (so contestant can run another test)

TestCase: OrchestratorCrashRecovery
1. Start test
2. Kill orchestrator pod
3. Wait 90 seconds (longer than orphan detection interval of 60s)
4. Restart orchestrator
5. Assert: orphan detection runs and either recovers or fails the test cleanly
6. Assert: test is NOT stuck in "running" forever

Output complete Go integration test code.
```

---

### PROMPT 72 — Load Testing: k6 Scripts

```
Create k6 load test scripts for infrastructure testing.

FILE: scripts/load-test/submission_api_load.js
k6 test for submission API:

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate } from 'k6/metrics';

export const options = {
  scenarios: {
    ramp_up: {
      executor: 'ramping-vus',
      stages: [
        { duration: '30s', target: 10 },
        { duration: '60s', target: 50 },
        { duration: '30s', target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_duration: ['p99<2000'],  // 99% of requests under 2s
    'http_req_duration{endpoint:health}': ['p99<100'],  // health checks fast
    http_req_failed: ['rate<0.01'],   // < 1% error rate
  },
};

export default function () {
  // Health check
  let res = http.get('http://submission-api:8080/v1/health');
  check(res, { 'health ok': (r) => r.status === 200 });
  
  // Get leaderboard (no auth)
  res = http.get('http://submission-api:8080/v1/leaderboard');
  check(res, { 'leaderboard ok': (r) => r.status === 200 });
  
  sleep(1);
}

FILE: scripts/load-test/websocket_load.js
k6 WebSocket load test:
import ws from 'k6/ws';
import { check } from 'k6';

export const options = {
  vus: 1000,  // 1000 concurrent WebSocket connections
  duration: '60s',
};

export default function () {
  const url = 'ws://leaderboard-api:8084/ws';
  const res = ws.connect(url, {}, function (socket) {
    socket.on('open', function () {
      // Connection established
    });
    
    socket.on('message', function (data) {
      const update = JSON.parse(data);
      check(update, {
        'has entries': (u) => u.entries && u.entries.length >= 0,
        'has timestamp': (u) => u.timestamp > 0,
      });
    });
    
    socket.on('error', function (e) {
      console.error('WebSocket error:', e);
    });
    
    // Keep connection open for 30s
    socket.setTimeout(function () {
      socket.close();
    }, 30000);
  });
  
  check(res, { 'status is 101': (r) => r && r.status === 101 });
}

FILE: scripts/load-test/bot_fleet_stress.js
Stress test the mock contestant container at maximum bot load:
Sends 10,000 concurrent order requests to a mock server.
Measures latency distribution.
Expects: all requests complete, p99 < 100ms for the mock (which just returns immediately).

FILE: scripts/load-test/run-all.sh
Script to run all load tests in sequence:
1. Start all services
2. Run submission_api_load.js → expect pass
3. Run websocket_load.js with 100 VUs → expect pass
4. Run websocket_load.js with 1000 VUs → record results (may degrade)
5. Generate HTML report (k6 cloud or local grafana-k6-reporter)
6. Save results to scripts/load-test/results/$(date +%Y%m%d)/

Output complete k6 JavaScript test files and shell script.
```

---

### PROMPT 73 — Reference Implementation: Sample C++ Order Book

```
Create the sample/reference C++ order book that serves as both the test binary
and the correctness reference for validation.

FILE: testdata/sample-orderbook/main.cpp

A correct, complete C++ HTTP order book server implementing:
- Price-time priority matching
- Limit and market orders
- Cancel support
- HTTP endpoints: POST /order, POST /cancel, GET /health, GET /orderbook

Use cpp-httplib (header-only) for the HTTP server.
Use standard C++ data structures.

Implementation:
- std::map<double, std::deque<Order>, std::greater<>> bids (descending price)
- std::map<double, std::deque<Order>> asks (ascending price)
- std::unordered_map<string, OrderRef> order_lookup (for cancels)
- std::mutex book_mutex (protect all book operations)

Match algorithm:
For LIMIT_BUY at price P:
  while (!asks.empty() && asks.begin()->first <= P && remaining > 0):
    fill from asks.begin()->second (FIFO deque)
    if deque empty: remove price level from asks
  if remaining > 0: insert into bids at price P

For MARKET_BUY:
  while (!asks.empty() && remaining > 0):
    fill from cheapest ask
  Note: if no liquidity, market order is rejected (or partially filled)

HTTP endpoints (all synchronous, mutex protected):
POST /order:
  Parse JSON body: { order_id, type, price, quantity }
  Acquire mutex
  Run matching algorithm
  Return: { order_id, status, filled_price, filled_quantity, remaining_quantity }

POST /cancel:
  Find order in lookup map
  Remove from book
  Return: { order_id, status: "CANCELLED" or "NOT_FOUND" }

GET /health:
  Return: { status: "ok" }

GET /orderbook:
  Return: { bids: [{price, quantity, count}], asks: [...] }

FILE: testdata/sample-orderbook/Makefile
Build with: g++ -O2 -std=c++17 -o orderbook main.cpp -lpthread
Run with: ./orderbook (starts on port 8080)

FILE: testdata/sample-orderbook/README.md
Document:
1. How to build and run locally
2. API specification
3. Common edge cases handled
4. Known limitations (not production-grade — this is a reference for testing)

FILE: testdata/sample-orderbook/tests/test_correctness.sh
Shell script that tests the order book:
1. Start ./orderbook in background
2. Test: Submit bid, submit ask at same price → both should fill
3. Test: Time priority — two bids at same price, one ask fills the earlier bid first
4. Test: Cancel — submit order, cancel it, verify cancelled
5. Test: Market order → fills against available liquidity
6. Kill orderbook

�� SMART: Having a correct reference implementation is crucial for two reasons:
1. It's your test binary — use it to verify your entire platform works end-to-end
2. It defines the expected behavior — contestants' implementations are compared to
   the SAME logic your shadow order book uses. If your shadow book and reference
   implementation diverge, correctness scores will be wrong.
Keep this implementation simple and obviously correct, NOT fast.

Output complete C++ code and test script.
```

---

### PROMPT 74 — Testing: Property-Based Testing for Shadow Order Book

```
Implement property-based tests for the shadow order book to verify correctness.

FILE: services/telemetry-ingester/shadowbook/property_test.go

Property-based testing using go-rapid (a Go property-testing library).

Properties to verify:

Property 1: "FIFO at same price level"
  Invariant: If two orders arrive at the same price level, the first one is filled first.
  Generator: Generate N random limit orders at the same price, then a matching opposite order.
  Assert: The order with the lowest sequence_number is filled first.

Property 2: "Best price first"
  Invariant: Among multiple asks, the cheapest ask is always used first.
  Generator: Generate asks at prices 100, 101, 102 in random order.
  Assert: A market buy fills the 100-ask first regardless of submission order.

Property 3: "No overfill"
  Invariant: The total quantity filled never exceeds the original order quantity.
  Generator: Random sequence of mixed orders.
  Assert: For each fill, filled_qty ≤ requested_qty.

Property 4: "Conservation of quantity"
  Invariant: Total bid quantity + total ask quantity changes predictably.
  Generator: Sequence of N random limit orders.
  Assert: After each match, the quantity removed from book = quantity added to fills.

Property 5: "Book state consistency"
  Invariant: The order lookup map and price level deques are always consistent.
  Generator: Sequence of adds and cancels.
  Assert: Every order in lookup map is also in a price level's deque.

Property 6: "Cancel is idempotent"
  Invariant: Cancelling an already-cancelled order returns NOT_FOUND, doesn't panic.
  Generator: Cancel the same order ID twice.
  Assert: Second cancel returns NOT_FOUND, no panic.

FILE: services/telemetry-ingester/shadowbook/fuzz_test.go
Go fuzzing for the shadow order book:
FuzzProcessOrder: feeds arbitrary JSON bytes as order input.
Assert: the shadow book NEVER panics regardless of input.
This is critical — if a contestant sends malformed responses, the shadow book
must handle it gracefully (return INCORRECT, not crash the telemetry ingester).

�� SMART: Property-based testing finds edge cases you never thought of.
Running Property 4 (quantity conservation) for 100,000 random order sequences
will almost certainly find a bug in a naive implementation. This is how trading
systems are tested in production — formal verification and property testing,
not just unit tests with hand-crafted cases.

Output complete Go property test and fuzz test code.
```

---

### PROMPT 75 — Testing: Contract Testing Between Services

```
Implement contract testing to ensure services agree on message formats.

FILE: tests/contracts/README.md
Document the contract testing strategy using Pact (consumer-driven contract testing).

Why contract tests? 
When the bot-fleet changes the TelemetryEvent JSON schema (e.g., adds a field),
the telemetry-ingester must be updated too. Without contract tests, this breakage
only shows up at integration test time or production. Contract tests catch it immediately.

FILE: tests/contracts/bot_fleet_telemetry_contract_test.go
Consumer (telemetry-ingester) defines what it expects from the producer (bot-fleet):

pact.AddInteraction().
  UponReceiving("a telemetry event from a bot").
  WithRequest(dsl.Request{
    Method: "message",
    Body: dsl.Like(map[string]interface{}{
      "contestant_id":    dsl.Like("abc123"),
      "test_id":          dsl.Like("test_xyz"),
      "bot_id":           dsl.Like("bot_042"),
      "bot_persona":      dsl.Term("market_maker", "market_maker|aggressive_taker|spammer|whale"),
      "order_id":         dsl.Like("ord_9919"),
      "sent_at_ns":       dsl.Like(1718000000100000000),
      "acked_at_ns":      dsl.Like(1718000000100450000),
      "latency_us":       dsl.Like(450),
      "order_type":       dsl.Term("LIMIT_BUY", "LIMIT_BUY|LIMIT_SELL|MARKET_BUY|MARKET_SELL|CANCEL"),
      "price":            dsl.Like(100.5),
      "quantity":         dsl.Like(10.0),
      "correct":          dsl.Like(true),
      "timed_out":        dsl.Like(false),
      "sequence_number":  dsl.Like(1),
    }),
  }).
  WillRespondWith(...)  // for message pact, this defines what consumer can handle

FILE: tests/contracts/orchestrator_bot_fleet_contract_test.go
Contract: orchestrator publishes START_TEST, bot-fleet expects certain format.
Consumer (bot-fleet) defines minimum required fields it needs to start a test.
Producing (orchestrator) must satisfy this contract.

FILE: tests/contracts/Makefile
Targets:
  pact-test: Run consumer contract tests, generate Pact files to tests/contracts/pacts/
  pact-verify: Run provider verification against P



  # MASTER PROMPTS — PART 2 (Prompts 76–130)
## Continuation from Prompt 75

---

## PHASE 9 — TESTING STRATEGY CONTINUED (Prompts 76–80)

---

### PROMPT 76 — Testing: Contract Testing Continued + Pact Broker

```
Complete the contract testing setup from Prompt 75.

FILE: tests/contracts/Makefile (complete version)
pact-test:
	cd services/bot-fleet && go test ./... -run TestPact -v
	cd services/telemetry-ingester && go test ./... -run TestPact -v

pact-verify:
	cd services/bot-fleet && go test ./... -run TestPactProvider -v

pact-publish:
	pact-broker publish tests/contracts/pacts \
	  --broker-base-url http://pact-broker:9292 \
	  --consumer-app-version $(git rev-parse --short HEAD)

FILE: tests/contracts/submission_api_contract_test.go
Contract: frontend (consumer) expects submission-api (provider) to return specific fields.

Consumer side (simulate frontend calling POST /v1/submissions):
  pact.AddInteraction().
    UponReceiving("create submission request").
    WithRequest(dsl.Request{
      Method: "POST",
      Path:   dsl.String("/v1/submissions"),
      Headers: dsl.MapMatcher{
        "X-API-Key":    dsl.String("test-key"),
        "Content-Type": dsl.StringContaining("multipart/form-data"),
      },
    }).
    WillRespondWith(dsl.Response{
      Status: 202,
      Body: dsl.Match(struct {
        SubmissionID string `json:"submission_id" pact:"example=sub_abc123"`
        Status       string `json:"status" pact:"example=pending"`
      }{}),
    })

This guarantees: if submission-api removes or renames submission_id field,
the contract test catches it BEFORE it reaches integration tests.

FILE: infra/k8s/pact-broker/deployment.yaml
Deploy Pact Broker in Kubernetes:
- image: pactfoundation/pact-broker:latest
- Postgres backend for storing Pact files
- Service: ClusterIP on port 9292
- Only accessible within cluster (used by CI, not public)

FILE: .github/workflows/contract-tests.yml
Contract test workflow:
1. Run consumer tests (bot-fleet, frontend) → generate Pact JSON files
2. Publish Pact files to broker
3. Run provider verification (submission-api, telemetry-ingester verify against broker)
4. If provider verification fails: block the PR

This creates a safety net: provider can never break consumers unknowingly.

FILE: tests/contracts/leaderboard_websocket_contract_test.go
Contract: frontend expects leaderboard WebSocket messages in specific format.
Use pact-go's async message support.
Define minimum required fields in LeaderboardUpdate message.
Verify: leaderboard-api produces messages matching this contract.

Output complete Go contract test code and Kubernetes YAML.
```

---

### PROMPT 77 — Testing: Correctness Test Suite for Order Books

```
Create a comprehensive correctness test suite that contestants can run locally.

FILE: tests/correctness/suite_test.go

This test suite validates ANY order book implementation against expected behavior.
Contestants can run this against their own implementation before submitting.

The test suite hits a running order book server (configurable via ORDER_BOOK_URL env var).

TestSuite struct:
  baseURL    string
  client     *http.Client
  orderCount atomic.Int64  // for generating unique order IDs

Helper: submitOrder(t, orderType, price, qty float64) OrderResponse
Helper: cancelOrder(t, orderID string) CancelResponse
Helper: getOrderBook(t) OrderBookState
Helper: resetBook(t)  // POST /reset to clear all state (only works in test mode)

Test cases (each completely independent, calls reset before running):

TestBasicLimitOrderFill:
  Submit LIMIT_SELL at $100 for 10 shares
  Submit LIMIT_BUY at $100 for 10 shares
  Assert: buy fill.status = FILLED, fill.price = 100.0, fill.quantity = 10

TestPartialFill:
  Submit LIMIT_SELL at $100 for 5 shares
  Submit LIMIT_BUY at $100 for 10 shares
  Assert: buy fill.status = PARTIAL, fill.quantity = 5, remaining_quantity = 5

TestMarketOrderFillsAtBestPrice:
  Submit LIMIT_SELL at $100 for 5
  Submit LIMIT_SELL at $101 for 5
  Submit LIMIT_SELL at $102 for 5
  Submit MARKET_BUY for 7
  Assert: 5 shares filled at $100, 2 shares filled at $101 (best price first)
  Assert: total filled = 7

TestPriceTimePriority:
  Submit LIMIT_SELL A at $100 for 5 (t=0ms)
  time.Sleep(1ms)
  Submit LIMIT_SELL B at $100 for 5 (t=1ms)
  Submit LIMIT_BUY at $100 for 5
  Assert: Order A is filled (not Order B) — time priority at same price

TestCancelUnfilledOrder:
  Submit LIMIT_BUY at $50 for 10 (far from market, won't fill)
  Cancel the order
  Assert: cancel response status = CANCELLED
  Assert: order book no longer shows this order

TestCancelFilledOrderReturnsAlreadyFilled:
  Submit LIMIT_SELL at $100 for 10
  Submit LIMIT_BUY at $100 for 10 (fills immediately)
  Try to cancel the buy order
  Assert: cancel response status = ALREADY_FILLED or NOT_FOUND

TestMarketOrderNoLiquidity:
  (Empty book)
  Submit MARKET_BUY for 10
  Assert: response status = REJECTED (no liquidity)
  NOTE: some implementations return PARTIAL with quantity=0 — both acceptable

TestLargeVolumeCorrectness:
  Submit 1000 LIMIT_SELLs at prices 100-199 (1 share each)
  Submit MARKET_BUY for 500 shares
  Assert: 500 fills happened at prices 100-199 in order (best price first)
  Assert: sum of all fill quantities = 500

TestConcurrentOrders:
  Launch 50 goroutines, each submitting 10 orders in parallel
  After all complete: fetch /orderbook
  Assert: order book is self-consistent (no phantom orders, no negative quantities)
  Assert: no 500 errors from concurrent access

TestOrderIDUniqueness:
  Submit same order_id twice
  Assert: second submission returns error (duplicate order_id should be rejected)

FILE: tests/correctness/run.sh
Script to run correctness suite against any server:
  ORDER_BOOK_URL=http://localhost:8080 go test ./tests/correctness/... -v -timeout 60s

FILE: docs/contestant-testing.md
Document how contestants use this suite:
1. Implement the API (POST /order, POST /cancel, GET /health, GET /orderbook, POST /reset)
2. Run: ./tests/correctness/run.sh
3. All tests must pass before submitting
4. The same suite runs on your submission during evaluation

Output complete Go test code and documentation.
```

---

### PROMPT 78 — Testing: Performance Benchmarking Suite

```
Create a performance benchmarking suite to measure order book throughput.

FILE: tests/performance/bench_test.go

Benchmark suite that measures throughput and latency of any order book.
Configured via environment variables: ORDER_BOOK_URL, BENCH_DURATION, BENCH_CONCURRENCY.

BenchmarkOrderBookThroughput:
  - Spawn BENCH_CONCURRENCY goroutines (default 100)
  - Each goroutine sends LIMIT_BUY orders as fast as possible
  - Run for BENCH_DURATION seconds (default 10)
  - Count total orders processed
  - Report: orders/second, p50/p90/p99 latency

BenchmarkMixedWorkload:
  - 40% LIMIT_BUY, 40% LIMIT_SELL, 10% MARKET_BUY, 10% CANCEL
  - Same concurrency and duration
  - More realistic than pure limit order flood

BenchmarkLatencyUnderLoad:
  - Send orders at fixed rate (100/s, 500/s, 1000/s, 5000/s, 10000/s)
  - At each rate: measure p99 latency
  - Find the "knee" where p99 degrades sharply (max sustainable throughput)
  - Report: throughput vs latency curve (like a Little's Law analysis)

BenchmarkCancelHeavy:
  - 90% CANCEL, 10% LIMIT_BUY (simulates HFT cancel-heavy workload)
  - Measures how well the implementation handles cancel storms

FILE: tests/performance/report.go
BenchmarkReporter: generates a formatted benchmark report.

After running: writes to tests/performance/results/report-{timestamp}.json:
{
  "timestamp": "2024-01-01T00:00:00Z",
  "server_url": "http://localhost:8080",
  "results": [
    {
      "benchmark": "OrderBookThroughput",
      "concurrency": 100,
      "duration_seconds": 10,
      "total_orders": 45230,
      "orders_per_second": 4523.0,
      "p50_latency_us": 89,
      "p90_latency_us": 210,
      "p99_latency_us": 450,
      "error_rate": 0.0
    }
  ]
}

Also generates a Markdown summary table for quick review.

FILE: tests/performance/compare.go
CompareReports: given two report JSON files, shows improvement/regression:
  go run tests/performance/compare.go results/report-before.json results/report-after.json

Output:
  OrderBookThroughput:
    TPS:     4523 → 8901  (+97% ✅)
    p99:     450µs → 210µs  (-53% ✅)
    Errors:  0.0% → 0.0%  (no change)

FILE: tests/performance/run.sh
Shell script to run full benchmark suite and compare with previous run:
  Previous result stored as: tests/performance/results/baseline.json
  New result: tests/performance/results/latest.json
  Run compare.go to show diff
  If p99 regression > 20%: exit 1 (fails CI)

Output complete Go benchmark and comparison code.
```

---

### PROMPT 79 — Testing: Chaos Testing for Bot Fleet

```
Create chaos tests specifically for the bot fleet behavior under failure conditions.

FILE: services/bot-fleet/chaos/chaos_test.go

Chaos tests that verify bot fleet resilience.

TestBotFleet_ContainerGoesDown_BotsRecoverGracefully:
  Setup:
  - Start mock contestant server
  - Start 50 bots against it
  - Verify bots are sending orders (check telemetry counter > 0)
  Action:
  - Kill the mock server (httptest.Server.Close())
  Expected:
  - Circuit breaker trips within 10 seconds
  - Bots stop sending (circuit OPEN), no goroutine panics
  - Telemetry continues emitting timed_out=true events
  Recovery:
  - Restart mock server on same address
  - Wait 15 seconds (circuit breaker half-open probe)
  - Assert: bots resume sending after circuit closes

TestBotFleet_KafkaProducerDown_TestContinues:
  Setup:
  - Start bots with a mock Kafka that starts accepting then rejects
  Action:
  - Close the mock Kafka connection mid-test
  Expected:
  - Bots continue sending HTTP orders to contestant container
  - Kafka producer buffers events (up to batchCh capacity)
  - dropped counter increments (observable via metrics)
  - Test does NOT halt or panic
  Recovery:
  - Re-enable mock Kafka
  - Assert: buffered events flushed (dropped counter stops growing)

TestBotFleet_SlowContestantServer_LatencyReflected:
  Setup:
  - Mock server with 2000ms fixed delay (2 seconds per response)
  - 10 bots, 30 second test
  Expected:
  - Latency events show ~2000ms latency
  - BotRequestTimeout (5000ms) NOT triggered (2s < 5s)
  - Circuit breaker NOT tripped (requests succeed, just slowly)
  - TPS ~= 5 ops/sec (limited by 2s response time)

TestBotFleet_ExtremelySlowServer_TimeoutsRecorded:
  Setup:
  - Mock server with 6000ms delay (exceeds 5000ms timeout)
  Expected:
  - timed_out=true events emitted
  - Circuit breaker trips after sufficient failures
  - Bots do NOT hang indefinitely waiting

FILE: services/bot-fleet/chaos/mock_server.go
ControllableMockServer for chaos tests:
- Start(delay, errorRate) — start with configurable delay and % error rate
- SetDelay(d time.Duration) — change delay mid-test (thread-safe)
- SetErrorRate(pct float64) — change error rate mid-test
- Kill() — immediately close all connections
- Restart() — restart on same port
- OrdersReceived() int64 — how many valid orders processed

Output complete Go chaos test code.
```

---

### PROMPT 80 — Testing: Test Data Generators + Fixtures

```
Create test data generators and fixtures for the full test suite.

FILE: tests/testdata/generators.go

ContestantGenerator: generates realistic test contestants.
  GenerateContestants(n int) []Contestant
  - Random names from a pool of 100 names
  - UUIDs for IDs
  - Valid email addresses
  - Random API keys (hex strings)

SubmissionGenerator: generates test submission data.
  GenerateCPPSubmission() []byte  — returns a valid, correct C++ order book as zip
  GenerateBrokenSubmission() []byte  — returns C++ that compiles but returns wrong fills
  GenerateSlowSubmission() []byte  — returns C++ that works but adds 5ms per request
  GenerateCompileFailSubmission() []byte  — returns C++ with syntax errors

TelemetryEventGenerator: generates realistic telemetry event streams.
  GenerateCorrectStream(contestantID, testID string, n int) []TelemetryEvent
    — n events, all correct, realistic latencies (normal distribution, mean=300µs, σ=100µs)
  
  GenerateIncorrectStream(contestantID, testID string, n int, errorRate float64) []TelemetryEvent
    — n events, errorRate% have wrong actual_fill vs expected_fill
  
  GenerateHighLatencyStream(contestantID, testID string) []TelemetryEvent
    — events with p99=5000µs (slow implementation)
  
  GenerateTimedOutStream(contestantID, testID string, timeoutRate float64) []TelemetryEvent
    — timeoutRate% of events have timed_out=true

FILE: tests/testdata/fixtures.go
Static test fixtures (deterministic, not random):

FixtureContestantAlice() Contestant  — always same ID and API key for stable tests
FixtureContestantBob() Contestant
FixtureSubmissionReady(contestantID string) Submission  — status=ready, has container info
FixtureTestRunning(submissionID, contestantID string) Test  — status=running
FixtureTestCompleted(submissionID, contestantID string, score float64) Test

FILE: tests/testdata/seed_db.go
SeedDatabase(db *pgxpool.Pool) function that inserts all fixtures into test DB.
Used in TestMain of integration tests for a consistent starting state.
Idempotent: uses INSERT ON CONFLICT DO NOTHING so repeated calls are safe.

FILE: tests/testdata/mock_kafka.go
MockKafkaProducer: in-memory Kafka producer for unit tests.
Stores all published messages in a []kafka.Message slice.
Thread-safe with sync.RWMutex.
Methods:
  Produce(topic, key, value []byte) error
  Messages(topic string) []kafka.Message  — returns all messages for a topic
  Reset()  — clear all messages

FILE: tests/testdata/mock_redis.go
MockRedisClient: wraps miniredis for unit tests.
Starts an in-memory Redis server.
Methods:
  Start() *miniredis.Miniredis
  Client() *redis.Client
  Reset()  — flush all keys

�� SMART: The TelemetryEventGenerator with controllable error rates lets you write
parameterized tests like:
  for errorRate := 0.0; errorRate <= 1.0; errorRate += 0.1 {
    stream := GenerateIncorrectStream("abc", "test", 1000, errorRate)
    rate := pipeline.Process(stream)
    assert.InDelta(t, errorRate, 1.0-rate, 0.02)
  }
This validates your correctness pipeline across the full range of input quality.

Output complete Go code for all files.
```

---

## PHASE 10 — PERFORMANCE OPTIMIZATIONS (Prompts 81–90)

---

### PROMPT 81 — Performance: Go Runtime Tuning

```
Implement Go runtime and GC tuning for all high-throughput services.

FILE: services/bot-fleet/runtime_tuning.go

Go runtime configuration for bot-fleet (highest throughput service):

func init() {
  // Tune GC: at 1M events/sec, default GC is too aggressive.
  // GOGC=400 means GC runs when heap is 4x the live set (default is 100 = 2x)
  // Trade: more memory usage, fewer GC pauses
  // At 1M events/sec and ~50 bytes/event = 50MB/sec heap growth
  // With GOGC=400: GC fires every ~200MB growth vs default ~50MB
  // Result: 4x fewer GC pauses, each pause slightly longer
  debug.SetGCPercent(400)
  
  // GOMAXPROCS: default is num CPUs, which is correct. But for bot-fleet
  // on a 4-core node running 200 bots: explicitly set to leave 1 core for OS
  if runtime.NumCPU() > 2 {
    runtime.GOMAXPROCS(runtime.NumCPU() - 1)
  }
  
  // Memory ballast: allocate a fixed chunk to artificially inflate live heap.
  // This delays GC triggers, reducing GC frequency without affecting semantics.
  // Size: 100MB ballast means GC won't trigger until 200MB of actual allocations.
  // Used in trading systems (Twitch, Cloudflare use this pattern).
  _ = make([]byte, 100*1024*1024)  // 100MB ballast, never freed
}

FILE: services/telemetry-ingester/runtime_tuning.go

For telemetry-ingester (shadow book is allocation-heavy):
  debug.SetGCPercent(200)  // moderate GC tuning
  
  // SetMaxStack: shadow book processes millions of orders, default stack per
  // goroutine is 8KB, grows as needed. Limit maximum to prevent runaway stacks.
  debug.SetMaxStack(64 * 1024)  // 64KB max stack per goroutine

FILE: services/bot-fleet/pool_allocations.go

Object pools to reduce allocations at 1M events/sec:

var orderRequestPool = sync.Pool{
  New: func() any { return &OrderRequest{} },
}

var orderResponsePool = sync.Pool{
  New: func() any { return &OrderResponse{} },
}

var telemetryEventPool = sync.Pool{
  New: func() any { return &TelemetryEvent{} },
}

var jsonBufPool = sync.Pool{
  New: func() any { return bytes.NewBuffer(make([]byte, 0, 512)) },
}

Usage pattern (CRITICAL — must reset before returning to pool):
  event := telemetryEventPool.Get().(*TelemetryEvent)
  defer func() {
    *event = TelemetryEvent{}  // zero out before returning
    telemetryEventPool.Put(event)
  }()

FILE: services/bot-fleet/bots/zero_alloc_json.go
Zero-allocation JSON serialization for TelemetryEvent using jsoniter:
  import jsoniter "github.com/json-iterator/go"
  var json = jsoniter.ConfigCompatibleWithStandardLibrary

MarshalTelemetryEvent(event *TelemetryEvent, buf *bytes.Buffer) error:
  Uses streaming encoder from pool (no intermediate allocation).
  Benchmarks: 3-5x faster than standard encoding/json.

�� SMART COMPARISON:
  Standard encoding/json at 1M events/sec:
    - ~200 bytes/event allocation × 1M = 200MB/sec GC pressure
    - GC pauses every 0.5-1 second, each 5-10ms
    - Result: measurable throughput degradation
  
  sync.Pool + jsoniter:
    - ~0 allocations per event (pool reuse)
    - GC pressure near zero from telemetry path
    - GC pauses rare and short
    - Result: consistent throughput, no latency spikes from GC

Output complete Go code.
```

---

### PROMPT 82 — Performance: HTTP Connection Pooling Deep Dive

```
Implement advanced HTTP connection pooling for the bot fleet.

FILE: services/bot-fleet/transport/optimized_transport.go

OptimizedTransport: a per-contestant-container HTTP transport with optimal settings.

Why per-contestant (not global)?
  - Each contestant container has a different IP
  - Connection pools are per-host in Go's http.Transport
  - A global transport with many hosts dilutes the pool benefit

ContestantTransportFactory struct:
  mu         sync.RWMutex
  transports map[string]*http.Transport  // keyed by "ip:port"

GetOrCreate(targetAddr string) *http.Transport:
  Check cache first (RLock).
  On miss: create new transport (Lock):
    &http.Transport{
      DialContext: (&net.Dialer{
        Timeout:   2 * time.Second,
        KeepAlive: 30 * time.Second,
      }).DialContext,
      MaxIdleConns:          200,   // total idle connections
      MaxIdleConnsPerHost:   200,   // all pointed at one host
      MaxConnsPerHost:       200,   // hard limit on concurrent connections
      IdleConnTimeout:       60 * time.Second,
      TLSHandshakeTimeout:   1 * time.Second,
      ExpectContinueTimeout: 1 * time.Second,
      DisableCompression:    true,  // no gzip — latency measurement
      DisableKeepAlives:     false, // KEEP keepalives on
      ForceAttemptHTTP2:     false, // HTTP/1.1 has lower overhead for this workload
                                    // HTTP/2 multiplexing is not beneficial here:
                                    // each bot request is independent, no head-of-line blocking
    }

FILE: services/bot-fleet/transport/connection_warmup.go
ConnectionWarmup: pre-establishes connections before test starts.

WarmUp(targetAddr string, connections int) error:
  Before bots start sending orders, pre-establish `connections` idle TCP connections.
  This avoids the initial latency spike from TCP handshakes on the first requests.
  
  Implementation:
  - Create `connections` HTTP clients pointed at targetAddr
  - Each sends GET /health to establish connection
  - Connection goes back to idle pool
  - When bots start: they reuse warm connections (no handshake latency)
  
  Why this matters:
  Without warmup: first 200 orders all incur TCP handshake (~1ms each)
  With warmup: all orders immediately reuse existing connections (0ms overhead)
  
  This is measurably visible in p99 latency for the first 10 seconds of a test.

FILE: services/bot-fleet/transport/transport_test.go
BenchmarkTransport_WithWarmup vs BenchmarkTransport_WithoutWarmup:
  Start mock server
  Without warmup: send 1000 requests, measure first-100 latency vs last-100 latency
  With warmup: send 1000 requests, measure first-100 latency vs last-100 latency
  Assert: with warmup, first-100 p99 ≈ last-100 p99 (no warmup spike)
  Assert: without warmup, first-100 p99 > last-100 p99 (warmup spike visible)

�� SMART: Connection warmup is a commonly overlooked optimization.
In a 5-minute test, the first 30 seconds of data have inflated latency from
TCP handshakes. Without warmup, your p99 includes connection establishment time.
With warmup, you measure ONLY matching engine latency — which is what you want.

Output complete Go code.
```

---

### PROMPT 83 — Performance: TimescaleDB Query Optimization

```
Implement TimescaleDB query optimization for the telemetry ingester API.

FILE: services/telemetry-ingester/storage/query_optimizer.go

QueryOptimizer wraps all TimescaleDB queries with caching and optimization.

Pattern 1: Continuous Aggregates (materialized views that auto-update):

FILE: services/telemetry-ingester/migrations/005_continuous_aggregates.sql

Create a TimescaleDB continuous aggregate for 1-minute buckets:
CREATE MATERIALIZED VIEW latency_1min
WITH (timescaledb.continuous) AS
SELECT
  time_bucket('1 minute', time) AS bucket,
  contestant_id,
  test_id,
  percentile_agg(latency_us) AS latency_percentile_state,
  count(*) AS order_count,
  sum(CASE WHEN correct THEN 1 ELSE 0 END) AS correct_count,
  avg(latency_us) AS avg_latency
FROM latency_samples
GROUP BY bucket, contestant_id, test_id
WITH NO DATA;

SELECT add_continuous_aggregate_policy('latency_1min',
  start_offset => INTERVAL '10 minutes',
  end_offset   => INTERVAL '1 minute',
  schedule_interval => INTERVAL '1 minute');

Then query the materialized view for 1-minute resolution (100x faster):
SELECT 
  bucket,
  approx_percentile(0.99, latency_percentile_state) AS p99,
  approx_percentile(0.50, latency_percentile_state) AS p50,
  order_count,
  correct_count::float / order_count AS correctness_rate
FROM latency_1min
WHERE contestant_id = $1 AND bucket >= $2
ORDER BY bucket ASC;

Pattern 2: Query result caching in Redis:

FILE: services/telemetry-ingester/storage/query_cache.go

CachedQueryExecutor:
  redis    *redis.Client
  db       *pgxpool.Pool
  cacheTTL time.Duration  // default 5s for live data, 60s for historical

GetLatencyHistory(ctx, contestantID, start, end, resolution string) ([]LatencyPoint, error):
  cacheKey := fmt.Sprintf("query:latency:%s:%d:%d:%s", contestantID, start, end, resolution)
  
  // Try cache
  cached, err := redis.Get(ctx, cacheKey)
  if err == nil {
    return unmarshal(cached), nil
  }
  
  // Cache miss: query DB
  result, err := queryLatencyHistory(ctx, contestantID, start, end, resolution)
  if err != nil { return nil, err }
  
  // Cache result with appropriate TTL
  ttl := 5 * time.Second
  if end < time.Now().Add(-1*time.Hour).UnixMilli() {
    ttl = 60 * time.Second  // historical data changes rarely
  }
  redis.Set(ctx, cacheKey, marshal(result), ttl)
  
  return result, nil

Pattern 3: Connection pool sizing:
TimescaleDB connection pool: maxConns = (CPU cores × 2) + active queries
For 4-core DB server: maxConns = 10 (not 100 — too many connections hurts PostgreSQL)
pgxpool.Config:
  MaxConns: 10
  MinConns: 2
  MaxConnLifetime: 30 * time.Minute  // recycle connections to prevent stale state
  HealthCheckPeriod: 30 * time.Second

FILE: services/telemetry-ingester/migrations/006_indexes.sql
Additional indexes for common query patterns:
CREATE INDEX CONCURRENTLY idx_latency_samples_contestant_time
  ON latency_samples (contestant_id, time DESC);

CREATE INDEX CONCURRENTLY idx_latency_samples_test_time
  ON latency_samples (test_id, time DESC);

ANALYZE latency_samples;  -- update query planner statistics

Output complete Go code and SQL migrations.
```

---

### PROMPT 84 — Performance: Redis Optimization

```
Implement Redis optimization for the leaderboard and metrics systems.

FILE: services/leaderboard-api/cache/redis_optimizer.go

RedisOptimizer implements efficient Redis access patterns.

Pattern 1: Pipeline all reads (avoid N round trips):

BEFORE (naive — N round trips for N contestants):
  for _, contestantID := range contestants {
    p99, _ := redis.HGet(ctx, "metrics:"+contestantID, "p99_latency_us")
    tps, _ := redis.HGet(ctx, "metrics:"+contestantID, "tps")
  }

AFTER (1 round trip for N contestants):
  pipe := redis.Pipeline()
  cmds := make(map[string]*redis.MapStringStringCmd)
  for _, id := range contestants {
    cmds[id] = pipe.HGetAll(ctx, "metrics:"+id)
  }
  pipe.Exec(ctx)
  for id, cmd := range cmds {
    data := cmd.Val()  // already resolved
    // process data
  }

Pattern 2: Use Redis Sorted Set for leaderboard ordering:
Instead of: fetch all metrics → sort in Go every 500ms
Use: ZADD leaderboard:scores {score} {contestant_id} when score changes
     ZREVRANGE leaderboard:scores 0 49 WITHSCORES → top 50 instantly

This turns the sort from O(n log n) in Go to O(log n + k) in Redis
(where k = number of results requested).

FILE: services/telemetry-ingester/cache/metrics_publisher.go

MetricsPipeline: efficient Redis publish pattern.

Instead of publishing each metric field separately:
  HSET metrics:alice p50 89
  HSET metrics:alice p90 210
  HSET metrics:alice p99 450
  HSET metrics:alice tps 9200
  (4 round trips)

Use single HSET with multiple fields:
  HSET metrics:alice p50 89 p90 210 p99 450 tps 9200 correctness 0.999 updated_ns 1234
  (1 round trip, atomic update)

Also: ZADD leaderboard:scores {composite_score} {contestant_id}
in the same pipeline.

Redis memory optimization:
  For 50 contestants × each having 8 metric fields:
  - Default: Redis stores as hash table (~400 bytes/contestant)
  - hash-max-listpack-entries 128 (default): uses ziplist for small hashes
  - With 8 fields: ziplist encoding → ~80 bytes/contestant (5x savings)
  
Add to Redis config: hash-max-listpack-entries 64, hash-max-listpack-value 64

FILE: services/leaderboard-api/cache/leaderboard_cache.go
LeaderboardCache: multi-tier caching for the leaderboard endpoint.

Tier 1: In-process memory cache (sync.Map, 500ms TTL)
  - Zero network overhead
  - Stale by up to 500ms (acceptable for leaderboard)
  
Tier 2: Redis cache (2s TTL)
  - Used when in-process cache misses
  - Shared across all leaderboard-api pods
  
Tier 3: Live compute (only on both cache misses)
  - Query Redis metrics + sort + compute scores

GetLeaderboard(ctx) ([]LeaderboardEntry, error):
  if cached, ok := localCache.Get("leaderboard"); ok { return cached }
  if cached, err := redis.Get("leaderboard:cached"); err == nil { return cached }
  result, err := computeLeaderboard(ctx)
  localCache.Set("leaderboard", result, 500ms)
  redis.Set("leaderboard:cached", result, 2s)
  return result, err

Output complete Go code.
```

---

### PROMPT 85 — Performance: Kafka Partition Strategy + Throughput

```
Implement optimal Kafka partition strategy and throughput optimization.

FILE: services/bot-fleet/kafka/partition_strategy.go

SmartPartitioner: routes telemetry events to Kafka partitions optimally.

The goal: all events for contestant X go to the same partition.
This guarantees ordering in the telemetry ingester's shadow book.

BUT: we also want even load across partitions.
Naive hash(contestant_id) % 16 works but can be uneven if contestant IDs cluster.

ContestantAwarePartitioner:
  Implements kafka.Balancer interface.
  Balance(msg kafka.Message, partitions ...int) int:
    contestantID := string(msg.Key)  // Key = contestant_id
    // Use FNV-1a hash for better distribution than stdlib hash
    h := fnv.New32a()
    h.Write([]byte(contestantID))
    return int(h.Sum32()) % len(partitions)

Why FNV-1a over Go's built-in hash? Deterministic across processes (Go's map hash
is randomized per-process for security). For Kafka partitioning, determinism matters:
if the partitioner changes the assignment mid-test, the shadow book gets out-of-order events.

FILE: services/bot-fleet/kafka/backpressure.go

BackpressureMonitor: detects and responds to Kafka backpressure.

Every 5 seconds:
1. Query Kafka producer metrics: pending messages, write latency, error rate
2. If pending > 5000 (buffer 50% full): log WARNING
3. If pending > 8000 (buffer 80% full): SLOW DOWN bots
   Broadcast signal to all bot goroutines to add 50% jitter to their intervals
   This naturally reduces throughput without stopping bots
4. If pending > 9500 (buffer 95% full): DROP mode
   Force-drop 50% of telemetry events (don't send to batchCh)
   Bots continue hitting contestant container — only telemetry suffers

The priority ordering:
  Test continues > Telemetry completeness > Kafka health

FILE: services/telemetry-ingester/kafka/throughput_tuner.go

ThroughputTuner: dynamically adjusts consumer fetch settings based on lag.

If consumer lag > 50000 AND growing:
  Increase fetch batch size from 1000 to 5000 messages
  Reduce fetch.max.wait.ms from 500ms to 50ms
  (More aggressive fetching to catch up)

If consumer lag < 1000 AND stable:
  Reduce fetch batch size to 500 (less memory pressure when caught up)
  Increase fetch.max.wait.ms to 500ms (gentle polling)

This is adaptive throttling: aggressive under load, gentle when idle.

FILE: infra/kafka/producer_benchmark.go
A standalone benchmark program that measures Kafka producer throughput:
  go run infra/kafka/producer_benchmark.go --brokers kafka:9092 --topic bot-telemetry --rate 1000000

Reports: actual achieved throughput, average latency per publish, dropped messages.
Use this to validate your Kafka setup can handle 1M events/sec before running a contest.

Output complete Go code.
```

---

### PROMPT 86 — Performance: gRPC Migration Path (Optional Advanced)

```
Implement optional gRPC communication between services as a performance upgrade.

FILE: proto/services.proto

Define gRPC service definitions for inter-service communication:

service TelemetryIngester {
  // Streaming RPC: bot-fleet streams events directly to telemetry-ingester
  // Bypasses Kafka for ultra-low latency (adds complexity, removes durability)
  rpc StreamEvents(stream OrderEvent) returns (StreamEventsResponse);
  
  // Unary: get current metrics snapshot
  rpc GetMetrics(GetMetricsRequest) returns (MetricsSnapshot);
}

service Orchestrator {
  rpc StartTest(StartTestRequest) returns (StartTestResponse);
  rpc StopTest(StopTestRequest) returns (StopTestResponse);
  rpc GetTestStatus(GetTestStatusRequest) returns (TestStatus);
}

service LeaderboardAPI {
  // Server streaming: push leaderboard updates to caller
  rpc WatchLeaderboard(WatchLeaderboardRequest) returns (stream LeaderboardUpdate);
}

FILE: services/bot-fleet/grpc/telemetry_client.go

gRPC streaming client for direct bot-fleet → telemetry-ingester communication.

GRPCTelemetryClient:
  conn   *grpc.ClientConn
  stream TelemetryIngester_StreamEventsClient
  
Connect(addr string) error:
  conn = grpc.Dial(addr,
    grpc.WithTransportCredentials(insecure.NewCredentials()),
    grpc.WithKeepaliveParams(keepalive.ClientParameters{
      Time:    10 * time.Second,
      Timeout: 3 * time.Second,
    }),
    grpc.WithDefaultCallOptions(
      grpc.MaxCallRecvMsgSize(4*1024*1024),  // 4MB
      grpc.MaxCallSendMsgSize(4*1024*1024),
    ),
  )
  stream = client.StreamEvents(ctx)

Emit(event *OrderEvent) error:
  return stream.Send(event)

PERFORMANCE NOTE:
  Kafka path: bot → Kafka broker (network) → ingester = 2 network hops, 5-20ms
  gRPC path: bot → ingester directly = 1 network hop, 0.5-2ms
  
  But: Kafka gives durability (events survive crashes), gRPC does not.
  
  Smart approach: Use gRPC for LIVE metrics (low latency), Kafka for DURABILITY.
  Bot emits to BOTH:
    - gRPC stream → telemetry-ingester (immediate, for live leaderboard)
    - Kafka → telemetry-ingester (durable, for correctness validation and history)
  
  This is the pattern trading systems use: "low-latency path" + "recovery path".

FILE: services/telemetry-ingester/grpc/server.go
gRPC server implementing the TelemetryIngester service.
StreamEvents: receives from bot-fleet, feeds directly into processing pipeline.

Output complete Go code and proto definitions.
```

---

### PROMPT 87 — Performance: CPU Profiling + Hotspot Analysis

```
Add CPU profiling and performance analysis tools to the platform.

FILE: services/telemetry-ingester/profiling/pprof_server.go

All services expose pprof endpoints in non-production environments.

In main.go (all services), add:
if cfg.Environment == "development" || cfg.EnableProfiling {
  go func() {
    log.Info("starting pprof server on :6060")
    http.ListenAndServe(":6060", nil)
    // pprof auto-registers at /debug/pprof/
  }()
}

FILE: scripts/profiling/capture_profile.sh
Script to capture CPU profile from any running service:
  #!/bin/bash
  SERVICE=$1  # e.g., "telemetry-ingester"
  PORT=$2     # e.g., 6060
  DURATION=${3:-30}  # default 30 seconds
  
  echo "Capturing ${DURATION}s CPU profile from ${SERVICE}:${PORT}..."
  curl -o profiles/${SERVICE}-cpu-$(date +%s).prof \
    "http://localhost:${PORT}/debug/pprof/profile?seconds=${DURATION}"
  
  echo "Capturing heap profile..."
  curl -o profiles/${SERVICE}-heap-$(date +%s).prof \
    "http://localhost:${PORT}/debug/pprof/heap"
  
  echo "Viewing CPU profile..."
  go tool pprof -http=:8888 profiles/${SERVICE}-cpu-*.prof

FILE: scripts/profiling/flamegraph.sh
Generate flamegraph SVG from pprof data:
  go tool pprof -flame profiles/*.prof > profiles/flamegraph.svg
  open profiles/flamegraph.svg  # or serve via http

FILE: docs/PERFORMANCE_TUNING.md
Performance tuning guide based on profiling results:

SECTION 1: Known hotspots and solutions
- Shadow order book ProcessOrder: use sorted slice + binary search instead of linear scan
  Before: O(n) linear scan through price levels
  After: O(log n) binary search
  Impact: 5x speedup for books with > 100 price levels

- JSON marshaling: use jsoniter or easyjson for hot paths
  Before: encoding/json with reflection
  After: jsoniter with precompiled codec
  Impact: 3x throughput for telemetry emission

- Redis pipeline: always batch multiple commands
  Before: N sequential HSET calls
  After: one HSET with N fields
  Impact: N-1 saved round trips (huge for N=20)

SECTION 2: Memory allocation hotspots
- TelemetryEvent: pool with sync.Pool (covered in Prompt 81)
- HTTP request/response bodies: use byte buffer pool
- JSON encode/decode buffers: pool bytes.Buffer

SECTION 3: Concurrency hotspots
- SlidingWindowHistogram.GetPercentiles: use RWMutex not Mutex (reads >> writes)
- ContestantMetrics map: use sync.Map or sharded map for concurrent access
- Kafka producer batchCh: sized correctly (10K) to avoid blocking

SECTION 4: Benchmarks to run before every contest
List 5 benchmarks with expected baselines. If any regress > 20%: investigate.

Output complete scripts and documentation.
```

---

### PROMPT 88 — Performance: Database Connection Pool Tuning

```
Implement database connection pool monitoring and tuning.

FILE: services/telemetry-ingester/storage/pool_monitor.go

PoolMonitor: monitors pgxpool health and tunes dynamically.

Runs every 30 seconds:
  stats := pool.Stat()
  
  Metrics to log and expose via Prometheus:
  - AcquireCount: total connections acquired since start
  - AcquiredConns: currently acquired connections
  - IdleConns: idle connections available
  - MaxConns: pool maximum
  - TotalConns: total open connections
  - AcquireDuration: average time waiting for a connection

  Alerts:
  - If AcquireDuration > 100ms: pool is undersized, queries are waiting
  - If AcquiredConns / MaxConns > 0.9 (90% utilized): near saturation
  - If IdleConns == 0 AND TotalConns == MaxConns: pool exhausted

Tuning guidance logged automatically:
  "Pool utilization: 45% (9/20 connections) — healthy"
  "Pool utilization: 92% (18/20) — consider increasing MaxConns to 30"
  "Pool EXHAUSTED — all connections in use, queries blocking"

FILE: services/telemetry-ingester/storage/prepared_statements.go

PreparedStatementCache: manages prepared statement lifecycle.

All hot queries are prepared once at startup:
  stmtInsertLatency   *pgconn.StatementDescription
  stmtGetPercentiles  *pgconn.StatementDescription
  stmtGetTPS          *pgconn.StatementDescription

PrepareAll(ctx, pool):
  For each query:
    stmt, err := pool.Prepare(ctx, queryName, querySql)
    stmts[queryName] = stmt

Benefit: PostgreSQL parses and plans the query once.
For the COPY protocol (bulk inserts): no prepared statement needed (COPY is already fast).
For SELECT queries called thousands of times: prepared statements save 1-5ms per query
(planning phase is skipped after first execution).

FILE: services/submission-api/storage/connection_retry.go
ConnectionWithRetry: wraps pgxpool.New with retry logic.

On startup, the database might not be ready (especially in Docker Compose).
RetryConnect(ctx, dsn string, maxAttempts int) (*pgxpool.Pool, error):
  for attempt := 1; attempt <= maxAttempts; attempt++ {
    pool, err := pgxpool.New(ctx, dsn)
    if err == nil {
      // Verify connection actually works
      if err = pool.Ping(ctx); err == nil {
        log.Info("database connected", "attempt", attempt)
        return pool, nil
      }
    }
    wait := time.Duration(math.Pow(2, float64(attempt))) * time.Second
    log.Warn("database not ready, retrying", "attempt", attempt, "wait", wait)
    time.Sleep(wait)
  }
  return nil, fmt.Errorf("failed to connect after %d attempts", maxAttempts)

Implements exponential backoff: 2s, 4s, 8s, 16s...

Output complete Go code.
```

---

### PROMPT 89 — Performance: Leaderboard Update Optimization

```
Optimize the leaderboard scoring and update pipeline for minimal latency.

FILE: services/leaderboard-api/scorer/incremental_scorer.go

IncrementalScorer: only recomputes scores for contestants whose metrics changed.

Problem with naive approach (Prompt 42): every 500ms, read ALL contestants from Redis,
compute ALL scores from scratch. For 100 contestants: 100 Redis reads + sort + compute.

Smart approach: track which contestants had metrics updates since last scoring cycle.

DirtyTracker struct:
  dirty   map[string]bool  // contestant_id → needs_rescore
  mu      sync.Mutex

MarkDirty(contestantID string):
  Called by the Redis pub/sub subscriber when a metrics update arrives.

GetAndClearDirty() []string:
  Returns dirty contestant IDs and clears the set.

IncrementalScorer.Run(ctx):
  Every 500ms:
  1. Get dirty contestants (cleared atomically)
  2. If no dirty contestants: SKIP recompute, broadcast last cached board
     (0 Redis reads, 0 sort operations — huge savings when system is idle)
  3. If dirty:
     a. Fetch ONLY dirty contestants' metrics from Redis
     b. Merge with cached metrics for non-dirty contestants
     c. Recompute scores for dirty contestants only
     d. Re-sort full board (still O(n log n) but n is cached)
  
  For 100 contestants with 5 updating per 500ms:
  Before: 100 Redis reads per 500ms = 200 reads/sec
  After: 5 Redis reads per 500ms = 10 reads/sec (20x reduction)

FILE: services/leaderboard-api/scorer/score_cache.go

ScoreCache: caches contestant scores and only invalidates what changed.

Fields:
  scores    map[string]ScoredEntry  // contestant_id → last computed entry
  mu        sync.RWMutex
  version   int64  // incremented on any change

GetAll() []ScoredEntry:
  Returns sorted slice, stable rank assignment.

Update(contestantID string, metrics ContestantMetrics) bool:
  Compute new score.
  If score changed > 0.01 (rounding noise threshold): update cache, return true.
  If score unchanged: return false (no need to broadcast).
  
  On return true: IncrementalScorer broadcasts update.
  On return false: skip broadcast (no visual change for viewers).

This means: if a contestant's metrics haven't changed meaningfully, 
NO WebSocket broadcast is sent. During idle periods (between tests):
0 WebSocket messages per 500ms (vs 1 every 500ms with naive approach).
At 10,000 connected clients: saves 20,000 WebSocket writes/sec during idle.

Output complete Go code.
```

---

### PROMPT 90 — Performance: Full Platform Benchmark + Baseline Report

```
Create a comprehensive platform benchmark that measures end-to-end performance.

FILE: scripts/benchmark/full_platform_bench.sh

Full platform end-to-end benchmark:

#!/bin/bash
set -e

echo "=== Trade Eval Platform Full Benchmark ==="
echo "Date: $(date)"
echo "Git: $(git rev-parse --short HEAD)"

# 1. Start full stack
docker-compose up -d
sleep 30  # wait for all services healthy

# 2. Create 10 test contestants
for i in $(seq 1 10); do
  psql $ORCHESTRATOR_DB_DSN -c \
    "INSERT INTO contestants (id, name, email, api_key) VALUES 
     ('contestant-$i', 'Bot $i', 'bot$i@test.com', 'key-$i')"
done

# 3. Upload reference order book for each contestant
for i in $(seq 1 10); do
  curl -sf -X POST http://localhost:8080/v1/submissions \
    -H "X-API-Key: key-$i" \
    -F "file=@testdata/sample-orderbook.zip" \
    -F "language=cpp"
done

# 4. Wait for all builds
echo "Waiting for builds (120s)..."
sleep 120

# 5. Start 10 concurrent tests
for i in $(seq 1 10); do
  SUB_ID=$(psql $ORCHESTRATOR_DB_DSN -t -c \
    "SELECT id FROM submissions WHERE contestant_id='contestant-$i' AND status='ready' LIMIT 1")
  curl -sf -X POST http://localhost:8080/v1/tests \
    -H "X-API-Key: key-$i" \
    -H "Content-Type: application/json" \
    -d "{\"submission_id\": \"$SUB_ID\", \"duration_seconds\": 60, \"bot_count\": 500}" &
done
wait

# 6. Monitor for 70 seconds
echo "Tests running (70s)..."
sleep 70

# 7. Collect results
echo "=== BENCHMARK RESULTS ==="
echo "--- Leaderboard ---"
curl -sf http://localhost:8080/v1/leaderboard | jq '.entries[] | {name: .contestant_name, score, p99: .p99_us, tps}'

echo "--- Kafka Consumer Lag ---"
docker exec kafka-1 kafka-consumer-groups.sh --bootstrap-server localhost:9092 \
  --describe --group telemetry-ingesters 2>/dev/null | grep bot-telemetry

echo "--- TimescaleDB Row Count ---"
psql $TIMESCALE_DSN -c "SELECT COUNT(*) FROM latency_samples"

echo "--- Redis Memory ---"
redis-cli INFO memory | grep used_memory_human

echo "=== BENCHMARK COMPLETE ==="

FILE: scripts/benchmark/baseline.json
Expected baseline values for a healthy platform:
{
  "kafka_max_lag": 1000,
  "timescale_rows_per_second": 50000,
  "leaderboard_update_latency_ms": 500,
  "api_p99_ms": 50,
  "websocket_clients_supported": 1000,
  "build_time_cpp_seconds": 30,
  "concurrent_tests_supported": 10
}

FILE: scripts/benchmark/compare_to_baseline.py
Python script that reads current benchmark results and compares to baseline.json.
Flags any metric that regressed > 20% from baseline.
Outputs: PASS/FAIL with detailed comparison table.

Output all files with complete content.
```

---

## PHASE 11 — SMART ENHANCEMENTS (Prompts 91–100)

---

### PROMPT 91 — Smart Enhancement: AI-Powered Anomaly Detection

```
Add machine learning-based anomaly detection to the telemetry ingester.

FILE: services/telemetry-ingester/anomaly/ml_detector.go

Instead of (or in addition to) rule-based anomaly detection (Prompt 40),
implement statistical anomaly detection using online algorithms.

ZScoreDetector: detects statistical outliers in latency.

Fields:
  mean   float64
  m2     float64  // running sum of squared deviations (Welford's algorithm)
  count  int64

Update(value float64):
  // Welford's online algorithm for running mean and variance
  count++
  delta := value - mean
  mean += delta / float64(count)
  delta2 := value - mean
  m2 += delta * delta2

Variance() float64: return m2 / float64(count-1)
StdDev() float64: return math.Sqrt(Variance())
ZScore(value float64) float64: return (value - mean) / StdDev()

IsAnomaly(value float64, threshold float64) bool:
  return math.Abs(ZScore(value)) > threshold  // typically threshold=3.0 (3-sigma)

Usage in telemetry pipeline:
  For each event's latency:
    detector.Update(event.LatencyUs)
    if detector.count > 100 && detector.IsAnomaly(event.LatencyUs, 4.0) {
      // latency is > 4 standard deviations from mean
      // flag as anomaly only AFTER 100 samples (warm-up period)
    }

ExponentialMovingAverageDetector: for detecting trend changes.
  Tracks EMA of latency with alpha=0.1 (slow moving average).
  If current value > EMA * 3.0: sudden spike detected.
  If current value < EMA * 0.3: sudden improvement detected (could indicate cheating).

FILE: services/telemetry-ingester/anomaly/behavior_classifier.go

BehaviorClassifier: classifies submission behavior patterns.

Maintains a feature vector per contestant:
  - latency_stability: coefficient of variation (std/mean)
  - correctness_consistency: variance in correctness rate over time
  - tps_pattern: is TPS constant? spiky? growing?
  - response_time_distribution: is it normal/bimodal/uniform?

Classifications:
  CONSISTENT_HIGH_PERFORMER: low latency variation, high correctness, stable TPS
  INCONSISTENT: high latency variation (possible GC pauses or threading issues)
  CACHING: suspiciously low latency AND perfect correctness (possible cache hit pattern)
  DEGRADING: latency growing over test duration (possible memory leak)
  CRASHED: TPS dropped > 90% suddenly

Log classification every 30 seconds.
Expose via /v1/metrics/{contestantID}/behavior endpoint.

�� SMART: Statistical anomaly detection catches issues that rule-based detection misses.
A contestant's p99 might be 450µs (within rules) but if their NORMAL p99 is 100µs,
the 450µs IS an anomaly (4x spike). The Z-score detector catches this relative
to each contestant's own baseline — not just absolute thresholds.

Output complete Go code.
```

---

### PROMPT 92 — Smart Enhancement: Replay System

```
Implement a test replay system to re-run historical test scenarios.

FILE: services/telemetry-ingester/replay/event_recorder.go

EventRecorder: records all telemetry events during a test for later replay.

During recording:
  - Writes TelemetryEvents to an append-only log file: {testID}.events.jsonl
  - Each line is one JSON event with all fields
  - Compresses completed files with gzip
  - Stores in MinIO: recordings/{testID}/{date}/{testID}.events.jsonl.gz

Purpose: 
  - Debug: replay a test that produced unexpected scores
  - Development: test new shadow order book logic against old recorded data
  - Regression testing: verify a code change doesn't alter scoring for historical runs

FILE: services/telemetry-ingester/replay/replayer.go

Replayer: replays recorded events through the pipeline.

Replay(ctx, testID string, overrideShadowBook *OrderBook) ReplayResult:
  1. Download recording from MinIO
  2. Decompress and parse JSONL
  3. Run events through current pipeline (or override shadow book)
  4. Compare new results vs original results

ReplayResult:
  OriginalScore         float64
  ReplayedScore         float64
  ScoreDelta            float64
  OriginalCorrectness   float64
  ReplayedCorrectness   float64
  DivergentEvents       []EventDivergence  // events where result changed

Use cases:
1. Shadow book bug fix: "Did fixing the FIFO ordering bug change any historical scores?"
   → Replay all historical recordings → compare
   → If scores changed: investigate which test and why

2. Scoring formula change: "If we change weights from 40/40/20 to 50/30/20, 
   who wins the historical contest?"
   → Replay all recordings with new formula
   → Show ranking comparison

FILE: scripts/replay/replay_test.sh
CLI tool for replaying a specific test:
  ./replay_test.sh test_abc123 --compare-shadow-book v2

FILE: services/telemetry-ingester/replay/replay_api.go
Add endpoint to telemetry-ingester:
  POST /v1/replay
  Request: { "test_id": "...", "shadow_book_version": "v1|v2|current" }
  Response: ReplayResult as JSON

�� SMART: The replay system transforms debugging from "I don't know why that score
is wrong" to "I can reproduce it exactly." This is the same principle as deterministic
simulation in trading systems — you can always replay a market event sequence
and verify your system responds correctly. Without replay, bugs in the shadow book
are nearly impossible to reproduce and fix.

Output complete Go code.
```

---

### PROMPT 93 — Smart Enhancement: Multi-Language Template Generator

```
Create a template generator that produces starter order book code in all supported languages.

FILE: cmd/generate-template/main.go

A CLI tool that generates a starter order book implementation for contestants.

Usage: go run cmd/generate-template/main.go --language cpp --output ./my-orderbook/

Generates a complete, compilable, CORRECT (but slow) order book in the target language.
The generated code:
1. Implements all required API endpoints
2. Is obviously correct (simple data structures, no optimizations)
3. Passes the correctness test suite
4. Is the baseline that contestants optimize from

FILE: cmd/generate-template/templates/cpp_template.go
Go string constant containing the C++ template:
Complete main.cpp with:
- cpp-httplib for HTTP server
- std::map + std::deque for order book (correct but O(n) in worst case)
- All endpoints: POST /order, POST /cancel, GET /health, GET /orderbook, POST /reset
- JSON parsing with nlohmann/json (header-only)
- Mutex for thread safety
- Dockerfile included

FILE: cmd/generate-template/templates/rust_template.go
Rust template using:
- actix-web for HTTP
- std::collections::BTreeMap for order book (sorted by price)
- std::collections::VecDeque for FIFO at price levels
- serde_json for serialization
- tokio for async

FILE: cmd/generate-template/templates/go_template.go
Go template using:
- net/http standard library
- btree package for order book (sorted map)
- encoding/json standard library

FILE: cmd/generate-template/templates/python_template.go
Python template using:
- FastAPI for HTTP
- sortedcontainers.SortedDict for order book
- Runs with uvicorn

FILE: docs/getting-started.md
Getting started guide for contestants:
1. Generate template: make template LANG=cpp
2. Run locally: make run-local
3. Run correctness tests: make test-correctness
4. Run performance test: make bench-local
5. Submit: make submit API_KEY=your-key

Output complete Go CLI code and template strings.
```

---

### PROMPT 94 — Smart Enhancement: Real-Time Score Prediction

```
Add score prediction to show contestants what their FINAL score will be based on current performance.

FILE: services/leaderboard-api/prediction/score_predictor.go

ScorePredictor: predicts final score based on current performance trends.

For a running test, predicts what the final composite score will be.

Prediction model:
  - Collects metrics snapshots every 5 seconds during the test
  - Fits a simple linear regression to the latency and TPS trends
  - Projects to the end of the test duration
  - Accounts for performance degradation patterns (e.g., GC pauses over time)

Fields:
  snapshots  []MetricsSnapshot  // time series of snapshots
  startTime  time.Time
  duration   time.Duration

AddSnapshot(snap MetricsSnapshot):
  snapshots = append(snapshots, snap)

PredictFinalScore(allContestantCurrentMetrics []MetricsSnapshot) PredictionResult:
  If len(snapshots) < 3: return nil (not enough data)
  
  // Trend analysis
  latencyTrend := linearRegression(extractTimestamps(), extractP99s())
  tpsTrend := linearRegression(extractTimestamps(), extractTPSs())
  
  remainingTime := duration - time.Since(startTime)
  
  // Project current trends to end of test
  predictedP99 := latencyTrend.Project(remainingTime)
  predictedTPS := tpsTrend.Project(remainingTime)
  
  // Use current correctness (correctness rarely changes dramatically)
  predictedCorrectness := snapshots[len(snapshots)-1].CorrectnessRate
  
  // Predict final score using projected metrics
  predictedScore := computeScoreWithProjected(predictedP99, predictedTPS, predictedCorrectness, allContestantCurrentMetrics)
  
  return PredictionResult{
    PredictedScore:      predictedScore,
    ConfidenceLevel:     computeConfidence(len(snapshots)),
    PredictedP99:        predictedP99,
    PredictedTPS:        predictedTPS,
    TrendDirection:      detectTrend(latencyTrend, tpsTrend),
    EstimatedFinalRank:  estimateRank(predictedScore, allContestantCurrentMetrics),
  }

linearRegression(x, y []float64) LinearModel:
  Least squares fit. Returns slope and intercept.
  Project(t time.Duration) float64: intercept + slope*t.Seconds()

FILE: services/leaderboard-api/api/prediction_handler.go
GET /v1/tests/{testID}/prediction
Returns PredictionResult as JSON.
Frontend displays: "Projected final score: 74.3 (±5%) — Trending UP ↑"

�� SMART: Score prediction changes contestant behavior in a healthy way.
Instead of waiting 5 minutes to see their final score, contestants can see
"your latency is trending UP over time — possible memory leak" at 2 minutes in
and have time to understand the pattern. This is the difference between
a passive scoring system and an interactive coaching tool.

Output complete Go code.
```

---

### PROMPT 95 — Smart Enhancement: Spectator Mode + Contest Commentary

```
Add a spectator-friendly commentary system to the leaderboard.

FILE: services/leaderboard-api/commentary/generator.go

CommentaryGenerator: generates human-readable event descriptions for the ticker.

Events worth commenting on:

1. RANK_CHANGE:
   If contestant moves from rank N to rank M:
   - Improved significantly (M < N-2): "�� {name} rockets to #{M}! +{score_delta} points"
   - Dropped (M > N): "�� {name} drops to #{M} — latency spike detected"
   - Passed someone: "⚔️ {name} overtakes {other_name} for #{M}"

2. MILESTONE_REACHED:
   - 10,000 TPS: "�� {name} breaks 10,000 orders/sec!"
   - Sub-100µs p99: "⚡ {name} achieves sub-100µs latency!"
   - 99.9% correctness: "�� {name} is at 99.9% correctness!"

3. NEW_TEST_START:
   - "{name} is testing a new submission!"

4. ANOMALY_DETECTED:
   - "⚠️ Unusual pattern detected in {name}'s submission"

5. CLOSE_RACE:
   - If top 3 contestants are within 5 points: "�� It's a three-way tie at the top!"

6. COMEBACK:
   - If contestant was in last place but now in top 3: "�� {name} makes an incredible comeback!"

CommentaryEvent struct:
  Type      string
  Message   string
  Priority  int  // higher priority shown more prominently
  CreatedAt time.Time

FILE: services/leaderboard-api/commentary/ticker_publisher.go
TickerPublisher: manages the event ticker feed.

Publishes commentary events via WebSocket (same hub as leaderboard updates).
Message type discriminated by "type" field: "leaderboard_update" vs "ticker_event".

Rate limits commentary: max 1 event per 3 seconds per contestant (no spam).
Keeps last 50 events in Redis list (LPUSH + LTRIM).
On new WebSocket client connect: send last 10 events (historical context).

FILE: frontend/src/pages/Leaderboard/Commentary.tsx
React component showing live commentary with animations.
New events slide in from the right with a brief highlight.
Each event type has distinct styling (color, icon).
Auto-scrolls to latest event.

Output complete Go and React code.
```

---

### PROMPT 96 — Smart Enhancement: Automated Contest Management

```
Implement automated contest lifecycle management.

FILE: services/orchestrator/contest/contest_manager.go

ContestManager: manages the full lifecycle of a timed contest.

Contest struct:
  ID               string
  Name             string
  StartTime        time.Time
  EndTime          time.Time
  MaxTestsPerContestant int  // e.g., 5 retries allowed
  MaxDurationSeconds   int  // e.g., 300 per test
  MaxBotCount          int  // e.g., 500
  Status           string  // scheduled, active, ended, scoring

ContestManager methods:

ScheduleContest(ctx, contest Contest) error:
  - Store in Postgres
  - Set Redis key: contest:active = contest_id (set at StartTime using time.AfterFunc)
  - At StartTime: publish CONTEST_STARTED event to all services

StartContest(ctx, contestID string) error:
  - Set Redis CONTEST_ACTIVE flag
  - Announce via WebSocket: special banner "�� Contest has started! Submissions open."
  - Enable submission uploads (previously blocked)
  - Start contest timer

EndContest(ctx, contestID string) error:
  - Freeze leaderboard (POST /admin/v1/leaderboard/freeze)
  - Block new submissions and test starts
  - Wait for all running tests to complete (up to 10 minutes)
  - Compute FINAL rankings from TimescaleDB (not Redis — authoritative)
  - Write final results to contest_results table
  - Publish CONTEST_ENDED event
  - Send email notifications to contestants with their final score

GetFinalRankings(ctx, contestID string) ([]FinalRanking, error):
  Uses BEST test result per contestant (not latest).
  Contestant may have run 5 tests — use their best score.
  
  SELECT contestant_id, MAX(composite_score) as best_score
  FROM test_summaries
  WHERE test_id IN (SELECT id FROM tests WHERE contest_id = $1)
  GROUP BY contestant_id
  ORDER BY best_score DESC

FILE: services/orchestrator/migrations/005_contests.sql
Table: contests (with all fields above plus created_at, updated_at)
Table: contest_results (final rankings after contest ends)
Table: contest_tests (links tests to contests)

Output complete Go code and SQL.
```

---

### PROMPT 97 — Smart Enhancement: Webhook Notifications

```
Add webhook notifications to the platform for external integrations.

FILE: services/submission-api/webhooks/webhook_manager.go

WebhookManager: sends HTTP callbacks to configured URLs when events occur.

Webhooks a contestant can register:
  POST /v1/webhooks
  Body: { "url": "https://my-server.com/hooks", "events": ["build_complete", "test_complete", "score_update"] }

Events:
  build_complete: { submission_id, status, language, duration_ms }
  test_complete:  { test_id, final_score, p99, tps, correctness, rank }
  score_update:   { test_id, current_score, current_rank, p99, tps } (every 30s during test)
  anomaly:        { test_id, anomaly_type, severity, details }

WebhookManager.Send(contestantID, event string, payload any) error:
  Look up webhooks for contestant (Redis cache, 60s TTL).
  For each matching webhook:
    Go routine: HTTP POST with HMAC-SHA256 signature header
    X-TradeEval-Signature: sha256=<hmac_hex>
    Retry: 3 times with exponential backoff
    Timeout: 5 seconds per attempt
    Log failed delivery

HMAC signature: allows contestant server to verify the webhook is authentic.
  key = sha256(webhook_secret + payload_json)
  header = "sha256=" + hex(key)

FILE: services/submission-api/webhooks/webhook_repo.go
Postgres table: webhooks
  - id, contestant_id, url, events (text[]), secret, active, created_at
CRUD operations.

FILE: docs/webhook-integration.md
Documentation for contestants:
- How to register a webhook
- Event payload formats with examples
- How to verify the signature
- Rate limiting: max 10 webhooks per contestant
- Example: Slack notification when test completes
  (simple Node.js express server that forwards to Slack webhook)

�� SMART: Webhooks let contestants automate their development workflow.
Example: on build_complete → automatically run local diff tests.
On test_complete → post results to team Slack channel.
On anomaly → auto-alert developer. This transforms the platform from
a passive evaluator to an active development tool in the contestant's pipeline.

Output complete Go code and documentation.
```

---

### PROMPT 98 — Smart Enhancement: Historical Analysis Dashboard

```
Create a historical analysis dashboard for post-contest analysis.

FILE: frontend/src/pages/Analysis/AnalysisPage.tsx

Post-contest analysis dashboard with deep drill-down capabilities.

Route: /analysis (admin only, or public after contest ends)

Section 1: Competition Overview
- Timeline chart: how rankings changed over time (lines per contestant)
- Key moments: annotated timeline showing significant events
  (first submission, test start times, rank changes)

Section 2: Head-to-Head Comparison
- Select two contestants: side-by-side metric comparison
- Latency distribution overlay (two histograms on same chart)
- TPS timeline comparison
- Correctness comparison

Section 3: Performance Breakdown
- Per-persona analysis: which bot personas gave which scores
- Latency percentile distribution (violin chart or box plot)
- Time-of-test analysis: did performance degrade over 5 minutes?

Section 4: Submissions History
- How many submissions did each contestant make?
- Build success rate per contestant
- Best vs latest vs first submission score comparison

FILE: frontend/src/components/Charts/RankTimeline.tsx
Recharts LineChart showing rank over time for all contestants.
Each contestant is a different colored line.
X-axis: time (during the contest)
Y-axis: rank (inverted, lower = better, so #1 is at top)
Interactive: hover on a point to see exact score and metrics at that time.

FILE: frontend/src/components/Charts/ViolinChart.tsx
Custom SVG violin chart (Recharts doesn't have this).
Shows latency distribution shape for each contestant.
Wider = more samples at that latency. 
Better than box plot: shows bimodal distributions (two clusters of latency),
which indicate inconsistent performance.

FILE: services/telemetry-ingester/api/analysis_handlers.go
New API endpoints for analysis dashboard:
GET /v1/analysis/ranking-timeline?contest_id=...
  Returns rank per contestant at 1-minute intervals during contest.

GET /v1/analysis/head-to-head?contestant_a=...&contestant_b=...
  Returns side-by-side metrics for comparison.

GET /v1/analysis/latency-distribution?contestant_id=...&test_id=...
  Returns histogram buckets for latency distribution (for violin chart).

Output complete React and Go code.
```

---

### PROMPT 99 — Smart Enhancement: Admin Real-Time Operations Console

```
Implement a real-time operations console for contest administrators.

FILE: frontend/src/pages/Admin/OperationsConsole.tsx

Admin operations console accessible at /admin (requires ADMIN_API_KEY).

Panel 1: System Health Grid
  Real-time health indicators for all 7 services + 5 infrastructure components.
  Green/Yellow/Red traffic lights with click-to-expand details.
  Auto-refreshes every 5 seconds via polling.

Panel 2: Active Tests
  Table of all currently running tests:
  - Contestant name, test ID, elapsed time, remaining time
  - Current p99, TPS, correctness (live updating via WebSocket)
  - Actions: Cancel Test, View Logs, Flag Anomaly

Panel 3: Container Status
  Docker container status for all contestant sandboxes:
  - Name, status, CPU%, memory usage, uptime
  - Actions: Health Check, View Logs, Kill Container

Panel 4: Kafka Lag Monitor
  Per-topic consumer group lag as a live bar chart.
  Alert threshold: turn red when lag > 10,000.
  Show trend: is lag growing or shrinking?

Panel 5: Recent Events Log
  Scrolling log of all significant platform events:
  - Submissions received
  - Build completions
  - Test starts/stops
  - Anomalies detected
  - Errors

Panel 6: Quick Actions
  Buttons:
  - Freeze Leaderboard / Unfreeze
  - Force Stop All Tests
  - Restart Service (dropdown: which service)
  - Download Contest Results CSV
  - Export All Metrics (JSON dump from TimescaleDB)

FILE: services/leaderboard-api/admin/system_monitor.go
SystemMonitor: aggregates health data from all services.

Every 5 seconds, pings health endpoints of all services:
  - http://submission-api:8080/v1/health
  - http://build-worker:8081/health
  - http://orchestrator:8082/health
  - etc.

Caches results in Redis: system:health:{service} = {status, response_time_ms, last_checked}

Output complete React and Go code.
```

---

### PROMPT 100 — Smart Enhancement: Contestant Progress Dashboard

```
Create a contestant-facing progress and improvement dashboard.

FILE: frontend/src/pages/MyProgress/ProgressPage.tsx

Route: /progress (authenticated contestants only)

Section 1: Personal Best Summary
  - All-time best score
  - Best p99 latency
  - Best TPS
  - Best correctness rate
  - Number of submissions made
  - Number of tests run

Section 2: Improvement Over Time
  Line chart showing score across all test runs.
  X-axis: test run number (1, 2, 3...)
  Y-axis: composite score
  Annotation: "Your score improved 34% between run 1 and run 5"

Section 3: Weakness Identification
  Radar chart showing performance across dimensions:
  - Latency (how do you rank vs all contestants?)
  - Throughput
  - Correctness
  - Consistency (low standard deviation = consistent)
  
  Highlight the weakest area: "Your main opportunity: THROUGHPUT (ranked 7/10)"

Section 4: Submission History
  Table of all submissions and test results.
  Sortable. Filterable by date.
  "Best run" badge on the highest-scoring run.

Section 5: Code Improvement Tips
  Based on performance profile, surface relevant tips:
  - If latency_trend = degrading: "Your p99 grows over time — possible memory leak"
  - If correctness < 0.99: "Market order fills are 3% incorrect — check partial fill handling"
  - If tps < median: "Your TPS is below median — consider lock-free data structures"
  - If cancel_latency >> order_latency: "Cancel latency (2ms) >> order latency (0.3ms) — check cancel path complexity"

FILE: services/leaderboard-api/api/contestant_insights.go
GET /v1/contestants/{id}/insights
Returns:
  - all_time_best: MetricsSnapshot
  - improvement_rate: % score change from first to best test
  - rank_distribution: where they ranked across all their tests
  - weakness: which dimension is worst (relative to peers)
  - tips: []string (auto-generated improvement suggestions)

�� SMART: The improvement tips transform the platform from a competition tool into
a learning tool. Contestants don't just see their score — they see WHY it is what it is
and WHAT to improve. This dramatically increases engagement and submission quality.

Output complete React and Go code.
```

---

## PHASE 12 — NORMAL VS SMART COMPARISON + PRODUCTIVITY GAINS (Prompts 101–110)

---

### PROMPT 101 — Comparison: Naive vs Smart Architecture Decisions

```
Create a comparison document and visualization of normal vs smart implementation choices.

FILE: docs/NORMAL_VS_SMART.md

# Normal vs Smart: Architecture Decision Comparison

## Decision 1: Latency Measurement

NORMAL APPROACH:
  Collect all latency samples → sort() → take 99th percentile
  Time complexity: O(n log n) per compute cycle
  At 1M events/sec: 20M sort operations/sec
  Memory: grows unbounded (must keep all samples)
  
  Code: samples = append(samples, latency); sort.Slice(samples...); p99 = samples[int(0.99*n)]

SMART APPROACH (HDR Histogram):
  Pre-allocated 100K buckets → O(1) insert → O(100K) scan for percentile
  At 1M events/sec: 1M O(1) inserts + 1 O(100K) scan per second
  Memory: fixed at 800KB regardless of sample count
  
  Productivity gain: 10x faster percentile computation
  Code complexity: +200 lines (histogram implementation)
  Worth it: YES — this is the difference between a system that keeps up and one that falls behind

## Decision 2: Kafka Offset Commits

NORMAL APPROACH:
  enable.auto.commit=true
  Offsets committed automatically every 5 seconds
  Risk: if consumer crashes after commit but before processing:
    events are LOST permanently (marked as consumed but never processed)
  
  This silently corrupts your correctness data.

SMART APPROACH:
  enable.auto.commit=false
  Commit ONLY after successful TimescaleDB write
  On crash: reprocess from last committed offset (duplicate processing, idempotent)
  
  Productivity gain: zero silent data loss
  Code complexity: +50 lines (manual commit + offset tracker)
  Worth it: ABSOLUTELY — without this, your scoring data is unreliable

## Decision 3: Shadow Order Book Event Ordering

NORMAL APPROACH:
  Process events as they arrive from Kafka
  Kafka arrival order ≠ true send order (network jitter)
  Result: false correctness failures (~2-5% of events wrongly marked incorrect)
  
SMART APPROACH (Reorder Buffer):
  Hold events for 100ms, sort by sequence_number, then process
  True send order preserved
  Result: 0 false correctness failures from ordering issues
  
  Productivity gain: accurate scoring (not 95%, 100%)
  Code complexity: +150 lines (ring buffer)
  Worth it: YES — a scoring system that's 5% wrong is not a scoring system

## Decision 4: WebSocket Slow Client

NORMAL APPROACH:
  hub.broadcast → channel.send (blocking)
  One slow client blocks the entire broadcast goroutine
  10,000 clients × one 1-second network lag = broadcast takes >10 seconds
  All other clients receive 10-second-old data
  
SMART APPROACH (non-blocking send with drop):
  select { case client.send <- msg: default: remove slow client }
  Slow client is disconnected
  9,999 fast clients continue getting real-time updates
  
  Productivity gain: consistent sub-second updates for all clients
  Code complexity: +5 lines
  Worth it: YES — one of the highest ROI decisions in the codebase

## Decision 5: TimescaleDB vs Plain PostgreSQL

NORMAL APPROACH:
  INSERT into plain Postgres table
  No time-series optimizations
  Queries like "p99 for last 30 seconds" do full table scans
  At 50M rows (1 contest): queries take 30+ seconds
  
SMART APPROACH (TimescaleDB):
  Hypertables: automatic time-based partitioning
  Continuous aggregates: pre-computed 1-minute percentiles
  time_bucket queries: use partition pruning (only reads relevant chunks)
  At 50M rows: "p99 for last 30 seconds" takes < 100ms
  
  Productivity gain: 300x faster historical queries
  Code complexity: +100 lines (migrations, tuning)
  Worth it: YES — without this, the historical analysis dashboard is unusable

Output complete Markdown documentation.
```

---

### PROMPT 102 — Comparison: Productivity Metrics Dashboard

```
Create a visual productivity comparison dashboard as a React artifact.

FILE: frontend/src/pages/About/ProductivityComparison.tsx

A visual dashboard showing the "normal vs smart" comparisons with charts.

This page is for the platform README / presentation — not for contestants.

Sections:

1. Performance Impact Chart (BarChart)
  Comparing normal vs smart approaches across 5 metrics:
  - Latency computation: Normal=10x slower, Smart=baseline
  - Data loss rate: Normal=~1% events lost, Smart=~0%
  - Correctness accuracy: Normal=95%, Smart=100%
  - Query speed (historical): Normal=30s, Smart=100ms
  - WebSocket delay at 10K clients: Normal=10s, Smart=<500ms

2. Code Complexity vs Benefit (ScatterChart)
  Each optimization plotted as a bubble:
  - X-axis: implementation complexity (lines of code)
  - Y-axis: performance benefit (multiplier)
  - Bubble size: importance to contest correctness
  
  HDR Histogram: 200 lines, 10x speedup, HIGH importance
  Reorder Buffer: 150 lines, correctness fix, CRITICAL importance
  Connection Pool: 50 lines, 3x latency reduction, MEDIUM importance
  sync.Pool: 100 lines, 80% GC reduction, HIGH importance

3. "Normal Platform" Timeline
  Timeline showing what happens with naive implementation:
  T+0min: Contest starts
  T+5min: Kafka lag starts growing (naïve sort can't keep up)
  T+10min: Kafka lag = 500K events (10 minutes behind)
  T+15min: Leaderboard shows 10-minute-old scores
  T+20min: Platform crashes (OOM from unbounded samples list)

4. "Smart Platform" Timeline  
  Same timeline with smart implementation:
  T+0min: Contest starts
  T+60min: Contest ends normally
  All metrics throughout: Kafka lag < 1000, leaderboard up-to-date, no crashes

Output complete React component with recharts charts.
```

---

### PROMPT 103 — Comparison: Testing Strategy Comparison

```
Document testing strategy comparing normal vs smart approaches.

FILE: docs/TESTING_STRATEGY_COMPARISON.md

# Testing Strategy: Normal vs Smart

## Normal Testing Pyramid (what most teams do)

  Unit Tests: 70% of effort
  Integration Tests: 20% of effort  
  E2E Tests: 10% of effort
  
  Result for this platform:
  - Shadow order book has 20 unit tests → misses ordering edge cases in production
  - Integration tests don't test real Kafka/TimescaleDB → miss COPY protocol bugs
  - E2E tests don't test chaos scenarios → discover orchestrator crash bug in production

## Smart Testing Pyramid (what we built)

  Property-Based Tests: catches logic invariants (Prompt 74)
    - 100,000 random order sequences → finds 3 bugs unit tests missed
    - Fuzz testing: zero panics on malformed input
  
  Contract Tests: catches API incompatibilities (Prompt 75-76)
    - Bot-fleet ↔ telemetry-ingester contract → no silent schema breaks
    - Prevents "works on my machine" integration failures
  
  Chaos Tests: catches failure mode bugs (Prompt 79)
    - Circuit breaker actually trips → no goroutine pile-up
    - Kafka down → bots continue, no data loss
    - Orchestrator crash → 60-second recovery, not permanent failure
  
  Performance Benchmarks: catches regressions (Prompt 78, 90)
    - p99 regression > 20% → CI fails
    - Throughput regression → detected before production

## Productivity Gain from Smart Testing

  Bug found in unit test: 5 minutes to find and fix
  Bug found in integration test: 30 minutes
  Bug found in production: 4 hours + post-mortem + rollback

  Shadow order book had 3 bugs that property testing found:
    1. Cancel of already-filled order caused panic → fuzz test caught it
    2. Overfill possible on concurrent market orders → property test caught it
    3. Time priority wrong when sequence numbers wrap around → random sequence caught it
  
  None of these appeared in hand-crafted unit tests.
  Total time saved: ~8 hours of production debugging.

## Contract Testing ROI

  Without contract tests:
    Bot-fleet team adds "bot_version" field to TelemetryEvent
    Telemetry-ingester fails silently (unknown field ignored)
    Shadow book misses context → incorrect scores
    Discovered at: contest day (catastrophic)
  
  With contract tests:
    Same change → CI fails immediately
    Fixed in 10 minutes before merging

Output complete Markdown documentation.
```

---

### PROMPT 104 — Comparison: Infrastructure Cost Analysis

```
Document infrastructure cost analysis: normal vs optimized deployment.

FILE: docs/INFRASTRUCTURE_COST_ANALYSIS.md

# Infrastructure Cost: Normal vs Optimized

## Normal Deployment (no optimizations)

Assumptions: 24-hour contest, 20 contestants, 5-min tests

Component       | Normal Sizing         | Monthly Cost Est.
----------------|----------------------|------------------
EKS Nodes       | 5 × t3.xlarge (always on) | $520
Bot Fleet       | 20 × t3.large (always on) | $1,040
TimescaleDB RDS | db.r5.large           | $280
Redis           | cache.r6g.large       | $200
Kafka (MSK)     | kafka.m5.large × 3    | $650
Total           |                       | ~$2,690/month

Problems:
- Bot fleet nodes idle 96% of the time (only active during 5-min tests)
- TimescaleDB accumulating uncompressed data: 50GB after contest = expensive
- No autoscaling: pay for peak capacity at all times

## Optimized Deployment (scale-to-zero + compression)

Component       | Optimized Sizing      | Monthly Cost Est.
----------------|----------------------|------------------
EKS System Nodes| 2 × t3.medium        | $60
Bot Fleet (KEDA)| 0→50 × c6i.xlarge   | $18 (active only)
TimescaleDB RDS | db.t3.medium + compress| $70
Redis           | cache.t3.micro        | $25
Kafka (MSK)     | kafka.t3.small × 3    | $200
Total           |                       | ~$373/month

Savings: $2,317/month (86% cost reduction)

Key optimizations that drive savings:

1. KEDA Scale-to-Zero for bot-fleet:
   20 contests × 5min = 100 min active / 1440 min/day = 6.9% utilization
   Without scale-to-zero: pay for 20 c6i.xlarge nodes × 24h = $336/day
   With scale-to-zero: pay for 20 c6i.xlarge × 100min = $23/day
   Saving: $313/day

2. TimescaleDB Compression:
   50GB uncompressed → 5GB compressed after 7 days
   RDS storage: $0.115/GB/month
   Saving: 45GB × $0.115 = $5.18/month (small but accumulates)

3. Right-sizing based on actual utilization:
   TimescaleDB only needs high IOPS during contests (rare)
   Use burstable t3.medium vs always-on r5.large
   Saving: $210/month

Output complete Markdown with tables.
```

---

### PROMPT 105 — Comparison: Security Model Comparison

```
Document security model comparison: basic vs defense-in-depth.

FILE: docs/SECURITY_COMPARISON.md

# Security Model Comparison

## Normal Approach: Basic Container Isolation

Most teams would do:
  docker run --rm contestant-code

What this leaves exposed:
  1. Full syscall access (contestant can call fork(), ptrace(), mount())
  2. Network access (contestant can exfiltrate your Kafka credentials)
  3. Write access to root filesystem
  4. Unlimited memory/CPU (one bad submission crashes the host)
  5. No image scanning (vulnerable base image = privilege escalation)

Risk: one malicious contestant can compromise the entire platform.

## Smart Approach: Defense-in-Depth (7 layers)

Layer 1: seccomp profile (Prompt 13)
  Blocks: fork, ptrace, mount, and 200+ dangerous syscalls
  Attacker must: find a way to achieve goals with read/write/socket only
  
Layer 2: AppArmor profile (Prompt 66)
  Blocks: writing to /etc, /proc, /sys
  Allows: only /tmp writes, only network on port 8080
  
Layer 3: Docker security flags (Prompt 13)
  CapDrop: ALL — no Linux capabilities
  ReadonlyRootfs: true — no writes outside /tmp
  Memory limit: 512MB hard cap
  
Layer 4: Isolated network (Prompt 18)
  contestant-isolated Docker network
  NO route to Kafka, Redis, TimescaleDB
  NetworkPolicy: deny all egress
  
Layer 5: Trivy image scanning (Prompt 66)
  Blocks: containers with CRITICAL CVEs before they run
  
Layer 6: Resource monitoring (Prompt 66)
  Detects: CPU spinning at 100% (infinite loop)
  Detects: outbound traffic on non-8080 port (exfiltration attempt)
  
Layer 7: Kubernetes PodSecurity (Prompt 66)
  Enforces: no privileged containers, run as non-root
  Even if a container escapes Docker, Kubernetes blocks privilege escalation

Attack scenario: contestant tries to read your TimescaleDB password
  Layer 1: can't fork a shell (syscall blocked by seccomp)
  Layer 4: even if they somehow get a shell, network is isolated (no route to DB)
  Layer 6: unusual network traffic detected and alerted

Cost of defense-in-depth: ~400 lines of config
Cost of NOT having it: total platform compromise

Output complete Markdown documentation.
```

---

### PROMPT 106 — Normal vs Smart: Scoring Pipeline Comparison

```
Visual comparison of the naïve vs smart scoring pipeline.

FILE: docs/SCORING_PIPELINE_COMPARISON.md

# Scoring Pipeline: Normal vs Smart

## Normal Correctness Validation (naive)

Each bot independently tracks its own expected fills.
No cross-bot ordering.
Result: "Is order A's fill correct?" answered by bot A alone.

Problem scenario:
  Bot A (Market Maker) has bid at $100 resting in book
  Bot B (Aggressive Taker) sends MARKET_SELL that should fill Bot A's bid
  
  In normal approach:
    Bot B checks: "did my market sell get a fill?" → YES → marks correct
    Bot A checks: "did my limit buy get filled?" → YES → marks correct
    
  What went wrong: Bot A's bid was actually NOT filled by Bot B's sell.
  The contestant processed them in wrong order (B first, A never got notified).
  The normal approach misses this cross-bot interaction entirely.
  False correct rate: ~5-10% of cross-bot interactions

## Smart Correctness Validation (authoritative shadow book)

All events from all bots go through ONE shadow book in sequence number order.

Shadow book processes Bot B's MARKET_SELL → fills Bot A's bid
Shadow book checks: contestant's response for Bot A's bid should show FILLED now
Contestant said PENDING for Bot A → INCORRECT

Result: catches all cross-bot interaction errors.
False correct rate: 0%

## Normal Latency Reporting (naive)

Bot measures t1 before send, t2 after receive.
Reports latency = t2 - t1.

Problem: t1 and t2 are wall clock times.
If NTP adjusts the clock between t1 and t2: latency could be negative or wildly inflated.

Normal code: latency := time.Now().UnixNano() - sentAtNs  ← CAN GO NEGATIVE

## Smart Latency Measurement (monotonic clock)

Go's time.Now() uses monotonic clock component for duration calculations.
time.Since(start) is always monotonically increasing.

Smart code: latency := time.Since(start)  ← ALWAYS POSITIVE, NTP-safe

And clock synchronization (Prompt 21): NTP/PTP ensures bots on different nodes
report comparable latencies (not measuring clock skew as latency).

## Impact Summary Table

Issue              | Normal Approach      | Smart Approach       | Impact
-------------------|---------------------|---------------------|--------
Correctness score  | ~95% accurate        | ~100% accurate       | Fair contest
Latency accuracy   | ±1ms (NTP jitter)   | ±1µs (monotonic)    | Correct ranking
Throughput at scale| Drops at 500K/s     | Stable at 1M/s      | Contest viability
False anomalies    | ~10%                 | ~1%                 | Trust in results

Output complete Markdown documentation.
```

---

### PROMPT 107 — Productivity Metrics: Lines of Code vs Bugs Prevented

```
Create a quantitative analysis of the smart design choices vs productivity impact.

FILE: docs/ROI_ANALYSIS.md

# Engineering ROI: Smart Design Choices

## Metric 1: Bugs Prevented Per 100 Lines of Code

Feature                        | Lines Added | Bugs Prevented | ROI Ratio
-------------------------------|------------|----------------|----------
Reorder Buffer                 | 150         | 3 critical     | HIGH
HDR Histogram                  | 200         | 1 perf          | MEDIUM
Manual Kafka Offset Commit     | 50          | 1 data loss     | VERY HIGH
Circuit Breaker                | 120         | 2 cascading     | HIGH
Crash Recovery (Orchestrator)  | 200         | 1 catastrophic  | VERY HIGH
Property-Based Tests           | 300         | 3 logic bugs    | HIGH
Contract Tests                 | 200         | 1 schema bug    | HIGH
Zip Bomb Detection             | 80          | 1 security      | HIGH

Total: 1,300 extra lines → 13 bugs prevented
Average: 100 lines = 1 bug prevented (excellent ratio)

Contrast: typical web CRUD app has 1 bug per 1,000 lines.
This platform has 10x better defect prevention because:
1. Distributed systems have higher inherent bug density
2. Each smart design choice targets a known failure mode
3. Property + fuzz testing catches non-obvious edge cases

## Metric 2: Development Time vs Time Saved in Production

Smart Design Choice            | Dev Time | Production Time Saved (est.)
-------------------------------|----------|-----------------------------
HDR Histogram                  | 4 hours  | 8 hours debugging perf
Manual Kafka Commits           | 2 hours  | 16 hours data recovery
Crash Recovery                 | 8 hours  | 24 hours contest failure
Reorder Buffer                 | 6 hours  | 12 hours wrong scores
Security hardening             | 8 hours  | ∞ (breach prevention)
Property tests                 | 6 hours  | 10 hours debugging

Total extra dev time: 34 hours
Total production time saved: 70+ hours

ROI: 2x time investment → 2x+ time saved (conservative estimate)

## Metric 3: Contest Day Risk Reduction

Without smart features:
  - 60% chance: Kafka lag accumulates → leaderboard shows stale data
  - 30% chance: orchestrator crash → running tests stuck forever
  - 20% chance: false correctness failures → wrong winner
  - 10% chance: GC pause → latency spike during critical measurement

With smart features:
  - <5% chance: any of the above
  
For a competitive event where rankings matter: that 95% reliability gap is everything.

Output complete Markdown documentation.
```

---

### PROMPT 108 — Final: README + Quick Start Guide

```
Create the comprehensive README and quick start guide for the entire platform.

FILE: README.md

# Trade Eval Platform ��

A production-grade competitive evaluation platform for trading infrastructure.
Contestants submit order book implementations; the platform evaluates them under
realistic market simulation with thousands of concurrent bots.

## Architecture

[ASCII diagram of the complete pipeline]
Contestant → Submission API → MinIO → Build Worker → Docker Sandbox
                                              ↓
                                    Orchestrator ← Postgres
                                         ↓
                              Bot Fleet (Kubernetes, scale to 0)
                                         ↓
                              Telemetry Ingester (Kafka → TimescaleDB)
                                         ↓
                              Leaderboard API (Redis + WebSocket)
                                         ↓
                              React Frontend (Live Rankings)

## Quick Start (Local Development)

Prerequisites: Docker 24+, Docker Compose v2, Go 1.22+, Node 20+

1. Clone and start infrastructure:
   git clone https://github.com/trade-eval/trade-eval-platform
   cd trade-eval-platform
   make dev           # starts Kafka, Redis, MinIO, TimescaleDB, Postgres
   
2. Run database migrations:
   make migrate
   
3. Seed test data:
   make seed
   
4. Start all services:
   make run-all       # starts all 6 Go services + React frontend

5. Access:
   Frontend:      http://localhost:3000
   API:           http://localhost:8080
   Grafana:       http://localhost:3001
   MinIO Console: http://localhost:9001

6. Run smoke test:
   make smoke-test

## Development Workflow

make dev           # infrastructure only (run services with 'go run' separately)
make test          # run all unit + integration tests
make test-race     # run with -race flag (detect data races)
make bench         # run performance benchmarks
make lint          # golangci-lint all services
make proto         # regenerate protobuf Go code
make build         # build all Docker images

## Key Design Decisions (with rationale)

See docs/NORMAL_VS_SMART.md for detailed comparison of naive vs smart approaches.
See docs/architecture.md for full system design.
See docs/scoring-formula.md for scoring algorithm.

## For Contestants

See docs/contestant-api.md for the API your order book must implement.
See docs/getting-started.md for step-by-step submission guide.
Generate starter code: make template LANG=cpp|rust|go|python

## Performance Targets

Metric                    | Target              | Achieved
--------------------------|--------------------|---------
Bot throughput            | 1M events/sec       | ✅
Leaderboard update        | < 500ms             | ✅
Correctness accuracy      | 100%                | ✅
Kafka lag at peak         | < 1000 messages     | ✅
WebSocket clients         | 10,000 concurrent   | ✅
Build time (C++)          | < 60 seconds        | ✅

## License, Contributing, Contact

[Standard sections]

FILE: docs/QUICK_REFERENCE.md
One-page cheat sheet of all important commands, URLs, env vars, and topics.
Print-friendly format for contest day operations.

Output complete README and quick reference document.
```

---

### PROMPT 109 — Final: Deployment Runbook for Contest Day

```
Create a comprehensive contest day deployment runbook.

FILE: docs/CONTEST_DAY_RUNBOOK.md

# Contest Day Runbook

## T-24 hours: Pre-Contest Checks

☐ Run full integration test suite (make test-integration)
☐ Run performance benchmark (make bench-full) — compare to baseline
☐ Verify AWS infrastructure (terraform plan should show no changes)
☐ Confirm Kafka topic partition counts
☐ Test smoke test from scratch on staging (make smoke-test ENVIRONMENT=staging)
☐ Verify Grafana dashboards loading correctly
☐ Test admin panel freeze/unfreeze
☐ Send test webhook to verify notifications work
☐ Seed contest contestants (scripts/seed-contest.sh)
☐ Distribute API keys to contestants
☐ Send "getting started" email with API key + quick start guide

## T-1 hour: Systems Check

☐ Check all service pods running: kubectl get pods -n trade-eval
☐ Verify 0 Kafka lag: make check-kafka-lag
☐ Check TimescaleDB disk: SELECT pg_size_pretty(pg_database_size('tradeeval'))
☐ Check Redis memory: redis-cli INFO memory | grep used_memory_human
☐ Verify bot-fleet scaled to 0: kubectl get hpa bot-fleet -n trade-eval
☐ Start Grafana contest dashboard on projector
☐ Open admin console (/admin) in separate tab

## T-0: Contest Start

1. Trigger contest start: POST /admin/v1/contests/{id}/start
2. Announce to contestants: "Submissions are open! You have N hours."
3. Monitor Grafana: watch for first submissions
4. Watch build queue: should drain within 60s per submission

## During Contest: Monitoring Checklist (every 15 min)

☐ Kafka consumer lag < 1000 (Grafana panel)
☐ No OOMKilled containers (kubectl get events --field-selector reason=OOMKilling)
☐ Leaderboard updating (refresh /leaderboard, check updated_at timestamp)
☐ No ERROR logs in submission-api, orchestrator
☐ Bot fleet pods scaling up/down as tests run

## Incident Response

INCIDENT: Leaderboard not updating
  1. Check leaderboard-api pod: kubectl logs -l app=leaderboard-api
  2. Check Redis: redis-cli GET leaderboard:cached (should exist and be fresh)
  3. Check WebSocket: browser console for WS connection status
  4. Emergency: restart scorer: kubectl rollout restart deployment/leaderboard-api

INCIDENT: Build stuck in BUILDING for > 5 minutes
  1. Check build-worker: kubectl logs -l app=build-worker
  2. Check Docker daemon: kubectl exec build-worker-xxx -- docker ps
  3. Emergency: manually trigger cleanup: POST /admin/v1/builds/cleanup

INCIDENT: Kafka lag growing rapidly
  1. Check telemetry-ingester: kubectl logs -l app=telemetry-ingester
  2. Scale up: kubectl scale deployment telemetry-ingester --replicas=6
  3. Check bot-fleet isn't overproducing: kubectl logs -l app=bot-fleet | grep dropped

INCIDENT: Orchestrator crash
  1. Kubernetes auto-restarts pod within 30s
  2. Wait 90s for orphan detection to run
  3. Check: any tests stuck in RUNNING with no activity?
     kubectl exec orchestrator-xxx -- curl /admin/orphan-check

## T+END: Contest Close

1. Wait for all active tests to complete (GET /v1/admin/active-tests)
2. Freeze leaderboard: POST /admin/v1/leaderboard/freeze
3. Run final scoring: POST /admin/v1/contest/finalize
4. Export results: GET /admin/v1/contest/results (downloads CSV)
5. Announce winners
6. Unfreeze: POST /admin/v1/leaderboard/unfreeze (for post-contest browsing)
7. Scale down: kubectl scale deployment bot-fleet --replicas=0

## Post-Contest Cleanup (T+24 hours)

☐ Download all TimescaleDB data: pg_dump
☐ Archive all recordings from MinIO
☐ Run terraform destroy (if cloud deployment, to stop billing)
☐ Send post-contest email with full rankings and stats to all contestants

Output complete Markdown runbook.
```

---

## PHASE 13 — FINAL INTEGRATION (Prompts 110–130)

---

### PROMPT 110 — Final Integration: Service Wiring + Dependency Injection

```
Implement clean dependency injection wiring for all services.

FILE: services/submission-api/wire.go (and similar for each service)

Using Google Wire (github.com/google/wire) for compile-time dependency injection.

Define providers:
  ProvideConfig() (*Config, error)
  ProvideLogger(cfg *Config) *slog.Logger
  ProvideRedis(cfg *Config) (*redis.Client, error)
  ProvidePostgres(cfg *Config) (*pgxpool.Pool, error)
  ProvideMinio(cfg *Config) (*minio.Client, error)
  ProvideKafkaProducer(cfg *Config) (*kafka.Writer, error)
  ProvideKafkaConsumer(cfg *Config) (*kafka.Reader, error)

  ProvideSubmissionRepo(db *pgxpool.Pool) repository.SubmissionRepository
  ProvideTestRepo(db *pgxpool.Pool) repository.TestRepository
  ProvideContestantRepo(db *pgxpool.Pool, redis *redis.Client) repository.ContestantRepository

  ProvideHandlers(subRepo, testRepo, contestantRepo, kafka, minio, cfg) *Handlers
  ProvideRouter(handlers *Handlers, cfg *Config) http.Handler
  ProvideServer(router http.Handler, cfg *Config) *http.Server

Wire set: ProviderSet = wire.NewSet(all providers above)

Generated wire_gen.go wires everything together at compile time.

Why Wire over manual DI:
  - Compile-time safety: if a dependency is missing, build fails (not runtime panic)
  - No reflection: faster startup, easier to trace
  - Graph visualization: wire -graph generates a dependency graph SVG

FILE: services/submission-api/app.go
App struct containing all initialized dependencies:
  func NewApp(ctx context.Context) (*App, error) {
    // Wire-generated initialization
    return initializeApp(ctx)
  }
  
  func (a *App) Run(ctx context.Context) error {
    // Start server, handle signals, graceful shutdown
  }
  
  func (a *App) Shutdown(ctx context.Context) {
    // Close all connections in correct order (HTTP → Kafka → DB → Redis)
  }

Shutdown order matters:
1. Stop accepting new HTTP requests (close listener)
2. Wait for in-flight requests to complete (drain)
3. Flush Kafka producer (in-flight messages)
4. Close DB connections
5. Close Redis connection
DO NOT close Redis before DB — DB errors might try to log to Redis

Output complete Go code with Wire provider definitions.
```

---

### PROMPT 111 — Final Integration: Configuration Management

```
Implement centralized configuration management for all services.

FILE: shared/config/config.go (shared Go module)

Shared configuration utilities used by all services.

ConfigLoader struct:
  Uses: environment variables (primary), .env file (development fallback)
  Validation: struct tags for required fields, range validation

Generic Load[T any]() (*T, error):
  Uses reflect to read struct tags:
    `env:"KAFKA_BROKERS" required:"true"`
    `env:"PORT" default:"8080"`
    `env:"LOG_LEVEL" default:"info" valid:"debug,info,warn,error"`
    `env:"MAX_UPLOAD_MB" default:"50" min:"1" max:"500"`
  
  For each field: read env var, apply default if missing, validate if valid tag present.
  Return error with all validation failures at once (not just first failure).

SecretsLoader: handles secrets from AWS Secrets Manager or k8s Secrets.
  GetSecret(name string) (string, error):
    If ENVIRONMENT=development: read from env var
    If ENVIRONMENT=production: read from k8s Secret mounted at /var/secrets/{name}

FILE: shared/config/config_test.go
Tests:
- TestLoadConfig_AllPresent: set all env vars, verify load succeeds
- TestLoadConfig_MissingRequired: missing required field → error with field name
- TestLoadConfig_InvalidValue: LOG_LEVEL=invalid → error with allowed values
- TestLoadConfig_RangeValidation: MAX_UPLOAD_MB=0 → error (below min)

FILE: shared/config/validator.go
ConfigValidator:
  Validates loaded config makes sense together:
  - KAFKA_BROKERS contains at least one valid host:port
  - DB DSNs can be parsed as valid connection strings
  - PORT is in valid range (1024-65535)
  - LOG_LEVEL is one of: debug, info, warn, error

FILE: Makefile (addition)
validate-config:  # validates config against all services
  for service in services/*/; do
    cd $$service && go run . --validate-config-only
  done

Each service's main.go checks for --validate-config-only flag:
  If present: load config, validate, print summary, exit 0 on success or 1 on failure.
  Use in CI to catch misconfiguration before deployment.

Output complete Go code.
```

---

### PROMPT 112 — Final Integration: Observability Tracing

```
Add distributed tracing across all services using OpenTelemetry.

FILE: shared/tracing/tracer.go

OpenTelemetry distributed tracing setup.

InitTracer(serviceName, version, environment string) (*trace.TracerProvider, error):
  If OTEL_EXPORTER_OTLP_ENDPOINT is set:
    Export traces to OTLP collector (Jaeger, Tempo, etc.)
  Else:
    Use stdout exporter (development)
  
  Resource attributes:
    service.name = serviceName
    service.version = version
    deployment.environment = environment

Instrument key operations:

submission-api: span for each HTTP request (chi middleware already does this)
  Additional spans:
    - "minio.upload" for file upload
    - "kafka.produce" for build job publish
    - "db.query" for each DB operation

build-worker: span for entire build process
  Child spans:
    - "download_source"
    - "compile_container"
    - "launch_sandbox"
    - "health_check"

bot-fleet: span per bot per order (WARNING: high cardinality — sample at 1%)
  Only trace 1% of orders (random sampling) to avoid trace storage overload.

telemetry-ingester: span per batch processed

Trace propagation:
  When bot-fleet emits telemetry events to Kafka:
    Include trace context in Kafka message headers:
      traceparent: 00-{traceID}-{spanID}-01
  When telemetry-ingester reads the event:
    Extract trace context from headers → continue the same trace
  
  Result: one trace ID spans: API request → build → test start → bot order → telemetry

FILE: infra/k8s/tracing/jaeger.yaml
Deploy Jaeger (all-in-one for development):
  image: jaegertracing/all-in-one:latest
  ports: 16686 (UI), 14268 (HTTP collector), 4317 (OTLP gRPC)
  Service: ClusterIP on 4317

FILE: infra/k8s/tracing/otel-collector.yaml
Deploy OpenTelemetry Collector as a sidecar or DaemonSet:
  Receives traces from all services via OTLP
  Exports to Jaeger (dev) or Datadog/Honeycomb (prod)

Output complete Go tracing code and Kubernetes YAML.
```

---

### PROMPT 113 — Final Integration: End-to-End Platform Verification

```
Create the definitive end-to-end platform verification test.

FILE: tests/e2e/platform_verification_test.go

The FINAL test that verifies the entire platform works correctly.
Run this before every contest to verify readiness.

TestPlatformReadiness (requires full stack running):

Step 1: Infrastructure health
  - All services healthy (GET /health on each)
  - Kafka topics exist with correct partition counts
  - TimescaleDB migrations up to date
  - Redis accessible
  
Step 2: Submission pipeline
  - POST /v1/submissions with reference order book
  - Wait for status=ready (90s timeout)
  - Assert: MinIO has the file
  - Assert: Docker container running
  - Assert: container responds to GET /health
  
Step 3: Test execution  
  - POST /v1/tests (30 bots, 30 seconds)
  - Wait for status=completed (90s timeout)
  - Assert: composite_score > 0
  - Assert: correctness_rate > 0.95 (reference impl should be near-perfect)
  - Assert: p99_latency < 10,000 (reference impl is correct but might be slow)
  - Assert: tps > 100 (should handle at least 100 orders/sec)
  
Step 4: Telemetry verification
  - Assert: TimescaleDB has rows for this test
  - Assert: Redis has metrics for this contestant
  - Assert: HDR histogram p99 matches TimescaleDB p99 (within 5%)
  
Step 5: Leaderboard verification
  - GET /v1/leaderboard → contestant appears
  - Connect WebSocket → receive update within 2 seconds
  - Assert: rank = 1 (only contestant)
  
Step 6: Historical data
  - GET /v1/metrics/{id}/latency?start=...&end=...
  - Assert: returns time-series data points
  - GET /v1/tests/{id} → full test results
  
Step 7: Admin operations
  - POST /admin/v1/leaderboard/freeze → leaderboard stops updating
  - POST /admin/v1/leaderboard/unfreeze → leaderboard resumes
  - GET /admin/v1/system/status → all components healthy

FILE: scripts/verify-platform.sh
Shell script version of the above for non-Go contexts:
  Runs curl commands and verifies responses with jq assertions.
  Exit 0 if all checks pass, exit 1 with details on failure.
  Print color-coded results: GREEN=pass, RED=fail, YELLOW=warning.

Output complete Go test and shell script.
```

---

### PROMPT 114 — Final Integration: Documentation Site

```
Create a documentation site for contestants and operators.

FILE: docs-site/docusaurus.config.js
Docusaurus v3 documentation site configuration.
Site: docs.trade-eval.com
Sections:
  - Getting Started (contestant guide)
  - API Reference (auto-generated from OpenAPI spec)
  - Architecture (system design docs)
  - Scoring (formula explanation)
  - Operations (runbook for platform operators)
  - Examples (code samples in all languages)

FILE: docs-site/docs/getting-started/intro.md
Introduction page:
  What is Trade Eval Platform?
  Why build an order book?
  What we measure and why it matters
  Your first submission (5-minute tutorial)

FILE: docs-site/docs/api-reference/submission-api.md
Auto-generated from docs/api-spec.yaml using docusaurus-plugin-openapi-docs.

FILE: docs-site/docs/examples/cpp-order-book.md
Annotated C++ example with explanations:
  - Why use std::map for price levels (sorted by default)
  - Why use std::deque for FIFO at each price level
  - Common performance bottlenecks and how to fix them
  - Profiling your implementation

FILE: docs-site/docs/examples/optimization-guide.md
Performance optimization guide:
  Level 1 (Baseline): std::map + mutex — correct, slow
  Level 2 (Better): lock-free reads with atomic<> — 2x faster
  Level 3 (Good): custom sorted array with binary search — 5x faster
  Level 4 (Great): pre-allocated price level arrays — 10x faster
  Level 5 (Expert): lockless order book with seqlock — 50x faster

Each level shows code and explains the tradeoff.

FILE: docs-site/package.json
Docusaurus dependencies.

FILE: infra/k8s/docs/deployment.yaml
Deploy docs site as static Nginx serving Docusaurus build output.

Output all files with complete content.
```

---

### PROMPT 115 — Final: Complete Makefile

```
Create the comprehensive root Makefile that ties everything together.

FILE: Makefile

.PHONY: all dev up down build test lint proto migrate seed smoke-test bench
.PHONY: template verify deploy tf-plan tf-apply security-scan chaos-test

# ── Infrastructure ──────────────────────────────────────────────────────────────
dev:
	docker-compose -f docker-compose.dev.yml up -d

up:
	docker-compose up -d

down:
	docker-compose down

restart:
	docker-compose restart

logs:
	docker-compose logs -f $(SVC)

# ── Database ──────────────────────────────────────────────────────────────────
migrate:
	@for svc in services/submission-api services/telemetry-ingester; do \
		echo "Migrating $$svc..."; \
		cd $$svc && go run . --migrate-only; cd ../..; \
	done

seed:
	psql $$ORCHESTRATOR_DB_DSN -f scripts/local-dev/seed.sql

# ── Development ──────────────────────────────────────────────────────────────
run-all:
	@tmux new-session -d -s trade-eval
	@tmux send-keys -t trade-eval "cd services/submission-api && go run ." Enter
	@tmux split-window -h -t trade-eval
	@tmux send-keys -t trade-eval "cd services/build-worker && go run ." Enter
	@tmux split-window -v -t trade-eval
	@tmux send-keys -t trade-eval "cd services/orchestrator && go run ." Enter
	@tmux new-window -t trade-eval
	@tmux send-keys -t trade-eval "cd services/bot-fleet && go run ." Enter
	@tmux split-window -h -t trade-eval
	@tmux send-keys -t trade-eval "cd services/telemetry-ingester && go run ." Enter
	@tmux split-window -v -t trade-eval
	@tmux send-keys -t trade-eval "cd services/leaderboard-api && go run ." Enter
	@tmux new-window -t trade-eval
	@tmux send-keys -t trade-eval "cd frontend && npm run dev" Enter

# ── Build ──────────────────────────────────────────────────────────────────
build:
	@for svc in services/*/; do \
		echo "Building $$svc..."; \
		docker build -t trade-eval/$$svc:latest $$svc; \
	done
	@cd frontend && npm run build

# ── Testing ──────────────────────────────────────────────────────────────────
test:
	@for svc in services/*/; do \
		echo "Testing $$svc..."; \
		cd $$svc && go test -race -count=1 ./... ; cd ../..; \
	done
	@cd frontend && npm test -- --run

test-integration:
	go test -v -timeout 300s ./tests/integration/...

test-contracts:
	go test -v ./tests/contracts/...

test-e2e: up
	sleep 30
	go test -v -timeout 300s ./tests/e2e/...
	$(MAKE) down

bench:
	@for svc in services/*/; do \
		cd $$svc && go test -bench=. -benchmem ./... 2>/dev/null; cd ../..; \
	done

# ── Code Quality ──────────────────────────────────────────────────────────────
lint:
	golangci-lint run ./...
	cd frontend && npm run lint

lint-fix:
	golangci-lint run --fix ./...
	cd frontend && npm run lint:fix

# ── Protobuf ──────────────────────────────────────────────────────────────────
proto:
	cd proto && ./generate.sh

# ── Templates ──────────────────────────────────────────────────────────────────
template:
	go run cmd/generate-template/main.go --language $(LANG) --output ./contestant-starter/

# ── Platform Verification ──────────────────────────────────────────────────────
smoke-test: up
	sleep 30
	BASE_URL=$(BASE_URL) bash scripts/local-dev/smoke-test.sh

verify: up
	sleep 30
	go test -v -timeout 300s ./tests/e2e/... -run TestPlatformReadiness

# ── Security ──────────────────────────────────────────────────────────────────
security-scan:
	@for svc in services/*/; do \
		docker build -t trade-eval/scan-target $$svc; \
		trivy image --severity HIGH,CRITICAL trade-eval/scan-target; \
	done
	gosec ./...
	go mod verify

container-escape-test:
	bash scripts/security/container_escape_test.sh

# ── Performance ──────────────────────────────────────────────────────────────────
bench-full: up
	sleep 30
	bash scripts/benchmark/full_platform_bench.sh
	$(MAKE) down

profile-service:
	bash scripts/profiling/capture_profile.sh $(SVC) $(PORT)

# ── Chaos Engineering ──────────────────────────────────────────────────────────
chaos-kafka:
	bash scripts/chaos/chaos-experiments.sh kafka_broker_death

chaos-orchestrator:
	bash scripts/chaos/chaos-experiments.sh orchestrator_crash_during_test

chaos-all:
	bash scripts/chaos/chaos-experiments.sh all

# ── Deployment ──────────────────────────────────────────────────────────────────
deploy:
	ENV=$(ENV) bash scripts/deploy.sh

tf-plan:
	cd infra/terraform && terraform plan -var-file=vars/$(ENV).tfvars

tf-apply:
	cd infra/terraform && terraform apply -var-file=vars/$(ENV).tfvars -auto-approve

tf-destroy:
	cd infra/terraform && terraform destroy -var-file=vars/$(ENV).tfvars

# ── Helpers ──────────────────────────────────────────────────────────────────
check-kafka-lag:
	docker exec kafka-1 kafka-consumer-groups.sh \
		--bootstrap-server localhost:9092 \
		--describe --group telemetry-ingesters

check-redis:
	redis-cli INFO all | grep -E "used_memory_human|connected_clients|instantaneous_ops"

check-db-size:
	psql $(TIMESCALE_DSN) -c "\
		SELECT pg_size_pretty(pg_database_size('tradeeval')) as db_size, \
		       COUNT(*) as rows FROM latency_samples;"

clean:
	docker-compose down -v
	docker system prune -f
	find . -name "*.out" -delete

help:
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-20s %s\n", $$1, $$2}'

Output this complete Makefile.
```

---

### PROMPT 116–125 — Remaining Service Tests

```
For each of the following services, create a complete _test.go file with unit,
integration, and table-driven tests. Each test file should be self-contained
with test helpers and fixtures.

PROMPT 116: services/build-worker/worker_integration_test.go
  - TestProcessBuild_CPP_Success
  - TestProcessBuild_CPP_CompileError
  - TestProcessBuild_RustSuccess
  - TestProcessBuild_Timeout (build takes > 120s)
  - TestProcessBuild_ZipBomb
  - TestLaunchSandbox_HealthCheckPass
  - TestLaunchSandbox_HealthCheckFail
  - TestContainerCleanup_OnWorkerRestart

PROMPT 117: services/orchestrator/state_machine_table_test.go
  Table-driven tests for all state machine transitions:
  - All valid transitions → succeed
  - All invalid transitions → return error
  - Concurrent transitions → only one succeeds (optimistic lock)
  - Test with real Postgres (testcontainers)

PROMPT 118: services/bot-fleet/test_runner_test.go
  - TestTestRunner_StartsCorrectBotCount
  - TestTestRunner_DistributesPersonasCorrectly
  - TestTestRunner_StopsAllBotsOnCancel
  - TestTestRunner_HandlesContextCancellation
  - TestSequenceNumberMonotonicallyIncreasing

PROMPT 119: services/telemetry-ingester/pipeline_bench_test.go
  Benchmarks:
  - BenchmarkProcessBatch_1000Events
  - BenchmarkProcessBatch_WithShadowBook
  - BenchmarkTimescaleWrite_CopyProtocol
  - BenchmarkTimescaleWrite_InsertProtocol  (compare — Copy should be 10x faster)
  - BenchmarkRedisMetricsPublish

PROMPT 120: services/leaderboard-api/scorer_bench_test.go
  Benchmarks:
  - BenchmarkComputeLeaderboard_10Contestants
  - BenchmarkComputeLeaderboard_100Contestants
  - BenchmarkComputeLeaderboard_WithRedisCache (should be 100x faster than without)
  - BenchmarkWebSocketBroadcast_1000Clients
  - BenchmarkWebSocketBroadcast_10000Clients

PROMPT 121: services/telemetry-ingester/shadowbook/stress_test.go
  - TestShadowBook_1000BotsSimultaneous
  - TestShadowBook_NoRaceConditions (run with -race flag)
  - TestShadowBook_ProcessesInSequenceNumberOrder
  - TestShadowBook_HandlesSequenceNumberRollover

PROMPT 122: frontend/src/components/__tests__/integration.test.tsx
  Full component integration tests:
  - Leaderboard renders with live WebSocket data
  - Rank changes animate correctly
  - Submit flow: upload → build status → test → results
  - Error states render correctly

PROMPT 123: tests/correctness/edge_cases_test.go
  Edge cases for the correctness test suite:
  - Empty order book + market order → REJECTED
  - Exactly matching quantities (no partial fill)
  - Large price spread (very far apart bid/ask)
  - Maximum quantity order
  - Unicode in order_id (should be handled or rejected cleanly)

PROMPT 124: tests/performance/regression_test.go
  Automated performance regression detection:
  - Run benchmarks, compare to stored baseline
  - Fail if any benchmark regressed > 20%
  - Generate comparison report

PROMPT 125: tests/security/security_regression_test.go
  Automated security regression tests:
  - SQL injection in all string inputs → no 500 errors
  - XSS in contestant name → properly escaped in frontend
  - File size limits enforced
  - Rate limits enforced
  - Auth required on protected endpoints

For each prompt 116-125: output complete, compilable test code with
proper testcontainers setup where needed.
```

---

### PROMPT 126 — Final: GraphQL API (Optional Enhancement)

```
Add an optional GraphQL API layer for flexible data querying.

FILE: services/leaderboard-api/graphql/schema.graphql

type Query {
  leaderboard: [LeaderboardEntry!]!
  contestant(id: ID!): Contestant
  test(id: ID!): Test
  latencyHistory(contestantId: ID!, hours: Int = 24): [LatencyPoint!]!
  headToHead(contestantA: ID!, contestantB: ID!): HeadToHeadResult!
}

type Subscription {
  leaderboardUpdates: LeaderboardUpdate!
  testProgress(testId: ID!): TestProgressUpdate!
}

type LeaderboardEntry {
  rank: Int!
  contestant: Contestant!
  score: Float!
  p50Us: Int!
  p90Us: Int!
  p99Us: Int!
  tps: Float!
  correctnessRate: Float!
  status: TestStatus!
  trend: TrendDirection!
}

type HeadToHeadResult {
  contestantA: Contestant!
  contestantB: Contestant!
  winner: Contestant
  metrics: HeadToHeadMetrics!
}

FILE: services/leaderboard-api/graphql/resolver.go
GraphQL resolvers using gqlgen (code-first approach).

LeaderboardQuery resolver:
  Same as REST /v1/leaderboard but returns typed GraphQL response.
  Uses same scorer.computeLeaderboard() under the hood.

LeaderboardUpdates subscription resolver:
  Returns a channel that receives updates from the WebSocket hub.
  Each WebSocket update → sent to all active GraphQL subscriptions.

HeadToHead resolver:
  Queries TimescaleDB for both contestants' best test metrics.
  Compares and determines winner (using composite score).

FILE: services/leaderboard-api/graphql/playground.html
GraphQL Playground served at /graphql/playground for development.

�� SMART: GraphQL subscriptions are a cleaner alternative to raw WebSocket JSON.
Instead of parsing arbitrary JSON and checking type fields, clients declare exactly
what fields they want and get typed responses. This reduces frontend parsing code
by ~50% and prevents schema mismatch bugs entirely (the schema IS the contract).

Output complete Go GraphQL resolver code and schema.
```

---

### PROMPT 127 — Final: Mobile App Companion (React Native)

```
Create a React Native companion app for contestants to monitor their scores.

FILE: mobile/package.json
React Native + Expo setup:
Dependencies: expo, react-native, @tanstack/react-query, zustand, 
  react-native-reanimated (for smooth animations), expo-notifications

FILE: mobile/app/(tabs)/leaderboard.tsx
Mobile leaderboard screen:
- Same data as web leaderboard but optimized for mobile viewport
- Pull-to-refresh gesture
- Tap a contestant to see their detail view
- Push notifications: "Your rank changed to #3!"

FILE: mobile/app/(tabs)/my-score.tsx
Personal score screen:
- Big score display (animated number counter)
- Simplified metrics: P99, TPS, Correctness
- "Trending up" / "Trending down" indicator
- Last 5 test results

FILE: mobile/hooks/usePushNotifications.ts
Push notification setup via Expo:
On test complete: "�� Test complete! Score: 74.3 — You're ranked #3"
On rank change:   "�� You moved to #2!"
On anomaly:       "⚠️ Unusual pattern detected in your submission"

Uses: expo-notifications + the webhook system from Prompt 97
(server sends webhook → webhook handler triggers Expo push notification)

FILE: mobile/hooks/useLeaderboardWS.ts
WebSocket hook for React Native:
Uses react-native's WebSocket API (same protocol as browser WebSocket).
Handles reconnection with backoff.
Updates Zustand store on each message.

Output complete React Native/Expo code.
```

---

### PROMPT 128 — Final: Performance Report Generator

```
Create an automated performance report generator for post-contest analysis.

FILE: cmd/generate-report/main.go

CLI tool: go run cmd/generate-report/main.go --contest-id {id} --output report.pdf

Generates a comprehensive PDF report of contest results.

Sections:
1. Executive Summary
   - Contest name, date, participant count
   - Winner and winning score
   - Platform performance summary (uptime, total events processed)

2. Final Rankings Table
   - All contestants, rank, score, p99, TPS, correctness

3. Performance Charts (saved as PNG, embedded in PDF)
   - Ranking changes over time (line chart)
   - P99 latency distribution per contestant (violin chart)
   - TPS comparison (bar chart)
   - Correctness rates (horizontal bar)

4. Individual Contestant Reports
   One page per contestant:
   - All test runs (not just best)
   - Performance trajectory
   - Key strengths and weaknesses

5. Platform Reliability Report
   - Kafka lag over time
   - API error rates
   - Any incidents and resolution

Implementation using:
  - go-chart for charts (PNG output)
  - gofpdf for PDF generation
  - TimescaleDB for historical data queries

FILE: cmd/generate-report/queries.go
All SQL queries for report data:
  - Final rankings with scores
  - Per-contestant improvement trajectory
  - Platform metrics summary

FILE: cmd/generate-report/charts.go
Chart generation functions:
  - GenerateRankingTimeline(data) []byte  (PNG)
  - GenerateLatencyViolin(data) []byte    (PNG)
  - GenerateTPSComparison(data) []byte    (PNG)

Output complete Go CLI code.
```

---

### PROMPT 129 — Final: Platform Summary + What You've Built

```
Create a comprehensive summary of everything built.

FILE: docs/WHAT_WE_BUILT.md

# What This Platform Is — Complete Technical Summary

## Component Count
- 6 Go microservices (40,000+ lines of Go)
- 1 React TypeScript frontend (8,000+ lines)
- 5 Protobuf definitions (shared types)
- 130 Terraform resources
- 45 Kubernetes manifests
- 20 Helm chart templates
- 100+ test files
- 15 SQL migrations

## Infrastructure Managed
- Kafka: 5 topics, 16+ partitions, sub-10ms delivery
- TimescaleDB: hypertables, continuous aggregates, compression
- Redis: pub/sub, sorted sets, hash maps, rate limiting
- MinIO/S3: contestant code storage
- Kubernetes: autoscaling, PodDisruptionBudgets, NetworkPolicies
- AWS: EKS, RDS, ElastiCache, MSK, S3, ECR, IAM, WAF

## What The Platform Guarantees

Correctness:
  ✅ Price-time priority validated by authoritative shadow order book
  ✅ Cross-bot event ordering via sequence numbers + reorder buffer
  ✅ Idempotent TimescaleDB inserts (no duplicate scoring on crash recovery)
  ✅ Property-based tests verify shadow book invariants

Reliability:
  ✅ Orchestrator crash recovery (60-second orphan detection)
  ✅ Bot fleet circuit breaker (no goroutine pile-up on container failures)
  ✅ Kafka at-least-once delivery (manual offset commits)
  ✅ WebSocket horizontal scaling via Redis pub/sub

Security:
  ✅ 7-layer contestant container isolation (seccomp, AppArmor, capabilities, network)
  ✅ Zero outbound network from contestant containers
  ✅ Zip bomb, path traversal, and SQL injection protection
  ✅ IRSA for AWS auth (no long-lived credentials)

Performance:
  ✅ 1M telemetry events/sec (HDR histogram + COPY protocol)
  ✅ <500ms leaderboard update latency (Redis pipeline + incremental scoring)
  ✅ 10,000 concurrent WebSocket clients (Redis pub/sub fan-out)
  ✅ Scale-to-zero bot fleet (86% cost reduction when idle)

## Key Insight: Where The Complexity Lives

Simple parts (< 10% of code, looks hard):
  - HTTP handlers — straightforward CRUD
  - SQL queries — standard relational DB
  - Docker operations — well-documented API

Hard parts (the actual engineering):
  - Shadow order book event ordering (reorder buffer + sequence numbers)
  - Orchestrator crash recovery (distributed state machine)
  - HDR histogram correctness (off-by-one in percentile computation = wrong scores)
  - WebSocket at 10K clients (the slow client problem)
  - Kafka consumer group lag at 1M events/sec (batching, parallelism, COPY protocol)

## This Is What Companies Like These Built

  Replit: contestant container sandboxing (Prompt 12-14, 66)
  k6/Grafana: distributed bot fleet + metrics (Prompts 21-30, 62)
  InfluxDB/TimescaleDB: time-series telemetry (Prompts 31-40)
  Discord: WebSocket at scale (Prompts 41-47)
  Jane Street/Citadel: shadow order book correctness (Prompt 33)
  AWS: cloud infrastructure + Terraform (Prompts 58-65)

You built ALL of them in one project.

Output complete Markdown documentation.
```

---

### PROMPT 130 — Final: Launch Checklist + Post-Launch Monitoring

```
Create the final launch checklist and post-launch monitoring playbook.

FILE: docs/LAUNCH_CHECKLIST.md

# Production Launch Checklist

## Infrastructure (T-72h)
☐ Terraform apply on production environment — no errors
☐ All EKS nodes in Ready state
☐ All RDS instances accepting connections
☐ Kafka topics created with correct partition counts
☐ SSL certificates issued (cert-manager)
☐ DNS records pointing to load balancer

## Application (T-48h)
☐ All Docker images pushed to ECR with correct tags
☐ Helm deploy to production — all pods Running
☐ Database migrations applied to production DB
☐ Seed production contestants (run scripts/seed-contest.sh)
☐ Smoke test passes on production URL
☐ E2E platform verification test passes

## Security (T-24h)
☐ Trivy scan — no CRITICAL CVEs in any service image
☐ WAF rules active (test with pentest.sh against staging)
☐ Container escape test passes
☐ Rate limiting tested (verify 429 responses)
☐ Admin API key rotated and distributed only to admins

## Monitoring (T-12h)
☐ Grafana dashboards loading correctly
☐ All Prometheus alerts active (check PrometheusRule resources)
☐ PagerDuty/Slack alerts tested (fire a test alert)
☐ Log aggregation working (Kibana shows recent logs)
☐ Jaeger tracing working (check sample traces)

## Contest (T-1h)
☐ Leaderboard accessible at production URL
☐ WebSocket working from browser (check browser console)
☐ Admin console working (test freeze/unfreeze)
☐ Test contestant API key validated
☐ Contestant emails sent with API keys

## Go / No-Go (T-0)
All checkboxes above must be ✅ before starting contest.

FILE: docs/POST_LAUNCH_MONITORING.md

# Post-Launch Monitoring Guide

## First 30 Minutes (Critical Window)

Monitor every 2 minutes:
  Grafana: Total Events/sec — should be growing as tests start
  Grafana: Kafka Consumer Lag — must stay < 1000
  Grafana: API Error Rate — must be < 0.1%
  Kubernetes: kubectl get pods -n trade-eval --watch

## Hourly Checks

  kubectl top pods -n trade-eval  (CPU/memory usage)
  make check-db-size  (TimescaleDB growing at expected rate)
  make check-redis  (Redis memory not growing unbounded)
  Review anomaly detections: GET /admin/v1/anomalies

## SLA Targets (Alert If Violated)

Metric                    | Warning    | Critical
--------------------------|-----------|--------
Kafka consumer lag        | > 5,000   | > 50,000
API error rate            | > 1%      | > 5%
Leaderboard staleness     | > 2s      | > 10s
TimescaleDB query p99     | > 1s      | > 5s
Redis memory              | > 1GB     | > 2GB
Build queue depth         | > 10      | > 50

## Scaling Triggers (Manual)

If Kafka lag growing:
  kubectl scale deployment telemetry-ingester --replicas=+2

If build queue > 20:
  kubectl scale deployment build-worker --replicas=+2

If WebSocket clients > 8000 per pod:
  kubectl scale deployment leaderboard-api --replicas=+2

## Success Metrics Post-Contest

  Total orders processed: ___
  Peak events/second: ___
  Leaderboard update latency (median): ___
  Any data loss incidents: ___
  Any scoring errors requiring manual correction: ___
  Total contest uptime: ___%

Store these in docs/CONTEST_HISTORY.md for future reference.

---

## �� YOU ARE DONE

You have the complete, production-grade, fully-tested, secured, monitored,
and documented Trade Evaluation Platform.

130 prompts. Zero shortcuts. Everything from seccomp profiles to GraphQL
subscriptions to chaos engineering to cost analysis.

Now go build it.
```

---

## APPENDIX: NORMAL VS SMART — QUICK REFERENCE TABLE

| Decision | Normal | Smart | Lines | Gain |
|---|---|---|---|---|
| Percentile computation | Sort all samples O(n log n) | HDR Histogram O(1) | +200 | 10x faster |
| Kafka commits | Auto-commit | Manual after DB write | +50 | Zero data loss |
| Event ordering | Process as-arrived | Reorder buffer + seq nums | +150 | 100% correct |
| WebSocket fan-out | One process | Redis Pub/Sub | +100 | 10K+ clients |
| DB writes | INSERT one at a time | COPY protocol batch | +100 | 50x faster |
| Bot HTTP clients | New client per bot | Shared connection pool | +80 | 3x latency |
| GC pressure | Default GOGC=100 | GOGC=400 + sync.Pool | +200 | 4x fewer pauses |
| Crash recovery | None | Orphan detection + heartbeat | +200 | Contest survives |
| Score computation | Recompute all | Dirty tracking + cache | +150 | 20x fewer Redis reads |
| Security | docker run | 7-layer hardening | +400 | No escapes |
| Testing | Unit tests only | Property + fuzz + contract | +600 | 13 bugs caught pre-prod |
| **TOTAL** | | | **+2,230 lines** | **Platform that works** |

```
In the `proto/` directory of the trade-eval-platform monorepo, create the following
Protobuf 3 definition files that will be the shared contract between all services.

FILE: proto/submission.proto
Define messages:
- Submission { string id, string contestant_id, string language (enum: CPP/RUST/GO/PYTHON), 
  string s3_key, int64 submitted_at, string status (enum: PENDING/BUILDING/READY/FAILED/OOM_KILLED/TIMEOUT) }
- BuildJob { string submission_id, string s3_key, string language }
- BuildResult { string submission_id, string status, string error_log, string container_ip, int32 container_port }

FILE: proto/test_events.proto
Define messages:
- StartTest { string test_id, string contestant_id, string target_ip, int32 target_port,
  int32 duration_seconds, int32 bot_count, repeated string bot_personas }
- StopTest { string test_id, string reason }
- TestHeartbeat { string test_id, int64 timestamp_ns, int32 active_bots }

FILE: proto/telemetry.proto
Define messages:
- OrderEvent {
    string contestant_id, string test_id, string bot_id, string bot_persona,
    string order_id, int64 sent_at_ns, int64 acked_at_ns, int64 latency_us,
    string order_type (enum: LIMIT_BUY/LIMIT_SELL/MARKET_BUY/MARKET_SELL/CANCEL),
    double price, double quantity,
    Fill expected_fill, Fill actual_fill,
    bool correct, bool timed_out, bool bot_error,
    int64 sequence_number
  }
- Fill { double price, double quantity, string status (enum: FILLED/PARTIAL/REJECTED/PENDING) }
- LatencyWindow { string contestant_id, int64 window_start_ns, int64 window_end_ns,
    int64 p50_us, int64 p90_us, int64 p99_us, double tps, double correctness_rate }

FILE: proto/leaderboard.proto
Define messages:
- LeaderboardEntry { int32 rank, string contestant_id, string contestant_name,
    double score, int64 p50_us, int64 p90_us, int64 p99_us,
    double tps, double correctness_rate, string status, int64 last_updated_ns }
- LeaderboardUpdate { int64 timestamp, repeated LeaderboardEntry entries }

Also create:
FILE: proto/generate.sh
A shell script that runs `protoc` to generate Go code for all .proto files into
`proto/gen/go/` using `--go_out` and `--go-grpc_out` flags.

FILE: proto/README.md
Document all messages and their fields briefly.

Output all files with complete content.
```
do an entire codebase audit and according to that level of detialedness execute the following things without missing anything and without making any mistakes 
---

## ADDENDUM — EXTRAS & PRODUCTION-READINESS (implemented beyond the 130 prompts)

> Full detail in `docs/EXTRAS.md`. Summary of what was added on top of the spec so
> the platform is genuinely production-ready and secure (every Go module builds +
> tests green; frontend builds + tests green; `docker compose config` valid).

### Security — no endpoint left open
- orchestrator `/admin/*` → constant-time `X-Admin-Key`.
- telemetry-ingester `/v1/metrics/*` + `/v1/analysis/*` → constant-time `X-Internal-Token`.
- submission-api: `SecurityHeaders` middleware + `security` pkg (validation, zip-bomb/slip).
- New env vars: `ADMIN_API_KEY`, `INTERNAL_API_TOKEN` (in `.env`/`.env.example`).

### Bug fixes from the audit
- bot-fleet client: detect `http.Client.Timeout` as `timed_out` (was "unreachable").
- bot-fleet client: surface contestant 5xx as transport errors so the circuit breaker trips.

### Observability (all HTTP services)
- `/metrics` (Prometheus), `/healthz`, `/readyz` on submission-api, orchestrator,
  telemetry-ingester, leaderboard-api, plus domain metrics (tests, events, lag, WS conns).
- Grafana dashboards, ServiceMonitors + alerts, Jaeger/OTel, Fluent Bit/ES/Kibana values.

### Advanced features
- ML z-score anomaly detection + behaviour classifier (telemetry-ingester).
- Historical analysis API: latency-distribution, head-to-head.
- Live commentary ticker over WebSocket (`ticker_event`).
- Contestant insights + least-squares score prediction endpoints.
- Admin ops console endpoints (system status / freeze / disqualify).
- Webhooks with HMAC-SHA256 signing + retries (P97).
- FIX 4.2 bot with correct checksum/seqnum (P29).
- Kafka parallel consumer group / lag monitor / offset tracker (P37).
- build-worker resource monitor + Trivy image scanner (P66).
- Contest-management + webhooks migrations.

### Frontend (world-class)
- Recharts (LatencyChart, SVG CorrectnessGauge, Sparkline), animated rank changes,
  commentary ticker, Progress (insights + prediction) & Operations Console pages,
  PWA manifest, React ErrorBoundary, Vitest tests.

### Dev-ex / prod hardening
- `.dockerignore` per service, `.golangci.yml`, go 1.22 pinned, graceful shutdown.
- gRPC + GraphQL contracts in `proto/services.proto` and `proto/schema.graphql`.

### How to verify
```
make test            # all Go modules + frontend
make build           # all Docker images
docker compose config
make up && make smoke-test
```
