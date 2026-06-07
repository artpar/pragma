package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/interactive"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/slash"
	"github.com/artpar/pragma/internal/task"
)

// Config contains the already-wired interactive runtime dependencies.
type Config struct {
	Bridge         *Bridge
	ParentCtx      context.Context
	RunInput       func(context.Context, string) <-chan interactive.Event
	Resume         func(sessionID string) error
	CloseSession   func() error
	Store          *app.StateStore
	CostTracker    *model.CostTracker
	ModelName      string
	Provider       string
	SlashCmds      *slash.Registry
	SlashDeps      slash.Deps
	Metrics        *observe.Metrics
	Workspace      string
	Version        string
	TaskReg        *task.Registry
	SessionStore   *session.Store
	EventRecorder  func(session.WebEventData) error
	SessionStart   time.Time
	McpServerNames []string
	WebAddr        string
}

// Run starts the web API server and blocks until the server shuts down.
func Run(ctx context.Context, cfg Config) error {
	if cfg.Bridge == nil {
		return errors.New("web bridge is required")
	}
	srv := newServer(cfg)
	cfg.Bridge.attach(srv.hub)

	mux := http.NewServeMux()
	mux.HandleFunc("/api/state", srv.handleState)
	mux.HandleFunc("/api/workbench", srv.handleWorkbench)
	mux.HandleFunc("/api/events/recent", srv.handleRecentEvents)
	mux.HandleFunc("/api/events", srv.handleEvents)
	mux.HandleFunc("/api/prompt", srv.handlePrompt)
	mux.HandleFunc("/api/cancel", srv.handleCancel)
	mux.HandleFunc("/api/permission/", srv.handlePermission)
	mux.HandleFunc("/api/ask/", srv.handleAsk)
	mux.HandleFunc("/api/sessions/", srv.handleSession)
	mux.HandleFunc("/api/sessions", srv.handleSessions)
	mux.HandleFunc("/api/resume", srv.handleResume)
	mux.HandleFunc("/api/artifact", srv.handleArtifact)
	mux.HandleFunc("/api/completions", srv.handleCompletions)
	mux.HandleFunc("/", srv.handleStatic)

	ln, err := net.Listen("tcp", cfg.listenAddr())
	if err != nil {
		return err
	}
	httpSrv := &http.Server{Handler: mux}
	url := "http://" + ln.Addr().String()
	fmt.Printf("Pragma web API: %s\n", url)

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpSrv.Serve(ln)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if cfg.CloseSession != nil {
			if err := cfg.CloseSession(); err != nil {
				_ = httpSrv.Shutdown(shutdownCtx)
				return err
			}
		}
		_ = httpSrv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (cfg Config) listenAddr() string {
	if cfg.WebAddr != "" {
		return cfg.WebAddr
	}
	return "127.0.0.1:0"
}

type server struct {
	cfg     Config
	hub     *hub
	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
}

var errRuntimeBusy = errors.New("interactive run already in progress")

func newServer(cfg Config) *server {
	srv := &server{
		cfg: cfg,
		hub: newHub(cfg.EventRecorder),
	}
	srv.seedSessionEvents()
	return srv
}
