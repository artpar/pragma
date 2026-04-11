package toolmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/mcp"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type listInput struct {
	Server string `json:"server" desc:"Optional server name to filter resources by"`
}

var listInputSchema = json.RawMessage(`{
	"type": "object",
	"properties": {
		"server": {
			"type": "string",
			"description": "Optional MCP server name to filter resources by. If omitted, lists resources from all connected servers."
		}
	}
}`)

// ListTool lists resources exposed by connected MCP servers.
type ListTool struct {
	Manager *mcp.Manager
}

func (t *ListTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"ListMcpResourcesTool\"")
	return "ListMcpResourcesTool"
}
func (t *ListTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: listInputSchema")
	return listInputSchema
}
func (t *ListTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *ListTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"List resources available from connected MCP servers. Resources provide data ...")
	return "List resources available from connected MCP servers. Resources provide data the server wants to expose (files, database records, API responses, etc.)."
}

func (t *ListTool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "toolmcp", "ListTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "toolmcp", "ListTool.CheckPerm", "exit")
	observe.TraceCtx(ctx, "toolmcp", "ListTool.CheckPerm", "return: checker.Check(ctx, \"ListMcpResourcesTool\", \"\")")
	return checker.Check(ctx, "ListMcpResourcesTool", "")
}

func (t *ListTool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "exit")
	var in listInput
	if len(input) > 0 {
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: len(input) > 0")
		if err := json.Unmarshal(input, &in); err != nil {
			observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: err != nil")
			observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
			return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
		}
	}

	clients := t.Manager.Clients()
	if len(clients) == 0 {
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: len(clients) == 0")
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "return: tool.InvokeResult{Content: \"No MCP servers connected.\"}, nil")
		return tool.InvokeResult{Content: "No MCP servers connected."}, nil
	}

	type resourceEntry struct {
		URI         string `json:"uri"`
		Name        string `json:"name"`
		MimeType    string `json:"mime_type,omitempty"`
		Description string `json:"description,omitempty"`
		Server      string `json:"server"`
	}

	var all []resourceEntry
	var errors []string

	for name, client := range clients {
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "range clients")
		if in.Server != "" && name != in.Server {
			observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: in.Server != \"\" && name != in.Server")
			continue
		}
		if !client.Connected() {
			observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: !client.Connected()")
			continue
		}

		resources, err := client.ListResources(ctx)
		if err != nil {
			observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: err != nil")
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}

		for _, r := range resources {
			observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "range resources")
			all = append(all, resourceEntry{
				URI:         r.URI,
				Name:        r.Name,
				MimeType:    r.MimeType,
				Description: r.Description,
				Server:      name,
			})
		}
	}

	if len(all) == 0 {
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: len(all) == 0")
		msg := "No resources found. MCP servers may still provide tools even if they have no resources."
		if len(errors) > 0 {
			observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: len(errors) > 0")
			msg += "\nErrors: " + strings.Join(errors, "; ")
		}
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "return: tool.InvokeResult{Content: msg}, nil")
		return tool.InvokeResult{Content: msg}, nil
	}

	data, _ := json.Marshal(all)
	result := string(data)
	if len(errors) > 0 {
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: len(errors) > 0")
		result += "\nErrors from some servers: " + strings.Join(errors, "; ")
	}
	observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "return: tool.InvokeResult{Content: result}, nil")
	return tool.InvokeResult{Content: result}, nil
}
