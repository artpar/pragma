# OBS-001/002/003 — Event-recording instruments cannot load current or historical artifacts

## OBS-001 — LoadEvents aborts on unregistered event kinds

- Case ID: `OBS-001`
- Source revision: `27f2586` (2026-09-11)
- Source artifact: `pragma-recording.jsonl` at repo root (2026-05-19, 14MB, 4352 lines;
  first unregistered kind at line 220) and
  `~/.pragma/sessions/ff950c55-490d-4ee3-a96a-5052a8c7507c.jsonl` (2026-04-20, line 1).
- Observed behavior: `./pragma metrics pragma-recording.jsonl` fails with
  `error: load events from "pragma-recording.jsonl": line 220: unmarshal event: unknown event kind: "AgentMDNotFound"`.
  Metrics on the session-store file fails with `line 1: unmarshal event: unknown event kind: "header"`.
  No aggregated metrics are produced even though the file contains thousands of
  events with currently-registered kinds.
- Expected behavior: the event-recording instruments (metrics, replay, audit) load
  any recording written by any harness revision, skipping events whose kinds are no
  longer registered, with a warning stating the skip count and first skipped kind.
  Unregistered kinds contribute nothing to current metrics/replay by construction,
  so skipping them cannot alter results for registered events.
- Contract source: `agent.md` — "Keep fixtures durable, not solely under /tmp";
  historical recordings are the regression fixtures of the replay methodology and
  must remain loadable across event-vocabulary drift.
- Executable assertion: `./bin/pragma metrics pragma-recording.jsonl` exits 0 and
  prints aggregated metrics with a stderr warning for skipped kinds;
  `go test ./internal/observe -count=1` includes
  `TestLoadEventsSkipsUnregisteredKinds` and `TestUnmarshalEventUnknownKindIsTyped`.
- Proposed mechanism: `UnmarshalEvent` returns a typed `UnknownEventKindError`;
  `LoadEvents` skips those lines (warning to stderr) instead of aborting. Malformed
  JSON remains fatal.
- Refuting evidence: if skipping unregistered kinds changes metrics for registered
  events, or if `pragma replay --deterministic` output changes for a recording
  that contains no unregistered kinds, the fix is wrong.
- Claim boundary: local loading and aggregation behavior only. No claim about
  provider behavior or task scores.

## OBS-002 — Recording directories rejected because events.jsonl is hardcoded

- Case ID: `OBS-002`
- Source revision: `27f2586` (2026-09-11)
- Source artifact: `~/.pragma/recordings/4fcfb9ee-b7aa-4ec3-99b5-917d76cf67aa/`
  containing exactly one recording file `20260911T075358.350980000Z.jsonl`
  (7 events, written 2026-09-11T07:53:58Z by the current recorder,
  `internal/cli/run.go:1775`).
- Observed behavior: `./pragma replay --events ~/.pragma/recordings/4fcfb9ee-b7aa-4ec3-99b5-917d76cf67aa`
  fails with `load events: stat .../events.jsonl: no such file or directory`.
  The same failure applies to `./pragma metrics` and `./pragma audit` on that
  directory. The commands work only when passed the recording file path directly,
  but the CLI help advertises directories.
- Expected behavior: a directory containing exactly one `*.jsonl` event file loads
  as that file. `events.jsonl` remains canonical when present. A directory with
  multiple `*.jsonl` files and no `events.jsonl` fails with a list of candidates
  rather than a guess.
- Contract source: CLI help of `metrics`/`audit` ("A replay directory containing
  events.jsonl") and `replay` usage (`pragma replay <session-dir>`); recorder layout
  writes `~/.pragma/recordings/<sessionID>/<timestamp>.jsonl`.
- Executable assertion: `./bin/pragma replay --events ~/.pragma/recordings/4fcfb9ee-b7aa-4ec3-99b5-917d76cf67aa`
  exits 0 and prints the 7 recorded events;
  `go test ./internal/observe -count=1` includes
  `TestLoadEventsDirectorySingleFileFallback` and
  `TestLoadEventsDirectoryMultipleFilesAmbiguous`.
- Proposed mechanism: directory path resolution prefers `events.jsonl`, falls back
  to a lone `*.jsonl`, and reports multiple candidates as an error.
- Refuting evidence: a directory that contains `events.jsonl` resolving to a
  different file; recorder fixtures under `internal/observe` changing behavior.
- Claim boundary: local path resolution only.

## OBS-003 — Session-store files produce opaque errors with no guidance

- Case ID: `OBS-003`
- Source revision: `27f2586` (2026-09-11)
- Source artifact: any of the 383 session-store files under `~/.pragma/sessions/`
  (format: `header`/`message`/`metadata` entries written by `internal/session`).
- Observed behavior: `./pragma metrics ~/.pragma/sessions/<id>.jsonl` fails with
  `unknown event kind: "header"` — the error does not say that the path is a
  session store, that metrics expects event recordings, or where those live.
- Expected behavior: pointing metrics at a file with zero recognized events
  prints an explicit "no recognized events" message with format guidance
  (session-store files vs event recordings under `~/.pragma/recordings/`).
- Contract source: CLI help text says "A .jsonl file" without qualifying the
  family; ergonomics requirement for fast feedback loops (visibility instruments
  must not mislead).
- Executable assertion: `./bin/pragma metrics ~/.pragma/sessions/ff950c55-490d-4ee3-a96a-5052a8c7507c.jsonl`
  exits 0 with a "No events found." line plus a hint naming both formats.
- Proposed mechanism: zero-events branch of `metrics` prints guidance.
- Refuting evidence: none local; the change is display-only.
- Claim boundary: message text only; metrics values are unchanged.

## Verification record

- Baseline (revision `27f2586`, 2026-09-11, observed pre-change):
  - `./pragma metrics pragma-recording.jsonl` → exit 1, `line 220: unmarshal event: unknown event kind: "AgentMDNotFound"`.
  - `./pragma replay --events ~/.pragma/recordings/4fcfb9ee-b7aa-4ec3-99b5-917d76cf67aa` → exit 1, `stat .../events.jsonl: no such file or directory`.
  - `./pragma metrics ~/.pragma/sessions/ff950c55-490d-4ee3-a96a-5052a8c7507c.jsonl` → exit 1, `line 1: unmarshal event: unknown event kind: "header"`.
- Candidate (typed `UnknownEventKindError`; `LoadEvents` skips unregistered kinds with
  a stderr warning; directory resolution prefers `events.jsonl`, falls back to a lone
  `*.jsonl`, reports multiple candidates; metrics zero-events guidance):
  - OBS-001: `./bin/pragma metrics pragma-recording.jsonl` → exit 0; 71,164 events
    processed, 15 API calls, 126,039 input tokens, tool breakdown (Read 37, Glob 17,
    Grep 8); stderr warning `skipped 5 event(s) with unregistered kinds (first: "AgentMDNotFound")`.
  - OBS-002: `./bin/pragma replay --events <recording-dir>` → exit 0, prints the 7
    recorded events; `./bin/pragma metrics <recording-dir>` and
    `./bin/pragma audit <recording-dir>` exit 0.
  - OBS-003: `./bin/pragma metrics <session-store-file>` → exit 0, "No events found."
    plus the two-line format guidance.
- Adjacent checks: `replay --events` on the recording file unchanged (7 events);
  `replay --deterministic` on the failed-session recording returns the pre-existing
  `recording has no recorded API responses` (confirmed identical on the 2026-06-21
  binary — expected, that session recorded no successful responses);
  `go test ./internal/observe -count=1` passes including the five new tests
  (`TestLoadEventsSkipsUnregisteredKinds`, `TestUnmarshalEventUnknownKindIsTyped`,
  `TestLoadEventsDirectorySingleFileFallback`,
  `TestLoadEventsDirectoryPrefersCanonicalEventsFile`,
  `TestLoadEventsDirectoryMultipleFilesAmbiguous`);
  `go test ./... -count=1` exits 0 (18s, 26 packages ok).
- Claim boundary held: local loading/aggregation/path-resolution behavior only.

## Follow-up observations (not fixed here, one mechanism per change)

1. **Flaky gate under parallel load.** During one full parallel
   `go test ./... -count=1` execution, `internal/provider/openrouter`
   `TestRecordedReasoningSurvivesSessionReplay` failed with
   `requests = 12, want 2` after 183s of retries. It passes in isolation on the
   baseline (0.53s), in isolation with these changes (0.42s), and in a subsequent
   full-suite run (3.6s). Suspected mechanism: local httptest latency under
   package-level parallelism triggers the bounded retry policy, inflating the
   request count past the assertion. Unconfirmed until reproduced; recorded because
   a flaky gate undermines every claim the loop produces.
2. **Captured live failure awaiting classification.** The recording
   `~/.pragma/recordings/4fcfb9ee-b7aa-4ec3-99b5-917d76cf67aa/20260911T075358.350980000Z.jsonl`
   captures a session start that failed ~1.1s after `APIRequestStarted`
   (`APIRequestFailed error=request_failed retryable=false`), followed by
   `MCPServerDisconnected` and `MCPServerFailed`. Whether the retryable
   classification was correct is unresolved; related recent commits
   (`60bc3eb` retry authorization, `a20443c` bounded retry acceptance) suggest
   the retryability decision path is an active work area.
3. **Stale active-session registry entry.** `~/.pragma/active-sessions/-1.json`
   (2026-04-13, gogent-era background session, `pid: -1`, empty session id,
   status `starting`) is never expired. Hygiene candidate for the registry.
