#!/usr/bin/env bash
# Boot one real pragma session against the scripted local provider and capture
# the raw HTTP wire. Usage:
#   run_probe.sh <label> <loop-mode: provider-tools|pragma> [delay_seconds]
# Requires ./bin/pragma to be built at the revision under test.
set -euo pipefail

LABEL="${1:?label required}"
MODE="${2:?loop mode required (provider-tools|pragma)}"
DELAY="${3:-15}"
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
OUT="$HERE/results/$LABEL"
PROBE="$OUT/provider"

rm -rf "$OUT"
mkdir -p "$OUT/raw"

PROFILE="$MODE"
python3 "$HERE/probe_provider.py" "$PROBE" "$PROFILE" "$DELAY" > "$OUT/provider.log" 2>&1 &
PROV_PID=$!
trap 'kill "$PROV_PID" 2>/dev/null || true' EXIT

for _ in $(seq 1 100); do
  [ -f "$PROBE/port" ] && break
  sleep 0.1
done
[ -f "$PROBE/port" ] || { echo "provider failed to start"; cat "$OUT/provider.log"; exit 1; }
PORT="$(cat "$PROBE/port")"
echo "probe provider on 127.0.0.1:$PORT (profile=$PROFILE delay=${DELAY}s)"

PROMPT="MCP injection boot probe: call the tools the scripted provider offers, then finish with the final text."

PRAGMA_RAW_HTTP_CAPTURE_DIR="$OUT/raw" \
OPENAI_BASE_URL="http://127.0.0.1:$PORT/v1" \
"$ROOT/bin/pragma" \
  -p "$PROMPT" \
  --provider openai --model gpt-4o --api-key probe-local-key \
  --loop "$MODE" \
  --permission-mode bypassPermissions \
  > "$OUT/stdout.txt" 2> "$OUT/stderr.txt" || true

kill "$PROV_PID" 2>/dev/null || true
trap - EXIT

echo "--- stderr (loop trace) ---"
cat "$OUT/stderr.txt"
echo "--- stdout ---"
cat "$OUT/stdout.txt"
echo "--- raw captures ---"
ls "$OUT/raw"
