package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newModelDialogFixture() *modelDialog {
	var d modelDialog
	d.Show([]string{
		"anthropic/claude-opus-4-6-20250610",
		"anthropic/claude-sonnet-4-6-20250514",
		"google/gemini-2.5-flash",
		"morphllm/morph-glm53-744b",
	}, "morphllm/morph-glm53-744b")
	return &d
}

func TestModelDialogFilterNarrowsAndSelects(t *testing.T) {
	d := newModelDialogFixture()

	for _, r := range "gem" {
		if selected := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}); selected != "" {
			t.Fatalf("typing %q unexpectedly selected %q", string(r), selected)
		}
	}

	visible := d.visible()
	if len(visible) != 1 || visible[0] != "google/gemini-2.5-flash" {
		t.Fatalf("visible after filter = %v, want [google/gemini-2.5-flash]", visible)
	}

	selected := d.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if selected != "google/gemini-2.5-flash" {
		t.Fatalf("Enter selected %q, want google/gemini-2.5-flash", selected)
	}
	if d.active {
		t.Error("dialog should be dismissed after selection")
	}
}

func TestModelDialogFilterMatchesProviderName(t *testing.T) {
	d := newModelDialogFixture()

	for _, r := range "anthropic" {
		d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	visible := d.visible()
	if len(visible) != 2 {
		t.Fatalf("visible after filter = %v, want the two anthropic models", visible)
	}
	for _, m := range visible {
		if m[:10] != "anthropic/" {
			t.Errorf("unexpected model %q in filtered view", m)
		}
	}
}

func TestModelDialogDigitJumpOnlyWithoutFilter(t *testing.T) {
	d := newModelDialogFixture()

	// With an empty filter, digits jump to an entry.
	if selected := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}}); selected != "" {
		t.Fatalf("digit unexpectedly selected %q", selected)
	}
	if d.selected != 1 {
		t.Fatalf("selected index = %d, want 1", d.selected)
	}

	// With a filter active, digits extend the filter instead.
	for _, r := range "g1" {
		d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if d.filter != "g1" {
		t.Fatalf("filter = %q, want \"g1\"", d.filter)
	}
}

func TestModelDialogBackspaceEditsFilter(t *testing.T) {
	d := newModelDialogFixture()

	for _, r := range "gpt" {
		d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	d.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if d.filter != "gp" {
		t.Fatalf("filter = %q, want \"gp\" after backspace", d.filter)
	}
}

func TestModelDialogEscClearsFilterBeforeDismissing(t *testing.T) {
	d := newModelDialogFixture()

	for _, r := range "gem" {
		d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if !d.active {
		t.Fatal("Esc with an active filter should clear the filter, not dismiss")
	}
	if d.filter != "" {
		t.Fatalf("filter = %q, want cleared", d.filter)
	}
	if len(d.visible()) != 4 {
		t.Fatalf("visible = %v, want all four models restored", d.visible())
	}

	d.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if d.active {
		t.Fatal("second Esc should dismiss the dialog")
	}
}

func TestModelDialogCurrentModelHighlighted(t *testing.T) {
	d := newModelDialogFixture()
	rendered := d.View(80, 40)
	if !contains(rendered, "(current)") {
		t.Error("current model marker missing from dialog view")
	}
}

func TestModelDialogRendersBoundedWindow(t *testing.T) {
	// The catalog can hold hundreds of "provider/model" entries; the
	// dialog must render a bounded window, not dump every row at once.
	models := make([]string, 300)
	for i := range models {
		models[i] = fmt.Sprintf("provider-a/model-%03d", i)
	}
	var d modelDialog
	d.Show(models, "provider-a/model-000")

	rendered := d.View(80, 24)
	if lines := strings.Count(rendered, "\n"); lines >= 24 {
		t.Fatalf("dialog rendered %d lines for a 24-row terminal, want a bounded window", lines)
	}
	if !contains(rendered, "model-000") {
		t.Error("current model should be visible in the initial window")
	}
	if !contains(rendered, "more below") {
		t.Error("missing indicator that the list continues below the window")
	}
	if !contains(rendered, "showing ") {
		t.Error("missing position footer (showing X–Y of N)")
	}
	// Bounded rendering must not depend on catalog size.
	renderedAgain := d.View(80, 24)
	if strings.Count(renderedAgain, "\n") != strings.Count(rendered, "\n") {
		t.Error("re-render should be stable, not grow")
	}
}

func TestModelDialogScrollsSelectionIntoView(t *testing.T) {
	models := make([]string, 300)
	for i := range models {
		models[i] = fmt.Sprintf("provider-a/model-%03d", i)
	}
	var d modelDialog
	d.Show(models, "provider-a/model-000")

	// Walk the selection far past the first window.
	for i := 0; i < 40; i++ {
		if selected := d.Update(tea.KeyMsg{Type: tea.KeyDown}); selected != "" {
			t.Fatalf("arrow down unexpectedly selected %q", selected)
		}
	}
	if d.selected != 40 {
		t.Fatalf("selected = %d, want 40", d.selected)
	}

	rendered := d.View(80, 24)
	if !contains(rendered, "model-040") {
		t.Error("selected model must be visible after scrolling down")
	}
	if !contains(rendered, "more above") {
		t.Error("missing indicator that the list continues above the window")
	}
}

func TestModelDialogFilterOnLargeCatalog(t *testing.T) {
	models := make([]string, 300)
	for i := range models {
		models[i] = fmt.Sprintf("provider-a/model-%03d", i)
	}
	var d modelDialog
	d.Show(models, "")

	// Type a filter matching only model-257. Sent as one batched event:
	// a lone first rune '2' would be interpreted as a digit jump (which
	// only applies to single-rune input while the filter is empty).
	if selected := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("257")}); selected != "" {
		t.Fatalf("typing filter unexpectedly selected %q", selected)
	}
	visible := d.visible()
	if len(visible) != 1 || visible[0] != "provider-a/model-257" {
		t.Fatalf("visible after filter = %v, want [provider-a/model-257]", visible)
	}

	// The filtered window shows the match without scroll indicators.
	rendered := d.View(80, 24)
	if !contains(rendered, "model-257") {
		t.Error("filtered match missing from rendered dialog")
	}
	if contains(rendered, "more below") || contains(rendered, "more above") {
		t.Error("single match should not carry scroll indicators")
	}

	if selected := d.Update(tea.KeyMsg{Type: tea.KeyEnter}); selected != "provider-a/model-257" {
		t.Fatalf("Enter selected %q, want provider-a/model-257", selected)
	}
	if d.active {
		t.Error("dialog should be dismissed after selection")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestModelDialogHandlesBatchedRunes(t *testing.T) {
	// Bubbletea batches consecutive printable input into a single KeyRunes
	// event (e.g. pasted text or fast terminal input); the whole batch must
	// land in the filter, not be dropped.
	d := newModelDialogFixture()

	selected := d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("gemini")})
	if selected != "" {
		t.Fatalf("batched input unexpectedly selected %q", selected)
	}
	if d.filter != "gemini" {
		t.Fatalf("filter = %q, want \"gemini\"", d.filter)
	}
	visible := d.visible()
	if len(visible) != 1 || visible[0] != "google/gemini-2.5-flash" {
		t.Fatalf("visible after batched filter = %v, want [google/gemini-2.5-flash]", visible)
	}
}

func TestModelDialogIgnoresAltRunes(t *testing.T) {
	d := newModelDialogFixture()
	d.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}, Alt: true})
	if d.filter != "" {
		t.Fatalf("alt-modified rune leaked into filter: %q", d.filter)
	}
}
