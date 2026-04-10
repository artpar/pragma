package tool

import (
	"context"
	"encoding/json"

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

// Descriptor defines a tool that can be invoked by the LLM.
// Each tool implementation satisfies this interface.
type Descriptor interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Invoke(ctx context.Context, input json.RawMessage, state StateSnapshot) (string, error)
	CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult
	Flags() ToolFlags
}
