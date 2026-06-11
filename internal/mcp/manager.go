package mcp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/artpar/pragma/internal/observe"
)

const maxConcurrentStdio = 5

const (
	StatusConnected    = "connected"
	StatusDisconnected = "disconnected"
	StatusFailed       = "failed"
	StatusNeedsAuth    = "needs-auth"
	StatusPending      = "pending"
	StatusDisabled     = "disabled"
)

// ServerStatusInfo is the serialized MCP server connection state.
type ServerStatusInfo struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	ToolCount int    `json:"tool_count,omitempty"`
	Transport string `json:"transport,omitempty"`
}

// Manager handles multiple MCP server connections.
type Manager struct {
	clients      map[string]*Client
	configs      map[string]ServerConfig // stored for reconnection
	statuses     map[string]string
	lastErrors   map[string]string
	mu           sync.RWMutex
	bus          *observe.EventBus
	lifecycleCtx context.Context
	generation   uint64
	stopped      bool
}

// NewManager creates a Manager.
func NewManager(bus *observe.EventBus) *Manager {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Manager{\n\tclients:\t\tmake(map[string]*Client),\n\tconfigs:\t\tmake(map[string]Ser...")
	observe.GlobalTrace("return: &Manager{\n\tclients:\tmake(map[string]*Client),\n\tconfigs:\tmake(map[string]Serve...")
	return &Manager{
		clients:      make(map[string]*Client),
		configs:      make(map[string]ServerConfig),
		statuses:     make(map[string]string),
		lastErrors:   make(map[string]string),
		bus:          bus,
		lifecycleCtx: context.Background(),
	}
}

// SetLifecycleContext scopes manager-owned async work such as OAuth callbacks.
func (m *Manager) SetLifecycleContext(ctx context.Context) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if ctx == nil {
		observe.GlobalTrace("if: ctx == nil")
		ctx = context.Background()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lifecycleCtx = ctx
}

func (m *Manager) LifecycleContext() context.Context {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.lifecycleCtx == nil {
		observe.GlobalTrace("if: m.lifecycleCtx == nil")
		observe.GlobalTrace("return: context.Background()")
		return context.Background()
	}
	observe.GlobalTrace("return: m.lifecycleCtx")
	return m.lifecycleCtx
}

// ConfigureServers records MCP servers and marks them pending without opening
// transports. Call this before background connection so status metadata is
// available immediately at startup.
func (m *Manager) ConfigureServers(servers map[string]ServerConfig) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, cfg := range servers {
		observe.GlobalTrace("range servers")
		m.configs[name] = cfg
		m.statuses[name] = StatusPending
		delete(m.lastErrors, name)
	}
}

// ReplaceServers makes the provided server set the authoritative MCP scope.
// Existing clients are disconnected first.
func (m *Manager) ReplaceServers(servers map[string]ServerConfig) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = false
	m.generation++

	for _, client := range m.clients {
		observe.GlobalTrace("range m.clients")
		_ = client.Disconnect()
	}

	m.clients = make(map[string]*Client)
	m.configs = make(map[string]ServerConfig, len(servers))
	m.statuses = make(map[string]string, len(servers))
	m.lastErrors = make(map[string]string)
	for name, cfg := range servers {
		observe.GlobalTrace("range servers")
		m.configs[name] = cfg
		m.statuses[name] = StatusPending
	}
}

func (m *Manager) activeGenerationLocked(generation uint64) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: !m.stopped && m.generation == generation")
	return !m.stopped && m.generation == generation
}

func (m *Manager) markDisconnectedIfActive(generation uint64, name string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.activeGenerationLocked(generation) {
		observe.GlobalTrace("if: !m.activeGenerationLocked(generation)")
		return
	}
	if _, configured := m.configs[name]; configured {
		observe.GlobalTrace("if: configured")
		m.statuses[name] = StatusDisconnected
	}
}

func (m *Manager) recordConnectFailure(generation uint64, name string, err error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.activeGenerationLocked(generation) {
		observe.GlobalTrace("if: !m.activeGenerationLocked(generation)")
		return
	}
	m.statuses[name] = StatusFailed
	m.lastErrors[name] = err.Error()
}

func (m *Manager) publishConnectedClient(ctx context.Context, generation uint64, name string, client *Client) bool {
	observe.TraceCtx(ctx, "mcp", "Manager.publishConnectedClient", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.publishConnectedClient", "exit")
	if ctx.Err() != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.publishConnectedClient", "if: ctx.Err() != nil")
		m.markDisconnectedIfActive(generation, name)
		_ = client.Disconnect()
		observe.TraceCtx(ctx, "mcp", "Manager.publishConnectedClient", "return: false")
		return false
	}
	m.mu.Lock()
	if !m.activeGenerationLocked(generation) {
		observe.TraceCtx(ctx, "mcp", "Manager.publishConnectedClient", "if: !m.activeGenerationLocked(generation)")
		m.mu.Unlock()
		_ = client.Disconnect()
		observe.TraceCtx(ctx, "mcp", "Manager.publishConnectedClient", "return: false")
		return false
	}
	m.clients[name] = client
	m.statuses[name] = StatusConnected
	delete(m.lastErrors, name)
	m.mu.Unlock()
	observe.TraceCtx(ctx, "mcp", "Manager.publishConnectedClient", "return: true")
	return true
}

// ConnectAll connects to all servers from config.
// Stdio servers connect with a concurrency limit to prevent process exhaustion.
// Returns per-server errors (does not fail-fast).
func (m *Manager) ConnectAll(ctx context.Context, servers map[string]ServerConfig) map[string]error {
	observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "exit")
	observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "return: m.connectAll(ctx, servers)")
	return m.connectAll(ctx, servers)
}

func (m *Manager) connectAll(ctx context.Context, servers map[string]ServerConfig) map[string]error {
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

	m.mu.Lock()
	m.stopped = false
	m.generation++
	generation := m.generation
	for name, cfg := range servers {
		observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "range servers")
		m.configs[name] = cfg
		m.statuses[name] = StatusPending
		delete(m.lastErrors, name)
		ns := namedServer{name: name, config: cfg}
		if cfg.effectiveType() == "stdio" {
			observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "if: cfg.effectiveType() == \"stdio\"")
			stdioServers = append(stdioServers, ns)
		} else {
			observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "else: cfg.effectiveType() == \"stdio\"")
			remoteServers = append(remoteServers, ns)
		}
	}
	m.mu.Unlock()

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
					observe.TraceCtx(ctx, "mcp", "Manager.connectAll", "if: err != nil")
					if errors.Is(ctx.Err(), context.Canceled) {
						observe.TraceCtx(ctx, "mcp", "Manager.connectAll", "if: errors.Is(ctx.Err(), context.Canceled)")
						m.markDisconnectedIfActive(generation, ns.name)
						return
					}
					observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "if: err != nil")
					m.bus.Emit(observe.ErrorOccurred{
						EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
						Severity:     "error",
						Component:    "mcp",
						ErrorType:    "server_connect_failed",
						ErrorMessage: fmt.Sprintf("MCP server %q failed to connect: %v", ns.name, err),
					})
					mu.Lock()
					errs[ns.name] = err
					mu.Unlock()
					m.recordConnectFailure(generation, ns.name, err)
					return
				}
				if !m.publishConnectedClient(ctx, generation, ns.name, client) {
					observe.TraceCtx(ctx, "mcp", "Manager.connectAll", "if: !m.publishConnectedClient(ctx, generation, ns.name, client)")
					return
				}
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
				observe.TraceCtx(ctx, "mcp", "Manager.connectAll", "if: err != nil")
				if errors.Is(ctx.Err(), context.Canceled) {
					observe.TraceCtx(ctx, "mcp", "Manager.connectAll", "if: errors.Is(ctx.Err(), context.Canceled)")
					m.markDisconnectedIfActive(generation, ns.name)
					return
				}
				observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "if: err != nil")
				mu.Lock()
				errs[ns.name] = err
				mu.Unlock()
				m.recordConnectFailure(generation, ns.name, err)
				return
			}
			if !m.publishConnectedClient(ctx, generation, ns.name, client) {
				observe.TraceCtx(ctx, "mcp", "Manager.connectAll", "if: !m.publishConnectedClient(ctx, generation, ns.name, client)")
				return
			}
		}(ns)
	}

	wg.Wait()
	observe.TraceCtx(ctx, "mcp", "Manager.ConnectAll", "return: errs")
	return errs
}

// DisconnectAll disconnects all servers.
func (m *Manager) DisconnectAll() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	m.generation++

	for name, client := range m.clients {
		observe.GlobalTrace("range m.clients")
		_ = client.Disconnect()
		if _, configured := m.configs[name]; configured {
			observe.GlobalTrace("if: configured")
			m.statuses[name] = StatusDisconnected
		}
	}
	m.clients = make(map[string]*Client)
}

// ServerStatus returns connection status for each server.
func (m *Manager) ServerStatus() map[string]string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()

	status := make(map[string]string, len(m.configs))
	for name := range m.configs {
		observe.GlobalTrace("range m.configs")
		if st := m.statuses[name]; st != "" {
			observe.GlobalTrace("if: st != \"\"")
			status[name] = st
		} else {
			observe.GlobalTrace("else: st != \"\"")
			status[name] = StatusDisconnected
		}
	}
	for name, client := range m.clients {
		observe.GlobalTrace("range m.clients")
		if client.Connected() {
			observe.GlobalTrace("if: client.Connected()")
			status[name] = StatusConnected
		} else if status[name] == "" || status[name] == StatusConnected {
			observe.GlobalTrace("else: client.Connected()")
			status[name] = StatusDisconnected
		}
	}
	observe.GlobalTrace("return: status")
	return status
}

// ServerStatuses returns sorted structured connection status for each configured server.
func (m *Manager) ServerStatuses() []ServerStatusInfo {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.configs))
	for name := range m.configs {
		observe.GlobalTrace("range m.configs")
		names = append(names, name)
	}
	sort.Strings(names)

	statuses := make([]ServerStatusInfo, 0, len(names))
	for _, name := range names {
		observe.GlobalTrace("range names")
		cfg := m.configs[name]
		st := m.statuses[name]
		if st == "" {
			observe.GlobalTrace("if: st == \"\"")
			st = StatusDisconnected
		}
		if client := m.clients[name]; client != nil && client.Connected() {
			observe.GlobalTrace("if: client != nil && client.Connected()")
			st = StatusConnected
		}
		statuses = append(statuses, ServerStatusInfo{
			Name:      name,
			Status:    st,
			Error:     m.lastErrors[name],
			ToolCount: 0,
			Transport: cfg.effectiveType(),
		})
	}
	observe.GlobalTrace("return: statuses")
	return statuses
}

// PendingServerNames returns configured MCP servers that are still connecting.
func (m *Manager) PendingServerNames() []string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	var names []string
	for name, st := range m.statuses {
		observe.GlobalTrace("range m.statuses")
		if st == StatusPending {
			observe.GlobalTrace("if: st == StatusPending")
			names = append(names, name)
		}
	}
	sort.Strings(names)
	observe.GlobalTrace("return: names")
	return names
}

func (m *Manager) hasPendingServers() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, st := range m.statuses {
		observe.GlobalTrace("range m.statuses")
		if st == StatusPending {
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
}

// HasConnectedResourceServer reports whether any connected server supports MCP resources.
func (m *Manager) HasConnectedResourceServer() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	clients := make([]*Client, 0, len(m.clients))
	for _, client := range m.clients {
		observe.GlobalTrace("range m.clients")
		clients = append(clients, client)
	}
	m.mu.RUnlock()
	for _, client := range clients {
		observe.GlobalTrace("range clients")
		if client.Connected() && client.SupportsResources() {
			observe.GlobalTrace("if: client.Connected() && client.SupportsResources()")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	return false
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
	return count
}

// ReconnectServer disconnects and reconnects a single server.
func (m *Manager) ReconnectServer(ctx context.Context, name string) error {
	observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "exit")
	_, err := m.reconnectServer(ctx, name)
	observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "return: err")
	return err
}

func (m *Manager) reconnectServer(ctx context.Context, name string) (*Client, error) {
	observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "enter")
	defer observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "exit")

	m.mu.Lock()
	cfg, ok := m.configs[name]
	if !ok {
		observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "if: !ok")
		m.mu.Unlock()
		observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "return: fmt.Errorf(\"server %q not found in config\", name)")
		observe.TraceCtx(ctx, "mcp", "Manager.reconnectServer", "return: nil, fmt.Errorf(\"server %q not found in config\", name)")
		return nil, fmt.Errorf("server %q not found in config", name)
	}
	generation := m.generation
	if m.stopped {
		observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "if: m.stopped")
		m.mu.Unlock()
		observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "return: fmt.Errorf(\"mcp manager stopped\")")
		observe.TraceCtx(ctx, "mcp", "Manager.reconnectServer", "return: nil, fmt.Errorf(\"mcp manager stopped\")")
		return nil, fmt.Errorf("mcp manager stopped")
	}

	if old, exists := m.clients[name]; exists {
		observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "if: exists")
		old.Disconnect()
		delete(m.clients, name)
	}

	m.statuses[name] = StatusPending
	delete(m.lastErrors, name)
	m.mu.Unlock()

	client := NewClient(name, cfg, m.bus)
	if err := client.Connect(ctx); err != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "if: err != nil")
		m.recordConnectFailure(generation, name, err)
		observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "return: fmt.Errorf(\"reconnect %q: %w\", name, err)")
		observe.TraceCtx(ctx, "mcp", "Manager.reconnectServer", "return: nil, fmt.Errorf(\"reconnect %q: %w\", name, err)")
		return nil, fmt.Errorf("reconnect %q: %w", name, err)
	}

	m.mu.Lock()
	if !m.activeGenerationLocked(generation) || ctx.Err() != nil {
		observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "if: inactive generation or context done")
		m.mu.Unlock()
		_ = client.Disconnect()
		if err := ctx.Err(); err != nil {
			observe.TraceCtx(ctx, "mcp", "Manager.reconnectServer", "if: err != nil")
			observe.TraceCtx(ctx, "mcp", "Manager.reconnectServer", "return: nil, err")
			return nil, err
		}
		observe.TraceCtx(ctx, "mcp", "Manager.reconnectServer", "return: nil, fmt.Errorf(\"mcp manager stopped\")")
		return nil, fmt.Errorf("mcp manager stopped")
	}
	m.clients[name] = client
	m.statuses[name] = StatusConnected
	delete(m.lastErrors, name)
	m.mu.Unlock()

	observe.TraceCtx(ctx, "mcp", "Manager.ReconnectServer", "return: nil")
	observe.TraceCtx(ctx, "mcp", "Manager.reconnectServer", "return: client, nil")
	return client, nil
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
	return result
}
