#!/usr/bin/env bash
# Full seed: creates an admin user, logs in, registers 10 servers.
# Usage: bash scripts/seed.sh

BASE="${BASE_URL:-http://localhost:8080}"

echo "==> Creating admin user..."
curl -sf -X POST "$BASE/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"name":"Admin","email":"admin@canopy.dev","password":"canopy123","role":"admin"}' \
  | python3 -m json.tool || echo "(may already exist)"

echo ""
echo "==> Logging in..."
TOKEN=$(curl -sf -X POST "$BASE/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"email":"admin@canopy.dev","password":"canopy123"}' \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['token'])")

if [ -z "$TOKEN" ]; then
  echo "Login failed. Is the server running?"
  exit 1
fi
echo "Token acquired."

echo ""
echo "==> Registering 10 servers..."
for i in $(seq 1 10); do
  REGION="us-east-1"
  if [ $i -gt 7 ]; then REGION="eu-west-1"; fi

  curl -sf -X POST "$BASE/servers" \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer $TOKEN" \
    -d "{
      \"name\": \"server-$(printf '%02d' $i)\",
      \"host\": \"10.0.0.$i\",
      \"region\": \"$REGION\",
      \"tags\": [\"app\",\"$REGION\"],
      \"version\": \"v1.0\"
    }" | python3 -m json.tool
  echo ""
done

echo ""
echo "==> Registering a Slack webhook..."
curl -sf -X POST "$BASE/webhooks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{
    "name": "slack-alerts",
    "url": "https://hooks.slack.com/services/YOUR/WEBHOOK/URL",
    "secret": "my-hmac-secret",
    "events": ["deployment.started","deployment.rolled_back","server.offline"]
  }' | python3 -m json.tool

echo ""
echo "Done. Ready to deploy:"
echo "  export TOKEN=$TOKEN"
echo "  bash scripts/deploy.sh"
