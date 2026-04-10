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
	bus      *observe.EventBus
}

// NewOrchestrator creates an Orchestrator.
func NewOrchestrator(registry *Registry, checker permission.Checker, bus *observe.EventBus) *Orchestrator {
	return &Orchestrator{
		registry: registry,
		checker:  checker,
		bus:      bus,
	}
}

// Execute runs a batch of tool calls, partitioning into concurrent and serial groups.
// Results are returned in the same order as the input calls.
func (o *Orchestrator) Execute(ctx context.Context, calls []model.ToolCallPart, state StateSnapshot) []model.ToolResultPart {
	results := make([]model.ToolResultPart, len(calls))

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
			results[i] = model.ToolResultPart{
				ToolCallID: call.ID,
				Content:    "unknown tool: " + call.Name,
				IsError:    true,
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
				results[ic.index] = o.executeSingle(gctx, ic.call, state, traceID, batchSpan, true)
				return nil
			})
		}
		_ = g.Wait()
		concurrentDuration = time.Since(concStart)
	}

	// Run serial tools
	if len(serial) > 0 {
		serStart := time.Now()
		for _, ic := range serial {
			results[ic.index] = o.executeSingle(ctx, ic.call, state, traceID, batchSpan, false)
		}
		serialDuration = time.Since(serStart)
	}

	o.bus.Emit(observe.ToolBatchCompleted{
		EventHeader:          observe.NewEventHeader("ToolBatchCompleted", traceID, batchSpan, ""),
		TotalDurationMs:      time.Since(batchStart).Milliseconds(),
		ConcurrentDurationMs: concurrentDuration.Milliseconds(),
		SerialDurationMs:     serialDuration.Milliseconds(),
	})

	return results
}

func (o *Orchestrator) executeSingle(
	ctx context.Context,
	call model.ToolCallPart,
	state StateSnapshot,
	traceID, parentSpan string,
	concurrent bool,
) model.ToolResultPart {
	spanID := observe.NewSpanID()

	// Tool is guaranteed to exist — unknown tools are filtered in Execute()
	desc, _ := o.registry.Get(call.Name)

	// Permission check
	result := desc.CheckPerm(ctx, call.Input, o.checker)
	o.bus.Emit(observe.ToolPermissionChecked{
		EventHeader: observe.NewEventHeader("ToolPermissionChecked", traceID, spanID, parentSpan),
		ToolCallID:  call.ID,
		ToolName:    call.Name,
		Decision:    string(result.Decision),
		Rule:        result.Rule.Pattern,
		Source:      result.Rule.Source,
	})

	if result.Decision == permission.DecisionDeny {
		o.bus.Emit(observe.PermissionDenialEnforced{
			EventHeader: observe.NewEventHeader("PermissionDenialEnforced", traceID, spanID, parentSpan),
			ToolCallID:  call.ID,
			ToolName:    call.Name,
			WasExecuted: false,
		})
		return model.ToolResultPart{
			ToolCallID: call.ID,
			Content:    "permission denied: " + result.Reason,
			IsError:    true,
		}
	}

	if result.Decision == permission.DecisionAsk {
		// "Ask" means the user must confirm before execution. The TUI prompt
		// integration will replace this deny with an interactive flow.
		o.bus.Emit(observe.PermissionDenialEnforced{
			EventHeader: observe.NewEventHeader("PermissionDenialEnforced", traceID, spanID, parentSpan),
			ToolCallID:  call.ID,
			ToolName:    call.Name,
			WasExecuted: false,
		})
		return model.ToolResultPart{
			ToolCallID: call.ID,
			Content:    "permission requires user confirmation (not yet implemented)",
			IsError:    true,
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
	output, err := desc.Invoke(ctx, call.Input, state)
	duration := time.Since(start)

	if err != nil {
		o.bus.Emit(observe.ToolExecutionFailed{
			EventHeader:  observe.NewEventHeader("ToolExecutionFailed", traceID, spanID, parentSpan),
			ToolCallID:   call.ID,
			ToolName:     call.Name,
			ErrorType:    "invocation_error",
			ErrorMessage: err.Error(),
		})
		return model.ToolResultPart{
			ToolCallID: call.ID,
			Content:    err.Error(),
			IsError:    true,
		}
	}

	o.bus.Emit(observe.ToolExecutionCompleted{
		EventHeader:     observe.NewEventHeader("ToolExecutionCompleted", traceID, spanID, parentSpan),
		ToolCallID:      call.ID,
		ToolName:        call.Name,
		DurationMs:      duration.Milliseconds(),
		OutputSizeBytes: len(output),
		IsError:         false,
	})

	return model.ToolResultPart{
		ToolCallID: call.ID,
		Content:    output,
	}
}
