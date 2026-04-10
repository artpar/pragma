package filewrite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

// FileWriteInput defines the parameters for the FileWrite tool.
type FileWriteInput struct {
	FilePath string `json:"file_path" desc:"The absolute path to the file to write (must be absolute, not relative)"`
	Content  string `json:"content" desc:"The content to write to the file"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["file_path", "content"],
	"properties": {
		"file_path": {
			"type": "string",
			"description": "The absolute path to the file to write (must be absolute, not relative)"
		},
		"content": {
			"type": "string",
			"description": "The content to write to the file"
		}
	}
}`)

// Tool implements the FileWrite tool.
type Tool struct{}

func (t *Tool) Name() string                { return "Write" }
func (t *Tool) Description() string          { return "Write a file to the local filesystem. Creates parent directories as needed." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: false, Concurrent: false}
}

func (t *Tool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "Write", input)
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	var in FileWriteInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.FilePath == "" {
		return tool.InvokeResult{}, fmt.Errorf("file_path is required")
	}

	filePath := in.FilePath
	if !filepath.IsAbs(filePath) {
		return tool.InvokeResult{}, fmt.Errorf("file_path must be absolute, got: %s", filePath)
	}

	// Determine if creating or updating
	_, err := os.Stat(filePath)
	isCreate := os.IsNotExist(err)

	// Create parent directories
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("create directory %s: %w", dir, err)
	}

	// Write the file
	if err := os.WriteFile(filePath, []byte(in.Content), 0644); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("write file: %w", err)
	}

	if isCreate {
		return tool.InvokeResult{Content: fmt.Sprintf("The file %s has been created successfully.", in.FilePath)}, nil
	}
	return tool.InvokeResult{Content: fmt.Sprintf("The file %s has been updated successfully.", in.FilePath)}, nil
}
