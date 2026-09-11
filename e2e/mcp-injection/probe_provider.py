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

def tool_calls_response():
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
                    "tool_calls": [BASH_TOOL_CALL, MCP_TOOL_CALL],
                },
                "finish_reason": "tool_calls",
            }
        ],
        "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
    }

def final_text_response():
    return {
        "id": "chatcmpl-probe-final",
        "object": "chat.completion",
        "created": int(time.time()),
        "model": "probe-model",
        "choices": [
            {
                "index": 0,
                "message": {
                    "role": "assistant",
                    "content": "PROBE_COMPLETE",
                },
                "finish_reason": "stop",
            }
        ],
        "usage": {"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
    }

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
            with open(requests_path, "a") as f:
                f.write(json.dumps({"seq": seq, "path": self.path, "body": json.loads(raw.decode())}) + "\n")
        if PROFILE == "provider-tools" and seq == 1:
            time.sleep(DELAY)
            self._send_json(200, tool_calls_response())
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
