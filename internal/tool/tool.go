package tool

import (
	"context"
	"encoding/json"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
)

// StateSnapshot provides read-only state to tools at invocation time.
type StateSnapshot interface {
	WorkDir() string
}

// ToolFlags describe tool behavior for orchestration decisions.
type ToolFlags struct {
	ReadOnly           bool
	Concurrent         bool
	Destructive        bool
	MaxResultSizeChars int
}

type SessionIDProvider interface {
	SessionID() string
}

// InvocationContext carries orchestrator-owned identity into a tool invocation.
type InvocationContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	ToolCallID   string
	ToolName     string
}

type invocationContextKey struct{}

func WithInvocationContext(ctx context.Context, meta InvocationContext) context.Context {
	observe.TraceCtx(ctx, "tool", "WithInvocationContext", "enter")
	defer observe.TraceCtx(ctx, "tool", "WithInvocationContext", "exit")
	observe.TraceCtx(ctx, "tool", "WithInvocationContext", "return: context.WithValue(ctx, invocationContextKey{}, meta)")
	return context.WithValue(ctx, invocationContextKey{}, meta)
}

func InvocationContextFrom(ctx context.Context) (InvocationContext, bool) {
	observe.TraceCtx(ctx, "tool", "InvocationContextFrom", "enter")
	defer observe.TraceCtx(ctx, "tool", "InvocationContextFrom", "exit")
	meta, ok := ctx.Value(invocationContextKey{}).(InvocationContext)
	observe.TraceCtx(ctx, "tool", "InvocationContextFrom", "return: meta, ok")
	return meta, ok
}

func SessionIDFrom(state StateSnapshot) (string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	provider, ok := state.(SessionIDProvider)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: \"\", false")
		return "", false
	}
	id := provider.SessionID()
	observe.GlobalTrace("return: id, id != \"\"")
	return id, id != ""
}

// InvokeResult holds the output from a tool invocation.
// Content is the text result sent to the LLM as the tool_result content.
// Supplements are optional additional content parts (e.g., DocumentPart for PDFs,
// ImagePart for extracted pages) that are included alongside the tool result
// in the conversation message sent to the LLM.
type InvokeResult struct {
	Content          string
	Supplements      []model.ContentPart
	StructuredOutput json.RawMessage
	UserMessages     []UserMessage
}

type UserMessageAttachment struct {
	Path    string
	Size    int64
	IsImage bool
}

type UserMessage struct {
	Message     string
	Status      string
	Attachments []UserMessageAttachment
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
