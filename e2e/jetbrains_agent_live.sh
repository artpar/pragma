#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATUS_FILE="$HOME/.pragma/jetbrains-mcp/latest.json"
RECORDING="$ROOT/pragma-recording.jsonl"
BACKUP=""
PROVIDER="${PRAGMA_E2E_PROVIDER:-google}"
MODEL="${PRAGMA_E2E_MODEL:-gemini-2.5-flash}"
TOOL="com.intellij.openapi.application.ApplicationInfo.getInstance"

cleanup() {
  if [[ -n "$BACKUP" && -f "$BACKUP" ]]; then
    mv "$BACKUP" "$RECORDING"
  else
    rm -f "$RECORDING"
  fi
}
trap cleanup EXIT

if [[ ! -f "$STATUS_FILE" ]] || ! grep -q "\"projectPath\": \"$ROOT\"" "$STATUS_FILE"; then
  echo "no live JetBrains MCP plugin discovery for $ROOT" >&2
  echo "Open this workspace in a JetBrains IDE with the Pragma plugin installed." >&2
  exit 1
fi

if [[ -f "$RECORDING" ]]; then
  BACKUP="$(mktemp "${TMPDIR:-/tmp}/pragma-recording.XXXXXX")"
  mv "$RECORDING" "$BACKUP"
fi

(
  cd "$ROOT"
  go run ./cmd/pragma \
    --provider "$PROVIDER" \
    --model "$MODEL" \
    --context-mode chat \
    --max-tokens 1024 \
    --temperature 0 \
    --stop-after-tool-exec \
    --allowed-tools "$TOOL" \
    --record \
    -p "Call the available JetBrains MCP tool $TOOL with empty JSON arguments, then stop. Do not answer from memory."
)

python3 - "$RECORDING" "$TOOL" "$ROOT" <<'PY'
import json
import sys

recording, tool, root = sys.argv[1:]
events = []
with open(recording, "r", encoding="utf-8") as f:
    for line in f:
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass

api_tool_call = None
for event in events:
    if event.get("kind") != "APIRequestCompleted":
        continue
    for part in event.get("content", []):
        data = part.get("data") or {}
        if part.get("type") == "tool_call" and data.get("name") == tool and data.get("input") == {}:
            api_tool_call = data
            break
    if api_tool_call:
        break

if not api_tool_call:
    raise SystemExit(f"model did not request expected tool call: {tool}")

mcp_completed = None
for event in events:
    if event.get("kind") == "MCPToolCallCompleted" and event.get("tool_name") == tool:
        mcp_completed = event
        break

if not mcp_completed:
    raise SystemExit(f"MCP tool did not complete: {tool}")

tool_completed = None
for event in events:
    if event.get("kind") == "ToolExecutionCompleted" and event.get("tool_name") == tool and not event.get("is_error"):
        tool_completed = event
        break

if not tool_completed:
    raise SystemExit(f"Pragma tool execution did not complete cleanly: {tool}")

output = tool_completed.get("output", "")
if root not in output or "WebStorm" not in output:
    raise SystemExit(f"unexpected tool output: {output}")

print("agent E2E passed")
print(f"tool_call_id={api_tool_call.get('id')}")
print(f"server={mcp_completed.get('server_name')}")
print(f"duration_ms={tool_completed.get('duration_ms')}")
PY
