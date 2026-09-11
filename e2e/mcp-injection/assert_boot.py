#!/usr/bin/env python3
"""Assertions for the MCP tool-injection probe (MCPINJ-001).

Parses the raw HTTP capture of a real pragma session boot and evaluates the
RED (baseline) or GREEN (candidate) assertion set, plus the pragma-loop-mode
adjacent check. Exits 0 on success, 1 on assertion failure.

Usage: assert_boot.py <results-dir> <expect: red|green|pragma|webred|webgreen>
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
        if "/chat/completions" not in meta.get("url", ""):
            # Non-model wires (e.g. the WebSearch tool's Brave call) are
            # separate evidence, not part of the model-request assertions.
            continue
        body_path = os.path.join(os.path.dirname(meta_path), "request.json")
        with open(body_path) as f:
            body = json.load(f)
        metas.append((meta, body))
    metas.sort(key=lambda pair: pair[0]["sequence"])
    return metas


def brave_captures():
    """Captured non-model wires (the WebSearch tool's Brave request, if any)."""
    out = []
    for meta_path in glob.glob(os.path.join(RAW, "*", "request.meta.json")):
        with open(meta_path) as f:
            meta = json.load(f)
        if "/chat/completions" in meta.get("url", ""):
            continue
        req = os.path.join(os.path.dirname(meta_path), "request.json")
        body = ""
        if os.path.exists(req):
            with open(req) as f:
                body = f.read()
        out.append((meta, body))
    out.sort(key=lambda pair: pair[0]["sequence"])
    return out


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


def assert_webred(reqs):
    check(len(reqs) >= 2, "expected >= 2 requests on the wire, got %d" % len(reqs))
    for meta, body in reqs:
        names = tool_names(body)
        check("WebSearch" not in names,
              "RED requires WebSearch absent from tools; got %s" % names)
    unknown = False
    for meta, body in reqs:
        for name, content in tool_messages(body):
            if name == "WebSearch" and "unknown tool" in content:
                unknown = True
    check(unknown, "RED requires the model-issued WebSearch call to fail with 'unknown tool'")
    print("PASS webred.absence: WebSearch absent from every tools list")
    print("PASS webred.execution: WebSearch call answered 'unknown tool'")


def assert_webgreen(reqs):
    check(len(reqs) >= 2, "expected >= 2 requests on the wire, got %d" % len(reqs))
    for i, (meta, body) in enumerate(reqs):
        names = tool_names(body)
        check("WebSearch" in names, "request %d missing WebSearch in tools: %s" % (i + 1, names))
    meta, body = reqs[-1]
    names = tool_names(body)
    check("Bash" in names, "GREEN requires Bash still in the tool list")
    check("apply_patch" in names, "GREEN requires apply_patch still in the tool list")
    check(any(MCP_RE.match(n) for n in names),
          "GREEN requires MCP defs still injected alongside WebSearch")
    builtin_idx = names.index("apply_patch")
    check(names.index("WebSearch") > builtin_idx,
          "WebSearch must follow the built-ins")
    result = None
    for m, b in reqs:
        for name, content in tool_messages(b):
            if name == "WebSearch":
                result = content
    check(result is not None, "GREEN requires an executed WebSearch tool result on the wire")
    check("unknown tool" not in (result or ""), "WebSearch result still says 'unknown tool'")
    check("STUB_BRAVE_RESULT" in (result or ""),
          "GREEN requires real tool-path output (stub marker) in the WebSearch result")
    stub_hits = brave_captures()
    check(len(stub_hits) >= 1, "GREEN requires the Brave search wire captured alongside the model wire")
    meta = stub_hits[0][0]
    check("/res/v1/web/search" in meta.get("url", "") and "q=pragma+harness" in meta.get("url", ""),
          "Brave wire must be a search call carrying the probe query: %s" % meta.get("url"))
    # token must be sent but redacted in the capture
    import glob as _g
    for mp in _g.glob(os.path.join(RAW, "*", "request.meta.json")):
        with open(mp) as f:
            m = json.load(f)
        if "/chat/completions" in m.get("url", ""):
            continue
        hp = os.path.join(os.path.dirname(mp), "request.headers.json")
        with open(hp) as f:
            headers = json.load(f)
        token = headers.get("X-Subscription-Token", [])
        if token:
            check(token[0] == "<redacted>",
                  "X-Subscription-Token must be redacted in captured headers: %r" % token)
    print("PASS webgreen.brave-wire: search request on the wire (%s), token redacted" % meta.get("url"))
    print("PASS webgreen.injection: WebSearch present in all %d requests after built-ins" % len(reqs))
    print("PASS webgreen.execution: WebSearch executed through the tool path (%d bytes)"
          % len(result or ""))


def assert_weblive(reqs):
    """Live Brave contract gate: the search executed against the real
    endpoint and its parsed results reached the model on the wire."""
    check(len(reqs) >= 2, "expected >= 2 requests on the wire, got %d" % len(reqs))
    for i, (meta, body) in enumerate(reqs):
        names = tool_names(body)
        check("WebSearch" in names, "request %d missing WebSearch in tools" % (i + 1))
    result = None
    for m, b in reqs:
        for name, content in tool_messages(b):
            if name == "WebSearch":
                result = content
    check(result is not None, "live gate requires an executed WebSearch result")
    check("Web search results for query" in (result or ""),
          "live result missing formatted output: %r" % (result or "")[:200])
    check("unknown tool" not in (result or ""), "live result is an unknown-tool error")
    check("WebSearch failed" not in (result or ""), "live result is a failure: %r" % (result or "")[:200])
    check("STUB_BRAVE_RESULT" not in (result or ""), "live result unexpectedly contains stub data")
    print("PASS weblive.contract: real Brave result parsed through the tool path (%d bytes)" % len(result))
    print("PASS weblive.pairing: 2 requests accepted (tool result paired)")


def assert_subred(reqs):
    check(len(reqs) >= 2, "expected >= 2 requests on the wire, got %d" % len(reqs))
    for meta, body in reqs:
        names = tool_names(body)
        check("Agent" not in names, "RED requires Agent absent from tools; got Agent in request %s" % meta["sequence"])
    unknown = False
    for meta, body in reqs:
        for name, content in tool_messages(body):
            if name == "Agent" and "unknown tool" in content:
                unknown = True
    check(unknown, "RED requires the model-issued Agent call to fail with 'unknown tool'")
    print("PASS subred.absence: Agent absent from every tools list")
    print("PASS subred.execution: Agent call answered 'unknown tool'")


def assert_subgreen(reqs):
    check(len(reqs) >= 3, "expected >= 3 requests on the wire (parent, sub, parent), got %d" % len(reqs))
    for meta, body in reqs:
        pass
    parent_tools = tool_names(reqs[0][1])
    check("Agent" in parent_tools, "parent request must advertise Agent: %s" % parent_tools[:4])
    # sub request: fresh conversation, Agent absent from its tools
    sub_meta, sub_body = reqs[1]
    sub_names = tool_names(sub_body)
    check("Agent" not in sub_names, "sub request must NOT advertise Agent (recursion guard): found it")
    user_msgs = [m for m in sub_body.get("messages", []) if m.get("role") == "user"]
    check(len(user_msgs) == 1 and "SUBAGENT_OUTPUT_MARKER" in (user_msgs[0].get("content") or ""),
          "sub request must be a fresh conversation carrying only the sub prompt; user messages: %r"
          % [m.get("content") for m in user_msgs][:2])
    any_parent_history = any("call_probe_agent" in json.dumps(m) for m in sub_body.get("messages", []))
    check(not any_parent_history, "sub request must not carry parent tool-call history")
    print("PASS subgreen.fresh: sub request is a fresh conversation, Agent excluded from its tools")
    result = None
    for m, b in reqs:
        for name, content in tool_messages(b):
            if name == "Agent":
                result = content
    check(result is not None, "GREEN requires the Agent tool result on the wire")
    check("SUBAGENT_OUTPUT_MARKER" in (result or ""),
          "Agent result must carry the sub-agent final text: %r" % (result or "")[:200])
    envelope = {}
    try:
        envelope = json.loads(result)
    except Exception:
        pass
    check(envelope.get("status") == "completed",
          "Agent result envelope must be status=completed: %r" % (result or "")[:200])
    check("prompt" in envelope and "result" in envelope and "tokens_used" in envelope,
          "Agent result envelope missing branch fields: %r" % (result or "")[:200])
    final_meta, final_body = reqs[-1]
    check("Agent" in tool_names(final_body), "parent must keep advertising Agent on later requests")
    print("PASS subgreen.result: sub-agent text returned in the branch envelope, parent loop completed")


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
        elif EXPECT == "webred":
            assert_webred(reqs)
        elif EXPECT == "webgreen":
            assert_webgreen(reqs)
        elif EXPECT == "weblive":
            assert_weblive(reqs)
        elif EXPECT == "subred":
            assert_subred(reqs)
        elif EXPECT == "subgreen":
            assert_subgreen(reqs)
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
