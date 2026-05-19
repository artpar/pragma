package toolresultread

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/toolresult"
)

const (
	defaultLimitBytes = 4_000
	maxLimitBytes     = 20_000
)

type input struct {
	ToolCallID  string `json:"tool_call_id"`
	OffsetBytes int64  `json:"offset_bytes,omitempty"`
	LimitBytes  int    `json:"limit_bytes,omitempty"`
}

type output struct {
	ToolCallID      string `json:"tool_call_id"`
	Content         string `json:"content"`
	OffsetBytes     int64  `json:"offset_bytes"`
	LimitBytes      int    `json:"limit_bytes"`
	NextOffsetBytes int64  `json:"next_offset_bytes"`
	HasMore         bool   `json:"has_more"`
	TotalBytes      int64  `json:"total_bytes"`
	SourcePath      string `json:"source_path"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["tool_call_id"],
	"properties": {
		"tool_call_id": {
			"type": "string",
			"description": "Tool call id from a <persisted-output> marker."
		},
		"offset_bytes": {
			"type": "integer",
			"description": "Byte offset to start reading from. Defaults to 0."
		},
		"limit_bytes": {
			"type": "integer",
			"description": "Maximum bytes to return. Defaults to 4000 and is capped at 20000."
		}
	}
}`)

// Tool pages through Pragma's own persisted tool-result files.
type Tool struct{}

func (t *Tool) Name() string { return "tool_result.read" }

func (t *Tool) Description() string {
	return "Read a paginated chunk of a large tool result that Pragma persisted for this session. Use offset_bytes and limit_bytes to page through the content."
}

func (t *Tool) InputSchema() json.RawMessage { return inputSchema }

func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, raw json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, t.Name(), string(raw))
}

func (t *Tool) Invoke(_ context.Context, raw json.RawMessage, state tool.StateSnapshot) (tool.InvokeResult, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		observe.GlobalTrace("tool_result.read invalid input")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.ToolCallID == "" {
		return tool.InvokeResult{}, fmt.Errorf("tool_call_id is required")
	}
	sessionID, ok := tool.SessionIDFrom(state)
	if !ok {
		return tool.InvokeResult{}, fmt.Errorf("session id is unavailable")
	}
	if in.OffsetBytes < 0 {
		return tool.InvokeResult{}, fmt.Errorf("offset_bytes must be >= 0")
	}
	limit := in.LimitBytes
	if limit <= 0 {
		limit = defaultLimitBytes
	}
	if limit > maxLimitBytes {
		limit = maxLimitBytes
	}

	path, err := toolresult.PersistedOutputPath(sessionID, in.ToolCallID)
	if err != nil {
		return tool.InvokeResult{}, err
	}
	f, err := os.Open(path)
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("open persisted tool result: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return tool.InvokeResult{}, fmt.Errorf("stat persisted tool result: %w", err)
	}
	total := stat.Size()
	if in.OffsetBytes > total {
		return tool.InvokeResult{}, fmt.Errorf("offset_bytes %d exceeds total_bytes %d", in.OffsetBytes, total)
	}

	buf := make([]byte, limit)
	n, err := f.ReadAt(buf, in.OffsetBytes)
	if err != nil && err != io.EOF {
		return tool.InvokeResult{}, fmt.Errorf("read persisted tool result: %w", err)
	}
	next := in.OffsetBytes + int64(n)
	out := output{
		ToolCallID:      in.ToolCallID,
		Content:         string(buf[:n]),
		OffsetBytes:     in.OffsetBytes,
		LimitBytes:      limit,
		NextOffsetBytes: next,
		HasMore:         next < total,
		TotalBytes:      total,
		SourcePath:      path,
	}
	data, err := json.Marshal(out)
	if err != nil {
		return tool.InvokeResult{}, err
	}
	return tool.InvokeResult{Content: string(data)}, nil
}
