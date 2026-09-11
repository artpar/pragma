package query

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/hook"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// Run starts the agentic loop in a goroutine and returns a channel of LoopEvents.
// The channel is closed when the loop finishes.
// Implements SPEC.md §6.1.
func (engine *Engine) Run(ctx context.Context, userMessage string) <-chan LoopEvent {
	observe.TraceCtx(ctx, "query", "Engine.Run", "enter")
	defer observe.TraceCtx(ctx, "query", "Engine.Run", "exit")
	ch := make(chan LoopEvent, 16)
	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				observe.TraceCtx(ctx, "query", "Engine.Run", "if: r != nil")
				ch <- ErrorEvent{Err: fmt.Errorf("query loop panic: %v", r)}
			}
		}()
		switch engine.config.LoopMode {
		case LoopModeProviderTools:
			observe.TraceCtx(ctx, "query", "Engine.Run", "case: LoopModeProviderTools")
			engine.runProviderToolsLoop(ctx, userMessage, ch)
		default:
			observe.TraceCtx(ctx, "query", "Engine.Run", "default")
			engine.runPragmaLoop(ctx, userMessage, ch)
		}
	}()
	observe.TraceCtx(ctx, "query", "Engine.Run", "return: ch")
	return ch
}

func (engine *Engine) runStopHook(ch chan<- LoopEvent) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if engine.hookMgr == nil {
		observe.GlobalTrace("if: engine.hookMgr == nil")
		return
	}
	hookCtx, hookCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer hookCancel()
	result := engine.hookMgr.ExecuteInWorkDir(hookCtx, hook.Stop, hook.HookInput{}, engine.store.Snapshot().CWD)
	if result.Blocked {
		observe.GlobalTrace("if: result.Blocked")
		ch <- ErrorEvent{Err: fmt.Errorf("blocked by hook: %s", result.BlockMsg)}
	}
}

func (engine *Engine) systemWithMCPStatus(system model.SystemPrompt) model.SystemPrompt {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if engine.config.MCPServerStatuses == nil {
		observe.GlobalTrace("if: engine.config.MCPServerStatuses == nil")
		observe.GlobalTrace("return: system")
		return system
	}
	statuses := engine.config.MCPServerStatuses()
	if len(statuses) == 0 {
		observe.GlobalTrace("if: len(statuses) == 0")
		observe.GlobalTrace("return: system")
		return system
	}

	var b strings.Builder
	b.WriteString("# MCP Servers\n\n")
	b.WriteString("mcp_servers:\n")
	for _, server := range statuses {
		observe.GlobalTrace("range statuses")
		if server.Name == "" {
			observe.GlobalTrace("if: server.Name == \"\"")
			continue
		}
		status := server.Status
		if status == "" {
			observe.GlobalTrace("if: status == \"\"")
			status = "disconnected"
		}
		fmt.Fprintf(&b, "- name: %s\n  status: %s\n", server.Name, status)
	}
	b.WriteString("\nUse this mcp_servers metadata as the authoritative MCP server configuration and connection status. When asked which MCP servers are configured, active, inactive, connected, failed, pending, or disabled, answer directly from mcp_servers without calling tools. ListMcpResourcesTool lists resources only and must not be used to infer MCP server status.")

	blocks := make([]model.SystemBlock, 0, len(system.Blocks)+1)
	blocks = append(blocks, system.Blocks...)
	blocks = append(blocks, model.SystemBlock{Text: b.String(), Cacheable: false})
	observe.GlobalTrace("return: model.SystemPrompt{Blocks: blocks}")
	return model.SystemPrompt{Blocks: blocks}
}

func (engine *Engine) systemWithPatchGuidance(system model.SystemPrompt, tools []model.ToolDef) model.SystemPrompt {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if !toolDefsContain(tools, "apply_patch") {
		observe.GlobalTrace("if: !toolDefsContain(tools, \"apply_patch\")")
		observe.GlobalTrace("return: system")
		return system
	}
	block := `# Patch Editing

When editing source files, use apply_patch. Its JSON input must be {"patch":"..."} with a patch body that starts with *** Begin Patch and ends with *** End Patch.

Inside update hunks, every line must start with a leading space for unchanged context, - for removed lines, + for added lines, or @@ for hunk markers. Do not use Bash redirection, sed, awk, tee, or other shell mutation commands for source edits. If apply_patch fails, fix the patch syntax or reread the target range and retry with a smaller patch instead of switching editing tools. Run focused tests after the last successful edit.`
	blocks := make([]model.SystemBlock, 0, len(system.Blocks)+1)
	blocks = append(blocks, system.Blocks...)
	blocks = append(blocks, model.SystemBlock{Text: block, Cacheable: false})
	observe.GlobalTrace("return: model.SystemPrompt{Blocks: blocks}")
	return model.SystemPrompt{Blocks: blocks}
}

func toolDefsContain(tools []model.ToolDef, name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, tool := range tools {
		observe.GlobalTrace("range tools")
		if tool.Name == name {
			observe.GlobalTrace("if: tool.Name == name")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

func (engine *Engine) messagesForRequestChecked(conv model.Conversation, startIndexes ...int) ([]model.Message, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	start := 0
	if len(startIndexes) > 0 {
		observe.GlobalTrace("if: len(startIndexes) > 0")
		start = startIndexes[0]
	} else {
		observe.GlobalTrace("else: len(startIndexes) > 0")
	}
	messages := engine.messagesForRequestFrom(conv, start)
	if err := validateToolResultPairing(messages); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, err")
		return nil, err
	}
	observe.GlobalTrace("return: messages, nil")
	return messages, nil
}

func (engine *Engine) messagesForRequestFrom(conv model.Conversation, start int) []model.Message {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if start < 0 {
		observe.GlobalTrace("if: start < 0")
		start = 0
	}
	if start > len(conv.Messages) {
		observe.GlobalTrace("if: start > len(conv.Messages)")
		start = len(conv.Messages)
	}
	scoped := conv
	scoped.Messages = conv.Messages[start:]
	observe.GlobalTrace("return: scoped.APIMessages()")
	return scoped.APIMessages()
}

func messageHasToolResult(msg model.Message) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	for _, part := range msg.Content {
		observe.GlobalTrace("range msg.Content")
		if _, ok := part.(model.ToolResultPart); ok {
			observe.GlobalTrace("if: ok")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}
