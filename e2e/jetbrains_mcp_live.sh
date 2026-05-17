#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PLUGIN_DIR="$ROOT/plugins/jetbrains-reflective-mcp"
STATUS_FILE="$HOME/.pragma/jetbrains-mcp/latest.json"
LOG_FILE="${TMPDIR:-/tmp}/pragma-jetbrains-mcp-runIde.log"
LAUNCH_SANDBOX=0
WAIT_SECONDS="${PRAGMA_JETBRAINS_MCP_WAIT_SECONDS:-5}"

case "${1:-}" in
  --launch-sandbox)
    LAUNCH_SANDBOX=1
    ;;
  "" )
    ;;
  * )
    echo "usage: $0 [--launch-sandbox]" >&2
    exit 64
    ;;
esac

if [[ ! -d "$PLUGIN_DIR" ]]; then
  echo "missing plugin directory: $PLUGIN_DIR" >&2
  exit 1
fi

cleanup() {
  if [[ -n "${RUN_IDE_PID:-}" ]] && kill -0 "$RUN_IDE_PID" 2>/dev/null; then
    kill "$RUN_IDE_PID" 2>/dev/null || true
    wait "$RUN_IDE_PID" 2>/dev/null || true
  fi
}
trap cleanup EXIT

if [[ "$LAUNCH_SANDBOX" == "1" ]]; then
  rm -f "$STATUS_FILE"

  (
    cd "$PLUGIN_DIR"
    gradle --no-daemon runIde --args "$ROOT"
  ) >"$LOG_FILE" 2>&1 &
  RUN_IDE_PID=$!

  echo "started runIde pid=$RUN_IDE_PID log=$LOG_FILE"
  WAIT_SECONDS="${PRAGMA_JETBRAINS_MCP_WAIT_SECONDS:-120}"
else
  echo "checking existing JetBrains MCP plugin discovery at $STATUS_FILE"
fi

for _ in $(seq 1 "$WAIT_SECONDS"); do
  if [[ -f "$STATUS_FILE" ]] && grep -q "\"projectPath\": \"$ROOT\"" "$STATUS_FILE"; then
    PRAGMA_JETBRAINS_MCP_LIVE=1 \
    PRAGMA_JETBRAINS_MCP_WORKDIR="$ROOT" \
    go test ./internal/mcp -run TestLiveJetBrainsMCPDiscovery -count=1 -v
    exit 0
  fi
  if [[ "$LAUNCH_SANDBOX" == "1" ]] && ! kill -0 "$RUN_IDE_PID" 2>/dev/null; then
    echo "runIde exited before writing JetBrains MCP discovery" >&2
    tail -120 "$LOG_FILE" >&2 || true
    exit 1
  fi
  sleep 1
done

if [[ "$LAUNCH_SANDBOX" == "1" ]]; then
  echo "timed out waiting for $STATUS_FILE for $ROOT" >&2
  tail -120 "$LOG_FILE" >&2 || true
else
  echo "no live JetBrains MCP plugin discovery for $ROOT" >&2
  echo "Open this workspace in an IDE with the Pragma JetBrains Reflective MCP plugin installed, then rerun:" >&2
  echo "  $0" >&2
  echo "Use --launch-sandbox only when interactive IntelliJ trust/import dialogs are acceptable." >&2
fi
exit 1
