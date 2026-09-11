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
CLK_STAMP_RE = re.compile(r"^\[pragma wall-clock (\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))\]$")
CLK_PREFIX_RE = re.compile(r"^\[pragma wall-clock (\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2}))\]\n")


class Failure(Exception):
    pass


def check(cond, msg):
    if not cond:
        raise Failure(msg)


def user_texts(body):
    """Yield the text contents of user-role wire messages, in order (CLK-001)."""
    for m in body.get("messages", []):
        if m.get("role") != "user":
            continue
        content = m.get("content")
        if isinstance(content, str):
            yield content
        elif isinstance(content, list):
            for part in content:
                if isinstance(part, dict) and part.get("type") == "text":
                    yield part.get("text", "")


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


def assert_tbgreen(reqs):
    """TURN-001 wire gate: the turn-budget notice appears once on the wire
    at the warn iteration and is retained, while the cap still kills the
    loop (--max-turns 8 => warn at turn 4, 4 remain)."""
    check(len(reqs) == 8, "expected exactly 8 requests (--max-turns 8), got %d" % len(reqs))

    def notice_msgs(body):
        n = 0
        text = ""
        for msg in body.get("messages", []):
            if msg.get("role") == "user" and "[pragma turn budget]" in str(msg.get("content") or ""):
                n += 1
                text = str(msg.get("content"))
        return n, text

    for i in range(4):
        n, _ = notice_msgs(reqs[i][1])
        check(n == 0, "request seq=%d carries %d notices, want 0" % (i + 1, n))
    n, text = notice_msgs(reqs[4][1])
    check(n == 1, "warn-iteration request (seq=5) carries %d notices, want 1" % n)
    check("4 of 8" in text and "4 remain" in text,
          "notice text missing budget state: %r" % text[:160])
    for i in (5, 6, 7):
        n, _ = notice_msgs(reqs[i][1])
        check(n == 1, "request seq=%d carries %d notices, want exactly 1 (retained)" % (i + 1, n))
    cap = ""
    for name in ("stderr.txt", "stdout.txt"):
        path = os.path.join(RESULTS, name)
        if os.path.exists(path):
            with open(path) as f:
                cap += f.read()
    check("exceeded maximum of 8 turns" in cap,
          "loop termination error 'exceeded maximum of 8 turns' not found in stdout/stderr")
    print("PASS tbgreen.notice: notice at seq=5 ('4 of 8', '4 remain'), absent before, retained once after")
    print("PASS tbgreen.cap: loop still terminated with the 8-turn error")


def assert_toklimit(reqs, expect):
    """TOK-001 wire gate: a tool-free finish_reason=length response terminates
    the provider-tools loop. expect=tokred (baseline): silent success — exit 0,
    no truncation signal. expect=tokgreen (candidate): honest classification —
    exit != 0 with the truncation error, partial text still delivered, and no
    auto-continuation (exactly one model request either way)."""
    check(len(reqs) == 1, "expected exactly 1 model request (truncated termination, no continuation), got %d" % len(reqs))
    names = tool_names(reqs[0][1])
    check(names, "boot request must carry the provider toolset")

    exit_path = os.path.join(RESULTS, "exit_code")
    check(os.path.exists(exit_path), "exit_code file missing (rerun run_probe.sh)")
    with open(exit_path) as f:
        code = int(f.read().strip())

    with open(os.path.join(RESULTS, "stdout.txt")) as f:
        stdout = f.read()
    with open(os.path.join(RESULTS, "stderr.txt")) as f:
        stderr = f.read()

    check("TOK_LIMIT_PROBE_PARTIAL_TRU" in stdout,
          "partial truncated text must still be delivered to the operator")

    if expect == "tokred":
        check(code == 0, "baseline must exit 0 (the recorded false-success classification), got %d" % code)
        check("truncated" not in stdout + stderr,
              "baseline must carry no truncation signal (that absence is the defect)")
        print("PASS tokred.shape: 1 request, exit 0, partial text, no truncation signal (RED confirmed)")
        return

    check(code != 0, "truncated termination must exit non-zero, got %d" % code)
    check("final response truncated by max_tokens output limit" in stderr,
          "truncation error text not found in stderr")
    check("conversation is preserved" in stderr,
          "truncation error must name the preserved-conversation continuation")
    print("PASS tokgreen.classification: exit %d, truncation error delivered" % code)
    print("PASS tokgreen.norestart: exactly 1 model request (no auto-continuation; E003 stays reverted)")


def clk_stamps(body):
    """(rfc3339_text, is_prefix_line) for every wall-clock stamp on the wire."""
    out = []
    for text in user_texts(body):
        m = CLK_PREFIX_RE.match(text)
        if m:
            out.append((m.group(1), True))
            continue
        m = CLK_STAMP_RE.match(text.strip())
        if m:
            out.append((m.group(1), False))
    return out


def assert_clkred(reqs, expect):
    """CLK-001 wire gate. expect=clkred (baseline): the wire carries zero
    wall-clock stamps while the loop still completes the scripted profile
    (>= 2 requests, turn-1 tools executed and paired)."""
    check(len(reqs) >= 2, "expected >= 2 requests on the wire, got %d" % len(reqs))
    stamps = []
    for meta, body in reqs:
        stamps.extend(clk_stamps(body))
    check(len(stamps) == 0,
          "RED requires zero wall-clock stamps on the wire; found %d" % len(stamps))
    meta, body = reqs[-1]
    names = tool_names(body)
    check("Bash" in names and "apply_patch" in names,
          "baseline loop shape broken: built-ins missing from tool list")
    print("PASS clkred.absence: 0 stamps across %d requests; loop completed the scripted profile" % len(reqs))


def assert_clkgreen(reqs):
    """CLK-001 wire gate (candidate): the prompt message on every request
    opens with a wall-clock prefix line; the final request carries a
    post-tools companion user message that is exactly the stamp; stamps
    parse as RFC3339 and are monotonic non-decreasing within the request."""
    check(len(reqs) >= 2, "expected >= 2 requests on the wire, got %d" % len(reqs))
    for meta, body in reqs:
        stamps = clk_stamps(body)
        check(len(stamps) >= 1,
              "request seq=%d carries no wall-clock stamp" % meta["sequence"])
        check(stamps[0][1] is True,
              "request seq=%d first stamp is not the prompt prefix line: %r"
              % (meta["sequence"], stamps[:1]))
        parsed = []
        for raw, _ in stamps:
            try:
                from datetime import datetime
                parsed.append(datetime.fromisoformat(raw.replace("Z", "+00:00")))
            except ValueError:
                raise Failure("stamp %r is not RFC3339-parseable" % raw)
        check(parsed == sorted(parsed),
              "stamps are not monotonic within request seq=%d: %s"
              % (meta["sequence"], [p.isoformat() for p in parsed]))
    meta, body = reqs[-1]
    msgs = body.get("messages", [])
    saw_tool = False
    companion = None
    for m in msgs:
        if m.get("role") == "tool":
            saw_tool = True
        elif m.get("role") == "user":
            content = m.get("content")
            text = content if isinstance(content, str) else None
            if saw_tool and text is not None and CLK_STAMP_RE.match(text.strip()):
                companion = text
    check(companion is not None,
          "final request lacks the post-tools companion stamp user message")
    names = tool_names(body)
    check("Bash" in names and "apply_patch" in names,
          "candidate loop shape broken: built-ins missing from tool list")
    print("PASS clkgreen.prefix: prompt stamp line on all %d requests" % len(reqs))
    print("PASS clkgreen.companion: post-tools companion stamp present on the final request")
    print("PASS clkgreen.monotonic: RFC3339 stamps non-decreasing on every request")


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
        elif EXPECT == "tbgreen":
            assert_tbgreen(reqs)
        elif EXPECT in ("tokred", "tokgreen"):
            assert_toklimit(reqs, EXPECT)
        elif EXPECT in ("clkred", "clkgreen"):
            assert_clkred(reqs, EXPECT) if EXPECT == "clkred" else assert_clkgreen(reqs)
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
