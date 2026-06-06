package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/observe"
	"golang.org/x/sync/errgroup"
)

// ErrMaxStepsExceeded signals that the graph hit its cycle safety limit.
var ErrMaxStepsExceeded = errors.New("lifecycle: max steps exceeded")

// Executor runs a Graph to completion using the superstep model.
// Each superstep executes all pending nodes in parallel, merges their
// state updates, then determines the next set of pending nodes via edges.
type Executor struct {
	graph        *Graph
	bus          *observe.EventBus
	checkpointer Checkpointer
}

// ExecutorOption configures an Executor.
type ExecutorOption func(*Executor)

// WithEventBus attaches observability. Events are emitted at each transition.
func WithEventBus(bus *observe.EventBus) ExecutorOption {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(e *Executor) { e.bus = bus }")
	return func(e *Executor) { e.bus = bus }
}

// WithCheckpointer enables state persistence after each superstep.
func WithCheckpointer(cp Checkpointer) ExecutorOption {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(e *Executor) { e.checkpointer = cp }")
	return func(e *Executor) { e.checkpointer = cp }
}

// NewExecutor creates an executor for the given graph.
func NewExecutor(graph *Graph, opts ...ExecutorOption) *Executor {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	e := &Executor{graph: graph}
	for _, opt := range opts {
		observe.GlobalTrace("range opts")
		opt(e)
	}
	observe.GlobalTrace("return: e")
	return e
}

// Run executes the graph from initial state to termination.
// Returns the final state and any error.
func (e *Executor) Run(ctx context.Context, initial State) (State, error) {
	observe.TraceCtx(ctx, "lifecycle", "Executor.Run", "enter")
	defer observe.TraceCtx(ctx, "lifecycle", "Executor.Run", "exit")
	state := initial.Snapshot()
	for ev := range e.Stream(ctx, initial) {
		observe.TraceCtx(ctx, "lifecycle", "Executor.Run", "range e.Stream(ctx, initial)")
		if ev.Type != "completed" {
			observe.TraceCtx(ctx, "lifecycle", "Executor.Run", "if: ev.Type != \"completed\"")
			continue
		}
		if ev.State != nil {
			observe.TraceCtx(ctx, "lifecycle", "Executor.Run", "if: ev.State != nil")
			state = ev.State.Snapshot()
		}
		observe.TraceCtx(ctx, "lifecycle", "Executor.Run", "return: state, ev.Err")
		return state, ev.Err
	}
	observe.TraceCtx(ctx, "lifecycle", "Executor.Run", "return: state, nil")
	return state, nil
}

// Stream executes the graph and sends events to a channel.
func (e *Executor) Stream(ctx context.Context, initial State) <-chan ExecutionEvent {
	observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "enter")
	defer observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "exit")
	ch := make(chan ExecutionEvent, 16)
	go func() {
		defer close(ch)
		state := initial.Snapshot()
		pending := []string{e.graph.initialNode}
		step := 0

		for len(pending) > 0 && step < e.graph.maxSteps {
			observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "for: len(pending) > 0 && step < e.graph.maxSteps")
			step++
			if ctx.Err() != nil {
				observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "if: ctx.Err() != nil")
				err := fmt.Errorf("lifecycle: context cancelled at step %d: %w", step, ctx.Err())
				e.emitCompleted(step, err)
				ch <- ExecutionEvent{Type: "completed", Step: step, State: state.Snapshot(), Err: err}
				return
			}

			ch <- ExecutionEvent{Type: "step_started", Step: step, Nodes: append([]string(nil), pending...)}

			updates, nodeErrors, nodeDurations := e.executeSuperstep(ctx, step, pending, state)

			for node, err := range nodeErrors {
				observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "range nodeErrors")
				dur := nodeDurations[node]
				if err != nil {
					observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "if: err != nil")
					err = fmt.Errorf("lifecycle: node %q failed at step %d: %w", node, step, err)
					ch <- ExecutionEvent{Type: "node_completed", Step: step, Node: node, Duration: dur, Err: err}
					e.emitCompleted(step, err)
					ch <- ExecutionEvent{Type: "completed", Step: step, State: state.Snapshot(), Err: err}
					return
				}
				ch <- ExecutionEvent{Type: "node_completed", Step: step, Node: node, Duration: dur}
			}

			for _, update := range updates {
				observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "range updates")
				state = applyUpdate(state, update, e.graph.reducers)
			}

			if e.checkpointer != nil {
				observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "if: e.checkpointer != nil")
				if cpErr := e.checkpointer.Save(step, state); cpErr != nil {
					observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "if: cpErr != nil")
					err := fmt.Errorf("lifecycle: checkpoint save failed at step %d: %w", step, cpErr)
					e.emitCompleted(step, err)
					ch <- ExecutionEvent{Type: "completed", Step: step, State: state.Snapshot(), Err: err}
					return
				}
			}

			nextPending, err := e.resolveNextNodes(step, pending, state, func(ev ExecutionEvent) {
				ch <- ev
			})
			if err != nil {
				e.emitCompleted(step, err)
				ch <- ExecutionEvent{Type: "completed", Step: step, State: state.Snapshot(), Err: err}
				return
			}
			pending = nextPending
		}

		var err error
		if step >= e.graph.maxSteps && len(pending) > 0 {
			observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "if: step >= e.graph.maxSteps && len(pending) > 0")
			err = ErrMaxStepsExceeded
		}
		e.emitCompleted(step, err)
		ch <- ExecutionEvent{Type: "completed", Step: step, State: state.Snapshot(), Err: err}
	}()
	observe.TraceCtx(ctx, "lifecycle", "Executor.Stream", "return: ch")
	return ch
}

// executeSuperstep runs all pending nodes in parallel.
// Returns ordered updates, per-node errors, and per-node durations.
func (e *Executor) executeSuperstep(ctx context.Context, step int, pending []string, state State) ([]StateUpdate, map[string]error, map[string]time.Duration) {
	observe.TraceCtx(ctx, "lifecycle", "Executor.executeSuperstep", "enter")
	defer observe.TraceCtx(ctx, "lifecycle", "Executor.executeSuperstep", "exit")
	updates := make([]StateUpdate, len(pending))
	nodeErrors := make(map[string]error, len(pending))
	nodeDurations := make(map[string]time.Duration, len(pending))

	if len(pending) == 1 {
		observe.TraceCtx(ctx, "lifecycle", "Executor.executeSuperstep", "if: len(pending) == 1")

		node := pending[0]
		start := time.Now()
		update, err := e.graph.nodes[node](ctx, state.Snapshot())
		dur := time.Since(start)
		nodeDurations[node] = dur

		if err != nil {
			observe.TraceCtx(ctx, "lifecycle", "Executor.executeSuperstep", "if: err != nil")
			nodeErrors[node] = err
			e.emitNodeCompleted(step, node, dur, err)
		} else {
			observe.TraceCtx(ctx, "lifecycle", "Executor.executeSuperstep", "else: err != nil")
			updates[0] = update
			e.emitNodeCompleted(step, node, dur, nil)
		}
		observe.TraceCtx(ctx, "lifecycle", "Executor.executeSuperstep", "return: updates, nodeErrors, nodeDurations")
		return updates, nodeErrors, nodeDurations
	}

	// Parallel execution via errgroup
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)

	for i, node := range pending {
		observe.TraceCtx(ctx, "lifecycle", "Executor.executeSuperstep", "range pending")
		i, node := i, node
		snapshot := state.Snapshot()
		g.Go(func() error {
			start := time.Now()
			update, err := e.graph.nodes[node](gctx, snapshot)
			dur := time.Since(start)

			mu.Lock()
			defer mu.Unlock()
			nodeDurations[node] = dur

			if err != nil {
				nodeErrors[node] = err
				e.emitNodeCompleted(step, node, dur, err)
				return err
			}
			updates[i] = update
			e.emitNodeCompleted(step, node, dur, nil)
			return nil
		})
	}

	g.Wait()
	observe.TraceCtx(ctx, "lifecycle", "Executor.executeSuperstep", "return: updates, nodeErrors, nodeDurations")
	return updates, nodeErrors, nodeDurations
}

// resolveNextNodes determines which nodes to execute next based on edges.
func (e *Executor) resolveNextNodes(step int, completed []string, state State, emit func(ExecutionEvent)) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	seen := make(map[string]bool)
	var next []string

	for _, node := range completed {
		observe.GlobalTrace("range completed")

		if targets, ok := e.graph.edges[node]; ok {
			observe.GlobalTrace("if: ok")
			for _, target := range targets {
				observe.GlobalTrace("range targets")
				e.emitTransition(step, node, target, "")
				if emit != nil {
					observe.GlobalTrace("if: emit != nil")
					emit(ExecutionEvent{Type: "transition", Step: step, FromNode: node, ToNode: target})
				}
				if !seen[target] {
					observe.GlobalTrace("if: !seen[target]")
					seen[target] = true
					next = append(next, target)
				}
			}
			continue
		}

		if ce, ok := e.graph.conditionalEdges[node]; ok {
			observe.GlobalTrace("if: ok")
			key := ce.Router(state)
			target, ok := ce.PathMap[key]
			if !ok {
				observe.GlobalTrace("if: !ok")
				return nil, fmt.Errorf("lifecycle: node %q router returned unmapped route key %q", node, key)
			}
			if target == "" {
				observe.GlobalTrace("if: target == \"\"")

				e.emitTransition(step, node, "END", key)
				if emit != nil {
					observe.GlobalTrace("if: emit != nil")
					emit(ExecutionEvent{Type: "transition", Step: step, FromNode: node, ToNode: "END", RouteKey: key})
				}
				continue
			}
			e.emitTransition(step, node, target, key)
			if emit != nil {
				observe.GlobalTrace("if: emit != nil")
				emit(ExecutionEvent{Type: "transition", Step: step, FromNode: node, ToNode: target, RouteKey: key})
			}
			if !seen[target] {
				observe.GlobalTrace("if: !seen[target]")
				seen[target] = true
				next = append(next, target)
			}
		}

	}
	observe.GlobalTrace("return: next")

	return next, nil
}

func (e *Executor) emitStepStarted(step int, nodes []string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.bus == nil {
		observe.GlobalTrace("if: e.bus == nil")
		return
	}
	e.bus.Emit(observe.LifecycleStepStarted{
		EventHeader: observe.NewEventHeader("LifecycleStepStarted", "", "", ""),
		Step:        step,
		Nodes:       nodes,
	})
}

func (e *Executor) emitNodeCompleted(step int, node string, dur time.Duration, err error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.bus == nil {
		observe.GlobalTrace("if: e.bus == nil")
		return
	}
	errStr := ""
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		errStr = err.Error()
	}
	e.bus.Emit(observe.LifecycleNodeCompleted{
		EventHeader: observe.NewEventHeader("LifecycleNodeCompleted", "", "", ""),
		Step:        step,
		Node:        node,
		Duration:    dur,
		Error:       errStr,
	})
}

func (e *Executor) emitTransition(step int, from, to, routeKey string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.bus == nil {
		observe.GlobalTrace("if: e.bus == nil")
		return
	}
	e.bus.Emit(observe.LifecycleTransition{
		EventHeader: observe.NewEventHeader("LifecycleTransition", "", "", ""),
		Step:        step,
		From:        from,
		To:          to,
		RouteKey:    routeKey,
	})
}

func (e *Executor) emitCompleted(totalSteps int, err error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if e.bus == nil {
		observe.GlobalTrace("if: e.bus == nil")
		return
	}
	errStr := ""
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		errStr = err.Error()
	}
	e.bus.Emit(observe.LifecycleCompleted{
		EventHeader: observe.NewEventHeader("LifecycleCompleted", "", "", ""),
		TotalSteps:  totalSteps,
		Error:       errStr,
	})
}
