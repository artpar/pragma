package sleep

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type sleepInput struct {
	Seconds int `json:"seconds" desc:"Number of seconds to sleep (1-300)"`
}

var inputSchema = json.RawMessage(`{
	"type": "object",
	"required": ["seconds"],
	"properties": {
		"seconds": {
			"type": "integer",
			"description": "Number of seconds to sleep (1-300)",
			"minimum": 1,
			"maximum": 300
		}
	}
}`)

// Tool implements an async delay. Context cancellation interrupts the sleep.
type Tool struct{}

func (t *Tool) Name() string                { return "Sleep" }
func (t *Tool) Description() string          { return "Pause execution for a specified number of seconds." }
func (t *Tool) InputSchema() json.RawMessage { return inputSchema }
func (t *Tool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *Tool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "Sleep", "")
}

func (t *Tool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in sleepInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Seconds < 1 {
		in.Seconds = 1
	}
	if in.Seconds > 300 {
		in.Seconds = 300
	}

	duration := time.Duration(in.Seconds) * time.Second
	select {
	case <-time.After(duration):
		unit := "seconds"
		if in.Seconds == 1 {
			unit = "second"
		}
		return tool.InvokeResult{
			Content: fmt.Sprintf("Slept for %d %s.", in.Seconds, unit),
		}, nil
	case <-ctx.Done():
		return tool.InvokeResult{
			Content: "Sleep cancelled.",
		}, nil
	}
}
