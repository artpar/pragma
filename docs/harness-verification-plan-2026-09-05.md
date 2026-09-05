# Harness verification record — 2026-09-05/06

Historical experiment record. [agent.md](../agent.md) supersedes the experiment
sequencing below. This document does not authorize further runs.

Update 2026-09-06: the nonstreaming reasoning fix is now applied in the main
working tree with an authentic offline baseline-fail/candidate-pass regression.
See [case provenance and command](../internal/provider/openrouter/testdata/README.md).
Statements below that the candidate remains isolated describe the earlier phase.

Direct API diagnostic, 2026-09-06: the recorded circuit-fibsqrt request was
reconstructed and posted to Lilac GLM-5.2 without Pragma or SDK conversion.
With max_tokens 256, HTTP 200 returned in 4.52s: finish_reason length,
content null, tool_calls [], 256 completion tokens, and 1090 characters in
reasoning ending mid-thought. This establishes reasoning-only truncation at
the provider boundary. With the original 32768 limit, the single call timed
out at 360.70s without response bytes; original-limit reproduction remains
unresolved. No retries. Raw evidence and reconstruction caveats:
`.pragma/verification/20260906-truncation/README.md`.

A bounded OpenRouter z-ai/glm-5.3 cross-provider probe at max_tokens 256 also
returned HTTP 200 with finish_reason length, content null, no tool calls, and
1132 characters of reasoning ending mid-analysis. Thus the response shape is
not Lilac-specific. Because model revision and routing differ, frequency and
behavior at 32768 tokens remain separate questions.

This follows `benchmark-investigation-2026-09-05.md`. Three subagents audited
runtime code, actual failed-task sessions, and the evaluator independently.
Their findings corrected several earlier suggestions. No improvement is
accepted solely because a mock, fixture, or unit test passes.

## Acceptance rule

For a proposed change, preserve evidence of:

1. The defect occurring in an actual Pragma task run.
2. The provider response, request, command, or workspace state locating its cause.
3. An authentic recorded regression through the production path: unchanged code
   fails the declared assertion and the isolated change passes it.
4. Only the additional integration, live-contract, or task-level evidence needed
   for the claim, under the gates in agent.md. A fresh complete solver run is not
   required to establish deterministic harness correctness.

Protocol correctness and task-score improvement are separate claims. Exact
field preservation can be established from actual HTTP exchanges. A single
task win cannot establish general score improvement or exclude sampling noise.

## Hypotheses after the independent audits

| Hypothesis | Evidence | Disposition |
| --- | --- | --- |
| Token truncation is reported as task completion | Actual final-89 circuit-fibsqrt and dna-assembly logs end at max_tokens; current provider-tools branch returns TurnCompleteEvent | Defect confirmed; continuation benefit still requires a real affected run |
| Reasoning is lost on the Lilac path | Conversion omits decoded reasoning; an existing passing test explicitly expects the loss | Defect confirmed; distinct from OpenRouter |
| Reasoning is lost on OpenRouter | Current dependency drops it in both response and request conversion; a real endpoint probe returned both reasoning fields | Candidate selected for real task-level wire verification |
| New process/polling infrastructure is needed | Shellrun already stores logs/status/PID and implements soft waiting; caller sets wait and timeout both to 300 seconds | Withdraw the proposed rewrite; expose existing behavior only if an actual wait bottleneck is observed |
| Compaction would fix observed failures | One trace reached 193,067 tokens relative to fallback 200k metadata, then hit the turn cap; no proven context-overflow failure | Defer |
| A generic final review would fix cancellation/Cython | Both agents already ran checks; cancellation check covered queued jobs but not awaited cleanup; Cython check excluded .pyx and verified imports rather than named function execution | Unproven; do not add a reviewer by default |
| POV-Ray used inauthentic source | Actual session downloads official archives; extraction piped to head returns 141; auxiliary files land in another directory | Earlier wording was too strong; investigate extraction completeness/output handling |

## Real cancellation failure reproduced

The exact archived `run.py`, not a simplified replacement, was recovered from
the successful write in the final-89 session. SHA256:
`957dc880ed54c35370939b061ce5d53deb50a7cc1f6f529d67b8b390fa8c59d0`.

A child process imports that file, starts three jobs with concurrency two,
waits until two have actually started, then receives SIGINT. Immediate cleanup
completes twice. Cleanup containing an await completes zero times. This occurs
on local Python 3.13.3 and 3.14.2. The historical container was 3.13.7; this
reproduction is real process behavior, not an official benchmark score.

The agent had tested five queued jobs/concurrency three using wait_for and a
synchronous list append in finally, with a weak assertion that at least one
cleanup happened. Therefore the earlier claim that it simply did not test
queued jobs was incorrect. Missing awaited cleanup is a demonstrated test gap.
No additional shell capability is necessary to exercise it.

## Restored evaluator and preflight

Original task paths under /tmp had disappeared. The exact task was recovered
from `https://huggingface.co/datasets/harborframework/terminal-bench-2.1` at
commit `2317b760e132c35b5c06d68e97affc4d9064da44`.

- Upstream revision named by the mirror: `7131e4375048a0e408a8fb404b5f499d726b695b`.
- Task directory checksum: `1566331f6913b8a4f9af6779e90cabbb79edf1b15c126b6bc544912df3f14de5`.
- Harbor task digest: `a3d048d351136e48070696cda8bb79660dfd74db1fea3b6da88559f0332699c1`.
- Both hashes match the historical saved results/lock.

The original prebuilt x86 image failed preflight: the official reference
solution received reward zero because uvx segfaulted under QEMU before pytest
ran. Harbor reported no exception. This is new direct confirmation that reward
zero alone is not sufficient evidence of task failure.

Building the unchanged official Dockerfile natively for arm64 made the
reference solution pass all six unchanged tests. Python is 3.13.15. This is a
new evaluation environment, not interchangeable with historical x86 scores.
The native image ID observed during preflight is
`sha256:b157436b0d1bf51ffa7caa284909b03a89023e67c01dc6365adfe139ec0e4fc0`.

## Historical isolated experiment (completed; outcomes below)

Task: cancel-async-tasks, restored source above. Actual Harbor execution and
official verifier; task tests/solution are not supplied to the task-solving
model. An adapter wrapper only enables existing HTTP capture and delegates to
the existing Pragma Harbor adapter. It does not synthesize any responses.

Fixed settings: exact OpenRouter z-ai/glm-5.3, temperature zero, maximum output
16,384, 100 model turns, 600-second agent timeout, native task environment,
serial execution. Both binaries start from source commit 1315159. The
candidate changes only OpenRouter nonstreaming reasoning conversion and
storage/replay support. Streaming remains outside the candidate's scope.

The live API confirmed funded credit and model availability. One preliminary
tool-call probe cost $0.0004008; that is endpoint evidence only, not acceptance
evidence. The first actual baseline task request then spent minutes waiting for
the model before any tool execution. The live catalog reports mandatory
reasoning with default effort max and supported efforts max/high/low. Changing
shell execution cannot shorten that observed pre-tool delay.

Before accepting the candidate, compare actual initial request bodies for
unintended parameter changes. For every returned tool call, match its assistant
message in the next request and compare reasoning text and the complete details
array. Record routing provider, model, input/output tokens, cost, time, and
official task result independently. Routing is observed rather than pinned;
different provider routes limit any task-score comparison.

Before any completed baseline task response, the default-effort calibration
was operator-cancelled after roughly seven minutes of its first request sending
only keep-alive whitespace. It is censored, not an agent task failure. The
paired experiment is reset for both arms to explicit supported low reasoning
effort, using existing CLI flags `--thinking --thinking-budget 1024`, which
translate to `reasoning_effort: low`. All other settings remain as above. This
does not prove that effort caused the latency; it establishes a bounded
configuration calibration to be checked in actual requests. No default-effort
candidate run is used as a comparison against the low-effort baseline.

## Historical proposed sequence (superseded; not an active queue)

1. Finish the real baseline/candidate pair. Reject or repair the candidate if
   fields are still lost, initial request semantics drift, the provider rejects
   history, or the task cannot reach a functioning verifier. Preserve a task
   failure even if protocol preservation succeeds.
2. If task outcomes tie, retain only the narrow evidence claim. Do not launch
   the full suite to search for a positive score.
3. Investigate inference latency separately: compare max versus a supported
   lower reasoning effort on the same real tasks, holding harness/model/output
   budget fixed. Measure time to first action and final task correctness.
   This is a future configuration experiment, not part of this adapter change.
4. Test a verification intervention only against an equal-budget ordinary
   continuation, using actual failed workspaces and original task instructions.
   Count whether it independently discovers the missed condition and repairs
   the result; do not feed it hidden verifier requirements.
5. Use installed Harbor Terminus2 for a same-model whole-harness comparison
   before designing additional personas or execution infrastructure. Different
   tool protocols are part of that treatment; report them rather than calling
   the comparison a tool-only ablation.

Broad benchmark evaluation follows a repeatable small-case effect. It is not
the first acceptance gate for every implementation change.

## Completed real pair and causal limit

Both fresh runs finished with working official verifiers:

| Measurement | Unmodified Pragma | Isolated candidate |
| --- | --- | --- |
| Official tests | 5 passed, 1 failed | 6 passed |
| Official reward | 0 | 1 |
| Actual returned reasoning fields checked in next requests | 4 lost / 4 checked | 0 lost / 4 checked |
| Model requests | 7 | 5 |
| Reported inference cost | $0.01488528 | $0.00988340 |
| Total job time, including verifier setup | 2m45s | 2m21s |

The initial real request JSON bodies are equal. Native image filesystem layers
are identical. Upstream routing varied (baseline AtlasCloud/DeepInfra/Modal;
candidate AtlasCloud/Modal), so this is not a provider-pinned score experiment.

Most importantly, the candidate's passing worker-pool implementation appeared
in its FIRST response, before reasoning replay could affect a subsequent
request. The baseline's first implementation instead used semaphore-wrapped
gather. Therefore the observed task win is NOT evidence that this adapter fix
caused the win. This pair establishes real field-loss repair and a real passing
task, but does not establish harness-caused task-success improvement. Claiming
otherwise would repeat the attribution error this investigation criticizes.

The candidate remains isolated at `/tmp/pragma-reasoning.dy8b9L/candidate`;
main product source is unchanged. Run evidence, generated implementations,
wire-audit script, and exact task source are preserved under
`.pragma/verification/20260905/`. Before pursuing score improvement, choose a failing decision where the
proposed intervention is active BEFORE the relevant choice, and compare real
continuations from equivalent task/conversation state.

## Subagent follow-through and reference comparison

The failure audit reproduced the original generated implementation failing
with awaited cleanup and corrected the diagnosis of the latest self-test:
its marker counted entry into cleanup, not completion after its await.
The proposed resumed-review intervention has not been run.

The experiment audit built a cached, disposable, network-disabled diagnostic
verifier using unchanged task tests. Actual baseline and candidate submissions
reproduced 5/6 and 6/6 in 13.20 and 14.16 seconds respectively. This is a fast
diagnostic rerun, not a new full official benchmark run.

A stock Terminus2 same-model reference was started, then operator-cancelled
after the runtime audit found installed LiteLLM silently drops low reasoning
effort for unknown GLM-5.3 metadata under Harbor's drop_params=True. That run
is censored and cannot be treated as a matched comparison or task failure.
The corrected configuration registers supports_reasoning and captures the
transformed outgoing request for verification before interpreting a score.
Reference artifacts live under /tmp/pragma-live-verification.taYP1h/jobs/.

### Corrected Terminus reference result

The real transformed request confirmed model z-ai/glm-5.3, temperature 0,
max_tokens 16384 and reasoning_effort low. The official verifier then reported
5/6 and reward 0: the same queued-cancellation cleanup failure as unmodified
Pragma (two tasks started, zero cleanup completions). This single case provides
no evidence that Terminus outperforms Pragma. Routes remain unpinned.

Terminus used five model calls at reported cost $0.03435492. Its own test put
the completion marker after awaited cleanup, but tested cancellation with one
task and concurrency separately. It never checked cancellation with queued
tasks. Its built-in completion confirmation produced reassurance rather than
an additional check. Visible analysis/plan and the confirmation prompt did not
prevent this failure. Hidden reasoning was not replayed by either stock harness.

The remaining concrete candidate is test construction for interacting task
requirements, not a missing shell capability. A future diagnostic continuation
must compare an instruction to test interacting requirements against an
equal-budget ordinary review from the same failed Pragma state, without
revealing hidden tests. No such continuation has been launched.
