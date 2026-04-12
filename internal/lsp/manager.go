package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/artpar/gogent/internal/observe"
)

// Manager routes LSP requests to the correct server based on file extension.
// Servers are started lazily on first use.
type Manager struct {
	servers      map[string]*Server
	extensionMap map[string]string // ".go" -> server name
	workDir      string
	bus          *observe.EventBus
	Diagnostics  *DiagnosticRegistry

	mu          sync.RWMutex
	openedFiles map[string]string // file URI -> server name
	fileVersion sync.Map          // file URI -> *atomic.Int64
}

// NewManager creates an LSP manager.
func NewManager(bus *observe.EventBus) *Manager {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Manager{...}")
	observe.GlobalTrace("return: &Manager{\n\tservers:\tmake(map[string]*Server),\n\textensionMap:\tmake(map[string]...")
	observe.GlobalTrace("return: &Manager{\n\tservers:\tmake(map[string]*Server),\n\textensionMap:\tmake(map[string]...")
	return &Manager{
		servers:      make(map[string]*Server),
		extensionMap: make(map[string]string),
		openedFiles:  make(map[string]string),
		bus:          bus,
		Diagnostics:  NewDiagnosticRegistry(),
	}
}

// Initialize sets up servers and extension routing from config.
func (m *Manager) Initialize(configs map[string]ServerConfig, workDir string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.Lock()
	defer m.mu.Unlock()

	m.workDir = workDir

	for name, cfg := range configs {
		observe.GlobalTrace("range configs")
		server := NewServer(name, cfg, m.bus)
		m.servers[name] = server

		for ext := range cfg.ExtensionToLanguage {
			observe.GlobalTrace("range cfg.ExtensionToLanguage")

			if _, exists := m.extensionMap[ext]; !exists {
				observe.GlobalTrace("if: !exists")
				m.extensionMap[ext] = name
			}
		}
	}
}

// Shutdown stops all running servers.
func (m *Manager) Shutdown() {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	servers := make([]*Server, 0, len(m.servers))
	for _, s := range m.servers {
		observe.GlobalTrace("range m.servers")
		servers = append(servers, s)
	}
	m.mu.RUnlock()

	for _, s := range servers {
		observe.GlobalTrace("range servers")
		_ = s.Stop()
	}
}

// ServerForFile returns the server responsible for the given file, if any.
func (m *Manager) ServerForFile(filePath string) (*Server, string, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	ext := strings.ToLower(filepath.Ext(filePath))
	m.mu.RLock()
	name, ok := m.extensionMap[ext]
	if !ok {
		observe.GlobalTrace("if: !ok")
		m.mu.RUnlock()
		observe.GlobalTrace("return: nil, \"\", false")
		observe.GlobalTrace("return: nil, \"\", false")
		observe.GlobalTrace("return: nil, \"\", false")
		return nil, "", false
	}
	server := m.servers[name]
	m.mu.RUnlock()

	langID := ""
	if server != nil {
		observe.GlobalTrace("if: server != nil")
		langID = server.config.ExtensionToLanguage[ext]
	}
	observe.GlobalTrace("return: server, langID, server != nil")
	observe.GlobalTrace("return: server, langID, server != nil")
	observe.GlobalTrace("return: server, langID, server != nil")

	return server, langID, server != nil
}

// SendRequest routes an LSP request to the correct server for the file.
// The server is lazily started if needed.
func (m *Manager) SendRequest(ctx context.Context, filePath, method string, params any) (json.RawMessage, error) {
	observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "exit")
	server, _, ok := m.ServerForFile(filePath)
	if !ok {
		observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "if: !ok")
		observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "return: nil, fmt.Errorf(\"%w: %s\", ErrNoServerForFile, filepath.Ext(filePath))")
		observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "return: nil, fmt.Errorf(\"%w: %s\", ErrNoServerForFile, filepath.Ext(filePath))")
		observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "return: nil, fmt.Errorf(\"%w: %s\", ErrNoServerForFile, filepath.Ext(filePath))")
		return nil, fmt.Errorf("%w: %s", ErrNoServerForFile, filepath.Ext(filePath))
	}

	if err := m.ensureServerWithDiagnostics(ctx, server); err != nil {
		observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "return: nil, err")
		observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "return: nil, err")
		observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "return: nil, err")
		return nil, err
	}

	start := time.Now()
	m.bus.Emit(observe.LSPRequestSent{
		EventHeader: observe.NewEventHeader("LSPRequestSent", observe.NewTraceID(), observe.NewSpanID(), ""),
		ServerName:  server.Name(),
		Method:      method,
		FilePath:    filePath,
	})

	result, err := server.SendRequest(ctx, method, params)

	m.bus.Emit(observe.LSPRequestCompleted{
		EventHeader: observe.NewEventHeader("LSPRequestCompleted", observe.NewTraceID(), observe.NewSpanID(), ""),
		ServerName:  server.Name(),
		Method:      method,
		DurationMs:  time.Since(start).Milliseconds(),
		HasResult:   result != nil,
		Error:       errString(err),
	})
	observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "return: result, err")
	observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "return: result, err")
	observe.TraceCtx(ctx, "lsp", "Manager.SendRequest", "return: result, err")

	return result, err
}

// OpenFile sends textDocument/didOpen to the appropriate server.
// Idempotent — skips if the file is already open.
func (m *Manager) OpenFile(ctx context.Context, filePath, content string) error {
	observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "exit")
	server, langID, ok := m.ServerForFile(filePath)
	if !ok {
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "if: !ok")
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: nil")
		return nil
	}

	uri := PathToURI(filePath)

	m.mu.Lock()
	if _, already := m.openedFiles[uri]; already {
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "if: already")
		m.mu.Unlock()
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: nil")
		return nil
	}
	m.openedFiles[uri] = server.Name()
	m.mu.Unlock()

	if err := m.ensureServerWithDiagnostics(ctx, server); err != nil {
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: err")
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: err")
		observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: err")
		return err
	}
	observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: server.SendNotification(\"textDocument/didOpen\", map[string]any{\n\t\"textDocumen...")
	observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: server.SendNotification(\"textDocument/didOpen\", map[string]any{\n\t\"textDocumen...")
	observe.TraceCtx(ctx, "lsp", "Manager.OpenFile", "return: server.SendNotification(\"textDocument/didOpen\", map[string]any{\n\t\"textDocumen...")

	return server.SendNotification("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri":        uri,
			"languageId": langID,
			"version":    m.nextVersion(uri),
			"text":       content,
		},
	})
}

// ChangeFile sends textDocument/didChange with full content.
// If the file hasn't been opened yet, falls back to OpenFile.
func (m *Manager) ChangeFile(ctx context.Context, filePath, content string) error {
	observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "exit")
	uri := PathToURI(filePath)

	m.mu.RLock()
	serverName, isOpen := m.openedFiles[uri]
	m.mu.RUnlock()

	if !isOpen {
		observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "if: !isOpen")
		observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "return: m.OpenFile(ctx, filePath, content)")
		observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "return: m.OpenFile(ctx, filePath, content)")
		observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "return: m.OpenFile(ctx, filePath, content)")
		return m.OpenFile(ctx, filePath, content)
	}

	m.mu.RLock()
	server := m.servers[serverName]
	m.mu.RUnlock()

	if server == nil {
		observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "if: server == nil")
		observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "return: nil")
		return nil
	}
	observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "return: server.SendNotification(\"textDocument/didChange\", map[string]any{\n\t\"textDocum...")
	observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "return: server.SendNotification(\"textDocument/didChange\", map[string]any{\n\t\"textDocum...")
	observe.TraceCtx(ctx, "lsp", "Manager.ChangeFile", "return: server.SendNotification(\"textDocument/didChange\", map[string]any{\n\t\"textDocum...")

	return server.SendNotification("textDocument/didChange", map[string]any{
		"textDocument": map[string]any{
			"uri":     uri,
			"version": m.nextVersion(uri),
		},
		"contentChanges": []map[string]any{
			{"text": content},
		},
	})
}

// SaveFile sends textDocument/didSave.
func (m *Manager) SaveFile(ctx context.Context, filePath string) error {
	observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "exit")
	uri := PathToURI(filePath)

	m.mu.RLock()
	serverName, isOpen := m.openedFiles[uri]
	m.mu.RUnlock()

	if !isOpen {
		observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "if: !isOpen")
		observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "return: nil")
		return nil
	}

	m.mu.RLock()
	server := m.servers[serverName]
	m.mu.RUnlock()

	if server == nil {
		observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "if: server == nil")
		observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "return: nil")
		return nil
	}
	observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "return: server.SendNotification(\"textDocument/didSave\", map[string]any{\n\t\"textDocumen...")
	observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "return: server.SendNotification(\"textDocument/didSave\", map[string]any{\n\t\"textDocumen...")
	observe.TraceCtx(ctx, "lsp", "Manager.SaveFile", "return: server.SendNotification(\"textDocument/didSave\", map[string]any{\n\t\"textDocumen...")

	return server.SendNotification("textDocument/didSave", map[string]any{
		"textDocument": map[string]any{
			"uri": uri,
		},
	})
}

// CloseFile sends textDocument/didClose and removes from tracking.
func (m *Manager) CloseFile(ctx context.Context, filePath string) error {
	observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "exit")
	uri := PathToURI(filePath)

	m.mu.Lock()
	serverName, isOpen := m.openedFiles[uri]
	if isOpen {
		observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "if: isOpen")
		delete(m.openedFiles, uri)
	}
	m.mu.Unlock()

	if !isOpen {
		observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "if: !isOpen")
		observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "return: nil")
		return nil
	}

	m.mu.RLock()
	server := m.servers[serverName]
	m.mu.RUnlock()

	if server == nil {
		observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "if: server == nil")
		observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "return: nil")
		return nil
	}
	observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "return: server.SendNotification(\"textDocument/didClose\", map[string]any{\n\t\"textDocume...")
	observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "return: server.SendNotification(\"textDocument/didClose\", map[string]any{\n\t\"textDocume...")
	observe.TraceCtx(ctx, "lsp", "Manager.CloseFile", "return: server.SendNotification(\"textDocument/didClose\", map[string]any{\n\t\"textDocume...")

	return server.SendNotification("textDocument/didClose", map[string]any{
		"textDocument": map[string]any{
			"uri": uri,
		},
	})
}

// IsConnected returns true if at least one server is not in error state.
func (m *Manager) IsConnected() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.servers) == 0 {
		observe.GlobalTrace("if: len(m.servers) == 0")
		observe.GlobalTrace("return: false")
		observe.GlobalTrace("return: false")
		observe.GlobalTrace("return: false")
		return false
	}

	for _, s := range m.servers {
		observe.GlobalTrace("range m.servers")
		state := s.State()
		if state != StateError {
			observe.GlobalTrace("if: state != StateError")
			observe.GlobalTrace("return: true")
			observe.GlobalTrace("return: true")
			observe.GlobalTrace("return: true")
			return true
		}
	}
	observe.GlobalTrace("return: false")
	observe.GlobalTrace("return: false")
	observe.GlobalTrace("return: false")
	return false
}

// ServerCount returns the number of configured servers.
func (m *Manager) ServerCount() int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	m.mu.RLock()
	defer m.mu.RUnlock()
	observe.GlobalTrace("return: len(m.servers)")
	observe.GlobalTrace("return: len(m.servers)")
	observe.GlobalTrace("return: len(m.servers)")
	return len(m.servers)
}

func (m *Manager) nextVersion(uri string) int64 {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	v, _ := m.fileVersion.LoadOrStore(uri, &atomic.Int64{})
	observe.GlobalTrace("return: v.(*atomic.Int64).Add(1)")
	observe.GlobalTrace("return: v.(*atomic.Int64).Add(1)")
	return v.(*atomic.Int64).Add(1)
}

// ensureServerWithDiagnostics starts a server and registers the publishDiagnostics handler.
func (m *Manager) ensureServerWithDiagnostics(ctx context.Context, server *Server) error {
	observe.TraceCtx(ctx, "lsp", "Manager.ensureServerWithDiagnostics", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Manager.ensureServerWithDiagnostics", "exit")
	wasRunning := server.State() == StateRunning
	if err := server.EnsureStarted(ctx, m.workDir); err != nil {
		observe.TraceCtx(ctx, "lsp", "Manager.ensureServerWithDiagnostics", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Manager.ensureServerWithDiagnostics", "return: err")
		observe.TraceCtx(ctx, "lsp", "Manager.ensureServerWithDiagnostics", "return: err")
		return err
	}

	if !wasRunning {
		observe.TraceCtx(ctx, "lsp", "Manager.ensureServerWithDiagnostics", "if: !wasRunning")
		diags := m.Diagnostics
		name := server.Name()
		server.OnNotification("textDocument/publishDiagnostics", func(params json.RawMessage) {
			var p struct {
				URI         string          `json:"uri"`
				Diagnostics json.RawMessage `json:"diagnostics"`
			}
			if err := json.Unmarshal(params, &p); err != nil {
				return
			}
			diags.Register(name, p.URI, p.Diagnostics)
		})
	}
	observe.TraceCtx(ctx, "lsp", "Manager.ensureServerWithDiagnostics", "return: nil")
	observe.TraceCtx(ctx, "lsp", "Manager.ensureServerWithDiagnostics", "return: nil")
	return nil
}

func errString(err error) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if err == nil {
		observe.GlobalTrace("if: err == nil")
		observe.GlobalTrace("return: \"\"")
		observe.GlobalTrace("return: \"\"")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: err.Error()")
	observe.GlobalTrace("return: err.Error()")
	observe.GlobalTrace("return: err.Error()")
	return err.Error()
}
