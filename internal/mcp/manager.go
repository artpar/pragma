package mcp

import (
	"context"
	"fmt"
	"sync"

	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/tool"
)

const maxConcurrentStdio = 5

// Manager handles multiple MCP server connections.
type Manager struct {
	clients         map[string]*Client
	registeredTools map[string][]string // server name → registered tool names
	mu              sync.RWMutex
	bus             *observe.EventBus
	registry        *tool.Registry
}

// NewManager creates a Manager.
func NewManager(bus *observe.EventBus, registry *tool.Registry) *Manager {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Manager{\n\tclients:\t\tmake(map[string]*Client),\n\tregisteredTools:\tmake(map[str...")
	observe.GlobalTrace("return: &Manager{\n\tclients:\t\tmake(map[string]*Client),\n\tregisteredTools:\tmake(map[str...")
	return &Manager{
		clients:         make(map[string]*Client),
		registeredTools: make(map[string][]string),
		bus:             bus,
		registry:        registry,
	}
}

// ConnectAll connects to all servers from config.
// Stdio servers connect with a concurrency limit to prevent process exhaustion.
// Returns per-server errors (does not fail-fast).
func (m *Manager) ConnectAll(ctx context.Context, servers map[string]ServerConfig) map[string]error {
	observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "exit")
	errs := make(map[string]error)
	var mu sync.Mutex

	// Partition into stdio and remote for different concurrency limits
	type namedServer struct {
		name   string
		config ServerConfig
	}
	var stdioServers, remoteServers []namedServer

	for name, cfg := range servers {
		observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "range servers")
		ns := namedServer{name: name, config: cfg}
		if cfg.effectiveType() == "stdio" {
			observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "if: cfg.effectiveType() == \"stdio\"")
			stdioServers = append(stdioServers, ns)
		} else {
			observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "else: cfg.effectiveType() == \"stdio\"")
			remoteServers = append(remoteServers, ns)
		}
	}

	var wg sync.WaitGroup

	if len(stdioServers) > 0 {
		observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "if: len(stdioServers) > 0")
		sem := make(chan struct{}, maxConcurrentStdio)
		for _, ns := range stdioServers {
			observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "range stdioServers")
			wg.Add(1)
			go func(ns namedServer) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				client := NewClient(ns.name, ns.config, m.bus)
				if err := client.Connect(ctx); err != nil {
					observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "if: err != nil")
					mu.Lock()
					errs[ns.name] = err
					mu.Unlock()
					return
				}
				m.mu.Lock()
				m.clients[ns.name] = client
				m.mu.Unlock()
			}(ns)
		}
	}

	for _, ns := range remoteServers {
		observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "range remoteServers")
		wg.Add(1)
		go func(ns namedServer) {
			defer wg.Done()
			client := NewClient(ns.name, ns.config, m.bus)
			if err := client.Connect(ctx); err != nil {
				observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "if: err != nil")
				mu.Lock()
				errs[ns.name] = err
				mu.Unlock()
				return
			}
			m.mu.Lock()
			m.clients[ns.name] = client
			m.mu.Unlock()
		}(ns)
	}

	wg.Wait()
	observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "return: errs")
	observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "return: errs")
	return errs
}

// RegisterTools lists tools from all connected servers and registers
// MCPToolAdapters in the tool.Registry.
func (m *Manager) RegisterTools(ctx context.Context) error {
	observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "exit")
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "range m.clients")
		if !client.Connected() {
			observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "if: !client.Connected()")
			continue
		}

		tools, err := client.ListTools(ctx)
		if err != nil {
			observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "if: err != nil")
			m.bus.Emit(observe.ErrorOccurred{
				EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
				Severity:     "warn",
				Component:    "mcp",
				ErrorType:    "list_tools_error",
				ErrorMessage: fmt.Sprintf("failed to list tools from %q: %v", client.Name(), err),
			})
			continue
		}

		var registered []string
		for _, info := range tools {
			observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "range tools")
			adapter := NewMCPToolAdapter(client, info)
			if err := m.registry.Register(adapter); err != nil {
				observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "if: err != nil")
				m.bus.Emit(observe.ErrorOccurred{
					EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
					Severity:     "warn",
					Component:    "mcp",
					ErrorType:    "register_tool_error",
					ErrorMessage: fmt.Sprintf("failed to register mcp tool %q: %v", adapter.Name(), err),
				})
			} else {
				observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "else: err != nil")
				registered = append(registered, adapter.Name())
			}
		}
		m.registeredTools[name] = registered
	}
	observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "return: nil")
	observe.TraceCtx(ctx, "mcp", "Manager.RegisterTools", "return: nil")

	return nil
}

// DisconnectAll disconnects all servers and unregisters their tools.
// Uses tracked tool names from registration — no re-listing required.
func (m *Manager) DisconnectAll() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, client := range m.clients {
		observe.GlobalTrace("range m.clients")

		for _, toolName := range m.registeredTools[name] {
			observe.GlobalTrace("range m.registeredTools[name]")
			m.registry.Unregister(toolName)
		}
		delete(m.registeredTools, name)
		_ = client.Disconnect()
	}
	m.clients = make(map[string]*Client)
}

// ServerStatus returns connection status for each server.
func (m *Manager) ServerStatus() map[string]string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]string, len(m.clients))
	for name, client := range m.clients {
		observe.GlobalTrace("range m.clients")
		if client.Connected() {
			observe.GlobalTrace("if: client.Connected()")
			status[name] = "connected"
		} else {
			observe.GlobalTrace("else: client.Connected()")
			status[name] = "disconnected"
		}
	}
	observe.GlobalTrace("return: status")
	observe.GlobalTrace("return: status")
	return status
}

// ConnectedCount returns the number of connected servers.
func (m *Manager) ConnectedCount() int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, client := range m.clients {
		observe.GlobalTrace("range m.clients")
		if client.Connected() {
			observe.GlobalTrace("if: client.Connected()")
			count++
		}
	}
	observe.GlobalTrace("return: count")
	observe.GlobalTrace("return: count")
	return count
}

// Clients returns a snapshot of all connected clients keyed by server name.
func (m *Manager) Clients() map[string]*Client {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*Client, len(m.clients))
	for name, client := range m.clients {
		observe.GlobalTrace("range m.clients")
		result[name] = client
	}
	observe.GlobalTrace("return: result")
	observe.GlobalTrace("return: result")
	return result
}
