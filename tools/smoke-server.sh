#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"
TOKEN="${TOKEN:-}"

curl_with_auth() {
  if [[ -n "$TOKEN" ]]; then
    curl -H "Authorization: Bearer $TOKEN" "$@"
    return
  fi
  curl "$@"
}

SESSION=$(curl_with_auth -fsS -X POST "${BASE_URL}/v1/sessions" | jq -r .session_id)
echo "session=${SESSION}"

curl_with_auth -fsS -N "${BASE_URL}/v1/sessions/${SESSION}/events" >/tmp/nandocodego-server-events.log &
SSE_PID=$!
sleep 1

curl_with_auth -fsS -X POST "${BASE_URL}/v1/sessions/${SESSION}/messages" \
  -H 'Content-Type: application/json' \
  -d '{"prompt":"Respond with exactly: ok"}' >/tmp/nandocodego-server-post.json

sleep 3
kill "$SSE_PID" 2>/dev/null || true

if ! grep -q 'assistant_text_delta' /tmp/nandocodego-server-events.log; then
  echo "missing assistant_text_delta" >&2
  exit 1
fi

echo "smoke ok"
