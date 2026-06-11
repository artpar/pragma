package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/observe"
)

func ensureCapabilitiesForActiveWorkDir(ctx context.Context, d *Deps) {
	observe.TraceCtx(ctx, "cli", "ensureCapabilitiesForActiveWorkDir", "enter")
	defer observe.TraceCtx(ctx, "cli", "ensureCapabilitiesForActiveWorkDir", "exit")
	if d == nil {
		observe.TraceCtx(ctx, "cli", "ensureCapabilitiesForActiveWorkDir", "if: d == nil")
		return
	}
	if err := refreshCapabilitiesForWorkDir(ctx, d, activeCapabilityWorkDir(d)); err != nil && d.Bus != nil {
		observe.TraceCtx(ctx, "cli", "ensureCapabilitiesForActiveWorkDir", "if: err != nil && d.Bus != nil")
		d.Bus.Emit(observe.ErrorOccurred{
			EventHeader:  observe.NewEventHeader("ErrorOccurred", observe.NewTraceID(), observe.NewSpanID(), ""),
			Severity:     "warn",
			Component:    "capabilities",
			ErrorType:    "refresh_failed",
			ErrorMessage: err.Error(),
		})
	}
}

func refreshCapabilitiesForWorkDir(ctx context.Context, d *Deps, workDir string) error {
	observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "enter")
	defer observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "exit")
	if d == nil {
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: d == nil")
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "return: nil")
		return nil
	}
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: workDir == \"\"")
		workDir = d.Cwd
	}

	d.capabilityMu.Lock()
	defer d.capabilityMu.Unlock()
	if d.CapabilityWorkDir == workDir {
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: d.CapabilityWorkDir == workDir")
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "return: nil")
		return nil
	}

	var mcpServers map[string]mcp.ServerConfig
	if d.McpManager != nil {
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: d.McpManager != nil")
		var mcpErr error
		mcpServers, mcpErr = mcp.LoadConfig(workDir, d.Bus)
		if mcpErr != nil {
			observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: mcpErr != nil")
			observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "return: fmt.Errorf(\"load mcp config for %s: %w\", workDir, mcpErr)")
			return fmt.Errorf("load mcp config for %s: %w", workDir, mcpErr)
		}
	}
	d.CapabilityWorkDir = workDir
	if d.McpManager == nil {
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: d.McpManager == nil")
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "return: nil")
		return nil
	}

	d.McpManager.ReplaceServers(mcpServers)
	if len(mcpServers) == 0 {
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: len(mcpServers) == 0")
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "return: nil")
		return nil
	}

	connectCtx := d.capabilityContext
	if connectCtx == nil {
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: connectCtx == nil")
		connectCtx = ctx
	}
	if connectCtx == nil {
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: connectCtx == nil")
		connectCtx = context.Background()
	}
	if d.capabilityWG != nil {
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "if: d.capabilityWG != nil")
		d.capabilityWG.Add(1)
		go func() {
			defer d.capabilityWG.Done()
			runMCPConnect(connectCtx, d.McpManager, mcpServers)
		}()
		observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "return: nil")
		return nil
	}
	go runMCPConnect(connectCtx, d.McpManager, mcpServers)
	observe.TraceCtx(ctx, "cli", "refreshCapabilitiesForWorkDir", "return: nil")
	return nil
}

func runMCPConnect(ctx context.Context, mgr *mcp.Manager, servers map[string]mcp.ServerConfig) {
	observe.TraceCtx(ctx, "cli", "runMCPConnect", "enter")
	defer observe.TraceCtx(ctx, "cli", "runMCPConnect", "exit")
	if mgr == nil {
		observe.TraceCtx(ctx, "cli", "runMCPConnect", "if: mgr == nil")
		return
	}
	connectCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	mgr.ConnectAll(connectCtx, servers)
}

func activeCapabilityWorkDir(d *Deps) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d == nil {
		observe.GlobalTrace("if: d == nil")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	if d.Store != nil {
		observe.GlobalTrace("if: d.Store != nil")
		if cwd := d.Store.Snapshot().CWD; cwd != "" {
			observe.GlobalTrace("if: cwd != \"\"")
			observe.GlobalTrace("return: cwd")
			return cwd
		}
	}
	observe.GlobalTrace("return: d.Cwd")
	return d.Cwd
}
