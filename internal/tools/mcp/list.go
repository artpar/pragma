package toolmcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/gogent/internal/mcp"
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

func (t *ListTool) Name() string                { return "ListMcpResourcesTool" }
func (t *ListTool) InputSchema() json.RawMessage { return listInputSchema }
func (t *ListTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *ListTool) Description() string {
	return "List resources available from connected MCP servers. Resources provide data the server wants to expose (files, database records, API responses, etc.)."
}

func (t *ListTool) CheckPerm(ctx context.Context, _ json.RawMessage, checker permission.Checker) permission.CheckResult {
	return checker.Check(ctx, "ListMcpResourcesTool", "")
}

func (t *ListTool) Invoke(ctx context.Context, input json.RawMessage, _ tool.StateSnapshot) (tool.InvokeResult, error) {
	var in listInput
	if len(input) > 0 {
		if err := json.Unmarshal(input, &in); err != nil {
			return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
		}
	}

	clients := t.Manager.Clients()
	if len(clients) == 0 {
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
		if in.Server != "" && name != in.Server {
			continue
		}
		if !client.Connected() {
			continue
		}

		resources, err := client.ListResources(ctx)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", name, err))
			continue
		}

		for _, r := range resources {
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
		msg := "No resources found. MCP servers may still provide tools even if they have no resources."
		if len(errors) > 0 {
			msg += "\nErrors: " + strings.Join(errors, "; ")
		}
		return tool.InvokeResult{Content: msg}, nil
	}

	data, _ := json.Marshal(all)
	result := string(data)
	if len(errors) > 0 {
		result += "\nErrors from some servers: " + strings.Join(errors, "; ")
	}
	return tool.InvokeResult{Content: result}, nil
}
