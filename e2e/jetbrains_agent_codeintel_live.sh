#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
STATUS_FILE="$HOME/.pragma/jetbrains-mcp/latest.json"
RECORDING="$ROOT/pragma-recording.jsonl"
PROVIDER="${PRAGMA_E2E_PROVIDER:-google}"
MODEL="${PRAGMA_E2E_MODEL:-gemini-2.5-flash}"
TOOL="com.intellij.psi.search.PsiSearchHelper.processElementsWithWord"
BACKUP=""
OUT=""

cleanup() {
  if [[ -n "$BACKUP" && -f "$BACKUP" ]]; then
    mv "$BACKUP" "$RECORDING"
  else
    rm -f "$RECORDING"
  fi
  if [[ -n "$OUT" ]]; then
    rm -f "$OUT"
  fi
}
trap cleanup EXIT

if [[ ! -f "$STATUS_FILE" ]] || ! grep -q "\"projectPath\": \"$ROOT\"" "$STATUS_FILE"; then
  echo "no live JetBrains MCP plugin discovery for $ROOT" >&2
  exit 1
fi

if ! grep -q '"toolCount": 11' "$STATUS_FILE"; then
  echo "JetBrains MCP discovery is stale; expected plugin with 11 tools" >&2
  cat "$STATUS_FILE" >&2
  exit 1
fi

if [[ -f "$RECORDING" ]]; then
  BACKUP="$(mktemp "${TMPDIR:-/tmp}/pragma-recording.XXXXXX")"
  mv "$RECORDING" "$BACKUP"
fi
OUT="$(mktemp "${TMPDIR:-/tmp}/pragma-jetbrains-codeintel.XXXXXX")"

(
  cd "$ROOT"
  go run ./cmd/pragma \
    --provider "$PROVIDER" \
    --model "$MODEL" \
    --context-mode chat \
    --max-tokens 2048 \
    --temperature 0 \
    --max-turns 5 \
    --allowed-tools "$TOOL" \
    --record \
    -p 'Using only the available JetBrains MCP tool, search the project for RegisterTools. From the returned lines, distinguish the definition line that starts with "func RegisterTools" from call lines. Report the definition and all cli.RegisterTools/RegisterTools call sites with file paths and line numbers. Do not use shell, grep, read, or prior knowledge.'
) >"$OUT"

cat "$OUT"

python3 - "$RECORDING" "$OUT" "$TOOL" <<'PY'
import json
import sys

recording, out_path, tool = sys.argv[1:]
events = []
with open(recording, encoding="utf-8") as f:
    for line in f:
        try:
            events.append(json.loads(line))
        except json.JSONDecodeError:
            pass

calls = []
for event in events:
    if event.get("kind") != "APIRequestCompleted":
        continue
    for part in event.get("content", []):
        data = part.get("data") or {}
        if part.get("type") == "tool_call":
            calls.append(data)

if not calls:
    raise SystemExit("model made no tool calls")
unexpected = [call for call in calls if call.get("name") != tool]
if unexpected:
    raise SystemExit(f"unexpected tool calls: {unexpected}")
if not any(call.get("input", {}).get("word") == "RegisterTools" for call in calls):
    raise SystemExit(f"model did not search RegisterTools: {calls}")

mcp_calls = [
    event for event in events
    if event.get("kind") == "MCPToolCallCompleted" and event.get("tool_name") == tool
]
if not mcp_calls:
    raise SystemExit("missing MCPToolCallCompleted")

completed = [
    event for event in events
    if event.get("kind") == "ToolExecutionCompleted"
    and event.get("tool_name") == tool
    and not event.get("is_error")
]
if not completed:
    raise SystemExit("missing successful ToolExecutionCompleted")

tool_output = "\n".join(event.get("output", "") for event in completed)
answer = open(out_path, encoding="utf-8").read()
expected_paths = [
    "internal/cli/tools.go",
    "internal/cli/run.go",
    "internal/cli/subcommands.go",
    "cmd/pragma/lifecycle.go",
    "cmd/pragma/replay.go",
]
for path in expected_paths:
    if path not in tool_output:
        raise SystemExit(f"missing expected occurrence in tool output: {path}")
    if path not in answer:
        raise SystemExit(f"missing expected path in final answer: {path}")
if "internal/cli/tools.go:56" not in answer:
    raise SystemExit("definition line was not reported correctly")

print("RegisterTools agent E2E passed")
print(f"tool_calls={len(calls)}")
print(f"mcp_calls={len(mcp_calls)}")
print(f"first_tool_call_id={calls[0].get('id')}")
PY
