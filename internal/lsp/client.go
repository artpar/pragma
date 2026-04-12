package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/artpar/gogent/internal/observe"
)

// Client manages a single LSP server process and JSON-RPC communication.
type Client struct {
	name     string
	command  string
	args     []string
	env      map[string]string
	workDir  string
	bus      *observe.EventBus
	initOpts json.RawMessage
	settings json.RawMessage

	mu          sync.Mutex
	cmd         *exec.Cmd
	codec       *Codec
	initialized bool
	stopping    bool
	exitCh      chan struct{} // closed when process exits
}

// NewClient creates an LSP client for the given server.
func NewClient(name, command string, args []string, env map[string]string, workDir string, bus *observe.EventBus, initOpts, settings json.RawMessage) *Client {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: &Client{\n\tname:\t\tname,\n\tcommand:\tcommand,\n\targs:\t\targs,\n\tenv:\t\tenv,\n\tworkDir:...")
	observe.GlobalTrace("return: &Client{\n\tname:\t\tname,\n\tcommand:\tcommand,\n\targs:\t\targs,\n\tenv:\t\tenv,\n\tworkDir:...")
	return &Client{
		name:     name,
		command:  command,
		args:     args,
		env:      env,
		workDir:  workDir,
		bus:      bus,
		initOpts: initOpts,
		settings: settings,
		exitCh:   make(chan struct{}),
	}
}

// Start spawns the LSP server process and starts the read loop.
func (c *Client) Start(ctx context.Context) error {
	observe.TraceCtx(ctx, "lsp", "Client.Start", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Client.Start", "exit")
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.cmd != nil {
		observe.TraceCtx(ctx, "lsp", "Client.Start", "if: c.cmd != nil")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"client already started\")")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"client already started\")")
		return fmt.Errorf("client already started")
	}

	cmd := exec.CommandContext(ctx, c.command, c.args...)
	cmd.Dir = c.workDir

	cmd.Env = os.Environ()
	for k, v := range c.env {
		observe.TraceCtx(ctx, "lsp", "Client.Start", "range c.env")
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	setProcAttr(cmd)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		observe.TraceCtx(ctx, "lsp", "Client.Start", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"create stdin pipe: %w\", err)")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"create stdin pipe: %w\", err)")
		return fmt.Errorf("create stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		observe.TraceCtx(ctx, "lsp", "Client.Start", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"create stdout pipe: %w\", err)")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"create stdout pipe: %w\", err)")
		return fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		observe.TraceCtx(ctx, "lsp", "Client.Start", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"create stderr pipe: %w\", err)")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"create stderr pipe: %w\", err)")
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		observe.TraceCtx(ctx, "lsp", "Client.Start", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"start %s: %w\", c.command, err)")
		observe.TraceCtx(ctx, "lsp", "Client.Start", "return: fmt.Errorf(\"start %s: %w\", c.command, err)")
		return fmt.Errorf("start %s: %w", c.command, err)
	}

	c.cmd = cmd
	c.codec = NewCodec(stdout, stdin)

	c.codec.OnRequest("workspace/configuration", func(params json.RawMessage) (any, error) {
		// Return null for each requested config item (matches TS behavior)
		var req struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return []any{nil}, nil
		}
		results := make([]any, len(req.Items))
		return results, nil
	})

	c.codec.OnRequest("client/registerCapability", func(params json.RawMessage) (any, error) {

		return nil, nil
	})

	c.codec.OnRequest("client/unregisterCapability", func(params json.RawMessage) (any, error) {
		return nil, nil
	})

	c.codec.OnRequest("window/workDoneProgress/create", func(params json.RawMessage) (any, error) {
		return nil, nil
	})

	go c.codec.ReadLoop()

	go func() {
		buf := make([]byte, 4096)
		for {
			observe.TraceCtx(ctx, "lsp", "Client.Start", "for: true")
			n, err := stderr.Read(buf)
			if n > 0 {
				observe.TraceCtx(ctx, "lsp", "Client.Start", "if: n > 0")
				c.bus.Trace("lsp", c.name+".stderr", string(buf[:n]))
			}
			if err != nil {
				observe.TraceCtx(ctx, "lsp", "Client.Start", "if: err != nil")
				return
			}
		}
	}()

	go func() {
		_ = cmd.Wait()
		c.codec.Close()
		close(c.exitCh)
	}()
	observe.TraceCtx(ctx, "lsp", "Client.Start", "return: nil")
	observe.TraceCtx(ctx, "lsp", "Client.Start", "return: nil")

	return nil
}

// Initialize sends the LSP initialize request and initialized notification.
func (c *Client) Initialize(ctx context.Context) error {
	observe.TraceCtx(ctx, "lsp", "Client.Initialize", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Client.Initialize", "exit")
	c.mu.Lock()
	if c.initialized {
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "if: c.initialized")
		c.mu.Unlock()
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: nil")
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: nil")
		return nil
	}
	codec := c.codec
	workDir := c.workDir
	initOpts := c.initOpts
	settings := c.settings
	c.mu.Unlock()

	if codec == nil {
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "if: codec == nil")
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: ErrNotInitialized")
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: ErrNotInitialized")
		return ErrNotInitialized
	}

	rootURI := PathToURI(workDir)

	params := map[string]any{
		"processId": os.Getpid(),
		"rootPath":  workDir,
		"rootUri":   rootURI,
		"workspaceFolders": []map[string]string{{
			"uri":  rootURI,
			"name": filepath.Base(workDir),
		}},
		"capabilities": map[string]any{
			"workspace": map[string]any{
				"configuration":    false,
				"workspaceFolders": false,
			},
			"textDocument": map[string]any{
				"synchronization": map[string]any{
					"dynamicRegistration": false,
					"willSave":            false,
					"willSaveWaitUntil":   false,
					"didSave":             true,
				},
				"publishDiagnostics": map[string]any{
					"relatedInformation":     true,
					"codeDescriptionSupport": true,
					"tagSupport": map[string]any{
						"valueSet": []int{1, 2},
					},
					"versionSupport": false,
					"dataSupport":    false,
				},
				"hover": map[string]any{
					"dynamicRegistration": false,
					"contentFormat":       []string{"markdown", "plaintext"},
				},
				"definition": map[string]any{
					"dynamicRegistration": false,
					"linkSupport":         true,
				},
				"references": map[string]any{
					"dynamicRegistration": false,
				},
				"documentSymbol": map[string]any{
					"dynamicRegistration":               false,
					"hierarchicalDocumentSymbolSupport": true,
				},
				"callHierarchy": map[string]any{
					"dynamicRegistration": false,
				},
				"implementation": map[string]any{
					"dynamicRegistration": false,
				},
			},
			"general": map[string]any{
				"positionEncodings": []string{"utf-16"},
			},
		},
	}

	if len(initOpts) > 0 {
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "if: len(initOpts) > 0")
		params["initializationOptions"] = initOpts
	}

	_, err := codec.Call(ctx, "initialize", params)
	if err != nil {
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: fmt.Errorf(\"initialize: %w\", err)")
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: fmt.Errorf(\"initialize: %w\", err)")
		return fmt.Errorf("initialize: %w", err)
	}

	if err := codec.Notify("initialized", struct{}{}); err != nil {
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "if: err != nil")
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: fmt.Errorf(\"initialized notification: %w\", err)")
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: fmt.Errorf(\"initialized notification: %w\", err)")
		return fmt.Errorf("initialized notification: %w", err)
	}

	if len(settings) > 0 {
		observe.TraceCtx(ctx, "lsp", "Client.Initialize", "if: len(settings) > 0")
		_ = codec.Notify("workspace/didChangeConfiguration", map[string]json.RawMessage{
			"settings": settings,
		})
	}

	c.mu.Lock()
	c.initialized = true
	c.mu.Unlock()
	observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: nil")
	observe.TraceCtx(ctx, "lsp", "Client.Initialize", "return: nil")

	return nil
}

// SendRequest sends an LSP request and returns the result.
func (c *Client) SendRequest(ctx context.Context, method string, params any) (json.RawMessage, error) {
	observe.TraceCtx(ctx, "lsp", "Client.SendRequest", "enter")
	defer observe.TraceCtx(ctx, "lsp", "Client.SendRequest", "exit")
	c.mu.Lock()
	codec := c.codec
	initialized := c.initialized
	c.mu.Unlock()

	if codec == nil || !initialized {
		observe.TraceCtx(ctx, "lsp", "Client.SendRequest", "if: codec == nil || !initialized")
		observe.TraceCtx(ctx, "lsp", "Client.SendRequest", "return: nil, ErrNotInitialized")
		observe.TraceCtx(ctx, "lsp", "Client.SendRequest", "return: nil, ErrNotInitialized")
		return nil, ErrNotInitialized
	}
	observe.TraceCtx(ctx, "lsp", "Client.SendRequest", "return: codec.Call(ctx, method, params)")
	observe.TraceCtx(ctx, "lsp", "Client.SendRequest", "return: codec.Call(ctx, method, params)")
	return codec.Call(ctx, method, params)
}

// SendNotification sends an LSP notification.
func (c *Client) SendNotification(method string, params any) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.mu.Lock()
	codec := c.codec
	c.mu.Unlock()

	if codec == nil {
		observe.GlobalTrace("if: codec == nil")
		observe.GlobalTrace("return: ErrNotInitialized")
		observe.GlobalTrace("return: ErrNotInitialized")
		return ErrNotInitialized
	}
	observe.GlobalTrace("return: codec.Notify(method, params)")
	observe.GlobalTrace("return: codec.Notify(method, params)")
	return codec.Notify(method, params)
}

// OnNotification registers a notification handler on the underlying codec.
func (c *Client) OnNotification(method string, handler func(json.RawMessage)) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.mu.Lock()
	codec := c.codec
	c.mu.Unlock()
	if codec != nil {
		observe.GlobalTrace("if: codec != nil")
		codec.OnNotification(method, handler)
	}
}

// Stop sends the LSP shutdown sequence and kills the process.
func (c *Client) Stop() error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.mu.Lock()
	if c.stopping {
		observe.GlobalTrace("if: c.stopping")
		c.mu.Unlock()
		observe.GlobalTrace("return: nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	c.stopping = true
	codec := c.codec
	cmd := c.cmd
	exitCh := c.exitCh
	c.mu.Unlock()

	if cmd == nil {
		observe.GlobalTrace("if: cmd == nil")
		observe.GlobalTrace("return: nil")
		observe.GlobalTrace("return: nil")
		return nil
	}

	if codec != nil {
		observe.GlobalTrace("if: codec != nil")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _ = codec.Call(shutdownCtx, "shutdown", nil)
		cancel()
		_ = codec.Notify("exit", nil)
	}

	select {
	case <-exitCh:
		observe.GlobalTrace("select: <-exitCh")
		return nil
	case <-time.After(5 * time.Second):
		observe.GlobalTrace("select: <-time.After(5 * time.Second)")
	}

	if cmd.Process != nil {
		observe.GlobalTrace("if: cmd.Process != nil")
		_ = cmd.Process.Kill()
	}
	observe.GlobalTrace("return: nil")
	observe.GlobalTrace("return: nil")

	return nil
}

// IsRunning returns true if the process is alive and initialized.
func (c *Client) IsRunning() bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd == nil || c.stopping {
		observe.GlobalTrace("if: c.cmd == nil || c.stopping")
		observe.GlobalTrace("return: false")
		observe.GlobalTrace("return: false")
		return false
	}
	select {
	case <-c.exitCh:
		observe.GlobalTrace("select: <-c.exitCh")
		return false
	default:
		observe.GlobalTrace("select: default")
		return c.initialized
	}
}

// ExitCh returns a channel that is closed when the process exits.
func (c *Client) ExitCh() <-chan struct{} {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: c.exitCh")
	observe.GlobalTrace("return: c.exitCh")
	return c.exitCh
}

// PathToURI converts an absolute file path to a file:// URI (RFC 8089).
func PathToURI(path string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	u := &url.URL{
		Scheme: "file",
		Path:   filepath.ToSlash(path),
	}
	observe.GlobalTrace("return: u.String()")
	observe.GlobalTrace("return: u.String()")
	return u.String()
}

// URIToPath converts a file:// URI back to a local file path.
func URIToPath(uri string) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	u, err := url.Parse(uri)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: uri")
		observe.GlobalTrace("return: uri")
		return uri
	}
	if u.Scheme != "file" {
		observe.GlobalTrace("if: u.Scheme != \"file\"")
		observe.GlobalTrace("return: uri")
		observe.GlobalTrace("return: uri")
		return uri
	}
	observe.GlobalTrace("return: filepath.FromSlash(u.Path)")
	observe.GlobalTrace("return: filepath.FromSlash(u.Path)")
	return filepath.FromSlash(u.Path)
}
