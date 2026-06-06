package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/task"
	"github.com/artpar/pragma/internal/tool"
)

// Config contains the already-wired interactive runtime dependencies.
type Config struct {
	Bridge         *Bridge
	ParentCtx      context.Context
	RunInput       func(context.Context, string) <-chan interactive.Event
	Resume         func(sessionID string) error
	CloseSession   func() error
	Store          *app.StateStore
	CostTracker    *model.CostTracker
	ModelName      string
	Provider       string
	SlashCmds      *slash.Registry
	SlashDeps      slash.Deps
	Metrics        *observe.Metrics
	Workspace      string
	Version        string
	TaskReg        *task.Registry
	SessionStart   time.Time
	McpServerNames []string
	PromptHistory  []string
}

// Run starts the browser UI and blocks until the server shuts down.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Bridge == nil {
		return errors.New("web bridge is required")
	}
	srv := newServer(cfg)
	cfg.Bridge.attach(srv.hub)

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.handleIndex)
	mux.HandleFunc("/api/state", srv.handleState)
	mux.HandleFunc("/api/events", srv.handleEvents)
	mux.HandleFunc("/api/prompt", srv.handlePrompt)
	mux.HandleFunc("/api/cancel", srv.handleCancel)
	mux.HandleFunc("/api/permission/", srv.handlePermission)
	mux.HandleFunc("/api/ask/", srv.handleAsk)
	mux.HandleFunc("/api/sessions", srv.handleSessions)
	mux.HandleFunc("/api/resume", srv.handleResume)
	mux.HandleFunc("/api/artifact", srv.handleArtifact)
	mux.HandleFunc("/api/completions", srv.handleCompletions)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Handler: mux}
	url := "http://" + ln.Addr().String()
	fmt.Printf("Pragma web UI: %s\n", url)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpSrv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if cfg.CloseSession != nil {
			if err := cfg.CloseSession(); err != nil {
				_ = httpSrv.Shutdown(shutdownCtx)
				return err
			}
		}
		_ = httpSrv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

type server struct {
	cfg        Config
	hub        *hub
	mu         sync.Mutex
	running    bool
	cancel     context.CancelFunc
	artifacts  map[string]struct{}
	workflow   workflowSnapshot
	workflowOn bool
}

func newServer(cfg Config) *server {
	return &server{
		cfg:       cfg,
		hub:       newHub(),
		artifacts: make(map[string]struct{}),
	}
}

type workflowSnapshot struct {
	Name        string                    `json:"name,omitempty"`
	Initial     string                    `json:"initial,omitempty"`
	Current     string                    `json:"current,omitempty"`
	Completed   bool                      `json:"completed,omitempty"`
	States      map[string]*workflowState `json:"states"`
	Transitions []workflowTransition      `json:"transitions"`
	Handoffs    []workflowHandoff         `json:"handoffs"`
}

type workflowState struct {
	ID        string    `json:"id"`
	Persona   string    `json:"persona,omitempty"`
	Control   string    `json:"control,omitempty"`
	Status    string    `json:"status,omitempty"`
	LastEvent string    `json:"last_event,omitempty"`
	Started   time.Time `json:"started,omitempty"`
	Completed time.Time `json:"completed,omitempty"`
	Duration  string    `json:"duration,omitempty"`
}

type workflowTransition struct {
	From  string `json:"from"`
	Event string `json:"event"`
	To    string `json:"to"`
}

type workflowHandoff struct {
	StateID   string `json:"state_id"`
	Event     string `json:"event"`
	Path      string `json:"path"`
	Direction string `json:"direction"`
}

func newWorkflowSnapshot() workflowSnapshot {
	return workflowSnapshot{States: make(map[string]*workflowState)}
}

type eventEnvelope struct {
	Sequence int         `json:"sequence"`
	Received time.Time   `json:"received_at"`
	Type     string      `json:"type"`
	DataType string      `json:"data_type"`
	Data     interface{} `json:"data"`
}

func (e eventEnvelope) MarshalJSON() ([]byte, error) {
	type wireEnvelope struct {
		Sequence int             `json:"sequence"`
		Received time.Time       `json:"received_at"`
		Type     string          `json:"type"`
		DataType string          `json:"data_type"`
		Data     json.RawMessage `json:"data"`
	}
	data, err := json.Marshal(completeJSONValue(e.Data))
	if err != nil {
		return nil, err
	}
	return json.Marshal(wireEnvelope{
		Sequence: e.Sequence,
		Received: e.Received,
		Type:     e.Type,
		DataType: e.DataType,
		Data:     data,
	})
}

type hub struct {
	mu          sync.Mutex
	subscribers map[chan eventEnvelope]struct{}
	recent      []eventEnvelope
	sequence    int
}

func newHub() *hub {
	return &hub{subscribers: make(map[chan eventEnvelope]struct{})}
}

func (h *hub) publish(kind string, data interface{}) {
	kind, dataType, data := normalizeWebEvent(kind, data)
	h.mu.Lock()
	h.sequence++
	ev := eventEnvelope{
		Sequence: h.sequence,
		Received: time.Now(),
		Type:     kind,
		DataType: dataType,
		Data:     data,
	}
	h.recent = append(h.recent, ev)
	if len(h.recent) > 200 {
		h.recent = h.recent[len(h.recent)-200:]
	}
	for ch := range h.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
	h.mu.Unlock()
}

func dataType(data interface{}) string {
	if data == nil {
		return ""
	}
	return fmt.Sprintf("%T", data)
}

type webPromptEvent struct {
	Prompt string `json:"prompt"`
}

type webTextEvent struct {
	Text string `json:"text"`
}

type webModelRequestEvent struct {
	Model   string `json:"model"`
	Attempt int    `json:"attempt"`
}

type webModelResponseEvent struct {
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
}

type webToolCallEvent struct {
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type webFileEffect struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
}

type webToolResultEvent struct {
	ID          string          `json:"id"`
	Content     string          `json:"content"`
	Display     string          `json:"display,omitempty"`
	IsError     bool            `json:"is_error,omitempty"`
	FileEffects []webFileEffect `json:"file_effects,omitempty"`
}

type webTurnCompleteEvent struct {
	StopReason string `json:"stop_reason"`
}

type webCompactionEvent struct {
	PreTokens           int    `json:"pre_tokens,omitempty"`
	PostTokens          int    `json:"post_tokens,omitempty"`
	Attempt             int    `json:"attempt,omitempty"`
	MaxRetry            int    `json:"max_retry,omitempty"`
	Error               string `json:"error,omitempty"`
	ConsecutiveFailures int    `json:"consecutive_failures,omitempty"`
}

type webLifecycleProgressEvent struct {
	Step       int      `json:"step"`
	Node       string   `json:"node,omitempty"`
	Nodes      []string `json:"nodes,omitempty"`
	Status     string   `json:"status"`
	DurationMs int64    `json:"duration_ms,omitempty"`
	Error      string   `json:"error,omitempty"`
	FromNode   string   `json:"from_node,omitempty"`
	ToNode     string   `json:"to_node,omitempty"`
	RouteKey   string   `json:"route_key,omitempty"`
}

type webOrchestrationStartedEvent struct {
	Name    string `json:"name"`
	Initial string `json:"initial"`
}

type webOrchestrationStateEvent struct {
	StateID    string `json:"state_id"`
	PersonaID  string `json:"persona_id,omitempty"`
	Control    string `json:"control,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	Event      string `json:"event,omitempty"`
}

type webOrchestrationTransitionEvent struct {
	From  string `json:"from"`
	Event string `json:"event"`
	To    string `json:"to"`
}

type webOrchestrationHandoffEvent struct {
	StateID   string `json:"state_id"`
	Event     string `json:"event"`
	Path      string `json:"path"`
	Direction string `json:"direction"`
}

type webOrchestrationCompletedEvent struct {
	Name string `json:"name"`
}

type webAgentProgressEvent struct {
	AgentID     string `json:"agent_id"`
	Description string `json:"description"`
	ToolCount   int    `json:"tool_count"`
	TokenCount  int    `json:"token_count"`
	LastTool    string `json:"last_tool,omitempty"`
	Status      string `json:"status"`
	Background  bool   `json:"background"`
	Error       string `json:"error,omitempty"`
}

type webRetryEvent struct {
	Attempt     int    `json:"attempt"`
	MaxAttempts int    `json:"max_attempts"`
	DelayMs     int64  `json:"delay_ms"`
	Kind        string `json:"kind"`
	Error       string `json:"error"`
}

type webErrorEvent struct {
	Message  string `json:"message"`
	Kind     string `json:"kind,omitempty"`
	Guidance string `json:"guidance,omitempty"`
}

func normalizeWebEvent(kind string, data interface{}) (string, string, interface{}) {
	switch kind {
	case "prompt_accepted":
		if ev, ok := data.(interactive.AcceptedPromptEvent); ok {
			return "prompt_accepted", "web.prompt", webPromptEvent{Prompt: ev.Prompt}
		}
	case "loop_event":
		return normalizeLoopEvent(data)
	}
	return kind, dataType(data), data
}

func normalizeLoopEvent(data interface{}) (string, string, interface{}) {
	switch ev := data.(type) {
	case query.TextEvent:
		return "text", "web.text", webTextEvent{Text: ev.Text}
	case query.ThinkingEvent:
		return "thinking", "web.thinking", webTextEvent{Text: ev.Text}
	case query.ModelRequestEvent:
		return "model_request", "web.model_request", webModelRequestEvent{Model: ev.Model, Attempt: ev.Attempt}
	case query.ModelResponseEvent:
		return "model_response", "web.model_response", webModelResponseEvent{Model: ev.Model, StopReason: string(ev.StopReason)}
	case query.ToolCallEvent:
		return "tool_call", "web.tool_call", webToolCallEvent{ID: ev.Call.ID, Name: ev.Call.Name, Input: ev.Call.Input}
	case query.ToolResultEvent:
		effects := make([]webFileEffect, 0, len(ev.FileEffects))
		for _, effect := range ev.FileEffects {
			effects = append(effects, webFileEffect{Path: effect.Path, Operation: effect.Operation})
		}
		return "tool_result", "web.tool_result", webToolResultEvent{
			ID:          ev.Result.ToolCallID,
			Content:     ev.Result.Content,
			Display:     ev.Display,
			IsError:     ev.Result.IsError,
			FileEffects: effects,
		}
	case query.TurnCompleteEvent:
		return "turn_complete", "web.turn_complete", webTurnCompleteEvent{StopReason: string(ev.StopReason)}
	case query.CompactionStartedEvent:
		return "compaction_started", "web.compaction_started", webCompactionEvent{}
	case query.CompactionEvent:
		return "compaction", "web.compaction", webCompactionEvent{PreTokens: ev.PreTokens, PostTokens: ev.PostTokens}
	case query.CompactionFailedEvent:
		return "compaction_failed", "web.compaction_failed", webCompactionEvent{Attempt: ev.Attempt, MaxRetry: ev.MaxRetry, Error: ev.ErrorMsg}
	case query.CompactionDisabledEvent:
		return "compaction_disabled", "web.compaction_disabled", webCompactionEvent{ConsecutiveFailures: ev.ConsecutiveFailures}
	case query.LifecycleProgressEvent:
		return "lifecycle_progress", "web.lifecycle_progress", webLifecycleProgressEvent{
			Step: ev.Step, Node: ev.Node, Nodes: ev.Nodes, Status: ev.Status,
			DurationMs: ev.Duration.Milliseconds(), Error: ev.Error,
			FromNode: ev.FromNode, ToNode: ev.ToNode, RouteKey: ev.RouteKey,
		}
	case query.OrchestrationStartedEvent:
		return "orchestration_started", "web.orchestration_started", webOrchestrationStartedEvent{Name: ev.Name, Initial: ev.Initial}
	case query.OrchestrationStateStartedEvent:
		return "orchestration_state_started", "web.orchestration_state_started", webOrchestrationStateEvent{StateID: ev.StateID, PersonaID: ev.PersonaID, Control: ev.Control}
	case query.OrchestrationStateCompletedEvent:
		return "orchestration_state_completed", "web.orchestration_state_completed", webOrchestrationStateEvent{StateID: ev.StateID, DurationMs: ev.Duration.Milliseconds()}
	case query.OrchestrationControlEvent:
		return "orchestration_control", "web.orchestration_control", webOrchestrationStateEvent{StateID: ev.StateID, Control: ev.Control, Event: ev.Event}
	case query.OrchestrationTransitionEvent:
		return "orchestration_transition", "web.orchestration_transition", webOrchestrationTransitionEvent{From: ev.From, Event: ev.Event, To: ev.To}
	case query.OrchestrationHandoffEvent:
		return "orchestration_handoff", "web.orchestration_handoff", webOrchestrationHandoffEvent{StateID: ev.StateID, Event: ev.Event, Path: ev.Path, Direction: ev.Direction}
	case query.OrchestrationCompletedEvent:
		return "orchestration_completed", "web.orchestration_completed", webOrchestrationCompletedEvent{Name: ev.Name}
	case query.AgentProgressEvent:
		return "agent_progress", "web.agent_progress", webAgentProgressEvent{
			AgentID: ev.AgentID, Description: ev.Description, ToolCount: ev.ToolCount,
			TokenCount: ev.TokenCount, LastTool: ev.LastTool, Status: ev.Status,
			Background: ev.Background, Error: ev.Error,
		}
	case query.RetryEvent:
		return "retry", "web.retry", webRetryEvent{
			Attempt: ev.Attempt, MaxAttempts: ev.MaxAttempts,
			DelayMs: ev.Delay.Milliseconds(), Kind: string(ev.Kind), Error: ev.ErrorMsg,
		}
	case query.ErrorEvent:
		message := ""
		if ev.Err != nil {
			message = ev.Err.Error()
		}
		return "run_error", "web.error", webErrorEvent{Message: message, Kind: string(ev.Kind), Guidance: ev.Guidance}
	default:
		return "loop_event", dataType(data), data
	}
}

func (h *hub) subscribe() (chan eventEnvelope, []eventEnvelope, func()) {
	ch := make(chan eventEnvelope, 32)
	h.mu.Lock()
	h.subscribers[ch] = struct{}{}
	recent := append([]eventEnvelope(nil), h.recent...)
	h.mu.Unlock()
	return ch, recent, func() {
		h.mu.Lock()
		delete(h.subscribers, ch)
		close(ch)
		h.mu.Unlock()
	}
}

// Bridge implements permission.Prompter and tool.Asker for the browser UI.
type Bridge struct {
	mu          sync.Mutex
	hub         *hub
	permissions map[string]chan permissionResponse
	asks        map[string]chan tool.AskResponse
}

// NewBridge creates a web permission and ask bridge.
func NewBridge() *Bridge {
	return &Bridge{
		permissions: make(map[string]chan permissionResponse),
		asks:        make(map[string]chan tool.AskResponse),
	}
}

func (b *Bridge) attach(h *hub) {
	b.mu.Lock()
	b.hub = h
	b.mu.Unlock()
}

type permissionResponse struct {
	Decision permission.Decision
	Remember bool
}

func (b *Bridge) Prompt(ctx context.Context, toolName string, toolInput json.RawMessage, content string, reason string) (permission.Decision, bool) {
	id := newID()
	respCh := make(chan permissionResponse, 1)
	b.mu.Lock()
	b.permissions[id] = respCh
	h := b.hub
	b.mu.Unlock()
	if h != nil {
		h.publish("permission_request", map[string]interface{}{
			"id": id, "tool": toolName, "input": json.RawMessage(toolInput),
			"content": content, "reason": reason,
		})
	}
	select {
	case resp := <-respCh:
		return resp.Decision, resp.Remember
	case <-ctx.Done():
		b.dropPermission(id)
		return permission.DecisionDeny, false
	}
}

func (b *Bridge) Ask(ctx context.Context, req tool.AskRequest) (tool.AskResponse, error) {
	id := newID()
	respCh := make(chan tool.AskResponse, 1)
	b.mu.Lock()
	b.asks[id] = respCh
	h := b.hub
	b.mu.Unlock()
	if h != nil {
		h.publish("ask_request", map[string]interface{}{"id": id, "request": req})
	}
	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		b.dropAsk(id)
		return tool.AskResponse{}, ctx.Err()
	}
}

func (b *Bridge) resolvePermission(id string, resp permissionResponse) bool {
	b.mu.Lock()
	ch := b.permissions[id]
	delete(b.permissions, id)
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- resp
	return true
}

func (b *Bridge) resolveAsk(id string, resp tool.AskResponse) bool {
	b.mu.Lock()
	ch := b.asks[id]
	delete(b.asks, id)
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- resp
	return true
}

func (b *Bridge) dropPermission(id string) {
	b.mu.Lock()
	delete(b.permissions, id)
	b.mu.Unlock()
}

func (b *Bridge) dropAsk(id string) {
	b.mu.Lock()
	delete(b.asks, id)
	b.mu.Unlock()
}

func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(indexHTML))
}

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	snap := s.cfg.Store.Snapshot()
	writeJSON(w, map[string]interface{}{
		"runtime": map[string]interface{}{
			"version":     s.cfg.Version,
			"workspace":   s.cfg.Workspace,
			"model":       s.cfg.ModelName,
			"provider":    s.cfg.Provider,
			"mcp_servers": s.cfg.McpServerNames,
			"running":     s.isRunning(),
		},
		"app_state":      snap,
		"prompt_history": s.cfg.PromptHistory,
	})
}

func (s *server) isRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch, recent, unsubscribe := s.hub.subscribe()
	defer unsubscribe()
	for _, ev := range recent {
		writeSSE(w, ev)
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	for {
		select {
		case ev := <-ch:
			writeSSE(w, ev)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	raw, err := decodeRawObject(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var prompt string
	if err := json.Unmarshal(raw["prompt"], &prompt); err != nil {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(prompt) == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}
	if s.start(prompt, raw) {
		writeJSON(w, map[string]bool{"ok": true})
		return
	}
	http.Error(w, "interactive run already in progress", http.StatusConflict)
}

func (s *server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !s.cancelRun() {
		http.Error(w, "interactive run is not in progress", http.StatusConflict)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) cancelRun() bool {
	s.mu.Lock()
	cancel := s.cancel
	running := s.running
	s.mu.Unlock()
	if !running || cancel == nil {
		return false
	}
	cancel()
	return true
}

func (s *server) start(input string, rawRequest map[string]json.RawMessage) bool {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return false
	}
	ctx, cancel := context.WithCancel(s.cfg.ParentCtx)
	s.running = true
	s.cancel = cancel
	s.mu.Unlock()

	go func() {
		defer func() {
			s.mu.Lock()
			s.running = false
			s.cancel = nil
			s.mu.Unlock()
			s.hub.publish("run_idle", nil)
		}()
		s.hub.publish("user_prompt", rawRequest)
		events := s.cfg.RunInput(ctx, input)
		for ev := range events {
			switch e := ev.(type) {
			case interactive.AcceptedPromptEvent:
				s.hub.publish("prompt_accepted", e)
				continue
			case interactive.SlashResultEvent:
				s.hub.publish("slash_result", e.Result)
				continue
			case interactive.LoopEvent:
				if handoff, ok := e.Event.(query.OrchestrationHandoffEvent); ok {
					s.rememberArtifact(handoff.Path)
				}
				if snapshot, ok := s.updateWorkflow(e.Event); ok {
					s.hub.publish("workflow_snapshot", snapshot)
				}
				s.hub.publish("loop_event", e.Event)
			default:
				s.hub.publish("interactive_event", e)
			}
		}
	}()
	return true
}

func (s *server) updateWorkflow(ev query.LoopEvent) (workflowSnapshot, bool) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	switch e := ev.(type) {
	case query.OrchestrationStartedEvent:
		s.workflow = newWorkflowSnapshot()
		s.workflow.Name = e.Name
		s.workflow.Initial = e.Initial
		s.workflow.Current = e.Initial
		s.workflowOn = true
	case query.OrchestrationStateStartedEvent:
		s.ensureWorkflow()
		row := s.workflowState(e.StateID)
		row.Persona = firstNonEmpty(row.Persona, e.PersonaID)
		row.Control = firstNonEmpty(row.Control, e.Control)
		row.Status = "running"
		row.Started = now
		s.workflow.Current = e.StateID
	case query.OrchestrationStateCompletedEvent:
		s.ensureWorkflow()
		row := s.workflowState(e.StateID)
		row.Status = "completed"
		row.Completed = now
		row.Duration = e.Duration.Round(time.Second).String()
	case query.OrchestrationControlEvent:
		s.ensureWorkflow()
		row := s.workflowState(e.StateID)
		row.Control = firstNonEmpty(row.Control, e.Control)
		row.LastEvent = firstNonEmpty(e.Event, row.LastEvent)
	case query.OrchestrationTransitionEvent:
		s.ensureWorkflow()
		s.workflow.Transitions = append(s.workflow.Transitions, workflowTransition{From: e.From, Event: e.Event, To: e.To})
		if e.To != "" {
			s.workflow.Current = e.To
		}
	case query.OrchestrationHandoffEvent:
		s.ensureWorkflow()
		s.workflow.Handoffs = append(s.workflow.Handoffs, workflowHandoff{
			StateID: e.StateID, Event: e.Event, Path: e.Path, Direction: e.Direction,
		})
	case query.OrchestrationCompletedEvent:
		s.ensureWorkflow()
		s.workflow.Name = firstNonEmpty(s.workflow.Name, e.Name)
		s.workflow.Completed = true
		s.workflow.Current = "completed"
	default:
		return workflowSnapshot{}, false
	}
	return cloneWorkflowSnapshot(s.workflow), true
}

func (s *server) ensureWorkflow() {
	if !s.workflowOn || s.workflow.States == nil {
		s.workflow = newWorkflowSnapshot()
		s.workflowOn = true
	}
}

func (s *server) workflowState(id string) *workflowState {
	if id == "" {
		id = "unknown"
	}
	if s.workflow.States == nil {
		s.workflow.States = make(map[string]*workflowState)
	}
	row := s.workflow.States[id]
	if row == nil {
		row = &workflowState{ID: id}
		s.workflow.States[id] = row
	}
	return row
}

func cloneWorkflowSnapshot(in workflowSnapshot) workflowSnapshot {
	out := in
	out.States = make(map[string]*workflowState, len(in.States))
	for id, row := range in.States {
		cp := *row
		out.States[id] = &cp
	}
	out.Transitions = append([]workflowTransition(nil), in.Transitions...)
	out.Handoffs = append([]workflowHandoff(nil), in.Handoffs...)
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *server) rememberArtifact(path string) {
	if strings.TrimSpace(path) == "" {
		return
	}
	s.mu.Lock()
	s.artifacts[filepath.Clean(path)] = struct{}{}
	s.mu.Unlock()
}

func (s *server) handleArtifact(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	path := filepath.Clean(r.URL.Query().Get("path"))
	if path == "." || path == "" {
		http.Error(w, "path is required", http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	_, known := s.artifacts[path]
	s.mu.Unlock()
	if !known {
		http.Error(w, "artifact path was not emitted by this run", http.StatusForbidden)
		return
	}
	content, err := os.ReadFile(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]interface{}{
		"path":    path,
		"content": string(content),
	})
}

type completionItem struct {
	Label       string `json:"label"`
	Detail      string `json:"detail,omitempty"`
	Replacement string `json:"replacement"`
}

type scoredCompletionItem struct {
	item  completionItem
	score int
}

func (s *server) handleCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	items := s.completionItems(r.URL.Query().Get("q"))
	if len(items) > 12 {
		items = items[:12]
	}
	writeJSON(w, items)
}

func (s *server) completionItems(value string) []completionItem {
	if !strings.HasPrefix(value, "/") || strings.Contains(value, "\n") {
		return nil
	}
	cmdToken, hasArgs := firstSlashToken(value)
	if !hasArgs {
		return s.commandCompletionItems(strings.TrimPrefix(cmdToken, "/"))
	}
	name := strings.TrimPrefix(cmdToken, "/")
	if name != "orchestrate" && name != "fsm" {
		return nil
	}
	return s.orchestrateCompletionItems(value)
}

func (s *server) commandCompletionItems(prefix string) []completionItem {
	if s.cfg.SlashCmds == nil {
		return nil
	}
	seen := make(map[string]bool)
	var scored []scoredCompletionItem
	for _, cmd := range s.cfg.SlashCmds.Commands() {
		add := func(name, detail string) {
			score, ok := fuzzyCompletionScore(name, prefix)
			if name == "" || seen[name] || !ok {
				return
			}
			seen[name] = true
			scored = append(scored, scoredCompletionItem{
				item: completionItem{
					Label:       "/" + name,
					Detail:      detail,
					Replacement: "/" + name + " ",
				},
				score: score,
			})
		}
		add(cmd.Name, cmd.Description)
		for _, alias := range cmd.Aliases {
			add(alias, "alias for /"+cmd.Name)
		}
	}
	return sortedCompletionItems(scored)
}

func (s *server) orchestrateCompletionItems(value string) []completionItem {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return nil
	}
	endsSpace := endsWithSpace(value)
	args := fields[1:]
	current := ""
	previous := fields[len(fields)-1]
	currentArgIndex := -1
	if !endsSpace {
		current = fields[len(fields)-1]
		if len(fields) > 1 {
			currentArgIndex = len(fields) - 2
			previous = fields[len(fields)-2]
		}
	}
	state := slash.ParseOrchestrateCompletionState(args, currentArgIndex)
	switch {
	case state.CurrentRole == "definition":
		return pathCompletionItems(value, current, s.cfg.Workspace, pathCompletionYAML)
	case state.CurrentRole == "persona":
		return pathCompletionItems(value, current, s.cfg.Workspace, pathCompletionDir)
	case state.CurrentRole == "flag":
		if strings.HasPrefix(current, "--persona-dir=") {
			prefix := strings.TrimPrefix(current, "--persona-dir=")
			items := pathCompletionItems(value, prefix, s.cfg.Workspace, pathCompletionDir)
			for i := range items {
				items[i].Replacement = replaceCurrentToken(value, "--persona-dir="+items[i].Label+" ")
			}
			return items
		}
		return flagCompletionItems(value, current, state.FlagCandidates())
	case endsSpace && previous != "--prompt":
		switch {
		case previous == "--persona-dir":
			return pathCompletionItems(value, "", s.cfg.Workspace, pathCompletionDir)
		case !state.HasDefinition:
			return pathCompletionItems(value, "", s.cfg.Workspace, pathCompletionYAML)
		default:
			return flagCompletionItems(value, "", state.FlagCandidates())
		}
	}
	return nil
}

func firstSlashToken(value string) (string, bool) {
	rest := strings.TrimPrefix(value, "/")
	i := strings.IndexAny(rest, " \t")
	if i < 0 {
		return value, false
	}
	return "/" + rest[:i], true
}

type pathCompletionKind int

const (
	pathCompletionYAML pathCompletionKind = iota
	pathCompletionDir
)

func pathCompletionItems(value, prefix, cwd string, kind pathCompletionKind) []completionItem {
	if cwd == "" {
		cwd = "."
	}
	dirPart, basePart := filepath.Split(prefix)
	readDir := dirPart
	if readDir == "" {
		readDir = "."
	}
	if !filepath.IsAbs(readDir) {
		readDir = filepath.Join(cwd, readDir)
	}
	entries, err := os.ReadDir(readDir)
	if err != nil {
		return nil
	}
	var scored []scoredCompletionItem
	for _, entry := range entries {
		name := entry.Name()
		score, ok := fuzzyCompletionScore(name, basePart)
		if !ok {
			continue
		}
		isDir := entry.IsDir()
		if kind == pathCompletionDir && !isDir {
			continue
		}
		if kind == pathCompletionYAML && !isDir && !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		candidate := filepath.ToSlash(filepath.Join(dirPart, name))
		if isDir {
			candidate += "/"
		}
		detail := "directory"
		if !isDir {
			detail = "orchestration yaml"
		}
		replacement := candidate
		if kind == pathCompletionDir || !isDir {
			replacement += " "
		}
		scored = append(scored, scoredCompletionItem{
			item: completionItem{
				Label:       candidate,
				Detail:      detail,
				Replacement: replaceCurrentToken(value, replacement),
			},
			score: score,
		})
	}
	return sortedCompletionItems(scored)
}

func flagCompletionItems(value, prefix string, flags []string) []completionItem {
	var scored []scoredCompletionItem
	for _, flag := range flags {
		score, ok := fuzzyCompletionScore(flag, prefix)
		if ok {
			scored = append(scored, scoredCompletionItem{
				item: completionItem{
					Label:       flag,
					Detail:      slash.OrchestrateFlagDetail(flag),
					Replacement: replaceCurrentToken(value, flag+" "),
				},
				score: score,
			})
		}
	}
	return sortedCompletionItems(scored)
}

func sortedCompletionItems(scored []scoredCompletionItem) []completionItem {
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].item.Label < scored[j].item.Label
	})
	items := make([]completionItem, 0, len(scored))
	for _, entry := range scored {
		items = append(items, entry.item)
	}
	return items
}

func fuzzyCompletionScore(candidate, query string) (int, bool) {
	query = strings.ToLower(query)
	candidate = strings.ToLower(candidate)
	if query == "" {
		return 0, true
	}
	if strings.HasPrefix(candidate, query) {
		return 100000 - len(candidate), true
	}
	score := 50000 - len(candidate)
	lastMatch := -1
	searchFrom := 0
	for _, want := range query {
		match := -1
		for i, have := range candidate[searchFrom:] {
			if have == want {
				match = searchFrom + i
				break
			}
		}
		if match < 0 {
			return 0, false
		}
		score += 100
		if isCompletionBoundary(candidate, match) {
			score += 25
		}
		if lastMatch >= 0 {
			gap := match - lastMatch - 1
			if gap == 0 {
				score += 50
			} else {
				score -= gap
			}
		}
		lastMatch = match
		searchFrom = match + 1
	}
	return score, true
}

func isCompletionBoundary(value string, index int) bool {
	if index == 0 || index >= len(value) {
		return true
	}
	switch value[index-1] {
	case '-', '_', '.', '/', ' ':
		return true
	default:
		return false
	}
}

func replaceCurrentToken(value, replacement string) string {
	if endsWithSpace(value) {
		return value + replacement
	}
	start := strings.LastIndexAny(value, " \t")
	if start < 0 {
		return replacement
	}
	return value[:start+1] + replacement
}

func endsWithSpace(value string) bool {
	return value != "" && (value[len(value)-1] == ' ' || value[len(value)-1] == '\t')
}

func (s *server) handlePermission(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/permission/")
	raw, err := decodeRawObject(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var decisionValue string
	if err := json.Unmarshal(raw["decision"], &decisionValue); err != nil {
		http.Error(w, "decision is required", http.StatusBadRequest)
		return
	}
	var remember bool
	if rawRemember, ok := raw["remember"]; ok {
		_ = json.Unmarshal(rawRemember, &remember)
	}
	decision := permission.Decision(decisionValue)
	if decision != permission.DecisionAllow && decision != permission.DecisionDeny {
		http.Error(w, "decision must be allow or deny", http.StatusBadRequest)
		return
	}
	submitted := map[string]interface{}{"id": id, "body": raw}
	if !s.cfg.Bridge.resolvePermission(id, permissionResponse{Decision: decision, Remember: remember}) {
		http.NotFound(w, r)
		return
	}
	s.hub.publish("permission_response", submitted)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) handleAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/ask/")
	raw, err := decodeRawObject(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var answers map[string]string
	if err := json.Unmarshal(raw["answers"], &answers); err != nil {
		http.Error(w, "answers are required", http.StatusBadRequest)
		return
	}
	submitted := map[string]interface{}{"id": id, "body": raw}
	if !s.cfg.Bridge.resolveAsk(id, tool.AskResponse{Answers: answers}) {
		http.NotFound(w, r)
		return
	}
	s.hub.publish("ask_response", submitted)
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) handleSessions(w http.ResponseWriter, r *http.Request) {
	candidates, err := slash.BrowseResumeCandidates(s.cfg.SlashDeps)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if candidates.Candidates == nil {
		writeJSON(w, []interface{}{})
		return
	}
	writeJSON(w, candidates.Candidates)
}

func (s *server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	raw, err := decodeRawObject(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var sessionID string
	if err := json.Unmarshal(raw["session_id"], &sessionID); err != nil {
		http.Error(w, "session_id is required", http.StatusBadRequest)
		return
	}
	s.hub.publish("resume_requested", raw)
	if err := s.resume(sessionID); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) resume(sessionID string) error {
	if s.cfg.Resume == nil {
		return errors.New("session resume is not available")
	}
	if err := s.cfg.Resume(sessionID); err != nil {
		return err
	}
	sess := s.cfg.Store.Snapshot().Conversation
	s.hub.publish("session_resumed", sess)
	return nil
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(completeJSONValue(v))
}

func writeSSE(w http.ResponseWriter, ev eventEnvelope) {
	data, _ := json.Marshal(ev)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}

func decodeRawObject(r *http.Request) (map[string]json.RawMessage, error) {
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return nil, errors.New("request body must be a JSON object")
	}
	return raw, nil
}

var (
	errorType       = reflect.TypeOf((*error)(nil)).Elem()
	jsonMarshalerTy = reflect.TypeOf((*json.Marshaler)(nil)).Elem()
)

func completeJSONValue(v interface{}) interface{} {
	return completeJSONReflect(reflect.ValueOf(v))
}

func completeJSONReflect(v reflect.Value) interface{} {
	if !v.IsValid() {
		return nil
	}
	if v.Type().Implements(errorType) && (!canBeNil(v.Kind()) || !v.IsNil()) {
		err := v.Interface().(error)
		return map[string]interface{}{
			"message": err.Error(),
			"type":    fmt.Sprintf("%T", err),
		}
	}
	if v.Type().Implements(jsonMarshalerTy) && v.CanInterface() {
		return v.Interface()
	}
	if reflect.PointerTo(v.Type()).Implements(jsonMarshalerTy) && v.CanAddr() {
		return v.Addr().Interface()
	}
	switch v.Kind() {
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return nil
		}
		return completeJSONReflect(v.Elem())
	case reflect.Struct:
		out := make(map[string]interface{}, v.NumField())
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := t.Field(i)
			if field.PkgPath != "" {
				continue
			}
			name := jsonFieldName(field)
			if name == "-" {
				continue
			}
			out[name] = completeJSONReflect(v.Field(i))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		if v.Type().Key().Kind() != reflect.String {
			if v.CanInterface() {
				return v.Interface()
			}
			return fmt.Sprint(v)
		}
		out := make(map[string]interface{}, v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out[iter.Key().String()] = completeJSONReflect(iter.Value())
		}
		return out
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		out := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			out[i] = completeJSONReflect(v.Index(i))
		}
		return out
	default:
		if v.CanInterface() {
			return v.Interface()
		}
		return fmt.Sprint(v)
	}
}

func jsonFieldName(field reflect.StructField) string {
	tag := field.Tag.Get("json")
	if tag == "" {
		return field.Name
	}
	name, _, _ := strings.Cut(tag, ",")
	if name == "" {
		return field.Name
	}
	return name
}

func canBeNil(kind reflect.Kind) bool {
	switch kind {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	default:
		return false
	}
}

func newID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

func openBrowser(url string) {
	if runtime.GOOS != "darwin" {
		return
	}
	_ = exec.Command("open", url).Start()
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<link rel="icon" href="data:,">
<title>Pragma</title>
<style>
:root{
  color-scheme:light;
  --bg:#f7f8fa;
  --surface:#fcfcfd;
  --surface-2:#f1f3f6;
  --line:#d9dee5;
  --line-strong:#c6ced8;
  --text:#171a1f;
  --muted:#66717e;
  --muted-2:#8a94a1;
  --accent:#1d5fd1;
  --danger:#9d1c1c;
  --code:#eef1f4;
  --shadow:0 18px 44px rgba(20,28,40,.14);
}
*{box-sizing:border-box}
html,body{height:100%;overflow:hidden}
body{margin:0;background:var(--bg);color:var(--text);font:13px/1.45 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
button,input,textarea{font:inherit}
button{cursor:pointer}
.shell{display:grid;grid-template-columns:292px minmax(0,1fr) 380px;height:100vh;min-height:0}
.rail,.inspector,.workspace{min-height:0}
.rail{display:grid;grid-template-rows:auto auto minmax(0,1fr);background:var(--surface);border-right:1px solid var(--line)}
.brand-block{padding:16px 14px 12px;border-bottom:1px solid var(--line)}
.brand{margin:0;font-size:17px;font-weight:680;letter-spacing:0}
.state-grid{display:grid;grid-template-columns:auto minmax(0,1fr);gap:4px 8px;margin-top:12px;font-size:12px}
.state-grid dt{color:var(--muted)}
.state-grid dd{margin:0;overflow-wrap:anywhere}
.rail-head{display:flex;align-items:center;justify-content:space-between;padding:12px 14px 8px;width:100%;border:0;background:transparent;text-align:left;color:inherit}
.label{font-size:11px;font-weight:700;letter-spacing:.04em;text-transform:uppercase;color:var(--muted)}
#sessionCount{font-size:11px;color:var(--muted)}
.sessions{overflow:auto;overflow-x:hidden;padding:0 8px 14px}
.session-card{border:1px solid transparent;border-radius:7px;padding:8px;margin-bottom:6px;background:transparent}
.session-card:hover{border-color:var(--line);background:var(--surface-2)}
.session-top{display:flex;align-items:flex-start;gap:8px}
.session-open{flex:1;border:0;background:transparent;text-align:left;color:var(--text);padding:0;min-width:0;overflow-wrap:anywhere}
.session-id{font:12px/1.35 ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
.session-meta{margin-top:5px;color:var(--muted);font-size:11px;display:block;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.raw-toggle{border:0;background:transparent;color:var(--muted);padding:0;font-size:11px}
.raw-toggle summary{cursor:pointer}
.raw-toggle pre{max-height:260px;overflow:auto;margin:7px 0 0}
.workspace{display:grid;grid-template-rows:auto auto auto minmax(0,1fr) auto;background:var(--bg)}
.topbar{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:11px 16px;border-bottom:1px solid var(--line);background:var(--surface)}
.status{display:flex;align-items:center;gap:8px;color:var(--muted)}
.dot{width:7px;height:7px;border-radius:50%;background:var(--muted-2)}
.dot.running{background:var(--accent)}
.toolbar{display:grid;grid-template-columns:minmax(240px,320px) minmax(0,1fr);gap:10px;padding:10px 16px;border-bottom:1px solid var(--line);background:var(--surface)}
.toolbar.hidden{visibility:hidden;height:0;padding:0;border:0;overflow:hidden}
.view-tabs{display:flex;gap:4px;align-items:center;padding:8px 16px;border-bottom:1px solid var(--line);background:var(--surface);overflow:auto}
.view-tab{border:1px solid transparent;background:transparent;color:var(--muted);border-radius:6px;padding:7px 10px;white-space:nowrap}
.view-tab.active{background:var(--surface-2);color:var(--text);border-color:var(--line-strong)}
.search{width:100%;border:1px solid var(--line-strong);border-radius:7px;background:#fbfcfd;color:var(--text);padding:8px 10px;outline:none}
.search:focus{border-color:#8ca9dc;box-shadow:0 0 0 3px rgba(29,95,209,.12)}
.filters{display:flex;gap:5px;align-items:center;overflow:auto}
.filter{border:1px solid var(--line);background:transparent;color:var(--muted);border-radius:6px;padding:7px 9px;white-space:nowrap}
.filter.active{background:var(--surface-2);color:var(--text);border-color:var(--line-strong)}
.stream{min-height:0;overflow:auto;padding:16px}
.workbench{display:grid;gap:14px;align-content:start}
.band{border:1px solid var(--line);border-radius:8px;background:var(--surface);padding:12px}
.band-title{display:flex;justify-content:space-between;gap:10px;font-weight:680;margin-bottom:8px}
.band-sub{font-size:12px;color:var(--muted);font-weight:400}
.overview-grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:12px}
.facts{display:grid;grid-template-columns:auto minmax(0,1fr);gap:5px 10px;margin:0;font-size:12px}
.facts dt{color:var(--muted)}
.facts dd{margin:0;overflow-wrap:anywhere}
.item-list{display:grid;gap:7px}
.item-row{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;align-items:start;border:1px solid var(--line);border-radius:7px;background:transparent;color:inherit;text-align:left;padding:9px}
.item-row:hover{background:var(--surface-2)}
.item-main{min-width:0}
.item-title{font-weight:650;overflow-wrap:anywhere}
.item-meta{margin-top:3px;color:var(--muted);font:11px ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
.pill{border:1px solid var(--line);border-radius:999px;padding:2px 7px;color:var(--muted);font-size:11px;white-space:nowrap}
.conversation-list{display:grid;gap:10px}
.message-card{border:1px solid var(--line);border-radius:8px;background:var(--surface);overflow:hidden}
.message-head{display:flex;justify-content:space-between;gap:12px;padding:9px 11px;border-bottom:1px solid var(--line);font-weight:650}
.message-time{color:var(--muted);font:11px ui-monospace,SFMono-Regular,Menlo,monospace;white-space:nowrap}
.message-body{display:grid;gap:8px;padding:10px 11px}
.part{display:grid;gap:5px}
.part-label{color:var(--muted);font-size:11px;font-weight:650;text-transform:uppercase;letter-spacing:.04em}
.part-text{white-space:pre-wrap;overflow-wrap:anywhere}
.workflow-grid{display:grid;grid-template-columns:minmax(220px,320px) minmax(0,1fr);gap:12px}
.state-row.running{border-color:#8ca9dc;background:#f4f7fc}
.state-row.completed{background:#f7f8fa}
.handoff-path{font:11px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--muted);overflow-wrap:anywhere}
.artifact-content{white-space:pre-wrap}
.empty{max-width:540px;margin:12vh auto 0;color:var(--muted);text-align:center}
.event-card{border:1px solid var(--line);border-radius:8px;background:var(--surface);margin:0 0 10px;overflow:hidden}
.event-card.selected{border-color:#8ca9dc;box-shadow:0 0 0 3px rgba(29,95,209,.10)}
.event-head{display:grid;grid-template-columns:auto minmax(0,1fr) auto;gap:10px;align-items:start;width:100%;border:0;background:transparent;color:inherit;text-align:left;padding:10px 11px}
.seq{font:12px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--muted)}
.event-title{min-width:0}
.event-type{font-weight:650;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.event-sub{font:11px ui-monospace,SFMono-Regular,Menlo,monospace;color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;margin-top:2px}
.event-preview{padding:0 11px 10px 44px;color:var(--muted);font-size:12px;white-space:pre-wrap;overflow-wrap:anywhere}
.event-time{font-size:11px;color:var(--muted);white-space:nowrap}
.event-card details{border-top:1px solid var(--line);padding:8px 11px 11px}
.event-card summary{cursor:pointer;color:var(--muted);font-size:11px}
.composer{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;padding:12px 16px;border-top:1px solid var(--line);background:var(--surface)}
.composer-box{min-width:0;position:relative}
#prompt{display:block;width:100%;min-height:62px;max-height:180px;resize:vertical;border:1px solid var(--line-strong);border-radius:7px;background:#fbfcfd;color:var(--text);padding:10px 11px;outline:none}
#prompt:focus{border-color:#8ca9dc;box-shadow:0 0 0 3px rgba(29,95,209,.12)}
.hint{font-size:11px;color:var(--muted);margin-top:6px}
.send{align-self:end;min-width:76px;height:40px;border:1px solid var(--accent);border-radius:7px;background:var(--accent);color:#f8fbff;font-weight:650}
.send.cancel{border-color:var(--danger);background:var(--danger)}
.send:disabled{opacity:.55;cursor:not-allowed}
.completion-menu{position:absolute;left:0;right:0;bottom:calc(100% + 8px);display:none;max-height:260px;overflow:auto;border:1px solid var(--line-strong);border-radius:8px;background:var(--surface);box-shadow:var(--shadow);z-index:25;padding:6px}
.completion-menu.open{display:grid;gap:3px}
.completion-item{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;width:100%;border:0;border-radius:6px;background:transparent;color:inherit;text-align:left;padding:8px 9px}
.completion-item.active,.completion-item:hover{background:var(--surface-2)}
.completion-label{font-weight:650;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.completion-detail{color:var(--muted);font-size:11px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.inspector{display:grid;grid-template-rows:auto auto minmax(0,1fr);border-left:1px solid var(--line);background:var(--surface)}
.inspector-head{display:grid;grid-template-columns:minmax(0,1fr) auto;gap:10px;align-items:start;padding:14px;border-bottom:1px solid var(--line)}
.inspector-close{display:none;border:1px solid var(--line);background:var(--surface);border-radius:6px;padding:5px 8px;color:var(--muted)}
.inspector-title{font-weight:680;overflow-wrap:anywhere}
.inspector-sub{margin-top:4px;color:var(--muted);font:11px ui-monospace,SFMono-Regular,Menlo,monospace;overflow-wrap:anywhere}
.tabs{display:flex;gap:4px;padding:8px 10px;border-bottom:1px solid var(--line)}
.tab{border:1px solid transparent;background:transparent;color:var(--muted);border-radius:6px;padding:6px 8px}
.tab.active{background:var(--surface-2);border-color:var(--line);color:var(--text)}
.inspector-body{overflow:auto;padding:12px}
.related-list{display:grid;gap:6px}
.related-item{border:1px solid var(--line);background:var(--surface);border-radius:6px;padding:8px;text-align:left;color:inherit}
.related-item small{display:block;color:var(--muted);margin-top:3px}
.modal-root:empty{display:none}
.panel{position:fixed;right:16px;bottom:92px;width:min(560px,calc(100vw - 32px));max-height:min(640px,calc(100vh - 120px));display:grid;grid-template-rows:auto minmax(0,1fr) auto;gap:10px;background:var(--surface);border:1px solid var(--line);border-radius:8px;box-shadow:var(--shadow);padding:14px;z-index:20}
.panel-title{font-weight:680}
.panel-intro{color:var(--muted);font-size:12px;margin-top:3px}
.panel-body{overflow:auto}
.panel-actions{display:flex;gap:8px;align-items:center;justify-content:flex-end}
.panel textarea{width:100%;min-height:80px;border:1px solid var(--line-strong);border-radius:7px;padding:9px}
.panel-details{margin-top:10px}
.panel-details summary{cursor:pointer;color:var(--muted);font-size:11px}
.question-list{display:grid;gap:10px;margin-top:10px}
.question-field{display:grid;gap:6px}
.question-label{font-size:12px;font-weight:650}
.option-row{display:flex;gap:7px;align-items:flex-start;font-size:12px;color:var(--muted)}
.option-row input{margin-top:2px}
.secondary{border:1px solid var(--line);background:var(--surface);border-radius:7px;padding:7px 10px;color:var(--text)}
.danger{border:1px solid #c18b8b;background:#fff7f7;color:var(--danger);border-radius:7px;padding:7px 10px}
.primary{border:1px solid var(--accent);background:var(--accent);color:#f8fbff;border-radius:7px;padding:7px 10px}
pre{margin:0;overflow:auto;background:var(--code);border-radius:7px;padding:10px;font:12px/1.45 ui-monospace,SFMono-Regular,Menlo,monospace;white-space:pre-wrap;word-break:break-word}
@media (max-width:1100px){
  .shell{grid-template-columns:270px minmax(0,1fr);grid-template-rows:minmax(0,1fr) 34vh}
  .inspector{grid-column:1/3;grid-row:2;border-left:0;border-top:1px solid var(--line)}
}
@media (max-width:760px){
  .shell{grid-template-columns:1fr;grid-template-rows:auto minmax(0,1fr)}
  .rail{grid-template-rows:auto auto auto;border-right:0;border-bottom:1px solid var(--line);max-height:24vh;overflow:hidden}
  .rail-head{padding:10px 14px;border-top:1px solid var(--line)}
  .sessions{display:none;overflow-x:hidden;overflow-y:auto;padding:0 10px 10px}
  body.runs-open .rail{max-height:52vh;overflow:auto}
  body.runs-open .sessions{display:block}
  .session-card{min-width:0;margin:0 0 6px}
  .toolbar{grid-template-columns:1fr}
  .view-tabs{flex-wrap:wrap;overflow:visible}
  .overview-grid,.workflow-grid{grid-template-columns:1fr}
  .filters{padding-bottom:2px}
  .stream{padding:12px}
  .composer{grid-template-columns:1fr;padding:10px}
  .send{width:100%}
  .inspector{position:fixed;inset:0;z-index:30;display:none;border:0}
  body.inspector-open .inspector{display:grid}
  .inspector-close{display:block}
}
</style>
</head>
<body>
<div class="shell">
  <aside class="rail" aria-label="Sessions">
    <section class="brand-block">
      <h1 class="brand">Pragma</h1>
      <dl class="state-grid" id="state"></dl>
      <details class="raw-toggle state-raw">
        <summary>Full state JSON</summary>
        <pre id="rawState">{}</pre>
      </details>
    </section>
    <button class="rail-head" id="runsToggle" type="button">
      <div class="label">Recent runs</div>
      <div id="sessionCount">0</div>
    </button>
    <nav class="sessions" id="sessions" aria-label="Saved sessions"></nav>
  </aside>

  <main class="workspace">
    <header class="topbar">
      <div class="status"><span class="dot" id="statusDot"></span><span id="status">Ready</span></div>
      <div class="label" id="eventCount">0 activity items</div>
    </header>
    <nav class="view-tabs" id="viewTabs" aria-label="Workbench views"></nav>
    <section class="toolbar" aria-label="Activity controls">
      <input class="search" id="search" type="search" placeholder="Search activity and details">
      <div class="filters" id="filters" role="tablist" aria-label="Activity filters"></div>
    </section>
    <section class="stream" id="stream" aria-live="polite">
      <div class="empty">Start a message or run a slash command.</div>
    </section>
    <form class="composer" id="promptForm">
      <div class="composer-box">
        <textarea id="prompt" placeholder="Message Pragma"></textarea>
        <div class="completion-menu" id="completionMenu" role="listbox" aria-label="Completions"></div>
        <div class="hint">Enter sends, Shift+Enter adds a new line.</div>
      </div>
      <button class="send" id="sendButton" type="button">Send</button>
    </form>
  </main>

  <aside class="inspector" aria-label="Details">
    <header class="inspector-head">
      <div>
        <div class="inspector-title" id="inspectorTitle">No item selected</div>
        <div class="inspector-sub" id="inspectorSub">Select activity or a session to view details.</div>
      </div>
      <button class="inspector-close" id="inspectorClose" type="button">Close</button>
    </header>
    <nav class="tabs" aria-label="Details tabs">
      <button class="tab active" data-tab="raw" type="button">Full JSON</button>
      <button class="tab" data-tab="text" type="button">File/Text</button>
      <button class="tab" data-tab="related" type="button">Related</button>
    </nav>
    <section class="inspector-body" id="inspectorBody">
      <pre>{}</pre>
    </section>
  </aside>
</div>
<div class="modal-root" id="modal"></div>
<script>
const state = {
  events: [],
  sessions: [],
  appState: null,
  runtime: null,
  promptHistory: [],
  selected: null,
  view: 'overview',
  filter: 'all',
  search: '',
  tab: 'raw',
  artifacts: {},
  workflowSnapshot: null,
  historyIndex: -1,
  historyDraft: '',
  completions: [],
  completionIndex: 0,
  completionRequest: 0,
  running: false,
  cancelPending: false
};

const streamEl = document.getElementById('stream');
const sessionsEl = document.getElementById('sessions');
const statusEl = document.getElementById('status');
const statusDot = document.getElementById('statusDot');
const eventCountEl = document.getElementById('eventCount');
const sessionCountEl = document.getElementById('sessionCount');
const inspectorTitle = document.getElementById('inspectorTitle');
const inspectorSub = document.getElementById('inspectorSub');
const inspectorBody = document.getElementById('inspectorBody');
const modal = document.getElementById('modal');
const promptEl = document.getElementById('prompt');
const completionMenu = document.getElementById('completionMenu');
const sendButton = document.getElementById('sendButton');

const filterDefs = [
  ['all','All'],
  ['model','Model'],
  ['text','Text'],
  ['tool','Tool'],
  ['permission','Permission'],
  ['ask','Ask'],
  ['orchestration','Orchestration'],
  ['error','Error']
];

const viewDefs = [
  ['overview','Overview'],
  ['conversation','Conversation'],
  ['workflow','Workflow'],
  ['handoffs','Handoffs'],
  ['activity','Activity'],
  ['state','State']
];

function json(value){ return JSON.stringify(value, null, 2); }
function label(value){ return value === undefined || value === null || value === '' ? 'none' : String(value); }
function receivedTime(value){ const d = new Date(value); return Number.isNaN(d.getTime()) ? '' : d.toLocaleTimeString(); }
function fullText(value){ try { return json(value).toLowerCase(); } catch { return String(value).toLowerCase(); } }
function displayValue(value){
  if(value === undefined || value === null) return '';
  if(typeof value === 'string') return value;
  try { return json(value); } catch { return String(value); }
}
function compactText(value, limit=220){
  const text = displayValue(value).replace(/\s+/g, ' ').trim();
  return text.length > limit ? text.slice(0, limit - 1) + '…' : text;
}
function eventCategory(envelope){
  switch(envelope.type){
    case 'permission_request':
    case 'permission_response':
    case 'permission_response_submitted':
      return 'permission';
    case 'ask_request':
    case 'ask_response':
    case 'ask_response_submitted':
      return 'ask';
    case 'tool_call':
    case 'tool_result':
      return 'tool';
    case 'model_request':
    case 'model_response':
      return 'model';
    case 'orchestration_started':
    case 'orchestration_state_started':
    case 'orchestration_state_completed':
    case 'orchestration_control':
    case 'orchestration_transition':
    case 'orchestration_handoff':
    case 'orchestration_completed':
    case 'workflow_snapshot':
      return 'orchestration';
    case 'run_error':
      return 'error';
    case 'text':
    case 'thinking':
      return 'text';
    default:
      return 'all';
  }
}

function eventDisplayTitle(envelope){
  const data = envelope.data || {};
  if(envelope.type === 'prompt_submitted' || envelope.type === 'user_prompt' || envelope.type === 'prompt_accepted') return 'You';
  if(envelope.type === 'permission_request') return 'Permission needed';
  if(envelope.type === 'permission_response' || envelope.type === 'permission_response_submitted') return 'Permission answered';
  if(envelope.type === 'ask_request') return 'Question';
  if(envelope.type === 'ask_response' || envelope.type === 'ask_response_submitted') return 'Question answered';
  if(envelope.type === 'session_resumed') return 'Session resumed';
  if(envelope.type === 'resume_requested') return 'Resume requested';
  if(envelope.type === 'slash_result') return 'Command result';
  if(envelope.type === 'run_idle') return 'Ready';
  switch(envelope.type){
    case 'text': return 'Assistant';
    case 'thinking': return 'Thinking';
    case 'tool_call': return 'Tool call' + (data.name ? ': ' + data.name : '');
    case 'tool_result': return 'Tool result';
    case 'model_request': return 'Contacting model';
    case 'model_response': return 'Model responded';
    case 'run_error': return 'Run error';
    case 'orchestration_started': return 'Workflow started';
    case 'orchestration_state_started': return 'Workflow step started';
    case 'orchestration_control': return 'Workflow control step';
    case 'orchestration_handoff': return 'Workflow handoff';
    case 'orchestration_transition': return 'Workflow moved';
    case 'orchestration_state_completed': return 'Workflow step completed';
    case 'orchestration_completed': return 'Workflow completed';
  }
  return envelope.type || 'Activity';
}

function eventDisplayMeta(envelope){
  const parts = [];
  if(envelope.type) parts.push(envelope.type);
  if(envelope.data_type) parts.push(envelope.data_type);
  return parts.join(' · ');
}

function eventPreview(envelope){
  const data = envelope.data || {};
  if(envelope.type === 'prompt_submitted' || envelope.type === 'user_prompt' || envelope.type === 'prompt_accepted') return compactText(data.prompt || data.Prompt || '');
  if(envelope.type === 'text' || envelope.type === 'thinking') return compactText(data.text || '');
  if(envelope.type === 'tool_call') return compactText(data.input || '');
  if(envelope.type === 'tool_result') return compactText(data.display || data.content || '');
  if(envelope.type === 'run_error') return compactText(data.message || '');
  if(envelope.type === 'permission_request') return compactText([data.tool, data.content, data.reason].filter(Boolean).join(' · '));
  if(envelope.type === 'ask_request') {
    const req = data.request || data.Request || {};
    if(req.Question || req.question) return compactText(req.Question || req.question);
    const questions = req.Questions || req.questions || [];
    if(questions.length) return compactText(questions.map(q => q.Question || q.question).join(' · '));
  }
  if(eventCategory(envelope) === 'orchestration') return compactText(Object.entries(data).map(([k,v]) => k + ': ' + displayValue(v)).join(' · '));
  return '';
}
function matchesCurrentView(envelope){
  const filterMatch = state.filter === 'all' || eventCategory(envelope) === state.filter;
  const searchMatch = !state.search || fullText(envelope).includes(state.search);
  return filterMatch && searchMatch;
}

function workflowView(){
  const snap = state.workflowSnapshot || {};
  const wf = {
    name: snap.name || '',
    initial: snap.initial || '',
    current: snap.current || '',
    completed: Boolean(snap.completed),
    states: new Map(),
    transitions: (snap.transitions || []).map(t => ({from: t.from, event: t.event, to: t.to})),
    handoffs: (snap.handoffs || []).map(h => ({
      stateID: h.state_id,
      event: h.event,
      path: h.path,
      direction: h.direction
    })),
    events: state.events.filter(envelope => envelope.type === 'workflow_snapshot')
  };
  Object.values(snap.states || {}).forEach(row => {
    if(!row || !row.id) return;
    wf.states.set(row.id, {
      id: row.id,
      persona: row.persona,
      control: row.control,
      status: row.status,
      lastEvent: row.last_event,
      started: row.started,
      completed: row.completed,
      duration: row.duration,
      events: []
    });
  });
  return wf;
}

function stateHandoff(){
  const app = state.appState || {};
  return app.handoff_state || app.HandoffState || app.Handoff || null;
}

function currentSessionLabel(id){
  return id ? 'current' : '';
}

function renderState(s){
  state.runtime = s.runtime || {};
  state.appState = s.app_state || s.AppState || null;
  state.promptHistory = s.prompt_history || [];
  const el = document.getElementById('state');
  const raw = document.getElementById('rawState');
  raw.textContent = json(s);
  const runtime = state.runtime || {};
  setRunning(Boolean(runtime.running));
  const appState = state.appState || {};
  const conversation = appState.conversation || appState.Conversation || {};
  const rows = [
    ['Provider', runtime.provider],
    ['Model', runtime.model || appState.model || appState.Model],
    ['Session', currentSessionLabel(conversation.id || conversation.ID)],
    ['Workspace', runtime.workspace]
  ];
  el.innerHTML = '';
  rows.forEach(([k,v]) => {
    const dt = document.createElement('dt');
    dt.textContent = k;
    const dd = document.createElement('dd');
    dd.textContent = label(v);
    el.append(dt, dd);
  });
  renderMain();
}

function renderFilters(){
  const el = document.getElementById('filters');
  el.innerHTML = '';
  filterDefs.forEach(([key,text]) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'filter' + (state.filter === key ? ' active' : '');
    button.textContent = text;
    button.onclick = () => { state.filter = key; renderFilters(); renderMain(); };
    el.appendChild(button);
  });
}

function renderViewTabs(){
  const el = document.getElementById('viewTabs');
  el.innerHTML = '';
  viewDefs.forEach(([key,text]) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'view-tab' + (state.view === key ? ' active' : '');
    button.textContent = text;
    button.onclick = () => {
      state.view = key;
      renderViewTabs();
      renderMain();
    };
    el.appendChild(button);
  });
}

function renderMain(){
  const toolbar = document.querySelector('.toolbar');
  toolbar.classList.toggle('hidden', state.view !== 'activity');
  if(state.view === 'activity'){
    renderEvents();
    return;
  }
  streamEl.innerHTML = '';
  streamEl.scrollTop = 0;
  if(state.view === 'overview') renderOverview();
  if(state.view === 'conversation') renderConversation();
  if(state.view === 'workflow') renderWorkflow();
  if(state.view === 'handoffs') renderHandoffs();
  if(state.view === 'state') renderStateView();
}

function appendFactList(container, rows){
  const dl = document.createElement('dl');
  dl.className = 'facts';
  rows.forEach(([k,v]) => {
    const dt = document.createElement('dt');
    dt.textContent = k;
    const dd = document.createElement('dd');
    dd.textContent = label(v);
    dl.append(dt, dd);
  });
  container.appendChild(dl);
}

function band(title, sub){
  const section = document.createElement('section');
  section.className = 'band';
  const head = document.createElement('div');
  head.className = 'band-title';
  const titleEl = document.createElement('div');
  titleEl.textContent = title;
  const subEl = document.createElement('div');
  subEl.className = 'band-sub';
  subEl.textContent = sub || '';
  head.append(titleEl, subEl);
  section.appendChild(head);
  return section;
}

function renderOverview(){
  const wrap = document.createElement('div');
  wrap.className = 'workbench overview-grid';
  const runtime = state.runtime || {};
  const app = state.appState || {};
  const conversation = app.conversation || app.Conversation || {};
  const wf = workflowView();

  const active = band('Active run', state.events.length + ' events');
  appendFactList(active, [
    ['Provider', runtime.provider],
    ['Model', runtime.model || app.model || app.Model],
    ['Workspace', runtime.workspace],
    ['Session', currentSessionLabel(conversation.id || conversation.ID)],
    ['Status', statusEl.textContent]
  ]);
  active.onclick = () => selectItem('app_state', {runtime, app_state: app}, 'Current app state', 'runtime + app_state');

  const workflow = band('Workflow', wf.events.length ? (wf.current || 'active') : 'none');
  appendFactList(workflow, [
    ['Name', wf.name],
    ['Initial', wf.initial],
    ['Current', wf.current],
    ['States', wf.states.size],
    ['Transitions', wf.transitions.length],
    ['Handoffs', wf.handoffs.length]
  ]);
  workflow.onclick = () => { state.view = 'workflow'; renderViewTabs(); renderMain(); };

  const blockers = band('Blocking prompts', '');
  const blocking = state.events.filter(ev => ev.type === 'permission_request' || ev.type === 'ask_request').slice(-5).reverse();
  const blist = document.createElement('div');
  blist.className = 'item-list';
  if(!blocking.length) blist.appendChild(emptyInline('No pending prompt events in the current stream.'));
  blocking.forEach(ev => blist.appendChild(eventRow(ev)));
  blockers.appendChild(blist);

  const recent = band('Recent activity', '');
  const rlist = document.createElement('div');
  rlist.className = 'item-list';
  state.events.slice(-8).reverse().forEach(ev => rlist.appendChild(eventRow(ev)));
  if(!state.events.length) rlist.appendChild(emptyInline('Start a message or run a slash command.'));
  recent.appendChild(rlist);

  wrap.append(active, workflow, blockers, recent);
  streamEl.appendChild(wrap);
}

function conversation(){
  const app = state.appState || {};
  return app.conversation || app.Conversation || {};
}

function conversationMessages(){
  const conv = conversation();
  return conv.messages || conv.Messages || [];
}

function contentParts(message){
  return message.content || message.Content || [];
}

function partType(part){
  return part.type || part.Type || 'part';
}

function partData(part){
  return part.data || part.Data || part;
}

function partDisplay(part){
  const typ = partType(part);
  const data = partData(part) || {};
  if(typ === 'text') return data.text || data.Text || '';
  if(typ === 'thinking') return data.text || data.Text || json(data);
  if(typ === 'tool_call'){
    const input = data.input || data.Input || {};
    return [data.name || data.Name || 'tool_call', displayValue(input)].filter(Boolean).join('\n');
  }
  if(typ === 'tool_result') return data.content || data.Content || json(data);
  return json(part);
}

function renderConversation(){
  const wrap = document.createElement('div');
  wrap.className = 'workbench';
  const messages = conversationMessages();
  const conv = conversation();
  const summary = band('Conversation', messages.length ? messages.length + ' messages' : 'empty');
  appendFactList(summary, [
    ['Session', currentSessionLabel(conv.id || conv.ID)],
    ['Model', conv.model || conv.Model],
    ['Created', conv.created_at || conv.CreatedAt],
    ['Updated', conv.updated_at || conv.UpdatedAt]
  ]);
  wrap.appendChild(summary);

  const list = document.createElement('div');
  list.className = 'conversation-list';
  if(!messages.length){
    list.appendChild(emptyInline('No conversation messages are loaded for this run.'));
  }
  messages.forEach((message, index) => {
    const card = document.createElement('article');
    card.className = 'message-card';
    const head = document.createElement('div');
    head.className = 'message-head';
    const role = document.createElement('div');
    role.textContent = label(message.role || message.Role || ('message ' + (index + 1)));
    const time = document.createElement('div');
    time.className = 'message-time';
    time.textContent = message.timestamp || message.Timestamp || '';
    head.append(role, time);

    const body = document.createElement('div');
    body.className = 'message-body';
    const parts = contentParts(message);
    if(!parts.length) body.appendChild(emptyInline('No content parts.'));
    parts.forEach(part => {
      const section = document.createElement('section');
      section.className = 'part';
      const pLabel = document.createElement('div');
      pLabel.className = 'part-label';
      pLabel.textContent = partType(part);
      const content = document.createElement('div');
      content.className = 'part-text';
      content.textContent = partDisplay(part);
      section.append(pLabel, content);
      body.appendChild(section);
    });

    card.onclick = () => selectItem('message', message, 'Message ' + (index + 1), message.role || message.Role || '');
    card.append(head, body);
    list.appendChild(card);
  });
  wrap.appendChild(list);
  streamEl.appendChild(wrap);
}

function emptyInline(text){
  const div = document.createElement('div');
  div.className = 'band-sub';
  div.textContent = text;
  return div;
}

function eventRow(envelope){
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'item-row';
  button.onclick = () => selectItem('event', envelope, eventDisplayTitle(envelope), eventDisplayMeta(envelope));
  const main = document.createElement('div');
  main.className = 'item-main';
  const title = document.createElement('div');
  title.className = 'item-title';
  title.textContent = eventDisplayTitle(envelope);
  const meta = document.createElement('div');
  meta.className = 'item-meta';
  meta.textContent = eventDisplayMeta(envelope);
  main.append(title, meta);
  const pill = document.createElement('div');
  pill.className = 'pill';
  pill.textContent = '#' + label(envelope.sequence);
  button.append(main, pill);
  return button;
}

function renderWorkflow(){
  const wf = workflowView();
  const wrap = document.createElement('div');
  wrap.className = 'workbench workflow-grid';
  const summary = band('Workflow', wf.events.length ? (wf.name || 'orchestration') : 'not started');
  appendFactList(summary, [
    ['Name', wf.name],
    ['Initial', wf.initial],
    ['Current', wf.current],
    ['Completed', wf.completed ? 'yes' : 'no'],
    ['Events', wf.events.length]
  ]);

  const states = band('States', wf.states.size + '');
  const stateList = document.createElement('div');
  stateList.className = 'item-list';
  Array.from(wf.states.values()).forEach(row => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'item-row state-row ' + (row.status || '');
    button.onclick = () => {
      const ev = row.events[row.events.length - 1];
      selectItem('workflow_state', {state: row, source_events: row.events}, 'Workflow state ' + row.id, ev ? eventDisplayMeta(ev) : '');
    };
    const main = document.createElement('div');
    main.className = 'item-main';
    const title = document.createElement('div');
    title.className = 'item-title';
    title.textContent = row.id;
    const meta = document.createElement('div');
    meta.className = 'item-meta';
    meta.textContent = [row.persona, row.control, row.lastEvent].filter(Boolean).join(' · ');
    main.append(title, meta);
    const pill = document.createElement('div');
    pill.className = 'pill';
    pill.textContent = row.status || 'seen';
    button.append(main, pill);
    stateList.appendChild(button);
  });
  if(!wf.states.size) stateList.appendChild(emptyInline('No orchestration state events yet.'));
  states.appendChild(stateList);

  const transitions = band('Transitions', wf.transitions.length + '');
  const tlist = document.createElement('div');
  tlist.className = 'item-list';
  wf.transitions.forEach(t => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'item-row';
    button.onclick = () => selectItem('workflow_transition', t, 'Workflow transition', [t.from, t.event, t.to].filter(Boolean).join(' -> '));
    const main = document.createElement('div');
    main.className = 'item-main';
    const title = document.createElement('div');
    title.className = 'item-title';
    title.textContent = label(t.from) + ' -> ' + label(t.to);
    const meta = document.createElement('div');
    meta.className = 'item-meta';
    meta.textContent = 'event: ' + label(t.event);
    main.append(title, meta);
    const pill = document.createElement('div');
    pill.className = 'pill';
    pill.textContent = 'transition';
    button.append(main, pill);
    tlist.appendChild(button);
  });
  if(!wf.transitions.length) tlist.appendChild(emptyInline('No transitions yet.'));
  transitions.appendChild(tlist);

  wrap.append(summary, states, transitions);
  streamEl.appendChild(wrap);
}

function renderHandoffs(){
  const wf = workflowView();
  const wrap = document.createElement('div');
  wrap.className = 'workbench';
  const files = band('Orchestration handoff files', wf.handoffs.length + '');
  const list = document.createElement('div');
  list.className = 'item-list';
  wf.handoffs.forEach(h => list.appendChild(handoffRow(h)));
  if(!wf.handoffs.length) list.appendChild(emptyInline('No orchestration handoff file events yet.'));
  files.appendChild(list);

  const sessionState = band('Session handoff state', stateHandoff() ? 'available' : 'none');
  if(stateHandoff()){
    const row = document.createElement('button');
    row.type = 'button';
    row.className = 'item-row';
    row.onclick = () => selectItem('handoff_state', stateHandoff(), 'Session handoff state', 'app_state.handoff_state');
    const main = document.createElement('div');
    main.className = 'item-main';
    const title = document.createElement('div');
    title.className = 'item-title';
    title.textContent = 'Current session handoff JSON';
    const meta = document.createElement('div');
    meta.className = 'item-meta';
    meta.textContent = 'source: app_state';
    main.append(title, meta);
    row.append(main);
    sessionState.appendChild(row);
  } else {
    sessionState.appendChild(emptyInline('No session handoff state in current app state.'));
  }

  wrap.append(files, sessionState);
  streamEl.appendChild(wrap);
}

function handoffRow(h){
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'item-row';
  button.onclick = () => {
    if(h.path) {
      loadArtifact(h.path, h);
      return;
    }
    selectItem('handoff_event', h, 'Handoff ' + label(h.direction), label(h.path));
  };
  const main = document.createElement('div');
  main.className = 'item-main';
  const title = document.createElement('div');
  title.className = 'item-title';
  title.textContent = label(h.stateID) + ' / ' + label(h.event);
  const meta = document.createElement('div');
  meta.className = 'handoff-path';
  meta.textContent = label(h.path);
  main.append(title, meta);
  const actions = document.createElement('div');
  const pill = document.createElement('div');
  pill.className = 'pill';
  pill.textContent = label(h.direction);
  actions.appendChild(pill);
  button.append(main, actions);
  return button;
}

async function loadArtifact(path, source){
  const cached = state.artifacts[path];
  if(cached){
    selectItem('artifact', cached, 'Handoff file', path);
    return;
  }
  const response = await fetch('/api/artifact?path=' + encodeURIComponent(path));
  if(!response.ok){
    appendLocalError(await response.text());
    return;
  }
  const artifact = await response.json();
  artifact.source_event = source;
  state.artifacts[path] = artifact;
  selectItem('artifact', artifact, 'Handoff file', path);
}

function renderStateView(){
  const wrap = document.createElement('div');
  wrap.className = 'workbench';
  const current = band('Current runtime and app state', '');
  const pre = document.createElement('pre');
  pre.textContent = json({runtime: state.runtime, app_state: state.appState});
  current.appendChild(pre);
  const history = band('Prompt history', state.promptHistory.length + '');
  const preHistory = document.createElement('pre');
  preHistory.textContent = json(state.promptHistory);
  history.appendChild(preHistory);
  wrap.append(current, history);
  streamEl.appendChild(wrap);
}

function pathBase(path){
  if(!path) return '';
  const parts = String(path).split('/').filter(Boolean);
  return parts.length ? parts[parts.length - 1] : String(path);
}

function sessionTitle(session){
  const workspace = pathBase(session.work_dir || session.workDir || '');
  const turns = session.turn_count !== undefined ? session.turn_count : session.turnCount;
  const parts = [];
  if(workspace) parts.push(workspace);
  if(turns !== undefined) parts.push(turns + ' turns');
  return parts.length ? parts.join(' · ') : 'Session';
}

function sessionMeta(session){
  return [session.model, session.updated_at].filter(Boolean).join(' · ');
}

async function refreshState(){
  const response = await fetch('/api/state');
  if(!response.ok) throw new Error(await response.text());
  const body = await response.json();
  renderState(body);
  return body;
}

async function openSession(session, button){
  if(!session || !session.id) return;
  const previousText = button ? button.textContent : '';
  if(button){
    button.disabled = true;
    button.textContent = 'Opening...';
  }
  try{
    const response = await fetch('/api/resume', {
      method:'POST',
      headers:{'content-type':'application/json'},
      body:JSON.stringify({session_id: session.id})
    });
    if(!response.ok) throw new Error(await response.text());
    await refreshState();
    state.view = 'conversation';
    state.selected = null;
    document.body.classList.remove('inspector-open', 'runs-open');
    renderViewTabs();
    renderMain();
    statusEl.textContent = 'Session resumed';
  }catch(err){
    appendLocalError(err.message);
  }finally{
    if(button){
      button.disabled = false;
      button.textContent = previousText;
    }
  }
}

function renderSessions(list){
  state.sessions = Array.isArray(list) ? list : [];
  sessionCountEl.textContent = state.sessions.length + '';
  sessionsEl.innerHTML = '';
  state.sessions.forEach((session, index) => {
    const card = document.createElement('article');
    card.className = 'session-card';
    const top = document.createElement('div');
    top.className = 'session-top';
    const open = document.createElement('button');
    open.className = 'session-open';
    open.type = 'button';
    open.textContent = sessionTitle(session);
    open.onclick = event => {
      event.stopPropagation();
      openSession(session, open);
    };
    top.appendChild(open);
    const meta = document.createElement('div');
    meta.className = 'session-meta';
    meta.textContent = sessionMeta(session);
    card.append(top, meta);
    card.onclick = () => {
      openSession(session, open);
    };
    sessionsEl.appendChild(card);
  });
}

function renderEvents(){
  streamEl.innerHTML = '';
  const visible = state.events.filter(matchesCurrentView);
  eventCountEl.textContent = state.events.length + (state.events.length === 1 ? ' activity item' : ' activity items');
  if(visible.length === 0){
    const empty = document.createElement('div');
    empty.className = 'empty';
    empty.textContent = state.events.length === 0 ? 'Start a message or run a slash command.' : 'No activity matches the current filter.';
    streamEl.appendChild(empty);
    return;
  }
  visible.forEach(envelope => streamEl.appendChild(eventCard(envelope)));
}

function eventCard(envelope){
  const card = document.createElement('article');
  card.className = 'event-card' + (state.selected && state.selected.value === envelope ? ' selected' : '');
  const head = document.createElement('button');
  head.type = 'button';
  head.className = 'event-head';
  head.onclick = () => selectItem('event', envelope, eventDisplayTitle(envelope), eventDisplayMeta(envelope));

  const seq = document.createElement('div');
  seq.className = 'seq';
  seq.textContent = '#' + label(envelope.sequence);
  const title = document.createElement('div');
  title.className = 'event-title';
  const type = document.createElement('div');
  type.className = 'event-type';
  type.textContent = eventDisplayTitle(envelope);
  const sub = document.createElement('div');
  sub.className = 'event-sub';
  sub.textContent = eventDisplayMeta(envelope);
  title.append(type, sub);
  const time = document.createElement('div');
  time.className = 'event-time';
  time.textContent = receivedTime(envelope.received_at);
  head.append(seq, title, time);

  const previewText = eventPreview(envelope);
  if(previewText){
    const preview = document.createElement('div');
    preview.className = 'event-preview';
    preview.textContent = previewText;
    card.append(head, preview);
  } else {
    card.appendChild(head);
  }

  const details = document.createElement('details');
  const summary = document.createElement('summary');
  summary.textContent = 'Full event JSON';
  const pre = document.createElement('pre');
  pre.textContent = json(envelope);
  details.append(summary, pre);
  card.appendChild(details);
  return card;
}

function selectItem(kind, value, title, subtitle){
  state.selected = {kind, value, title, subtitle};
  inspectorTitle.textContent = title;
  inspectorSub.textContent = subtitle || kind;
  document.body.classList.add('inspector-open');
  renderInspector();
  renderMain();
}

function renderInspector(){
  const selected = state.selected;
  document.querySelectorAll('.tab').forEach(tab => tab.classList.toggle('active', tab.dataset.tab === state.tab));
  if(!selected){
    inspectorBody.innerHTML = '<pre>{}</pre>';
    return;
  }
  if(state.tab === 'raw'){
    inspectorBody.innerHTML = '';
    const pre = document.createElement('pre');
    pre.textContent = json(selected.value);
    inspectorBody.appendChild(pre);
    return;
  }
  if(state.tab === 'text'){
    inspectorBody.innerHTML = '';
    const pre = document.createElement('pre');
    const value = selected.value || {};
    pre.textContent = value.content || value.Content || eventPreview(value) || displayValue(value);
    inspectorBody.appendChild(pre);
    return;
  }
  inspectorBody.innerHTML = '';
  const related = document.createElement('div');
  related.className = 'related-list';
  const raw = fullText(selected.value);
  const ids = Array.from(new Set((raw.match(/[a-zA-Z0-9_-]{8,}/g) || []).slice(0,24)));
  const matches = state.events.filter(ev => ev !== selected.value && ids.some(id => fullText(ev).includes(id))).slice(0,30);
  if(matches.length === 0){
    const pre = document.createElement('pre');
    pre.textContent = '[]';
    related.appendChild(pre);
  } else {
    matches.forEach(ev => {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'related-item';
      button.textContent = '#' + label(ev.sequence) + ' ' + eventDisplayTitle(ev);
      const small = document.createElement('small');
      small.textContent = eventDisplayMeta(ev);
      button.appendChild(small);
      button.onclick = () => selectItem('event', ev, eventDisplayTitle(ev), eventDisplayMeta(ev));
      related.appendChild(button);
    });
  }
  inspectorBody.appendChild(related);
}

function appendEnvelope(envelope){
  if(envelope.type === 'workflow_snapshot') state.workflowSnapshot = envelope.data || null;
  if(envelope.type === 'prompt_accepted') rememberPrompt((envelope.data || {}).Prompt || (envelope.data || {}).prompt || '');
  state.events.push(envelope);
  statusEl.textContent = envelope.type || 'event';
  if(envelope.type === 'run_idle') statusEl.textContent = 'Ready';
  renderMain();
  if(state.view === 'activity') streamEl.scrollTop = streamEl.scrollHeight;
}

function appendLocalError(text){
  appendEnvelope({
    sequence: 'local',
    received_at: new Date().toISOString(),
    type: 'ui_error',
    data_type: 'browser.error',
    data: {message: text}
  });
}

function rememberPrompt(text){
  if(!text) return;
  const history = state.promptHistory;
  if(history.length && history[history.length - 1] === text) return;
  history.push(text);
  if(history.length > 200) history.splice(0, history.length - 200);
}

function currentLineInfo(textarea){
  const before = textarea.value.slice(0, textarea.selectionStart || 0);
  const after = textarea.value.slice(textarea.selectionEnd || 0);
  return {
    atFirstLine: !before.includes('\n'),
    atLastLine: !after.includes('\n')
  };
}

function previousHistory(){
  if(!state.promptHistory.length) return false;
  if(state.historyIndex < 0){
    state.historyDraft = promptEl.value;
    state.historyIndex = state.promptHistory.length - 1;
  }else if(state.historyIndex > 0){
    state.historyIndex--;
  }
  promptEl.value = state.promptHistory[state.historyIndex] || '';
  movePromptCursorToEnd();
  clearCompletions();
  return true;
}

function nextHistory(){
  if(!state.promptHistory.length || state.historyIndex < 0) return false;
  state.historyIndex++;
  if(state.historyIndex >= state.promptHistory.length){
    state.historyIndex = -1;
    promptEl.value = state.historyDraft;
    state.historyDraft = '';
  }else{
    promptEl.value = state.promptHistory[state.historyIndex] || '';
  }
  movePromptCursorToEnd();
  clearCompletions();
  return true;
}

function resetHistoryNavigation(){
  state.historyIndex = -1;
  state.historyDraft = '';
}

function movePromptCursorToEnd(){
  const end = promptEl.value.length;
  promptEl.focus();
  promptEl.setSelectionRange(end, end);
}

function renderCompletions(){
  completionMenu.innerHTML = '';
  if(!state.completions.length){
    completionMenu.classList.remove('open');
    promptEl.removeAttribute('aria-activedescendant');
    return;
  }
  completionMenu.classList.add('open');
  state.completions.forEach((item, index) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.id = 'completion-' + index;
    button.className = 'completion-item' + (index === state.completionIndex ? ' active' : '');
    button.setAttribute('role', 'option');
    button.setAttribute('aria-selected', index === state.completionIndex ? 'true' : 'false');
    button.onmousedown = event => event.preventDefault();
    button.onclick = () => applyCompletion(index);
    const labelEl = document.createElement('div');
    labelEl.className = 'completion-label';
    labelEl.textContent = item.label || '';
    const detailEl = document.createElement('div');
    detailEl.className = 'completion-detail';
    detailEl.textContent = item.detail || '';
    button.append(labelEl, detailEl);
    completionMenu.appendChild(button);
  });
  promptEl.setAttribute('aria-activedescendant', 'completion-' + state.completionIndex);
}

function clearCompletions(){
  state.completions = [];
  state.completionIndex = 0;
  renderCompletions();
}

function cycleCompletion(delta){
  if(!state.completions.length) return false;
  state.completionIndex = (state.completionIndex + delta + state.completions.length) % state.completions.length;
  renderCompletions();
  return true;
}

function applyCompletion(index){
  const item = state.completions[index ?? state.completionIndex];
  if(!item) return false;
  promptEl.value = item.replacement || item.label || promptEl.value;
  movePromptCursorToEnd();
  resetHistoryNavigation();
  clearCompletions();
  updateCompletions();
  return true;
}

async function updateCompletions(){
  const value = promptEl.value;
  const request = ++state.completionRequest;
  if(!value.startsWith('/') || value.includes('\n')){
    clearCompletions();
    return;
  }
  try{
    const response = await fetch('/api/completions?q=' + encodeURIComponent(value));
    if(request !== state.completionRequest) return;
    if(!response.ok) throw new Error(await response.text());
    const items = await response.json();
    state.completions = Array.isArray(items) ? items : [];
    state.completionIndex = 0;
    renderCompletions();
  }catch(err){
    if(request === state.completionRequest) clearCompletions();
  }
}

function setRunning(running){
  state.running = running;
  state.cancelPending = false;
  sendButton.disabled = false;
  sendButton.textContent = running ? 'Cancel' : 'Send';
  sendButton.classList.toggle('cancel', running);
  sendButton.setAttribute('aria-label', running ? 'Cancel current turn' : 'Send message');
  statusDot.classList.toggle('running', running);
}

function showPermission(envelope){
  const req = envelope.data || {};
  modal.innerHTML = '';
  const panel = document.createElement('section');
  panel.className = 'panel';
  panel.innerHTML = '<div><div class="panel-title">Allow this action?</div><div class="panel-intro"></div></div><div class="panel-body"><details class="panel-details"><summary>Full request JSON</summary><pre></pre></details></div><div class="panel-actions"><label><input id="remember" type="checkbox"> remember for session</label><button class="danger" id="deny" type="button">Deny</button><button class="primary" id="allow" type="button">Allow</button></div>';
  panel.querySelector('.panel-intro').textContent = compactText([req.tool, req.content, req.reason].filter(Boolean).join(' · ') || 'Pragma needs permission to continue.');
  panel.querySelector('pre').textContent = json(req);
  panel.querySelector('#allow').onclick = () => replyPerm(req.id, 'allow');
  panel.querySelector('#deny').onclick = () => replyPerm(req.id, 'deny');
  modal.appendChild(panel);
}

async function replyPerm(id, decision){
  const remember = Boolean(document.getElementById('remember') && document.getElementById('remember').checked);
  const response = {decision, remember};
  appendEnvelope({sequence:'local', received_at:new Date().toISOString(), type:'permission_response_submitted', data_type:'browser.permissionResponse', data:response});
  await fetch('/api/permission/' + id, {method:'POST', headers:{'content-type':'application/json'}, body:JSON.stringify(response)});
  modal.innerHTML = '';
}

function showAsk(envelope){
  const req = envelope.data || {};
  const askRequest = req.request || req.Request || {};
  modal.innerHTML = '';
  const panel = document.createElement('section');
  panel.className = 'panel';
  panel.innerHTML = '<div><div class="panel-title">Pragma has a question</div><div class="panel-intro">Answer to continue.</div></div><div class="panel-body"><div class="question-list"></div><details class="panel-details"><summary>Full request JSON</summary><pre></pre></details></div><div class="panel-actions"><button class="primary" id="answerBtn" type="button">Answer</button></div>';
  panel.querySelector('pre').textContent = json(req);
  renderAskFields(panel.querySelector('.question-list'), askRequest);
  panel.querySelector('#answerBtn').onclick = () => replyAsk(req.id, askRequest);
  modal.appendChild(panel);
}

function askQuestions(askRequest){
  if(Array.isArray(askRequest.Questions) && askRequest.Questions.length) return askRequest.Questions;
  if(Array.isArray(askRequest.questions) && askRequest.questions.length) return askRequest.questions;
  const question = askRequest.Question || askRequest.question || 'answer';
  return [{Question: question}];
}

function questionText(question){
  return question.Question || question.question || 'answer';
}

function renderAskFields(container, askRequest){
  container.innerHTML = '';
  askQuestions(askRequest).forEach((question, index) => {
    const text = questionText(question);
    const options = question.Options || question.options || [];
    const multi = Boolean(question.MultiSelect || question.multiSelect);
    const field = document.createElement('div');
    field.className = 'question-field';
    const labelEl = document.createElement('div');
    labelEl.className = 'question-label';
    labelEl.textContent = text;
    field.appendChild(labelEl);
    if(Array.isArray(options) && options.length){
      options.forEach((option, optionIndex) => {
        const optionLabel = option.Label || option.label || '';
        const row = document.createElement('label');
        row.className = 'option-row';
        const input = document.createElement('input');
        input.type = multi ? 'checkbox' : 'radio';
        input.name = 'ask_' + index;
        input.value = optionLabel;
        if(!multi && optionIndex === 0) input.checked = true;
        const copy = document.createElement('span');
        copy.textContent = optionLabel + (option.Description || option.description ? ': ' + (option.Description || option.description) : '');
        row.append(input, copy);
        field.appendChild(row);
      });
    } else {
      const textarea = document.createElement('textarea');
      textarea.dataset.questionIndex = String(index);
      textarea.placeholder = text;
      field.appendChild(textarea);
    }
    container.appendChild(field);
  });
}

function collectAskPayload(askRequest){
  const answers = {};
  const answer_values = {};
  askQuestions(askRequest).forEach((question, index) => {
    const text = questionText(question);
    const options = question.Options || question.options || [];
    if(Array.isArray(options) && options.length){
      const selected = Array.from(document.querySelectorAll('input[name="ask_' + index + '"]:checked')).map(input => input.value);
      answer_values[text] = selected;
      answers[text] = selected.join(', ');
      return;
    }
    const textarea = document.querySelector('textarea[data-question-index="' + index + '"]');
    const value = textarea ? textarea.value : '';
    answer_values[text] = value;
    answers[text] = value;
  });
  return {answers, answer_values};
}

async function replyAsk(id, askRequest){
  const response = collectAskPayload(askRequest);
  appendEnvelope({sequence:'local', received_at:new Date().toISOString(), type:'ask_response_submitted', data_type:'browser.askResponse', data:response});
  await fetch('/api/ask/' + id, {method:'POST', headers:{'content-type':'application/json'}, body:JSON.stringify(response)});
  modal.innerHTML = '';
}

document.getElementById('inspectorClose').onclick = () => document.body.classList.remove('inspector-open');
document.getElementById('runsToggle').onclick = () => document.body.classList.toggle('runs-open');
document.querySelectorAll('.tab').forEach(tab => {
  tab.onclick = () => { state.tab = tab.dataset.tab; renderInspector(); };
});
document.getElementById('search').addEventListener('input', event => {
  state.search = event.target.value.trim().toLowerCase();
  renderMain();
});
sendButton.onclick = async () => {
  if(!state.running){
    document.getElementById('promptForm').requestSubmit();
    return;
  }
  if(state.cancelPending) return;
  state.cancelPending = true;
  sendButton.disabled = true;
  try{
    const response = await fetch('/api/cancel', {method:'POST'});
    if(response.ok) return;
    appendLocalError(await response.text());
  }catch(err){
    appendLocalError(err.message);
  }finally{
    state.cancelPending = false;
    sendButton.disabled = false;
  }
};
document.getElementById('promptForm').onsubmit = async event => {
  event.preventDefault();
  if(state.running) return;
  const prompt = promptEl.value;
  if(!prompt.trim()) return;
  resetHistoryNavigation();
  clearCompletions();
  promptEl.value = '';
  setRunning(true);
  const requestBody = {prompt};
  appendEnvelope({sequence:'local', received_at:new Date().toISOString(), type:'prompt_submitted', data_type:'browser.promptRequest', data:requestBody});
  try{
    const response = await fetch('/api/prompt', {
      method:'POST',
      headers:{'content-type':'application/json'},
      body:JSON.stringify(requestBody)
    });
    if(!response.ok){
      setRunning(false);
      appendLocalError(await response.text());
    }
  }catch(err){
    setRunning(false);
    appendLocalError(err.message);
  }
};
promptEl.addEventListener('keydown', event => {
  if(event.key === 'ArrowUp'){
    if(cycleCompletion(-1)){
      event.preventDefault();
      return;
    }
    const line = currentLineInfo(promptEl);
    if(line.atFirstLine && previousHistory()){
      event.preventDefault();
      return;
    }
  }
  if(event.key === 'ArrowDown'){
    if(cycleCompletion(1)){
      event.preventDefault();
      return;
    }
    const line = currentLineInfo(promptEl);
    if(line.atLastLine && nextHistory()){
      event.preventDefault();
      return;
    }
  }
  if(event.key === 'Tab' && state.completions.length){
    event.preventDefault();
    applyCompletion();
    return;
  }
  if(event.key === 'Escape' && state.completions.length){
    event.preventDefault();
    clearCompletions();
    return;
  }
  if(event.key === 'Enter' && !event.shiftKey){
    event.preventDefault();
    if(state.completions.length && applyCompletion()) return;
    document.getElementById('promptForm').requestSubmit();
  }
});
promptEl.addEventListener('input', () => {
  resetHistoryNavigation();
  updateCompletions();
});
promptEl.addEventListener('blur', () => {
  setTimeout(clearCompletions, 120);
});

renderFilters();
renderViewTabs();
renderMain();
refreshState().catch(err => appendLocalError(err.message));
fetch('/api/sessions').then(r => r.json()).then(renderSessions).catch(err => appendLocalError(err.message));
new EventSource('/api/events').onmessage = event => {
  const envelope = JSON.parse(event.data);
  appendEnvelope(envelope);
  if(envelope.type === 'permission_request') showPermission(envelope);
  if(envelope.type === 'ask_request') showAsk(envelope);
  if(envelope.type === 'run_idle') setRunning(false);
  if(envelope.type === 'session_resumed') refreshState().catch(err => appendLocalError(err.message));
};
</script>
</body>
</html>`
