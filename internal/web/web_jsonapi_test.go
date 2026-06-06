package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/tool"
)

func TestHandleStateReturnsJSONAPIResource(t *testing.T) {
	srv := newServer(Config{
		Bridge:    NewBridge(),
		Store:     app.NewStateStore(app.AppState{CWD: "/repo", Model: "glm", Provider: "lilac"}),
		Workspace: "/repo",
		ModelName: "glm",
		Provider:  "lilac",
	})
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	rec := httptest.NewRecorder()

	srv.handleState(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != jsonAPIMediaType {
		t.Fatalf("content type: got %q, want %q", ct, jsonAPIMediaType)
	}
	var doc jsonAPIDocument
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode JSON:API document: %v", err)
	}
	resource, ok := doc.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("data: got %T, want resource object", doc.Data)
	}
	if resource["type"] != "runtime-states" || resource["id"] != "active" {
		t.Fatalf("resource identity: got type=%v id=%v", resource["type"], resource["id"])
	}
	attrs, ok := resource["attributes"].(map[string]interface{})
	if !ok {
		t.Fatalf("attributes: got %T", resource["attributes"])
	}
	if _, ok := attrs["app_state"]; !ok {
		t.Fatal("app_state missing from attributes")
	}
}

func TestHandleCompletionsReturnsJSONAPICollectionPagination(t *testing.T) {
	srv := newServer(Config{Bridge: NewBridge()})
	req := httptest.NewRequest(http.MethodGet, "/api/completions?page[number]=2&page[size]=1", nil)
	req.Header.Set("Accept", jsonAPIMediaType)
	rec := httptest.NewRecorder()

	srv.handleCompletions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct != jsonAPIMediaType {
		t.Fatalf("content type: got %q, want %q", ct, jsonAPIMediaType)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("decode JSON:API document: %v", err)
	}
	if _, ok := doc["data"].([]interface{}); !ok {
		t.Fatalf("data: got %T, want collection", doc["data"])
	}
	links, ok := doc["links"].(map[string]interface{})
	if !ok {
		t.Fatalf("links: got %T", doc["links"])
	}
	for _, key := range []string{"self", "first", "last"} {
		if _, ok := links[key].(string); !ok {
			t.Fatalf("missing pagination link %q in %v", key, links)
		}
	}
	meta, ok := doc["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("meta: got %T", doc["meta"])
	}
	if _, ok := meta["page"].(map[string]interface{}); !ok {
		t.Fatalf("meta.page missing: %v", meta)
	}
}

func TestHandleCompletionsRejectsInvalidPaginationAsJSONAPIError(t *testing.T) {
	srv := newServer(Config{Bridge: NewBridge()})
	req := httptest.NewRequest(http.MethodGet, "/api/completions?page[number]=zero&page[size]=1", nil)
	req.Header.Set("Accept", jsonAPIMediaType)
	rec := httptest.NewRecorder()

	srv.handleCompletions(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusBadRequest)
	}
	doc := decodeJSONAPIMap(t, rec.Body.Bytes())
	errors := doc["errors"].([]interface{})
	errObj := errors[0].(map[string]interface{})
	source := errObj["source"].(map[string]interface{})
	if source["parameter"] != "page[number]" {
		t.Fatalf("source.parameter: got %v", source["parameter"])
	}
}

func TestAPIRejectsUnacceptableAcceptHeader(t *testing.T) {
	srv := newServer(Config{
		Bridge: NewBridge(),
		Store:  app.NewStateStore(app.AppState{}),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()

	srv.handleState(rec, req)

	if rec.Code != http.StatusNotAcceptable {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusNotAcceptable)
	}
	if ct := rec.Header().Get("Content-Type"); ct != jsonAPIMediaType {
		t.Fatalf("content type: got %q, want %q", ct, jsonAPIMediaType)
	}
	assertJSONAPIError(t, rec.Body.Bytes(), "Accept must allow application/vnd.api+json")
}

func TestHandlePromptRejectsNonJSONAPIMediaType(t *testing.T) {
	srv := newServer(Config{Bridge: NewBridge()})
	req := httptest.NewRequest(http.MethodPost, "/api/prompt", strings.NewReader(`{"prompt":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handlePrompt(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
	assertJSONAPIError(t, rec.Body.Bytes(), "Content-Type must be application/vnd.api+json")
}

func TestHandlePromptRejectsUnsupportedJSONAPIMediaParameters(t *testing.T) {
	srv := newServer(Config{Bridge: NewBridge()})
	req := httptest.NewRequest(http.MethodPost, "/api/prompt", strings.NewReader(`{"data":{"type":"prompt-submissions","attributes":{"prompt":"hello"}}}`))
	req.Header.Set("Content-Type", "application/vnd.api+json; charset=utf-8")
	rec := httptest.NewRecorder()

	srv.handlePrompt(rec, req)

	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusUnsupportedMediaType)
	}
	assertJSONAPIError(t, rec.Body.Bytes(), `Content-Type must not include unsupported JSON:API media type parameter "charset"`)
}

func TestHandlePromptAcceptsJSONAPIResourceRequest(t *testing.T) {
	srv := newServer(Config{
		Bridge:    NewBridge(),
		ParentCtx: context.Background(),
		RunInput: func(context.Context, string) <-chan interactive.Event {
			ch := make(chan interactive.Event, 1)
			ch <- interactive.AcceptedPromptEvent{Prompt: "hello"}
			close(ch)
			return ch
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/prompt", strings.NewReader(`{"data":{"type":"prompt-submissions","attributes":{"prompt":"hello"}}}`))
	req.Header.Set("Accept", jsonAPIMediaType)
	req.Header.Set("Content-Type", jsonAPIMediaType)
	rec := httptest.NewRecorder()

	srv.handlePrompt(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	doc := decodeJSONAPIMap(t, rec.Body.Bytes())
	data := doc["data"].(map[string]interface{})
	if data["type"] != "prompt-submissions" {
		t.Fatalf("type: got %v", data["type"])
	}
	attrs := data["attributes"].(map[string]interface{})
	if attrs["accepted"] != true {
		t.Fatalf("accepted: got %v", attrs["accepted"])
	}
}

func TestHandleSessionsReturnsJSONAPICollectionPagination(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewStore()
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	for _, id := range []string{"session-1", "session-2", "session-3"} {
		w, err := store.Create(session.HeaderData{
			SessionID: id,
			Model:     "glm",
			Provider:  "lilac",
			WorkDir:   "/repo",
			CreatedAt: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("create session %s: %v", id, err)
		}
		if err := w.WriteMetadata(session.MetadataData{
			TurnCount: 2,
			CostUSD:   0.01,
			UpdatedAt: time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC),
		}); err != nil {
			t.Fatalf("write metadata: %v", err)
		}
		if err := w.Close(); err != nil {
			t.Fatalf("close session writer: %v", err)
		}
	}
	srv := newServer(Config{Bridge: NewBridge(), SessionStore: store, Workspace: "/repo"})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions?page[number]=2&page[size]=2", nil)
	rec := httptest.NewRecorder()

	srv.handleSessions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d", rec.Code, http.StatusOK)
	}
	doc := decodeJSONAPIMap(t, rec.Body.Bytes())
	data, ok := doc["data"].([]interface{})
	if !ok {
		t.Fatalf("data: got %T, want collection", doc["data"])
	}
	if len(data) != 1 {
		t.Fatalf("page length: got %d, want 1", len(data))
	}
	resource := data[0].(map[string]interface{})
	if resource["type"] != "sessions" {
		t.Fatalf("resource type: got %v", resource["type"])
	}
	attrs := resource["attributes"].(map[string]interface{})
	if _, ok := attrs["id"]; ok {
		t.Fatalf("attributes must not repeat JSON:API resource id: %v", attrs)
	}
	if attrs["in_current_work_dir"] != true {
		t.Fatalf("in_current_work_dir: got %v", attrs["in_current_work_dir"])
	}
	meta := doc["meta"].(map[string]interface{})["page"].(map[string]interface{})
	if meta["number"] != float64(2) || meta["size"] != float64(2) || meta["total"] != float64(3) || meta["pages"] != float64(2) {
		t.Fatalf("page meta: got %v", meta)
	}
	links := doc["links"].(map[string]interface{})
	if _, ok := links["prev"].(string); !ok {
		t.Fatalf("prev link missing: %v", links)
	}
}

func TestHandleSessionReturnsFullLoadedSessionPayload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewStore()
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	createdAt := time.Date(2026, 6, 6, 2, 3, 4, 0, time.UTC)
	w, err := store.Create(session.HeaderData{
		SessionID: "inspect-full",
		Model:     "glm",
		Provider:  "lilac",
		WorkDir:   "/repo",
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := w.WriteMessage(model.Message{
		ID:        "inspect-message",
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: "inspect without resume"}},
		Timestamp: createdAt,
	}); err != nil {
		t.Fatalf("write message: %v", err)
	}
	handoff := model.NewHandoffState("inspect goal")
	handoff.CurrentFocus = "load full session"
	if err := w.WriteHandoffState(handoff); err != nil {
		t.Fatalf("write handoff state: %v", err)
	}
	if err := w.WriteMetadata(session.MetadataData{
		Summary:   "inspection summary",
		CostUSD:   0.5,
		TurnCount: 1,
		UpdatedAt: createdAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	srv := newServer(Config{Bridge: NewBridge(), SessionStore: store})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/inspect-full", nil)
	req.Header.Set("Accept", jsonAPIMediaType)
	rec := httptest.NewRecorder()

	srv.handleSession(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	doc := decodeJSONAPIMap(t, rec.Body.Bytes())
	data := doc["data"].(map[string]interface{})
	if data["type"] != "sessions" || data["id"] != "inspect-full" {
		t.Fatalf("resource identity: got %v", data)
	}
	attrs := data["attributes"].(map[string]interface{})
	conversation := attrs["conversation"].(map[string]interface{})
	if conversation["id"] != "inspect-full" {
		t.Fatalf("conversation id: got %v", conversation["id"])
	}
	messages := conversation["messages"].([]interface{})
	if messages[0].(map[string]interface{})["id"] != "inspect-message" {
		t.Fatalf("message payload lost: %v", messages)
	}
	handoffState := attrs["handoff_state"].(map[string]interface{})
	if handoffState["current_focus"] != "load full session" {
		t.Fatalf("handoff payload lost: %v", handoffState)
	}
}

func TestHandleRecentEventsReturnsJSONAPICollectionPagination(t *testing.T) {
	srv := newServer(Config{Bridge: NewBridge()})
	srv.hub.publish("text", map[string]string{"text": "one"})
	srv.hub.publish("text", map[string]string{"text": "two"})
	srv.hub.publish("run_error", map[string]string{"message": "three"})
	req := httptest.NewRequest(http.MethodGet, "/api/events/recent?page[number]=2&page[size]=2", nil)
	req.Header.Set("Accept", jsonAPIMediaType)
	rec := httptest.NewRecorder()

	srv.handleRecentEvents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	doc := decodeJSONAPIMap(t, rec.Body.Bytes())
	data, ok := doc["data"].([]interface{})
	if !ok {
		t.Fatalf("data: got %T, want collection", doc["data"])
	}
	if len(data) != 1 {
		t.Fatalf("page length: got %d, want 1", len(data))
	}
	resource := data[0].(map[string]interface{})
	if resource["type"] != "events" || resource["id"] != "3" {
		t.Fatalf("event resource identity: got %v", resource)
	}
	attrs := resource["attributes"].(map[string]interface{})
	envelope := attrs["envelope"].(map[string]interface{})
	if envelope["type"] != "run_error" {
		t.Fatalf("event envelope: got %v", envelope)
	}
	meta := doc["meta"].(map[string]interface{})["page"].(map[string]interface{})
	if meta["number"] != float64(2) || meta["size"] != float64(2) || meta["total"] != float64(3) || meta["pages"] != float64(2) {
		t.Fatalf("page meta: got %v", meta)
	}
	links := doc["links"].(map[string]interface{})
	if _, ok := links["prev"].(string); !ok {
		t.Fatalf("prev link missing: %v", links)
	}
}

func TestNewServerSeedsRecentEventsFromLoadedSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewStore()
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	received := time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC)
	w, err := store.Create(session.HeaderData{
		SessionID: "seed-events",
		Model:     "glm",
		Provider:  "lilac",
		WorkDir:   "/repo",
		CreatedAt: received,
		System:    model.SystemPrompt{},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := w.WriteWebEvent(session.WebEventData{
		Sequence:   9,
		ReceivedAt: received,
		Type:       "tool_result",
		DataType:   "query.ToolResultEvent",
		Data:       json.RawMessage(`{"tool_call_id":"call-1","unknown":"visible"}`),
	}); err != nil {
		t.Fatalf("write web event: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	srv := newServer(Config{
		Bridge:       NewBridge(),
		Store:        app.NewStateStore(app.AppState{Conversation: model.Conversation{ID: "seed-events"}}),
		SessionStore: store,
	})
	req := httptest.NewRequest(http.MethodGet, "/api/events/recent?page[number]=1&page[size]=10", nil)
	req.Header.Set("Accept", jsonAPIMediaType)
	rec := httptest.NewRecorder()

	srv.handleRecentEvents(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	doc := decodeJSONAPIMap(t, rec.Body.Bytes())
	data := doc["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("event count: got %d, want 1", len(data))
	}
	resource := data[0].(map[string]interface{})
	if resource["id"] != "9" {
		t.Fatalf("resource id: got %v", resource["id"])
	}
	attrs := resource["attributes"].(map[string]interface{})
	envelope := attrs["envelope"].(map[string]interface{})
	if envelope["type"] != "tool_result" || envelope["data_type"] != "query.ToolResultEvent" {
		t.Fatalf("seeded envelope identity lost: %v", envelope)
	}
	payload := envelope["data"].(map[string]interface{})
	if payload["unknown"] != "visible" {
		t.Fatalf("seeded payload lost unknown field: %v", payload)
	}
}

func TestHubRecordsPublishedEventsThroughSessionOwner(t *testing.T) {
	var recorded []session.WebEventData
	hub := newHub(func(event session.WebEventData) error {
		recorded = append(recorded, event)
		return nil
	})

	hub.publish("text", map[string]string{"text": "durable"})

	if len(recorded) != 1 {
		t.Fatalf("recorded count: got %d, want 1", len(recorded))
	}
	if recorded[0].Sequence != 1 || recorded[0].Type != "text" {
		t.Fatalf("recorded identity: %#v", recorded[0])
	}
	if string(recorded[0].Data) != `{"text":"durable"}` {
		t.Fatalf("recorded data: %s", recorded[0].Data)
	}
}

func TestWriteSSEUsesJSONAPIResourceDocument(t *testing.T) {
	rec := httptest.NewRecorder()
	writeSSE(rec, eventEnvelope{
		Sequence: 7,
		Received: time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC),
		Type:     "text",
		DataType: "map[string]string",
		Data:     map[string]string{"text": "hello"},
	})

	line := strings.TrimSpace(rec.Body.String())
	if !strings.HasPrefix(line, "data: ") {
		t.Fatalf("SSE line: got %q", line)
	}
	doc := decodeJSONAPIMap(t, []byte(strings.TrimPrefix(line, "data: ")))
	data := doc["data"].(map[string]interface{})
	if data["type"] != "events" || data["id"] != "7" {
		t.Fatalf("event resource identity: got %v", data)
	}
	attrs := data["attributes"].(map[string]interface{})
	envelope := attrs["envelope"].(map[string]interface{})
	if envelope["type"] != "text" {
		t.Fatalf("event envelope: got %v", envelope)
	}
	links := doc["links"].(map[string]interface{})
	if links["self"] != "/api/events" {
		t.Fatalf("self link: got %v", links)
	}
}

func TestHandleResumePublishesFullLoadedSessionPayload(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store, err := session.NewStore()
	if err != nil {
		t.Fatalf("new session store: %v", err)
	}
	createdAt := time.Date(2026, 6, 6, 1, 2, 3, 0, time.UTC)
	w, err := store.Create(session.HeaderData{
		SessionID: "resume-full",
		Model:     "glm",
		Provider:  "lilac",
		WorkDir:   "/repo",
		CreatedAt: createdAt,
		System:    model.SystemPrompt{Blocks: []model.SystemBlock{{Text: "system"}}},
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := w.WriteMessage(model.Message{
		ID:        "msg-resume-1",
		Role:      model.RoleUser,
		Content:   []model.ContentPart{model.TextPart{Text: "resume me"}},
		Timestamp: createdAt,
	}); err != nil {
		t.Fatalf("write message: %v", err)
	}
	handoff := model.NewHandoffState("resume goal")
	handoff.CurrentFocus = "verify full payload"
	if err := w.WriteHandoffState(handoff); err != nil {
		t.Fatalf("write handoff state: %v", err)
	}
	if err := w.WriteTodos([]app.TodoItem{{Content: "prove resume payload", Status: "completed"}}); err != nil {
		t.Fatalf("write todos: %v", err)
	}
	artifact := app.OrchestrationArtifact{Path: filepath.Join(t.TempDir(), "handoff.md"), StateID: "review", Event: "done", Direction: "write", CreatedAt: createdAt}
	if err := w.WriteOrchestrationArtifacts([]app.OrchestrationArtifact{artifact}); err != nil {
		t.Fatalf("write orchestration artifacts: %v", err)
	}
	if err := w.WriteMetadata(session.MetadataData{
		Summary:   "loaded summary",
		CostUSD:   1.25,
		TurnCount: 3,
		UpdatedAt: createdAt.Add(time.Minute),
	}); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	resumeCalled := false
	srv := newServer(Config{
		Bridge:       NewBridge(),
		SessionStore: store,
		Resume: func(sessionID string) error {
			resumeCalled = sessionID == "resume-full"
			return nil
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/resume", strings.NewReader(`{"data":{"type":"session-resumes","id":"resume-full","attributes":{"session_id":"resume-full"}}}`))
	req.Header.Set("Content-Type", jsonAPIMediaType)
	rec := httptest.NewRecorder()

	srv.handleResume(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	if !resumeCalled {
		t.Fatal("Resume owner was not called")
	}
	hubEvents := recentEvents(srv.hub)
	var resumed *eventEnvelope
	for i := range hubEvents {
		if hubEvents[i].Type == "session_resumed" {
			resumed = &hubEvents[i]
			break
		}
	}
	if resumed == nil {
		t.Fatalf("session_resumed event missing from %#v", hubEvents)
	}
	payload, ok := resumed.Data.(session.Session)
	if !ok {
		t.Fatalf("session_resumed data: got %T, want session.Session", resumed.Data)
	}
	if payload.Conversation.ID != "resume-full" || len(payload.Conversation.Messages) != 1 {
		t.Fatalf("conversation payload lost: %#v", payload.Conversation)
	}
	if payload.HandoffState.CurrentFocus != "verify full payload" {
		t.Fatalf("handoff state lost: %#v", payload.HandoffState)
	}
	if len(payload.Todos) != 1 || payload.Todos[0].Content != "prove resume payload" {
		t.Fatalf("todos lost: %#v", payload.Todos)
	}
	if len(payload.OrchestrationArtifacts) != 1 || payload.OrchestrationArtifacts[0].Path != artifact.Path {
		t.Fatalf("artifacts lost: %#v", payload.OrchestrationArtifacts)
	}
}

func TestBridgePublishesExpiredPromptEventsOnContextCancel(t *testing.T) {
	bridge := NewBridge()
	hub := newHub()
	bridge.attach(hub)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	decision, scope := bridge.Prompt(ctx, "Bash", json.RawMessage(`{"cmd":"date"}`), "run date", "test")
	if decision != permission.DecisionDeny || scope != permission.RememberNone {
		t.Fatalf("permission result: got %q/%q", decision, scope)
	}

	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if _, err := bridge.Ask(ctx, tool.AskRequest{Question: "Continue?"}); err == nil {
		t.Fatal("ask should return context error")
	}

	hub.mu.Lock()
	recent := append([]eventEnvelope(nil), hub.recent...)
	hub.mu.Unlock()
	if len(recent) != 4 {
		t.Fatalf("recent event count: got %d, want 4", len(recent))
	}
	if recent[0].Type != "permission_request" || recent[1].Type != "permission_expired" {
		t.Fatalf("permission events: got %q then %q", recent[0].Type, recent[1].Type)
	}
	if recent[2].Type != "ask_request" || recent[3].Type != "ask_expired" {
		t.Fatalf("ask events: got %q then %q", recent[2].Type, recent[3].Type)
	}
}

func TestHandlePermissionPublishesResponseEvent(t *testing.T) {
	bridge := NewBridge()
	respCh := make(chan permissionResponse, 1)
	bridge.permissions["permit-1"] = respCh
	srv := newServer(Config{Bridge: bridge})
	req := httptest.NewRequest(http.MethodPost, "/api/permission/permit-1", strings.NewReader(`{"data":{"type":"permission-responses","id":"permit-1","attributes":{"decision":"allow","scope":"session"}}}`))
	req.Header.Set("Content-Type", jsonAPIMediaType)
	rec := httptest.NewRecorder()

	srv.handlePermission(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := <-respCh
	if resp.Decision != permission.DecisionAllow || resp.Scope != permission.RememberSession {
		t.Fatalf("permission response: got %q/%q", resp.Decision, resp.Scope)
	}
	recent := recentEvents(srv.hub)
	if len(recent) != 1 || recent[0].Type != "permission_response" {
		t.Fatalf("recent events: got %#v", recent)
	}
	data, ok := recent[0].Data.(map[string]interface{})
	if !ok || data["id"] != "permit-1" {
		t.Fatalf("permission_response data: got %#v", recent[0].Data)
	}
	body := data["body"].(map[string]json.RawMessage)
	if _, ok := body["decision"]; !ok {
		t.Fatalf("permission_response body missing decision: %#v", body)
	}
}

func TestHandleAskPublishesResponseEvent(t *testing.T) {
	bridge := NewBridge()
	respCh := make(chan tool.AskResponse, 1)
	bridge.asks["ask-1"] = respCh
	srv := newServer(Config{Bridge: bridge})
	req := httptest.NewRequest(http.MethodPost, "/api/ask/ask-1", strings.NewReader(`{"data":{"type":"ask-responses","id":"ask-1","attributes":{"answers":{"Branch":"main"}}}}`))
	req.Header.Set("Content-Type", jsonAPIMediaType)
	rec := httptest.NewRecorder()

	srv.handleAsk(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	resp := <-respCh
	if resp.Answers["Branch"] != "main" {
		t.Fatalf("ask response: got %#v", resp.Answers)
	}
	recent := recentEvents(srv.hub)
	if len(recent) != 1 || recent[0].Type != "ask_response" {
		t.Fatalf("recent events: got %#v", recent)
	}
	data, ok := recent[0].Data.(map[string]interface{})
	if !ok || data["id"] != "ask-1" {
		t.Fatalf("ask_response data: got %#v", recent[0].Data)
	}
	body := data["body"].(map[string]json.RawMessage)
	if _, ok := body["answers"]; !ok {
		t.Fatalf("ask_response body missing answers: %#v", body)
	}
}

func recentEvents(h *hub) []eventEnvelope {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]eventEnvelope(nil), h.recent...)
}

func TestHandleArtifactServesOnlyRecordedArtifactsAsJSONAPI(t *testing.T) {
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "handoff.md")
	if err := os.WriteFile(artifactPath, []byte("handoff content"), 0o600); err != nil {
		t.Fatalf("write artifact: %v", err)
	}
	unrecordedPath := filepath.Join(dir, "other.md")
	if err := os.WriteFile(unrecordedPath, []byte("other content"), 0o600); err != nil {
		t.Fatalf("write unrecorded artifact: %v", err)
	}
	srv := newServer(Config{
		Bridge: NewBridge(),
		Store: app.NewStateStore(app.AppState{
			OrchestrationArtifacts: []app.OrchestrationArtifact{{Path: artifactPath, StateID: "review"}},
		}),
	})

	req := httptest.NewRequest(http.MethodGet, "/api/artifact?path="+url.QueryEscape(artifactPath), nil)
	rec := httptest.NewRecorder()
	srv.handleArtifact(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d, want %d; body=%s", rec.Code, http.StatusOK, rec.Body.String())
	}
	doc := decodeJSONAPIMap(t, rec.Body.Bytes())
	data := doc["data"].(map[string]interface{})
	if data["type"] != "artifacts" {
		t.Fatalf("resource type: got %v", data["type"])
	}
	attrs := data["attributes"].(map[string]interface{})
	if attrs["path"] != artifactPath || attrs["content"] != "handoff content" {
		t.Fatalf("artifact attributes: got %v", attrs)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/artifact?path="+url.QueryEscape(unrecordedPath), nil)
	rec = httptest.NewRecorder()
	srv.handleArtifact(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("unrecorded status: got %d, want %d", rec.Code, http.StatusForbidden)
	}
	assertJSONAPIError(t, rec.Body.Bytes(), "artifact path is not recorded in this session")
}

func decodeJSONAPIMap(t *testing.T, body []byte) map[string]interface{} {
	t.Helper()
	var doc map[string]interface{}
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decode JSON:API document: %v", err)
	}
	return doc
}

func assertJSONAPIError(t *testing.T, body []byte, wantDetail string) {
	t.Helper()
	doc := decodeJSONAPIMap(t, body)
	errors, ok := doc["errors"].([]interface{})
	if !ok || len(errors) != 1 {
		t.Fatalf("errors: got %T %v", doc["errors"], doc["errors"])
	}
	errObj := errors[0].(map[string]interface{})
	if detail := errObj["detail"]; detail != wantDetail {
		t.Fatalf("detail: got %v, want %q", detail, wantDetail)
	}
}
