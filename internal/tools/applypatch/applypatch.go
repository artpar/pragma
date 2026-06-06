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
	observe.GlobalTrace("return: ToolName")
	return ToolName
}

func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: description")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: LegacyToolName")
	return LegacyToolName
}

func (t *Tool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: inputSchema")
	return inputSchema
}

func (t *Tool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: false, Concurrent: false}")
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "applypatch", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "applypatch", "Tool.CheckPerm", "exit")
	var in Input
	if err := json.Unmarshal(input, &in); err != nil || strings.TrimSpace(in.Patch) == "" {
		observe.TraceCtx(ctx, "applypatch", "Tool.CheckPerm", "if: err != nil || strings.TrimSpace(in.Patch) == \"\"")
		observe.TraceCtx(ctx, "applypatch", "Tool.CheckPerm", "return: checker.Check(ctx, ToolName, \"\")")
		return checker.Check(ctx, ToolName, "")
	}
	paths, err := PatchPaths(in.Patch)
	if err != nil {
		observe.TraceCtx(ctx, "applypatch", "Tool.CheckPerm", "if: err != nil")
		observe.TraceCtx(ctx, "applypatch", "Tool.CheckPerm", "return: checker.Check(ctx, ToolName, \"\")")
		return checker.Check(ctx, ToolName, "")
	}
	sort.Strings(paths)
	observe.TraceCtx(ctx, "applypatch", "Tool.CheckPerm", "return: checker.Check(ctx, ToolName, strings.Join(paths, \"\\n\"))")
	return checker.Check(ctx, ToolName, strings.Join(paths, "\n"))
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "applypatch", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "applypatch", "Tool.Invoke", "exit")
	var in Input
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "applypatch", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "applypatch", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	observe.TraceCtx(ctx, "applypatch", "Tool.Invoke", "return: ApplyPatchText(ctx, in.Patch, state.WorkDir(), state)")
	return ApplyPatchText(ctx, in.Patch, state.WorkDir(), state)
}

func ApplyPatchText(ctx context.Context, patch string, workDir string, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "enter")
	defer observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "exit")
	parsed, err := Parse(patch)
	if err != nil {
		observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "if: err != nil")
		observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "return: tool.InvokeResult{}, fmt.Errorf(\"apply_patch verification failed: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("apply_patch verification failed: %w", err)
	}
	if len(parsed.Ops) == 0 {
		observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "if: len(parsed.Ops) == 0")
		observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "return: tool.InvokeResult{}, fmt.Errorf(\"apply_patch verification failed: patch conta...")
		return tool.InvokeResult{}, fmt.Errorf("apply_patch verification failed: patch contains no file hunks")
	}
	verified, err := verify(parsed, workDir)
	if err != nil {
		observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "if: err != nil")
		observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "return: tool.InvokeResult{}, fmt.Errorf(\"apply_patch verification failed: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("apply_patch verification failed: %w", err)
	}
	if err := writeVerified(ctx, verified, state); err != nil {
		observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "if: err != nil")
		observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}
	observe.TraceCtx(ctx, "applypatch", "ApplyPatchText", "return: renderResult(verified), nil")
	return renderResult(verified), nil
}

func PatchPaths(patch string) ([]string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	parsed, err := Parse(patch)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	seen := map[string]bool{}
	var paths []string
	for _, op := range parsed.Ops {
		observe.GlobalTrace("range parsed.Ops")
		if !seen[op.Path] {
			observe.GlobalTrace("if: !seen[op.Path]")
			seen[op.Path] = true
			paths = append(paths, op.Path)
		}
	}
	observe.GlobalTrace("return: paths, nil")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	input = strings.ReplaceAll(input, "\r\n", "\n")
	lines := strings.Split(input, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		observe.GlobalTrace("for: len(lines) > 0 && strings.TrimSpace(lines[0]) == \"\"")
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		observe.GlobalTrace("for: len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == \"\"")
		lines = lines[:len(lines)-1]
	}
	if len(lines) < 2 || strings.TrimSpace(lines[0]) != "*** Begin Patch" {
		observe.GlobalTrace("if: len(lines) < 2 || strings.TrimSpace(lines[0]) != \"*** Begin Patch\"")
		observe.GlobalTrace("return: Patch{}, fmt.Errorf(\"first non-empty line must be '*** Begin Patch'\")")
		return Patch{}, fmt.Errorf("first non-empty line must be '*** Begin Patch'")
	}
	if strings.TrimSpace(lines[len(lines)-1]) != "*** End Patch" {
		observe.GlobalTrace("if: strings.TrimSpace(lines[len(lines)-1]) != \"*** End Patch\"")
		observe.GlobalTrace("return: Patch{}, fmt.Errorf(\"last non-empty line must be '*** End Patch'\")")
		return Patch{}, fmt.Errorf("last non-empty line must be '*** End Patch'")
	}

	var patch Patch
	for i := 1; i < len(lines)-1; {
		observe.GlobalTrace("for: i < len(lines)-1")
		line := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(line, "*** Add File: "):
			observe.GlobalTrace("case: strings.HasPrefix(line, \"*** Add File: \")")
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Add File: "))
			if path == "" {
				observe.GlobalTrace("return: Patch{}, fmt.Errorf(\"add hunk at line %d has empty path\", i+1)")
				return Patch{}, fmt.Errorf("add hunk at line %d has empty path", i+1)
			}
			op := Operation{Kind: "add", Path: path}
			i++
			for i < len(lines)-1 && !strings.HasPrefix(strings.TrimSpace(lines[i]), "*** ") {
				if !strings.HasPrefix(lines[i], "+") {
					observe.GlobalTrace("if: !strings.HasPrefix(lines[i], \"+\")")
					observe.GlobalTrace("return: Patch{}, fmt.Errorf(\"add hunk for %s line %d must start with '+'\", path, i+1)")
					return Patch{}, fmt.Errorf("add hunk for %s line %d must start with '+'", path, i+1)
				}
				op.Lines = append(op.Lines, strings.TrimPrefix(lines[i], "+"))
				i++
			}
			if len(op.Lines) == 0 {
				observe.GlobalTrace("return: Patch{}, fmt.Errorf(\"add hunk for %s is empty\", path)")
				return Patch{}, fmt.Errorf("add hunk for %s is empty", path)
			}
			patch.Ops = append(patch.Ops, op)

		case strings.HasPrefix(line, "*** Delete File: "):
			observe.GlobalTrace("case: strings.HasPrefix(line, \"*** Delete File: \")")
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Delete File: "))
			if path == "" {
				observe.GlobalTrace("return: Patch{}, fmt.Errorf(\"delete hunk at line %d has empty path\", i+1)")
				return Patch{}, fmt.Errorf("delete hunk at line %d has empty path", i+1)
			}
			patch.Ops = append(patch.Ops, Operation{Kind: "delete", Path: path})
			i++

		case strings.HasPrefix(line, "*** Update File: "):
			observe.GlobalTrace("case: strings.HasPrefix(line, \"*** Update File: \")")
			path := strings.TrimSpace(strings.TrimPrefix(line, "*** Update File: "))
			if path == "" {
				observe.GlobalTrace("return: Patch{}, fmt.Errorf(\"update hunk at line %d has empty path\", i+1)")
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
					observe.GlobalTrace("if: trimmed == \"@@\" || strings.HasPrefix(trimmed, \"@@ \")")
					flush()
					i++
					continue
				}
				if raw == "" {
					observe.GlobalTrace("if: raw == \"\"")
					chunk.Old = append(chunk.Old, "")
					chunk.New = append(chunk.New, "")
					i++
					continue
				}
				switch raw[0] {
				case ' ':
					observe.GlobalTrace("case: ' '")
					line := raw[1:]
					chunk.Old = append(chunk.Old, line)
					chunk.New = append(chunk.New, line)
				case '-':
					observe.GlobalTrace("case: '-'")
					chunk.Old = append(chunk.Old, raw[1:])
				case '+':
					observe.GlobalTrace("case: '+'")
					chunk.New = append(chunk.New, raw[1:])
				default:
					observe.GlobalTrace("default")
					return Patch{}, malformedUpdateLine(path, i+1, raw)
				}
				i++
			}
			flush()
			if len(op.Chunks) == 0 {
				observe.GlobalTrace("return: Patch{}, fmt.Errorf(\"update hunk for %s is empty\", path)")
				return Patch{}, fmt.Errorf("update hunk for %s is empty", path)
			}
			patch.Ops = append(patch.Ops, op)

		default:
			observe.GlobalTrace("default")
			return Patch{}, fmt.Errorf("unexpected line %d: %s", i+1, lines[i])
		}
	}
	observe.GlobalTrace("return: patch, nil")
	return patch, nil
}

func malformedUpdateLine(path string, lineNo int, line string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	display := line
	if display == "" {
		observe.GlobalTrace("if: display == \"\"")
		display = "<empty line>"
	}
	if len(display) > 120 {
		observe.GlobalTrace("if: len(display) > 120")
		display = display[:120] + "..."
	}
	observe.GlobalTrace("return: fmt.Errorf(\"update hunk for %s line %d is malformed: %q\\nEvery update hunk li...")
	return fmt.Errorf("update hunk for %s line %d is malformed: %q\nEvery update hunk line must start with ' ', '+', '-', or '@@'. Unchanged context lines need a leading space. Remove stray '***' lines inside update hunks and reread the target range before retrying", path, lineNo, display)
}

type verifiedChange struct {
	op         Operation
	absPath    string
	oldContent string
	newContent string
}

func verify(patch Patch, workDir string) ([]verifiedChange, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	current := map[string]string{}
	existsByPath := map[string]bool{}
	var changes []verifiedChange
	for _, op := range patch.Ops {
		observe.GlobalTrace("range patch.Ops")
		absPath, err := resolvePath(workDir, op.Path)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		oldContent, ok := current[absPath]
		exists := existsByPath[absPath]
		if !ok {
			observe.GlobalTrace("if: !ok")
			data, readErr := os.ReadFile(absPath)
			if readErr != nil && !os.IsNotExist(readErr) {
				observe.GlobalTrace("if: readErr != nil && !os.IsNotExist(readErr)")
				observe.GlobalTrace("return: nil, fmt.Errorf(\"read %s: %w\", op.Path, readErr)")
				return nil, fmt.Errorf("read %s: %w", op.Path, readErr)
			}
			if readErr == nil {
				observe.GlobalTrace("if: readErr == nil")
				oldContent = strings.ReplaceAll(string(data), "\r\n", "\n")
				exists = true
			}
		}

		nextContent, nextExists, err := verifyOperation(op, oldContent, exists)
		if err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, err")
			return nil, err
		}
		if nextContent == oldContent && op.Kind != "delete" {
			observe.GlobalTrace("if: nextContent == oldContent && op.Kind != \"delete\"")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"%s produced no content change\", op.Path)")
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
	observe.GlobalTrace("return: changes, nil")
	return changes, nil
}

func verifyOperation(op Operation, oldContent string, exists bool) (string, bool, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch op.Kind {
	case "add":
		observe.GlobalTrace("case: \"add\"")
		if exists {
			observe.GlobalTrace("return: \"\", false, fmt.Errorf(\"add target already exists: %s\", op.Path)")
			return "", false, fmt.Errorf("add target already exists: %s", op.Path)
		}
		return joinPatchLines(op.Lines), true, nil
	case "delete":
		observe.GlobalTrace("case: \"delete\"")
		if !exists {
			observe.GlobalTrace("return: \"\", false, fmt.Errorf(\"delete target does not exist: %s\", op.Path)")
			return "", false, fmt.Errorf("delete target does not exist: %s", op.Path)
		}
		return "", false, nil
	case "update":
		observe.GlobalTrace("case: \"update\"")
		if !exists {
			observe.GlobalTrace("return: \"\", false, fmt.Errorf(\"update target does not exist: %s\", op.Path)")
			return "", false, fmt.Errorf("update target does not exist: %s", op.Path)
		}
		next, err := applyChunks(oldContent, op.Chunks, op.Path)
		if err != nil {
			observe.GlobalTrace("return: \"\", false, err")
			return "", false, err
		}
		return next, true, nil
	default:
		observe.GlobalTrace("default")
		return "", false, fmt.Errorf("unknown operation %q for %s", op.Kind, op.Path)
	}
}

func applyChunks(content string, chunks []Chunk, displayPath string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lines, trailing := splitContentLines(content)
	cursor := 0
	for _, chunk := range chunks {
		observe.GlobalTrace("range chunks")
		if len(chunk.Old) == 0 {
			observe.GlobalTrace("if: len(chunk.Old) == 0")
			observe.GlobalTrace("return: \"\", fmt.Errorf(\"update hunk for %s has no context or removed lines\", displayP...")
			return "", fmt.Errorf("update hunk for %s has no context or removed lines", displayPath)
		}
		idx := findSubsequence(lines, chunk.Old, cursor)
		if idx < 0 {
			observe.GlobalTrace("if: idx < 0")
			observe.GlobalTrace("return: \"\", fmt.Errorf(\"update hunk for %s did not match current file content near:\\n...")
			return "", fmt.Errorf("update hunk for %s did not match current file content near:\n%s\nReread the target range, keep unchanged context lines exact, and retry with a smaller hunk if needed", displayPath, formatPatchExcerpt(chunk.Old))
		}
		next := make([]string, 0, len(lines)-len(chunk.Old)+len(chunk.New))
		next = append(next, lines[:idx]...)
		next = append(next, chunk.New...)
		next = append(next, lines[idx+len(chunk.Old):]...)
		lines = next
		cursor = idx + len(chunk.New)
	}
	observe.GlobalTrace("return: joinContentLines(lines, trailing), nil")
	return joinContentLines(lines, trailing), nil
}

func formatPatchExcerpt(lines []string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(lines) == 0 {
		observe.GlobalTrace("if: len(lines) == 0")
		observe.GlobalTrace("return: \"<empty hunk>\"")
		return "<empty hunk>"
	}
	if len(lines) > 6 {
		observe.GlobalTrace("if: len(lines) > 6")
		lines = lines[:6]
	}
	var b strings.Builder
	for _, line := range lines {
		observe.GlobalTrace("range lines")
		b.WriteString("  ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	observe.GlobalTrace("return: strings.TrimRight(b.String(), \"\\n\")")
	return strings.TrimRight(b.String(), "\n")
}

func findSubsequence(lines []string, needle []string, start int) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(needle) == 0 || len(needle) > len(lines) {
		observe.GlobalTrace("if: len(needle) == 0 || len(needle) > len(lines)")
		observe.GlobalTrace("return: -1")
		return -1
	}
	for i := start; i <= len(lines)-len(needle); i++ {
		observe.GlobalTrace("for: i <= len(lines)-len(needle)")
		match := true
		for j := range needle {
			observe.GlobalTrace("range needle")
			if lines[i+j] != needle[j] {
				observe.GlobalTrace("if: lines[i+j] != needle[j]")
				match = false
				break
			}
		}
		if match {
			observe.GlobalTrace("if: match")
			observe.GlobalTrace("return: i")
			return i
		}
	}
	observe.GlobalTrace("return: -1")
	return -1
}

func splitContentLines(content string) ([]string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if content == "" {
		observe.GlobalTrace("if: content == \"\"")
		observe.GlobalTrace("return: nil, false")
		return nil, false
	}
	trailing := strings.HasSuffix(content, "\n")
	if trailing {
		observe.GlobalTrace("if: trailing")
		content = strings.TrimSuffix(content, "\n")
	}
	if content == "" {
		observe.GlobalTrace("if: content == \"\"")
		observe.GlobalTrace("return: []string{\"\"}, trailing")
		return []string{""}, trailing
	}
	observe.GlobalTrace("return: strings.Split(content, \"\\n\"), trailing")
	return strings.Split(content, "\n"), trailing
}

func joinContentLines(lines []string, trailing bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := strings.Join(lines, "\n")
	if trailing {
		observe.GlobalTrace("if: trailing")
		out += "\n"
	}
	observe.GlobalTrace("return: out")
	return out
}

func joinPatchLines(lines []string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if len(lines) == 0 {
		observe.GlobalTrace("if: len(lines) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: strings.Join(lines, \"\\n\") + \"\\n\"")
	return strings.Join(lines, "\n") + "\n"
}

func writeVerified(ctx context.Context, changes []verifiedChange, state tool.StateSnapshot) error {
	observe.TraceCtx(ctx, "applypatch", "writeVerified", "enter")
	defer observe.TraceCtx(ctx, "applypatch", "writeVerified", "exit")
	for _, change := range changes {
		observe.TraceCtx(ctx, "applypatch", "writeVerified", change.absPath)
		if change.op.Kind == "delete" {
			observe.TraceCtx(ctx, "applypatch", "writeVerified", "if: change.op.Kind == \"delete\"")
			if err := os.Remove(change.absPath); err != nil {
				observe.TraceCtx(ctx, "applypatch", "writeVerified", "if: err != nil")
				observe.TraceCtx(ctx, "applypatch", "writeVerified", "return: fmt.Errorf(\"delete %s: %w\", change.op.Path, err)")
				return fmt.Errorf("delete %s: %w", change.op.Path, err)
			}
			tool.RecordFileDelete(state, change.absPath)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(change.absPath), 0755); err != nil {
			observe.TraceCtx(ctx, "applypatch", "writeVerified", "if: err != nil")
			observe.TraceCtx(ctx, "applypatch", "writeVerified", "return: fmt.Errorf(\"create parent directory for %s: %w\", change.op.Path, err)")
			return fmt.Errorf("create parent directory for %s: %w", change.op.Path, err)
		}
		if err := os.WriteFile(change.absPath, []byte(change.newContent), 0644); err != nil {
			observe.TraceCtx(ctx, "applypatch", "writeVerified", "if: err != nil")
			observe.TraceCtx(ctx, "applypatch", "writeVerified", "return: fmt.Errorf(\"write %s: %w\", change.op.Path, err)")
			return fmt.Errorf("write %s: %w", change.op.Path, err)
		}
		if timestamp, statErr := tool.FileTimestamp(change.absPath); statErr == nil {
			observe.TraceCtx(ctx, "applypatch", "writeVerified", "if: statErr == nil")
			tool.RecordFileWriteState(state, change.absPath, change.newContent, timestamp, nil, nil, false)
		}
	}
	observe.TraceCtx(ctx, "applypatch", "writeVerified", "return: nil")
	return nil
}

func renderResult(changes []verifiedChange) tool.InvokeResult {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	paths := make([]string, 0, len(changes))
	for _, change := range changes {
		observe.GlobalTrace("range changes")
		paths = append(paths, change.op.Path)
	}
	observe.GlobalTrace("return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"Applied patch successfully. Changed...")
	return tool.InvokeResult{
		Content: fmt.Sprintf("Applied patch successfully. Changed files: %s", strings.Join(paths, ", ")),
	}
}

func resolvePath(workDir, path string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.TrimSpace(path) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(path) == \"\"")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"empty patch path\")")
		return "", fmt.Errorf("empty patch path")
	}
	if filepath.IsAbs(path) {
		observe.GlobalTrace("if: filepath.IsAbs(path)")
		clean := filepath.Clean(path)
		if clean == string(filepath.Separator) {
			observe.GlobalTrace("if: clean == string(filepath.Separator)")
			observe.GlobalTrace("return: \"\", fmt.Errorf(\"refusing root path\")")
			return "", fmt.Errorf("refusing root path")
		}
		observe.GlobalTrace("return: clean, nil")
		return clean, nil
	}
	clean := filepath.Clean(path)
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		observe.GlobalTrace("if: clean == \".\" || strings.HasPrefix(clean, \"..\"+string(filepath.Separator)) || ...")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"patch path escapes working directory: %s\", path)")
		return "", fmt.Errorf("patch path escapes working directory: %s", path)
	}
	observe.GlobalTrace("return: filepath.Join(workDir, clean), nil")
	return filepath.Join(workDir, clean), nil
}

func ExtractShellApplyPatch(command string) (patch string, workDir string, ok bool, err error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		observe.GlobalTrace("if: trimmed == \"\"")
		observe.GlobalTrace("return: \"\", \"\", false, nil")
		return "", "", false, nil
	}
	if strings.HasPrefix(trimmed, "cd ") {
		observe.GlobalTrace("if: strings.HasPrefix(trimmed, \"cd \")")
		dirPart, rest, found := strings.Cut(strings.TrimPrefix(trimmed, "cd "), "&&")
		if !found {
			observe.GlobalTrace("if: !found")
			observe.GlobalTrace("return: \"\", \"\", false, fmt.Errorf(\"cd before apply_patch must be followed by && apply...")
			return "", "", false, fmt.Errorf("cd before apply_patch must be followed by && apply_patch")
		}
		dir, parseErr := parseShellPath(dirPart)
		if parseErr != nil {
			observe.GlobalTrace("if: parseErr != nil")
			observe.GlobalTrace("return: \"\", \"\", false, parseErr")
			return "", "", false, parseErr
		}
		workDir = dir
		trimmed = strings.TrimSpace(rest)
	}
	if !strings.HasPrefix(trimmed, "apply_patch") && !strings.HasPrefix(trimmed, "applypatch") {
		observe.GlobalTrace("if: !strings.HasPrefix(trimmed, \"apply_patch\") && !strings.HasPrefix(trimmed, \"ap...")
		if strings.Contains(trimmed, "apply_patch") || strings.Contains(trimmed, "applypatch") {
			observe.GlobalTrace("if: strings.Contains(trimmed, \"apply_patch\") || strings.Contains(trimmed, \"applyp...")
			observe.GlobalTrace("return: \"\", \"\", false, fmt.Errorf(\"mixed shell apply_patch commands are not allowed; ...")
			return "", "", false, fmt.Errorf("mixed shell apply_patch commands are not allowed; use the ApplyPatch tool or a single apply_patch heredoc")
		}
		observe.GlobalTrace("return: \"\", \"\", false, nil")
		return "", "", false, nil
	}
	body, parseErr := extractHeredoc(trimmed)
	if parseErr != nil {
		observe.GlobalTrace("if: parseErr != nil")
		observe.GlobalTrace("return: \"\", \"\", false, parseErr")
		return "", "", false, parseErr
	}
	observe.GlobalTrace("return: body, workDir, true, nil")
	return body, workDir, true, nil
}

func extractHeredoc(command string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	firstLine, rest, found := strings.Cut(command, "\n")
	if !found {
		observe.GlobalTrace("if: !found")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"apply_patch shell command must use a heredoc\")")
		return "", fmt.Errorf("apply_patch shell command must use a heredoc")
	}
	fields := strings.Fields(firstLine)
	if len(fields) != 2 || (fields[0] != "apply_patch" && fields[0] != "applypatch") || !strings.HasPrefix(fields[1], "<<") {
		observe.GlobalTrace("if: len(fields) != 2 || (fields[0] != \"apply_patch\" && fields[0] != \"applypatch\")...")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"apply_patch shell command must be exactly: apply_patch <<'EOF'\")")
		return "", fmt.Errorf("apply_patch shell command must be exactly: apply_patch <<'EOF'")
	}
	delim := strings.TrimPrefix(fields[1], "<<")
	delim = strings.Trim(delim, `"'`)
	if delim == "" {
		observe.GlobalTrace("if: delim == \"\"")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"apply_patch heredoc delimiter is empty\")")
		return "", fmt.Errorf("apply_patch heredoc delimiter is empty")
	}
	lines := strings.Split(rest, "\n")
	for i, line := range lines {
		observe.GlobalTrace("range lines")
		if strings.TrimSpace(line) == delim {
			observe.GlobalTrace("if: strings.TrimSpace(line) == delim")
			tail := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			if tail != "" {
				observe.GlobalTrace("if: tail != \"\"")
				observe.GlobalTrace("return: \"\", fmt.Errorf(\"apply_patch heredoc must not have trailing shell commands\")")
				return "", fmt.Errorf("apply_patch heredoc must not have trailing shell commands")
			}
			observe.GlobalTrace("return: strings.Join(lines[:i], \"\\n\"), nil")
			return strings.Join(lines[:i], "\n"), nil
		}
	}
	observe.GlobalTrace("return: \"\", fmt.Errorf(\"apply_patch heredoc missing closing delimiter %q\", delim)")
	return "", fmt.Errorf("apply_patch heredoc missing closing delimiter %q", delim)
}

func parseShellPath(input string) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	input = strings.TrimSpace(input)
	if input == "" {
		observe.GlobalTrace("if: input == \"\"")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"cd path before apply_patch is empty\")")
		return "", fmt.Errorf("cd path before apply_patch is empty")
	}
	if strings.ContainsAny(input, ";&|`$<>") {
		observe.GlobalTrace("if: strings.ContainsAny(input, \";&|`$<>\")")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"cd path before apply_patch must be a simple path\")")
		return "", fmt.Errorf("cd path before apply_patch must be a simple path")
	}
	if (strings.HasPrefix(input, `"`) && strings.HasSuffix(input, `"`)) ||
		(strings.HasPrefix(input, `'`) && strings.HasSuffix(input, `'`)) {
		observe.GlobalTrace("if: (strings.HasPrefix(input, `\"`) && strings.HasSuffix(input, `\"`)) ||\n\t(strings...")
		input = input[1 : len(input)-1]
	}
	observe.GlobalTrace("return: input, nil")
	return input, nil
}
