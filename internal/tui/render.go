package tui

// This file previously contained all rendering logic.
// It has been refactored into the internal/tui/render/ sub-package:
//   - render/glyphs.go: Unicode glyph constants
//   - render/markdown.go: glamour-based markdown renderer
//   - render/content.go: message, tool call, thinking, bracket rendering
//   - render/toolrender.go: per-tool result renderers (Bash, Read, Edit, etc.)
//
// The tui package now imports render/ and uses its exported functions.
