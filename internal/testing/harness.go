package testing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/replay"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/tool"
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &SequenceProvider{responses: responses}")
	return &SequenceProvider{responses: responses}
}

func (sp *SequenceProvider) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"test\"")
	return "test"
}

func (sp *SequenceProvider) Stream(_ context.Context, params provider.RequestParams) (<-chan provider.StreamChunk, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sp.mu.Lock()
	sp.calls = append(sp.calls, params)
	idx := sp.idx
	sp.idx++
	sp.mu.Unlock()

	if idx >= len(sp.responses) {
		observe.GlobalTrace("if: idx >= len(sp.responses)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"no more responses configured in test provider (requested tur...")
		return nil, fmt.Errorf("no more responses configured in test provider (requested turn %d, have %d)", idx+1, len(sp.responses))
	}

	chunks := replay.ResponseToChunks(sp.responses[idx])
	ch := make(chan provider.StreamChunk, len(chunks))
	for _, c := range chunks {
		observe.GlobalTrace("range chunks")
		ch <- c
	}
	close(ch)
	observe.GlobalTrace("return: ch, nil")
	return ch, nil
}

func (sp *SequenceProvider) Complete(ctx context.Context, params provider.RequestParams) (model.Response, error) {
	observe.TraceCtx(ctx, "testing", "SequenceProvider.Complete", "enter")
	defer observe.TraceCtx(ctx, "testing", "SequenceProvider.Complete", "exit")
	sp.mu.Lock()
	sp.calls = append(sp.calls, params)
	idx := sp.idx
	sp.idx++
	sp.mu.Unlock()

	if idx >= len(sp.responses) {
		observe.TraceCtx(ctx, "testing", "SequenceProvider.Complete", "if: idx >= len(sp.responses)")
		observe.TraceCtx(ctx, "testing", "SequenceProvider.Complete", "return: model.Response{}, fmt.Errorf(\"no more responses configured in test provider (...")
		return model.Response{}, fmt.Errorf("no more responses configured in test provider (requested turn %d, have %d)", idx+1, len(sp.responses))
	}
	observe.TraceCtx(ctx, "testing", "SequenceProvider.Complete", "return: sp.responses[idx], nil")
	return sp.responses[idx], nil
}

func (sp *SequenceProvider) SupportsFeature(_ provider.Feature) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: true")
	return true
}
func (sp *SequenceProvider) Pricing(_ string) (model.Pricing, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: model.Pricing{}, false")
	return model.Pricing{}, false
}
func (sp *SequenceProvider) ContextWindow(_ string) (int, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: 200_000, true")
	return 200_000, true
}

// Calls returns the RequestParams from each provider call, for test inspection.
func (sp *SequenceProvider) Calls() []provider.RequestParams {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sp.mu.Lock()
	defer sp.mu.Unlock()
	out := make([]provider.RequestParams, len(sp.calls))
	copy(out, sp.calls)
	observe.GlobalTrace("return: out")
	return out
}

// Harness wires a complete engine for test scenarios.
// Use the builder methods to configure before calling Run.
type Harness struct {
	provider *SequenceProvider
	bus      *observe.EventBus
	registry *tool.Registry
	store    *app.StateStore
	ct       *model.CostTracker
	checker  permission.Checker
	engine   *query.Engine
	events   []observe.Event
	loopEvts []query.LoopEvent
	maxTurns int
	built    bool
}

// NewHarness creates an unconfigured Harness. Call builder methods then Run.
func NewHarness() *Harness {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Harness{\n\tmaxTurns: 100,\n}")
	return &Harness{
		maxTurns: 100,
	}
}

// WithProviderResponses sets the predetermined responses the provider will return.
func (h *Harness) WithProviderResponses(responses ...model.Response) *Harness {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	h.provider = NewSequenceProvider(responses...)
	observe.GlobalTrace("return: h")
	return h
}

// WithTool registers a simple tool that calls handler when invoked.
func (h *Harness) WithTool(name string, handler func(json.RawMessage) (string, error)) *Harness {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	h.ensureRegistry()
	_ = h.registry.Register(&simpleTool{
		name:    name,
		handler: handler,
	})
	observe.GlobalTrace("return: h")
	return h
}

// WithToolError registers a tool that always returns the given error.
func (h *Harness) WithToolError(name string, errMsg string) *Harness {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	h.ensureRegistry()
	_ = h.registry.Register(&simpleTool{
		name: name,
		handler: func(_ json.RawMessage) (string, error) {
			return "", errors.New(errMsg)
		},
	})
	observe.GlobalTrace("return: h")
	return h
}

// WithPermissionDeny configures the harness to deny the named tools.
// All other tools are allowed.
func (h *Harness) WithPermissionDeny(toolNames ...string) *Harness {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	deny := make(map[string]bool, len(toolNames))
	for _, n := range toolNames {
		observe.GlobalTrace("range toolNames")
		deny[n] = true
	}
	h.checker = &denyListChecker{deny: deny}
	observe.GlobalTrace("return: h")
	return h
}

// WithMaxTurns sets the maximum number of engine turns.
func (h *Harness) WithMaxTurns(n int) *Harness {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	h.maxTurns = n
	observe.GlobalTrace("return: h")
	return h
}

func (h *Harness) ensureRegistry() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if h.bus == nil {
		observe.GlobalTrace("if: h.bus == nil")
		h.bus = observe.NewEventBus(256)
	}
	if h.registry == nil {
		observe.GlobalTrace("if: h.registry == nil")
		h.registry = tool.NewRegistry(h.bus)
	}
}

func (h *Harness) build() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if h.built {
		observe.GlobalTrace("if: h.built")
		return
	}
	h.built = true

	if h.bus == nil {
		observe.GlobalTrace("if: h.bus == nil")
		h.bus = observe.NewEventBus(256)
	}
	if h.registry == nil {
		observe.GlobalTrace("if: h.registry == nil")
		h.registry = tool.NewRegistry(h.bus)
	}
	if h.provider == nil {
		observe.GlobalTrace("if: h.provider == nil")
		h.provider = NewSequenceProvider()
	}
	if h.checker == nil {
		observe.GlobalTrace("if: h.checker == nil")
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
	observe.TraceCtx(ctx, "testing", "Harness.Run", "enter")
	defer observe.TraceCtx(ctx, "testing", "Harness.Run", "exit")
	h.build()

	ch := h.engine.Run(ctx, userMessage)
	var loopErr error
	for ev := range ch {
		observe.TraceCtx(ctx, "testing", "Harness.Run", "range ch")
		h.loopEvts = append(h.loopEvts, ev)
		if e, ok := ev.(query.ErrorEvent); ok {
			observe.TraceCtx(ctx, "testing", "Harness.Run", "if: ok")
			loopErr = e.Err
		}
	}

	h.bus.Drain()
	observe.TraceCtx(ctx, "testing", "Harness.Run", "return: h.events, loopErr")
	return h.events, loopErr
}

// ProviderCalls returns the RequestParams from each provider call.
func (h *Harness) ProviderCalls() []provider.RequestParams {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if h.provider == nil {
		observe.GlobalTrace("if: h.provider == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: h.provider.Calls()")
	return h.provider.Calls()
}

// LoopEvents returns the raw LoopEvents from the last Run.
func (h *Harness) LoopEvents() []query.LoopEvent {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: h.loopEvts")
	return h.loopEvts
}

// ConversationMessages returns the conversation messages after Run.
func (h *Harness) ConversationMessages() []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if h.store == nil {
		observe.GlobalTrace("if: h.store == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	observe.GlobalTrace("return: h.store.Snapshot().Conversation.Messages")
	return h.store.Snapshot().Conversation.Messages
}

// AssertEvent checks that at least one event with the given kind exists.
// If matcher is non-nil, at least one event of that kind must satisfy it.
func (h *Harness) AssertEvent(kind string, matcher func(observe.Event) bool) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, ev := range h.events {
		observe.GlobalTrace("range h.events")
		if ev.EventKind() != kind {
			observe.GlobalTrace("if: ev.EventKind() != kind")
			continue
		}
		if matcher == nil || matcher(ev) {
			observe.GlobalTrace("if: matcher == nil || matcher(ev)")
			observe.GlobalTrace("return: nil")
			return nil
		}
	}
	if matcher != nil {
		observe.GlobalTrace("if: matcher != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"no %q event matched the provided matcher (found %d events of that...")
		return fmt.Errorf("no %q event matched the provided matcher (found %d events of that kind)", kind, countKind(h.events, kind))
	}
	observe.GlobalTrace("return: fmt.Errorf(\"no %q event found in %d total events\", kind, len(h.events))")
	return fmt.Errorf("no %q event found in %d total events", kind, len(h.events))
}

// AssertNoEvent checks that no event with the given kind exists.
func (h *Harness) AssertNoEvent(kind string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, ev := range h.events {
		observe.GlobalTrace("range h.events")
		if ev.EventKind() == kind {
			observe.GlobalTrace("if: ev.EventKind() == kind")
			observe.GlobalTrace("return: fmt.Errorf(\"unexpected %q event found\", kind)")
			return fmt.Errorf("unexpected %q event found", kind)
		}
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// AssertEventSequence checks that the given kinds appear in order (not necessarily contiguous).
func (h *Harness) AssertEventSequence(kinds ...string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	idx := 0
	for _, ev := range h.events {
		observe.GlobalTrace("range h.events")
		if idx < len(kinds) && ev.EventKind() == kinds[idx] {
			observe.GlobalTrace("if: idx < len(kinds) && ev.EventKind() == kinds[idx]")
			idx++
		}
	}
	if idx < len(kinds) {
		observe.GlobalTrace("if: idx < len(kinds)")
		observe.GlobalTrace("return: fmt.Errorf(\"event sequence incomplete: matched %d of %d kinds, stuck at %q\", ...")
		return fmt.Errorf("event sequence incomplete: matched %d of %d kinds, stuck at %q", idx, len(kinds), kinds[idx])
	}
	observe.GlobalTrace("return: nil")
	return nil
}

// AssertEventField checks that at least one event with the given kind has a JSON field
// matching the expected value. Fields are checked via JSON serialization.
func (h *Harness) AssertEventField(kind string, field string, expected string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, ev := range h.events {
		observe.GlobalTrace("range h.events")
		if ev.EventKind() != kind {
			observe.GlobalTrace("if: ev.EventKind() != kind")
			continue
		}
		if matchEventField(ev, field, expected) {
			observe.GlobalTrace("if: matchEventField(ev, field, expected)")
			observe.GlobalTrace("return: nil")
			return nil
		}
	}
	observe.GlobalTrace("return: fmt.Errorf(\"no %q event has field %q=%q\", kind, field, expected)")
	return fmt.Errorf("no %q event has field %q=%q", kind, field, expected)
}

func countKind(events []observe.Event, kind string) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	n := 0
	for _, ev := range events {
		observe.GlobalTrace("range events")
		if ev.EventKind() == kind {
			observe.GlobalTrace("if: ev.EventKind() == kind")
			n++
		}
	}
	observe.GlobalTrace("return: n")
	return n
}

func matchEventField(ev observe.Event, field string, expected string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	data, err := json.Marshal(ev)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: false")
		return false
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: false")
		return false
	}
	val, ok := m[field]
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: false")
		return false
	}
	observe.GlobalTrace("return: fmt.Sprint(val) == expected")
	return fmt.Sprint(val) == expected
}

type eventCollector struct {
	harness *Harness
}

func (ec *eventCollector) HandleEvent(event observe.Event) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ec.harness.events = append(ec.harness.events, event)
}

// allowAllChecker allows all tool invocations.
type allowAllChecker struct{}

func (a *allowAllChecker) Check(_ context.Context, _ string, _ string) permission.CheckResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: permission.CheckResult{\n\tDecision:\tpermission.DecisionAllow,\n\tRule:\t\t&permiss...")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d.deny[toolName] {
		observe.GlobalTrace("if: d.deny[toolName]")
		observe.GlobalTrace("return: permission.CheckResult{\n\tDecision:\tpermission.DecisionDeny,\n\tRule:\t\t&permissi...")
		return permission.CheckResult{
			Decision: permission.DecisionDeny,
			Rule:     &permission.Rule{ToolName: toolName, Decision: permission.DecisionDeny, Source: "test"},
			Reason:   "denied by test harness",
		}
	}
	observe.GlobalTrace("return: permission.CheckResult{\n\tDecision:\tpermission.DecisionAllow,\n\tRule:\t\t&permiss...")
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

func (t *simpleTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: t.name")
	return t.name
}
func (t *simpleTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"test tool: \" + t.name")
	return "test tool: " + t.name
}
func (t *simpleTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: json.RawMessage(`{\"type\":\"object\",\"properties\":{}}`)")
	return json.RawMessage(`{"type":"object","properties":{}}`)
}

func (t *simpleTool) Invoke(_ context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	result, err := t.handler(input)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}
	observe.GlobalTrace("return: tool.InvokeResult{Content: result}, nil")
	return tool.InvokeResult{Content: result}, nil
}

func (t *simpleTool) CheckPerm(_ context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: checker.Check(context.Background(), t.name, \"\")")
	return checker.Check(context.Background(), t.name, "")
}

func (t *simpleTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}
