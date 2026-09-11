# REG-001 — Active-session registry never garbage-collects orphaned records

- Case ID: `REG-001`
- Source revision: `9d82c3d` (2026-09-11)
- Source observation: `~/.pragma/active-sessions/-1.json` — a registry record
  written 2026-04-13 (gogent era) with `pid: -1`, `pgid: -1`, empty
  session id, status `starting`, for the prompt `Write a haiku about Go
  programming` — has survived five months.
- Observed behavior:
  1. The record is invisible to every current read path: `ListProcesses`
     filters filenames against `^\d+\.json$`, so `-1.json` is never read.
  2. The record is unreachable by every cleanup path: stale records are
     removed only by `validateControlRecord` when a specific PID is being
     controlled, which first requires a recognized record. Files that do
     not match the pattern are never removed by anything.
  3. `Register` does not validate the PID, so the same writer bug can
     recreate `-1.json` today.
  4. Latent variant: a pattern-matching `<pid>.json` whose process died
     without `Unregister` is skipped by `recordActive` forever — filtered
     from every listing and never deleted.
- Expected behavior: the registry is self-cleaning at its natural read
  point. `ListProcesses` removes records that provably belong to the
  registry and are dead or invalid: non-PID-named files that parse as
  `ProcessInfo` with `PID <= 0`, and PID-named records whose heartbeat is
  stale and whose process is no longer alive. `Register` refuses
  `PID <= 0`. Files that do not parse as `ProcessInfo` are left untouched
  (conservative posture per the `pidFilePattern` comment, TS bug #34210:
  never delete data that cannot be proven ours).
- Contract source: registry semantics (active-sessions = live background
  processes); `validateControlRecord` already encodes that dead/stale
  records are removable "without killing"; the observed five-month-old
  artifact proves no sweep exists.
- Executable assertion:
  `go test ./internal/background -count=1` (the package's first tests):
  `TestRegisterRejectsInvalidPID`, `TestListProcessesSweepsOrphanedRecords`
  (sweeps `-1`-style and dead-PID records, preserves a live record),
  `TestListProcessesKeepsAliveButStaleHeartbeat` (conservative keep),
  `TestListProcessesLeavesUnrecognizedFiles` (safety).
- Authentic end-to-end gate: back up `~/.pragma/active-sessions/-1.json`,
  run `./bin/pragma sessions`, verify the file is swept from the real
  registry directory.
- Proposed mechanism: PID validation on write; bounded sweep on read.
- Refuting evidence: if the sweep removes a live process's record or any
  file that is not a registry-owned record, the fix is wrong.
- Claim boundary: local registry behavior only.

## Verification record

- Baseline (revision `9d82c3d`, 2026-09-11): the four new tests are the
  package's first. RED:
  - `TestRegisterRejectsInvalidPID` — `Register` accepted `PID: -1` and
    wrote `-1.json`.
  - `TestListProcessesSweepsOrphanedRecords` — neither the `-1`-style
    orphan nor the dead-process record was swept.
  Safety tests (`KeepsAliveButStaleHeartbeat`, `LeavesUnrecognizedFiles`)
  passed trivially on the baseline (current code never deletes anything).
- Candidate (`Register` refuses `PID <= 0`; `ListProcesses` sweeps only
  files that carry a `pid` field — lenient unmarshaling of arbitrary JSON
  objects into empty `ProcessInfo` is not proof of ownership — and removes
  non-PID-named records with `PID <= 0` plus PID-named records whose
  process is dead and heartbeat is stale):
  - All four tests pass, including the corrected safety contract: a
    non-registry JSON object without a `pid` field (`notes.json`) and a
    non-JSON PID-named file (`99999.json`) are left untouched. The first
    candidate draft swept the pid-less JSON object; the probe (`PID *int`)
    was introduced because lenient unmarshaling made the draft refuting
    evidence real — the safety test caught the overreach.
  - `go test ./internal/background -count=1` passes;
    `go test ./... -count=1` exits 0, 28 packages (background previously
    had no tests).
- Authentic end-to-end gate: `~/.pragma/active-sessions/-1.json` backed up,
  `./bin/pragma sessions` run against the real registry — "No active
  background sessions." — and the five-month-old orphan was swept from the
  directory. Registry dir is now empty.
- Claim boundary held: local registry behavior only.

## Artifact (authentic baseline, preserved)

```json
{"pid":-1,"pgid":-1,"session_id":"","cwd":"/Users/artpar/workspace/code/gogent","started_at":"2026-04-13T23:36:15.664943+05:30","updated_at":"2026-04-13T23:36:15.664943+05:30","status":"starting","log_path":"/Users/artpar/.gogent/logs/bg-2026-04-13T23-36-15.log","model":"gemini-2.5-flash","provider":"google","prompt":"Write a haiku about Go programming"}
```
