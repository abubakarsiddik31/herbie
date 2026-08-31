#!/usr/bin/env bash
# Smoke test: register, create a conversation, stream one message.
# Requires a running stack (make up && make backend) and a GEMINI_API_KEY for the model call.
set -euo pipefail
BASE=${BASE:-http://localhost:8080}
EMAIL=${EMAIL:-smoke@test.dev}
res=$(curl -sf -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"longenough1\"}") \
  || res=$(curl -sf -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"longenough1\"}")  # 409 email taken → login instead
token=$(echo "$res" | python3 -c 'import json,sys; print(json.load(sys.stdin)["accessToken"])')
conv=$(curl -sf -X POST "$BASE/api/conversations" -H "Authorization: Bearer $token" -H 'Content-Type: application/json' -d '{}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["id"])')
curl -sf -N -X POST "$BASE/api/conversations/$conv/messages" \
  -H "Authorization: Bearer $token" -H 'Content-Type: application/json' \
  -d '{"content":"Reply with exactly: pong"}' | awk 'NR<=40'
echo "smoke ok"
