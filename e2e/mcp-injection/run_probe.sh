#!/usr/bin/env bash
# Boot one real pragma session against the scripted local provider and capture
# the raw HTTP wire. Usage:
#   run_probe.sh <label> <profile: provider-tools|pragma|websearch|subagent|turnbudget> [delay_seconds] [live]
# The websearch profile also starts the local Brave stub and points
# BRAVE_SEARCH_BASE_URL at it (WEB-001 hermetic gate). Pass "live" as the
# 4th arg to skip the stub and hit the real Brave endpoint (WEB-001 live
# contract gate; budget: 1 search call).
# Requires ./bin/pragma to be built at the revision under test.
set -euo pipefail

LABEL="${1:?label required}"
MODE="${2:?loop mode required (provider-tools|pragma)}"
DELAY="${3:-15}"
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
OUT="$HERE/results/$LABEL"
PROBE="$OUT/provider"
BRAVE="$OUT/brave"

rm -rf "$OUT"
mkdir -p "$OUT/raw"

PROFILE="$MODE"
LOOP="$PROFILE"
case "$PROFILE" in
  websearch|subagent|turnbudget) LOOP="provider-tools" ;;
esac
python3 "$HERE/probe_provider.py" "$PROBE" "$PROFILE" "$DELAY" > "$OUT/provider.log" 2>&1 &
PROV_PID=$!
PIDS="$PROV_PID"
LIVE="${4:-}"
if [ "$PROFILE" = "websearch" ] && [ "$LIVE" != "live" ]; then
  python3 "$HERE/brave_stub.py" "$BRAVE" > "$OUT/brave.log" 2>&1 &
  BRAVE_PID=$!
  PIDS="$PROV_PID $BRAVE_PID"
fi
trap 'for p in $PIDS; do kill "$p" 2>/dev/null || true; done' EXIT

for _ in $(seq 1 100); do
  [ -f "$PROBE/port" ] && break
  sleep 0.1
done
[ -f "$PROBE/port" ] || { echo "provider failed to start"; cat "$OUT/provider.log"; exit 1; }
PORT="$(cat "$PROBE/port")"
echo "probe provider on 127.0.0.1:$PORT (profile=$PROFILE delay=${DELAY}s)"

EXTRA_ENV=()
EXTRA_ARGS=()
if [ "$PROFILE" = "websearch" ] && [ "$LIVE" != "live" ]; then
  for _ in $(seq 1 100); do
    [ -f "$BRAVE/port" ] && break
    sleep 0.1
  done
  [ -f "$BRAVE/port" ] || { echo "brave stub failed to start"; cat "$OUT/brave.log"; exit 1; }
  BRAVE_PORT="$(cat "$BRAVE/port")"
  echo "brave stub on 127.0.0.1:$BRAVE_PORT"
  EXTRA_ENV=("BRAVE_SEARCH_BASE_URL=http://127.0.0.1:$BRAVE_PORT")
fi
if [ "$PROFILE" = "turnbudget" ]; then
  EXTRA_ARGS=(--max-turns 8)
fi

PROMPT="MCP injection boot probe: call the tools the scripted provider offers, then finish with the final text."

env \
  PRAGMA_RAW_HTTP_CAPTURE_DIR="$OUT/raw" \
  OPENAI_BASE_URL="http://127.0.0.1:$PORT/v1" \
  "${EXTRA_ENV[@]}" \
"$ROOT/bin/pragma" \
  -p "$PROMPT" \
  --provider openai --model gpt-4o --api-key probe-local-key \
  --loop "$LOOP" \
  "${EXTRA_ARGS[@]}" \
  --permission-mode bypassPermissions \
  > "$OUT/stdout.txt" 2> "$OUT/stderr.txt" || true

for p in $PIDS; do kill "$p" 2>/dev/null || true; done
trap - EXIT

echo "--- stderr (loop trace) ---"
cat "$OUT/stderr.txt"
echo "--- stdout ---"
cat "$OUT/stdout.txt"
echo "--- raw captures ---"
ls "$OUT/raw"
