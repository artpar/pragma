package fileedit

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/artpar/pragma/internal/lsp"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/util"
)

const maxEditFileSize = 1024 * 1024 * 1024 // 1 GiB

// FileEditInput defines the parameters for the FileEdit tool.
type FileEditInput struct {
	FilePath   string `json:"file_path" desc:"The path to the file to edit"`
	OldString  string `json:"old_string" desc:"The exact string to find and replace"`
	NewString  string `json:"new_string" desc:"The replacement string"`
	ReplaceAll bool   `json:"replace_all,omitempty" desc:"If true, replace all occurrences. Default false."`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
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
	LSP *lsp.Manager // optional; nil if no LSP servers configured
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
- You must use your Read tool at least once in the conversation before editing. This tool will error if you attempt an edit without reading the file.
- When editing text from Read tool output, ensure you preserve the exact indentation (tabs/spaces) as it appears AFTER the → arrow. The line number prefix format is: spaces + line number + →. Everything after the → is the actual file content to match. Never include the line number or → in old_string or new_string.
- ALWAYS prefer editing existing files in the codebase. NEVER write new files unless explicitly required.
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

	filePath := util.ExpandPath(in.FilePath, state.WorkDir())

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

	content = strings.ReplaceAll(content, "\r\n", "\n")

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
		if t.LSP != nil {
			observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: t.LSP != nil")
			_ = t.LSP.ChangeFile(ctx, filePath, in.NewString)
			_ = t.LSP.SaveFile(ctx, filePath)
		}
		display := util.GenerateEditDiff(content, in.OldString, in.NewString, in.FilePath, false, 3)
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{\n\tContent:\tfmt.Sprintf(\"The file %s has been updated succes...")
		return tool.InvokeResult{
			Content: fmt.Sprintf("The file %s has been updated successfully.", in.FilePath),
			Display: display,
		}, nil
	}

	count := strings.Count(content, in.OldString)

	if count == 0 {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: count == 0")

		hint := diagnoseWhitespaceMismatch(content, in.OldString)
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"string to replace not found in file...\")")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"string to replace not found in file.%s\\nStri...")
		return tool.InvokeResult{}, fmt.Errorf("string to replace not found in file.%s\nString: %s", hint, in.OldString)
	}

	if count > 1 && !in.ReplaceAll {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: count > 1 && !in.ReplaceAll")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"found %d matches of the string to replace, b...")
		return tool.InvokeResult{}, fmt.Errorf("found %d matches of the string to replace, but replace_all is false. To replace all occurrences, set replace_all to true. To replace only one occurrence, please provide more context to uniquely identify the instance.\nString: %s", count, in.OldString)
	}

	// Perform replacement
	var updated string
	if in.ReplaceAll {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: in.ReplaceAll")
		updated = strings.ReplaceAll(content, in.OldString, in.NewString)
	} else {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "else: in.ReplaceAll")
		updated = strings.Replace(content, in.OldString, in.NewString, 1)
	}

	display := util.GenerateEditDiff(content, in.OldString, in.NewString, in.FilePath, in.ReplaceAll, 3)

	if err := os.WriteFile(filePath, []byte(updated), 0644); err != nil {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"write file: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("write file: %w", err)
	}

	if t.LSP != nil {
		observe.TraceCtx(ctx, "fileedit", "Tool.Invoke", "if: t.LSP != nil")
		_ = t.LSP.ChangeFile(ctx, filePath, updated)
		_ = t.LSP.SaveFile(ctx, filePath)
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
