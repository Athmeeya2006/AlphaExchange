#!/usr/bin/env bash
# End-to-end integration test against a running full stack. Requires curl + jq.
set -euo pipefail
BASE_URL=${BASE_URL:-http://localhost:8080}
API_KEY=${API_KEY:-key-alice-0001}

echo "== creating submission =="
RESULT=$(curl -sf -X POST "$BASE_URL/v1/submissions" \
  -H "X-API-Key: $API_KEY" \
  -F "file=@testdata/sample-orderbook.zip" \
  -F "language=cpp")
SUB_ID=$(echo "$RESULT" | jq -r .submission_id)
echo "submission: $SUB_ID"

echo "== waiting for READY (120s) =="
for _ in $(seq 1 60); do
  STATUS=$(curl -sf "$BASE_URL/v1/submissions/$SUB_ID" -H "X-API-Key: $API_KEY" | jq -r .status)
  echo "  $STATUS"
  [ "$STATUS" = "ready" ] && break
  [ "$STATUS" = "failed" ] && { echo "build failed"; exit 1; }
  sleep 2
done

echo "== triggering test =="
TEST_ID=$(curl -sf -X POST "$BASE_URL/v1/tests" \
  -H "X-API-Key: $API_KEY" -H "Content-Type: application/json" \
  -d "{\"submission_id\":\"$SUB_ID\",\"duration_seconds\":30,\"bot_count\":10}" | jq -r .test_id)
echo "test: $TEST_ID"

echo "== waiting for completion (120s) =="
for _ in $(seq 1 60); do
  STATUS=$(curl -sf "$BASE_URL/v1/tests/$TEST_ID" -H "X-API-Key: $API_KEY" | jq -r .test.status)
  echo "  $STATUS"
  [ "$STATUS" = "completed" ] && break
  [ "$STATUS" = "failed" ] && { echo "test failed"; exit 1; }
  sleep 2
done

echo "== leaderboard =="
LB=$(curl -sf "$BASE_URL/v1/leaderboard")
echo "$LB" | jq .
COUNT=$(echo "$LB" | jq '.entries | length')
[ "$COUNT" -ge 1 ] || { echo "no leaderboard entries"; exit 1; }
SCORE=$(echo "$LB" | jq '.entries[0].score')
echo "top score: $SCORE"
echo "INTEGRATION TEST PASSED"
