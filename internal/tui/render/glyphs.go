package render

import (
	"runtime"
	"github.com/artpar/pragma/internal/observe"
)

// Unicode glyphs for visual hierarchy, matching Pragma TS reference.
// Platform-aware where needed (darwin vs linux/windows).
var (
	// BlackCircle is the status/spinner indicator.
	// macOS uses ⏺ for better vertical alignment; others use ●.
	BlackCircle = pickGlyph("⏺", "●")
)

const (
	// Bracket is the MessageResponse wrapper glyph (⎿).
	// Renders as "  ⎿  " — 2 spaces before, 2 after, dimColor.
	Bracket = "⎿"

	// BracketPrefix is the full bracket with spacing, matching TS reference.
	BracketPrefix = "  ⎿  "

	// Bullet is the list/item indicator.
	Bullet = "∙"

	// ToolHint is the tool call hint glyph.
	ToolHint = "⤿"

	// BlockquoteBar is the left bar for blockquotes.
	BlockquoteBar = "▎"

	// DiamondOpen indicates a running agent/task.
	DiamondOpen = "◇"

	// DiamondFilled indicates a completed agent/task.
	DiamondFilled = "◆"

	// HeavyHorizontal is used for separators.
	HeavyHorizontal = "━"

	// ThinkGlyph is the thinking indicator.
	ThinkGlyph = "✻"

	// ContentIndent is the indentation under a bracket.
	// Matches the width of BracketPrefix so content aligns.
	ContentIndent = "     "
)

func pickGlyph(darwin, other string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if runtime.GOOS == "darwin" {
		observe.GlobalTrace("if: runtime.GOOS == \"darwin\"")
		observe.GlobalTrace("return: darwin")
		return darwin
	}
	observe.GlobalTrace("return: other")
	return other
}
