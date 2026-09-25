# PACT-001 — apply_patch count-bearing hunk headers silently ignored (preflight gap)

Recorded 2026-09-25. Fix commit: 89522be (RED test + mechanism), this
record follows it. Queue item PACT-001; priority list entry 1 in
docs/analyses/reading/MASTER-BLINDSPOTS-2026-09-24.md (B.1, fix direction
"patch-format preflight in the tool (validate hunk headers before
applying)").

## Observed failure (wire-proven)

Session 729e2564 (2026-09-24), apply_patch call 17, target calc.go, sent:

    *** Update File: calc.go
    @@
    @@ -29,6 +29,18 @@
     ...
     (4 old-side lines, 19 new-side lines)

The tool returned:

    apply_patch verification failed: update hunk for calc.go did not
    match current file content near:
      \treturn total / float64(len(scores))
      }
    <blank>
      func main() {
    Reread the target range, keep unchanged context lines exact, and
    retry with a smaller hunk if needed

The header stated 6 old / 18 new lines; the body carried 4 old / 19 new.
`Parse` discarded everything after "@@ " (internal/tools/applypatch/
applypatch.go at 0f758d6: separator branch), so the header/body
disagreement was invisible. Two failure shapes follow from that:

1. Misleading error: the body's context did not match the file, and the
   tool blamed the file ("Reread the target range...") while the actual
   defect was the model's own header/body mismatch — driving re-read
   loops (18-12-25 calls 32-46: 15 calls, 8 failed hunks).
2. Silent misapplication: when the body's context *did* match, the patch
   applied despite the lying header (reproduced: subtest B at parent
   returns success).

Related recorded instances with count-bearing headers: 7ac8ea2b calls
9/13 (`@@ -1,13 +1,22`, `@@ -1,5 +1,6`), 79dcb05b (three failures),
729e2564 call 17. Cross-session cost: apply_patch friction is the
dominant mechanical waste class of 2026-09-24 (MASTER-BLINDSPOTS B.1);
the supervisor alone hit the patch format 10x (B2).

## Expected behavior

A hunk header that carries unified-diff counts must agree with its hunk
body, and the check must happen at parse time — before any file is read
or matched — so the error names the patch's own defect (stated vs actual
counts) instead of the file's content. Source of contract: queue item
PACT-001 deliverable ("preflight validation in the tool with actionable
error") + blindspot B.1 fix direction.

## Reproduction gate (RED at parent)

- Parent: 0f758d6 in a detached worktree, new test file only:
  `go test ./internal/tools/applypatch/ -run 'TestApplyPatchPreflight' -count=1`
  - subtest A fails with the *verbatim recorded error* (misleading
    content-mismatch text above, byte-for-byte).
  - subtest B fails with "got success" (silent acceptance).
- Changed tree: both subtests pass;
  `TestApplyPatchPreflightAcceptsMatchingHunkHeaderCounts` guards
  against over-rejection (correct counts still apply).
- Adjacent gates after the change: full `./internal/tools/applypatch/`
  and `./internal/query/` packages pass (the apply_patch consumers are
  provider_tools_loop.go:874 and miniswe_loop.go:1800); gofmt clean.

## Mechanism (one change)

`Parse`'s update-hunk loop now routes "@@" separator lines through
`parseHunkHeaderLine`: bare "@@" and non-count content keep separator
semantics; count-shaped lines ("-N[,M] +N[,M]" with optional trailing
"@@ context") are parsed and attached to the following chunk; at flush,
`validateHunkHeaderCounts` compares stated old/new counts to
len(Old)/len(New) and fails the whole patch at parse time. Count-shaped
but malformed lines (e.g. spaces after commas) are rejected with an
actionable message instead of being silently discarded. Error:

    apply_patch verification failed: update hunk for calc.go line 4:
    hunk header "@@ -29,6 +29,18 @@" states 6 old and 18 new lines, but
    the hunk body has 4 old and 19 new lines
    Make the header counts match the hunk body (old = context plus
    removed lines; new = context plus added lines), or drop the counts
    and use a bare @@; header counts are validated before any file is read

Claim boundary: this proves parse-time rejection of header/body count
disagreement for the recorded input class. It does not claim a reduction
in model retry counts (that is a live-behavior claim requiring a bounded
continuation); it removes the misleading-error and silent-misapplication
mechanisms for this class.

## Payload claims verified vs refuted

Verified:
- "hunk-count mismatches" (B.1): confirmed unvalidated at 0f758d6;
  wire-recorded inputs reproduced; fixed.
- "apply_patch friction is the dominant mechanical waste" (B.1): taken
  as recorded evidence (2,725 calls individually read); not re-proven.

Refuted as defects needing a code change (no change made):
- "Empty patches": already handled — tool layer returns "apply_patch
  input requires non-empty patch" (provider_tools_loop.go:871) and
  ApplyPatchText returns "patch contains no file hunks".
- "No-op patches": already handled — verify() rejects "%s produced no
  content change" (applypatch.go, verify loop).
- Supervisor "+drop 10x" (B.2) as a tool-validatable defect: a dropped
  "+" on an update line that begins with a space or tab is *not
  distinguishable* from a legitimate context line at parse time (the
  space-leading form parses as context by design; the tool cannot know
  intent). The tool-side paths that ARE detectable (lines starting with
  neither ' ', '+', '-' nor '@@') already had actionable errors. Root
  cause is model emission behavior; the queue's tool-owned fix
  direction (header/counts preflight) is what was implemented.

Not addressed here (separate mechanisms, deliberately not bundled):
- Add-file blank-line guidance (14-06-42 six-try add: error could say
  "emit blank lines as a lone '+'") — error wording, not preflight.
- Header *line-number* validation (headers carry positions that
  findSubsequence ignores) — would change matching semantics; needs its
  own case if a recorded misposition event is found.

## Revision note

- 2026-09-25 (first revision, this file): case opened from queue item
  PACT-001. Verified the hunk-count class against wire records
  (sessions 729e2564, 7ac8ea2b, 79dcb05b), refuted the empty-patch,
  no-op-patch, and +drop claims as already-handled or not
  preflight-detectable (documented above, nothing changed for them),
  implemented the single count-preflight mechanism, gated RED at parent
  (detached worktree at 0f758d6, then removed) -> GREEN at 89522be.
- Pre-existing, unrelated: cmd/pragma TestProviderToolsCLIContract fails
  at pristine HEAD 0f758d6 in a detached worktree (exercises Bash +
  mock provider, never apply_patch) — not this case, not re-fixed.
- Working meta-observation: while landing this fix the worker dropped
  the leading space on removal lines ("-}" vs "- }") three times in its
  own edit patches, each rejected by the tool's verify-then-apply gate
  with no partial application — the same mechanical class this case
  addresses, and direct evidence that the count-preflight error
  belongs in the tool rather than in more doctrine rules.
