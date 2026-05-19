package render

import (
	"runtime"
	"github.com/artpar/pragma/internal/observe"
)

// Unicode glyphs for visual hierarchy.
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

	// BracketPrefix is the full bracket with spacing.
	BracketPrefix = "  ⎿  "

	// DiamondOpen indicates a running agent/task.
	DiamondOpen = "◇"

	// DiamondFilled indicates a completed agent/task.
	DiamondFilled = "◆"

	// ThinkGlyph is the thinking indicator (∴ = U+2234 THEREFORE).
	ThinkGlyph = "∴"

	// TeardropAsterisk is the system message marker (✻ = U+273B).
	// Used for compaction boundary, scheduled tasks, permission retry.
	TeardropAsterisk = "✻"

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
