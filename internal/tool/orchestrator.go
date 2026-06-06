package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/toolresult"
)

// Orchestrator executes tool calls with permission checking,
// concurrent/serial partitioning, hook execution, and event emission.
type Orchestrator struct {
	registry *Registry
	checker  permission.Checker
	prompter permission.Prompter
	bus      *observe.EventBus
	hookMgr  *hook.Manager // nil if no hooks configured
}

// NewOrchestrator creates an Orchestrator.
func NewOrchestrator(registry *Registry, checker permission.Checker, prompter permission.Prompter, bus *observe.EventBus) *Orchestrator {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Orchestrator{...}")
	observe.GlobalTrace("return: &Orchestrator{\n\tregistry:\tregistry,\n\tchecker:\tchecker,\n\tprompter:\tprompter,\n\t...")
	return &Orchestrator{
		registry: registry,
		checker:  checker,
		prompter: prompter,
		bus:      bus,
	}
}

// SetHookManager sets the hook manager for PreToolUse/PostToolUse hooks.
func (o *Orchestrator) SetHookManager(mgr *hook.Manager) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	o.hookMgr = mgr
}

func (o *Orchestrator) emitPermissionDecision(traceID, spanID, parentSpan, toolCallID, toolName, decision, userDecision, rule, source string, wasExecuted bool) {
	o.bus.Emit(observe.PermissionDecisionFinal{
		EventHeader:  observe.NewEventHeader("PermissionDecisionFinal", traceID, spanID, parentSpan),
		ToolCallID:   toolCallID,
		ToolName:     toolName,
		Decision:     decision,
		UserDecision: userDecision,
		Rule:         rule,
		Source:       source,
		WasExecuted:  wasExecuted,
	})
}

// ExecuteResult holds the results of a tool batch execution.
type ExecuteResult struct {
	Results             []model.ToolResultPart
	Supplements         []model.ContentPart   // compatibility view of all supplemental content
	SupplementsByResult [][]model.ContentPart // per-result supplemental content, same index as Results
	FileEffects         [][]FileEffect        // per-result file mutation receipts, same index as Results
}

func (r ExecuteResult) ContentParts() []model.ContentPart {
	content := make([]model.ContentPart, 0, len(r.Results)+len(r.Supplements))
	for i, result := range r.Results {
		content = append(content, result)
		if i < len(r.SupplementsByResult) {
			content = append(content, r.SupplementsByResult[i]...)
		}
	}
	if len(r.SupplementsByResult) == 0 {
		content = append(content, r.Supplements...)
	}
	return content
}

// singleResult holds the output of one tool invocation.
type singleResult struct {
	part        model.ToolResultPart
	supplements []model.ContentPart
	fileEffects []FileEffect
}

// Execute runs a batch of tool calls, partitioning into concurrent and serial groups.
// Results are returned in the same order as the input calls.
func (o *Orchestrator) Execute(ctx context.Context, calls []model.ToolCallPart, state StateSnapshot) ExecuteResult {
	observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "enter")
	defer observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "exit")
	singles := make([]singleResult, len(calls))

	// Partition in input order. Consecutive concurrent-safe tools share a batch;
	// each non-concurrent tool forms its own serial batch.
	type indexedCall struct {
		index int
		call  model.ToolCallPart
	}
	type toolBatch struct {
		concurrent bool
		calls      []indexedCall
	}
	var batches []toolBatch
	var concurrentCount, serialCount int

	traceID := observe.NewTraceID()
	batchSpan := observe.NewSpanID()

	for i, call := range calls {
		observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "range calls")
		o.bus.Emit(observe.ToolCallReceived{
			EventHeader:    observe.NewEventHeader("ToolCallReceived", traceID, batchSpan, ""),
			ToolCallID:     call.ID,
			ToolName:       call.Name,
			InputSizeBytes: len(call.Input),
			Input:          call.Input,
		})

		desc, ok := o.registry.Get(call.Name)
		if !ok {
			observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "if: !ok")
			singles[i] = singleResult{
				part: model.ToolResultPart{
					ToolCallID: call.ID,
					Content:    "unknown tool: " + call.Name,
					IsError:    true,
				},
			}
			continue
		}
		ic := indexedCall{index: i, call: call}
		if desc.Flags().Concurrent {
			observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "if: desc.Flags().Concurrent")
			concurrentCount++
			if len(batches) > 0 && batches[len(batches)-1].concurrent {
				observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "if: len(batches) > 0 && batches[len(batches)-1].concurrent")
				batches[len(batches)-1].calls = append(batches[len(batches)-1].calls, ic)
			} else {
				observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "else: len(batches) > 0 && batches[len(batches)-1].concurrent")
				batches = append(batches, toolBatch{concurrent: true, calls: []indexedCall{ic}})
			}
		} else {
			observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "else: desc.Flags().Concurrent")
			serialCount++
			batches = append(batches, toolBatch{calls: []indexedCall{ic}})
		}
	}
	o.bus.Emit(observe.ToolBatchStarted{
		EventHeader:     observe.NewEventHeader("ToolBatchStarted", traceID, batchSpan, ""),
		ConcurrentCount: concurrentCount,
		SerialCount:     serialCount,
		TotalCount:      len(calls),
	})

	batchStart := time.Now()
	var concurrentDuration, serialDuration time.Duration

	for _, batch := range batches {
		observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "range batches")
		if ctx.Err() != nil {
			observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "if: ctx.Err() != nil")
			break
		}
		if !batch.concurrent {
			observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "if: !batch.concurrent")
			serStart := time.Now()
			for _, ic := range batch.calls {
				observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "range batch.calls")
				if ctx.Err() != nil {
					observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "if: ctx.Err() != nil")
					break
				}
				singles[ic.index] = o.executeSingle(ctx, ic.call, state, traceID, batchSpan, false)
			}
			serialDuration += time.Since(serStart)
			continue
		}

		concStart := time.Now()
		g, gctx := errgroup.WithContext(ctx)
		for _, ic := range batch.calls {
			observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "range batch.calls")
			ic := ic
			g.Go(func() error {
				defer func() {
					if r := recover(); r != nil {
						singles[ic.index] = singleResult{
							part: model.ToolResultPart{
								ToolCallID: ic.call.ID,
								Content:    fmt.Sprintf("tool %q panicked: %v", ic.call.Name, r),
								IsError:    true,
							},
						}
						o.bus.Emit(observe.ToolExecutionFailed{
							EventHeader:  observe.NewEventHeader("ToolExecutionFailed", traceID, batchSpan, ""),
							ToolCallID:   ic.call.ID,
							ToolName:     ic.call.Name,
							ErrorType:    "panic",
							ErrorMessage: fmt.Sprintf("%v", r),
						})
					}
				}()
				singles[ic.index] = o.executeSingle(gctx, ic.call, state, traceID, batchSpan, true)
				return gctx.Err()
			})
		}
		if err := g.Wait(); err != nil {
			observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "if: err != nil")
			o.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", traceID, batchSpan, ""),
				Severity:     "warn",
				Component:    "orchestrator",
				ErrorType:    "context_cancelled",
				ErrorMessage: err.Error(),
			})
		}
		concurrentDuration += time.Since(concStart)
	}

	if ctx.Err() != nil {
		observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "if: ctx.Err() != nil")
		for _, batch := range batches {
			observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "range batches")
			for _, ic := range batch.calls {
				observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "range batch.calls")
				if singles[ic.index].part.ToolCallID == "" {
					observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "if: singles[ic.index].part.ToolCallID == \"\"")
					singles[ic.index] = singleResult{
						part: model.ToolResultPart{
							ToolCallID: ic.call.ID,
							Content:    "cancelled: " + ctx.Err().Error(),
							IsError:    true,
						},
					}
				}
			}
		}
	}

	o.bus.Emit(observe.ToolBatchCompleted{
		EventHeader:          observe.NewEventHeader("ToolBatchCompleted", traceID, batchSpan, ""),
		TotalDurationMs:      time.Since(batchStart).Milliseconds(),
		ConcurrentDurationMs: concurrentDuration.Milliseconds(),
		SerialDurationMs:     serialDuration.Milliseconds(),
	})

	out := ExecuteResult{
		Results:             make([]model.ToolResultPart, len(singles)),
		SupplementsByResult: make([][]model.ContentPart, len(singles)),
		FileEffects:         make([][]FileEffect, len(singles)),
	}
	for i, s := range singles {
		observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "range singles")
		out.Results[i] = s.part
		out.SupplementsByResult[i] = append([]model.ContentPart(nil), s.supplements...)
		out.FileEffects[i] = append([]FileEffect(nil), s.fileEffects...)
		out.Supplements = append(out.Supplements, s.supplements...)
	}
	observe.TraceCtx(ctx, "tool", "Orchestrator.Execute", "return: out")
	return out
}

func (o *Orchestrator) executeSingle(
	ctx context.Context,
	call model.ToolCallPart,
	state StateSnapshot,
	traceID, parentSpan string,
	concurrent bool,
) singleResult {
	observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "enter")
	defer observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "exit")
	spanID := observe.NewSpanID()

	desc, _ := o.registry.Get(call.Name)

	if schema := o.registry.GetSchema(call.Name); schema != nil {
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: schema != nil")
		var v any
		if err := json.Unmarshal(call.Input, &v); err != nil {
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: err != nil")
			o.bus.Emit(observe.ToolExecutionFailed{
				EventHeader:  observe.NewEventHeader("ToolExecutionFailed", traceID, spanID, parentSpan),
				ToolCallID:   call.ID,
				ToolName:     call.Name,
				ErrorType:    "json_parse_error",
				ErrorMessage: err.Error(),
			})
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "return: singleResult{\n\tpart: model.ToolResultPart{\n\t\tToolCallID:\tcall.ID,\n\t\tContent:\t...")
			return singleResult{
				part: model.ToolResultPart{
					ToolCallID: call.ID,
					Content:    "invalid JSON input: " + err.Error(),
					IsError:    true,
				},
			}
		}

		if err := schema.Validate(v); err != nil {
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: err != nil")
			msg := err.Error()
			o.bus.Emit(observe.ToolExecutionFailed{
				EventHeader:  observe.NewEventHeader("ToolExecutionFailed", traceID, spanID, parentSpan),
				ToolCallID:   call.ID,
				ToolName:     call.Name,
				ErrorType:    "schema_validation_error",
				ErrorMessage: msg,
			})
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "return: singleResult{\n\tpart: model.ToolResultPart{\n\t\tToolCallID:\tcall.ID,\n\t\tContent:\t...")
			return singleResult{
				part: model.ToolResultPart{
					ToolCallID: call.ID,
					Content:    "Tool input validation failed:\n" + msg,
					IsError:    true,
				},
			}
		}
	}

	var hookSupplements []model.ContentPart
	if o.hookMgr != nil {
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: o.hookMgr != nil")
		hookResult := o.hookMgr.Execute(ctx, hook.PreToolUse, hook.HookInput{
			ToolName:  call.Name,
			ToolInput: call.Input,
		})
		if hookResult.Blocked {
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: hookResult.Blocked")
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "return: singleResult{\n\tpart: model.ToolResultPart{\n\t\tToolCallID:\tcall.ID,\n\t\tContent:\t...")
			return singleResult{
				part: model.ToolResultPart{
					ToolCallID: call.ID,
					Content:    "blocked by hook: " + hookResult.BlockMsg,
					IsError:    true,
				},
			}
		}
		hookSupplements = append(hookSupplements, hookFeedbackSupplements(hook.PreToolUse, hookResult)...)
	}

	permResult := desc.CheckPerm(ctx, call.Input, o.checker)

	rulePattern := ""
	ruleSource := ""
	if permResult.Rule != nil {
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: permResult.Rule != nil")
		rulePattern = permResult.Rule.Content
		if rulePattern == "" {
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: rulePattern == \"\"")
			rulePattern = permResult.Rule.ToolName
		}
		ruleSource = string(permResult.Rule.Source)
	}
	o.bus.Emit(observe.ToolPermissionChecked{
		EventHeader: observe.NewEventHeader("ToolPermissionChecked", traceID, spanID, parentSpan),
		ToolCallID:  call.ID,
		ToolName:    call.Name,
		Decision:    string(permResult.Decision),
		Rule:        rulePattern,
		Source:      ruleSource,
	})

	if permResult.Decision == permission.DecisionDeny {
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: permResult.Decision == permission.DecisionDeny")
		o.emitPermissionDecision(traceID, spanID, parentSpan, call.ID, call.Name, string(permResult.Decision), "", rulePattern, ruleSource, false)
		o.bus.Emit(observe.PermissionDenialEnforced{
			EventHeader: observe.NewEventHeader("PermissionDenialEnforced", traceID, spanID, parentSpan),
			ToolCallID:  call.ID,
			ToolName:    call.Name,
			WasExecuted: false,
		})
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "return: singleResult{\n\tpart: model.ToolResultPart{\n\t\tToolCallID:\tcall.ID,\n\t\tContent:\t...")
		return singleResult{
			part: model.ToolResultPart{
				ToolCallID: call.ID,
				Content:    "permission denied: " + permResult.Reason,
				IsError:    true,
			},
			supplements: hookSupplements,
		}
	}

	finalDecision := permResult.Decision
	userDecision := ""
	if permResult.Decision == permission.DecisionAsk {
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: permResult.Decision == permission.DecisionAsk")
		o.bus.Emit(observe.ToolPermissionPromptStarted{
			EventHeader: observe.NewEventHeader("ToolPermissionPromptStarted", traceID, spanID, parentSpan),
			ToolCallID:  call.ID,
			ToolName:    call.Name,
		})
		promptStart := time.Now()
		decision, rememberScope := o.prompter.Prompt(ctx, call.Name, call.Input, permResult.Content, permResult.Reason)
		promptDuration := time.Since(promptStart)
		finalDecision = decision
		userDecision = string(decision)

		if rememberScope != permission.RememberNone {
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: rememberScope != permission.RememberNone")
			rule := permission.SessionRuleForPrompt(call.Name, permResult, decision)
			switch rememberScope {
			case permission.RememberSession:
				o.checker.AddSessionRule(rule)
			case permission.RememberPersistent:
				if err := o.checker.AddPersistentRule(rule); err != nil {
					o.bus.Emit(observe.ErrorOccurred{
						EventHeader:  observe.NewEventHeader("ErrorOccurred", traceID, spanID, parentSpan),
						Severity:     "warn",
						Component:    "permission",
						ErrorType:    "persist_rule_failed",
						ErrorMessage: fmt.Sprintf("persist permission rule for %s: %v", call.Name, err),
					})
					o.checker.AddSessionRule(rule)
				} else {
					o.bus.Emit(observe.PermissionPersisted{
						EventHeader: observe.NewEventHeader("PermissionPersisted", traceID, spanID, parentSpan),
						ToolName:    rule.ToolName,
						Content:     rule.Content,
						Decision:    string(rule.Decision),
					})
				}
			default:
				o.bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", traceID, spanID, parentSpan),
					Severity:     "warn",
					Component:    "permission",
					ErrorType:    "unknown_remember_scope",
					ErrorMessage: fmt.Sprintf("unknown remember scope %q for %s", rememberScope, call.Name),
				})
				o.checker.AddSessionRule(rule)
			}
		}

		o.bus.Emit(observe.ToolPermissionPrompted{
			EventHeader:  observe.NewEventHeader("ToolPermissionPrompted", traceID, spanID, parentSpan),
			ToolCallID:   call.ID,
			ToolName:     call.Name,
			UserDecision: string(decision),
			DurationMs:   promptDuration.Milliseconds(),
		})

		if decision != permission.DecisionAllow {
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: decision != permission.DecisionAllow")
			o.emitPermissionDecision(traceID, spanID, parentSpan, call.ID, call.Name, string(decision), userDecision, rulePattern, ruleSource, false)
			o.bus.Emit(observe.PermissionDenialEnforced{
				EventHeader: observe.NewEventHeader("PermissionDenialEnforced", traceID, spanID, parentSpan),
				ToolCallID:  call.ID,
				ToolName:    call.Name,
				WasExecuted: false,
			})
			observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "return: singleResult{\n\tpart: model.ToolResultPart{\n\t\tToolCallID:\tcall.ID,\n\t\tContent:\t...")
			return singleResult{
				part: model.ToolResultPart{
					ToolCallID: call.ID,
					Content:    "permission denied by user",
					IsError:    true,
				},
				supplements: hookSupplements,
			}
		}
	}

	if ctx.Err() != nil {
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: ctx.Err() != nil")
		o.emitPermissionDecision(traceID, spanID, parentSpan, call.ID, call.Name, string(finalDecision), userDecision, rulePattern, ruleSource, false)
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "return: singleResult{\n\tpart: model.ToolResultPart{\n\t\tToolCallID:\tcall.ID,\n\t\tContent:\t...")
		return singleResult{
			part: model.ToolResultPart{
				ToolCallID: call.ID,
				Content:    "cancelled: " + ctx.Err().Error(),
				IsError:    true,
			},
			supplements: hookSupplements,
		}
	}

	o.emitPermissionDecision(traceID, spanID, parentSpan, call.ID, call.Name, string(finalDecision), userDecision, rulePattern, ruleSource, true)
	o.bus.Emit(observe.ToolExecutionStarted{
		EventHeader:    observe.NewEventHeader("ToolExecutionStarted", traceID, spanID, parentSpan),
		ToolCallID:     call.ID,
		ToolName:       call.Name,
		Concurrent:     concurrent,
		Input:          call.Input,
		InputSizeBytes: len(call.Input),
	})

	var fileState *FileStateCache
	var fileEffectCursor int
	if cache, ok := FileStateCacheFrom(state); ok {
		fileState = cache
		fileEffectCursor = cache.EffectCursor()
	}
	start := time.Now()
	invokeCtx := WithInvocationContext(ctx, InvocationContext{
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: parentSpan,
		ToolCallID:   call.ID,
		ToolName:     call.Name,
	})
	invokeResult, err := desc.Invoke(invokeCtx, call.Input, state)
	duration := time.Since(start)
	var fileEffects []FileEffect
	if fileState != nil {
		fileEffects = fileState.EffectsSince(fileEffectCursor)
	}

	if err != nil {
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: err != nil")
		o.bus.Emit(observe.ToolExecutionFailed{
			EventHeader:  observe.NewEventHeader("ToolExecutionFailed", traceID, spanID, parentSpan),
			ToolCallID:   call.ID,
			ToolName:     call.Name,
			ErrorType:    "invocation_error",
			ErrorMessage: err.Error(),
		})
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "return: singleResult{\n\tpart: model.ToolResultPart{\n\t\tToolCallID:\tcall.ID,\n\t\tContent:\t...")
		return singleResult{
			part: model.ToolResultPart{
				ToolCallID: call.ID,
				Content:    err.Error(),
				IsError:    true,
			},
			supplements: hookSupplements,
			fileEffects: fileEffects,
		}
	}

	o.bus.Emit(observe.ToolExecutionCompleted{
		EventHeader:     observe.NewEventHeader("ToolExecutionCompleted", traceID, spanID, parentSpan),
		ToolCallID:      call.ID,
		ToolName:        call.Name,
		DurationMs:      duration.Milliseconds(),
		OutputSizeBytes: len(invokeResult.Content),
		IsError:         false,
		Output:          invokeResult.Content,
	})

	if o.hookMgr != nil {
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: o.hookMgr != nil")
		postHookResult := o.hookMgr.Execute(ctx, hook.PostToolUse, hook.HookInput{
			ToolName:  call.Name,
			ToolInput: call.Input,
			Response:  invokeResult.Content,
		})
		hookSupplements = append(hookSupplements, hookFeedbackSupplements(hook.PostToolUse, postHookResult)...)
	}
	observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "return: singleResult{\n\tpart: model.ToolResultPart{\n\t\tToolCallID:\tcall.ID,\n\t\tContent:\t...")

	part := model.ToolResultPart{
		ToolCallID: call.ID,
		Content:    invokeResult.Content,
	}
	sessionID, _ := SessionIDFrom(state)
	if processed, processErr := toolresult.ProcessToolResult(part, call.Name, desc.Flags().MaxResultSizeChars, sessionID); processErr == nil {
		observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "if: processErr == nil")
		part = processed
	}
	observe.TraceCtx(ctx, "tool", "Orchestrator.executeSingle", "return: singleResult{\n\tpart:\t\tpart,\n\tsupplements:\tinv...")

	supplements := make([]model.ContentPart, 0, len(hookSupplements)+len(invokeResult.Supplements))
	supplements = append(supplements, hookSupplements...)
	supplements = append(supplements, invokeResult.Supplements...)

	return singleResult{
		part:        part,
		supplements: supplements,
		fileEffects: fileEffects,
	}
}

func hookFeedbackSupplements(event hook.Event, result hook.AggregatedResult) []model.ContentPart {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	text := hookFeedbackText(event, result.Feedback)
	if text == "" {
		observe.GlobalTrace("if: text == \"\"")
		return nil
	}
	observe.GlobalTrace("return: []model.ContentPart{model.TextPart{Text: text}}")
	return []model.ContentPart{model.TextPart{Text: text}}
}

func hookFeedbackText(event hook.Event, feedback []string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cleaned := make([]string, 0, len(feedback))
	for _, msg := range feedback {
		observe.GlobalTrace("range feedback")
		if trimmed := strings.TrimSpace(msg); trimmed != "" {
			observe.GlobalTrace("if: trimmed != \"\"")
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		observe.GlobalTrace("if: len(cleaned) == 0")
		return ""
	}
	observe.GlobalTrace("return: \"Hook context from \" + string(event) + \":\\n\" + strings.Join(cleaned, \"\\n\\n\")")
	return "Hook context from " + string(event) + ":\n" + strings.Join(cleaned, "\n\n")
}
