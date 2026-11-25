#!/usr/bin/env bash
# Smoke test: register, create a conversation, stream one message.
# Requires a running stack (make up && make backend) and a provider key for the
# model call. Set SMOKE_SKIP_MODEL=1 to verify auth and conversation creation
# only (used by CI, which has no provider key).
set -euo pipefail
BASE=${BASE:-http://localhost:8080}
EMAIL=${EMAIL:-smoke@test.dev}
SKIP_MODEL=${SMOKE_SKIP_MODEL:-0}
res=$(curl -sf -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"longenough1\"}") \
  || res=$(curl -sf -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"longenough1\"}")  # 409 email taken → login instead
token=$(echo "$res" | python3 -c 'import json,sys; print(json.load(sys.stdin)["accessToken"])')
conv=$(curl -sf -X POST "$BASE/api/conversations" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d '{}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
if [ "$SKIP_MODEL" = "1" ]; then
  echo "smoke ok (model call skipped)"
  exit 0
fi
curl -sf -N -X POST "$BASE/api/conversations/$conv/messages" \
  -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
  -d '{"content":"Reply with exactly: pong"}' | awk 'NR<=40'
echo "smoke ok"
