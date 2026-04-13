#!/usr/bin/env bash
# Quick system status — fleet health + active deployments.
# Usage: TOKEN=<jwt> bash scripts/status.sh

BASE="${BASE_URL:-http://localhost:8080}"
TOKEN="${TOKEN:?TOKEN env var required. Run seed.sh first.}"

echo "==> Canopy System Status"
echo ""

echo "--- Health ---"
curl -sf "$BASE/health" | python3 -m json.tool

echo ""
echo "--- Fleet Summary ---"
curl -sf "$BASE/status" \
  -H "Authorization: Bearer $TOKEN" | python3 -m json.tool

echo ""
echo "--- All Servers ---"
curl -sf "$BASE/servers" \
  -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
servers = json.load(sys.stdin)
print(f'Total: {len(servers)}')
for s in servers:
    canary = ' [CANARY]' if s.get('is_canary') else ''
    print(f\"  {s['name']:20} {s['status']:12} {s['current_version']:8} {s.get('region',''):15}{canary}\")
"

echo ""
echo "--- Recent Deployments ---"
curl -sf "$BASE/deployments?limit=5" \
  -H "Authorization: Bearer $TOKEN" | python3 -c "
import sys, json
data = json.load(sys.stdin)
deploys = data.get('deployments', [])
print(f'Showing {len(deploys)} most recent:')
for d in deploys:
    print(f\"  {d['id'][:8]}  {d['status']:15} {d['version']:10} by {d.get('created_by_name','?')}\")
"
