# What This Clean-Code Stabilization Goal Taught Me

Working through the clean-code stabilization goal was less like fixing a list of
bugs and more like auditing a system of promises. The objective was not simply
to make Pragma behave correctly on the visible path. It was to make each
important invariant owned by the right source-level boundary, then verify that
callers could no longer bypass that owner. That distinction shaped the entire
experience.

The most important discipline was reading the governing document first. The
clean architecture doctrine made one thing clear: package names, comments, and
intent are not enforcement. A component owns an invariant only when mutation,
cleanup, persistence, replay, resume, and secondary entrypoints all have to go
through it. That standard made many tempting quick fixes unacceptable. It was
not enough to add a guard in the web layer, normalize data in a CLI command, or
teach a prompt to remember a rule. The fix had to move the rule to the owner.

That changed how each issue had to be approached. For every item in
`issues-goal.md`, the first useful question was not "where is the bug?" but
"which invariant is split?" Session persistence, capability scope, prompt
acceptance, permissions, hooks, background process control, tool-result
artifacts, compaction, and raw HTTP capture all had different symptoms, but the
same review lens applied: find the mutation or lifecycle boundary, then make
every relevant path depend on it.

The goal also exposed how easily architecture drift happens in interactive
systems. Pragma has CLI, TUI, web, background, replay, subagent, lifecycle, and
provider paths. Each path is useful, and each path has local pressure to solve
its own immediate problem. Over time, that can produce several small owners for
one invariant. A UI stores prompt state before the runtime accepts it. A replay
command reads artifacts with a weaker contract than the writer intended. A
subagent uses setup-time state after the parent runtime has changed workdir. A
background registry treats a stale PID file as authority. None of these splits
look catastrophic in isolation, but together they make the system harder to
reason about.

The strongest fixes were the ones that removed ambiguity. Runtime prompt
admission became the owner of accepted interactive prompts. Session checkpointing
became tied to conversation mutation instead of UI event consumption. Permission
checks learned typed path semantics instead of guessing from raw strings. The
MCP manager became the owner of reconnect and refreshed capability state. Raw
HTTP capture gained a shared evidence reader that decides whether a response is
complete and intact. In each case, the point was not to add another layer of
defensive code. It was to make the correct path the only dependable path.

The no-tests constraint made verification more deliberate. Normally, focused
tests would be the obvious way to lock many of these boundaries. Here, no tests
were to be added or run, so the verification had to be source-backed and
command-backed without pretending to be stronger than it was. The recurring
verification pattern was `go build`, scoped `go vet`, `git diff --check`, and
source scans for bypass paths, duplicate owners, and stale call sites. That is
not a full substitute for tests, but it can still prove important facts when the
scope is explicit. A scan for removed functions, direct reconnect calls, stale
setup-time workdir references, or raw artifact readers can demonstrate whether a
boundary has actually moved.

Committing after each issue also mattered. It forced each fix to be coherent on
its own. A commit had to contain a bounded source change, an updated issue
ledger entry, and verification evidence. That cadence reduced the risk of
smearing several architectural concerns into one vague batch. It also made later
audit work easier: when issue 83's implementation was correct but its status
block was accidentally placed under issue 1, the commit history made the source
truth clear. The final correction was a ledger fix, not a source rewrite.

The most delicate part of the work was respecting existing user and worktree
state. There were scratch screenshots and logs in the tree, and they were left
alone. There were known stale TUI tests that still referenced removed state, so
verification avoided broad commands that would conflate an older test mismatch
with the current issue. That kind of restraint is part of clean engineering too.
The goal was not to make the repository look tidy by erasing unrelated state.
The goal was to move the architecture toward stronger ownership.

Another lesson was that documentation can be an active artifact, not a passive
summary. `issues-goal.md` was the tracking contract. Each closed issue needed
current status, owner evidence, and verification evidence. The final completion
audit did not rely on memory of the work. It counted the numbered issues,
checked that each had a status and verification entry, scanned for unresolved
status markers, confirmed the last build and vet checks, and inspected the
working tree. That audit found the misplaced issue 83 status block, which is a
good reminder that completion has to be proven from current artifacts, not from
confidence.

The overall experience reinforced a simple rule: clean architecture is not a
style preference. It is operational reliability. If the same invariant is owned
by a UI, a CLI command, a background worker, and a replay tool, the system will
eventually disagree with itself. If persistence is inferred from events,
capability scope from startup state, or response evidence from file presence,
then behavior depends on timing and luck. Moving ownership into one enforceable
source path makes the software less surprising.

By the end, the useful measure of progress was not the number of edits or the
number of green commands. It was the shrinking number of places where callers
could make their own version of the truth. The goal succeeded when the ledger
showed every issue closed with evidence, the latest commands passed, and the
remaining worktree noise was unrelated scratch state. The codebase was not made
perfect, but the targeted architecture defects were resolved in the way the
doctrine required: owner first, source enforcement second, verification last.
