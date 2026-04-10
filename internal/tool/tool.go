package tool

import (
	"context"
	"encoding/json"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/permission"
)

// StateSnapshot provides read-only state to tools at invocation time.
type StateSnapshot interface {
	WorkDir() string
}

// ToolFlags describe tool behavior for orchestration decisions.
type ToolFlags struct {
	ReadOnly    bool
	Concurrent  bool
	Destructive bool
}

// InvokeResult holds the output from a tool invocation.
// Content is the text result sent to the LLM as the tool_result content.
// Supplements are optional additional content parts (e.g., DocumentPart for PDFs,
// ImagePart for extracted pages) that are included alongside the tool result
// in the conversation message sent to the LLM.
type InvokeResult struct {
	Content     string
	Supplements []model.ContentPart
}

// Descriptor defines a tool that can be invoked by the LLM.
// Each tool implementation satisfies this interface.
type Descriptor interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Invoke(ctx context.Context, input json.RawMessage, state StateSnapshot) (InvokeResult, error)
	CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult
	Flags() ToolFlags
}
