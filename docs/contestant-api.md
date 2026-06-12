# Contestant Order Book API

Your submission must run an HTTP server on port **8080** implementing the
following endpoints. The bot fleet calls these; the telemetry ingester validates
your fills against a reference shadow order book.

## POST /order

Request:
```json
{ "order_id": "ord_xxx", "type": "LIMIT_BUY|LIMIT_SELL|MARKET_BUY|MARKET_SELL", "price": 100.50, "quantity": 10 }
```
(Market orders omit `price`.)

Response:
```json
{ "order_id": "ord_xxx", "status": "FILLED|PARTIAL|PENDING|REJECTED",
  "filled_price": 100.50, "filled_quantity": 10, "remaining_quantity": 0 }
```

## POST /cancel

Request: `{ "order_id": "ord_xxx" }`

Response: `{ "order_id": "ord_xxx", "status": "CANCELLED|NOT_FOUND|ALREADY_FILLED" }`

## GET /health

Must return HTTP 200 with `{ "status": "ok" }` within 3 seconds of startup.
The build worker waits up to 30s for this before marking the submission ready.

## GET /orderbook (optional, for debugging)

`{ "bids": [ {"price": 100.0, "quantity": 10} ], "asks": [ ... ] }`

## POST /reset (test mode only)

Clears all state. Used by the local correctness suite between tests.

## Semantics

- **Price-time priority**: at a given price level, earlier orders fill first.
- **Market orders** consume the best available opposite-side liquidity.
- **Limit orders** that cross fill immediately; the remainder rests in the book.
- Fill prices for limit orders must match the resting price exactly.

## Timing SLA

The bot fleet times out after 5s. Latency is measured as the HTTP round trip.
Lower p99 latency scores higher (see `docs/scoring-formula.md`).

### Minimal C++ example (cpp-httplib)

```cpp
#include "httplib.h"
int main() {
  httplib::Server s;
  s.Get("/health", [](const httplib::Request&, httplib::Response& r){
    r.set_content("{\"status\":\"ok\"}", "application/json");
  });
  // implement /order and /cancel ...
  s.listen("0.0.0.0", 8080);
}
```
