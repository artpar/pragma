#!/usr/bin/env python3
"""Assertions for the MCP tool-injection probe (MCPINJ-001).

Parses the raw HTTP capture of a real pragma session boot and evaluates the
RED (baseline) or GREEN (candidate) assertion set, plus the pragma-loop-mode
adjacent check. Exits 0 on success, 1 on assertion failure.

Usage: assert_boot.py <results-dir> <expect: red|green|pragma>
"""
import glob
import json
import os
import re
import sys

RESULTS = sys.argv[1]
EXPECT = sys.argv[2]
RAW = os.path.join(RESULTS, "raw")

MCP_RE = re.compile(r"^mcp__")
SERVERS_RE = re.compile(r"^- name: (\S+)\n  status: (\S+)$", re.M)


class Failure(Exception):
    pass


def check(cond, msg):
    if not cond:
        raise Failure(msg)


def load_requests():
    metas = []
    for meta_path in glob.glob(os.path.join(RAW, "*", "request.meta.json")):
        with open(meta_path) as f:
            meta = json.load(f)
        body_path = os.path.join(os.path.dirname(meta_path), "request.json")
        with open(body_path) as f:
            body = json.load(f)
        metas.append((meta, body))
    metas.sort(key=lambda pair: pair[0]["sequence"])
    return metas


def system_text(body):
    parts = []
    for msg in body.get("messages", []):
        if msg.get("role") == "system":
            parts.append(msg.get("content") or "")
    return "\n".join(parts)


def tool_names(body):
    names = []
    for t in body.get("tools") or []:
        fn = t.get("function") or {}
        names.append(fn.get("name") or t.get("name") or "")
    return names


def mcp_statuses(sys_text):
    servers = {}
    for m in SERVERS_RE.finditer(sys_text):
        servers[m.group(1)] = m.group(2)
    return servers


def connected(servers):
    return {k for k, v in servers.items() if v == "connected"}


def tool_messages(body):
    """role=tool messages: (tool_name, content), names recovered from the
    assistant tool_call ids (the wire shape carries only tool_call_id)."""
    id_to_name = {}
    for msg in body.get("messages", []):
        if msg.get("role") == "assistant":
            for tc in msg.get("tool_calls") or []:
                id_to_name[tc.get("id")] = (tc.get("function") or {}).get("name") or ""
    out = []
    for msg in body.get("messages", []):
        if msg.get("role") == "tool":
            out.append((id_to_name.get(msg.get("tool_call_id"), ""), msg.get("content") or ""))
    return out


def assert_red(reqs):
    check(len(reqs) >= 2, "expected >= 2 requests on the wire, got %d" % len(reqs))
    absence_req = None
    for meta, body in reqs:
        names = tool_names(body)
        servers = connected(mcp_statuses(system_text(body)))
        mcp_names = [n for n in names if MCP_RE.match(n)]
        if len(servers) >= 4 and sorted(names) == ["Bash", "apply_patch"] and not mcp_names:
            absence_req = (meta["sequence"], servers)
    check(absence_req is not None,
          "RED requires a request with >=4 MCP servers connected whose tools are exactly "
          "[Bash, apply_patch] with zero mcp__ entries")
    print("PASS red.absence: request seq=%d connected_servers=%d tools=[Bash, apply_patch] only"
          % (absence_req[0], len(absence_req[1])))
    unknown = False
    for meta, body in reqs:
        for name, content in tool_messages(body):
            if MCP_RE.match(name) and "unknown tool" in content:
                unknown = True
    check(unknown, "RED requires the model-issued mcp__ tool call to fail with 'unknown tool' "
                  "(execution-path absence)")
    print("PASS red.execution: mcp__ call answered 'unknown tool'")


def assert_green(reqs):
    check(len(reqs) >= 2, "expected >= 2 requests on the wire, got %d" % len(reqs))
    meta, body = reqs[-1]
    names = tool_names(body)
    servers = connected(mcp_statuses(system_text(body)))
    check(len(servers) >= 4,
          "GREEN requires >=4 MCP servers connected at final request; got %d: %s"
          % (len(servers), sorted(servers)))
    check("Bash" in names, "GREEN requires Bash still in the tool list")
    check("apply_patch" in names, "GREEN requires apply_patch still in the tool list")
    mcp_names = [n for n in names if MCP_RE.match(n)]
    check(len(mcp_names) > 0, "GREEN requires mcp__ tool defs injected; got none")
    prefixes = {n.split("__")[1] for n in mcp_names}
    missing = {s for s in servers if s not in prefixes}
    check(not missing, "GREEN requires >=1 injected tool per connected server; missing: %s" % sorted(missing))
    check("mcp__past-conversations__list_projects" in mcp_names,
          "GREEN requires the probed tool mcp__past-conversations__list_projects to be advertised")
    print("PASS green.injection: request seq=%d tools=%d (%d mcp__ from %d servers: %s)"
          % (meta["sequence"], len(names), len(mcp_names), len(prefixes), sorted(prefixes)))
    mcp_result = None
    bash_result = None
    for m, b in reqs:
        for name, content in tool_messages(b):
            if name == "mcp__past-conversations__list_projects":
                mcp_result = content
            if name == "Bash" and "MCP_INJECTION_PROBE_BASE_PATH_OK" in content:
                bash_result = content
    check(mcp_result is not None, "GREEN requires an executed mcp__ tool result on the wire")
    check("unknown tool" not in (mcp_result or ""), "mcp__ tool result still says 'unknown tool'")
    check(len(mcp_result.strip()) > 0, "mcp__ tool result is empty")
    print("PASS green.execution: mcp__past-conversations__list_projects returned %d bytes from the real MCP server"
          % len(mcp_result))
    check(bash_result is not None, "GREEN requires the Bash tool result (built-in path unchanged)")
    print("PASS green.builtin: Bash echo result present on the wire")
    print("PASS green.pairing: request 2 accepted by the loop (both tool calls of turn 1 paired)")


def assert_pragma(reqs):
    check(len(reqs) >= 1, "expected >= 1 request on the wire, got 0")
    for meta, body in reqs:
        names = tool_names(body)
        check(not names, "pragma loop mode must not send provider tools; got %s" % names)
    print("PASS pragma.shape: %d request(s), no tools sent" % len(reqs))


def main():
    reqs = load_requests()
    print("loaded %d captured requests from %s" % (len(reqs), RAW))
    try:
        if EXPECT == "red":
            assert_red(reqs)
        elif EXPECT == "green":
            assert_green(reqs)
        elif EXPECT == "pragma":
            assert_pragma(reqs)
        else:
            raise Failure("unknown expectation %r" % EXPECT)
    except Failure as e:
        print("ASSERTION FAILED (%s): %s" % (EXPECT, e))
        sys.exit(1)
    summary = {
        "expect": EXPECT,
        "results_dir": RESULTS,
        "request_count": len(reqs),
        "requests": [
            {
                "seq": meta["sequence"],
                "url": meta["url"],
                "tool_names": tool_names(body),
                "mcp_connected": sorted(connected(mcp_statuses(system_text(body)))),
                "sha256": meta.get("request_sha256", ""),
            }
            for meta, body in reqs
        ],
    }
    with open(os.path.join(RESULTS, "assertion.json"), "w") as f:
        json.dump(summary, f, indent=2)
    print("wrote %s" % os.path.join(RESULTS, "assertion.json"))
    print("ASSERTION PASSED (%s)" % EXPECT)


if __name__ == "__main__":
    main()
