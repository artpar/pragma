package web

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/tool"
)

// Bridge implements permission.Prompter and tool.Asker for web API clients.
type Bridge struct {
	mu          sync.Mutex
	hub         *hub
	permissions map[string]chan permissionResponse
	asks        map[string]chan tool.AskResponse
}

// NewBridge creates a web permission and ask bridge.
func NewBridge() *Bridge {
	return &Bridge{
		permissions: make(map[string]chan permissionResponse),
		asks:        make(map[string]chan tool.AskResponse),
	}
}

func (b *Bridge) attach(h *hub) {
	b.mu.Lock()
	b.hub = h
	b.mu.Unlock()
}

type permissionResponse struct {
	Decision permission.Decision
	Scope    permission.RememberScope
}

func (b *Bridge) Prompt(ctx context.Context, toolName string, toolInput json.RawMessage, content string, reason string) (permission.Decision, permission.RememberScope) {
	id := newID()
	respCh := make(chan permissionResponse, 1)
	b.mu.Lock()
	b.permissions[id] = respCh
	h := b.hub
	b.mu.Unlock()
	if h != nil {
		h.publish("permission_request", map[string]interface{}{
			"id": id, "tool": toolName, "input": json.RawMessage(toolInput),
			"content": content, "reason": reason,
		})
	}
	select {
	case resp := <-respCh:
		return resp.Decision, resp.Scope
	case <-ctx.Done():
		b.dropPermission(id)
		if h != nil {
			h.publish("permission_expired", map[string]interface{}{
				"id": id, "tool": toolName, "input": json.RawMessage(toolInput),
				"content": content, "reason": reason, "error": ctx.Err().Error(),
			})
		}
		return permission.DecisionDeny, permission.RememberNone
	}
}

func (b *Bridge) Ask(ctx context.Context, req tool.AskRequest) (tool.AskResponse, error) {
	id := newID()
	respCh := make(chan tool.AskResponse, 1)
	b.mu.Lock()
	b.asks[id] = respCh
	h := b.hub
	b.mu.Unlock()
	if h != nil {
		h.publish("ask_request", map[string]interface{}{"id": id, "request": req})
	}
	select {
	case resp := <-respCh:
		return resp, nil
	case <-ctx.Done():
		b.dropAsk(id)
		if h != nil {
			h.publish("ask_expired", map[string]interface{}{"id": id, "request": req, "error": ctx.Err().Error()})
		}
		return tool.AskResponse{}, ctx.Err()
	}
}

func (b *Bridge) resolvePermission(id string, resp permissionResponse) bool {
	b.mu.Lock()
	ch := b.permissions[id]
	delete(b.permissions, id)
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- resp
	return true
}

func (b *Bridge) resolveAsk(id string, resp tool.AskResponse) bool {
	b.mu.Lock()
	ch := b.asks[id]
	delete(b.asks, id)
	b.mu.Unlock()
	if ch == nil {
		return false
	}
	ch <- resp
	return true
}

func (b *Bridge) dropPermission(id string) {
	b.mu.Lock()
	delete(b.permissions, id)
	b.mu.Unlock()
}

func (b *Bridge) dropAsk(id string) {
	b.mu.Lock()
	delete(b.asks, id)
	b.mu.Unlock()
}
