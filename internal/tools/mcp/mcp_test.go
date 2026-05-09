package toolmcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/tool"
)

func setupManager(t *testing.T) *mcp.Manager {
	t.Helper()
	bus := observe.NewEventBus(16)
	t.Cleanup(func() { bus.Drain() })
	reg := tool.NewRegistry(bus)
	return mcp.NewManager(bus, reg)
}

func TestListTool_Name(t *testing.T) {
	tl := &ListTool{}
	if tl.Name() != "ListMcpResourcesTool" {
		t.Fatalf("expected ListMcpResourcesTool, got %s", tl.Name())
	}
}

func TestListTool_Flags(t *testing.T) {
	tl := &ListTool{}
	if !tl.Flags().ReadOnly {
		t.Fatal("ListMcpResourcesTool should be ReadOnly")
	}
}

func TestReadTool_Name(t *testing.T) {
	tl := &ReadTool{}
	if tl.Name() != "ReadMcpResourceTool" {
		t.Fatalf("expected ReadMcpResourceTool, got %s", tl.Name())
	}
}

func TestReadTool_Flags(t *testing.T) {
	tl := &ReadTool{}
	if !tl.Flags().ReadOnly {
		t.Fatal("ReadMcpResourceTool should be ReadOnly")
	}
}

func TestListTool_Invoke_NoServers(t *testing.T) {
	mgr := setupManager(t)
	tl := &ListTool{Manager: mgr}

	result, err := tl.Invoke(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "No resources found") {
		t.Fatalf("expected 'No resources found', got %q", result.Content)
	}
}

func TestListTool_DescriptionMatchesResourceScope(t *testing.T) {
	tl := &ListTool{}
	desc := tl.Description()
	if !strings.Contains(desc, "Lists available resources from configured MCP servers") {
		t.Fatalf("description should describe MCP resource listing, got %q", desc)
	}
}

func TestListTool_Invoke_InvalidJSON(t *testing.T) {
	mgr := setupManager(t)
	tl := &ListTool{Manager: mgr}

	_, err := tl.Invoke(context.Background(), json.RawMessage(`{bad`), nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestReadTool_Invoke_MissingServer(t *testing.T) {
	mgr := setupManager(t)
	tl := &ReadTool{Manager: mgr}

	result, err := tl.Invoke(context.Background(), json.RawMessage(`{"server":"nonexistent","uri":"file:///test"}`), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Content, "not found") {
		t.Fatalf("expected 'not found' in content, got %q", result.Content)
	}
}

func TestReadTool_Invoke_EmptyServer(t *testing.T) {
	mgr := setupManager(t)
	tl := &ReadTool{Manager: mgr}

	_, err := tl.Invoke(context.Background(), json.RawMessage(`{"server":"","uri":"file:///test"}`), nil)
	if err == nil {
		t.Fatal("expected error for empty server")
	}
	if !strings.Contains(err.Error(), "server is required") {
		t.Fatalf("expected 'server is required', got %q", err.Error())
	}
}

func TestReadTool_Invoke_EmptyURI(t *testing.T) {
	mgr := setupManager(t)
	tl := &ReadTool{Manager: mgr}

	_, err := tl.Invoke(context.Background(), json.RawMessage(`{"server":"test","uri":""}`), nil)
	if err == nil {
		t.Fatal("expected error for empty URI")
	}
	if !strings.Contains(err.Error(), "uri is required") {
		t.Fatalf("expected 'uri is required', got %q", err.Error())
	}
}

func TestReadTool_Invoke_InvalidJSON(t *testing.T) {
	mgr := setupManager(t)
	tl := &ReadTool{Manager: mgr}

	_, err := tl.Invoke(context.Background(), json.RawMessage(`{bad`), nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestExtensionForMIME(t *testing.T) {
	tests := []struct {
		mimeType string
		expect   string
	}{
		{"application/pdf", ".pdf"},
		{"application/json", ".json"},
		{"text/csv", ".csv"},
		{"text/plain", ".txt"},
		{"text/html", ".html"},
		{"image/png", ".png"},
		{"image/jpeg", ".jpg"},
		{"image/gif", ".gif"},
		{"image/svg+xml", ".svg"},
		{"image/webp", ".webp"},
		{"application/xml", ".xml"},
		{"text/xml", ".xml"},
		{"application/zip", ".zip"},
		{"application/gzip", ".gz"},
		{"application/octet-stream", ".bin"},
		{"unknown/type", ".bin"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := extensionForMIME(tt.mimeType)
			if got != tt.expect {
				t.Fatalf("expected %q, got %q", tt.expect, got)
			}
		})
	}
}
