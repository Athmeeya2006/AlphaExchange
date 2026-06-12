# Normal vs Smart: Key Engineering Decisions

The decisions below are the ones that separate a platform that survives contest day
from one that silently corrupts results. Each is implemented in this codebase.

## 1. Latency percentiles — HDR histogram, not sort

A naive `sort(samples)[0.99*n]` is O(n log n) per window and keeps every sample in
memory. At 1M events/sec it falls behind and eventually OOMs. The HDR histogram
(`telemetry-ingester/latency/hdr_histogram.go`) records in O(1) into fixed buckets
and scans once for a percentile — fixed memory, ~10x faster.

## 2. Kafka offsets — commit after the write, not before

Auto-commit marks events consumed before they're persisted; a crash then loses them
silently, corrupting scores. The build-worker commits offsets only after
`ProcessBuild` returns (at-least-once delivery).

## 3. Event ordering — reorder buffer + sequence numbers

Kafka arrival order ≠ true send order across distributed bots. Without correction
the shadow book produces false correctness failures. The reorder buffer
(`telemetry-ingester/reorder_buffer.go`) holds events briefly and sorts by the
bot-assigned sequence number before the authoritative book replays them.

## 4. WebSocket fan-out — drop slow clients, never block

One slow client must not stall the broadcast for 10,000 others. The hub
(`leaderboard-api/hub/websocket_hub.go`) does a non-blocking `select` send and
disconnects clients whose buffer is full.

## 5. Horizontal WS scale — Redis pub/sub

The scorer publishes to `leaderboard:updates`; every pod's hub subscribes and
broadcasts. The scorer's own pod does **not** also broadcast directly, so there's no
double-delivery. Add pods freely behind a load balancer.

## 6. TimescaleDB writes — COPY, not INSERT

Bulk `COPY` (`storage/timescale_writer.go`) sustains the row rate that batched
INSERT cannot. (Because COPY can't do `ON CONFLICT`, de-duplication of at-least-once
replays is handled at query time rather than via a unique constraint.)

## 7. Orchestrator crash recovery

Each running test writes a heartbeat. A second orchestrator detects stale heartbeats
(`crash_recovery.go`), checks container health, and either re-registers the stop
timer or fails the test cleanly — turning a pod restart into a ~60s blip instead of
a permanently stuck test.

## 8. Bot HTTP — one shared, warm connection pool

A new `http.Client` per bot exhausts file descriptors and adds handshake latency to
every measurement. All bots targeting one container share a tuned transport
(`bot-fleet/bots/http_client.go`).

## 9. Circuit breaker with a minimum-traffic guard

The breaker only trips after ≥20 requests, so a container that's slow to warm up
doesn't trip it before the test really starts (`client/circuit_breaker.go`).
