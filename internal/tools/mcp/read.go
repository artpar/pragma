package toolmcp

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/mcp"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/tool"
)

type readInput struct {
	Server string `json:"server" desc:"The MCP server name"`
	URI    string `json:"uri" desc:"The resource URI to read"`
}

var readInputSchema = json.RawMessage(`{
	"type": "object",
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
	CacheDir string // directory for persisting binary content (default: .gogent/cache)
}

func (t *ReadTool) Name() string                { return "ReadMcpResourceTool" }
func (t *ReadTool) InputSchema() json.RawMessage { return readInputSchema }
func (t *ReadTool) Flags() tool.ToolFlags {
	return tool.ToolFlags{ReadOnly: true, Concurrent: true}
}

func (t *ReadTool) Description() string {
	return "Read a specific resource from an MCP server by its URI. Returns text content inline or saves binary content to a local file."
}

func (t *ReadTool) CheckPerm(ctx context.Context, input json.RawMessage, checker permission.Checker) permission.CheckResult {
	var in readInput
	if err := json.Unmarshal(input, &in); err == nil && in.Server != "" {
		return checker.Check(ctx, "ReadMcpResourceTool", in.Server+":"+in.URI)
	}
	return checker.Check(ctx, "ReadMcpResourceTool", "")
}

func (t *ReadTool) Invoke(ctx context.Context, input json.RawMessage, snap tool.StateSnapshot) (tool.InvokeResult, error) {
	var in readInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tool.InvokeResult{}, fmt.Errorf("invalid input: %w", err)
	}
	if in.Server == "" {
		return tool.InvokeResult{}, fmt.Errorf("server is required")
	}
	if in.URI == "" {
		return tool.InvokeResult{}, fmt.Errorf("uri is required")
	}

	clients := t.Manager.Clients()
	client, ok := clients[in.Server]
	if !ok {
		available := make([]string, 0, len(clients))
		for name := range clients {
			available = append(available, name)
		}
		return tool.InvokeResult{Content: fmt.Sprintf("Server %q not found. Available servers: %s", in.Server, strings.Join(available, ", "))}, nil
	}

	if !client.Connected() {
		return tool.InvokeResult{Content: fmt.Sprintf("Server %q is not connected.", in.Server)}, nil
	}

	contents, err := client.ReadResource(ctx, in.URI)
	if err != nil {
		return tool.InvokeResult{Content: fmt.Sprintf("Error reading resource: %v", err)}, nil
	}

	type contentEntry struct {
		URI        string `json:"uri"`
		MimeType   string `json:"mime_type,omitempty"`
		Text       string `json:"text,omitempty"`
		BlobSavedTo string `json:"blob_saved_to,omitempty"`
	}

	result := struct {
		Contents []contentEntry `json:"contents"`
	}{}

	for i, c := range contents {
		entry := contentEntry{
			URI:      c.URI,
			MimeType: c.MimeType,
		}

		if c.Blob != "" {
			// Binary content — decode and persist to file
			decoded, decErr := base64.StdEncoding.DecodeString(c.Blob)
			if decErr != nil {
				entry.Text = fmt.Sprintf("Error decoding binary content: %v", decErr)
			} else {
				path, writeErr := t.persistBinary(snap, decoded, c.MimeType, i)
				if writeErr != nil {
					entry.Text = fmt.Sprintf("Error saving binary content: %v", writeErr)
				} else {
					entry.BlobSavedTo = path
				}
			}
		} else {
			entry.Text = c.Text
		}

		result.Contents = append(result.Contents, entry)
	}

	data, _ := json.Marshal(result)
	return tool.InvokeResult{Content: string(data)}, nil
}

func (t *ReadTool) persistBinary(snap tool.StateSnapshot, data []byte, mimeType string, index int) (string, error) {
	cacheDir := t.CacheDir
	if cacheDir == "" {
		cacheDir = filepath.Join(snap.WorkDir(), ".gogent", "cache")
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	ext := extensionForMIME(mimeType)
	randBytes := make([]byte, 4)
	_, _ = rand.Read(randBytes)
	filename := fmt.Sprintf("mcp-resource-%d-%d-%s%s", time.Now().UnixMilli(), index, hex.EncodeToString(randBytes), ext)
	path := filepath.Join(cacheDir, filename)

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return path, nil
}

func extensionForMIME(mimeType string) string {
	switch mimeType {
	case "application/pdf":
		return ".pdf"
	case "application/json":
		return ".json"
	case "text/csv":
		return ".csv"
	case "text/plain":
		return ".txt"
	case "text/html":
		return ".html"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/gif":
		return ".gif"
	case "image/svg+xml":
		return ".svg"
	case "image/webp":
		return ".webp"
	case "application/xml", "text/xml":
		return ".xml"
	case "application/zip":
		return ".zip"
	case "application/gzip":
		return ".gz"
	default:
		return ".bin"
	}
}
