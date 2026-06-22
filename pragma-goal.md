
  Build, test, and iterate a general SWE-bench Pro persona/FSM orchestration system for Pragma, using the Flipt Kubernetes authentication instance only as the first baseline calibration task.

  The goal is to produce a reusable, task-agnostic orchestration/persona framework that can preserve task intent, evidence, validation state, and persona-to-persona handoff fidelity across
  long SWE-bench Pro runs.

  The implementation must not hardcode task, stack, repository, persona, state, artifact, or baseline-specific behavior into generic runtime code. Remove any such hardcoding if you come across something like that.


  Non-negotiable: do not implement any fix until you first present the abstraction boundary and I approve it.

  For any failure, your first response must be identification only:
  1. What invariant failed?
  2. What generic abstraction is missing?
  3. What concrete strings/commands/files/tasks appeared in the failure?
  4. Why a fix using any of those concrete strings would be hardcoding.
  5. What production files would need to change, if any.

  You must not edit code, YAML, prompts, docs-as-policy, tests, fixtures, or runtime behavior in the same turn as failure identification.

  A proposed fix is invalid if it contains:
  - exact command names observed in the failure
  - exact file extensions from the failure
  - exact file paths from the failure
  - provider/model names
  - task/repo names
  - persona/state/artifact names used as special cases
  - regexes or allow/deny lists over observed strings
  - “policy” text that merely restates the observed bad command in generic words

  If the only fix you can think of is a denylist, allowlist, regex, string match, or prompt warning, stop and say: “I do not have a valid abstraction-level fix yet.”

  Do not promote replay evidence unless the tested case changes the abstract contract, not just the observed command shape.

  Before any edit, show the proposed diff conceptually and label every literal string it introduces as one of:
  - domain model required by existing architecture
  - user-facing artifact name already present
  - new hardcoding risk

  If any new hardcoding risk exists, do not edit.

  The important part is: force an identify-only turn before edits.

 ## Methodology

  Use progressive prompt/payload-level AB testing against captured raw replay turns before running full benchmark attempts.

  For each failure or drift point:

  1. Identify the exact raw HTTP turn payload where the persona/FSM contract failed.
  2. Classify whether the failure is caused by persona responsibility, state ordering, transition policy, handoff content, missing artifact contract, runtime support, or provider behavior.
  3. Create focused replay variants that change only the minimum prompt, payload, persona responsibility, state, transition, or declarative contract needed to test a hypothesis.
  4. Compare variants against the same captured turn payload wherever possible, instead of rerunning the full task from scratch.
  5. Prefer AB testing persona sets, state boundaries, transition rules, artifact contracts, and handoff payload shapes progressively.
  6. Promote a variant only when replay evidence shows better preservation of task intent, evidence fidelity, validation discipline, and generality.
  7. Record rejected variants and why they failed.
  8. Run a fresh full benchmark only when you are confident for a flow spanning atleast 3 personas to validate/concretise your findinds


  Use progressive prompt/payload-level AB testing against captured raw replay turns before running full benchmark attempts. Every change must be tied to an exact captured turn, a concrete
  hypothesis, and an auditable replay command.

  For each failure or drift point:

  1. Identify the exact raw HTTP turn where the persona/FSM contract failed.

     ```bash
     go run ./cmd/pragma replay raw-http dump <run-dir-or-raw-http-dir> --out <analysis-dir> --overwrite

  Inspect the relevant turn files:

  sed -n '1,240p' <analysis-dir>/turn-000NN/request_messages.md
  sed -n '1,240p' <analysis-dir>/turn-000NN/response_content.md

  2. Classify the failure source.

     Use the dumped request/response plus orchestration logs to decide whether the failure came from:
      - persona responsibility
      - state ordering
      - transition policy
      - handoff content
      - artifact contract
      - runtime support
      - provider behavior
      - task implementation drift

     Inspect logs:

     sed -n '1,240p' <run-dir>/pragma.stdout.log
     sed -n '1,240p' <run-dir>/pragma.stderr.log
     rg -n "<state-id>|<artifact-id>|<failure text>" <run-dir> <analysis-dir>

  3. Create focused AB replay variants from the captured turn payload.

     Copy the failing case into a named prompt-AB directory:

     mkdir -p .pragma/prompt-ab/<timestamp>-<topic>/<case-id>
     cp <raw-http-turn-dir>/request.json .pragma/prompt-ab/<timestamp>-<topic>/<case-id>/request.json
     cp <raw-http-turn-dir>/request.meta.json .pragma/prompt-ab/<timestamp>-<topic>/<case-id>/request.meta.json

     Record the source and hypothesis:

     cat > .pragma/prompt-ab/<timestamp>-<topic>/<case-id>/source.txt <<'EOF'
     source_run: <run-dir>
     source_turn: <raw-http-turn-dir>
     failure: <exact failure>
     hypothesis: <what this variant changes and why>
     EOF

  4. Modify only the payload element being tested.

     Examples:
      - persona prompt only: replace the system prompt in request.json
      - handoff shape only: alter the user message artifact block
      - state responsibility only: use a payload representing a split/merged persona
      - transition contract only: change the transition/handoff prompt content
      - declarative artifact contract only: add/remove the relevant contract text

     Do not change unrelated payload content. Do not rerun the whole task to test one prompt hypothesis.

  5. Replay the focused variant.

     go run ./cmd/pragma replay raw-http \
       .pragma/prompt-ab/<timestamp>-<topic>/<case-id> \
       --provider lilac \
       --format raw \
       --out .pragma/prompt-ab/<timestamp>-<topic>/<case-id>/response.raw \
       --timeout 180s

     If metadata is not written automatically, write audit-compatible metadata from the actual response bytes:

     bytes=$(wc -c < .pragma/prompt-ab/<timestamp>-<topic>/<case-id>/response.raw | tr -d ' ')
     sha=$(shasum -a 256 .pragma/prompt-ab/<timestamp>-<topic>/<case-id>/response.raw | awk '{print $1}')
     now=$(date -u +%Y-%m-%dT%H:%M:%SZ)

     jq -n \
       --arg now "$now" \
       --arg sha "$sha" \
       --argjson bytes "$bytes" \
       '{
         status:"200 OK",
         status_code:200,
         started_at:$now,
         completed_at:$now,
         response_bytes:$bytes,
         response_sha256:$sha,
         duration_ms:0
       }' > .pragma/prompt-ab/<timestamp>-<topic>/<case-id>/response.meta.json

  6. Inspect and score the replay result.

     jq -r '.choices[0].message.content' \
       .pragma/prompt-ab/<timestamp>-<topic>/<case-id>/response.raw \
       | sed -n '1,240p'

     Check for the exact property under test, for example:

     jq -r '.choices[0].message.content' <case>/response.raw | rg -n "<required phrase|artifact|command|ID>"
     jq -r '.choices[0].message.content' <case>/response.raw | rg -n "<forbidden phrase|hardcoded baseline string>"

     A variant passes only if it improves the target failure without introducing:
      - task-specific hardcoding
      - weakened validation
      - fake/dummy outputs
      - skipped evaluator-facing behavior
      - information loss between personas
      - unsupported completion claims

  7. Audit replay evidence.

     go run ./cmd/pragma replay raw-http audit \
       .pragma/prompt-ab/<timestamp>-<topic> \
       --require-responses

  8. Promote only the best passing variant.

     Update production personas/FSM/declarative contracts only after replay evidence proves the direction.

     Before promotion, scan for forbidden hardcoding:

     rg -n "Flipt|Kubernetes|ACCEPT-K8S|ACCEPT-KUBERNETES|auth.proto|METHOD_KUBERNETES|mage Proto|go build ./\\.\\.\\.|go test ./internal/config" \
       personas-research-v2 orchestrations internal

     Production files must not contain baseline-specific strings unless the file is a clearly labeled test fixture or evidence artifact.

  9. Validate the edited production configuration without running the full benchmark.

     go run ./cmd/pragma orchestration visualize \
       orchestrations/swe-bench-pro-engineering-loop.yaml \
       --persona-dir personas-research-v2 \
       --details compact

     git diff --check

     Use compile-only checks only when needed and do not add tests unless explicitly asked:

     go test ./internal/query ./internal/orchestration -run '^$'
     go test ./internal/cli ./internal/observe ./internal/app -run '^$'

  10. Run a fresh full benchmark only after focused AB and static checks pass.

  ts=$(date -u +%Y%m%dT%H%M%SZ)
  out=.pragma/swe-bench-pro/${ts}-instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446

  python3 tools/run_swebench_pro_instance.py \
    --instance-id instance_flipt-io__flipt-0fd09def402258834b9d6c0eaa6d3b4ab93b4446 \
    --output-dir "$out" \
    --provider lilac \
    --model minimaxai/minimax-m2.7 \
    --orchestration /pragma/orchestrations/swe-bench-pro-engineering-loop.yaml \
    --persona-dir /pragma/personas-research-v2 \
    --generator-toolchain \
    --evaluate

  Poll long-running runs no more often than once every 60 seconds.

  11. After every full run, inspect before changing anything.

  cat <run-dir>/agent-status.txt
  sed -n '1,240p' <run-dir>/pragma.stdout.log
  sed -n '1,240p' <run-dir>/pragma.stderr.log
  find <run-dir>/raw-http-pragma -maxdepth 1 -mindepth 1 -type d | sort | tail -n 20

  If it fails, return to step 1 with the exact failing turn. Do not restart the full task blindly.

  Hard constraints:

  - Do not hardcode persona names, FSM state names, artifact IDs, artifact paths, acceptance-map semantics, worker-report semantics, or SWE-bench-specific contracts in generic runtime code.
  - Do not encode Flipt, Kubernetes, Go, proto, `mage`, `go build`, `go test`, `auth.proto`, or any baseline-task detail into production personas or generic orchestration logic.
  - Baseline-specific strings may exist only in baseline evidence, AB fixtures, run logs, or clearly labeled docs.
  - Do not bypass, dummy out, weaken, skip, or fake validation, evaluator checks, artifact checks, handoff checks, tests, or runtime behavior.
  - Do not add Pragma tests unless explicitly asked.
  - Do not use prompt-only reminders when the issue is an enforceable contract; make contracts declarative in orchestration/persona configuration.
  - Generic runtime may enforce only generic declared policies, not magic artifact names.
  - Prefer prompt/payload-level AB testing before full benchmark reruns.
  - Track exact persona-to-persona information transfer, including what each persona receives, writes, changes, preserves, and passes onward.
  - Persona responsibilities may be split, merged, added, or removed, but every responsibility boundary must be explicit and evidence-backed.
  - The first Flipt task is a baseline for calibration only; the architecture must generalize beyond that task, stack, repository, and failure mode.

  Completion requires:

  - Production orchestration/persona files express the required FSM and handoff contracts declaratively.
  - Generic runtime contains no special cases for this SWE-bench Pro persona set.
  - Production prompts contain no baseline-task or stack-specific leakage.
  - AB replay evidence passes for every changed persona/FSM contract.
  - A fresh Flipt SWE-bench Pro run using the cleaned generic system passes evaluator, or a real external blocker is proven with exact logs and a minimal next action.
  - At least one non-Flipt/generalization check is performed or explicitly blocked.
  - The ledger documents every edit, every rejected variant, every run, and a completion audit proving the above.
