#!/usr/bin/env bash
# Manually rollback an active deployment.
# Usage: DEPLOY_ID=<id> TOKEN=<jwt> bash scripts/rollback.sh

BASE="${BASE_URL:-http://localhost:8080}"
TOKEN="${TOKEN:?TOKEN env var required}"
DEPLOY_ID="${DEPLOY_ID:?DEPLOY_ID env var required}"

echo "==> Rolling back deployment: $DEPLOY_ID"
curl -sf -X POST "$BASE/deployments/$DEPLOY_ID/rollback" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

echo ""
echo "==> Audit trail for this deployment:"
curl -sf "$BASE/audit?resource_id=$DEPLOY_ID" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool
