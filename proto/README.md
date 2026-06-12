# Protobuf Contracts

Shared message definitions for all trade-eval-platform services. Regenerate Go
code with `./generate.sh` (or `make proto` from the repo root).

## submission.proto

| Message | Purpose | Fields |
|---|---|---|
| `Submission` | Canonical record of an uploaded program | `id`, `contestant_id`, `language` (CPP/RUST/GO/PYTHON), `s3_key`, `submitted_at`, `status` (PENDING/BUILDING/READY/FAILED/OOM_KILLED/TIMEOUT) |
| `BuildJob` | Queued build request (topic: `build-jobs`) | `submission_id`, `s3_key`, `language` |
| `BuildResult` | Build outcome from build-worker | `submission_id`, `status`, `error_log`, `container_ip`, `container_port` |

## test_events.proto

| Message | Purpose | Fields |
|---|---|---|
| `StartTest` | Tells bot-fleet to start a load test | `test_id`, `contestant_id`, `target_ip`, `target_port`, `duration_seconds`, `bot_count`, `bot_personas` |
| `StopTest` | Halts all bots for a test | `test_id`, `reason` |
| `TestHeartbeat` | Orchestrator liveness signal during a test | `test_id`, `timestamp_ns`, `active_bots` |

## telemetry.proto

| Message | Purpose | Fields |
|---|---|---|
| `OrderEvent` | Per-order telemetry from bots (topic: `bot-telemetry`) | identity fields (`contestant_id`, `test_id`, `bot_id`, `bot_persona`, `order_id`), timing (`sent_at_ns`, `acked_at_ns`, `latency_us`), order details (`order_type`, `price`, `quantity`), correctness (`expected_fill`, `actual_fill`, `correct`, `timed_out`, `bot_error`), `sequence_number` |
| `Fill` | Expected or actual execution of an order | `price`, `quantity`, `status` (FILLED/PARTIAL/REJECTED/PENDING) |
| `LatencyWindow` | Aggregated per-window performance | `contestant_id`, `window_start_ns`, `window_end_ns`, `p50_us`, `p90_us`, `p99_us`, `tps`, `correctness_rate` |

## leaderboard.proto

| Message | Purpose | Fields |
|---|---|---|
| `LeaderboardEntry` | One ranked contestant row | `rank`, `contestant_id`, `contestant_name`, `score`, `p50_us`, `p90_us`, `p99_us`, `tps`, `correctness_rate`, `status`, `last_updated_ns` |
| `LeaderboardUpdate` | Full leaderboard snapshot | `timestamp`, `entries` |
