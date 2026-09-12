package tui

import (
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
	rendered := d.View(80)
	if !contains(rendered, "(current)") {
		t.Error("current model marker missing from dialog view")
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
