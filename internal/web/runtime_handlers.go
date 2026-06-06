package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/interactive"
)

func (s *server) handleState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	snap := s.cfg.Store.Snapshot()
	runtime := s.runtimeState(snap)
	writeJSONAPIResource(w, r, "runtime-states", "active", map[string]interface{}{
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

func (s *server) handlePrompt(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	raw, err := decodeJSONAPIAttributes(r, "prompt-submissions")
	if err != nil {
		writeJSONAPIRequestError(w, err)
		return
	}
	var prompt string
	if err := json.Unmarshal(raw["prompt"], &prompt); err != nil {
		writeJSONAPIError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if strings.TrimSpace(prompt) == "" {
		writeJSONAPIError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if s.start(prompt) {
		writeJSONAPIOK(w, r, "prompt-submissions", newID(), map[string]interface{}{"accepted": true})
		return
	}
	writeJSONAPIError(w, http.StatusConflict, "interactive run already in progress")
}

func (s *server) handleCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	if _, err := decodeJSONAPIAttributes(r, "run-cancellations"); err != nil {
		writeJSONAPIRequestError(w, err)
		return
	}
	if !s.cancelRun() {
		writeJSONAPIError(w, http.StatusConflict, "interactive run is not in progress")
		return
	}
	writeJSONAPIOK(w, r, "run-cancellations", newID(), map[string]interface{}{"accepted": true})
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

func (s *server) start(input string) bool {
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
