# INT-002 — Bash tool calls whose input uses a synonymous key (`command`)
# fail with a terse error instead of executing or self-correcting

Opened 2026-09-12 after live recurrence inside the roadmap-continuation
session itself. Methodology: `agent.md`. Related case: INT-001 (opened the
same night); separate mechanism, separate change.

## Source events (authentic, recorded)

1. **Tonight, this session** — `~/.pragma/logs/2026-09-12T00-46-33.jsonl`:
   the same session's own Bash calls intermittently returned
   `Bash input requires non-empty cmd` (IsError). The completed-request
   contents show the emitted tool-call inputs carried the command under a
   `command` key, not the schema's `cmd` — verified at completions
   01:27:49.059, 01:27:57.700, 01:28:20.995 (+ two at ~01:25:01/01:25:07
   and two later at 01:31:49/01:33:43 — 7+ occurrences), interleaved with
   successful calls whose inputs used `cmd`. Retrying an equivalent call
   often succeeds — the failure follows the model's per-call key emission,
   not the command content.
2. **Historical, 2026-08-30** — Terminal-Bench GLM-52 session
   `.pragma/terminal-bench/jobs/pragma-lilac-glm52-final-89/distribution-search__pmsyzhi/agent/home/.pragma/sessions/0f9bc465-9717-4e12-9791-9b4957e25c9c.jsonl`
   (line 78): the identical error string — the defect has been silently
   burning benchmark and operator turns for weeks on this route.

## Production path and the earliest wrong transition

`internal/query/provider_tools_loop.go executeProviderBashTool`
(provider-tools loop; the mode this route runs): `json.Unmarshal(call.Input,
&struct{Cmd string `json:"cmd"`})` — Go silently ignores the unknown
`command` key, leaving `Cmd` empty; the empty check then returns
`"Bash input requires non-empty cmd"`. The earliest wrong transition is the
parse: a semantically clear input is treated as absent.

## Observed vs expected

Observed: `{"command": "<shell text>"}` → error result; the error does not
echo which keys were received, so the model cannot tell key-mismatch from
empty-command and must rediscover the cause (tonight: four wasted turns and
a session-log investigation to diagnose).

Expected: a Bash tool call carrying the command under the observed
synonymous key executes; if no recognized key is present, the error lists
the received top-level keys so the model self-corrects in one turn. Source:
harness robustness policy for the tool bridge (the loop's purpose is to
execute model intent; the schema is a contract for well-formed calls, not a
reason to discard legible ones). Not claimed: any spec guarantee that models
emit exact keys — this is a bridge-tolerance case.

## Proposed mechanism (one)

Alias parse in `executeProviderBashTool` only: `cmd` (schema key, primary),
`command` (the alias observed on this route — no speculative additions);
when neither is present, the error echoes the received top-level keys
(`"Bash input requires a non-empty command (cmd); received keys: [...]"`).
The tool definition's schema stays `cmd`; the pragma loop mode's Bash
executor and every other tool stay untouched (not evidenced).

## Executable assertion (gate)

`go test ./internal/query -run TestProviderBashToolAcceptsCommandAlias -count=1`
must fail on `36ac918` (baseline RED: `{"command": ...}` → IsError result,
no execution) and pass on the candidate (execution result with the command's
output). A companion assertion pins the diagnosable error shape
(`TestProviderBashToolEmptyInputErrorListsReceivedKeys`) and the schema-key
path stays green (`{"cmd": ...}` executes on both baseline and candidate —
adjacent-behavior check).

## Evidence that would refute or reshape

- The `command` alias correlating with a specific harmful semantic (e.g.
  models intending a different tool by that name) — none observed; the
  emitted values are ordinary shell commands.
- Other aliases appearing (would extend the alias set — each extension
  needs its own observed evidence, not speculation).
- The route's tool-call emission being fixed model-side (then this is a
  belt-and-suspenders change; the error-shape improvement alone still
  carries the diagnosability claim).

## Claim boundary

Execution-through-alias and error diagnosability are mechanism claims gated
by the recorded regression through the production executor. No task-success
claim. The historical TB instance is provenance of recurrence, not a claim
that the alias cost a specific score.
