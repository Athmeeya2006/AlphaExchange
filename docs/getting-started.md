# Getting Started (Contestants)

You implement an HTTP order book; the platform load-tests and scores it.

## 1. Generate a starter

```bash
make template LANG=cpp     # or rust | go | python -> ./contestant-starter/
```

## 2. Implement the API

Your server must listen on **:8080** and implement (see `docs/contestant-api.md`):

- `POST /order` — match/rest an order, return its fill
- `POST /cancel` — cancel a resting order
- `GET /health` — return 200 within 3s of startup

Price-time priority: at each price level, earlier orders fill first. Limit fills
must report the resting price exactly; market orders take best available liquidity.

## 3. Test locally

```bash
ORDER_BOOK_URL=http://localhost:8080 bash tests/correctness/run.sh
```

A correct reference implementation lives in `testdata/sample-orderbook/` — compare
against it.

## 4. Submit

```bash
curl -X POST http://localhost:8080/v1/submissions \
  -H "X-API-Key: $YOUR_KEY" \
  -F "file=@my-orderbook.zip" -F "language=cpp"
```

Poll the returned `submission_id` until `status: ready`, then start a test from the
web UI or `POST /v1/tests`. Watch your rank on the live leaderboard.

## 5. Optimize

You're scored 40% throughput, 40% (inverse) p99 latency, 20% correctness. See
`docs/scoring-formula.md`. Common wins: a shared connection-friendly server, lock
contention reduction, and avoiding per-order allocations.
