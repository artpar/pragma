package tool

import (
	"context"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
)

// Orchestrator executes tool calls with permission checking,
// concurrent/serial partitioning, and event emission.
type Orchestrator struct {
	registry *Registry
	checker  permission.Checker
	prompter permission.Prompter
	bus      *observe.EventBus
}

// NewOrchestrator creates an Orchestrator.
func NewOrchestrator(registry *Registry, checker permission.Checker, prompter permission.Prompter, bus *observe.EventBus) *Orchestrator {
	return &Orchestrator{
		registry: registry,
		checker:  checker,
		prompter: prompter,
		bus:      bus,
	}
}

// ExecuteResult holds the results of a tool batch execution.
type ExecuteResult struct {
	Results     []model.ToolResultPart
	Supplements []model.ContentPart // additional content parts (e.g., DocumentPart for PDFs)
}

// singleResult holds the output of one tool invocation.
type singleResult struct {
	part        model.ToolResultPart
	supplements []model.ContentPart
}

// Execute runs a batch of tool calls, partitioning into concurrent and serial groups.
// Results are returned in the same order as the input calls.
func (o *Orchestrator) Execute(ctx context.Context, calls []model.ToolCallPart, state StateSnapshot) ExecuteResult {
	singles := make([]singleResult, len(calls))

	// Partition
	type indexedCall struct {
		index int
		call  model.ToolCallPart
	}
	var concurrent, serial []indexedCall

	traceID := observe.NewTraceID()
	batchSpan := observe.NewSpanID()

	for i, call := range calls {
		o.bus.Emit(observe.ToolCallReceived{
			EventHeader:    observe.NewEventHeader("ToolCallReceived", traceID, batchSpan, ""),
			ToolCallID:     call.ID,
			ToolName:       call.Name,
			InputSizeBytes: len(call.Input),
		})

		desc, ok := o.registry.Get(call.Name)
		if !ok {
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
			concurrent = append(concurrent, ic)
		} else {
			serial = append(serial, ic)
		}
	}
	o.bus.Emit(observe.ToolBatchStarted{
		EventHeader:     observe.NewEventHeader("ToolBatchStarted", traceID, batchSpan, ""),
		ConcurrentCount: len(concurrent),
		SerialCount:     len(serial),
		TotalCount:      len(calls),
	})

	batchStart := time.Now()
	var concurrentDuration, serialDuration time.Duration

	// Run concurrent tools
	if len(concurrent) > 0 {
		concStart := time.Now()
		g, gctx := errgroup.WithContext(ctx)
		for _, ic := range concurrent {
			ic := ic
			g.Go(func() error {
				singles[ic.index] = o.executeSingle(gctx, ic.call, state, traceID, batchSpan, true)
				return gctx.Err()
			})
		}
		if err := g.Wait(); err != nil {
			o.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", traceID, batchSpan, ""),
				Severity:     "warn",
				Component:    "orchestrator",
				ErrorType:    "context_cancelled",
				ErrorMessage: err.Error(),
			})
		}
		concurrentDuration = time.Since(concStart)
	}

	// Run serial tools
	if len(serial) > 0 {
		serStart := time.Now()
		for _, ic := range serial {
			singles[ic.index] = o.executeSingle(ctx, ic.call, state, traceID, batchSpan, false)
		}
		serialDuration = time.Since(serStart)
	}

	o.bus.Emit(observe.ToolBatchCompleted{
		EventHeader:          observe.NewEventHeader("ToolBatchCompleted", traceID, batchSpan, ""),
		TotalDurationMs:      time.Since(batchStart).Milliseconds(),
		ConcurrentDurationMs: concurrentDuration.Milliseconds(),
		SerialDurationMs:     serialDuration.Milliseconds(),
	})

	// Collect results and supplements
	out := ExecuteResult{
		Results: make([]model.ToolResultPart, len(singles)),
	}
	for i, s := range singles {
		out.Results[i] = s.part
		out.Supplements = append(out.Supplements, s.supplements...)
	}
	return out
}

func (o *Orchestrator) executeSingle(
	ctx context.Context,
	call model.ToolCallPart,
	state StateSnapshot,
	traceID, parentSpan string,
	concurrent bool,
) singleResult {
	spanID := observe.NewSpanID()

	// Tool is guaranteed to exist — unknown tools are filtered in Execute()
	desc, _ := o.registry.Get(call.Name)

	// Permission check
	permResult := desc.CheckPerm(ctx, call.Input, o.checker)

	rulePattern := ""
	ruleSource := ""
	if permResult.Rule != nil {
		rulePattern = permResult.Rule.Content
		if rulePattern == "" {
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
		o.bus.Emit(observe.PermissionDenialEnforced{
			EventHeader: observe.NewEventHeader("PermissionDenialEnforced", traceID, spanID, parentSpan),
			ToolCallID:  call.ID,
			ToolName:    call.Name,
			WasExecuted: false,
		})
		return singleResult{
			part: model.ToolResultPart{
				ToolCallID: call.ID,
				Content:    "permission denied: " + permResult.Reason,
				IsError:    true,
			},
		}
	}

	if permResult.Decision == permission.DecisionAsk {
		promptStart := time.Now()
		decision, sessionRule := o.prompter.Prompt(ctx, call.Name, permResult.Content, permResult.Reason)
		promptDuration := time.Since(promptStart)

		if sessionRule != nil {
			o.checker.AddSessionRule(*sessionRule)
		}

		o.bus.Emit(observe.ToolPermissionPrompted{
			EventHeader:  observe.NewEventHeader("ToolPermissionPrompted", traceID, spanID, parentSpan),
			ToolCallID:   call.ID,
			ToolName:     call.Name,
			UserDecision: string(decision),
			DurationMs:   promptDuration.Milliseconds(),
		})

		if decision != permission.DecisionAllow {
			o.bus.Emit(observe.PermissionDenialEnforced{
				EventHeader: observe.NewEventHeader("PermissionDenialEnforced", traceID, spanID, parentSpan),
				ToolCallID:  call.ID,
				ToolName:    call.Name,
				WasExecuted: false,
			})
			return singleResult{
				part: model.ToolResultPart{
					ToolCallID: call.ID,
					Content:    "permission denied by user",
					IsError:    true,
				},
			}
		}
	}

	// Check context before execution
	if ctx.Err() != nil {
		return singleResult{
			part: model.ToolResultPart{
				ToolCallID: call.ID,
				Content:    "cancelled: " + ctx.Err().Error(),
				IsError:    true,
			},
		}
	}

	// Execute
	o.bus.Emit(observe.ToolExecutionStarted{
		EventHeader: observe.NewEventHeader("ToolExecutionStarted", traceID, spanID, parentSpan),
		ToolCallID:  call.ID,
		ToolName:    call.Name,
		Concurrent:  concurrent,
	})

	start := time.Now()
	invokeResult, err := desc.Invoke(ctx, call.Input, state)
	duration := time.Since(start)

	if err != nil {
		o.bus.Emit(observe.ToolExecutionFailed{
			EventHeader:  observe.NewEventHeader("ToolExecutionFailed", traceID, spanID, parentSpan),
			ToolCallID:   call.ID,
			ToolName:     call.Name,
			ErrorType:    "invocation_error",
			ErrorMessage: err.Error(),
		})
		return singleResult{
			part: model.ToolResultPart{
				ToolCallID: call.ID,
				Content:    err.Error(),
				IsError:    true,
			},
		}
	}

	o.bus.Emit(observe.ToolExecutionCompleted{
		EventHeader:     observe.NewEventHeader("ToolExecutionCompleted", traceID, spanID, parentSpan),
		ToolCallID:      call.ID,
		ToolName:        call.Name,
		DurationMs:      duration.Milliseconds(),
		OutputSizeBytes: len(invokeResult.Content),
		IsError:         false,
	})

	return singleResult{
		part: model.ToolResultPart{
			ToolCallID: call.ID,
			Content:    invokeResult.Content,
		},
		supplements: invokeResult.Supplements,
	}
}
