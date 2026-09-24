# SWE-bench Pro calibration run — declared plan (2026-09-24)

Run plan declared BEFORE any credit spend, per agent.md live-experiment rules.

## Question this run answers

Does the full pipeline work end-to-end (runner → Docker container → pragma
agent → patch → official local evaluator), and what does ONE instance cost
in wall-time and USD through the current harness?

## Frozen candidate

Commit `b0b06c3` (2026-09-24, includes: pragma-watch + daemon, CMP-001
auto-compact restoration in both loop modes, CMP-001.1 cooldown/dead-assertion
fix, CMP-001.2 session-rewrite + prompt preservation, CMP-001.4a fork-deps
fix, budget cap --max-cost, self-improvement loop v1.3). Binary built in a
detached git worktree pinned at the commit — no working-tree drift can leak
into the candidate.

## Instance selection (deterministic, declared before any result)

Calibration: FIRST line of
`SWE-bench_Pro-os/helper_code/sweap_eval_full_v2.jsonl` =
`instance_NodeBB__NodeBB-04998908ba6721d64eba79ae3b65a351dcfbc5b5-vnan`.
Chosen by position, not content; no instance was inspected before selection.

## Runner configuration

`tools/run_swebench_pro_instance.py --pull-image --evaluate` with defaults:
provider=morphllm, model=morph-glm53-744b, PRAGMA_MAX_TURNS=250,
PRAGMA_AGENT_TIMEOUT=7200s. Cost cap: provider-side budget (operator
delegation 2026-09-24: "work as long as the key works").

## Metrics recorded

- Solved / not solved (official local evaluator), evaluator log preserved
- Wall time; agent turn count; USD cost (session metadata)
- Any pipeline failure with its earliest wrong transition

## Claim boundary

This run calibrates mechanics and cost ONLY. No task-success claim may be
made from n=1. The subsequent declared set (20 instances, deterministic
rule: every 37th line of the file starting at index 0, i.e. lines
0,37,74,...,703) is the first generalization measurement, with uncertainty
reported over its sample size.

## Pipeline defect found by attempt 1 (recorded before attempt 2)

Attempt 1 (2026-09-24 15:30, output dir 20260924T100017Z) failed before any
model call: the agent binary crashed at startup inside the container with
`runtime: taggedPointerPack invalid packing` in `netpollopen` on the first
pollable file open (exit 2, empty patch). Reproduced deterministically with
zero credit spend (`pragma --help` in alpine/linux-amd64 on this arm64 Mac).
Root cause: Go 1.25 runtime binaries crash under Rosetta x86-64 emulation
(golang/go#75721; the emulated heap places pointers where the runtime's
tagged-pointer packing cannot fit them). Not a pragma defect; the host toolchain
was go1.25.0 (only toolchain in ~/sdk; /usr/local/go 1.22.2 cannot build the
repo — any-llm-go v0.9.0 requires go >= 1.25).

Fix applied to build infrastructure only (frozen source candidate unchanged):
rebuild the linux/amd64 benchmark binary with GOTOOLCHAIN=go1.26.8 —
verified clean under emulation (--help boots, opens files, exits 0).
Attempt 2 relaunched 15:38 with the go1.26.8 toolchain (output dir
20260924T100832Z). Standing rule recorded: linux/amd64 emulation-target builds
of pragma must use go >= 1.26 (local macOS-native builds unaffected).
