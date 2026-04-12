package mcp

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/artpar/gogent/internal/observe"
)

// ResourceInfo describes an MCP resource exposed by a server.
type ResourceInfo struct {
	URI         string `json:"uri"`
	Name        string `json:"name"`
	MimeType    string `json:"mimeType,omitempty"`
	Description string `json:"description,omitempty"`
}

// ResourceContent holds one piece of content returned by ReadResource.
type ResourceContent struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
	Blob     string `json:"blob,omitempty"` // base64-encoded binary
}

// ListResources returns the resources offered by this server.
func (c *Client) ListResources(ctx context.Context) ([]ResourceInfo, error) {
	observe.TraceCtx(ctx, "mcp", "Client.ListResources", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Client.ListResources", "exit")
	c.mu.Lock()
	if !c.connected || c.mcpCli == nil {
		observe.TraceCtx(ctx, "mcp", "Client.ListResources", "if: !c.connected || c.mcpCli == nil")
		c.mu.Unlock()
		observe.TraceCtx(ctx, "mcp", "Client.ListResources", "return: nil, fmt.Errorf(\"%w: %s\", ErrServerNotConnected, c.name)")
		observe.TraceCtx(ctx, "mcp", "Client.ListResources", "return: nil, fmt.Errorf(\"%w: %s\", ErrServerNotConnected, c.name)")
		observe.TraceCtx(ctx, "mcp", "Client.ListResources", "return: nil, fmt.Errorf(\"%w: %s\", ErrServerNotConnected, c.name)")
		return nil, fmt.Errorf("%w: %s", ErrServerNotConnected, c.name)
	}
	cli := c.mcpCli
	c.mu.Unlock()

	result, err := cli.ListResources(ctx, mcp.ListResourcesRequest{})
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Client.ListResources", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "Client.ListResources", "return: nil, fmt.Errorf(\"list resources from %q: %w\", c.name, err)")
		observe.TraceCtx(ctx, "mcp", "Client.ListResources", "return: nil, fmt.Errorf(\"list resources from %q: %w\", c.name, err)")
		observe.TraceCtx(ctx, "mcp", "Client.ListResources", "return: nil, fmt.Errorf(\"list resources from %q: %w\", c.name, err)")
		return nil, fmt.Errorf("list resources from %q: %w", c.name, err)
	}

	resources := make([]ResourceInfo, 0, len(result.Resources))
	for _, r := range result.Resources {
		observe.TraceCtx(ctx, "mcp", "Client.ListResources", "range result.Resources")
		resources = append(resources, ResourceInfo{
			URI:         r.URI,
			Name:        r.Name,
			MimeType:    r.MIMEType,
			Description: r.Description,
		})
	}
	observe.TraceCtx(ctx, "mcp", "Client.ListResources", "return: resources, nil")
	observe.TraceCtx(ctx, "mcp", "Client.ListResources", "return: resources, nil")
	observe.TraceCtx(ctx, "mcp", "Client.ListResources", "return: resources, nil")
	return resources, nil
}

// ReadResource reads a resource by URI from this server.
func (c *Client) ReadResource(ctx context.Context, uri string) ([]ResourceContent, error) {
	observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "exit")
	c.mu.Lock()
	if !c.connected || c.mcpCli == nil {
		observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "if: !c.connected || c.mcpCli == nil")
		c.mu.Unlock()
		observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "return: nil, fmt.Errorf(\"%w: %s\", ErrServerNotConnected, c.name)")
		observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "return: nil, fmt.Errorf(\"%w: %s\", ErrServerNotConnected, c.name)")
		observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "return: nil, fmt.Errorf(\"%w: %s\", ErrServerNotConnected, c.name)")
		return nil, fmt.Errorf("%w: %s", ErrServerNotConnected, c.name)
	}
	cli := c.mcpCli
	c.mu.Unlock()

	result, err := cli.ReadResource(ctx, mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: uri},
	})
	if err != nil {
		observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "if: err != nil")
		observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "return: nil, fmt.Errorf(\"read resource %q from %q: %w\", uri, c.name, err)")
		observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "return: nil, fmt.Errorf(\"read resource %q from %q: %w\", uri, c.name, err)")
		observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "return: nil, fmt.Errorf(\"read resource %q from %q: %w\", uri, c.name, err)")
		return nil, fmt.Errorf("read resource %q from %q: %w", uri, c.name, err)
	}

	contents := make([]ResourceContent, 0, len(result.Contents))
	for _, content := range result.Contents {
		observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "range result.Contents")
		switch c := content.(type) {
		case mcp.TextResourceContents:
			observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "typecase: mcp.TextResourceContents")
			contents = append(contents, ResourceContent{
				URI:      c.URI,
				MimeType: c.MIMEType,
				Text:     c.Text,
			})
		case mcp.BlobResourceContents:
			observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "typecase: mcp.BlobResourceContents")
			contents = append(contents, ResourceContent{
				URI:      c.URI,
				MimeType: c.MIMEType,
				Blob:     c.Blob,
			})
		}
	}
	observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "return: contents, nil")
	observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "return: contents, nil")
	observe.TraceCtx(ctx, "mcp", "Client.ReadResource", "return: contents, nil")
	return contents, nil
}
