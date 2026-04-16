package filewrite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/artpar/pragma/internal/lsp"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/util"
)

// FileWriteInput defines the parameters for the FileWrite tool.
type FileWriteInput struct {
	FilePath string `json:"file_path" desc:"The path to the file to write"`
	Content  string `json:"content" desc:"The content to write to the file"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["file_path", "content"],
	"properties": {
		"file_path": {
			"type": "string",
			"description": "The path to the file to write"
		},
		"content": {
			"type": "string",
			"description": "The content to write to the file"
		}
	}
}`)

// Tool implements the FileWrite tool.
type Tool struct {
	LSP *lsp.Manager // optional; nil if no LSP servers configured
}

func (t *Tool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Write\"")
	return "Write"
}
func (t *Tool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Writes a file to the local filesystem...\"")
	observe.GlobalTrace("return: writeDescription")
	return writeDescription
}

const writeDescription = `Writes a file to the local filesystem.

Usage:
- This tool will overwrite the existing file if there is one at the provided path.
- If this is an existing file, you MUST use the Read tool first to read the file's contents. This tool will fail if you did not read the file first.
- Prefer the Edit tool for modifying existing files — it only sends the diff. Only use this tool to create new files or for complete rewrites.
- NEVER create documentation files (*.md) or README files unless explicitly requested by the User.
- Only use emojis if the user explicitly requests it. Avoid writing emojis to files unless asked.`

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
	observe.TraceCtx(ctx, "filewrite", "Tool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "filewrite", "Tool.CheckPerm", "exit")
	var in struct {
		FilePath string `json:"file_path"`
	}
	if err := json.Unmarshal(input, &in); err != nil || in.FilePath == "" {
		observe.TraceCtx(ctx, "filewrite", "Tool.CheckPerm", "if: err != nil || in.FilePath == \"\"")
		observe.TraceCtx(ctx, "filewrite", "Tool.CheckPerm", "return: checker.Check(ctx, \"Write\", \"\")")
		return checker.Check(ctx, "Write", "")
	}
	observe.TraceCtx(ctx, "filewrite", "Tool.CheckPerm", "return: checker.Check(ctx, \"Write\", in.FilePath)")
	return checker.Check(ctx, "Write", in.FilePath)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "exit")
	var in FileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.FilePath == "" {
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "if: in.FilePath == \"\"")
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"file_path is required\")")
		return tool.InvokeResult{}, fmt.Errorf("file_path is required")
	}

	filePath := util.ExpandPath(in.FilePath, state.WorkDir())

	_, err := os.Stat(filePath)
	isCreate := os.IsNotExist(err)

	// Capture old content before overwriting (needed for diff generation)
	var oldContent string
	if !isCreate {
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "if: !isCreate")
		if data, readErr := os.ReadFile(filePath); readErr == nil {
			oldContent = string(data)
		}
	}

	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"create directory %s: %w\", dir, err)")
		return tool.InvokeResult{}, fmt.Errorf("create directory %s: %w", dir, err)
	}

	if err := os.WriteFile(filePath, []byte(in.Content), 0644); err != nil {
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"write file: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("write file: %w", err)
	}

	if t.LSP != nil {
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "if: t.LSP != nil")
		_ = t.LSP.ChangeFile(ctx, filePath, in.Content)
		_ = t.LSP.SaveFile(ctx, filePath)
	}

	// Generate display diff for TUI (never sent to LLM)
	var display string
	if isCreate {
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "if: isCreate")
		display = util.GenerateEditDiff("", "", in.Content, in.FilePath, false, 3)
		observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "return: tool.InvokeResult{Content: ..., Display: display}")
		return tool.InvokeResult{
			Content: fmt.Sprintf("File created successfully at: %s", in.FilePath),
			Display: display,
		}, nil
	}
	display = util.GenerateEditDiff(oldContent, oldContent, in.Content, in.FilePath, false, 3)
	observe.TraceCtx(ctx, "filewrite", "Tool.Invoke", "return: tool.InvokeResult{Content: ..., Display: display}")
	return tool.InvokeResult{
		Content: fmt.Sprintf("The file %s has been updated successfully.", in.FilePath),
		Display: display,
	}, nil
}
