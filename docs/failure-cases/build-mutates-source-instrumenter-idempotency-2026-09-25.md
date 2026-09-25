# INST-001 — `make build` mutates the source tree: non-idempotent instrumenter re-adds truncated duplicate traces and deletes rationale comments

- Case ID: `INST-001`
- Source revision: pre-fix instrumenter `cmd/pragma-instrument/main.go` at
  `1a9fbdc^..1a9fbdc` lineage (unchanged since the GOGENT-34 era,
  977769e); defect observed against HEAD `a229500..HEAD` working tree,
  2026-09-24 21:28 → 2026-09-25 10:54 build clusters.
- Fix commits: `4342198` (message canonicalization + truncation-aware
  dedup; comment preservation via kept `file.Comments` + anchored
  injections), `c113588` (empty-clause anchor past in-clause comments),
  `3f2c585` (tree reconciliation), inventory closure commit (fork-test
  salvage, reader records, go.mod.bak removal).

## Observed (authentic, recorded)

`make build` depends on `instrument` (`go run ./cmd/pragma-instrument/
./internal/...`), which the orchestrator runs every cycle (~15 min), and
the Makefile comment claims "Re-run AST instrumentation (idempotent)".
The working tree accumulated exactly the mutations the instrumenter
itself produces:

- 11 modified internal files, mtimes clustering at the last three
  build times (8 files 2026-09-24 21:28:47, 2 files 2026-09-25
  10:26:10, 1 file 10:54:44) — `internal/cli/run.go`,
  `internal/compact/{auto,compact}.go`,
  `internal/metaobserve/{critic,digest,observer}.go`,
  `internal/query/miniswe_loop.go`,
  `internal/tools/applypatch/applypatch.go`,
  `internal/watcher/{discover,postmortem,session}.go`.
- Re-added truncated duplicate trace lines, one per return whose
  committed message did not byte-equal the newly computed one:
  `internal/compact/auto.go` 27 → 28 GlobalTrace lines (e.g.
  `observe.GlobalTrace("return: !t.disabled &&\n\tt.consecutiveFailures < MaxConsecutiveFailures &&\n\t!(t.compac...")`
  inserted beside the committed full-form line);
  `internal/tools/applypatch/applypatch.go` +4; `internal/cli/run.go`
  +4 (two truncated duplicates, plus an `else:` trace and one return
  trace absent at HEAD).
- Deleted in-body block comments — the failure-case citations — from
  every written file: CMP-001.4 F8 zero-window-disable rationale
  (auto.go, 10 lines), CMP-001.2 F2/F5, CMP-001.4 F8 wiring, INT-001
  queued-input (run.go), CMP-001.2 F4 (compact.go), CMP-001.3/F2/F3/F6
  (miniswe_loop.go), CMP-001.4a/F6-era comments (digest.go, observer.go,
  session.go: `// the attempt counts against the budget`,
  `// drop partial first line` trailing comments too). run.go 88 → 39
  `//` lines, auto.go 39 → 29 in the dirty tree; HEAD still carried all
  of them.

Consequences (verified in the payload, re-verified here): any worker
that stage-alls these files silently erases failure-case citations from
history; every `git status` shows ~12 phantom-dirty files (distraction
+ false-positive critic findings).

## Expected behavior and its source

The Makefile's own comment ("Re-run AST instrumentation (idempotent)")
is the contract; a re-run over already-instrumented source must be a
byte-identical no-op, and instrumentation must not delete comments it was
never asked to touch.

## Responsible production path and earliest wrong transitions

`cmd/pragma-instrument/main.go`:

1. **Dedup** (`isTraceCallWithMsg`, used only for return-trace
   idempotency) required exact `lit.Value == %q(msg)`. But
   `exprString` truncated at 80 **bytes** (`s[:77]+"..."`, able to
   split multi-byte runes) and `printer.Fprint` renders a multi-line
   source expression with raw `\n`/`\t`, so the computed message for
   such returns is a truncated, newline-bearing string that can never
   byte-equal the committed full-form single-space message → one
   duplicate truncated line inserted per run, then self-stable (the
   next run matches its own truncated form — hence exactly one
   duplicate per site, not one per build).
2. **Comments** (`processFile`): `file.Comments = nil` before
   `format.Node`, dropping every in-body comment in any file the
   instrumenter writes (declaration Doc comments survive via
   go/printer's node-attached printing; in-body comments live only in
   `file.Comments`).

## Reproduction and gates (all in-repo, no /tmp dependency)

`go test ./cmd/pragma-instrument/ -count=1` (RED before the fix for
exactly the two reasons above; RED log quoted below):

- `TestAlreadyInstrumentedFileIsFixedPoint` — a HEAD-style
  already-instrumented fixture (full-form return trace, rationale
  comments) must be a byte-identical fixed point.
- `TestInstrumenterIdempotentAndCommentPreserving` — a fresh file gets
  traces while every block comment above if/for/return statements
  survives above its statement (including a comment-only case clause,
  the digest.go mangling shape); a second run is byte-identical.

Real-production verification (stronger than fixtures):

- A scratch copy of HEAD `internal/compact/auto.go`: fixed instrumenter
  writes nothing (40/40 branch points recognized) — byte-identical.
- Full tree reconciliation (HEAD + fixed instrumenter, 354/8472 points,
  9 files): insertions only, comment counts equal HEAD per file
  (88/44/14/36/31/104/17/15/65), zero deletions.
- Second run over the reconciled tree: `0/8472 branch points across 0
  files`, `git status` shows no change; `make build` after commit
  `3f2c585`: "Instrumented 0/8472 branch points across 0 files".
- `go build ./...` clean; `internal/{compact,cli,metaobserve,watcher}`
  + `internal/tools/applypatch` + `internal/query` suites green; the 9
  written files gofmt-clean.

### RED log (pre-fix, key lines; full log 2026-09-25 11:47)

```
--- FAIL: TestAlreadyInstrumentedFileIsFixedPoint
    instrumenter mutated an already-instrumented file (must be a byte-identical fixed point)
    [... observe.GlobalTrace("return: !disabled &&\n\tfailures < compactMaxConsecutiveFailures &&\n\t!(turnsSince < min...")  ← duplicate truncated line]
--- FAIL: TestInstrumenterIdempotentAndCommentPreserving
    first run dropped the block comment "// rationale-if: the disabled short-circuit."
    first run dropped the block comment "// rationale-for: the cooldown drain loop."
    first run dropped the block comment "// rationale-return: the multi-line conjunction"
```

## Mechanism changes (one per defect)

1. Message identity: `exprString`/`exprListString`/`stmtString` now
   emit a canonical single-space form (`strings.Fields` join), truncated
   **rune-safely** at 80 runes (never splitting a multi-byte rune like
   `—`); dedup (`isTraceCallWithMsg` → `traceMsgMatches`) unquotes the
   existing literal, canonicalizes both sides, and matches exact or
   either direction of the `"..."`-truncation prefix — a committed
   full-form message is recognized, not re-instrumented.
2. Comment preservation: keep `file.Comments`; anchor every injected
   statement to a real position (`makeTraceStmtAt` wired in — it existed
   as dead code; now covers entry/exit, if/else/else-if, case, for,
   range, select, return; `setExprPos` also positions the selector's
   `Sel` and all call args) so go/format keeps each in-body comment
   above the statement it documents.
3. `c113588`: empty clause bodies (comments only) anchor their trace to
   the NEXT clause (or switch/select closing brace). First
   reconciliation dry-run caught the missing case on real production
   input: digest.go's comment-only
   `case "MCPHealthCheck", "SessionSaved":` rendered
   `observe.` <comment> `GlobalTrace(...)` — valid Go, so build and
   gofmt would NOT have caught it; found by diff review and proven by
   replaying the fixture shape against `4342198` in a detached
   worktree (same mangling).

## Payload claims: verified vs corrected

- Verified: 12 modified paths (11 internal + orchestrator's queue file)
  with build-time mtime clusters (8+2+1 across the three build times);
  duplicate truncated traces re-added per run (auto.go 27→28 exactly);
  rationale block comments deleted from the working tree while HEAD
  carries all of them; `make build` runs every cycle; Makefile idempotency
  claim false pre-fix; the dedup root cause (must recognize the
  truncated form it emits) and the comment root cause (statement
  rewrite path drops attached comments) — both confirmed, with the
  comment path being `file.Comments = nil`, not node replacement.
- Corrected (label): the truncated form is 80 bytes (77+`"..."`), not
  "120-char" — 120 approximates the rendered `%q`-escaped literal
  length; substance unaffected.
- Corrected (numbers): run.go counts given as 584 vs 588 did not
  reproduce under two counting methods (call sites 503→507 including
  TraceCtx; `observe.` prefix 597→601 in the dirty tree). The operative
  delta (+4) is confirmed; the absolute figures are from the
  supervisor's method/state and not reproduced here.
- go.mod.bak verified byte-identical to go.mod (Rosetta-era leftover,
  deleted); content_replacement_fork_test.go verified green and salvaged
  (was a229500's never-staged gate).

## Claim boundary and residual risk

- The residual known-benign noise: hand-abbreviated trace messages
  (e.g. `return: fmt.Errorf("no API key for provider")` in
  `switchProviderModel`) are not mechanically recognizable as covering
  the same return, so the reconciled tree carries the abbreviated line
  plus one canonical truncated line at those two sites — insert-only,
  byte-stable across runs. Deleting them would require replace-semantics
  the instrumenter deliberately does not have.
- A `traceMsgMatches` prefix pair sharing the first 77 runes would at
  worst SKIP a trace (never duplicate); only reachable for two
  consecutive returns with >80-rune identical prefixes.
- Pre-existing gofmt-dirty files at HEAD (buildinfo, observe/bus,
  model/stop, several *_test.go — flagged by `gofmt -l ./internal/`)
  are NOT instrumenter output and are untouched here; separate hygiene
  work.
- Follow-up item for the queue (not done — the tree is no longer
  dirty): instrument to a temp overlay instead of in-tree so builds
  never touch source at all, removing the fixed-point requirement.
