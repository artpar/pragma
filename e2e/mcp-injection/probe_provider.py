#!/usr/bin/env python3
"""Scripted local OpenAI-compatible provider for the MCP tool-injection probe.

Serves POST /v1/chat/completions (and GET /v1/models) with fixed responses so
a real pragma session boot can be observed without any inference spend. Every
request body is appended to requests.jsonl for cross-checking the raw HTTP
capture.

Profiles:
  provider-tools: request 1 delays PROBE_DELAY seconds (letting the harness's
    async MCP connect complete), then returns two tool calls in one turn:
    a Bash echo and a read-only MCP tool (mcp__past-conversations__list_projects).
    Request 2+ answers with final text.
  pragma: every request answers immediately with final text.
  websearch: like provider-tools, but the first response carries a single
    WebSearch tool call instead (WEB-001 probe).
  subagent: request 1 returns a single Agent tool call (SUB-001 probe);
    the sub-agent's own request (fresh conversation) and the parent's
    follow-up both answer with final text.
  turnbudget: request 1 delays (MCP connect), then EVERY request returns a
    single Bash tool call (TURN-001 probe); the loop never gets final text
    and must exhaust the turn budget (run with --max-turns 8).
  toklimit: request 1 delays (MCP connect), then returns a tool-free
    finish_reason=length response with content cut mid-word (TOK-001 probe);
    the loop must classify the truncated termination (baseline: silent
    exit 0 success; candidate: truncation error, exit != 0).
  orchestration: persona-keyed responses for the ORCH capability gate — the
    system message of each request names the active persona (architect,
    implementer, prosecutor, repair); each scripted answer is pragma-loop
    text carrying one fenced bash block that writes the state artifact and
    echoes the COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT sentinel, so a real
    `pragma orchestration run` boot walks architect → implementer →
    prosecutor → APPROVE verdict without any inference spend.

Usage: probe_provider.py <workdir> <profile> [delay_seconds]
"""
import json
import os
import sys
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

WORKDIR = sys.argv[1]
PROFILE = sys.argv[2] if len(sys.argv) > 2 else "provider-tools"
DELAY = float(sys.argv[3]) if len(sys.argv) > 3 else 15.0

os.makedirs(WORKDIR, exist_ok=True)
requests_path = os.path.join(WORKDIR, "requests.jsonl")
lock = threading.Lock()
counter = {"n": 0}

AGENT_TOOL_CALL = {
    "id": "call_probe_agent",
    "type": "function",
    "function": {
        "name": "Agent",
        "arguments": json.dumps({
            "prompt": "Reply exactly SUBAGENT_OUTPUT_MARKER and nothing else.",
            "description": "subagent probe",
        }),
    },
}

WEBSEARCH_TOOL_CALL = {
    "id": "call_probe_websearch",
    "type": "function",
    "function": {
        "name": "WebSearch",
        "arguments": json.dumps({"query": "pragma harness ai coding assistant"}),
    },
}

BASH_TOOL_CALL = {
    "id": "call_probe_bash",
    "type": "function",
    "function": {
        "name": "Bash",
        "arguments": json.dumps({"cmd": "echo MCP_INJECTION_PROBE_BASE_PATH_OK"}),
    },
}
MCP_TOOL_CALL = {
    "id": "call_probe_mcp",
    "type": "function",
    "function": {
        "name": "mcp__past-conversations__list_projects",
        "arguments": "{}",
    },
}

PAR_BLOCKER_TOOL_CALL = {
    "id": "call_probe_par_a",
    "type": "function",
    "function": {
        "name": "Bash",
        "arguments": json.dumps({"cmd": "sleep 3; echo PAR_A_DONE"}),
    },
}
PAR_QUICK_TOOL_CALL = {
    "id": "call_probe_par_b",
    "type": "function",
    "function": {
        "name": "Bash",
        "arguments": json.dumps({"cmd": "sleep 3; echo PAR_B_DONE"}),
    },
}

def tool_calls_response():
    calls = [BASH_TOOL_CALL, MCP_TOOL_CALL]
    if PROFILE == "websearch":
        calls = [WEBSEARCH_TOOL_CALL]
    if PROFILE == "subagent":
        calls = [AGENT_TOOL_CALL]
    if PROFILE == "parallel":
        calls = [PAR_BLOCKER_TOOL_CALL, PAR_QUICK_TOOL_CALL]
    if PROFILE == "turnbudget":
        calls = [{
            "id": "call_probe_tb",
            "type": "function",
            "function": {
                "name": "Bash",
                "arguments": json.dumps({"cmd": "echo TURN_BUDGET_PROBE_OK"}),
            },
        }]
    return {
        "id": "chatcmpl-probe-1",
        "object": "chat.completion",
        "created": int(time.time()),
        "model": "probe-model",
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": None,
                    "tool_calls": calls,
                },
                "finish_reason": "tool_calls",
            }
        ],
        "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
    }

def truncated_text_response():
    return {
        "id": "chatcmpl-probe-truncated",
        "object": "chat.completion",
        "created": int(time.time()),
        "model": "probe-model",
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": "TOK_LIMIT_PROBE_PARTIAL_TRU",
                },
                "finish_reason": "length",
            }
        ],
        "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
    }

def final_text_response():
    return text_response("PROBE_COMPLETE")

def text_response(content):
    return {
        "id": "chatcmpl-probe-final-" + str(int(time.time() * 1000) % 100000),
        "object": "chat.completion",
        "created": int(time.time()),
        "model": "probe-model",
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": content,
                },
                "finish_reason": "stop",
            }
        ],
        "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
    }

def _persona_bash_block(script):
    # One fenced bash block per the pragma-loop runtime contract; the loop
    # executes the script and the sentinel in its output completes the state.
    return "```bash\n" + script + "\necho COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT\n```"

def orchestration_persona_response(system_text):
    if "You are the architect" in system_text:
        script = (
            "mkdir -p /tmp/pragma\n"
            "printf 'Task: create /tmp/pragma/orch-gate.txt containing exactly "
            "gate-ok.\\nApproach: one printf shell action.\\n' > /tmp/pragma/architect-brief.md"
        )
    elif "You are the implementer" in system_text:
        script = (
            "mkdir -p /tmp/pragma\n"
            "printf 'gate-ok\\n' > /tmp/pragma/orch-gate.txt\n"
            "printf 'Created /tmp/pragma/orch-gate.txt with content gate-ok via printf.\\n' "
            "> /tmp/pragma/implementer-report.md"
        )
    elif "You are the prosecutor" in system_text:
        script = (
            "mkdir -p /tmp/pragma\n"
            "test \"$(cat /tmp/pragma/orch-gate.txt)\" = \"gate-ok\"\n"
            "printf 'Findings:\\nDeliverable present with exact required content, "
            "verified by string equality.\\n\\nRequired repair:\\nNone\\n\\n"
            "Decision:\\nAPPROVE\\n' > /tmp/pragma/prosecutor-verdict.md"
        )
    elif "You are the repair" in system_text:
        script = (
            "mkdir -p /tmp/pragma\n"
            "printf 'gate-ok\\n' > /tmp/pragma/orch-gate.txt\n"
            "printf 'Findings:\\nRe-created the deliverable.\\n\\nRequired repair:\\nNone\\n\\n"
            "Decision:\\nAPPROVE\\n' > /tmp/pragma/prosecutor-verdict.md"
        )
    else:
        return final_text_response()
    # The pragma loop's extraction contract: exactly one fenced bash block
    # with no prose before or after it (extractPragmaLoopCommand rejects
    # anything else), so the scripted persona response is block-only.
    return text_response(_persona_bash_block(script))

class Handler(BaseHTTPRequestHandler):
    protocol_version = "HTTP/1.1"

    def log_message(self, fmt, *args):
        pass

    def _send_json(self, status, obj):
        body = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        if self.path.rstrip("/").endswith("/models"):
            self._send_json(200, {"object": "list", "data": [{"id": "probe-model", "object": "model"}]})
            return
        self._send_json(404, {"error": {"message": "not found"}})

    def do_POST(self):
        if not self.path.rstrip("/").endswith("/chat/completions"):
            self._send_json(404, {"error": {"message": "not found"}})
            return
        length = int(self.headers.get("Content-Length", "0"))
        raw = self.rfile.read(length)
        with lock:
            counter["n"] += 1
            seq = counter["n"]
            body = json.loads(raw.decode())
            with open(requests_path, "a") as f:
                f.write(json.dumps({"seq": seq, "path": self.path, "body": body}) + "\n")
        if PROFILE == "orchestration":
            if seq == 1:
                time.sleep(min(DELAY, 5.0))
            system_text = ""
            for message in body.get("messages", []):
                if isinstance(message, dict) and message.get("role") == "system":
                    content = message.get("content")
                    if isinstance(content, str):
                        system_text += content
            self._send_json(200, orchestration_persona_response(system_text))
            return
        if PROFILE in ("provider-tools", "websearch", "subagent", "parallel") and seq == 1:
            time.sleep(DELAY)
            self._send_json(200, tool_calls_response())
            return
        if PROFILE == "turnbudget":
            if seq == 1:
                time.sleep(DELAY)
            self._send_json(200, tool_calls_response())
            return
        if PROFILE == "toklimit" and seq == 1:
            time.sleep(DELAY)
            self._send_json(200, truncated_text_response())
            return
        self._send_json(200, final_text_response())

def main():
    server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
    port = server.server_address[1]
    with open(os.path.join(WORKDIR, "port"), "w") as f:
        f.write(str(port))
    with open(os.path.join(WORKDIR, "profile"), "w") as f:
        f.write(PROFILE + "\n")
    with open(os.path.join(WORKDIR, "delay"), "w") as f:
        f.write(str(DELAY) + "\n")
    print("probe provider listening on 127.0.0.1:%d profile=%s delay=%s" % (port, PROFILE, DELAY), flush=True)
    server.serve_forever()

if __name__ == "__main__":
    main()
