package applypatch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/util"
)

type Input struct {
	Patch string `json:"patch" desc:"The apply_patch body to parse, verify, and apply"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["patch"],
	"properties": {
		"patch": {
			"type": "string",
			"description": "Patch body using *** Begin Patch / *** End Patch format"
		}
	}
}`)

const (
	ToolName       = "apply_patch"
	LegacyToolName = "ApplyPatch"
)

type Tool struct{}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return ToolName
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return description
}

const description = `Applies structured file edits after parsing and verifying patch hunks against the current filesystem.

Use this for source edits, especially multi-line or multi-file changes. The input is JSON with a single patch string:

{"patch":"*** Begin Patch\n...\n*** End Patch"}

Patch grammar:
- The first non-empty line must be exactly *** Begin Patch.
- The last non-empty line must be exactly *** End Patch.
- Use *** Add File: path, *** Update File: path, or *** Delete File: path.
- Inside an update hunk, every line must start with one of: space for unchanged context, - for removed lines, + for added lines, or @@ to start a hunk.
- Unchanged context lines must include the leading space.
- Do not place standalone *** lines inside update hunks.
- Read the target range immediately before patching so update hunks match current file content.

Valid examples:

*** Begin Patch
*** Add File: path
+new line
*** End Patch

*** Begin Patch
*** Update File: path
@@
 context line
-old line
+new line
 unchanged context
*** End Patch

*** Begin Patch
*** Delete File: path
*** End Patch

Every update hunk must match current file content before any file is written. If verification fails, no files are changed.`

// LegacyTool keeps old saved sessions and local toolsets callable while the
// model-visible primary tool uses Codex-compatible lowercase naming.
type LegacyTool struct{ Tool }

func (t *LegacyTool) Name() string {
	return LegacyToolName
}

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return inputSchema
}

func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "applypatch", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "applypatch", "Tool.CheckPerm", "exit")
	var in Input
	if err := json.Unmarshal(input, &in); err != nil || strings.TrimSpace(in.Patch) == "" {
		return checker.Check(ctx, ToolName, "")
	}
	paths, err := PatchPaths(in.Patch)
	if err != nil {
		return checker.Check(ctx, ToolName, "")
	}
	sort.Strings(paths)
	return checker.Check(ctx, ToolName, strings.Join(paths, "\n"))
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "applypatch", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "applypatch", "Tool.Invoke", "exit")
	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	return ApplyPatchText(ctx, in.Patch, state.WorkDir(), state)
}

func ApplyPatchText(ctx context.Context, patch string, workDir string, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "enter")
	defer observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "exit")
	parsed, err := Parse(patch)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("apply_patch verification failed: %w", err)
	}
	if len(parsed.Ops) == 0 {
		return tool.InvokeResult{}, fmt.Errorf("apply_patch verification failed: patch contains no file hunks")
	}
	verified, err := verify(parsed, workDir)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("apply_patch verification failed: %w", err)
	}
	if err := writeVerified(ctx, verified, state); err != nil {
		return tool.InvokeResult{}, err
	}
	return renderResult(verified), nil
}

func PatchPaths(patch string) ([]string, error) {
	parsed, err := Parse(patch)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var paths []string
	for _, op := range parsed.Ops {
		if !seen[op.Path] {
			seen[op.Path] = true
			paths = append(paths, op.Path)
		}
	}
	return paths, nil
}

type Patch struct {
	Ops []Operation
}

type Operation struct {
	Kind   string
	Path   string
	Lines  []string
	Chunks []Chunk
}

type Chunk struct {
	Old []string
	New []string
}

func Parse(input string) (Patch, error) {
	input = strings.ReplaceAll(input, "\r\n", "\n")
	lines := strings.Split(input, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		return Patch{}, fmt.Errorf("first non-empty line must be '*** Begin Patch'")
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "*** End Patch" {
		return Patch{}, fmt.Errorf("last non-empty line must be '*** End Patch'")
	}

	var patch Patch
	for i := 1; i < len(lines)-1; {
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			if path == "" {
				return Patch{}, fmt.Errorf("add hunk at line %d has empty path", i+1)
			}
			op := Operation{Kind: "add", Path: path}
			i++
			for i < len(lines)-1 && !strings.HasPrefix(strings.TrimSpace(lines[i]), "*** ") {
				if !strings.HasPrefix(lines[i], "+") {
					return Patch{}, fmt.Errorf("add hunk for %s line %d must start with '+'", path, i+1)
				}
				op.Lines = append(op.Lines, strings.TrimPrefix(lines[i], "+"))
				i++
			}
			if len(op.Lines) == 0 {
				return Patch{}, fmt.Errorf("add hunk for %s is empty", path)
			}
			patch.Ops = append(patch.Ops, op)

		case strings.HasPrefix(line, "*** Delete File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			if path == "" {
				return Patch{}, fmt.Errorf("delete hunk at line %d has empty path", i+1)
			}
			patch.Ops = append(patch.Ops, Operation{Kind: "delete", Path: path})
			i++

		case strings.HasPrefix(line, "*** Update File: "):
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			if path == "" {
				return Patch{}, fmt.Errorf("update hunk at line %d has empty path", i+1)
			}
			op := Operation{Kind: "update", Path: path}
			i++
			var chunk Chunk
			flush := func() {
				if len(chunk.Old) > 0 || len(chunk.New) > 0 {
					op.Chunks = append(op.Chunks, chunk)
				}
				chunk = Chunk{}
			}
			for i < len(lines)-1 && !strings.HasPrefix(strings.TrimSpace(lines[i]), "*** ") {
				raw := lines[i]
				trimmed := strings.TrimSpace(raw)
				if trimmed == "@@" || strings.HasPrefix(trimmed, "@@ ") {
					flush()
					i++
					continue
				}
				if raw == "" {
					chunk.Old = append(chunk.Old, "")
					chunk.New = append(chunk.New, "")
					i++
					continue
				}
				switch raw[0] {
				case ' ':
					line := raw[1:]
					chunk.Old = append(chunk.Old, line)
					chunk.New = append(chunk.New, line)
				case '-':
					chunk.Old = append(chunk.Old, raw[1:])
				case '+':
					chunk.New = append(chunk.New, raw[1:])
				default:
					return Patch{}, malformedUpdateLine(path, i+1, raw)
				}
				i++
			}
			flush()
			if len(op.Chunks) == 0 {
				return Patch{}, fmt.Errorf("update hunk for %s is empty", path)
			}
			patch.Ops = append(patch.Ops, op)

		default:
			return Patch{}, fmt.Errorf("unexpected line %d: %s", i+1, lines[i])
		}
	}
	return patch, nil
}

func malformedUpdateLine(path string, lineNo int, line string) error {
	display := line
	if display == "" {
		display = "<empty line>"
	}
	if len(display) > 120 {
		display = display[:120] + "..."
	}
	return fmt.Errorf("update hunk for %s line %d is malformed: %q\nEvery update hunk line must start with ' ', '+', '-', or '@@'. Unchanged context lines need a leading space. Remove stray '***' lines inside update hunks and reread the target range before retrying", path, lineNo, display)
}

type verifiedChange struct {
	op         Operation
	absPath    string
	oldContent string
	newContent string
}

func verify(patch Patch, workDir string) ([]verifiedChange, error) {
	current := map[string]string{}
	existsByPath := map[string]bool{}
	var changes []verifiedChange
	for _, op := range patch.Ops {
		absPath, err := resolvePath(workDir, op.Path)
		if err != nil {
			return nil, err
		}
		oldContent, ok := current[absPath]
		exists := existsByPath[absPath]
		if !ok {
			data, readErr := os.ReadFile(absPath)
			if readErr != nil && !os.IsNotExist(readErr) {
				return nil, fmt.Errorf("read %s: %w", op.Path, readErr)
			}
			if readErr == nil {
				oldContent = strings.ReplaceAll(string(data), "\r\n", "\n")
				exists = true
			}
		}

		nextContent, nextExists, err := verifyOperation(op, oldContent, exists)
		if err != nil {
			return nil, err
		}
		if nextContent == oldContent && op.Kind != "delete" {
			return nil, fmt.Errorf("%s produced no content change", op.Path)
		}
		current[absPath] = nextContent
		existsByPath[absPath] = nextExists
		changes = append(changes, verifiedChange{
			op:         op,
			absPath:    absPath,
			oldContent: oldContent,
			newContent: nextContent,
		})
	}
	return changes, nil
}

func verifyOperation(op Operation, oldContent string, exists bool) (string, bool, error) {
	switch op.Kind {
	case "add":
		if exists {
			return "", false, fmt.Errorf("add target already exists: %s", op.Path)
		}
		return joinPatchLines(op.Lines), true, nil
	case "delete":
		if !exists {
			return "", false, fmt.Errorf("delete target does not exist: %s", op.Path)
		}
		return "", false, nil
	case "update":
		if !exists {
			return "", false, fmt.Errorf("update target does not exist: %s", op.Path)
		}
		next, err := applyChunks(oldContent, op.Chunks, op.Path)
		if err != nil {
			return "", false, err
		}
		return next, true, nil
	default:
		return "", false, fmt.Errorf("unknown operation %q for %s", op.Kind, op.Path)
	}
}

func applyChunks(content string, chunks []Chunk, displayPath string) (string, error) {
	lines, trailing := splitContentLines(content)
	cursor := 0
	for _, chunk := range chunks {
		if len(chunk.Old) == 0 {
			return "", fmt.Errorf("update hunk for %s has no context or removed lines", displayPath)
		}
		idx := findSubsequence(lines, chunk.Old, cursor)
		if idx < 0 {
			return "", fmt.Errorf("update hunk for %s did not match current file content near:\n%s\nReread the target range, keep unchanged context lines exact, and retry with a smaller hunk if needed", displayPath, formatPatchExcerpt(chunk.Old))
		}
		next := make([]string, 0, len(lines)-len(chunk.Old)+len(chunk.New))
		next = append(next, lines[:idx]...)
		next = append(next, chunk.New...)
		next = append(next, lines[idx+len(chunk.Old):]...)
		lines = next
		cursor = idx + len(chunk.New)
	}
	return joinContentLines(lines, trailing), nil
}

func formatPatchExcerpt(lines []string) string {
	if len(lines) == 0 {
		return "<empty hunk>"
	}
	if len(lines) > 6 {
		lines = lines[:6]
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func findSubsequence(lines []string, needle []string, start int) int {
	if len(needle) == 0 || len(needle) > len(lines) {
		return -1
	}
	for i := start; i <= len(lines)-len(needle); i++ {
		match := true
		for j := range needle {
			if lines[i+j] != needle[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func splitContentLines(content string) ([]string, bool) {
	if content == "" {
		return nil, false
	}
	trailing := strings.HasSuffix(content, "\n")
	if trailing {
		content = strings.TrimSuffix(content, "\n")
	}
	if content == "" {
		return []string{""}, trailing
	}
	return strings.Split(content, "\n"), trailing
}

func joinContentLines(lines []string, trailing bool) string {
	out := strings.Join(lines, "\n")
	if trailing {
		out += "\n"
	}
	return out
}

func joinPatchLines(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func writeVerified(ctx context.Context, changes []verifiedChange, state tool.StateSnapshot) error {
	for _, change := range changes {
		observe.TraceCtx(ctx, "applypatch", "writeVerified", change.absPath)
		if change.op.Kind == "delete" {
			if err := os.Remove(change.absPath); err != nil {
				return fmt.Errorf("delete %s: %w", change.op.Path, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(change.absPath), 0755); err != nil {
			return fmt.Errorf("create parent directory for %s: %w", change.op.Path, err)
		}
		if err := os.WriteFile(change.absPath, []byte(change.newContent), 0644); err != nil {
			return fmt.Errorf("write %s: %w", change.op.Path, err)
		}
		if timestamp, statErr := tool.FileTimestamp(change.absPath); statErr == nil {
			tool.RecordFileState(state, change.absPath, change.newContent, timestamp, nil, nil, false)
		}
	}
	return nil
}

func renderResult(changes []verifiedChange) tool.InvokeResult {
	var display strings.Builder
	for _, change := range changes {
		if display.Len() > 0 {
			display.WriteString("\n")
		}
		display.WriteString(util.GenerateEditDiff(change.oldContent, change.oldContent, change.newContent, change.op.Path, false, 3))
	}
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		paths = append(paths, change.op.Path)
	}
	return tool.InvokeResult{
		Content: fmt.Sprintf("Applied patch successfully. Changed files: %s", strings.Join(paths, ", ")),
		Display: display.String(),
	}
}

func resolvePath(workDir, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("empty patch path")
	}
	if filepath.IsAbs(path) {
		clean := filepath.Clean(path)
		if clean == string(filepath.Separator) {
			return "", fmt.Errorf("refusing root path")
		}
		return clean, nil
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("patch path escapes working directory: %s", path)
	}
	return filepath.Join(workDir, clean), nil
}

func ExtractShellApplyPatch(command string) (patch string, workDir string, ok bool, err error) {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return "", "", false, nil
	}
	if strings.HasPrefix(trimmed, "cd ") {
		dirPart, rest, found := strings.Cut(strings.TrimPrefix(trimmed, "cd "), "&&")
		if !found {
			return "", "", false, fmt.Errorf("cd before apply_patch must be followed by && apply_patch")
		}
		dir, parseErr := parseShellPath(dirPart)
		if parseErr != nil {
			return "", "", false, parseErr
		}
		workDir = dir
		trimmed = strings.TrimSpace(rest)
	}
	if !strings.HasPrefix(trimmed, "apply_patch") && !strings.HasPrefix(trimmed, "applypatch") {
		if strings.Contains(trimmed, "apply_patch") || strings.Contains(trimmed, "applypatch") {
			return "", "", false, fmt.Errorf("mixed shell apply_patch commands are not allowed; use the ApplyPatch tool or a single apply_patch heredoc")
		}
		return "", "", false, nil
	}
	body, parseErr := extractHeredoc(trimmed)
	if parseErr != nil {
		return "", "", false, parseErr
	}
	return body, workDir, true, nil
}

func extractHeredoc(command string) (string, error) {
	firstLine, rest, found := strings.Cut(command, "\n")
	if !found {
		return "", fmt.Errorf("apply_patch shell command must use a heredoc")
	}
	fields := strings.Fields(firstLine)
	if len(fields) != 2 || (fields[0] != "apply_patch" && fields[0] != "applypatch") || !strings.HasPrefix(fields[1], "<<") {
		return "", fmt.Errorf("apply_patch shell command must be exactly: apply_patch <<'EOF'")
	}
	delim := strings.TrimPrefix(fields[1], "<<")
	delim = strings.Trim(delim, `"'`)
	if delim == "" {
		return "", fmt.Errorf("apply_patch heredoc delimiter is empty")
	}
	lines := strings.Split(rest, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == delim {
			tail := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			if tail != "" {
				return "", fmt.Errorf("apply_patch heredoc must not have trailing shell commands")
			}
			return strings.Join(lines[:i], "\n"), nil
		}
	}
	return "", fmt.Errorf("apply_patch heredoc missing closing delimiter %q", delim)
}

func parseShellPath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", fmt.Errorf("cd path before apply_patch is empty")
	}
	if strings.ContainsAny(input, ";&|`$<>") {
		return "", fmt.Errorf("cd path before apply_patch must be a simple path")
	}
	if (strings.HasPrefix(input, `"`) && strings.HasSuffix(input, `"`)) ||
		(strings.HasPrefix(input, `'`) && strings.HasSuffix(input, `'`)) {
		input = input[1 : len(input)-1]
	}
	return input, nil
}
