package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/artpar/gogent/internal/observe"
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
	return func(e *Executor) { e.bus = bus }
}

// WithCheckpointer enables state persistence after each superstep.
func WithCheckpointer(cp Checkpointer) ExecutorOption {
	return func(e *Executor) { e.checkpointer = cp }
}

// NewExecutor creates an executor for the given graph.
func NewExecutor(graph *Graph, opts ...ExecutorOption) *Executor {
	e := &Executor{graph: graph}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run executes the graph from initial state to termination.
// Returns the final state and any error.
func (e *Executor) Run(ctx context.Context, initial State) (State, error) {
	state := initial.Snapshot()
	pending := []string{e.graph.initialNode}
	step := 0

	for len(pending) > 0 && step < e.graph.maxSteps {
		step++

		if err := ctx.Err(); err != nil {
			return state, fmt.Errorf("lifecycle: context cancelled at step %d: %w", step, err)
		}

		e.emitStepStarted(step, pending)

		// Execute all pending nodes in parallel (superstep)
		updates, nodeErrors := e.executeSuperstep(ctx, step, pending, state)

		// Check for fatal node errors
		for node, err := range nodeErrors {
			if err != nil {
				return state, fmt.Errorf("lifecycle: node %q failed at step %d: %w", node, step, err)
			}
		}

		// Merge all updates into state using reducers
		for _, update := range updates {
			state = applyUpdate(state, update, e.graph.reducers)
		}

		// Checkpoint after superstep
		if e.checkpointer != nil {
			if err := e.checkpointer.Save(step, state); err != nil {
				return state, fmt.Errorf("lifecycle: checkpoint save failed at step %d: %w", step, err)
			}
		}

		// Determine next pending set from edges
		pending = e.resolveNextNodes(step, pending, state)
	}

	if step >= e.graph.maxSteps && len(pending) > 0 {
		e.emitCompleted(step, ErrMaxStepsExceeded)
		return state, ErrMaxStepsExceeded
	}

	e.emitCompleted(step, nil)
	return state, nil
}

// Stream executes the graph and sends events to a channel.
func (e *Executor) Stream(ctx context.Context, initial State) <-chan ExecutionEvent {
	ch := make(chan ExecutionEvent, 16)
	go func() {
		defer close(ch)
		state := initial.Snapshot()
		pending := []string{e.graph.initialNode}
		step := 0

		for len(pending) > 0 && step < e.graph.maxSteps {
			step++
			if ctx.Err() != nil {
				ch <- ExecutionEvent{Type: "completed", Step: step, Err: ctx.Err()}
				return
			}

			ch <- ExecutionEvent{Type: "step_started", Step: step, State: state.Snapshot()}

			updates, nodeErrors := e.executeSuperstep(ctx, step, pending, state)

			for node, err := range nodeErrors {
				if err != nil {
					ch <- ExecutionEvent{Type: "node_completed", Step: step, Node: node, Err: err}
					ch <- ExecutionEvent{Type: "completed", Step: step, Err: err}
					return
				}
			}

			for _, update := range updates {
				state = applyUpdate(state, update, e.graph.reducers)
			}

			if e.checkpointer != nil {
				if cpErr := e.checkpointer.Save(step, state); cpErr != nil {
					ch <- ExecutionEvent{Type: "completed", Step: step, Err: cpErr}
					return
				}
			}

			pending = e.resolveNextNodes(step, pending, state)
		}

		var err error
		if step >= e.graph.maxSteps && len(pending) > 0 {
			err = ErrMaxStepsExceeded
		}
		ch <- ExecutionEvent{Type: "completed", Step: step, State: state.Snapshot(), Err: err}
	}()
	return ch
}

// executeSuperstep runs all pending nodes in parallel.
// Returns ordered updates and any per-node errors.
func (e *Executor) executeSuperstep(ctx context.Context, step int, pending []string, state State) ([]StateUpdate, map[string]error) {
	updates := make([]StateUpdate, len(pending))
	nodeErrors := make(map[string]error, len(pending))

	if len(pending) == 1 {
		// Fast path: single node, no goroutine overhead
		node := pending[0]
		start := time.Now()
		update, err := e.graph.nodes[node](ctx, state.Snapshot())
		dur := time.Since(start)

		if err != nil {
			nodeErrors[node] = err
			e.emitNodeCompleted(step, node, dur, err)
		} else {
			updates[0] = update
			e.emitNodeCompleted(step, node, dur, nil)
		}
		return updates, nodeErrors
	}

	// Parallel execution via errgroup
	var mu sync.Mutex
	g, gctx := errgroup.WithContext(ctx)

	for i, node := range pending {
		i, node := i, node
		snapshot := state.Snapshot() // each node gets its own snapshot
		g.Go(func() error {
			start := time.Now()
			update, err := e.graph.nodes[node](gctx, snapshot)
			dur := time.Since(start)

			mu.Lock()
			defer mu.Unlock()

			if err != nil {
				nodeErrors[node] = err
				e.emitNodeCompleted(step, node, dur, err)
				return err // cancels other nodes in errgroup
			}
			updates[i] = update
			e.emitNodeCompleted(step, node, dur, nil)
			return nil
		})
	}

	g.Wait()
	return updates, nodeErrors
}

// resolveNextNodes determines which nodes to execute next based on edges.
func (e *Executor) resolveNextNodes(step int, completed []string, state State) []string {
	seen := make(map[string]bool)
	var next []string

	for _, node := range completed {
		// Check static edges
		if targets, ok := e.graph.edges[node]; ok {
			for _, target := range targets {
				e.emitTransition(step, node, target, "")
				if !seen[target] {
					seen[target] = true
					next = append(next, target)
				}
			}
			continue // static edges are exclusive with conditional
		}

		// Check conditional edges
		if ce, ok := e.graph.conditionalEdges[node]; ok {
			key := ce.Router(state)
			target := ce.PathMap[key]
			if target == "" {
				// END — this branch terminates
				e.emitTransition(step, node, "END", key)
				continue
			}
			e.emitTransition(step, node, target, key)
			if !seen[target] {
				seen[target] = true
				next = append(next, target)
			}
		}
		// No edges from this node = implicit END for this branch
	}

	return next
}

// Event emission helpers (nil-safe — skip if no bus).

func (e *Executor) emitStepStarted(step int, nodes []string) {
	if e.bus == nil {
		return
	}
	e.bus.Emit(observe.LifecycleStepStarted{
		EventHeader: observe.NewEventHeader("LifecycleStepStarted", "", "", ""),
		Step:        step,
		Nodes:       nodes,
	})
}

func (e *Executor) emitNodeCompleted(step int, node string, dur time.Duration, err error) {
	if e.bus == nil {
		return
	}
	errStr := ""
	if err != nil {
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
	if e.bus == nil {
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
	if e.bus == nil {
		return
	}
	errStr := ""
	if err != nil {
		errStr = err.Error()
	}
	e.bus.Emit(observe.LifecycleCompleted{
		EventHeader: observe.NewEventHeader("LifecycleCompleted", "", "", ""),
		TotalSteps:  totalSteps,
		Error:       errStr,
	})
}
