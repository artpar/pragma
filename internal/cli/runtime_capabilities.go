package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/observe"
	toolapplypatch "github.com/artpar/pragma/internal/tools/applypatch"
	"github.com/artpar/pragma/internal/toolset"
)

func ensureCapabilitiesForActiveWorkDir(ctx context.Context, d *Deps) {
	if d == nil {
		return
	}
	if err := refreshCapabilitiesForWorkDir(ctx, d, activeCapabilityWorkDir(d)); err != nil && d.Bus != nil {
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
	if d == nil {
		return nil
	}
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		workDir = d.Cwd
	}

	d.capabilityMu.Lock()
	defer d.capabilityMu.Unlock()
	if d.CapabilityWorkDir == workDir {
		return nil
	}

	activeToolset, err := resolveRuntimeToolset(workDir, d.Cfg.Toolset)
	if err != nil {
		return err
	}

	var mcpServers map[string]mcp.ServerConfig
	if d.McpManager != nil {
		var mcpErr error
		mcpServers, mcpErr = mcp.LoadConfig(workDir, d.Bus)
		if mcpErr != nil {
			return fmt.Errorf("load mcp config for %s: %w", workDir, mcpErr)
		}
	}
	if activeToolset != nil {
		mcpServers = activeToolset.FilterMCPServers(mcpServers)
	}

	d.Toolset = activeToolset
	d.CapabilityWorkDir = workDir
	if d.Registry != nil {
		d.Registry.SetExposureFilter(runtimeToolExposureFilter(d.ToolPolicy, activeToolset))
	}
	if d.McpManager == nil {
		return nil
	}

	d.McpManager.SetRegistryToolFilter(d.ToolPolicy.Allows)
	if activeToolset != nil {
		d.McpManager.SetToolFilter(activeToolset.AllowMCPTool)
	} else {
		d.McpManager.SetToolFilter(nil)
	}
	d.McpManager.ReplaceServers(mcpServers)
	if len(mcpServers) == 0 {
		return nil
	}

	connectCtx := d.capabilityContext
	if connectCtx == nil {
		connectCtx = ctx
	}
	if connectCtx == nil {
		connectCtx = context.Background()
	}
	if d.capabilityWG != nil {
		d.capabilityWG.Add(1)
		go func() {
			defer d.capabilityWG.Done()
			runMCPConnect(connectCtx, d.McpManager, mcpServers)
		}()
		return nil
	}
	go runMCPConnect(connectCtx, d.McpManager, mcpServers)
	return nil
}

func runMCPConnect(ctx context.Context, mgr *mcp.Manager, servers map[string]mcp.ServerConfig) {
	if mgr == nil {
		return
	}
	connectCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	mgr.ConnectAllAndRegister(connectCtx, servers)
}

func resolveRuntimeToolset(workDir, name string) (*toolset.Compiled, error) {
	if strings.TrimSpace(name) == "" {
		return nil, nil
	}
	return toolset.Resolve(workDir, name)
}

func activeCapabilityWorkDir(d *Deps) string {
	if d == nil {
		return ""
	}
	if d.Store != nil {
		if cwd := d.Store.Snapshot().CWD; cwd != "" {
			return cwd
		}
	}
	return d.Cwd
}

func runtimeToolExposureFilter(policy ToolExposurePolicy, activeToolset *toolset.Compiled) func(string) bool {
	return func(name string) bool {
		if !policy.Allows(name) {
			return false
		}
		if strings.HasPrefix(name, "mcp__") {
			return true
		}
		return builtinAllowedByToolset(activeToolset, name)
	}
}

func builtinAllowedByToolset(activeToolset *toolset.Compiled, name string) bool {
	if isRuntimeBuiltinTool(name) {
		return true
	}
	if activeToolset == nil {
		return true
	}
	if name == toolapplypatch.ToolName && activeToolset.AllowBuiltinTool(toolapplypatch.LegacyToolName) {
		return true
	}
	if name == toolapplypatch.LegacyToolName && activeToolset.AllowBuiltinTool(toolapplypatch.ToolName) {
		return true
	}
	return activeToolset.AllowBuiltinTool(name)
}
