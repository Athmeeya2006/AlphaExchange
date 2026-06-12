# Reference Order Book

A correct (price-time priority) but intentionally simple HTTP order book used as:
1. the platform's end-to-end test binary, and
2. the behavioural reference the telemetry ingester validates against.

Self-contained — builds with just `g++ -O2 -std=c++17 -o orderbook main.cpp -lpthread`
(no external headers), so it compiles inside the contestant sandbox unchanged.

## Run
    make run        # listens on :8080

## API
See `docs/contestant-api.md`. Endpoints: POST /order, POST /cancel, GET /health,
POST /reset.

## Known limitations
Single global mutex (not fast), no persistence, minimal JSON parsing tuned to the
fixed request shapes. This is a *reference*, not a competitive entry.
