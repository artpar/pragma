package testing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/provider"
	"github.com/artpar/gogent/internal/provider/replay"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/tool"
)

// SequenceProvider implements provider.Provider with predetermined responses.
// Each call to Stream() returns the next response converted to StreamChunks.
// This is a real provider returning recorded data — same pattern as ReplayProvider.
type SequenceProvider struct {
	responses []model.Response
	mu        sync.Mutex
	idx       int
	calls     []provider.RequestParams
}

func NewSequenceProvider(responses ...model.Response) *SequenceProvider {
	return &SequenceProvider{responses: responses}
}

func (sp *SequenceProvider) Name() string { return "test" }

func (sp *SequenceProvider) Stream(_ context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	sp.mu.Lock()
	sp.calls = append(sp.calls, params)
	idx := sp.idx
	sp.idx++
	sp.mu.Unlock()

	if idx >= len(sp.responses) {
		return nil, fmt.Errorf("no more responses configured in test provider (requested turn %d, have %d)", idx+1, len(sp.responses))
	}

	chunks := replay.ResponseToChunks(sp.responses[idx])
	ch := make(chan provider.StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch, nil
}

func (sp *SequenceProvider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	sp.mu.Lock()
	sp.calls = append(sp.calls, params)
	idx := sp.idx
	sp.idx++
	sp.mu.Unlock()

	if idx >= len(sp.responses) {
		return model.Response{}, fmt.Errorf("no more responses configured in test provider (requested turn %d, have %d)", idx+1, len(sp.responses))
	}
	return sp.responses[idx], nil
}

func (sp *SequenceProvider) SupportsFeature(_ provider.Feature) bool { return true }
func (sp *SequenceProvider) Pricing(_ string) (model.Pricing, bool) { return model.Pricing{}, false }
func (sp *SequenceProvider) ContextWindow(_ string) (int, bool)     { return 200_000, true }

// Calls returns the RequestParams from each provider call, for test inspection.
func (sp *SequenceProvider) Calls() []provider.RequestParams {
	sp.mu.Lock()
	defer sp.mu.Unlock()
	out := make([]provider.RequestParams, len(sp.calls))
	copy(out, sp.calls)
	return out
}

// Harness wires a complete engine for test scenarios.
// Use the builder methods to configure before calling Run.
type Harness struct {
	provider  *SequenceProvider
	bus       *observe.EventBus
	registry  *tool.Registry
	store     *app.StateStore
	ct        *model.CostTracker
	checker   permission.Checker
	engine    *query.Engine
	events    []observe.Event
	loopEvts  []query.LoopEvent
	maxTurns  int
	built     bool
}

// NewHarness creates an unconfigured Harness. Call builder methods then Run.
func NewHarness() *Harness {
	return &Harness{
		maxTurns: 100,
	}
}

// WithProviderResponses sets the predetermined responses the provider will return.
func (h *Harness) WithProviderResponses(responses ...model.Response) *Harness {
	h.provider = NewSequenceProvider(responses...)
	return h
}

// WithTool registers a simple tool that calls handler when invoked.
func (h *Harness) WithTool(name string, handler func(json.RawMessage) (string, error)) *Harness {
	h.ensureRegistry()
	_ = h.registry.Register(&simpleTool{
		name:    name,
		handler: handler,
	})
	return h
}

// WithToolError registers a tool that always returns the given error.
func (h *Harness) WithToolError(name string, errMsg string) *Harness {
	h.ensureRegistry()
	_ = h.registry.Register(&simpleTool{
		name: name,
		handler: func(_ json.RawMessage) (string, error) {
			return "", errors.New(errMsg)
		},
	})
	return h
}

// WithPermissionDeny configures the harness to deny the named tools.
// All other tools are allowed.
func (h *Harness) WithPermissionDeny(toolNames ...string) *Harness {
	deny := make(map[string]bool, len(toolNames))
	for _, n := range toolNames {
		deny[n] = true
	}
	h.checker = &denyListChecker{deny: deny}
	return h
}

// WithMaxTurns sets the maximum number of engine turns.
func (h *Harness) WithMaxTurns(n int) *Harness {
	h.maxTurns = n
	return h
}

func (h *Harness) ensureRegistry() {
	if h.bus == nil {
		h.bus = observe.NewEventBus(256)
	}
	if h.registry == nil {
		h.registry = tool.NewRegistry(h.bus)
	}
}

func (h *Harness) build() {
	if h.built {
		return
	}
	h.built = true

	if h.bus == nil {
		h.bus = observe.NewEventBus(256)
	}
	if h.registry == nil {
		h.registry = tool.NewRegistry(h.bus)
	}
	if h.provider == nil {
		h.provider = NewSequenceProvider()
	}
	if h.checker == nil {
		h.checker = &allowAllChecker{}
	}

	collector := &eventCollector{harness: h}
	h.bus.Subscribe(collector)

	prompter := &permission.NonInteractivePrompter{}
	orch := tool.NewOrchestrator(h.registry, h.checker, prompter, h.bus)

	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp/test")
	h.store = app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp/test",
		Model:        "test-model",
		Provider:     "test",
		MaxTokens:    4096,
	})
	h.ct = model.NewCostTracker()

	h.engine = query.NewEngine(h.provider, h.registry, orch, h.store, h.ct, h.bus, query.EngineConfig{
		Model:     "test-model",
		MaxTokens: 4096,
		MaxTurns:  h.maxTurns,
	})
}

// Run builds the engine (if not built) and runs the agentic loop with the given user message.
// Returns all observe.Event values captured from the EventBus.
func (h *Harness) Run(ctx context.Context, userMessage string) ([]observe.Event, error) {
	h.build()

	ch := h.engine.Run(ctx, userMessage)
	var loopErr error
	for ev := range ch {
		h.loopEvts = append(h.loopEvts, ev)
		if e, ok := ev.(query.ErrorEvent); ok {
			loopErr = e.Err
		}
	}

	h.bus.Drain()
	return h.events, loopErr
}

// ProviderCalls returns the RequestParams from each provider call.
func (h *Harness) ProviderCalls() []provider.RequestParams {
	if h.provider == nil {
		return nil
	}
	return h.provider.Calls()
}

// LoopEvents returns the raw LoopEvents from the last Run.
func (h *Harness) LoopEvents() []query.LoopEvent {
	return h.loopEvts
}

// ConversationMessages returns the conversation messages after Run.
func (h *Harness) ConversationMessages() []model.Message {
	if h.store == nil {
		return nil
	}
	return h.store.Snapshot().Conversation.Messages
}

// AssertEvent checks that at least one event with the given kind exists.
// If matcher is non-nil, at least one event of that kind must satisfy it.
func (h *Harness) AssertEvent(kind string, matcher func(observe.Event) bool) error {
	for _, ev := range h.events {
		if ev.EventKind() != kind {
			continue
		}
		if matcher == nil || matcher(ev) {
			return nil
		}
	}
	if matcher != nil {
		return fmt.Errorf("no %q event matched the provided matcher (found %d events of that kind)", kind, countKind(h.events, kind))
	}
	return fmt.Errorf("no %q event found in %d total events", kind, len(h.events))
}

// AssertNoEvent checks that no event with the given kind exists.
func (h *Harness) AssertNoEvent(kind string) error {
	for _, ev := range h.events {
		if ev.EventKind() == kind {
			return fmt.Errorf("unexpected %q event found", kind)
		}
	}
	return nil
}

// AssertEventSequence checks that the given kinds appear in order (not necessarily contiguous).
func (h *Harness) AssertEventSequence(kinds ...string) error {
	idx := 0
	for _, ev := range h.events {
		if idx < len(kinds) && ev.EventKind() == kinds[idx] {
			idx++
		}
	}
	if idx < len(kinds) {
		return fmt.Errorf("event sequence incomplete: matched %d of %d kinds, stuck at %q", idx, len(kinds), kinds[idx])
	}
	return nil
}

// AssertEventField checks that at least one event with the given kind has a JSON field
// matching the expected value. Fields are checked via JSON serialization.
func (h *Harness) AssertEventField(kind string, field string, expected string) error {
	for _, ev := range h.events {
		if ev.EventKind() != kind {
			continue
		}
		if matchEventField(ev, field, expected) {
			return nil
		}
	}
	return fmt.Errorf("no %q event has field %q=%q", kind, field, expected)
}

func countKind(events []observe.Event, kind string) int {
	n := 0
	for _, ev := range events {
		if ev.EventKind() == kind {
			n++
		}
	}
	return n
}

func matchEventField(ev observe.Event, field string, expected string) bool {
	data, err := json.Marshal(ev)
	if err != nil {
		return false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return false
	}
	val, ok := m[field]
	if !ok {
		return false
	}
	return fmt.Sprint(val) == expected
}

// --- internal types ---

type eventCollector struct {
	harness *Harness
}

func (ec *eventCollector) HandleEvent(event observe.Event) {
	ec.harness.events = append(ec.harness.events, event)
}

// allowAllChecker allows all tool invocations.
type allowAllChecker struct{}

func (a *allowAllChecker) Check(_ context.Context, _ string, _ string) permission.CheckResult {
	return permission.CheckResult{
		Decision: permission.DecisionAllow,
		Rule:     &permission.Rule{ToolName: "*", Decision: permission.DecisionAllow, Source: "test"},
	}
}

func (a *allowAllChecker) AddSessionRule(_ permission.Rule) {}

// denyListChecker denies the named tools, allows everything else.
type denyListChecker struct {
	deny map[string]bool
}

func (d *denyListChecker) Check(_ context.Context, toolName string, _ string) permission.CheckResult {
	if d.deny[toolName] {
		return permission.CheckResult{
			Decision: permission.DecisionDeny,
			Rule:     &permission.Rule{ToolName: toolName, Decision: permission.DecisionDeny, Source: "test"},
			Reason:   "denied by test harness",
		}
	}
	return permission.CheckResult{
		Decision: permission.DecisionAllow,
		Rule:     &permission.Rule{ToolName: "*", Decision: permission.DecisionAllow, Source: "test"},
	}
}

func (d *denyListChecker) AddSessionRule(_ permission.Rule) {}

// simpleTool implements tool.Descriptor for test scenarios.
type simpleTool struct {
	name    string
	handler func(json.RawMessage) (string, error)
}

func (t *simpleTool) Name() string        { return t.name }
func (t *simpleTool) Description() string { return "test tool: " + t.name }
func (t *simpleTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *simpleTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	result, err := t.handler(input)
	if err != nil {
		return tool.InvokeResult{}, err
	}
	return tool.InvokeResult{Content: result}, nil
}

func (t *simpleTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(context.Background(), t.name, "")
}

func (t *simpleTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}
