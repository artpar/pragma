package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/artpar/pragma/internal/model"
)

func TestInputComponentAlwaysActive(t *testing.T) {
	ic := newInputComponent()

	// Enter with no text should not produce a message
	cmd := ic.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("empty enter should not produce a command")
	}

	// View always renders the textarea (never disabled)
	view := ic.View()
	if view == "" {
		t.Error("view should not be empty")
	}
}

func TestInputComponentEmptyEnter(t *testing.T) {
	ic := newInputComponent()

	// Enter with no text should not produce a message
	cmd := ic.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Error("empty enter should not produce a command")
	}
}

func TestInputComponentReset(t *testing.T) {
	ic := newInputComponent()
	ic.Reset()
	// Should not panic, and value should be empty
	if ic.textarea.Value() != "" {
		t.Error("reset should clear textarea value")
	}
}

func TestInputComponentPromptHistoryRing(t *testing.T) {
	ic := newInputComponent()
	submitInput(t, &ic, "first prompt")
	submitInput(t, &ic, "second prompt")

	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "second prompt" {
		t.Fatalf("first up = %q, want second prompt", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "first prompt" {
		t.Fatalf("second up = %q, want first prompt", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "first prompt" {
		t.Fatalf("up at oldest = %q, want first prompt", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := ic.textarea.Value(); got != "second prompt" {
		t.Fatalf("down = %q, want second prompt", got)
	}
}

func TestInputComponentPromptHistoryRestoresDraft(t *testing.T) {
	ic := newInputComponent()
	submitInput(t, &ic, "previous prompt")

	ic.textarea.SetValue("draft prompt")
	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "previous prompt" {
		t.Fatalf("up = %q, want previous prompt", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyDown})
	if got := ic.textarea.Value(); got != "draft prompt" {
		t.Fatalf("down restored = %q, want draft prompt", got)
	}
}

func TestInputComponentPromptHistoryLimit(t *testing.T) {
	ic := newInputComponent()
	for i := 0; i < inputHistoryLimit+5; i++ {
		ic.remember(string(rune('a' + i%26)))
	}
	if len(ic.history) != inputHistoryLimit {
		t.Fatalf("history len = %d, want %d", len(ic.history), inputHistoryLimit)
	}
}

func TestInputComponentSetHistory(t *testing.T) {
	ic := newInputComponent()
	ic.SetHistory([]string{"prior prompt 1", "prior prompt 2"})

	// Verify history is populated
	if len(ic.history) != 2 {
		t.Fatalf("history len = %d, want 2", len(ic.history))
	}

	// Navigate up should show prior prompt 2
	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "prior prompt 2" {
		t.Fatalf("first up = %q, want prior prompt 2", got)
	}

	// Navigate up again should show prior prompt 1
	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "prior prompt 1" {
		t.Fatalf("second up = %q, want prior prompt 1", got)
	}
}

func TestInputComponentSetHistoryThenNewPrompts(t *testing.T) {
	ic := newInputComponent()
	ic.SetHistory([]string{"prior prompt"})

	// Add a new prompt in this session
	ic.textarea.SetValue("new prompt")
	ic.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Up should show new prompt first
	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "new prompt" {
		t.Fatalf("up after new prompt = %q, want new prompt", got)
	}

	// Up again should show prior prompt
	ic.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := ic.textarea.Value(); got != "prior prompt" {
		t.Fatalf("up again = %q, want prior prompt", got)
	}
}

func TestInputComponentReverseSearchAcceptsMatch(t *testing.T) {
	ic := newInputComponent()
	ic.SetHistory([]string{"inspect auth flow", "fix prompt history", "copy assistant message"})

	ic.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	typeRunes(&ic, "history")
	if got := ic.textarea.Value(); got != "fix prompt history" {
		t.Fatalf("search match = %q, want fix prompt history", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if got := ic.textarea.Value(); got != "fix prompt history" {
		t.Fatalf("accepted search = %q, want fix prompt history", got)
	}

	cmd := ic.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("accepted search should submit on second enter")
	}
}

func TestInputComponentReverseSearchCyclesAndCancels(t *testing.T) {
	ic := newInputComponent()
	ic.SetHistory([]string{"older copy task", "middle history task", "newer copy task"})
	ic.textarea.SetValue("draft")

	ic.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	typeRunes(&ic, "copy")
	if got := ic.textarea.Value(); got != "newer copy task" {
		t.Fatalf("first search match = %q, want newer copy task", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	if got := ic.textarea.Value(); got != "older copy task" {
		t.Fatalf("cycled search match = %q, want older copy task", got)
	}

	ic.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if got := ic.textarea.Value(); got != "draft" {
		t.Fatalf("cancelled search = %q, want draft", got)
	}
}

func TestInputComponentReverseSearchNoMatchKeepsDraft(t *testing.T) {
	ic := newInputComponent()
	ic.SetHistory([]string{"previous prompt"})
	ic.textarea.SetValue("draft")

	ic.Update(tea.KeyMsg{Type: tea.KeyCtrlR})
	typeRunes(&ic, "zzz")
	if got := ic.textarea.Value(); got != "draft" {
		t.Fatalf("no-match search = %q, want draft", got)
	}
}

func TestExtractUserPrompts(t *testing.T) {
	msgs := []model.Message{
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.TextPart{Text: "first user prompt"},
			},
		},
		{
			Role: model.RoleAssistant,
			Content: []model.ContentPart{
				model.TextPart{Text: "assistant response"},
			},
		},
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.TextPart{Text: "second user prompt"},
			},
		},
	}

	prompts := extractUserPrompts(msgs)
	if len(prompts) != 2 {
		t.Fatalf("prompts len = %d, want 2", len(prompts))
	}
	if prompts[0] != "first user prompt" {
		t.Fatalf("prompts[0] = %q, want first user prompt", prompts[0])
	}
	if prompts[1] != "second user prompt" {
		t.Fatalf("prompts[1] = %q, want second user prompt", prompts[1])
	}
}

func TestExtractUserPromptsIgnoresNonText(t *testing.T) {
	// Image parts should be ignored for history
	msgs := []model.Message{
		{
			Role: model.RoleUser,
			Content: []model.ContentPart{
				model.TextPart{Text: "prompt with image"},
				model.ImagePart{MimeType: "image/png", Data: []byte("pngdata")},
			},
		},
	}

	prompts := extractUserPrompts(msgs)
	if len(prompts) != 1 {
		t.Fatalf("prompts len = %d, want 1", len(prompts))
	}
	if prompts[0] != "prompt with image" {
		t.Fatalf("prompts[0] = %q, want prompt with image", prompts[0])
	}
}

func typeRunes(ic *inputComponent, text string) {
	for _, r := range text {
		ic.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func submitInput(t *testing.T, ic *inputComponent, text string) {
	t.Helper()
	ic.textarea.SetValue(text)
	cmd := ic.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("submit %q returned nil command", text)
	}
	msg := cmd()
	submitted, ok := msg.(InputSubmittedMsg)
	if !ok {
		t.Fatalf("submit %q message type = %T, want InputSubmittedMsg", text, msg)
	}
	if submitted.Text != text {
		t.Fatalf("submitted text = %q, want %q", submitted.Text, text)
	}
}
