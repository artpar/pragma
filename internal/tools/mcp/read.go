package toolmcp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
	"github.com/artpar/pragma/internal/toolresult"
)

type readInput struct {
	Server string `json:"server" desc:"The MCP server name"`
	URI    string `json:"uri" desc:"The resource URI to read"`
}

var readInputSchema = json.RawMessage(`{
	"type": "object",
	"additionalProperties": false,
	"required": ["server", "uri"],
	"properties": {
		"server": {
			"type": "string",
			"description": "The MCP server name that hosts the resource"
		},
		"uri": {
			"type": "string",
			"description": "The resource URI to read"
		}
	}
}`)

// ReadTool reads a specific MCP resource by URI.
type ReadTool struct {
	Manager *mcp.Manager
}

func (t *ReadTool) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"ReadMcpResourceTool\"")
	return "ReadMcpResourceTool"
}
func (t *ReadTool) InputSchema() json.RawMessage {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: readInputSchema")
	return readInputSchema
}
func (t *ReadTool) Flags() tool.ToolFlags {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: tool.ToolFlags{ReadOnly: true, Concurrent: true}")
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *ReadTool) Description() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: \"Reads a specific resource from an MCP server...\"")
	observe.GlobalTrace("return: readMcpDescription")
	return readMcpDescription
}

const readMcpDescription = `Reads a specific resource from an MCP server, identified by server name and resource URI.
- server: The name of the MCP server to read from
- uri: The URI of the resource to read

Usage examples:
- Read a resource: ReadMcpResource({ server: "myserver", uri: "my-resource-uri" })

Parameters:
- server (required): The name of the MCP server from which to read the resource
- uri (required): The URI of the resource to read`

func (t *ReadTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	observe.TraceCtx(ctx, "toolmcp", "ReadTool.CheckPerm", "enter")
	defer observe.TraceCtx(ctx, "toolmcp", "ReadTool.CheckPerm", "exit")
	var in readInput
	if err := json.Unmarshal(input, &in); err == nil && in.Server != "" {
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.CheckPerm", "if: err == nil && in.Server != \"\"")
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.CheckPerm", "return: checker.Check(ctx, \"ReadMcpResourceTool\", in.Server+\":\"+in.URI)")
		return checker.Check(ctx, "ReadMcpResourceTool", in.Server+":"+in.URI)
	}
	observe.TraceCtx(ctx, "toolmcp", "ReadTool.CheckPerm", "return: checker.Check(ctx, \"ReadMcpResourceTool\", \"\")")
	return checker.Check(ctx, "ReadMcpResourceTool", "")
}

func (t *ReadTool) Invoke(ctx context.Context, input json.RawMessage, snap tool.StateSnapshot) (tool.InvokeResult, error) {
	observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "enter")
	defer observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "exit")
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"invalid input: %w\", err)")
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Server == "" {
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "if: in.Server == \"\"")
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"server is required\")")
		return tool.InvokeResult{}, fmt.Errorf("server is required")
	}
	if in.URI == "" {
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "if: in.URI == \"\"")
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "return: tool.InvokeResult{}, fmt.Errorf(\"uri is required\")")
		return tool.InvokeResult{}, fmt.Errorf("uri is required")
	}

	clients := t.Manager.Clients()
	client, ok := clients[in.Server]
	if !ok {
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "if: !ok")
		available := make([]string, 0, len(clients))
		for name := range clients {
			observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "range clients")
			available = append(available, name)
		}
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Server %q not found. Available server...")
		return tool.InvokeResult{Content: fmt.Sprintf("Server %q not found. Available servers: %s", in.Server, strings.Join(available, ", "))}, nil
	}

	if !client.Connected() {
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "if: !client.Connected()")
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Server %q is not connected.\", in.Serv...")
		return tool.InvokeResult{Content: fmt.Sprintf("Server %q is not connected.", in.Server)}, nil
	}

	contents, err := client.ReadResource(ctx, in.URI)
	if err != nil {
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "if: err != nil")
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "return: tool.InvokeResult{Content: fmt.Sprintf(\"Error reading resource: %v\", err)}, nil")
		return tool.InvokeResult{Content: fmt.Sprintf("Error reading resource: %v", err)}, nil
	}

	type contentEntry struct {
		URI              string `json:"uri"`
		MimeType         string `json:"mime_type,omitempty"`
		Text             string `json:"text,omitempty"`
		BlobArtifactPath string `json:"blob_artifact_path,omitempty"`
		BlobBase64       string `json:"blob_base64,omitempty"`
		BlobBytes        int    `json:"blob_bytes,omitempty"`
	}

	result := struct {
		Contents []contentEntry `json:"contents"`
	}{}

	for i, c := range contents {
		observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "range contents")
		entry := contentEntry{
			URI:      c.URI,
			MimeType: c.MimeType,
		}

		if c.Blob != "" {
			observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "if: c.Blob != \"\"")

			decoded, decErr := base64.StdEncoding.DecodeString(c.Blob)
			if decErr != nil {
				observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "if: decErr != nil")
				entry.Text = fmt.Sprintf("Error decoding binary content: %v", decErr)
			} else {
				observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "else: decErr != nil")
				entry.BlobBytes = len(decoded)
				path, writeErr := t.persistBinaryArtifact(snap, in.Server, c.URI, decoded, c.MimeType, i)
				if writeErr != nil {
					observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "if: writeErr != nil")
					entry.BlobBase64 = c.Blob
					entry.Text = fmt.Sprintf("Binary content returned inline because no session artifact could be created: %v", writeErr)
				} else {
					observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "else: writeErr != nil")
					entry.BlobArtifactPath = path
				}
			}
		} else {
			observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "else: c.Blob != \"\"")
			entry.Text = c.Text
		}

		result.Contents = append(result.Contents, entry)
	}

	data, _ := json.Marshal(result)
	observe.TraceCtx(ctx, "toolmcp", "ReadTool.Invoke", "return: tool.InvokeResult{Content: string(data)}, nil")
	return tool.InvokeResult{Content: string(data)}, nil
}

func (t *ReadTool) persistBinaryArtifact(snap tool.StateSnapshot, server string, uri string, data []byte, mimeType string, index int) (string, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sessionID, ok := tool.SessionIDFrom(snap)
	if !ok {
		observe.GlobalTrace("if: !ok")
		observe.GlobalTrace("return: \"\", fmt.Errorf(\"session id is required\")")
		return "", fmt.Errorf("session id is required")
	}
	ext := extensionForMIME(mimeType)
	artifactID := fmt.Sprintf("mcp-resource-%s-%s-%d", server, uri, index)
	observe.GlobalTrace("return: toolresult.PersistBinaryOutput(data, artifactID, sessionID, ext)")
	return toolresult.PersistBinaryOutput(data, artifactID, sessionID, ext)
}

func extensionForMIME(mimeType string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	switch mimeType {
	case "application/pdf":
		observe.GlobalTrace("case: \"application/pdf\"")
		return ".pdf"
	case "application/json":
		observe.GlobalTrace("case: \"application/json\"")
		return ".json"
	case "text/csv":
		observe.GlobalTrace("case: \"text/csv\"")
		return ".csv"
	case "text/plain":
		observe.GlobalTrace("case: \"text/plain\"")
		return ".txt"
	case "text/html":
		observe.GlobalTrace("case: \"text/html\"")
		return ".html"
	case "image/png":
		observe.GlobalTrace("case: \"image/png\"")
		return ".png"
	case "image/jpeg":
		observe.GlobalTrace("case: \"image/jpeg\"")
		return ".jpg"
	case "image/gif":
		observe.GlobalTrace("case: \"image/gif\"")
		return ".gif"
	case "image/svg+xml":
		observe.GlobalTrace("case: \"image/svg+xml\"")
		return ".svg"
	case "image/webp":
		observe.GlobalTrace("case: \"image/webp\"")
		return ".webp"
	case "application/xml", "text/xml":
		observe.GlobalTrace("case: \"application/xml\", \"text/xml\"")
		return ".xml"
	case "application/zip":
		observe.GlobalTrace("case: \"application/zip\"")
		return ".zip"
	case "application/gzip":
		observe.GlobalTrace("case: \"application/gzip\"")
		return ".gz"
	default:
		observe.GlobalTrace("default")
		return ".bin"
	}
}
