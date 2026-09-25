package query

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/shellrun"
	"github.com/artpar/pragma/internal/tools/applypatch"
)

const pragmaLoopSystemPrompt = `Pragma loop mode is a shell-action transport.

Follow the active task and persona instructions. This wrapper only defines how
to send the next shell action.

When you need to inspect or change the system, your response must contain one
fenced bash code block with one command or one shell script. Do not write prose,
analysis sections, headings, or bullets outside the bash code block.
When you are done and want to answer the user, write the final answer directly
with no fenced bash code block unless the active runtime contract gives a
different completion signal. Do not use a bash block to end the turn unless the
active runtime contract requires it.
When you need to ask the user a question, ask it directly with no fenced bash
code block. Do not use echo or any other command to ask user questions.
Use commands, command output, and required task artifacts for reasoning and evidence.
Format your response as shown in <format_example>.

<format_example>
` + "```bash" + `
your_command_here
` + "```" + `
</format_example>

Failure to follow these rules will cause your response to be rejected.`

func PragmaLoopSystemPrompt() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: pragmaLoopSystemPrompt")
	return pragmaLoopSystemPrompt
}

const pragmaLoopInstanceSuffix = `

<system_information>
Current working directory: %s
%s
</system_information>
`

const pragmaLoopFormatErrorTemplate = `Please always provide EXACTLY ONE bash action in triple backticks and no prose outside the code block when you need a shell action. Found %d actions.
If you want to end the task, write the final answer directly with no fenced bash code block.
If you need to ask the user a question, ask it directly with no fenced bash code block; do not use echo.
Else, please format your response exactly as follows:

<response_example>
` + "```bash" + `
<action>
` + "```" + `
</response_example>

Note: In rare cases, if you need to reference a similar format in your command, you might have
to proceed in two steps, first writing TRIPLEBACKTICKSBASH, then replacing them with ` + "```bash" + `.`

var pragmaLoopCommandTimeout = 300 * time.Second
var pragmaLoopForegroundWait = 300 * time.Second

const pragmaLoopRunningOutputLines = 100

const pragmaLoopMaxNoActionRetries = 3

type pragmaLoopRunConfig struct {
	System              model.SystemPrompt
	UserMessage         string
	CompletionCheck     PragmaLoopCompletionCheck
	MessageStartIndexes []int
	MaxTurns            int
	ExplicitMaxTurns    bool
}

type pragmaLoopTurnRequest struct {
	Model  string
	Params provider.RequestParams
}

type pragmaLoopActionKind int

const (
	pragmaLoopActionFinal pragmaLoopActionKind = iota
	pragmaLoopActionBash
	pragmaLoopActionInvalid
	pragmaLoopActionNativeToolCall
)

type pragmaLoopAction struct {
	Kind  pragmaLoopActionKind
	Count int
	Bash  string
}

type pragmaLoopAssistantTurn struct {
	Response      model.Response
	Text          string
	ReplayContent []model.ContentPart
	Action        pragmaLoopAction
}

type pragmaLoopCommandResult struct {
	Command   string
	Result    pragmaLoopBashResult
	TimedOut  bool
	Submitted bool
}

type pragmaLoopObservation struct {
	Text     string
	Complete bool
}

type pragmaLoopEvidencePathKey struct{}
type pragmaLoopCommandPolicyKey struct{}

type PragmaLoopCommandEvidenceConfig struct {
	Path        string
	ReportPaths []string
}

type PragmaLoopCommandPolicyConfig struct {
	DenyPatterns        []string
	RequirePatterns     []string
	DenyMessage         string
	RenderedInputPaths  []string
	WritablePaths       []string
	ProtectedWritePaths []string
}

type PragmaLoopRunOptions struct {
	IncludePriorConversation bool
	MaxTurns                 int
	FinalTextOnly            bool
	FinalTextCheck           PragmaLoopFinalTextCheck
}

// WithPragmaLoopEvidencePath records shell-loop command evidence to path while
// this context is active. The evidence file stores hashes and status metadata,
// not full command output.
func WithPragmaLoopEvidencePath(ctx context.Context, path string) context.Context {
	observe.TraceCtx(ctx, "query", "WithPragmaLoopEvidencePath", "enter")
	defer observe.TraceCtx(ctx, "query", "WithPragmaLoopEvidencePath", "exit")
	observe.TraceCtx(ctx, "query", "WithPragmaLoopEvidencePath", "return: WithPragmaLoopCommandEvidence(ctx, PragmaLoopCommandEvidenceConfig{Path: path})")
	return WithPragmaLoopCommandEvidence(ctx, PragmaLoopCommandEvidenceConfig{Path: path})
}

func WithPragmaLoopCommandEvidence(ctx context.Context, cfg PragmaLoopCommandEvidenceConfig) context.Context {
	observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandEvidence", "enter")
	defer observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandEvidence", "exit")
	if strings.TrimSpace(cfg.Path) == "" {
		observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandEvidence", "if: strings.TrimSpace(cfg.Path) == \"\"")
		observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandEvidence", "return: ctx")
		return ctx
	}
	cfg.Path = strings.TrimSpace(cfg.Path)
	observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandEvidence", "return: context.WithValue(ctx, pragmaLoopEvidencePathKey{}, cfg)")
	return context.WithValue(ctx, pragmaLoopEvidencePathKey{}, cfg)
}

func WithPragmaLoopCommandPolicy(ctx context.Context, cfg PragmaLoopCommandPolicyConfig) context.Context {
	observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandPolicy", "enter")
	defer observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandPolicy", "exit")
	if len(cfg.DenyPatterns) == 0 && len(cfg.RequirePatterns) == 0 && len(cfg.RenderedInputPaths) == 0 && len(cfg.ProtectedWritePaths) == 0 {
		observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandPolicy", "if: len(cfg.DenyPatterns) == 0 && len(cfg.RenderedInputPaths) == 0 && len(cfg.Pro...")
		observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandPolicy", "return: ctx")
		return ctx
	}
	observe.TraceCtx(ctx, "query", "WithPragmaLoopCommandPolicy", "return: context.WithValue(ctx, pragmaLoopCommandPolicyKey{}, cfg)")
	return context.WithValue(ctx, pragmaLoopCommandPolicyKey{}, cfg)
}

func (engine *Engine) runPragmaLoop(ctx context.Context, userMessage string, ch chan<- LoopEvent) {
	observe.TraceCtx(ctx, "query", "Engine.runPragmaLoop", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.runPragmaLoop", "exit")
	snap := engine.store.Snapshot()
	system := engine.pragmaLoopSystemPrompt()
	engine.runPragmaLoopWithInitialPrompt(ctx, system, pragmaLoopInstancePrompt(userMessage, snap.CWD), nil, ch, PragmaLoopRunOptions{})
}

func (engine *Engine) pragmaLoopSystemPrompt() model.SystemPrompt {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	systemText := pragmaLoopSystemPrompt
	if custom := strings.TrimSpace(engine.config.CustomSystemPrompt); custom != "" {
		observe.GlobalTrace("if: custom != \"\"")
		systemText = custom + "\n\n" + pragmaLoopSystemPrompt
	}
	observe.GlobalTrace("return: model.SystemPrompt")
	observe.GlobalTrace("return: model.SystemPrompt{Blocks: []model.SystemBlock{{Text: systemText, Cacheable: ...")
	return model.SystemPrompt{Blocks: []model.SystemBlock{{Text: systemText, Cacheable: false}}}
}

// WithCustomSystemPrompt prepends the active custom system prompt to an
// explicit system prompt. This is used by orchestration paths that build their
// own persona system prompt instead of using pragmaLoopSystemPrompt.
func (engine *Engine) WithCustomSystemPrompt(system model.SystemPrompt) model.SystemPrompt {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	custom := strings.TrimSpace(engine.config.CustomSystemPrompt)
	if custom == "" {
		observe.GlobalTrace("if: custom == \"\"")
		observe.GlobalTrace("return: system")
		return system
	}
	blocks := make([]model.SystemBlock, 0, len(system.Blocks)+1)
	blocks = append(blocks, model.SystemBlock{Text: custom, Cacheable: false})
	blocks = append(blocks, system.Blocks...)
	observe.GlobalTrace("return: model.SystemPrompt{Blocks: blocks}")
	return model.SystemPrompt{Blocks: blocks}
}

// PragmaLoopCompletionCheck can reject a submitted bash turn and keep the same
// loop running with a corrective user observation.
type PragmaLoopCompletionCheck func() (bool, string, error)
type PragmaLoopFinalTextCheck func(string) (bool, string, error)

// RunPragmaLoopWithSystemCompletionCheck runs the shell-action loop with an
// optional completion check after a command emits the completion sentinel.
func (engine *Engine) RunPragmaLoopWithSystemCompletionCheck(ctx context.Context, system model.SystemPrompt, userMessage string, completionCheck PragmaLoopCompletionCheck) <-chan LoopEvent {
	observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "exit")
	observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "return: engine.RunPragmaLoopWithSystemCompletionCheckOptions(ctx, system, userMessage...")
	return engine.RunPragmaLoopWithSystemCompletionCheckOptions(ctx, system, userMessage, completionCheck, PragmaLoopRunOptions{})
}

// RunPragmaLoopWithSystemCompletionCheckOptions runs the shell-action loop with
// optional completion checks and request-history controls. By default the
// request is scoped to the current activation; IncludePriorConversation keeps
// earlier messages in the engine conversation visible to the model.
func (engine *Engine) RunPragmaLoopWithSystemCompletionCheckOptions(ctx context.Context, system model.SystemPrompt, userMessage string, completionCheck PragmaLoopCompletionCheck, opts PragmaLoopRunOptions) <-chan LoopEvent {
	observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "exit")
	ch := make(chan LoopEvent, 16)
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "if: r != nil")
				ch <- ErrorEvent{Err: fmt.Errorf("query loop panic: %v", r)}
			}
		}()
		if opts.IncludePriorConversation {
			observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheckOptions", "if: opts.IncludePriorConversation")
			engine.runPragmaLoopWithInitialPrompt(ctx, system, userMessage, completionCheck, ch, opts)
			return
		}
		startIndex := len(engine.store.Snapshot().Conversation.Messages)
		engine.runPragmaLoopWithInitialPrompt(ctx, system, userMessage, completionCheck, ch, opts, startIndex)
	}()
	observe.TraceCtx(ctx, "query", "Engine.RunPragmaLoopWithSystemCompletionCheck", "return: ch")
	return ch
}

// maybeAutoCompactPragmaLoop is the ONE unscoped pragma-loop auto-compact
// trigger (CMP-001.3, carrying the CMP-001.1 F4 single-increment
// semantics, the CMP-001.2 F1/F2/F3 application semantics, and the
// CMP-001.4 F6 request-shape count + eligibility bound). It is called
// before each model-request build by BOTH sub-paths of
// runPragmaLoopWithInitialPrompt: the main shell-action loop and the
// FinalTextOnly capture sub-loop (CMP-001.3.F3: orchestration
// final_text-capture states take that sub-loop even when their runs are
// UNSCOPED — IncludePriorConversation and FinalTextOnly are independent
// state predicates — so an over-window persistent capture state had no
// relief valve while the record claimed unscoped pragma-loop runs carry
// the trigger).
// Scoped activations (run.MessageStartIndexes set — orchestration state
// runs that request only their own conversation slice) skip the trigger:
// compaction replaces the WHOLE conversation, which would invalidate the
// scope's start index into the message array. The trigger deliberately
// does NOT IncrementTurn — each sub-path keeps its ONE per-iteration
// increment site (CMP-001 F4); a trigger-side increment would
// double-count the compaction's own iteration and halve MinTurnsCooldown.
// Returns false only when the caller must abort the run (a failed
// post-compaction session rewrite; the ErrorEvent is already emitted).
func (engine *Engine) maybeAutoCompactPragmaLoop(ctx context.Context, ch chan<- LoopEvent, run pragmaLoopRunConfig) bool {
	observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "exit")
	if len(run.MessageStartIndexes) != 0 || engine.compactor == nil || engine.autoTracker == nil {
		observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "if: len(run.MessageStartIndexes) != 0 || engine.compactor == nil || engine.autoTracker == nil")
		observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "return: true")
		return true
	}
	// CMP-001.4 F6 bound: when the tracker is disabled, the breaker has
	// tripped, or the cooldown is still active, ShouldAutoCompact is
	// false for EVERY token count, so skip computing one (provider
	// CountTokens is a network call on Google). IncrementTurn still runs
	// every iteration in the sub-paths — the cooldown expires by
	// counting model-request iterations.
	if !engine.autoTracker.AutoCompactEligible() {
		observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "if: !engine.autoTracker.AutoCompactEligible()")
		observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "return: true")
		return true
	}
	compSnap := engine.store.Snapshot()
	// CMP-001.4 F6: count the REQUEST shape. run.System is the exact
	// system every pragma-loop request carries
	// (buildPragmaLoopTurnRequest); pragma-loop requests carry no tool
	// schemas, and unscoped runs (the only ones reaching this point)
	// request the full conversation. Restores the fed8bd7^
	// requestTokenCount semantics — the pre-F6 estimate counted
	// APIMessages only and the pre-F6 precise call sent the raw
	// conversation system, undercounting the request payload the model
	// actually receives and delaying the trigger.
	tokenCount := engine.requestTokenCount(ctx,
		firstNonEmpty(compSnap.Model, compSnap.Conversation.Model, engine.config.Model),
		compSnap.Conversation.APIMessages(), run.System, nil)
	if !engine.autoTracker.ShouldAutoCompact(tokenCount, engine.windowConfig) {
		observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "if: !engine.autoTracker.ShouldAutoCompact(tokenCount, engine.windowConfig)")
		observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "return: true")
		return true
	}
	observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "if: engine.autoTracker.ShouldAutoCompact(tokenCount, engine.windowConfig)")
	ch <- CompactionStartedEvent{}
	compResult, compErr := engine.compactor.Compact(ctx, compSnap.Conversation.APIMessages(), compSnap.Conversation.System, "")
	switch {
	case compErr != nil && ctx.Err() == nil:
		// A real failure: count it and possibly trip the breaker.
		observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "case: compErr != nil && ctx.Err() == nil")
		tripped := engine.autoTracker.RecordFailure()
		ch <- CompactionFailedEvent{
			Attempt:  engine.autoTracker.FailureCount(),
			MaxRetry: compact.MaxConsecutiveFailures,
			ErrorMsg: compErr.Error(),
		}
		if tripped {
			engine.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", "", "", ""),
				Severity:     "warn",
				Component:    "compact",
				ErrorType:    "circuit_breaker_tripped",
				ErrorMessage: fmt.Sprintf("auto-compaction disabled after %d consecutive failures", compact.MaxConsecutiveFailures),
			})
			ch <- CompactionDisabledEvent{ConsecutiveFailures: compact.MaxConsecutiveFailures}
		}
	case compErr == nil:
		observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "case: compErr == nil")
		engine.autoTracker.RecordSuccess()
		// CMP-001.2 F3: compaction replaces the WHOLE conversation, but
		// the unanswered operator prompt (appended before iteration 0, or
		// the fresh bash observation at the tail of a later iteration)
		// must still reach the model verbatim.
		// CMP-001.2.F1: the pending set is derived from the LIVE store
		// state inside the replacement, never from the pre-compaction
		// snapshot, so operator input delivered mid-summary survives.
		// CMP-001.2.F3: both auto loops and manual /compact apply
		// compaction results through the one shared application point,
		// compact.ApplyResult — the semantics (pending re-append after
		// the summary, atomic live-state derivation) live there and
		// cannot diverge between paths again.
		compact.ApplyResult(engine.store, compResult)
		// CMP-001.2 F2: the compaction REPLACED Conversation.Messages,
		// but the incremental session checkpoint is index-based — it
		// would skip the summary and the first post-compaction exchange
		// (all land below the stale SessionLastIdx), leaving the file
		// with the full pre-compaction history for --resume to
		// resurrect. Rewrite the durable session file, exactly as manual
		// /compact does.
		if err := engine.rewriteSession(); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "if: err != nil")
			ch <- ErrorEvent{Err: fmt.Errorf("rewrite session after compaction: %w", err)}
			observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "return: false")
			return false
		}
		ch <- CompactionEvent{PreTokens: compResult.PreTokenCount, PostTokens: compResult.PostTokenCount}
	}
	// compErr != nil && ctx.Err() != nil: the loop is being torn down —
	// no failure count, no event, the caller's next ctx check exits
	// cleanly.
	observe.TraceCtx(ctx, "query", "Engine.maybeAutoCompactPragmaLoop", "return: true")
	return true
}

func (engine *Engine) runPragmaLoopWithInitialPrompt(ctx context.Context, system model.SystemPrompt, userMessage string, completionCheck PragmaLoopCompletionCheck, ch chan<- LoopEvent, opts PragmaLoopRunOptions, messageStartIndexes ...int) {
	observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "exit")
	defer func() {
		engine.runStopHook(ch)
	}()

	run := engine.newPragmaLoopRunConfig(system, userMessage, completionCheck, opts, messageStartIndexes...)
	engine.setConversationSystemPrompt(run.System)

	if err := engine.appendPragmaLoopInitialUserMessage(run); err != nil {
		observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
		ch <- ErrorEvent{Err: err}
		return
	}

	if opts.FinalTextOnly {
		observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: opts.FinalTextOnly")
		for turnIdx := 0; turnIdx < run.MaxTurns; turnIdx++ {
			// CMP-001.3.F3: the shared unscoped auto-compact trigger. A
			// persistent final_text-capture orchestration state runs THIS
			// sub-loop with IncludePriorConversation (unscoped) — before
			// this port the sub-loop never consulted the tracker, so an
			// over-window persistent capture state had no relief valve
			// while unscoped runs were claimed to carry the trigger.
			// Scoped capture runs (root-engine capture states) still skip
			// the trigger inside the helper.
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "for: turnIdx < run.MaxTurns")
			if !engine.maybeAutoCompactPragmaLoop(ctx, ch, run) {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: !engine.maybeAutoCompactPragmaLoop(ctx, ch, run)")
				return
			}
			request, _, err := engine.buildPragmaLoopTurnRequest(run)
			if err != nil {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
				ch <- ErrorEvent{Err: err}
				return
			}
			ch <- ModelRequestEvent{Model: request.Model, Attempt: turnIdx + 1}
			response, err := engine.completePragmaLoopResponse(ctx, request.Params, ch)
			if err != nil {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
				ch <- ErrorEvent{Err: err}
				return
			}
			ch <- ModelResponseEvent{Model: request.Model, StopReason: response.StopReason}
			turn := classifyPragmaLoopAssistantTurn(response)
			if err := engine.appendPragmaLoopAssistantTurn(turn); err != nil {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
				ch <- ErrorEvent{Err: err}
				return
			}
			if engine.autoTracker != nil {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: engine.autoTracker != nil")
				engine.autoTracker.IncrementTurn()
			}
			if opts.FinalTextCheck != nil {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: opts.FinalTextCheck != nil")
				ok, message, err := opts.FinalTextCheck(turn.Text)
				if err != nil {
					observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
					ch <- ErrorEvent{Err: err}
					return
				}
				if !ok {
					observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: !ok")
					if strings.TrimSpace(message) == "" {
						observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: strings.TrimSpace(message) == \"\"")
						message = "Final text was rejected because it did not match the required output contract. Return only the required final text."
					}
					if err := engine.appendPragmaLoopUserMessage(appendPragmaLoopBudgetNotice(message, run, turnIdx)); err != nil {
						observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
						ch <- ErrorEvent{Err: err}
						return
					}
					continue
				}
			}
			emitPragmaLoopResponseText(response, ch)
			ch <- TurnCompleteEvent{Response: response, StopReason: model.StopEndTurn}
			return
		}
		ch <- ErrorEvent{Err: fmt.Errorf("final text did not satisfy the required output contract after %d turns", run.MaxTurns)}
		return
	}

	for turn := 0; turn < run.MaxTurns; turn++ {
		observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "for: turn < maxTurns")
		if err := ctx.Err(); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: fmt.Errorf("context cancelled: %w", model.ErrContextCancelled)}
			return
		}
		if engine.config.MaxCostUSD > 0 && engine.costTracker.TotalUSD() >= engine.config.MaxCostUSD {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: spend ceiling reached")
			ch <- ErrorEvent{Err: fmt.Errorf(
				"session spend ceiling reached: $%.2f of $%.2f budget; the conversation is preserved — resume with a new prompt or --resume, or raise the ceiling",
				engine.costTracker.TotalUSD(), engine.config.MaxCostUSD)}
			return
		}

		// CMP-001.3: consult the auto-compact tracker before each request,
		// exactly as the provider-tools loop does since CMP-001. Nil deps
		// (subagent engines, #27794) leave this a no-op. Scoped activations
		// (run.MessageStartIndexes set) skip the trigger — and only the
		// trigger: compaction replaces the WHOLE conversation, which would
		// invalidate the scope's start index into the message array. The
		// full CMP-001..CMP-001.4 trigger semantics live in ONE place,
		// maybeAutoCompactPragmaLoop, shared with the FinalTextOnly
		// sub-loop of this same function (CMP-001.3.F3).
		if !engine.maybeAutoCompactPragmaLoop(ctx, ch, run) {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: !engine.maybeAutoCompactPragmaLoop(ctx, ch, run)")
			return
		}
		// CMP-001.3.F2: IncrementTurn is deliberately OUTSIDE the scope
		// gate above — it is the ONE per-iteration turn advance
		// (2e9f01b / CMP-001 F4 semantics) and runs for every
		// model-request iteration, scoped ones included: a scoped
		// iteration still appends its turns to the engine's whole
		// conversation, and the FinalTextOnly sub-loop of this same
		// function increments per model request unconditionally. When the
		// port kept this inside the scope gate, scoped runs advanced the
		// cooldown zero turns per iteration — the opposite semantics —
		// while nothing else changed: the pragma loop's old post-assistant
		// increment site was removed with the port (a second increment
		// inside the same iteration would expire MinTurnsCooldown within
		// the compaction's own iteration and let a still-over-threshold
		// conversation re-compact immediately).
		if engine.autoTracker != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: engine.autoTracker != nil")
			engine.autoTracker.IncrementTurn()
		}

		request, workDir, err := engine.buildPragmaLoopTurnRequest(run)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}

		assistantTurn, err := engine.completePragmaLoopAssistantTurn(ctx, request, ch)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}
		emitPragmaLoopResponseText(assistantTurn.Response, ch)
		ch <- ModelResponseEvent{Model: request.Model, StopReason: assistantTurn.Response.StopReason}

		if err := engine.appendPragmaLoopAssistantTurn(assistantTurn); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}

		if assistantTurn.Action.Kind == pragmaLoopActionFinal {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: actionCount == 0")
			if run.CompletionCheck != nil {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: completionCheck != nil for final action")
				ok, message, err := run.CompletionCheck()
				if err != nil {
					observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: completionCheck err")
					ch <- ErrorEvent{Err: err}
					return
				}
				if !ok {
					observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: !ok")
					if strings.TrimSpace(message) == "" {
						observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: strings.TrimSpace(message) == \"\"")
						message = "Completion was rejected because required output artifacts are missing. Create the missing artifacts and echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT again."
					}
					if err := engine.appendPragmaLoopUserMessage(appendPragmaLoopBudgetNotice(message, run, turn)); err != nil {
						observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
						ch <- ErrorEvent{Err: err}
						return
					}
					continue
				}
			}
			ch <- TurnCompleteEvent{Response: assistantTurn.Response, StopReason: model.StopEndTurn}
			return
		}
		if assistantTurn.Action.Kind == pragmaLoopActionInvalid {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: actionCount > 1")
			message := appendPragmaLoopBudgetNotice(fmt.Sprintf(pragmaLoopFormatErrorTemplate, assistantTurn.Action.Count), run, turn)
			if err := engine.appendPragmaLoopUserMessage(message); err != nil {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
				ch <- ErrorEvent{Err: err}
				return
			}
			continue
		}

		if message, rejected := rejectPragmaLoopCommand(ctx, assistantTurn.Action.Bash); rejected {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: rejected")
			if err := engine.appendPragmaLoopUserMessage(appendPragmaLoopBudgetNotice(message, run, turn)); err != nil {
				observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
				ch <- ErrorEvent{Err: err}
				return
			}
			continue
		}
		commandResult, err := executePragmaLoopCommand(ctx, workDir, assistantTurn.Action.Bash)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}
		obs, err := commandResult.Observation(run.CompletionCheck)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: submitted")
			ch <- ErrorEvent{Err: err}
			return
		}
		if obs.Complete {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: obs.Complete")
			ch <- TurnCompleteEvent{Response: assistantTurn.Response, StopReason: model.StopEndTurn}
			return
		}
		if err := engine.appendPragmaLoopUserMessage(appendPragmaLoopBudgetNotice(obs.Text, run, turn)); err != nil {
			observe.TraceCtx(ctx, "query", "Engine.runPragmaLoopWithInitialPrompt", "if: err != nil")
			ch <- ErrorEvent{Err: err}
			return
		}
	}

	ch <- ErrorEvent{Err: fmt.Errorf("agentic loop exceeded maximum of %d turns", run.MaxTurns)}
}

func (engine *Engine) newPragmaLoopRunConfig(system model.SystemPrompt, userMessage string, completionCheck PragmaLoopCompletionCheck, opts PragmaLoopRunOptions, messageStartIndexes ...int) pragmaLoopRunConfig {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	maxTurns := engine.config.MaxTurns
	explicitMaxTurns := false
	if opts.MaxTurns > 0 {
		observe.GlobalTrace("if: opts.MaxTurns > 0")
		maxTurns = opts.MaxTurns
		explicitMaxTurns = true
	}
	if maxTurns <= 0 {
		observe.GlobalTrace("if: maxTurns <= 0")
		maxTurns = 300
	}
	indexes := append([]int(nil), messageStartIndexes...)
	observe.GlobalTrace("return: pragmaLoopRunConfig")
	observe.GlobalTrace("return: pragmaLoopRunConfig{\n\tSystem:\t\t\tsystem,\n\tUserMessage:\t\tuserMessage,\n\tCompleti...")
	return pragmaLoopRunConfig{
		System:              system,
		UserMessage:         userMessage,
		CompletionCheck:     completionCheck,
		MessageStartIndexes: indexes,
		MaxTurns:            maxTurns,
		ExplicitMaxTurns:    explicitMaxTurns,
	}
}

func appendPragmaLoopBudgetNotice(text string, run pragmaLoopRunConfig, turn int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !run.ExplicitMaxTurns || run.MaxTurns <= 0 {
		observe.GlobalTrace("if: !run.ExplicitMaxTurns || run.MaxTurns <= 0")
		observe.GlobalTrace("return: text")
		return text
	}
	used := turn + 1
	remaining := run.MaxTurns - used
	if remaining < 0 {
		observe.GlobalTrace("if: remaining < 0")
		remaining = 0
	}
	var b strings.Builder
	b.WriteString(strings.TrimRight(text, "\n"))
	fmt.Fprintf(&b, "\n\n<runtime_budget>\nState turn budget: %d/%d turns used; %d remaining.\n", used, run.MaxTurns, remaining)
	if remaining <= 10 && remaining > 0 {
		observe.GlobalTrace("if: remaining <= 10 && remaining > 0")
		b.WriteString("Budget warning: finish the state-specific completion contract within the remaining turns; do not start broad new repair threads unless they are necessary to complete.\n")
	}
	if remaining == 0 {
		observe.GlobalTrace("if: remaining == 0")
		b.WriteString("Budget exhausted: this state will stop if the previous action did not satisfy its completion contract.\n")
	}
	b.WriteString("</runtime_budget>")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func (engine *Engine) appendPragmaLoopInitialUserMessage(run pragmaLoopRunConfig) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	userMsg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: run.UserMessage}},
		Timestamp: time.Now(),
	}
	observe.GlobalTrace("return: engine.appendConversationMessage(userMsg)")
	return engine.appendConversationMessage(userMsg)
}

func (engine *Engine) buildPragmaLoopTurnRequest(run pragmaLoopRunConfig) (pragmaLoopTurnRequest, string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	snap := engine.store.Snapshot()
	resolvedModel := engine.config.Model
	if snap.Model != "" {
		observe.GlobalTrace("if: snap.Model != \"\"")
		resolvedModel = snap.Model
	}

	messagesForQuery, err := engine.messagesForRequestChecked(snap.Conversation, run.MessageStartIndexes...)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: pragmaLoopTurnRequest{}, \"\", err")
		return pragmaLoopTurnRequest{}, "", err
	}
	params := provider.RequestParams{
		Model:          resolvedModel,
		MaxTokens:      engine.config.MaxTokens,
		Messages:       messagesForQuery,
		System:         run.System,
		Temperature:    engine.config.Temperature,
		Thinking:       pragmaLoopThinkingConfig(engine.config.Thinking),
		ResponseSchema: engine.config.ResponseSchema,
	}
	observe.GlobalTrace("return: pragmaLoopTurnRequest, snap.CWD, nil")
	observe.GlobalTrace("return: pragmaLoopTurnRequest{\n\tModel:\tresolvedModel,\n\tParams:\tparams,\n}, snap.CWD, nil")
	return pragmaLoopTurnRequest{
		Model:  resolvedModel,
		Params: params,
	}, snap.CWD, nil
}

func pragmaLoopThinkingConfig(cfg *provider.ThinkingConfig) *provider.ThinkingConfig {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if cfg != nil {
		observe.GlobalTrace("if: cfg != nil")
		observe.GlobalTrace("return: cfg")
		return cfg
	}
	observe.GlobalTrace("return: &provider.ThinkingConfig{Enabled: false}")
	return &provider.ThinkingConfig{Enabled: false}
}

func (engine *Engine) completePragmaLoopAssistantTurn(ctx context.Context, request pragmaLoopTurnRequest, ch chan<- LoopEvent) (pragmaLoopAssistantTurn, error) {
	observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "exit")
	for noActionRetries := 0; ; noActionRetries++ {
		observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "for: true")
		ch <- ModelRequestEvent{Model: request.Model, Attempt: noActionRetries + 1}
		response, err := engine.completePragmaLoopResponse(ctx, request.Params, ch)
		if err != nil {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "if: err != nil")
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "return: pragmaLoopAssistantTurn{}, err")
			return pragmaLoopAssistantTurn{}, err
		}

		turn := classifyPragmaLoopAssistantTurn(response)
		switch turn.Action.Kind {
		case pragmaLoopActionNativeToolCall:
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "case: pragmaLoopActionNativeToolCall")
			if noActionRetries >= pragmaLoopMaxNoActionRetries {
				observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "return: pragmaLoopAssistantTurn{}, fmt.Errorf(\"model returned native tool call in Pra...")
				return pragmaLoopAssistantTurn{}, fmt.Errorf("model returned native tool call in Pragma bash loop after %d retries", pragmaLoopMaxNoActionRetries)
			}
		case pragmaLoopActionFinal:
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "case: pragmaLoopActionFinal")
			return turn, nil
		default:
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopAssistantTurn", "default")
			return turn, nil
		}
	}
}

func classifyPragmaLoopAssistantTurn(response model.Response) pragmaLoopAssistantTurn {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text := responseText(response)
	action := classifyPragmaLoopAction(response.Content, text)
	observe.GlobalTrace("return: pragmaLoopAssistantTurn")
	observe.GlobalTrace("return: pragmaLoopAssistantTurn{\n\tResponse:\tresponse,\n\tText:\t\ttext,\n\tReplayContent:\tp...")
	return pragmaLoopAssistantTurn{
		Response:      response,
		Text:          text,
		ReplayContent: pragmaLoopReplayContent(response.Content),
		Action:        action,
	}
}

func classifyPragmaLoopAction(content []model.ContentPart, text string) pragmaLoopAction {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if pragmaLoopHasToolCall(content) {
		observe.GlobalTrace("if: pragmaLoopHasToolCall(content)")
		observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionNativeToolCall}")
		return pragmaLoopAction{Kind: pragmaLoopActionNativeToolCall}
	}
	command, actionCount := extractPragmaLoopCommand(text)
	if actionCount == 0 {
		observe.GlobalTrace("if: actionCount == 0")
		observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionFinal}")
		return pragmaLoopAction{Kind: pragmaLoopActionFinal}
	}
	if actionCount > 1 {
		observe.GlobalTrace("if: actionCount > 1")
		observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionInvalid, Count: actionCount}")
		return pragmaLoopAction{Kind: pragmaLoopActionInvalid, Count: actionCount}
	}
	observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionBash}")
	observe.GlobalTrace("return: pragmaLoopAction{Kind: pragmaLoopActionBash, Count: actionCount, Bash: command}")
	return pragmaLoopAction{Kind: pragmaLoopActionBash, Count: actionCount, Bash: command}
}

func (engine *Engine) appendPragmaLoopAssistantTurn(turn pragmaLoopAssistantTurn) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	assistantMsg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleAssistant,
		Content:   turn.ReplayContent,
		Timestamp: time.Now(),
	}
	observe.GlobalTrace("return: engine.appendConversationMessage(assistantMsg)")
	return engine.appendConversationMessage(assistantMsg)
}

func executePragmaLoopCommand(ctx context.Context, workDir, command string) (pragmaLoopCommandResult, error) {
	observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "enter")
	defer observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "exit")
	result, timedOut, err := runPragmaLoopBash(ctx, workDir, command)
	if err != nil {
		observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "if: err != nil")
		observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "return: pragmaLoopCommandResult{}, err")
		return pragmaLoopCommandResult{}, err
	}
	submitted := false
	if !timedOut {
		observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "if: !timedOut")
		submitted, _ = pragmaLoopSubmitted(result)
	}
	commandResult := pragmaLoopCommandResult{
		Command:   command,
		Result:    result,
		TimedOut:  timedOut,
		Submitted: submitted,
	}
	recordPragmaLoopCommandEvidence(ctx, commandResult)
	observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "return: pragmaLoopCommandResult")
	observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "return: pragmaLoopCommandResult{\n\tCommand:\tcommand,\n\tResult:\t\tresult,\n\tTimedOut:\ttime...")
	observe.TraceCtx(ctx, "query", "executePragmaLoopCommand", "return: commandResult, nil")
	return commandResult, nil
}

func recordPragmaLoopCommandEvidence(ctx context.Context, result pragmaLoopCommandResult) {
	observe.TraceCtx(ctx, "query", "recordPragmaLoopCommandEvidence", "enter")
	defer observe.TraceCtx(ctx, "query", "recordPragmaLoopCommandEvidence", "exit")
	cfg, _ := ctx.Value(pragmaLoopEvidencePathKey{}).(PragmaLoopCommandEvidenceConfig)
	if strings.TrimSpace(cfg.Path) == "" {
		observe.TraceCtx(ctx, "query", "recordPragmaLoopCommandEvidence", "if: strings.TrimSpace(cfg.Path) == \"\"")
		return
	}
	record := struct {
		Timestamp          string `json:"timestamp"`
		CommandPreview     string `json:"command_preview"`
		CommandSHA256      string `json:"command_sha256"`
		OutputSHA256       string `json:"output_sha256"`
		OutputBytes        int    `json:"output_bytes"`
		ReturnCode         int    `json:"returncode"`
		TimedOut           bool   `json:"timed_out"`
		Submitted          bool   `json:"submitted"`
		CompletionSentinel bool   `json:"completion_sentinel"`
		WritesReport       bool   `json:"writes_report"`
		RepoMutation       bool   `json:"repo_mutation"`
	}{
		Timestamp:          time.Now().UTC().Format(time.RFC3339Nano),
		CommandPreview:     pragmaLoopCommandPreview(result.Command),
		CommandSHA256:      sha256HexString(result.Command),
		OutputSHA256:       sha256HexString(result.Result.Output),
		OutputBytes:        len(result.Result.Output),
		ReturnCode:         result.Result.ReturnCode,
		TimedOut:           result.TimedOut,
		Submitted:          result.Submitted,
		CompletionSentinel: strings.Contains(result.Result.Output, "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT") || strings.Contains(result.Command, "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"),
		WritesReport:       commandWritesDeclaredReport(result.Command, cfg.ReportPaths),
		RepoMutation:       commandLooksLikeRepoMutation(result.Command, cfg.ReportPaths),
	}
	data, err := json.Marshal(record)
	if err != nil {
		observe.TraceCtx(ctx, "query", "recordPragmaLoopCommandEvidence", "if: err != nil")
		return
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Path), 0o700); err != nil {
		observe.TraceCtx(ctx, "query", "recordPragmaLoopCommandEvidence", "if: err != nil")
		return
	}
	f, err := os.OpenFile(cfg.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		observe.TraceCtx(ctx, "query", "recordPragmaLoopCommandEvidence", "if: err != nil")
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}

func commandLooksLikeRepoMutation(command string, reportPaths []string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lower := strings.ToLower(strings.Join(strings.Fields(command), " "))
	if lower == "" {
		observe.GlobalTrace("return: false")
		return false
	}
	if strings.Contains(lower, "/tmp/pragma/swe/") && !strings.Contains(lower, "/app/") && !strings.Contains(lower, " internal/") && !strings.Contains(lower, " rpc/") {
		observe.GlobalTrace("return: false")
		return false
	}
	for _, marker := range []string{
		"sed -i",
		"perl -pi",
		"patch ",
		"> /app/",
		">> /app/",
		"mv /tmp/",
		"cp /tmp/",
		"os.replace(",
		".write(",
		"write_text(",
	} {
		observe.GlobalTrace("range markers")
		if strings.Contains(lower, marker) {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	for _, pathMarker := range []string{
		" internal/",
		" rpc/",
		" cmd/",
		" pkg/",
		" go.mod",
		" go.sum",
		"/app/internal/",
		"/app/rpc/",
		"/app/cmd/",
		"/app/pkg/",
		"/app/go.mod",
		"/app/go.sum",
	} {
		observe.GlobalTrace("range path markers")
		if strings.Contains(lower, pathMarker) && (strings.Contains(lower, " mv ") || strings.Contains(lower, " cp ") || strings.Contains(lower, "os.replace(") || strings.Contains(lower, ".write(")) {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func commandWritesDeclaredReport(command string, reportPaths []string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, path := range reportPaths {
		observe.GlobalTrace("range reportPaths")
		path = strings.TrimSpace(path)
		if path == "" {
			observe.GlobalTrace("if: path == \"\"")
			continue
		}
		if strings.Contains(command, path) {
			observe.GlobalTrace("if: strings.Contains(command, path)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func rejectPragmaLoopCommand(ctx context.Context, command string) (string, bool) {
	observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "enter")
	defer observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "exit")
	cfg, _ := ctx.Value(pragmaLoopCommandPolicyKey{}).(PragmaLoopCommandPolicyConfig)
	if len(cfg.DenyPatterns) == 0 && len(cfg.RequirePatterns) == 0 && len(cfg.RenderedInputPaths) == 0 && len(cfg.ProtectedWritePaths) == 0 {
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: len(cfg.DenyPatterns) == 0 && len(cfg.RenderedInputPaths) == 0 && len(cfg.Pro...")
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "return: \"\", false")
		return "", false
	}
	scannable := shellPolicyScannableText(command)
	if path, ok := commandMutatesProtectedPath(scannable, cfg.ProtectedWritePaths); ok {
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: ok")
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "return: fmt.Sprintf(\"Command was rejected because it attempts to modify runtime-autho...")
		return fmt.Sprintf("Command was rejected because it attempts to modify runtime-authored artifact `%s`. Do not create, edit, delete, truncate, overwrite, or fabricate runtime-authored artifacts. Run real commands so the runtime can capture evidence, or edit model-authored output artifacts so they only claim evidence that exists.", path), true
	}
	for _, pattern := range cfg.DenyPatterns {
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "range cfg.DenyPatterns")
		re, err := regexp.Compile(pattern)
		if err != nil {
			observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: err != nil")
			continue
		}
		if !re.MatchString(scannable) {
			observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: !re.MatchString(scannable)")
			continue
		}
		message := strings.TrimSpace(cfg.DenyMessage)
		if message == "" {
			observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: message == \"\"")
			message = "Command was rejected by the active shell policy. Choose a command allowed by the state contract."
		}
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "return: message, true")
		return message, true
	}
	for _, pattern := range cfg.RequirePatterns {
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "range cfg.RequirePatterns")
		re, err := regexp.Compile(pattern)
		if err != nil {
			observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: err != nil")
			continue
		}
		if re.MatchString(scannable) {
			observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: re.MatchString(scannable)")
			continue
		}
		message := strings.TrimSpace(cfg.DenyMessage)
		if message == "" {
			observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: message == \"\"")
			message = fmt.Sprintf("Command was rejected because it does not satisfy required shell policy pattern %q.", pattern)
		}
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "return: message, true")
		return message, true
	}
	if blocksRenderedInputPath(scannable, cfg.RenderedInputPaths, cfg.WritablePaths) {
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: blocksRenderedInputPath(scannable, cfg.RenderedInputPaths, cfg.WritablePaths)")
		message := strings.TrimSpace(cfg.DenyMessage)
		if message == "" {
			observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "if: message == \"\"")
			message = "Command was rejected by the active shell policy. This state receives declared handoff inputs as rendered content; write declared outputs from that rendered content instead of reading handoff artifact paths again."
		}
		observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "return: message, true")
		return message, true
	}
	observe.TraceCtx(ctx, "query", "rejectPragmaLoopCommand", "return: \"\", false")
	return "", false
}

func commandMutatesProtectedPath(scannable string, paths []string) (string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, line := range strings.Split(scannable, "\n") {
		observe.GlobalTrace("range strings.Split(scannable, \"\\n\")")
		for _, path := range paths {
			observe.GlobalTrace("range paths")
			path = strings.TrimSpace(path)
			if path == "" {
				observe.GlobalTrace("if: path == \"\"")
				continue
			}
			if lineMutatesPath(line, path) {
				observe.GlobalTrace("if: lineMutatesPath(line, path)")
				observe.GlobalTrace("return: path, true")
				return path, true
			}
		}
	}
	observe.GlobalTrace("return: \"\", false")
	return "", false
}

func lineMutatesPath(line string, path string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if lineWritesPath(line, path) {
		observe.GlobalTrace("if: lineWritesPath(line, path)")
		observe.GlobalTrace("return: true")
		return true
	}
	if !strings.Contains(line, path) {
		observe.GlobalTrace("if: !strings.Contains(line, path)")
		observe.GlobalTrace("return: false")
		return false
	}
	for _, segment := range shellCommandSegments(line) {
		observe.GlobalTrace("range shellCommandSegments(line)")
		if !strings.Contains(segment, path) {
			observe.GlobalTrace("if: !strings.Contains(segment, path)")
			continue
		}
		if shellSegmentMutatesReferencedPath(segment) {
			observe.GlobalTrace("if: shellSegmentMutatesReferencedPath(segment)")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func shellCommandSegments(line string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	replacer := strings.NewReplacer("&&", "\n", "||", "\n", ";", "\n", "|", "\n")
	observe.GlobalTrace("return: strings.Split(replacer.Replace(line), \"\\n\")")
	return strings.Split(replacer.Replace(line), "\n")
}

func shellSegmentMutatesReferencedPath(segment string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	fields := shellSegmentFields(segment)
	if len(fields) == 0 {
		observe.GlobalTrace("if: len(fields) == 0")
		observe.GlobalTrace("return: false")
		return false
	}
	cmd := filepath.Base(fields[0])
	switch cmd {
	case "rm", "unlink", "truncate", "touch", "tee", "mv":
		observe.GlobalTrace("case: \"rm\", \"unlink\", \"truncate\", \"touch\", \"tee\", \"mv\"")
		return true
	case "sed", "perl":
		observe.GlobalTrace("case: \"sed\", \"perl\"")
		for _, field := range fields[1:] {
			if field == "-i" || strings.HasPrefix(field, "-i.") || strings.HasPrefix(field, "-i") {
				observe.GlobalTrace("if: field == \"-i\" || strings.HasPrefix(field, \"-i.\") || strings.HasPrefix(field, ...")
				observe.GlobalTrace("return: true")
				return true
			}
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func shellSegmentFields(segment string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	raw := strings.Fields(strings.TrimSpace(segment))
	fields := make([]string, 0, len(raw))
	for _, field := range raw {
		observe.GlobalTrace("range raw")
		field = strings.Trim(field, `"'`)
		if field == "" {
			observe.GlobalTrace("if: field == \"\"")
			continue
		}
		fields = append(fields, field)
	}
	observe.GlobalTrace("return: fields")
	return fields
}

func blocksRenderedInputPath(scannable string, inputPaths []string, writablePaths []string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, line := range strings.Split(scannable, "\n") {
		observe.GlobalTrace("range strings.Split(scannable, \"\\n\")")
		for _, path := range inputPaths {
			observe.GlobalTrace("range inputPaths")
			path = strings.TrimSpace(path)
			if path == "" || !strings.Contains(line, path) {
				observe.GlobalTrace("if: path == \"\" || !strings.Contains(line, path)")
				continue
			}
			if lineWritesPath(line, path) && pathInList(path, writablePaths) {
				observe.GlobalTrace("if: lineWritesPath(line, path) && pathInList(path, writablePaths)")
				continue
			}
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func lineWritesPath(line string, path string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	path = strings.TrimSpace(path)
	if path == "" {
		observe.GlobalTrace("if: path == \"\"")
		observe.GlobalTrace("return: false")
		return false
	}
	for _, op := range []string{">>", ">"} {
		observe.GlobalTrace("range []string{\">>\", \">\"}")
		idx := strings.Index(line, op)
		for idx >= 0 {
			observe.GlobalTrace("for: idx >= 0")
			after := strings.TrimLeft(line[idx+len(op):], " \t")
			if shellPathPrefix(after, path) {
				observe.GlobalTrace("if: shellPathPrefix(after, path)")
				observe.GlobalTrace("return: true")
				return true
			}
			next := strings.Index(line[idx+len(op):], op)
			if next < 0 {
				observe.GlobalTrace("if: next < 0")
				break
			}
			idx += len(op) + next
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func shellPathPrefix(text string, path string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.HasPrefix(text, path) {
		observe.GlobalTrace("if: strings.HasPrefix(text, path)")
		observe.GlobalTrace("return: shellPathBoundary(text[len(path):])")
		return shellPathBoundary(text[len(path):])
	}
	if len(text) < 2 {
		observe.GlobalTrace("if: len(text) < 2")
		observe.GlobalTrace("return: false")
		return false
	}
	quote := text[0]
	if quote != '\'' && quote != '"' {
		observe.GlobalTrace("if: quote != '\\'' && quote != '\"'")
		observe.GlobalTrace("return: false")
		return false
	}
	rest := text[1:]
	if !strings.HasPrefix(rest, path) {
		observe.GlobalTrace("if: !strings.HasPrefix(rest, path)")
		observe.GlobalTrace("return: false")
		return false
	}
	after := rest[len(path):]
	observe.GlobalTrace("return: len(after) > 0 && after[0] == quote")
	return len(after) > 0 && after[0] == quote
}

func shellPathBoundary(text string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if text == "" {
		observe.GlobalTrace("if: text == \"\"")
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: strings.ContainsRune(\" \\t\\r\\n;&|)\", rune(text[0]))")
	return strings.ContainsRune(" \t\r\n;&|)", rune(text[0]))
}

func pathInList(path string, paths []string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, candidate := range paths {
		observe.GlobalTrace("range paths")
		if strings.TrimSpace(candidate) == path {
			observe.GlobalTrace("if: strings.TrimSpace(candidate) == path")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func shellPolicyScannableText(command string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var out []string
	var heredocEnd string
	for _, line := range strings.Split(command, "\n") {
		observe.GlobalTrace("range strings.Split(command, \"\\n\")")
		trimmed := strings.TrimSpace(line)
		if heredocEnd != "" {
			observe.GlobalTrace("if: heredocEnd != \"\"")
			if trimmed == heredocEnd {
				observe.GlobalTrace("if: trimmed == heredocEnd")
				heredocEnd = ""
			}
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			observe.GlobalTrace("if: strings.HasPrefix(trimmed, \"#\")")
			continue
		}
		out = append(out, line)
		if end := shellHeredocEndToken(line); end != "" {
			observe.GlobalTrace("if: end != \"\"")
			heredocEnd = end
		}
	}
	observe.GlobalTrace("return: strings.Join(out, \"\\n\")")
	return strings.Join(out, "\n")
}

func shellHeredocEndToken(line string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	idx := strings.Index(line, "<<")
	if idx < 0 {
		observe.GlobalTrace("if: idx < 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	token := strings.TrimSpace(line[idx+2:])
	if strings.HasPrefix(token, "-") {
		observe.GlobalTrace("if: strings.HasPrefix(token, \"-\")")
		token = strings.TrimSpace(strings.TrimPrefix(token, "-"))
	}
	fields := strings.Fields(token)
	if len(fields) == 0 {
		observe.GlobalTrace("if: len(fields) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	token = fields[0]
	token = strings.Trim(token, `"'`)
	if token == "" || strings.ContainsAny(token, `/\`) {
		observe.GlobalTrace("if: token == \"\" || strings.ContainsAny(token, `/\\`)")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: token")
	return token
}

func pragmaLoopCommandPreview(command string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	command = strings.TrimSpace(command)
	if command == "" {
		observe.GlobalTrace("if: command == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	line := command
	if idx := strings.IndexByte(line, '\n'); idx >= 0 {
		observe.GlobalTrace("if: idx >= 0")
		line = line[:idx]
	}
	line = strings.TrimSpace(line)
	if len(line) > 200 {
		observe.GlobalTrace("if: len(line) > 200")
		observe.GlobalTrace("return: line[:200] + \"...\"")
		return line[:200] + "..."
	}
	observe.GlobalTrace("return: line")
	return line
}

func sha256HexString(value string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sum := sha256.Sum256([]byte(value))
	observe.GlobalTrace("return: fmt.Sprintf(\"%x\", sum[:])")
	return fmt.Sprintf("%x", sum[:])
}

func (result pragmaLoopCommandResult) Observation(completionCheck PragmaLoopCompletionCheck) (pragmaLoopObservation, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if result.TimedOut {
		observe.GlobalTrace("if: result.TimedOut")
		observe.GlobalTrace("return: pragmaLoopObservation{Text: formatPragmaLoopTimeout(result.Command, result.Re...")
		return pragmaLoopObservation{Text: formatPragmaLoopTimeout(result.Command, result.Result.Output)}, nil
	}
	if !result.Submitted {
		observe.GlobalTrace("if: !result.Submitted")
		observe.GlobalTrace("return: pragmaLoopObservation{Text: formatPragmaLoopObservation(result.Result)}, nil")
		return pragmaLoopObservation{Text: formatPragmaLoopObservation(result.Result)}, nil
	}
	if completionCheck == nil {
		observe.GlobalTrace("if: completionCheck == nil")
		observe.GlobalTrace("return: pragmaLoopObservation{Complete: true}, nil")
		return pragmaLoopObservation{Complete: true}, nil
	}
	ok, message, err := completionCheck()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: pragmaLoopObservation{}, err")
		return pragmaLoopObservation{}, err
	}
	if ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: pragmaLoopObservation{Complete: true}, nil")
		return pragmaLoopObservation{Complete: true}, nil
	}
	if strings.TrimSpace(message) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(message) == \"\"")
		message = "Completion was rejected because required output artifacts are missing. Create the missing artifacts and echo COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT again."
	}
	observe.GlobalTrace("return: pragmaLoopObservation{Text: message}, nil")
	return pragmaLoopObservation{Text: message}, nil
}

func (engine *Engine) completePragmaLoopResponse(ctx context.Context, params provider.RequestParams, ch chan<- LoopEvent) (model.Response, error) {
	observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "exit")
	const maxStreamRetries = 10
	const maxConsecutiveOverloaded = 3
	var consecutiveOverloaded int

	for attempt := range maxStreamRetries + 1 {
		observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "range maxStreamRetries + 1")
		response, err := engine.provider.Complete(ctx, params)
		if err == nil {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: err == nil")
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "return: response, nil")
			return response, nil
		}

		classified := ClassifyStreamError(err)
		if classified.Kind == ErrorKindOverloaded {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: classified.Kind == ErrorKindOverloaded")
			consecutiveOverloaded++
			if consecutiveOverloaded >= maxConsecutiveOverloaded {
				observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: consecutiveOverloaded >= maxConsecutiveOverloaded")
				observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "return: model.Response{}, fmt.Errorf(\"repeated overloaded errors\")")
				return model.Response{}, fmt.Errorf("repeated overloaded errors")
			}
		} else {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "else: classified.Kind == ErrorKindOverloaded")
			consecutiveOverloaded = 0
		}
		if !classified.Retryable || attempt >= maxStreamRetries {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: !classified.Retryable || attempt >= maxStreamRetries")
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "return: model.Response{}, classified.Err")
			return model.Response{}, classified.Err
		}

		delay := classified.RetryAfter
		if delay == 0 {
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "if: delay == 0")
			delay = retryDelay(attempt)
		}
		ch <- RetryEvent{
			Attempt:     attempt + 1,
			MaxAttempts: maxStreamRetries + 1,
			Delay:       delay,
			Kind:        classified.Kind,
			ErrorMsg:    err.Error(),
		}
		select {
		case <-time.After(delay):
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "select: <-time.After(delay)")
		case <-ctx.Done():
			observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "select: <-ctx.Done()")
			return model.Response{}, fmt.Errorf("context cancelled during retry: %w", model.ErrContextCancelled)
		}
	}
	observe.TraceCtx(ctx, "query", "Engine.completePragmaLoopResponse", "return: model.Response{}, errors.New(\"model request failed\")")

	return model.Response{}, errors.New("model request failed")
}

func emitPragmaLoopResponseText(response model.Response, ch chan<- LoopEvent) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, part := range response.Content {
		observe.GlobalTrace("range response.Content")
		if text, ok := part.(model.TextPart); ok && text.Text != "" {
			observe.GlobalTrace("if: ok && text.Text != \"\"")
			ch <- TextEvent{Text: text.Text}
		}
	}
}

func (engine *Engine) appendPragmaLoopUserMessage(text string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	msg := model.Message{
		ID:        model.NewUUID(),
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: text}},
		Timestamp: time.Now(),
	}
	observe.GlobalTrace("return: engine.appendConversationMessage(msg)")
	return engine.appendConversationMessage(msg)
}

func pragmaLoopInstancePrompt(task, cwd string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Please solve this task: \" + task + fmt.Sprintf(pragmaLoopInstanceSuffix, cwd...")
	return "Please solve this task: " + task + fmt.Sprintf(pragmaLoopInstanceSuffix, cwd, pragmaLoopSystemInformation(cwd))
}

func pragmaLoopSystemInformation(cwd string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out, err := exec.Command("uname", "-srmv").Output()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: cwd")
		return cwd
	}
	fields := strings.Fields(strings.TrimSpace(string(out)))
	if len(fields) < 4 {
		observe.GlobalTrace("if: len(fields) < 4")
		observe.GlobalTrace("return: strings.TrimSpace(string(out))")
		return strings.TrimSpace(string(out))
	}
	system := fields[0]
	release := fields[1]
	machine := fields[len(fields)-1]
	version := strings.Join(fields[2:len(fields)-1], " ")
	observe.GlobalTrace("return: fmt.Sprintf(\"%s %s %s %s\", system, release, version, machine)")
	return fmt.Sprintf("%s %s %s %s", system, release, version, machine)
}

func responseText(response model.Response) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	for _, part := range response.Content {
		observe.GlobalTrace("range response.Content")
		if text, ok := part.(model.TextPart); ok {
			observe.GlobalTrace("if: ok")
			b.WriteString(text.Text)
		}
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func pragmaLoopReplayContent(content []model.ContentPart) []model.ContentPart {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := make([]model.ContentPart, 0, len(content))
	for _, part := range content {
		observe.GlobalTrace("range content")
		if _, ok := part.(model.ThinkingPart); ok {
			observe.GlobalTrace("if: ok")
			continue
		}
		if _, ok := part.(model.ToolCallPart); ok {
			observe.GlobalTrace("if: ok")
			continue
		}
		if text, ok := part.(model.TextPart); ok {
			observe.GlobalTrace("if: ok")
			text.Text = stripPragmaLoopThinkBlocks(text.Text)
			if strings.TrimSpace(text.Text) == "" {
				observe.GlobalTrace("if: strings.TrimSpace(text.Text) == \"\"")
				continue
			}
			out = append(out, text)
			continue
		}
		out = append(out, part)
	}
	observe.GlobalTrace("return: out")
	return out
}

func stripPragmaLoopThinkBlocks(text string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for {
		observe.GlobalTrace("for: true")
		lower := strings.ToLower(text)
		start := strings.Index(lower, "<think>")
		if start < 0 {
			observe.GlobalTrace("return: strings.TrimSpace(text)")
			return strings.TrimSpace(text)
		}
		end := strings.Index(lower[start+len("<think>"):], "</think>")
		if end < 0 {
			observe.GlobalTrace("return: strings.TrimSpace(text[:start])")
			return strings.TrimSpace(text[:start])
		}
		end += start + len("<think>") + len("</think>")
		text = text[:start] + text[end:]
	}
}

func pragmaLoopHasToolCall(content []model.ContentPart) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, part := range content {
		observe.GlobalTrace("range content")
		if _, ok := part.(model.ToolCallPart); ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func extractPragmaLoopCommand(text string) (string, int) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text = stripPragmaLoopThinkBlocks(text)
	matches := extractPragmaLoopBashBlockSpans(text)
	if len(matches) != 1 {
		observe.GlobalTrace("if: len(matches) != 1")
		observe.GlobalTrace("return: \"\", len(matches)")
		return "", len(matches)
	}
	lines := strings.Split(text, "\n")
	before := strings.TrimSpace(strings.Join(lines[:matches[0].startLine], "\n"))
	after := strings.TrimSpace(strings.Join(lines[matches[0].endLine+1:], "\n"))
	if before != "" || after != "" {
		observe.GlobalTrace("if: before != \"\" || after != \"\"")
		observe.GlobalTrace("return: \"\", 2")
		return "", 2
	}
	if pragmaLoopCommandContainsFenceOutsideHeredoc(matches[0].body) {
		observe.GlobalTrace("if: pragmaLoopCommandContainsFenceOutsideHeredoc(matches[0].body)")
		observe.GlobalTrace("return: \"\", 2")
		return "", 2
	}
	observe.GlobalTrace("return: strings.TrimSpace(matches[0].body), 1")
	return strings.TrimSpace(matches[0].body), 1
}

func pragmaLoopCommandContainsFenceOutsideHeredoc(command string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var heredocs []pragmaLoopHeredoc
	for _, line := range strings.Split(command, "\n") {
		observe.GlobalTrace("range strings.Split(command, \"\\n\")")
		if len(heredocs) > 0 {
			observe.GlobalTrace("if: len(heredocs) > 0")
			if pragmaLoopHeredocEnds(line, heredocs[0]) {
				observe.GlobalTrace("if: pragmaLoopHeredocEnds(line, heredocs[0])")
				heredocs = heredocs[1:]
			}
			continue
		}
		if strings.Contains(line, "```") {
			observe.GlobalTrace("if: strings.Contains(line, \"```\")")
			observe.GlobalTrace("return: true")
			return true
		}
		heredocs = append(heredocs, extractPragmaLoopHeredocs(line)...)
	}
	observe.GlobalTrace("return: false")
	return false
}

type pragmaLoopHeredoc struct {
	delimiter string
	stripTabs bool
}

type pragmaLoopBashBlock struct {
	body      string
	startLine int
	endLine   int
}

func extractPragmaLoopBashBlocks(text string) []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	spans := extractPragmaLoopBashBlockSpans(text)
	blocks := make([]string, 0, len(spans))
	for _, span := range spans {
		observe.GlobalTrace("range spans")
		blocks = append(blocks, span.body)
	}
	observe.GlobalTrace("return: blocks")
	return blocks
}

func extractPragmaLoopBashBlockSpans(text string) []pragmaLoopBashBlock {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lines := strings.Split(text, "\n")
	var blocks []pragmaLoopBashBlock
	for i := 0; i < len(lines); i++ {
		observe.GlobalTrace("for: i < len(lines)")
		if strings.TrimSpace(lines[i]) != "```bash" {
			observe.GlobalTrace("if: strings.TrimSpace(lines[i]) != \"```bash\"")
			continue
		}

		start := i + 1
		var heredocs []pragmaLoopHeredoc
		for j := start; j < len(lines); j++ {
			observe.GlobalTrace("for: j < len(lines)")
			line := lines[j]
			if len(heredocs) > 0 {
				observe.GlobalTrace("if: len(heredocs) > 0")
				if pragmaLoopHeredocEnds(line, heredocs[0]) {
					observe.GlobalTrace("if: pragmaLoopHeredocEnds(line, heredocs[0])")
					heredocs = heredocs[1:]
				}
				continue
			}

			if strings.TrimSpace(line) == "```" {
				observe.GlobalTrace("if: strings.TrimSpace(line) == \"```\"")
				blocks = append(blocks, pragmaLoopBashBlock{
					body:      strings.Join(lines[start:j], "\n"),
					startLine: i,
					endLine:   j,
				})
				i = j
				break
			}

			heredocs = append(heredocs, extractPragmaLoopHeredocs(line)...)
		}
	}
	observe.GlobalTrace("return: blocks")
	return blocks
}

func pragmaLoopHeredocEnds(line string, heredoc pragmaLoopHeredoc) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if heredoc.stripTabs {
		observe.GlobalTrace("if: heredoc.stripTabs")
		line = strings.TrimLeft(line, "\t")
	}
	observe.GlobalTrace("return: line == heredoc.delimiter")
	return line == heredoc.delimiter
}

func extractPragmaLoopHeredocs(line string) []pragmaLoopHeredoc {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var heredocs []pragmaLoopHeredoc
	var inSingle, inDouble, escaped bool
	for i := 0; i < len(line); i++ {
		observe.GlobalTrace("for: i < len(line)")
		ch := line[i]
		if escaped {
			observe.GlobalTrace("if: escaped")
			escaped = false
			continue
		}
		if ch == '\\' && !inSingle {
			observe.GlobalTrace("if: ch == '\\\\' && !inSingle")
			escaped = true
			continue
		}
		if ch == '\'' && !inDouble {
			observe.GlobalTrace("if: ch == '\\'' && !inDouble")
			inSingle = !inSingle
			continue
		}
		if ch == '"' && !inSingle {
			observe.GlobalTrace("if: ch == '\"' && !inSingle")
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			observe.GlobalTrace("if: inSingle || inDouble")
			continue
		}
		if ch == '#' {
			observe.GlobalTrace("if: ch == '#'")
			break
		}
		if ch != '<' || i+1 >= len(line) || line[i+1] != '<' {
			observe.GlobalTrace("if: ch != '<' || i+1 >= len(line) || line[i+1] != '<'")
			continue
		}

		pos := i + 2
		stripTabs := false
		if pos < len(line) && line[pos] == '-' {
			observe.GlobalTrace("if: pos < len(line) && line[pos] == '-'")
			stripTabs = true
			pos++
		}
		delimiter, next, ok := readPragmaLoopShellWord(line, pos)
		if ok {
			observe.GlobalTrace("if: ok")
			heredocs = append(heredocs, pragmaLoopHeredoc{
				delimiter: delimiter,
				stripTabs: stripTabs,
			})
			i = next - 1
		}
	}
	observe.GlobalTrace("return: heredocs")
	return heredocs
}

func readPragmaLoopShellWord(line string, pos int) (string, int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for pos < len(line) && (line[pos] == ' ' || line[pos] == '\t') {
		observe.GlobalTrace("for: pos < len(line) && (line[pos] == ' ' || line[pos] == '\\t')")
		pos++
	}

	var b strings.Builder
	var inSingle, inDouble, escaped bool
	for pos < len(line) {
		observe.GlobalTrace("for: pos < len(line)")
		ch := line[pos]
		if escaped {
			observe.GlobalTrace("if: escaped")
			b.WriteByte(ch)
			escaped = false
			pos++
			continue
		}
		if ch == '\\' && !inSingle {
			observe.GlobalTrace("if: ch == '\\\\' && !inSingle")
			escaped = true
			pos++
			continue
		}
		if ch == '\'' && !inDouble {
			observe.GlobalTrace("if: ch == '\\'' && !inDouble")
			inSingle = !inSingle
			pos++
			continue
		}
		if ch == '"' && !inSingle {
			observe.GlobalTrace("if: ch == '\"' && !inSingle")
			inDouble = !inDouble
			pos++
			continue
		}
		if !inSingle && !inDouble && isPragmaLoopShellWordTerminator(ch) {
			observe.GlobalTrace("if: !inSingle && !inDouble && isPragmaLoopShellWordTerminator(ch)")
			break
		}
		b.WriteByte(ch)
		pos++
	}

	if b.Len() == 0 || inSingle || inDouble || escaped {
		observe.GlobalTrace("return: \"\", pos, false")
		return "", pos, false
	}
	observe.GlobalTrace("return: b.String(), pos, true")
	return b.String(), pos, true
}

func isPragmaLoopShellWordTerminator(ch byte) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: ch == ' ' || ch == '\\t' || strings.ContainsRune(\";|&<>()\", rune(ch))")
	return ch == ' ' || ch == '\t' || strings.ContainsRune(";|&<>()", rune(ch))
}

type pragmaLoopBashResult struct {
	ReturnCode int
	Output     string
}

func runPragmaLoopBash(ctx context.Context, workDir, command string) (pragmaLoopBashResult, bool, error) {
	observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "enter")
	defer observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "exit")
	if patch, patchWorkDir, ok, err := applypatch.ExtractShellApplyPatch(command); err != nil {
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "if: err != nil")
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "return: pragmaLoopBashResult{ReturnCode: 1, Output: err.Error()}, false")
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "return: pragmaLoopBashResult{ReturnCode: 1, Output: err.Error()}, false, nil")
		return pragmaLoopBashResult{ReturnCode: 1, Output: err.Error()}, false, nil
	} else if ok {
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "else-if: ok")
		applyWorkDir := workDir
		if patchWorkDir != "" {
			if filepath.IsAbs(patchWorkDir) {
				applyWorkDir = patchWorkDir
			} else {
				applyWorkDir = filepath.Join(workDir, patchWorkDir)
			}
		}
		result, err := applypatch.ApplyPatchText(ctx, patch, applyWorkDir)
		if err != nil {
			return pragmaLoopBashResult{ReturnCode: 1, Output: err.Error()}, false, nil
		}
		return pragmaLoopBashResult{ReturnCode: 0, Output: result.Content}, false, nil
	}
	result, err := shellrun.Execute(ctx, shellrun.Options{
		Command:            command,
		WorkDir:            workDir,
		Timeout:            pragmaLoopCommandTimeout,
		ForegroundWait:     pragmaLoopForegroundWait,
		BaseDirName:        "pragma-loop-bash",
		UsePipefail:        true,
		UseErrexit:         true,
		RunningOutputLines: pragmaLoopRunningOutputLines,
	})
	if err != nil {
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "if: err != nil")
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "return: pragmaLoopBashResult{}, false, fmt.Errorf(\"shell command failed before execut...")
		return pragmaLoopBashResult{}, false, fmt.Errorf("shell command failed before execution: %w", err)
	}
	out := pragmaLoopBashResult{
		ReturnCode: result.ExitCode,
		Output:     result.Output,
	}
	if result.Err != nil && result.ExitCode == -1 && !result.TimedOut && !result.Running {
		observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "if: result.Err != nil && result.ExitCode == -1 && !result.TimedOut && !result.Run...")
		if out.Output != "" {
			observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "if: out.Output != \"\"")
			out.Output += "\n"
		}
		out.Output += result.Err.Error()
	}
	observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "return: out, result.TimedOut")
	observe.TraceCtx(ctx, "query", "runPragmaLoopBash", "return: out, result.TimedOut, nil")
	return out, result.TimedOut, nil
}

func formatPragmaLoopObservation(result pragmaLoopBashResult) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(result.Output) < 10_000 {
		observe.GlobalTrace("if: len(result.Output) < 10_000")
		observe.GlobalTrace("return: fmt.Sprintf(\"<returncode>%d</returncode>\\n<output>\\n%s</output>\", result.Retu...")
		return fmt.Sprintf("<returncode>%d</returncode>\n<output>\n%s</output>", result.ReturnCode, result.Output)
	}
	elidedChars := len(result.Output) - 10_000
	observe.GlobalTrace("return: fmt.Sprintf(`<returncode>%d</returncode>\n<warning>\nThe output of your last co...")
	return fmt.Sprintf(`<returncode>%d</returncode>
<warning>
The output of your last command was too long.
Please try a different command that produces less output.
If you're inspecting a file for patch context, use a bounded file-context read without display-only line numbers.
If a search produced too much output, use a more selective search pattern.
Do not truncate validation commands such as tests or builds as proof of success.
For large validation output, preserve the full output separately, print useful excerpts, and exit with the original status.
</warning>
<output_head>
%s
</output_head>
<elided_chars>
%d characters elided
</elided_chars>
<output_tail>
%s
</output_tail>`, result.ReturnCode, result.Output[:5000], elidedChars, result.Output[len(result.Output)-5000:])
}

func formatPragmaLoopTimeout(command, output string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var body string
	if len(output) < 10_000 {
		observe.GlobalTrace("if: len(output) < 10_000")
		body = fmt.Sprintf("<output>\n%s\n</output>", output)
	} else {
		observe.GlobalTrace("else: len(output) < 10_000")
		body = fmt.Sprintf(`<warning>Output was too long and has been truncated.</warning>
<output_head>
%s
</output_head>
<elided_chars>%d characters elided</elided_chars>
<output_tail>
%s
</output_tail>`, output[:5000], len(output)-10_000, output[len(output)-5000:])
	}
	observe.GlobalTrace("return: fmt.Sprintf(`The last command <command>%s</command> timed out and has been ki...")
	return fmt.Sprintf(`The last command <command>%s</command> timed out and has been killed.
The output of the command was:
%s
Please try another command and make sure to avoid those requiring interactive input.`, command, body)
}

func pragmaLoopSubmitted(result pragmaLoopBashResult) (bool, string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if result.ReturnCode != 0 {
		observe.GlobalTrace("if: result.ReturnCode != 0")
		observe.GlobalTrace("return: false, \"\"")
		return false, ""
	}
	observe.GlobalTrace("return: strings.Contains(result.Output, \"MINI_SWE_AGENT_FINAL_OUTPUT\") ||\n\tstrings.Co...")
	return strings.Contains(result.Output, "MINI_SWE_AGENT_FINAL_OUTPUT") ||
		strings.Contains(result.Output, "COMPLETE_TASK_AND_SUBMIT_FINAL_OUTPUT"), ""
}
