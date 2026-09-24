# META-OBS — Value-level waste is invisible to every live observer

- Case ID: `META-OBS` (queue item `META-observe`)
- Spec source (operator directive, 2026-09-24 16:14): "maybe we just
  dont have enough operational observability we will keep missing things,
  what about one watch for each of these every which check every couple of
  minutes... efficiency always requires meta observability"
- Source observation: the CLK-002 wall-clock stamp-noise was a value-level
  defect (a design choice that wasted conversation and operator attention)
  that ran live for days; every mechanical observer missed it; only the
  operator caught it. No pragma-owned observer judges *what a session is
  doing* — only whether countable invariants (identical tool sets, retry
  counts, context %, stalls, MCP state) are violated.
- Source revision: `97d3dfe` (2026-09-24).

## Payload claims verified against the code before acting

- "The wall-clock noise (CLK-002) was a value-level defect invisible to
  every current observer — only the operator caught it." **Verified with a
  boundary.** The watcher counts `StampishUserMsgs`
  (internal/watcher/session.go — user text-only messages with token
  estimate 8–25, which the CLK-001 companion stamp matched), but that
  counter is (a) never turned into an `Alert` — no alert kind exists for
  it — and (b) never rendered in the live dashboard or `sessionView` at
  all; it appears only in `PostMortem` JSON written after a session dies.
  So during a live session the defect was invisible to every operator-
  facing surface; it surfaced only post-mortem, as a raw number with no
  judgment attached. "Only the operator caught it" held live.
- "Mechanical invariants (watcher alerts) cover countable failure; this
  adds the judging layer." **Verified.** Every alert kind in the watcher
  is a countable threshold: `tool_loop` (3 identical tool sets), `context`
  (80/95%), `retry_storm` (5 retries), `max_tokens`, `api_failed`,
  `mcp_down`, `error`, stall timing. None of them judge value: re-reads,
  redundant re-derivations, low-value turns, operator corrections that
  never became queue items.
- "A daemon ... every ~3 minutes takes the tail of each live session
  event log" — **infrastructure verified.** Live event logs exist at
  `~/.pragma/logs/<timestamp>.jsonl` (Info-level JSONL: `MessageAppended`
  with role/types/token-estimate, `APIRequestCompleted` carrying full
  response content — thinking text and tool calls with inputs — plus
  usage/duration, `SessionSaved` carrying the session UUID, failures,
  compaction, MCP state). `watcher.ActiveLogFiles` + `watcher.ParseLogName`
  + `watcher.MatchProcessToLog` + `watcher.FindPragmaProcesses` give the
  live-session set (and exclude `pragma-watch` itself via
  `IsPragmaCommand`).
- "runs a CHEAP critic prompt over it" — **route verified.**
  `cli.CreateProvider(config.Config{Provider, APIKey}, bus)` plus
  `config.LoadCredentials()` (`~/.pragma/credentials.yml`) construct the
  same production providers pragma sessions use; `provider.Complete` with
  one small system+user request is the cheap call.
- Operator message text availability — **verified and constrained.**
  `MessageAppended`/`UserTurnAccepted` events carry metadata only (char
  counts, token estimates), and `APIRequestStarted` (which carries full
  request messages) is Debug-level, filtered out of default logs. Operator
  text IS durably present in the session files
  `~/.pragma/sessions/<uuid>.jsonl` (`message` lines with full content),
  and the event log links to them via `SessionSaved.session_id` — so the
  meta-observer joins the event-log tail to the session-file tail by that
  UUID.
- "Findings auto-append to this queue with session+time provenance" —
  **verified** the queue (docs/self-improvement-queue.jsonl) is
  append-only JSONL; findings are appended as
  `kind:"meta-finding"` entries carrying a provenance payload.
- Budget claim ("one small model call per sample, ~$0.5-2/hour at 3min
  cadence over 1-2 live sessions") — arithmetic-consistent (20 samples/h
  × $0.02–0.10); implemented with a mechanical bound
  (`--critic-max-calls`, rolling hour) plus skip rules (no new bytes → no
  call; no live process → no call).
- Correlated-blind-spot caveat — **accepted as a design constraint**, not
  refuted: the critic defaults to the active route's model and therefore
  shares its blind spots; `--critic-provider`/`--critic-model` make
  rotation possible, and every finding records which critic model
  produced it.
- Refuted claims: none.

## Earliest wrong transition

Not a code bug but a coverage gap: pragma has no observer that applies
judgment (a model call) to what a live session is doing. The watcher's
alert kinds are all countable; value-level waste enters the record only
when a human notices.

## Expected behavior (the mechanism this case pins)

A meta-observer daemon (new `--meta` mode of pragma-watch, mirroring the
existing recording-daemon pattern) that every `--critic-every` (default 3m)
samples each *live* pragma session (event-log tail joined to session-file
tail via `SessionSaved.session_id`), runs one cheap critic model call
asking the three operator questions —
(1) what here is wasted motion (re-reads, stamp-noise, low-value turns,
context growth per unit of progress)? (2) what is this session NOT
checking that it should? (3) do recent operator messages read as spec
corrections that are not yet queue items?
— and appends findings to the self-improvement queue (`--queue`), with a
durable copy under the observations dir, each finding carrying session +
time provenance and the critic model that produced it. Mechanical bounds:
no call without new log bytes; no call for ended sessions; a rolling
per-hour call cap; per-session finding dedup; a queue-append cap.

## Evidence that would refute it

Any live session whose value-level waste the critic finds but the
mechanical watcher also alerted on would weaken the "beyond mechanical
invariants" claim; a live hour where the critic finds nothing the
mechanical watcher missed would refute the need claim; provider rejection
of the small critic request shape would refute the route.

## Gates

- RED (mechanism absent at `97d3dfe`): the package tests below fail for
  the stated reason — no meta-observer mechanism exists anywhere in the
  tree (package `internal/metaobserve` does not exist; no pragma-watch
  mode makes critic calls; nothing appends `meta-finding` queue entries).
- GREEN: all gates pass; `go test ./internal/metaobserve/ ./internal/watcher/`
  clean; `go build ./...` clean; gofmt clean on touched files.
- Live: one live hour of the detached daemon recording findings over live
  sessions with the real critic route, compared against the mechanical
  watcher's `alerts.jsonl` over the same window.

(Filled in as the work proceeds — RED/GREEN/Live observations below.)

## Revision 2 (WORKER session, 2026-09-24 17:31) — what was verified,
## refuted, changed

The implementation arrived in the working tree (uncommitted) from the
prior session: package `internal/metaobserve` (observer/critic/digest +
tests), `cmd/pragma-watch/meta.go`, meta flags in `main.go`. It was
**inert**: the package did not compile, and even compiled it was never
dispatched. Two defects, each one mechanism-level change, RED→GREEN:

### Defect A — build break: `config` identifier collision (RED→GREEN)

RED (verbatim, `go build ./...` before the fix):

    cmd/pragma-watch/main.go:36:6: config already declared through import of package config ("github.com/artpar/pragma/internal/config")
        cmd/pragma-watch/meta.go:12:2: other declaration of config
    cmd/pragma-watch/meta.go:19:22: config is not a type
    cmd/pragma-watch/meta.go:44:30: config is not a type
    cmd/pragma-watch/meta.go:73:19: config is not a type
    cmd/pragma-watch/meta.go:93:27: config is not a type

`main.go` declares the CLI struct `type config struct` (package scope);
`meta.go` imported `internal/config` (file scope) — collision. One change:
alias the import (`pragmaconfig "…/internal/config"`) and qualify its two
uses (`pragmaconfig.LoadCredentials`, `pragmaconfig.Config`), so `*config`
resolves to the CLI struct. GREEN: `go build ./...` exit 0.

### Defect B — flags registered but never dispatched (RED→GREEN)

The `--meta`/`--meta-daemon`/`--meta-child` flags were registered in
`main()` but the dispatch `switch` had no cases for them, so `--meta` ran
the **dashboard** observer and a spawned `--meta-child` daemon would have
run the dashboard loop detached, never sampling. RED (verbatim, binary
built after Defect A, `--meta` passed):

    pragma-watch: observing /tmp/meta-red-home/logs every 1s (Ctrl-C to stop)

(dashboard banner — wrong mode). One change: dispatch cases in `main()`'s
switch — `metaDaemon` → `startMetaDaemon(cfg, cfg.queuePath)`; `meta`,
`metaChild` → `runMeta(cfg, cfg.queuePath)`. GREEN (verbatim):

    pragma-watch meta-observe: sampling /tmp/meta-red-home/logs every 3m0s (critic morphllm/morph-glm53-744b), findings to /tmp/meta-red-home/observations/meta-findings.jsonl

and the daemon path end-to-end: `--meta-daemon` spawns a detached child
(meta.pid written; child banner in meta.log; `--critic-every 30s`
propagated to the child).

### Refinement on the "CHEAP critic" claim (not refuted, bounded)

Default critic model = `cli.DefaultModelFor("morphllm")` =
`morph-glm53-744b` — the provider's main model, **not** a designated
small model; the morphllm catalog aliases (`glm5.3`/`glm-5.3`) all map
to it, so no cheaper rotation exists in the catalog today. The cheapness
that IS implemented is structural: digest capped at 12 000 chars in,
`MaxTokens: 800` out, one call per sample, no call without new log
bytes, no call for ended sessions, rolling 60-calls/hour cap
(`--critic-max-calls`). Rotation remains possible via
`--critic-model`/`--critic-provider` and every finding records its
critic model. The live hour below therefore ran same-model — the
correlated-blind-spot caveat in force, not mitigated.

### Changed (this revision)

1. `cmd/pragma-watch/meta.go`: import alias fix (Defect A).
2. `cmd/pragma-watch/main.go`: meta dispatch cases (Defect B); gofmt
   alignment of the config struct.
3. `cmd/pragma-watch/meta_test.go` (new): locks the operator-specified
   defaults — 3-minute cadence, 15-minute active window, findings under
   the observations dir, `--critic-max-calls` passthrough, 90s critic
   timeout.

Gates: `go build ./...` clean; `go test ./cmd/pragma-watch/…
./internal/metaobserve/…` → ok / ok; gofmt clean.

## Live hour (2026-09-24, 17:31–18:31 IST) — recorded findings

Daemon: `bin/pragma-watch --meta-daemon --home ~/.pragma --queue
docs/self-improvement-queue.jsonl` (pid 62652), critic
morphllm/morph-glm53-744b, 3-minute cadence, findings to
`~/.pragma/observations/meta-findings.jsonl` + queue appends. The
mechanical recording daemon (pid 43446, running since 16:19) provides
the comparison layer over the same window (`alerts.jsonl`).

First tick (17:31:02, live data): correctly skipped the ended session
`2026-09-24T16-58-58.jsonl` ("no live pragma process" — live-process
gating works against real sessions) and sampled this worker session's
own log (`2026-09-24T17-18-12.jsonl`); one real critic call (~8 s);
0 findings (nothing judged notable beyond the existing queue items on
that window). The observer is, by design, observing this session's own
work — the meta loop applied to the session building it.

(Findings summary + comparison against the mechanical watcher's
alerts for the same window are appended at the end of the hour.)
