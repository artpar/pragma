package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/artpar/gogent/internal/observe"
)

// ServerState represents the lifecycle state of an LSP server.
type ServerState string

const (
	StateStopped  ServerState = "stopped"
	StateStarting ServerState = "starting"
	StateRunning  ServerState = "running"
	StateStopping ServerState = "stopping"
	StateError    ServerState = "error"
)

const (
	retryMaxAttempts    = 3
	retryBaseDelay      = 500 * time.Millisecond
	contentModifiedCode = -32801
)

// Server wraps a Client with state tracking, crash recovery, and transient error retry.
type Server struct {
	name   string
	config ServerConfig
	bus    *observe.EventBus

	mu           sync.Mutex
	state        ServerState
	client       *Client
	restartCount int
	lastError    error
}

// NewServer creates an LSP server instance.
func NewServer(name string, cfg ServerConfig, bus *observe.EventBus) *Server {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Server{\n\tname:\tname,\n\tconfig:\tcfg,\n\tbus:\tbus,\n\tstate:\tStateStopped,\n}")
	observe.GlobalTrace("return: &Server{\n\tname:\tname,\n\tconfig:\tcfg,\n\tbus:\tbus,\n\tstate:\tStateStopped,\n}")
	return &Server{
		name:   name,
		config: cfg,
		bus:    bus,
		state:  StateStopped,
	}
}

// EnsureStarted starts the server if it's not running. If it crashed and
// restarts are available, it restarts.
func (s *Server) EnsureStarted(ctx context.Context, workDir string) error {
	observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "exit")
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.state {
	case StateRunning:
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "case: StateRunning")

		if s.client != nil && s.client.IsRunning() {
			observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: nil")
			observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: nil")
			return nil
		}

		s.state = StateError

	case StateStarting:
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "case: StateStarting")
		return nil

	case StateStopping:
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "case: StateStopping")
		return ErrServerStopping

	case StateStopped, StateError:
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "case: StateStopped, StateError")

	}

	if s.state == StateError {
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "if: s.state == StateError")
		if s.restartCount >= s.config.maxRestartsOrDefault() {
			observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "if: s.restartCount >= s.config.maxRestartsOrDefault()")
			observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: fmt.Errorf(\"%w: %s restarted %d times\", ErrMaxRestarts, s.name, s.restartCount)")
			observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: fmt.Errorf(\"%w: %s restarted %d times\", ErrMaxRestarts, s.name, s.restartCount)")
			return fmt.Errorf("%w: %s restarted %d times", ErrMaxRestarts, s.name, s.restartCount)
		}
		s.restartCount++
	}

	s.state = StateStarting

	s.bus.Emit(observe.LSPServerStarted{
		EventHeader: observe.NewEventHeader("LSPServerStarted", observe.NewTraceID(), observe.NewSpanID(), ""),
		ServerName:  s.name,
		Command:     s.config.Command,
	})

	client := NewClient(
		s.name, s.config.Command, s.config.Args,
		s.config.Env, workDir, s.bus,
		s.config.InitializationOptions, s.config.Settings,
	)

	startCtx, cancel := context.WithTimeout(ctx, time.Duration(s.config.startupTimeoutMsOrDefault())*time.Millisecond)
	defer cancel()

	if err := client.Start(startCtx); err != nil {
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "if: err != nil")
		s.state = StateError
		s.lastError = err
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: fmt.Errorf(\"start %s: %w\", s.name, err)")
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: fmt.Errorf(\"start %s: %w\", s.name, err)")
		return fmt.Errorf("start %s: %w", s.name, err)
	}

	if err := client.Initialize(startCtx); err != nil {
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "if: err != nil")
		s.state = StateError
		s.lastError = err
		_ = client.Stop()
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: fmt.Errorf(\"initialize %s: %w\", s.name, err)")
		observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: fmt.Errorf(\"initialize %s: %w\", s.name, err)")
		return fmt.Errorf("initialize %s: %w", s.name, err)
	}

	s.client = client
	s.state = StateRunning

	go func() {
		<-client.ExitCh()
		s.mu.Lock()
		if s.state == StateRunning {
			observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "if: s.state == StateRunning")
			s.state = StateError
			s.lastError = ErrServerCrashed
			s.bus.Emit(observe.LSPServerStopped{
				EventHeader: observe.NewEventHeader("LSPServerStopped", observe.NewTraceID(), observe.NewSpanID(), ""),
				ServerName:  s.name,
				Reason:      "crashed",
			})
		}
		s.mu.Unlock()
	}()
	observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: nil")
	observe.TraceCtx(ctx, "lsp", "Server.EnsureStarted", "return: nil")

	return nil
}

// SendRequest sends an LSP request with transient error retry.
// Error code -32801 (content modified) is retried up to 3 times
// with exponential backoff (500ms, 1s, 2s).
func (s *Server) SendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "exit")
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()

	if client == nil {
		observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "if: client == nil")
		observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "return: nil, ErrNotInitialized")
		observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "return: nil, ErrNotInitialized")
		return nil, ErrNotInitialized
	}

	var lastErr error
	for attempt := range retryMaxAttempts {
		observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "range retryMaxAttempts")
		result, err := client.SendRequest(ctx, method, params)
		if err == nil {
			observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "if: err == nil")
			observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "return: result, nil")
			observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "return: result, nil")
			return result, nil
		}

		var rpcErr *RPCError
		if errors.As(err, &rpcErr) && rpcErr.Code == contentModifiedCode {
			observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "if: errors.As(err, &rpcErr) && rpcErr.Code == contentModifiedCode")
			lastErr = err
			delay := retryBaseDelay * time.Duration(1<<attempt)
			select {
			case <-time.After(delay):
				observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "select: <-time.After(delay)")
				continue
			case <-ctx.Done():
				observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "select: <-ctx.Done()")
				return nil, ctx.Err()
			}
		}
		observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "return: nil, err")
		observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "return: nil, err")

		return nil, err
	}
	observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "return: nil, fmt.Errorf(\"content modified after %d retries: %w\", retryMaxAttempts, la...")
	observe.TraceCtx(ctx, "lsp", "Server.SendRequest", "return: nil, fmt.Errorf(\"content modified after %d retries: %w\", retryMaxAttempts, la...")

	return nil, fmt.Errorf("content modified after %d retries: %w", retryMaxAttempts, lastErr)
}

// SendNotification sends an LSP notification.
func (s *Server) SendNotification(method string, params any) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client == nil {
		observe.GlobalTrace("if: client == nil")
		observe.GlobalTrace("return: ErrNotInitialized")
		observe.GlobalTrace("return: ErrNotInitialized")
		return ErrNotInitialized
	}
	observe.GlobalTrace("return: client.SendNotification(method, params)")
	observe.GlobalTrace("return: client.SendNotification(method, params)")
	return client.SendNotification(method, params)
}

// OnNotification registers a notification handler.
func (s *Server) OnNotification(method string, handler func(json.RawMessage)) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s.mu.Lock()
	client := s.client
	s.mu.Unlock()
	if client != nil {
		observe.GlobalTrace("if: client != nil")
		client.OnNotification(method, handler)
	}
}

// Stop shuts down the server.
func (s *Server) Stop() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s.mu.Lock()
	if s.state == StateStopped || s.state == StateStopping {
		observe.GlobalTrace("if: s.state == StateStopped || s.state == StateStopping")
		s.mu.Unlock()
		observe.GlobalTrace("return: nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	s.state = StateStopping
	client := s.client
	s.mu.Unlock()

	var err error
	if client != nil {
		observe.GlobalTrace("if: client != nil")
		err = client.Stop()
	}

	s.mu.Lock()
	s.state = StateStopped
	s.client = nil
	s.mu.Unlock()

	s.bus.Emit(observe.LSPServerStopped{
		EventHeader: observe.NewEventHeader("LSPServerStopped", observe.NewTraceID(), observe.NewSpanID(), ""),
		ServerName:  s.name,
		Reason:      "shutdown",
	})
	observe.GlobalTrace("return: err")
	observe.GlobalTrace("return: err")

	return err
}

// State returns the current server state.
func (s *Server) State() ServerState {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	s.mu.Lock()
	defer s.mu.Unlock()
	observe.GlobalTrace("return: s.state")
	observe.GlobalTrace("return: s.state")
	return s.state
}

// Name returns the server name.
func (s *Server) Name() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: s.name")
	observe.GlobalTrace("return: s.name")
	return s.name
}
