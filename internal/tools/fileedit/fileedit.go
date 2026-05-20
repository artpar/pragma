package fileedit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/util"
)

const maxEditFileSize = 1024 * 1024 * 1024 // 1 GiB

const (
	leftSingleCurlyQuote  = "‘"
	rightSingleCurlyQuote = "’"
	leftDoubleCurlyQuote  = "“"
	rightDoubleCurlyQuote = "”"
)

type stringReplacement struct {
	from string
	to   string
}

var desanitizations = []stringReplacement{
	{"<fnr>", "<function_results>"},
	{"<n>", "<name>"},
	{"</n>", "</name>"},
	{"<o>", "<output>"},
	{"</o>", "</output>"},
	{"<e>", "<error>"},
	{"</e>", "</error>"},
	{"<s>", "<system>"},
	{"</s>", "</system>"},
	{"<r>", "<result>"},
	{"</r>", "</result>"},
	{"< META_START >", "<META_START>"},
	{"< META_END >", "<META_END>"},
	{"< EOT >", "<EOT>"},
	{"< META >", "<META>"},
	{"< SOS >", "<SOS>"},
	{"\n\nH:", "\n\nHuman:"},
	{"\n\nA:", "\n\nAssistant:"},
}

// FileEditInput defines the parameters for the FileEdit tool.
type FileEditInput struct {
	FilePath   string `json:"file_path" desc:"The path to the file to edit"`
	OldString  string `json:"old_string" desc:"The exact string to find and replace"`
	NewString  string `json:"new_string" desc:"The replacement string"`
	ReplaceAll bool   `json:"replace_all,omitempty" desc:"If true, replace all occurrences. Default false."`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["file_path", "old_string", "new_string"],
	"properties": {
		"file_path": {
			"type": "string",
			"description": "The path to the file to edit"
		},
		"old_string": {
			"type": "string",
			"description": "The exact string to find and replace"
		},
		"new_string": {
			"type": "string",
			"description": "The replacement string"
		},
		"replace_all": {
			"type": "boolean",
			"description": "If true, replace all occurrences. Default false.",
			"default": false
		}
	}
}`)

// Tool implements the FileEdit tool.
type Tool struct {
	PatchMode bool
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Edit\"")
	return "Edit"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Performs exact string replacements in files...\"")
	observe.GlobalTrace("return: editDescription")
	return editDescription
}

const editDescription = `Performs exact string replacements in files.

Usage:
- You must use an available file-reading capability at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file.
- When editing text from file-read output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the → arrow. The line number prefix format is: spaces + line number + →. Everything after the → is the actual file content to match. Never include the line number or → in old_string or new_string.
- Use this only for small single-site exact replacements. When apply_patch is available, use apply_patch for multi-line or multi-file code edits.
- Prefer editing existing files in the codebase. NEVER write new files unless explicitly required.
- Only use emojis if the user explicitly requests it. Avoid adding emojis to files unless asked.
- The edit will FAIL if ` + "`old_string`" + ` is not unique in the file. Either provide a larger string with more surrounding context to make it unique or use ` + "`replace_all`" + ` to change every instance of ` + "`old_string`" + `.
- Use ` + "`replace_all`" + ` for replacing and renaming strings across the file. This parameter is useful if you want to rename a variable for instance.`

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
	observe.TraceCtx(ctx, "fileedit", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "fileedit", "Tool.CheckPerm", "exit")
	var in struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.FilePath == "" {
		observe.TraceCtx(ctx, "fileedit", "Tool.CheckPerm", "if: err != nil || in.FilePath == \"\"")
		observe.TraceCtx(ctx, "fileedit", "Tool.CheckPerm", "return: checker.Check(ctx, \"Edit\", \"\")")
		return checker.Check(ctx, "Edit", "")
	}
	observe.TraceCtx(ctx, "fileedit", "Tool.CheckPerm", "return: checker.Check(ctx, \"Edit\", in.FilePath)")
	return checker.Check(ctx, "Edit", in.FilePath)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "exit")
	var in FileEditInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.FilePath == "" {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: in.FilePath == \"\"")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file_path is required\")")
		return tool.InvokeResult{}, fmt.Errorf("file_path is required")
	}
	if t.PatchMode && isMultilineEdit(in) {
		return tool.InvokeResult{}, fmt.Errorf("Edit rejected: use apply_patch for multi-line edits when apply_patch is available. Keep Edit for one-line exact replacements only")
	}

	filePath := util.ExpandPath(in.FilePath, state.WorkDir())
	in.NewString = normalizeNewString(filePath, in.NewString)

	if in.OldString == "" && in.NewString == "" {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: in.OldString == \"\" && in.NewString == \"\"")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"missing required fields 'old_string' and 'ne...")
		return tool.InvokeResult{}, fmt.Errorf("missing required fields 'old_string' and 'new_string': Edit requires 'file_path', 'old_string' (text to find), and 'new_string' (replacement). Do not use 'operations' or other formats")
	}

	if in.OldString == in.NewString {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: in.OldString == in.NewString")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"no changes to make: old_string and new_strin...")
		return tool.InvokeResult{}, fmt.Errorf("no changes to make: old_string and new_string are exactly the same")
	}

	if strings.HasSuffix(strings.ToLower(filePath), ".ipynb") {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: strings.HasSuffix(strings.ToLower(filePath), \".ipynb\")")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file is a Jupyter Notebook. Use the Notebook...")
		return tool.InvokeResult{}, fmt.Errorf("file is a Jupyter Notebook. Use the NotebookEdit tool to edit this file")
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
		if os.IsNotExist(err) {
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: os.IsNotExist(err)")
			result, err := handleNonexistentFile(filePath, in)
			if err != nil {
				observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
				observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, err")
				return tool.InvokeResult{}, err
			}
			display := util.GenerateEditDiff("", in.OldString, in.NewString, in.FilePath, false, 3)
			if timestamp, statErr := tool.FileTimestamp(filePath); statErr == nil {
				observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: statErr == nil")
				tool.RecordFileState(state, filePath, tool.NormalizeTextContent(in.NewString), timestamp, nil, nil, false)
			}
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{Content: result, Display: display}, nil")
			return tool.InvokeResult{Content: result, Display: display}, nil
		}
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"read file: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("read file: %w", err)
	}

	if len(data) > maxEditFileSize {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: len(data) > maxEditFileSize")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file is too large to edit (%d bytes). Maximu...")
		return tool.InvokeResult{}, fmt.Errorf("file is too large to edit (%d bytes). Maximum editable file size is 1 GiB", len(data))
	}

	content := string(data)

	content = tool.NormalizeTextContent(content)

	if in.OldString != "" {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: in.OldString != \"\"")
		timestamp, err := tool.FileTimestamp(filePath)
		if err != nil {
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"stat file: %w\", err)")
			return tool.InvokeResult{}, fmt.Errorf("stat file: %w", err)
		}
		if err := tool.EnsureFileFreshForWrite(state, filePath, content, timestamp); err != nil {
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, err")
			return tool.InvokeResult{}, err
		}
	}

	if in.OldString == "" {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: in.OldString == \"\"")
		if strings.TrimSpace(content) != "" {
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: strings.TrimSpace(content) != \"\"")
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"cannot create new file - file already exists...")
			return tool.InvokeResult{}, fmt.Errorf("cannot create new file - file already exists and is not empty")
		}

		if err := os.WriteFile(filePath, []byte(in.NewString), 0644); err != nil {
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"write file: %w\", err)")
			return tool.InvokeResult{}, fmt.Errorf("write file: %w", err)
		}
		if timestamp, statErr := tool.FileTimestamp(filePath); statErr == nil {
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: statErr == nil")
			tool.RecordFileState(state, filePath, tool.NormalizeTextContent(in.NewString), timestamp, nil, nil, false)
		}
		display := util.GenerateEditDiff(content, in.OldString, in.NewString, in.FilePath, false, 3)
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"The file %s has been updated succes...")
		return tool.InvokeResult{
			Content: fmt.Sprintf("The file %s has been updated successfully.", in.FilePath),
			Display: display,
		}, nil
	}

	match := findEditMatch(content, in.OldString, in.NewString)

	if !match.Found {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: count == 0")

		hint := diagnoseWhitespaceMismatch(content, in.OldString)
		nearby := findNearestContext(content, in.OldString)
		retry := findRetryCandidate(content, in.OldString)
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"string to replace not found in file.%s%s\\nSt...")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"string to replace not found in file.%s%s%s\\n...")
		return tool.InvokeResult{}, fmt.Errorf("string to replace not found in file.%s%s%s\nString: %s", hint, nearby, retry, in.OldString)
	}
	count := strings.Count(content, match.OldString)

	if count > 1 && !in.ReplaceAll {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: count > 1 && !in.ReplaceAll")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"found %d matches of the string to replace, b...")
		return tool.InvokeResult{}, fmt.Errorf("found %d matches of the string to replace, but replace_all is false. To replace all occurrences, set replace_all to true. To replace only one occurrence, please provide more context to uniquely identify the instance.\nString: %s", count, match.OldString)
	}

	// Perform replacement
	var updated string
	if in.ReplaceAll {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: in.ReplaceAll")
		updated = strings.ReplaceAll(content, match.OldString, match.NewString)
	} else {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "else: in.ReplaceAll")
		updated = strings.Replace(content, match.OldString, match.NewString, 1)
	}

	if updated == content {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: updated == content")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"no changes to make: old_string and new_strin...")
		return tool.InvokeResult{}, fmt.Errorf("no changes to make: old_string and new_string produce identical file content")
	}

	display := util.GenerateEditDiff(content, match.OldString, match.NewString, in.FilePath, in.ReplaceAll, 3)

	timestamp, err := tool.FileTimestamp(filePath)
	if err != nil {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"stat file: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("stat file: %w", err)
	}
	if err := tool.EnsureFileFreshForWrite(state, filePath, content, timestamp); err != nil {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, err")
		return tool.InvokeResult{}, err
	}

	if err := os.WriteFile(filePath, []byte(updated), 0644); err != nil {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"write file: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("write file: %w", err)
	}

	if timestamp, statErr := tool.FileTimestamp(filePath); statErr == nil {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: statErr == nil")
		tool.RecordFileState(state, filePath, updated, timestamp, nil, nil, false)
	}

	if in.ReplaceAll && count > 1 {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: in.ReplaceAll && count > 1")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"The file %s has been updated. All %...")
		return tool.InvokeResult{
			Content: fmt.Sprintf("The file %s has been updated. All %d occurrences were successfully replaced.", in.FilePath, count),
			Display: display,
		}, nil
	}
	observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"The file %s has been updated succes...")
	return tool.InvokeResult{
		Content: fmt.Sprintf("The file %s has been updated successfully.", in.FilePath),
		Display: display,
	}, nil
}

func isMultilineEdit(in FileEditInput) bool {
	return strings.Contains(in.OldString, "\n") || strings.Contains(in.NewString, "\n")
}

func handleNonexistentFile(filePath string, in FileEditInput) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if in.OldString == "" {
		observe.GlobalTrace("if: in.OldString == \"\"")

		dir := filepath.Dir(filePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: \"\", fmt.Errorf(\"create directory: %w\", err)")
			return "", fmt.Errorf("create directory: %w", err)
		}
		if err := os.WriteFile(filePath, []byte(in.NewString), 0644); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: \"\", fmt.Errorf(\"write file: %w\", err)")
			return "", fmt.Errorf("write file: %w", err)
		}
		observe.GlobalTrace("return: fmt.Sprintf(\"The file %s has been created successfully.\", in.FilePath), nil")
		return fmt.Sprintf("The file %s has been created successfully.", in.FilePath), nil
	}
	observe.GlobalTrace("return: \"\", fmt.Errorf(\"file does not exist: %s. Make sure the path is correct.\", in....")
	return "", fmt.Errorf("file does not exist: %s. Make sure the path is correct.", in.FilePath)
}

type editMatch struct {
	Found     bool
	OldString string
	NewString string
}

func normalizeNewString(filePath, newString string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ext := strings.ToLower(filepath.Ext(filePath))
	if ext == ".md" || ext == ".mdx" {
		observe.GlobalTrace("if: ext == \".md\" || ext == \".mdx\"")
		observe.GlobalTrace("return: newString")
		return newString
	}
	observe.GlobalTrace("return: stripTrailingWhitespace(newString)")
	return stripTrailingWhitespace(newString)
}

func stripTrailingWhitespace(s string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' })
	if len(parts) == 0 && s == "" {
		observe.GlobalTrace("if: len(parts) == 0 && s == \"\"")
		observe.GlobalTrace("return: s")
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); {
		observe.GlobalTrace("for: i < len(s)")
		j := i
		for j < len(s) && s[j] != '\n' && s[j] != '\r' {
			observe.GlobalTrace("for: j < len(s) && s[j] != '\\n' && s[j] != '\\r'")
			j++
		}
		b.WriteString(strings.TrimRight(s[i:j], " \t"))
		if j >= len(s) {
			observe.GlobalTrace("if: j >= len(s)")
			break
		}
		if s[j] == '\r' && j+1 < len(s) && s[j+1] == '\n' {
			observe.GlobalTrace("if: s[j] == '\\r' && j+1 < len(s) && s[j+1] == '\\n'")
			b.WriteString("\r\n")
			i = j + 2
		} else {
			observe.GlobalTrace("else: s[j] == '\\r' && j+1 < len(s) && s[j+1] == '\\n'")
			b.WriteByte(s[j])
			i = j + 1
		}
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func findEditMatch(content, oldString, newString string) editMatch {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if actualOld, ok := findActualString(content, oldString); ok {
		observe.GlobalTrace("if: ok")
		observe.GlobalTrace("return: editMatch{\n\tFound:\t\ttrue,\n\tOldString:\tactualOld,\n\tNewString:\tpreserveQuoteSty...")
		return editMatch{
			Found:     true,
			OldString: actualOld,
			NewString: preserveQuoteStyle(oldString, actualOld, newString),
		}
	}

	desanitizedOld, _ := desanitizeMatchString(oldString)
	if desanitizedOld != oldString {
		observe.GlobalTrace("if: desanitizedOld != oldString")
		desanitizedNew, _ := desanitizeMatchString(newString)
		if actualOld, ok := findActualString(content, desanitizedOld); ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: editMatch{\n\tFound:\t\ttrue,\n\tOldString:\tactualOld,\n\tNewString:\tpreserveQuoteSty...")
			return editMatch{
				Found:     true,
				OldString: actualOld,
				NewString: preserveQuoteStyle(desanitizedOld, actualOld, desanitizedNew),
			}
		}
	}
	observe.GlobalTrace("return: editMatch{}")

	return editMatch{}
}

func findActualString(content, search string) (string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.Contains(content, search) {
		observe.GlobalTrace("if: strings.Contains(content, search)")
		observe.GlobalTrace("return: search, true")
		return search, true
	}

	normalizedContent := normalizeQuotes(content)
	normalizedSearch := normalizeQuotes(search)
	if !strings.Contains(normalizedContent, normalizedSearch) {
		observe.GlobalTrace("if: !strings.Contains(normalizedContent, normalizedSearch)")
		observe.GlobalTrace("return: \"\", false")
		return "", false
	}
	contentRunes := []rune(content)
	searchLen := len([]rune(search))
	if searchLen == 0 || searchLen > len(contentRunes) {
		observe.GlobalTrace("if: searchLen == 0 || searchLen > len(contentRunes)")
		observe.GlobalTrace("return: \"\", false")
		return "", false
	}
	for i := 0; i+searchLen <= len(contentRunes); i++ {
		observe.GlobalTrace("for: i+searchLen <= len(contentRunes)")
		candidate := string(contentRunes[i : i+searchLen])
		if normalizeQuotes(candidate) == normalizedSearch {
			observe.GlobalTrace("if: normalizeQuotes(candidate) == normalizedSearch")
			observe.GlobalTrace("return: candidate, true")
			return candidate, true
		}
	}
	observe.GlobalTrace("return: \"\", false")
	return "", false
}

func normalizeQuotes(s string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r := strings.NewReplacer(
		leftSingleCurlyQuote, "'",
		rightSingleCurlyQuote, "'",
		leftDoubleCurlyQuote, "\"",
		rightDoubleCurlyQuote, "\"",
	)
	observe.GlobalTrace("return: r.Replace(s)")
	return r.Replace(s)
}

func desanitizeMatchString(s string) (string, []stringReplacement) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out := s
	applied := make([]stringReplacement, 0)
	for _, replacement := range desanitizations {
		observe.GlobalTrace("range desanitizations")
		next := strings.ReplaceAll(out, replacement.from, replacement.to)
		if next != out {
			observe.GlobalTrace("if: next != out")
			applied = append(applied, replacement)
			out = next
		}
	}
	observe.GlobalTrace("return: out, applied")
	return out, applied
}

func preserveQuoteStyle(oldString, actualOldString, newString string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if oldString == actualOldString {
		observe.GlobalTrace("if: oldString == actualOldString")
		observe.GlobalTrace("return: newString")
		return newString
	}
	hasDoubleQuotes := strings.Contains(actualOldString, leftDoubleCurlyQuote) || strings.Contains(actualOldString, rightDoubleCurlyQuote)
	hasSingleQuotes := strings.Contains(actualOldString, leftSingleCurlyQuote) || strings.Contains(actualOldString, rightSingleCurlyQuote)
	if !hasDoubleQuotes && !hasSingleQuotes {
		observe.GlobalTrace("if: !hasDoubleQuotes && !hasSingleQuotes")
		observe.GlobalTrace("return: newString")
		return newString
	}
	if hasDoubleQuotes {
		observe.GlobalTrace("if: hasDoubleQuotes")
		newString = applyCurlyDoubleQuotes(newString)
	}
	if hasSingleQuotes {
		observe.GlobalTrace("if: hasSingleQuotes")
		newString = applyCurlySingleQuotes(newString)
	}
	observe.GlobalTrace("return: newString")
	return newString
}

func applyCurlyDoubleQuotes(s string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		observe.GlobalTrace("range runes")
		if r == '"' {
			observe.GlobalTrace("if: r == '\"'")
			if isOpeningQuoteContext(runes, i) {
				observe.GlobalTrace("if: isOpeningQuoteContext(runes, i)")
				b.WriteString(leftDoubleCurlyQuote)
			} else {
				observe.GlobalTrace("else: isOpeningQuoteContext(runes, i)")
				b.WriteString(rightDoubleCurlyQuote)
			}
			continue
		}
		b.WriteRune(r)
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func applyCurlySingleQuotes(s string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	runes := []rune(s)
	var b strings.Builder
	for i, r := range runes {
		observe.GlobalTrace("range runes")
		if r == '\'' {
			observe.GlobalTrace("if: r == '\\''")
			if i > 0 && i < len(runes)-1 && unicode.IsLetter(runes[i-1]) && unicode.IsLetter(runes[i+1]) {
				observe.GlobalTrace("if: i > 0 && i < len(runes)-1 && unicode.IsLetter(runes[i-1]) && unicode.IsLetter...")
				b.WriteString(rightSingleCurlyQuote)
			} else if isOpeningQuoteContext(runes, i) {
				observe.GlobalTrace("else-if: isOpeningQuoteContext(runes, i)")
				b.WriteString(leftSingleCurlyQuote)
			} else {
				b.WriteString(rightSingleCurlyQuote)
			}
			continue
		}
		b.WriteRune(r)
	}
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

func isOpeningQuoteContext(runes []rune, index int) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if index == 0 {
		observe.GlobalTrace("if: index == 0")
		observe.GlobalTrace("return: true")
		return true
	}
	switch runes[index-1] {
	case ' ', '\t', '\n', '\r', '(', '[', '{', '—', '–':
		observe.GlobalTrace("case: ' ', '\\t', '\\n', '\\r', '(', '[', '{', '—', '–'")
		return true
	default:
		observe.GlobalTrace("default")
		return false
	}
}

func findRetryCandidate(content, oldString string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	oldLines := strings.Split(oldString, "\n")
	lineCount := len(oldLines)
	if lineCount == 0 {
		observe.GlobalTrace("if: lineCount == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	lines := strings.Split(content, "\n")
	if lineCount > len(lines) {
		observe.GlobalTrace("if: lineCount > len(lines)")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	normalizedOld := normalizeEditWhitespace(oldString)
	candidates := make([]string, 0, 2)
	for i := 0; i+lineCount <= len(lines); i++ {
		observe.GlobalTrace("for: i+lineCount <= len(lines)")
		candidate := strings.Join(lines[i:i+lineCount], "\n")
		if normalizeEditWhitespace(candidate) == normalizedOld {
			observe.GlobalTrace("if: normalizeEditWhitespace(candidate) == normalizedOld")
			candidates = append(candidates, candidate)
			if len(candidates) > 1 {
				observe.GlobalTrace("if: len(candidates) > 1")
				observe.GlobalTrace("return: \"\"")
				return ""
			}
		}
	}
	if len(candidates) != 1 {
		observe.GlobalTrace("if: len(candidates) != 1")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: \"\\nRetry with this exact old_string:\\n```\\n\" + candidates[0] + \"\\n```\"")

	return "\nRetry with this exact old_string:\n```\n" + candidates[0] + "\n```"
}

func normalizeEditWhitespace(s string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		observe.GlobalTrace("range lines")
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	observe.GlobalTrace("return: strings.TrimSpace(strings.Join(lines, \"\\n\"))")
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// findNearestContext searches for the first line of old_string in the file
// and returns surrounding context to help the model see what's actually there.
// Returns empty string if no meaningful match is found.
func findNearestContext(content, oldString string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	lines := strings.Split(content, "\n")
	oldLines := strings.Split(oldString, "\n")
	if len(oldLines) == 0 {
		observe.GlobalTrace("if: len(oldLines) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	// Try to find the first non-empty line of old_string in the file
	var searchLine string
	for _, l := range oldLines {
		observe.GlobalTrace("range oldLines")
		trimmed := strings.TrimSpace(l)
		if trimmed != "" && trimmed != "{" && trimmed != "}" && len(trimmed) > 5 {
			observe.GlobalTrace("if: trimmed != \"\" && trimmed != \"{\" && trimmed != \"}\" && len(trimmed) > 5")
			searchLine = trimmed
			break
		}
	}
	if searchLine == "" {
		observe.GlobalTrace("if: searchLine == \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	bestIdx := -1
	for i, line := range lines {
		observe.GlobalTrace("range lines")
		if strings.Contains(strings.TrimSpace(line), searchLine) {
			observe.GlobalTrace("if: strings.Contains(strings.TrimSpace(line), searchLine)")
			bestIdx = i
			break
		}
	}

	if bestIdx == -1 {
		observe.GlobalTrace("if: bestIdx == -1")

		if len(searchLine) > 30 {
			observe.GlobalTrace("if: len(searchLine) > 30")
			prefix := searchLine[:30]
			for i, line := range lines {
				observe.GlobalTrace("range lines")
				if strings.Contains(line, prefix) {
					observe.GlobalTrace("if: strings.Contains(line, prefix)")
					bestIdx = i
					break
				}
			}
		}
	}

	if bestIdx == -1 {
		observe.GlobalTrace("if: bestIdx == -1")
		observe.GlobalTrace("return: \"\\nHint: no similar content found. Re-read the file with the Read tool to see...")
		return "\nHint: no similar content found. Re-read the file with the Read tool to see its current content before retrying."
	}

	start := bestIdx - 2
	if start < 0 {
		observe.GlobalTrace("if: start < 0")
		start = 0
	}
	end := bestIdx + len(oldLines) + 2
	if end > len(lines) {
		observe.GlobalTrace("if: end > len(lines)")
		end = len(lines)
	}

	if end-start > 15 {
		observe.GlobalTrace("if: end-start > 15")
		end = start + 15
	}

	var b strings.Builder
	b.WriteString("\nHint: found similar content at lines ")
	b.WriteString(fmt.Sprintf("%d-%d", start+1, end))
	b.WriteString(". Actual content:\n")
	for i := start; i < end; i++ {
		observe.GlobalTrace("for: i < end")
		b.WriteString(fmt.Sprintf("  %d\t%s\n", i+1, lines[i]))
	}
	b.WriteString("Use the actual content shown above as your old_string for the next edit attempt.")
	observe.GlobalTrace("return: b.String()")
	return b.String()
}

// diagnoseWhitespaceMismatch checks whether a tab↔space conversion would
// produce a match and returns a diagnostic hint for the error message.
// Returns empty string when no whitespace mismatch is detected.
func diagnoseWhitespaceMismatch(content, oldString string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	hasTabs := strings.Contains(oldString, "\t")
	hasLeadingSpaces := false
	for _, line := range strings.Split(oldString, "\n") {
		observe.GlobalTrace("range strings.Split(oldString, \"\\n\")")
		if len(line) > 0 && line[0] == ' ' {
			observe.GlobalTrace("if: len(line) > 0 && line[0] == ' '")
			hasLeadingSpaces = true
			break
		}
	}
	if !hasTabs && !hasLeadingSpaces {
		observe.GlobalTrace("if: !hasTabs && !hasLeadingSpaces")
		observe.GlobalTrace("return: \"\"")
		return ""
	}

	tabToSpaces := strings.ReplaceAll(oldString, "\t", "    ")
	if strings.Contains(content, tabToSpaces) {
		observe.GlobalTrace("if: strings.Contains(content, tabToSpaces)")
		observe.GlobalTrace("return: \"\\nHint: the file uses spaces for indentation but old_string contains tabs. R...")
		return "\nHint: the file uses spaces for indentation but old_string contains tabs. Replace tabs with spaces and retry."
	}

	spacesToTab := strings.ReplaceAll(oldString, "    ", "\t")
	if strings.Contains(content, spacesToTab) {
		observe.GlobalTrace("if: strings.Contains(content, spacesToTab)")
		observe.GlobalTrace("return: \"\\nHint: the file uses tabs for indentation but old_string contains spaces. R...")
		return "\nHint: the file uses tabs for indentation but old_string contains spaces. Replace leading spaces with tabs and retry."
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}
