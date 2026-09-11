package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider"
	"github.com/artpar/pragma/internal/provider/anthropic"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/skill"
	"github.com/artpar/pragma/internal/tools/websearch"
)

// RegisterTools constructs the query engine. The active loop mode decides
// whether tools are expressed through the Pragma shell loop or provider-native
// tool calls.
func RegisterTools(d *Deps, _ permission.Prompter) (*query.Engine, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.EngineCfg.MCPServerStatuses = func() []query.MCPServerStatus {
		ensureCapabilitiesForActiveWorkDir(context.Background(), d)
		return mcpStatusesForQuery(d.McpManager)
	}
	d.EngineCfg.MCPToolDefs = func(ctx context.Context) []model.ToolDef {
		observe.TraceCtx(ctx, "cli", "RegisterTools.MCPToolDefs", "enter")
		defer observe.TraceCtx(ctx, "cli", "RegisterTools.MCPToolDefs", "exit")
		ensureCapabilitiesForActiveWorkDir(ctx, d)
		if d.McpManager == nil {
			observe.TraceCtx(ctx, "cli", "RegisterTools.MCPToolDefs", "if: d.McpManager == nil")
			observe.TraceCtx(ctx, "cli", "RegisterTools.MCPToolDefs", "return: nil")
			return nil
		}
		observe.TraceCtx(ctx, "cli", "RegisterTools.MCPToolDefs", "return: d.McpManager.ToolDefs(ctx)")
		return d.McpManager.ToolDefs(ctx)
	}
	d.EngineCfg.MCPCallTool = func(ctx context.Context, toolName string, input json.RawMessage) (string, error) {
		observe.TraceCtx(ctx, "cli", "RegisterTools.MCPCallTool", "enter")
		defer observe.TraceCtx(ctx, "cli", "RegisterTools.MCPCallTool", "exit")
		if d.McpManager == nil {
			observe.TraceCtx(ctx, "cli", "RegisterTools.MCPCallTool", "if: d.McpManager == nil")
			observe.TraceCtx(ctx, "cli", "RegisterTools.MCPCallTool", "return: \"\", fmt.Errorf(...)")
			return "", fmt.Errorf("mcp tool %s: no MCP manager configured", toolName)
		}
		observe.TraceCtx(ctx, "cli", "RegisterTools.MCPCallTool", "return: d.McpManager.CallMCPTool(ctx, toolName, input)")
		return d.McpManager.CallMCPTool(ctx, toolName, input)
	}
	if key := webSearchKey(d); key != "" {
		observe.GlobalTrace("if: key := webSearchKey(d); key != \"\"")
		baseURL := resolveWebSearchBaseURL()
		d.EngineCfg.WebSearch = func(ctx context.Context, input json.RawMessage) (string, error) {
			observe.TraceCtx(ctx, "cli", "RegisterTools.WebSearch", "enter")
			defer observe.TraceCtx(ctx, "cli", "RegisterTools.WebSearch", "exit")
			observe.TraceCtx(ctx, "cli", "RegisterTools.WebSearch", "return: websearch.Execute(ctx, key, baseURL, input)")
			return websearch.Execute(ctx, key, baseURL, input)
		}
	}
	d.EngineCfg.RefreshCapabilities = func(ctx context.Context) {
		ensureCapabilitiesForActiveWorkDir(ctx, d)
	}
	engine := query.NewEngine(d.Prov, d.Store, d.CostTracker, d.Bus, d.EngineCfg)
	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		engine.SetHookManager(d.HookMgr)
	}
	d.Engine = engine
	observe.GlobalTrace("return: engine, nil")
	return engine, nil
}

// webSearchKey resolves the Brave Search API key: the
// BRAVE_SEARCH_API_KEY environment variable (the branch's source) first,
// then the credentials store's "brave" provider.
func webSearchKey(d *Deps) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if key := strings.TrimSpace(os.Getenv("BRAVE_SEARCH_API_KEY")); key != "" {
		observe.GlobalTrace("if: key := strings.TrimSpace(os.Getenv(\"BRAVE_SEARCH_API_KEY\")); key != \"\"")
		observe.GlobalTrace("return: key")
		return key
	}
	observe.GlobalTrace("return: d.Creds.CredentialFor(\"brave\").APIKey")
	return d.Creds.CredentialFor("brave").APIKey
}

// resolveWebSearchBaseURL allows overriding the Brave endpoint for hermetic
// tests, mirroring OPENAI_BASE_URL.
func resolveWebSearchBaseURL() string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.TrimSpace(os.Getenv(\"BRAVE_SEARCH_BASE_URL\"))")
	return strings.TrimSpace(os.Getenv("BRAVE_SEARCH_BASE_URL"))
}

func RebindProviderBackedTools(_ *Deps) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
}

func mcpStatusesForQuery(mgr interface {
	ServerStatus() map[string]string
}) []query.MCPServerStatus {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if mgr == nil {
		observe.GlobalTrace("if: mgr == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	statuses := mgr.ServerStatus()
	out := make([]query.MCPServerStatus, 0, len(statuses))
	for name, status := range statuses {
		observe.GlobalTrace("range statuses")
		if strings.TrimSpace(name) == "" {
			observe.GlobalTrace("if: strings.TrimSpace(name) == \"\"")
			continue
		}
		out = append(out, query.MCPServerStatus{Name: name, Status: status})
	}
	observe.GlobalTrace("return: out")
	return out
}

func validateActiveModel(d *Deps, modelID string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if strings.TrimSpace(modelID) == "" {
		observe.GlobalTrace("if: strings.TrimSpace(modelID) == \"\"")
		observe.GlobalTrace("return: fmt.Errorf(\"model is required\")")
		return fmt.Errorf("model is required")
	}
	if ml, ok := d.Prov.(provider.ModelLister); ok {
		observe.GlobalTrace("if: ok")
		for _, known := range ml.ListModels() {
			observe.GlobalTrace("range ml.ListModels()")
			if known == modelID {
				observe.GlobalTrace("return: nil")
				return nil
			}
		}
		if d.Cfg.Provider == "anthropic" {
			observe.GlobalTrace("if: d.Cfg.Provider == \"anthropic\"")
			if _, ok := anthropic.LookupModel(modelID); ok {
				observe.GlobalTrace("return: nil")
				return nil
			}
		}
		observe.GlobalTrace("return: fmt.Errorf(\"unknown model %q for provider %s\", modelID, d.Cfg.Provider)")
		return fmt.Errorf("unknown model %q for provider %s", modelID, d.Cfg.Provider)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func switchActiveModel(d *Deps, modelID string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if err := validateActiveModel(d, modelID); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: err")
		return err
	}
	d.Store.Update(func(s *app.AppState) {
		s.Model = modelID
		s.Conversation.Model = modelID
	})
	if d.Engine != nil {
		observe.GlobalTrace("if: d.Engine != nil")
		d.Engine.RebindProvider(d.Prov, modelID)
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func activeModelForDeps(d *Deps) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d != nil && d.Store != nil {
		observe.GlobalTrace("if: d != nil && d.Store != nil")
		if modelID := strings.TrimSpace(d.Store.Snapshot().Model); modelID != "" {
			observe.GlobalTrace("return: modelID")
			return modelID
		}
	}
	if d != nil {
		observe.GlobalTrace("return: d.Cfg.Model")
		return d.Cfg.Model
	}
	observe.GlobalTrace("return: \"\"")
	return ""
}

func runtimeSkillCatalog(d *Deps) skill.Catalog {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d == nil {
		observe.GlobalTrace("return: nil")
		return nil
	}
	catalog := skill.NewRuntimeCatalog(d.Cwd, func() string {
		if d.Store == nil {
			return d.Cwd
		}
		return d.Store.Snapshot().CWD
	})
	observe.GlobalTrace("return: catalog")
	return catalog
}

func runtimeCapabilitiesForDeps(d *Deps) func(context.Context, string) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: func(ctx context.Context, workDir string) {\n\tif d == nil {\n\t\treturn\n\t}\n\t_ = r...")
	return func(ctx context.Context, workDir string) {
		if d == nil {
			return
		}
		_ = refreshCapabilitiesForWorkDir(ctx, d, workDir)
	}
}
