#!/usr/bin/env bash
# Start a canary deployment and then simulate healthy metrics so the
# watcher auto-promotes it after the monitor window.
#
# Usage:
#   export TOKEN=<your jwt>
#   bash scripts/deploy.sh

BASE="${BASE_URL:-http://localhost:8080}"
TOKEN="${TOKEN:?TOKEN env var required. Run seed.sh first.}"
VERSION="${VERSION:-v2.0}"
PREV="${PREV:-v1.0}"

echo "==> Starting canary: $PREV -> $VERSION"
DEPLOY=$(curl -sf -X POST "$BASE/deployments" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{
    \"name\": \"release-$VERSION\",
    \"version\": \"$VERSION\",
    \"prev_version\": \"$PREV\",
    \"canary_percent\": 10,
    \"monitor_seconds\": 60,
    \"max_error_rate\": 0.05,
    \"max_latency_ms\": 500,
    \"notes\": \"Scripted canary release\"
  }")

echo "$DEPLOY" | python3 -m json.tool
DEPLOY_ID=$(echo "$DEPLOY" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")

if [ -z "$DEPLOY_ID" ]; then
  echo "Failed to start deployment."
  exit 1
fi

echo ""
echo "==> Deployment ID: $DEPLOY_ID"
echo "==> Sending healthy metrics for 70 seconds (watcher fires at 60s)..."

for i in $(seq 1 14); do
  curl -sf -X POST "$BASE/metrics" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "{
      \"server_id\": \"000000000000000000000001\",
      \"deployment_id\": \"$DEPLOY_ID\",
      \"version\": \"$VERSION\",
      \"error_rate\": 0.01,
      \"latency_ms\": 180,
      \"request_count\": 5000,
      \"crash_count\": 0,
      \"mem_usage_mb\": 256,
      \"cpu_percent\": 12.5
    }" > /dev/null
  echo -n "."
  sleep 5
done

echo ""
echo ""
echo "==> Health report:"
curl -sf "$BASE/metrics/deployment/$DEPLOY_ID/report" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

echo ""
echo "Watcher will auto-promote within the next 30s."
echo "Or promote manually: curl -X POST $BASE/deployments/$DEPLOY_ID/promote -H 'Authorization: Bearer \$TOKEN'"
