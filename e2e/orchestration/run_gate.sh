#!/usr/bin/env bash
# ORCH capability gate — real-boot persona orchestration against the
# scripted local provider (no inference spend). Boots the actual
# `pragma orchestration run` path: engine, pragma loop, LLM resolver,
# FSM runner, artifact-verdict control — architect → implementer →
# prosecutor → APPROVE → done, with bash blocks executed for real.
#
# Usage: run_gate.sh <label>
# Requires ./bin/pragma built at the revision under test.
set -euo pipefail

LABEL="${1:?label required}"
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
OUT="$HERE/results/$LABEL"
PROBE="$OUT/provider"

rm -rf "$OUT"
mkdir -p "$OUT/raw" "$OUT/provider"

python3 "$ROOT/e2e/mcp-injection/probe_provider.py" "$PROBE" orchestration 5 > "$OUT/provider.log" 2>&1 &
PROV_PID=$!
trap 'kill "$PROV_PID" 2>/dev/null || true' EXIT

for _ in $(seq 1 100); do
    [ -f "$PROBE/port" ] && break
    sleep 0.1
done
[ -f "$PROBE/port" ] || { echo "probe provider failed to start"; cat "$OUT/provider.log"; exit 1; }
PORT="$(cat "$PROBE/port")"
echo "probe provider on 127.0.0.1:$PORT (profile=orchestration)"

mkdir -p /tmp/pragma
rm -f /tmp/pragma/orch-gate.txt /tmp/pragma/architect-brief.md \
    /tmp/pragma/implementer-report.md /tmp/pragma/prosecutor-verdict.md

STATUS_FILE="$OUT/exit_code"
set +e
env \
  PRAGMA_RAW_HTTP_CAPTURE_DIR="$OUT/raw" \
  OPENAI_BASE_URL="http://127.0.0.1:$PORT/v1" \
"$ROOT/bin/pragma" \
  orchestration run "$ROOT/orchestrations/architect-implementer-prosecutor.yaml" \
  --persona-dir "$ROOT/personas" \
  --prompt "Create the file /tmp/pragma/orch-gate.txt containing exactly gate-ok" \
  --provider openai --model gpt-4o --api-key probe-local-key \
  > "$OUT/stdout.txt" 2> "$OUT/stderr.txt"
STATUS=$?
set -e
echo "$STATUS" > "$STATUS_FILE"

kill "$PROV_PID" 2>/dev/null || true
trap - EXIT

echo "exit code: $STATUS"

FAIL=0
check() {
    if eval "$2"; then
        echo "PASS: $1"
    else
        echo "FAIL: $1"
        FAIL=1
    fi
}

check "orchestration exited 0" "[ '$STATUS' = 0 ]"
check "task deliverable created with exact content" \
    "[ -f /tmp/pragma/orch-gate.txt ] && [ \"\$(cat /tmp/pragma/orch-gate.txt)\" = 'gate-ok' ]"
check "prosecutor verdict written with APPROVE" \
    "grep -q 'APPROVE' /tmp/pragma/prosecutor-verdict.md"
check "architect brief written" "[ -s /tmp/pragma/architect-brief.md ]"
check "implementer report written" "[ -s /tmp/pragma/implementer-report.md ]"
check "three persona requests hit the wire" \
    "[ \"\$(grep -c 'You are the' '$PROBE/requests.jsonl' 2>/dev/null || true)\" -ge 3 ]"
check "raw wire captures recorded" "[ -n \"\$(ls '$OUT/raw' 2>/dev/null)\" ]"
check "FSM reached the terminal state (stdout trace)" \
    "grep -qi 'done\|orchestration.*complet' '$OUT/stdout.txt' || grep -qi 'done\|orchestration.*complet' '$OUT/stderr.txt'"

echo "--- stdout tail ---"
tail -20 "$OUT/stdout.txt"
echo "--- stderr tail ---"
tail -20 "$OUT/stderr.txt"

exit "$FAIL"
