package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/session"
)

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
	recorder    func(session.WebEventData) error
}

func newHub(recorders ...func(session.WebEventData) error) *hub {
	var recorder func(session.WebEventData) error
	if len(recorders) > 0 {
		recorder = recorders[0]
	}
	return &hub{subscribers: make(map[chan eventEnvelope]struct{}), recorder: recorder}
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
	if h.recorder == nil {
		return
	}
	_ = h.recorder(session.WebEventData{
		Sequence:   ev.Sequence,
		ReceivedAt: ev.Received,
		Type:       ev.Type,
		DataType:   ev.DataType,
		Data:       eventDataRawMessage(ev.Data),
	})
}

func (h *hub) seed(events []eventEnvelope) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recent = append(h.recent, events...)
	if len(h.recent) > 200 {
		h.recent = h.recent[len(h.recent)-200:]
	}
	for _, ev := range h.recent {
		if ev.Sequence > h.sequence {
			h.sequence = ev.Sequence
		}
	}
}

func eventDataRawMessage(data interface{}) json.RawMessage {
	raw, err := json.Marshal(completeJSONValue(data))
	if err != nil {
		return json.RawMessage(`null`)
	}
	return raw
}

func (s *server) seedSessionEvents() {
	if s.cfg.SessionStore == nil || s.cfg.Store == nil {
		return
	}
	sessionID := s.cfg.Store.Snapshot().SessionID()
	if sessionID == "" {
		return
	}
	sess, err := s.cfg.SessionStore.Load(sessionID)
	if err != nil {
		return
	}
	s.hub.seed(sessionWebEvents(sess.WebEvents))
}

func sessionWebEvents(events []session.WebEventData) []eventEnvelope {
	out := make([]eventEnvelope, 0, len(events))
	for _, event := range events {
		if event.Type == "" {
			continue
		}
		out = append(out, eventEnvelope{
			Sequence: event.Sequence,
			Received: event.ReceivedAt,
			Type:     event.Type,
			DataType: event.DataType,
			Data:     event.Data,
		})
	}
	return out
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

func (s *server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
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

func (h *hub) recentEvents() []eventEnvelope {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]eventEnvelope(nil), h.recent...)
}

func (s *server) handleRecentEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !acceptsJSONAPI(w, r) {
		return
	}
	page, err := pageRequestFromQuery(r)
	if err != nil {
		writeJSONAPIRequestError(w, err)
		return
	}
	events, pageMeta := paginateSlice(s.hub.recentEvents(), page)
	resources := make([]jsonAPIResource, 0, len(events))
	for _, ev := range events {
		resources = append(resources, jsonAPIResource{
			Type:       "events",
			ID:         fmt.Sprintf("%d", ev.Sequence),
			Attributes: map[string]interface{}{"envelope": ev},
		})
	}
	writeJSONAPICollection(w, r, resources, pageMeta)
}

func writeSSE(w http.ResponseWriter, ev eventEnvelope) {
	data, _ := json.Marshal(jsonAPIResourceDocument("/api/events", "events", fmt.Sprintf("%d", ev.Sequence), map[string]interface{}{
		"envelope": ev,
	}))
	_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
}
