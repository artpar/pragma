package toolmcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

type listInput struct {
	Server string `json:"server" desc:"Optional server name to filter resources by"`
}

var listInputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
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
	observe.GlobalTrace("return: \"Lists available resources from configured MCP servers...\"")
	observe.GlobalTrace("return: listMcpDescription")
	return listMcpDescription
}

const listMcpDescription = `Lists available resources from configured MCP servers. Each resource object includes a 'server' field indicating which server it's from.

Usage examples:
- List all resources from all servers: ListMcpResources
- List resources from a specific server: ListMcpResources({ server: "myserver" })

Parameters:
- server (optional): The name of a specific MCP server to get resources from. If not provided, resources from all servers will be returned.`

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
	status := t.Manager.ServerStatus()
	if in.Server != "" {
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: in.Server != \"\"")
		if _, ok := clients[in.Server]; !ok {
			observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: in.Server not connected")
			if _, configured := status[in.Server]; !configured {
				observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "if: !configured")
				var available []string
				for name := range status {
					observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "range ServerStatus")
					available = append(available, name)
				}
				observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "return: not found")
				observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"server %q not found. Available servers: %v\",...")
				return tool.InvokeResult{}, fmt.Errorf("server %q not found. Available servers: %v", in.Server, available)
			}
		}
	}

	type resourceEntry struct {
		URI         string `json:"uri"`
		Name        string `json:"name"`
		MimeType    string `json:"mime_type,omitempty"`
		Description string `json:"description,omitempty"`
		Server      string `json:"server"`
	}

	var all []resourceEntry

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
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "return: tool.InvokeResult{Content: no resources}, nil")
		observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "return: tool.InvokeResult{Content: \"No resources found. MCP servers may still provide...")
		return tool.InvokeResult{Content: "No resources found. MCP servers may still provide tools even if they have no resources."}, nil
	}

	data, _ := json.Marshal(all)
	result := string(data)
	observe.TraceCtx(ctx, "toolmcp", "ListTool.Invoke", "return: tool.InvokeResult{Content: result}, nil")
	return tool.InvokeResult{Content: result}, nil
}
