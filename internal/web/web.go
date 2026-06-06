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
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
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
	SessionStore   *session.Store
	SessionStart   time.Time
	McpServerNames []string
}

// Run starts the web API server and blocks until the server shuts down.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Bridge == nil {
		return errors.New("web bridge is required")
	}
	srv := newServer(cfg)
	cfg.Bridge.attach(srv.hub)

	mux := http.NewServeMux()
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
	fmt.Printf("Pragma web API: %s\n", url)

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
	cfg     Config
	hub     *hub
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
}

var errRuntimeBusy = errors.New("interactive run already in progress")

func newServer(cfg Config) *server {
	return &server{
		cfg: cfg,
		hub: newHub(),
	}
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

func normalizeWebEvent(kind string, data interface{}) (string, string, interface{}) {
	switch kind {
	case "prompt_accepted":
		if ev, ok := data.(interactive.AcceptedPromptEvent); ok {
			return "prompt_accepted", dataType(ev), ev
		}
	case "loop_event":
		return normalizeLoopEvent(data)
	}
	return kind, dataType(data), data
}

func normalizeLoopEvent(data interface{}) (string, string, interface{}) {
	switch ev := data.(type) {
	case query.TextEvent:
		return "text", dataType(ev), ev
	case query.ThinkingEvent:
		return "thinking", dataType(ev), ev
	case query.ModelRequestEvent:
		return "model_request", dataType(ev), ev
	case query.ModelResponseEvent:
		return "model_response", dataType(ev), ev
	case query.ToolCallEvent:
		return "tool_call", dataType(ev), ev
	case query.ToolResultEvent:
		return "tool_result", dataType(ev), ev
	case query.StructuredOutputEvent:
		return "structured_output", dataType(ev), ev
	case query.UserMessageEvent:
		return "user_message", dataType(ev), ev
	case query.TurnCompleteEvent:
		return "turn_complete", dataType(ev), ev
	case query.CompactionStartedEvent:
		return "compaction_started", dataType(ev), ev
	case query.CompactionEvent:
		return "compaction", dataType(ev), ev
	case query.CompactionFailedEvent:
		return "compaction_failed", dataType(ev), ev
	case query.CompactionDisabledEvent:
		return "compaction_disabled", dataType(ev), ev
	case query.LifecycleProgressEvent:
		return "lifecycle_progress", dataType(ev), ev
	case query.OrchestrationStartedEvent:
		return "orchestration_started", dataType(ev), ev
	case query.OrchestrationStateStartedEvent:
		return "orchestration_state_started", dataType(ev), ev
	case query.OrchestrationStateCompletedEvent:
		return "orchestration_state_completed", dataType(ev), ev
	case query.OrchestrationControlEvent:
		return "orchestration_control", dataType(ev), ev
	case query.OrchestrationTransitionEvent:
		return "orchestration_transition", dataType(ev), ev
	case query.OrchestrationHandoffEvent:
		return "orchestration_handoff", dataType(ev), ev
	case query.OrchestrationCompletedEvent:
		return "orchestration_completed", dataType(ev), ev
	case query.OrchestrationSnapshotEvent:
		return "workflow_snapshot", dataType(ev), ev
	case query.AgentProgressEvent:
		return "agent_progress", dataType(ev), ev
	case query.RetryEvent:
		return "retry", dataType(ev), ev
	case query.ErrorEvent:
		return "run_error", dataType(ev), ev
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

// Bridge implements permission.Prompter and tool.Asker for web API clients.
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
	Scope    permission.RememberScope
}

func (b *Bridge) Prompt(ctx context.Context, toolName string, toolInput json.RawMessage, content string, reason string) (permission.Decision, permission.RememberScope) {
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
		return resp.Decision, resp.Scope
	case <-ctx.Done():
		b.dropPermission(id)
		return permission.DecisionDeny, permission.RememberNone
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

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	snap := s.cfg.Store.Snapshot()
	runtime := s.runtimeState(snap)
	writeJSON(w, map[string]interface{}{
		"runtime":        runtime,
		"app_state":      snap,
		"prompt_history": snap.PromptHistory,
	})
}

func (s *server) runtimeState(snap app.AppState) map[string]interface{} {
	modelName := firstNonEmptyString(snap.Model, snap.Conversation.Model, s.cfg.ModelName)
	providerName := firstNonEmptyString(snap.Provider, snap.Conversation.Provider, s.cfg.Provider)
	workspace := firstNonEmptyString(snap.CWD, snap.Conversation.WorkDir, s.cfg.Workspace)
	sessionStart := snap.Conversation.CreatedAt
	if sessionStart.IsZero() {
		sessionStart = s.cfg.SessionStart
	}
	return map[string]interface{}{
		"version":       s.cfg.Version,
		"workspace":     workspace,
		"model":         modelName,
		"provider":      providerName,
		"mcp_servers":   s.cfg.McpServerNames,
		"session_start": sessionStart,
		"running":       s.isRunning(),
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
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
	ctx, cancel := context.WithCancel(s.cfg.ParentCtx)
	endTransition, err := s.beginRuntimeTransition(cancel)
	if err != nil {
		cancel()
		return false
	}

	go func() {
		defer endTransition()
		events := s.cfg.RunInput(ctx, input)
		for ev := range events {
			switch e := ev.(type) {
			case interactive.AcceptedPromptEvent:
				s.hub.publish("prompt_accepted", e)
				continue
			case interactive.SlashResultEvent:
				s.hub.publish("slash_result", e.Result)
				continue
			case interactive.RuntimeTerminatedEvent:
				s.hub.publish("runtime_terminated", e)
				continue
			case interactive.LoopEvent:
				s.hub.publish("loop_event", e.Event)
			default:
				s.hub.publish("interactive_event", e)
			}
		}
	}()
	return true
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
	if !s.sessionOwnsArtifact(path) {
		http.Error(w, "artifact path is not recorded in this session", http.StatusForbidden)
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

func (s *server) sessionOwnsArtifact(path string) bool {
	if s.cfg.Store == nil {
		return false
	}
	for _, artifact := range s.cfg.Store.Snapshot().OrchestrationArtifacts {
		if filepath.Clean(artifact.Path) == path {
			return true
		}
	}
	return false
}

type completionItem struct {
	Label       string `json:"label"`
	Detail      string `json:"detail,omitempty"`
	Replacement string `json:"replacement"`
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
	var commands []slash.Command
	if s.cfg.SlashCmds == nil {
		commands = nil
	} else {
		commands = s.cfg.SlashCmds.CommandsWithDeps(s.cfg.SlashDeps)
	}
	workspace := s.cfg.Workspace
	if s.cfg.Store != nil {
		if cwd := s.cfg.Store.Snapshot().CWD; cwd != "" {
			workspace = cwd
		}
	}
	items := slash.CompletionItems(value, commands, workspace)
	out := make([]completionItem, 0, len(items))
	for _, item := range items {
		out = append(out, completionItem{
			Label:       item.Label,
			Detail:      item.Detail,
			Replacement: item.Replacement,
		})
	}
	return out
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
	scope := permission.RememberNone
	if rawScope, ok := raw["scope"]; ok {
		var scopeValue string
		if err := json.Unmarshal(rawScope, &scopeValue); err != nil {
			http.Error(w, "scope must be a string", http.StatusBadRequest)
			return
		}
		scope = permission.RememberScope(scopeValue)
		if scope != permission.RememberNone && scope != permission.RememberSession && scope != permission.RememberPersistent {
			http.Error(w, "scope must be none, session, or persistent", http.StatusBadRequest)
			return
		}
	}
	if rawRemember, ok := raw["remember"]; ok {
		var remember bool
		_ = json.Unmarshal(rawRemember, &remember)
		if remember {
			scope = permission.RememberSession
		}
	}
	decision := permission.Decision(decisionValue)
	if decision != permission.DecisionAllow && decision != permission.DecisionDeny {
		http.Error(w, "decision must be allow or deny", http.StatusBadRequest)
		return
	}
	submitted := map[string]interface{}{"id": id, "body": raw}
	if !s.cfg.Bridge.resolvePermission(id, permissionResponse{Decision: decision, Scope: scope}) {
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
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	store := s.cfg.SessionStore
	if store == nil {
		var err error
		store, err = session.NewStore()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	summaries, err := store.List()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if summaries == nil {
		writeJSON(w, []interface{}{})
		return
	}
	rows, err := webSessionSummaries(summaries, s.cfg.Workspace)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, rows)
}

func webSessionSummaries(summaries []session.SessionSummary, workspace string) ([]map[string]interface{}, error) {
	out := make([]map[string]interface{}, 0, len(summaries))
	cwd := filepath.Clean(strings.TrimSpace(workspace))
	for _, summary := range summaries {
		var row map[string]interface{}
		data, err := json.Marshal(summary)
		if err != nil {
			return nil, fmt.Errorf("marshal session summary %q: %w", summary.ID, err)
		}
		if err := json.Unmarshal(data, &row); err != nil {
			return nil, fmt.Errorf("project session summary %q: %w", summary.ID, err)
		}
		if cwd != "" && filepath.Clean(summary.WorkDir) == cwd {
			row["in_current_work_dir"] = true
		} else {
			row["in_current_work_dir"] = false
		}
		out = append(out, row)
	}
	return out, nil
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
	if err := s.resume(sessionID, raw); err != nil {
		if errors.Is(err, errRuntimeBusy) {
			http.Error(w, err.Error(), http.StatusConflict)
			return
		}
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, map[string]bool{"ok": true})
}

func (s *server) resume(sessionID string, raw map[string]json.RawMessage) error {
	if s.cfg.Resume == nil {
		return errors.New("session resume is not available")
	}
	endTransition, err := s.beginRuntimeTransition(nil)
	if err != nil {
		return err
	}
	defer endTransition()

	s.hub.publish("resume_requested", raw)
	if err := s.cfg.Resume(sessionID); err != nil {
		return err
	}
	store := s.cfg.SessionStore
	if store == nil {
		store, err = session.NewStore()
		if err != nil {
			return err
		}
	}
	sess, err := store.Load(sessionID)
	if err != nil {
		return err
	}
	s.hub.publish("session_resumed", sess)
	return nil
}

func (s *server) beginRuntimeTransition(cancel context.CancelFunc) (func(), error) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil, errRuntimeBusy
	}
	s.running = true
	s.cancel = cancel
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		s.running = false
		s.cancel = nil
		s.mu.Unlock()
		s.hub.publish("run_idle", nil)
	}, nil
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
