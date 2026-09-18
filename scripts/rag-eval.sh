#!/bin/bash
# Golden-query RAG eval: upload 3 fixture docs, ask 3 questions, assert the
# top-cited source document AND a matching [n] citation in the answer.
# Usage: BASE=http://localhost:8080 ./scripts/rag-eval.sh
set -euo pipefail
BASE="${BASE:-http://localhost:8080}"
EMAIL="${EMAIL:-eval@test.dev}"
PASSWORD="${PASSWORD:-evalpassword1}"
EVDIR="$(dirname "$0")/../backend/internal/rag/testdata/eval"

# Auth block from scripts/smoke.sh (register-or-login → TOKEN).
res=$(curl -sf -X POST "$BASE/api/auth/register" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}") \
  || res=$(curl -sf -X POST "$BASE/api/auth/login" -H 'Content-Type: application/json' \
  -d "{\"email\":\"$EMAIL\",\"password\":\"$PASSWORD\"}")  # 409 email taken → login instead
TOKEN=$(echo "$res" | python3 -c 'import json,sys; print(json.load(sys.stdin)["accessToken"])')

# Idempotency: drop fixture docs from earlier runs so each eval starts from
# exactly one copy per file (plain re-uploads duplicate the corpus under new
# ids and crowd the rankings).
for id in $(curl -s -X GET "$BASE/api/documents" -H "Authorization: Bearer $TOKEN" \
  | jq -r '.documents[] | select(.filename == "cats.md" or .filename == "dogs.md" or .filename == "birds.md") | .id' || true); do
  curl -s -X DELETE "$BASE/api/documents/$id" -H "Authorization: Bearer $TOKEN" > /dev/null
done

for f in cats dogs birds; do
  curl -s -X POST "$BASE/api/documents" -H "Authorization: Bearer $TOKEN" \
    -F "file=@$EVDIR/$f.md" | jq -e '.status == "ready"' > /dev/null
done

CONV="$(curl -s -X POST "$BASE/api/conversations" -H "Authorization: Bearer $TOKEN" \
  -H 'Content-Type: application/json' -d '{"title":"eval"}' | jq -r '.id')"

# Send-message block from scripts/smoke.sh (conversation message-POST +
# SSE-collect), wired to $1 (conversation id) and $2 (query).
send_message() {
  curl -sf -N -X POST "$BASE/api/conversations/$1/messages" \
    -H "Authorization: Bearer $TOKEN" -H 'Content-Type: application/json' \
    -d "{\"content\":\"$2\"}"
}

check() { # $1 query, $2 expected doc
  OUT="$(send_message "$CONV" "$1" || true)"
  FIRST="$(echo "$OUT" | grep '^data:' | grep -o '"title":"[^"]*"' | head -1)"
  echo "query=$1 first_source=$FIRST"
  echo "$FIRST" | grep -q "$2" || { echo "FAIL: top source, want $2"; exit 1; }
  echo "$OUT" | grep -o '\[[0-9][0-9, ]*\]' | head -1 | grep -q . || { echo "FAIL: answer carries no bracket citation"; exit 1; }
}
check "What is the canary word for cats?" "cats.md"
check "What is the canary word for dogs?" "dogs.md"
check "What is the canary word for birds?" "birds.md"
echo "RAG eval PASS (3/3 top-cited, all cited)"
