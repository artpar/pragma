package util

import (
	"fmt"
	"strings"
)

const maxDiffLines = 50000

// GenerateEditDiff generates a unified diff string for an edit operation.
// It takes the original file content, the old/new strings, file path,
// whether to replace all occurrences, and how many context lines to show.
// Returns empty string if the file is too large or the old string is not found.
func GenerateEditDiff(fileContent, oldStr, newStr, filePath string, replaceAll bool, contextLines int) string {
	if oldStr == newStr {
		return ""
	}
	if oldStr == "" && fileContent == "" {
		// New file creation: show all new lines as additions
		return generateNewFileDiff(newStr)
	}
	if oldStr == "" {
		// Empty old_string with existing content: no match to diff
		return ""
	}

	origLines := strings.Split(fileContent, "\n")
	if len(origLines) > maxDiffLines {
		return ""
	}

	// Find all occurrence positions (byte offsets)
	var positions []int
	if replaceAll {
		start := 0
		for {
			idx := strings.Index(fileContent[start:], oldStr)
			if idx < 0 {
				break
			}
			positions = append(positions, start+idx)
			start += idx + len(oldStr)
		}
	} else {
		idx := strings.Index(fileContent, oldStr)
		if idx >= 0 {
			positions = []int{idx}
		}
	}

	if len(positions) == 0 {
		return ""
	}

	// Build hunks from actual file lines (not old_string/new_string splits)
	var hunks []hunk
	for _, pos := range positions {
		h := buildHunk(fileContent, origLines, pos, oldStr, newStr, contextLines)
		hunks = append(hunks, h)
	}

	// Merge overlapping hunks
	hunks = mergeHunks(hunks)

	// Format as unified diff
	var b strings.Builder
	for i, h := range hunks {
		if i > 0 {
			b.WriteString("...\n")
		}
		b.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", h.oldStart, h.oldCount, h.newStart, h.newCount))
		for _, line := range h.lines {
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

type hunk struct {
	oldStart int // 1-based
	oldCount int
	newStart int // 1-based
	newCount int
	lines    []string // prefixed with " ", "-", "+"
}

// buildHunk creates a unified diff hunk for a single replacement at the given byte offset.
// Uses actual file lines for the diff, not the raw old_string/new_string, so partial-line
// matches render correctly (e.g., old_string without leading whitespace).
func buildHunk(fileContent string, origLines []string, byteOffset int, oldStr, newStr string, contextLines int) hunk {
	// Find which lines the match spans
	matchStartLine := strings.Count(fileContent[:byteOffset], "\n")
	matchEndLine := matchStartLine + strings.Count(oldStr, "\n")

	// Build the modified content locally to extract the new lines
	modified := fileContent[:byteOffset] + newStr + fileContent[byteOffset+len(oldStr):]
	modLines := strings.Split(modified, "\n")

	// How many new lines the replacement produces
	newLineCount := (matchEndLine - matchStartLine + 1) + (strings.Count(newStr, "\n") - strings.Count(oldStr, "\n"))
	if newLineCount < 0 {
		newLineCount = 0
	}

	// Context boundaries (clamped to file bounds)
	ctxStart := matchStartLine - contextLines
	if ctxStart < 0 {
		ctxStart = 0
	}
	ctxEnd := matchEndLine + 1 + contextLines
	if ctxEnd > len(origLines) {
		ctxEnd = len(origLines)
	}

	// Build diff lines
	var diffLines []string

	// Context before (from original)
	for i := ctxStart; i < matchStartLine; i++ {
		diffLines = append(diffLines, " "+origLines[i])
	}
	// Removed lines (actual file lines)
	for i := matchStartLine; i <= matchEndLine; i++ {
		diffLines = append(diffLines, "-"+origLines[i])
	}
	// Added lines (from modified content at the same position)
	modEndLine := matchStartLine + newLineCount - 1
	if newLineCount == 0 {
		// Pure deletion — no added lines
	} else {
		for i := matchStartLine; i <= modEndLine && i < len(modLines); i++ {
			diffLines = append(diffLines, "+"+modLines[i])
		}
	}
	// Context after (from original)
	for i := matchEndLine + 1; i < ctxEnd; i++ {
		diffLines = append(diffLines, " "+origLines[i])
	}

	// Compute hunk header counts
	ctxBeforeCount := matchStartLine - ctxStart
	ctxAfterCount := ctxEnd - (matchEndLine + 1)
	oldCount := ctxBeforeCount + (matchEndLine - matchStartLine + 1) + ctxAfterCount
	newCount := ctxBeforeCount + newLineCount + ctxAfterCount

	return hunk{
		oldStart: ctxStart + 1, // 1-based
		oldCount: oldCount,
		newStart: ctxStart + 1,
		newCount: newCount,
		lines:    diffLines,
	}
}

// mergeHunks merges overlapping or adjacent hunks.
func mergeHunks(hunks []hunk) []hunk {
	if len(hunks) <= 1 {
		return hunks
	}

	var merged []hunk
	current := hunks[0]

	for i := 1; i < len(hunks); i++ {
		next := hunks[i]
		// Check if hunks overlap or are adjacent
		currentEnd := current.oldStart + current.oldCount
		if currentEnd >= next.oldStart {
			current = mergeTwo(current, next)
		} else {
			merged = append(merged, current)
			current = next
		}
	}
	merged = append(merged, current)
	return merged
}

// mergeTwo merges two overlapping hunks into one.
func mergeTwo(a, b hunk) hunk {
	aEnd := a.oldStart + a.oldCount
	overlapLines := aEnd - b.oldStart
	if overlapLines < 0 {
		overlapLines = 0
	}

	// Take all of a's lines, skip overlapping context from b
	var lines []string
	lines = append(lines, a.lines...)
	skip := overlapLines
	for _, l := range b.lines {
		if skip > 0 && strings.HasPrefix(l, " ") {
			skip--
			continue
		}
		skip = 0
		lines = append(lines, l)
	}

	// Recount from the actual lines
	oldCount := 0
	newCount := 0
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, " "):
			oldCount++
			newCount++
		case strings.HasPrefix(l, "-"):
			oldCount++
		case strings.HasPrefix(l, "+"):
			newCount++
		}
	}

	return hunk{
		oldStart: a.oldStart,
		oldCount: oldCount,
		newStart: a.newStart,
		newCount: newCount,
		lines:    lines,
	}
}

// generateNewFileDiff generates a diff for new file creation (empty → content).
func generateNewFileDiff(newContent string) string {
	if newContent == "" {
		return ""
	}
	newLines := strings.Split(newContent, "\n")
	var b strings.Builder
	b.WriteString(fmt.Sprintf("@@ -0,0 +1,%d @@\n", len(newLines)))
	for _, l := range newLines {
		b.WriteString("+" + l + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
