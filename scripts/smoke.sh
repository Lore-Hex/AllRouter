#!/usr/bin/env bash
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TMPDIR="$(mktemp -d)"
BIN="$TMPDIR/allrouter"
LOG="$TMPDIR/allrouter.log"
PID=""
FAILURES=0

cleanup() {
  if [[ -n "$PID" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    wait "$PID" 2>/dev/null || true
  fi
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

pass() {
  printf 'PASS %s\n' "$1"
}

fail() {
  printf 'FAIL %s\n' "$1"
  FAILURES=$((FAILURES + 1))
}

finish() {
  if [[ "$FAILURES" -ne 0 ]]; then
    if [[ -s "$LOG" ]]; then
      printf '\n--- allrouter log ---\n'
      cat "$LOG"
    fi
    exit 1
  fi
}

ALLROUTER_LOCAL_URL="${ALLROUTER_LOCAL_URL:-http://127.0.0.1:11434}"
ALLROUTER_LISTEN="${ALLROUTER_LISTEN:-127.0.0.1:8383}"
if [[ -n "${ALLROUTER_HOST:-}" ]]; then
  HOST="$ALLROUTER_HOST"
elif [[ "$ALLROUTER_LISTEN" == :* ]]; then
  HOST="http://127.0.0.1$ALLROUTER_LISTEN"
else
  HOST="http://$ALLROUTER_LISTEN"
fi

if (cd "$ROOT" && go build -o "$BIN" ./cmd/allrouter); then
  pass "build"
else
  fail "build"
  finish
fi

"$BIN" \
  -listen "$ALLROUTER_LISTEN" \
  -local-url "$ALLROUTER_LOCAL_URL" \
  -tr-api-key "" \
  >"$LOG" 2>&1 &
PID="$!"

READY=0
for _ in $(seq 1 80); do
  if curl -fsS "$HOST/healthz" >/dev/null 2>&1; then
    READY=1
    break
  fi
  if ! kill -0 "$PID" 2>/dev/null; then
    break
  fi
  sleep 0.1
done

if [[ "$READY" -eq 1 ]]; then
  pass "healthz"
else
  fail "healthz"
  finish
fi

MODELS_JSON="$TMPDIR/models.json"
if curl -fsS "$HOST/v1/models" -o "$MODELS_JSON"; then
  pass "models"
else
  fail "models"
  finish
fi

if [[ -n "${ALLROUTER_MODEL:-}" ]]; then
  MODEL="$ALLROUTER_MODEL"
else
  MODEL="$(grep -Eo '"id"[[:space:]]*:[[:space:]]*"[^"]+"' "$MODELS_JSON" | sed -E 's/.*"([^"]+)".*/\1/' | grep '^local/' | head -n 1)"
  if [[ -z "$MODEL" ]]; then
    FIRST_MODEL="$(grep -Eo '"id"[[:space:]]*:[[:space:]]*"[^"]+"' "$MODELS_JSON" | sed -E 's/.*"([^"]+)".*/\1/' | head -n 1)"
    if [[ -n "$FIRST_MODEL" ]]; then
      MODEL="local/$FIRST_MODEL"
    fi
  fi
fi

if [[ -z "$MODEL" ]]; then
  fail "select model"
  finish
fi
pass "select model $MODEL"

CHAT_HEADERS="$TMPDIR/chat.headers"
CHAT_BODY="$TMPDIR/chat.json"
CHAT_PAYLOAD='{"model":"'"$MODEL"'","max_tokens":8,"messages":[{"role":"user","content":"Reply with one short word."}]}'
if curl -fsS -D "$CHAT_HEADERS" -o "$CHAT_BODY" \
  "$HOST/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d "$CHAT_PAYLOAD" &&
  grep -qi '^X-AllRouter-Route: local' "$CHAT_HEADERS" &&
  [[ -s "$CHAT_BODY" ]]; then
  pass "local chat"
else
  fail "local chat"
fi

STREAM_HEADERS="$TMPDIR/stream.headers"
STREAM_BODY="$TMPDIR/stream.txt"
STREAM_PAYLOAD='{"model":"'"$MODEL"'","max_tokens":8,"stream":true,"messages":[{"role":"user","content":"Reply with one short word."}]}'
if curl -fsS -N --max-time 90 -D "$STREAM_HEADERS" -o "$STREAM_BODY" \
  "$HOST/v1/chat/completions" \
  -H "Content-Type: application/json" \
  -d "$STREAM_PAYLOAD" &&
  grep -qi '^X-AllRouter-Route: local' "$STREAM_HEADERS" &&
  grep -q '^data:' "$STREAM_BODY" &&
  grep -q '\[DONE\]' "$STREAM_BODY"; then
  pass "local stream"
else
  fail "local stream"
fi

finish
