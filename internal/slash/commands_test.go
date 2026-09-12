package slash

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
)

func TestHandleClear(t *testing.T) {
	conv := model.NewConversation(model.SystemPrompt{}, "test-model", "test", "/tmp")
	conv.Append(model.Message{
		ID:      model.NewUUID(),
		Role:    model.RoleUser,
		Content: []model.ContentPart{model.TextPart{Text: "hello"}},
	})
	oldID := conv.ID

	store := app.NewStateStore(app.AppState{
		Conversation: conv,
		CWD:          "/tmp",
	})

	deps := Deps{Store: store}
	result, err := handleClear(context.Background(), "", deps)
	if err != nil {
		t.Fatalf("handleClear error: %v", err)
	}

	if !result.ClearConversation {
		t.Error("ClearConversation should be true")
	}

	snap := store.Snapshot()
	if len(snap.Conversation.Messages) != 0 {
		t.Error("messages should be empty after clear")
	}
	if snap.Conversation.ID == oldID {
		t.Error("conversation ID should be regenerated after clear")
	}
}

func TestHandleCost(t *testing.T) {
	ct := model.NewCostTracker(0)
	ct.Record("claude-haiku", "anthropic", model.TokenUsage{
		InputTokens:  1000,
		OutputTokens: 500,
	}, model.Pricing{
		InputPerMToken:  0.25,
		OutputPerMToken: 1.25,
	})

	deps := Deps{
		CostTracker: ct,
		ModelName:   "claude-haiku",
		Provider:    "anthropic",
	}

	result, err := handleCost(context.Background(), "", deps)
	if err != nil {
		t.Fatalf("handleCost error: %v", err)
	}

	if !strings.Contains(result.DisplayText, "Session cost:") {
		t.Error("should contain session cost header")
	}
	if !strings.Contains(result.DisplayText, "claude-haiku") {
		t.Error("should contain model name")
	}
	if !strings.Contains(result.DisplayText, "1000") {
		t.Error("should contain input token count")
	}
}

func TestHandleCostEmpty(t *testing.T) {
	ct := model.NewCostTracker(0)
	deps := Deps{CostTracker: ct}

	result, err := handleCost(context.Background(), "", deps)
	if err != nil {
		t.Fatalf("handleCost error: %v", err)
	}
	if !strings.Contains(result.DisplayText, "$0.0000") {
		t.Error("empty cost should show $0.0000")
	}
}

func TestHandleCopyWritesLatestAssistantText(t *testing.T) {
	var copied string
	deps := Deps{
		LatestAssistantText: func() string { return "latest assistant message" },
		ClipboardWrite: func(text string) error {
			copied = text
			return nil
		},
	}

	result, err := handleCopy(context.Background(), "", deps)
	if err != nil {
		t.Fatalf("handleCopy error: %v", err)
	}
	if copied != "latest assistant message" {
		t.Fatalf("copied = %q, want latest assistant message", copied)
	}
	if !strings.Contains(result.DisplayText, "Copied") {
		t.Fatalf("display = %q, want copied status", result.DisplayText)
	}
}

func TestHandleCopyNoAssistantMessage(t *testing.T) {
	deps := Deps{
		LatestAssistantText: func() string { return "  " },
		ClipboardWrite:      func(string) error { return nil },
	}

	result, err := handleCopy(context.Background(), "", deps)
	if err != nil {
		t.Fatalf("handleCopy error: %v", err)
	}
	if !strings.Contains(result.DisplayText, "No assistant message") {
		t.Fatalf("display = %q, want no assistant message status", result.DisplayText)
	}
}

func TestHandleCopyClipboardFailure(t *testing.T) {
	wantErr := errors.New("clipboard unavailable")
	deps := Deps{
		LatestAssistantText: func() string { return "message" },
		ClipboardWrite:      func(string) error { return wantErr },
	}

	_, err := handleCopy(context.Background(), "", deps)
	if !errors.Is(err, wantErr) {
		t.Fatalf("handleCopy error = %v, want %v", err, wantErr)
	}
}

func TestHandleCopyUnavailableWithoutClipboard(t *testing.T) {
	result, err := handleCopy(context.Background(), "", Deps{})
	if err != nil {
		t.Fatalf("handleCopy error: %v", err)
	}
	if !strings.Contains(result.DisplayText, "clipboard access") {
		t.Fatalf("display = %q, want clipboard unavailable status", result.DisplayText)
	}
}

func TestHandleHelp(t *testing.T) {
	// Use Registry.Execute so deps.Commands is populated
	r := NewRegistry()
	result, err := r.Execute(context.Background(), "help", "", Deps{})
	if err != nil {
		t.Fatalf("handleHelp error: %v", err)
	}
	if !strings.Contains(result.DisplayText, "/compact") {
		t.Error("help should list /compact")
	}
	if !strings.Contains(result.DisplayText, "/clear") {
		t.Error("help should list /clear")
	}
	if !strings.Contains(result.DisplayText, "/exit") {
		t.Error("help should list /exit")
	}
	if !strings.Contains(result.DisplayText, "/copy") {
		t.Error("help should list /copy")
	}
}

func TestHandleExit(t *testing.T) {
	saved := false
	deps := Deps{
		SessionSave: func() error {
			saved = true
			return nil
		},
	}

	result, err := handleExit(context.Background(), "", deps)
	if err != nil {
		t.Fatalf("handleExit error: %v", err)
	}
	if !result.Quit {
		t.Error("Quit should be true")
	}
	if !saved {
		t.Error("session should be saved on exit")
	}
}

func TestHandleExitNilSave(t *testing.T) {
	// Should not panic with nil SessionSave
	result, err := handleExit(context.Background(), "", Deps{})
	if err != nil {
		t.Fatalf("handleExit error: %v", err)
	}
	if !result.Quit {
		t.Error("Quit should be true even with nil SessionSave")
	}
}

func TestHandleCompactNilCompactor(t *testing.T) {
	deps := Deps{
		Store: app.NewStateStore(app.AppState{}),
	}
	result, err := handleCompact(context.Background(), "", deps)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !strings.Contains(result.DisplayText, "not available") {
		t.Error("should indicate compaction not available")
	}
}

func testKnownProviders() []string {
	return []string{"anthropic", "openai", "openrouter", "morphllm", "google", "google-vertex", "groq"}
}

func TestHandleModelRejectsCrossProviderWithoutSwitcher(t *testing.T) {
	conv := model.NewConversation(model.SystemPrompt{}, "morph-glm53-744b", "morphllm", "/tmp")
	store := app.NewStateStore(app.AppState{Conversation: conv, Model: "morph-glm53-744b", CWD: "/tmp"})
	deps := Deps{
		Store:          store,
		ModelName:      "morph-glm53-744b",
		Provider:       "morphllm",
		KnownProviders: testKnownProviders(),
	}

	result, err := handleModel(context.Background(), "google/gemini-2.5-flash", deps)
	if err != nil {
		t.Fatalf("handleModel error: %v", err)
	}
	if !strings.Contains(result.DisplayText, "--provider google") {
		t.Errorf("expected restart guidance in %q", result.DisplayText)
	}
	if store.Snapshot().Model != "morph-glm53-744b" {
		t.Errorf("store model changed to %q despite rejected switch", store.Snapshot().Model)
	}
}

func TestHandleModelStripsSameProviderQualification(t *testing.T) {
	conv := model.NewConversation(model.SystemPrompt{}, "claude-sonnet-4-6-20250514", "anthropic", "/tmp")
	store := app.NewStateStore(app.AppState{Conversation: conv, Model: "claude-sonnet-4-6-20250514", CWD: "/tmp"})
	deps := Deps{
		Store:          store,
		ModelName:      "claude-sonnet-4-6-20250514",
		Provider:       "anthropic",
		KnownProviders: testKnownProviders(),
		ContextWindowFunc: func(modelID string) (int, bool) {
			if modelID == "claude-sonnet-4-6-20250514" {
				return 200000, true
			}
			return 0, false
		},
	}

	result, err := handleModel(context.Background(), "anthropic/claude-sonnet-4-6-20250514", deps)
	if err != nil {
		t.Fatalf("handleModel error: %v", err)
	}
	if snap := store.Snapshot(); snap.Model != "claude-sonnet-4-6-20250514" {
		t.Errorf("store model = %q, want bare claude-sonnet-4-6-20250514", snap.Model)
	}
	if !strings.Contains(result.DisplayText, "claude-sonnet-4-6-20250514") {
		t.Errorf("expected switch confirmation in %q", result.DisplayText)
	}
}

func TestHandleModelBareSlashedModelIDStaysWhole(t *testing.T) {
	// OpenRouter model IDs contain a vendor slash but no provider prefix.
	conv := model.NewConversation(model.SystemPrompt{}, "z-ai/glm-5.3", "openrouter", "/tmp")
	store := app.NewStateStore(app.AppState{Conversation: conv, Model: "z-ai/glm-5.3", CWD: "/tmp"})
	deps := Deps{
		Store:          store,
		ModelName:      "z-ai/glm-5.3",
		Provider:       "openrouter",
		KnownProviders: testKnownProviders(),
		ContextWindowFunc: func(modelID string) (int, bool) {
			if modelID == "z-ai/glm-5.3" || modelID == "z-ai/glm-5.3-flash" {
				return 1310720, true
			}
			return 0, false
		},
	}

	_, err := handleModel(context.Background(), "z-ai/glm-5.3-flash", deps)
	if err != nil {
		t.Fatalf("handleModel error: %v", err)
	}
	if snap := store.Snapshot(); snap.Model != "z-ai/glm-5.3-flash" {
		t.Errorf("store model = %q, want z-ai/glm-5.3-flash unchanged", snap.Model)
	}
}

func TestSplitQualifiedModelArg(t *testing.T) {
	deps := Deps{KnownProviders: testKnownProviders()}
	cases := []struct {
		input        string
		wantProvider string
		wantModel    string
	}{
		{"google/gemini-2.5-flash", "google", "gemini-2.5-flash"},
		{"z-ai/glm-5.3", "", "z-ai/glm-5.3"},
		{"morph-glm53-744b", "", "morph-glm53-744b"},
		{"/leading", "", "/leading"},
	}
	for _, tc := range cases {
		gotProvider, gotModel := splitQualifiedModelArg(deps, tc.input)
		if gotProvider != tc.wantProvider || gotModel != tc.wantModel {
			t.Errorf("splitQualifiedModelArg(%q) = (%q, %q), want (%q, %q)",
				tc.input, gotProvider, gotModel, tc.wantProvider, tc.wantModel)
		}
	}
}
