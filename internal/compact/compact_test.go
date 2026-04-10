package compact

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/provider"
)

// replayProvider is a real provider.Provider backed by a predetermined response.
type replayProvider struct {
	mu       sync.Mutex
	response model.Response
	err      error
}

func (rp *replayProvider) Name() string                            { return "replay" }
func (rp *replayProvider) SupportsFeature(_ provider.Feature) bool { return true }
func (rp *replayProvider) Pricing(_ string) (model.Pricing, bool) {
	return model.Pricing{InputPerMToken: 0.25, OutputPerMToken: 1.25}, true
}
func (rp *replayProvider) ContextWindow(_ string) (int, bool) { return 200_000, true }

func (rp *replayProvider) Stream(_ context.Context, _ provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, errors.New("streaming not supported by replay provider")
}

func (rp *replayProvider) Complete(_ context.Context, _ provider.RequestParams) (model.Response, error) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.response, rp.err
}

func buildTestMessages(n int) []model.Message {
	msgs := make([]model.Message, n)
	for i := range n {
		role := model.RoleUser
		if i%2 == 1 {
			role = model.RoleAssistant
		}
		msgs[i] = model.Message{
			ID:        model.NewUUID(),
			Role:      role,
			Content:   []model.ContentPart{model.TextPart{Text: strings.Repeat("x", 200)}},
			Timestamp: time.Now(),
		}
	}
	return msgs
}

func TestCompactSuccess(t *testing.T) {
	summaryText := "<analysis>\nSome thinking...\n</analysis>\n\n<summary>\n1. Primary Request:\nUser asked for help.\n</summary>"
	prov := &replayProvider{
		response: model.Response{
			Model:      "claude-haiku",
			Content:    []model.ContentPart{model.TextPart{Text: summaryText}},
			StopReason: model.StopEndTurn,
			Usage:      model.TokenUsage{InputTokens: 1000, OutputTokens: 200},
		},
	}

	bus := observe.NewEventBus(64)
	ct := model.NewCostTracker()
	svc := NewService(prov, bus, ct, "claude-haiku")

	msgs := buildTestMessages(6)
	system := model.SystemPrompt{Blocks: []model.SystemBlock{{Text: "You are a helpful assistant."}}}

	result, err := svc.Compact(context.Background(), msgs, system, "")
	if err != nil {
		t.Fatalf("Compact() error: %v", err)
	}

	if result.PreTokenCount == 0 {
		t.Error("PreTokenCount should be > 0")
	}
	if result.PostTokenCount >= result.PreTokenCount {
		t.Errorf("PostTokenCount (%d) should be < PreTokenCount (%d)", result.PostTokenCount, result.PreTokenCount)
	}
	if result.MessagesRemoved != 6 {
		t.Errorf("MessagesRemoved = %d, want 6", result.MessagesRemoved)
	}
	if len(result.ReplacementMessages) != 1 {
		t.Fatalf("expected 1 replacement message, got %d", len(result.ReplacementMessages))
	}
	if !result.ReplacementMessages[0].Flags.IsCompactSummary {
		t.Error("replacement message should have IsCompactSummary flag")
	}
	if !strings.Contains(result.Summary, "Primary Request") {
		t.Error("summary should contain formatted content")
	}
	// Verify cost was tracked
	if ct.TotalUSD() == 0 {
		t.Error("compaction API call cost should be tracked")
	}

	bus.Drain()
}

func TestCompactTooFewMessages(t *testing.T) {
	bus := observe.NewEventBus(64)
	ct := model.NewCostTracker()
	svc := NewService(&replayProvider{}, bus, ct, "claude-haiku")

	_, err := svc.Compact(context.Background(), buildTestMessages(3), model.SystemPrompt{}, "")
	if !errors.Is(err, ErrTooFewMessages) {
		t.Errorf("expected ErrTooFewMessages, got: %v", err)
	}
	bus.Drain()
}

func TestCompactEmptySummary(t *testing.T) {
	prov := &replayProvider{
		response: model.Response{
			Content:    []model.ContentPart{model.TextPart{Text: ""}},
			StopReason: model.StopEndTurn,
		},
	}

	bus := observe.NewEventBus(64)
	ct := model.NewCostTracker()
	svc := NewService(prov, bus, ct, "claude-haiku")

	_, err := svc.Compact(context.Background(), buildTestMessages(6), model.SystemPrompt{}, "")
	if !errors.Is(err, ErrEmptySummary) {
		t.Errorf("expected ErrEmptySummary, got: %v", err)
	}
	bus.Drain()
}

func TestCompactAPIError(t *testing.T) {
	prov := &replayProvider{
		err: errors.New("API rate limit exceeded"),
	}

	bus := observe.NewEventBus(64)
	ct := model.NewCostTracker()
	svc := NewService(prov, bus, ct, "claude-haiku")

	_, err := svc.Compact(context.Background(), buildTestMessages(6), model.SystemPrompt{}, "")
	if err == nil {
		t.Fatal("expected error from API failure")
	}
	if !strings.Contains(err.Error(), "compaction API call") {
		t.Errorf("error should wrap with compaction context, got: %v", err)
	}
	bus.Drain()
}

func TestCompactGrew(t *testing.T) {
	// Return a summary that's larger than the original messages
	hugeSummary := "<summary>\n" + strings.Repeat("verbose output ", 5000) + "\n</summary>"
	prov := &replayProvider{
		response: model.Response{
			Content:    []model.ContentPart{model.TextPart{Text: hugeSummary}},
			StopReason: model.StopEndTurn,
			Usage:      model.TokenUsage{InputTokens: 100, OutputTokens: 20000},
		},
	}

	bus := observe.NewEventBus(64)
	ct := model.NewCostTracker()
	svc := NewService(prov, bus, ct, "claude-haiku")

	// Use short messages so the summary is guaranteed to be bigger
	msgs := buildTestMessages(4) // minimum
	_, err := svc.Compact(context.Background(), msgs, model.SystemPrompt{}, "")
	if !errors.Is(err, ErrCompactionGrew) {
		t.Errorf("expected ErrCompactionGrew, got: %v", err)
	}
	bus.Drain()
}

func TestCompactWithCustomInstructions(t *testing.T) {
	var capturedParams provider.RequestParams
	prov := &capturingProvider{
		response: model.Response{
			Content:    []model.ContentPart{model.TextPart{Text: "<summary>\nCustom summary\n</summary>"}},
			StopReason: model.StopEndTurn,
		},
		capture: func(p provider.RequestParams) { capturedParams = p },
	}

	bus := observe.NewEventBus(64)
	ct := model.NewCostTracker()
	svc := NewService(prov, bus, ct, "claude-haiku")

	_, err := svc.Compact(context.Background(), buildTestMessages(6), model.SystemPrompt{}, "Focus on test results")
	if err != nil {
		t.Fatalf("Compact() error: %v", err)
	}

	// Verify custom instructions were included in the prompt
	if len(capturedParams.Messages) == 0 {
		t.Fatal("expected captured params to have messages")
	}
	userContent := ""
	for _, part := range capturedParams.Messages[0].Content {
		if tp, ok := part.(model.TextPart); ok {
			userContent = tp.Text
		}
	}
	if !strings.Contains(userContent, "Focus on test results") {
		t.Error("custom instructions should be included in compaction prompt")
	}
	bus.Drain()
}

// capturingProvider captures the params sent to Complete for inspection.
type capturingProvider struct {
	response model.Response
	capture  func(provider.RequestParams)
}

func (cp *capturingProvider) Name() string                            { return "capture" }
func (cp *capturingProvider) SupportsFeature(_ provider.Feature) bool { return true }
func (cp *capturingProvider) Pricing(_ string) (model.Pricing, bool) {
	return model.Pricing{InputPerMToken: 0.25, OutputPerMToken: 1.25}, true
}
func (cp *capturingProvider) ContextWindow(_ string) (int, bool) { return 200_000, true }
func (cp *capturingProvider) Stream(_ context.Context, _ provider.RequestParams) (<-chan provider.StreamChunk, error) {
	return nil, errors.New("not supported")
}
func (cp *capturingProvider) Complete(_ context.Context, params provider.RequestParams) (model.Response, error) {
	if cp.capture != nil {
		cp.capture(params)
	}
	return cp.response, nil
}
