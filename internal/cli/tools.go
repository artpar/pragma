package cli

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/provider/anthropic"
	"github.com/artpar/pragma/internal/query"
	"github.com/artpar/pragma/internal/remote"
	"github.com/artpar/pragma/internal/skill"
	"github.com/artpar/pragma/internal/tool"
	toolagent "github.com/artpar/pragma/internal/tools/agent"
	toolapplypatch "github.com/artpar/pragma/internal/tools/applypatch"
	toolask "github.com/artpar/pragma/internal/tools/ask"
	toolbash "github.com/artpar/pragma/internal/tools/bash"
	toolbrief "github.com/artpar/pragma/internal/tools/brief"
	toolconfig "github.com/artpar/pragma/internal/tools/config"
	toolcron "github.com/artpar/pragma/internal/tools/cron"
	toolfileedit "github.com/artpar/pragma/internal/tools/fileedit"
	toolfilewrite "github.com/artpar/pragma/internal/tools/filewrite"
	toolglob "github.com/artpar/pragma/internal/tools/glob"
	toolgrep "github.com/artpar/pragma/internal/tools/grep"
	toollifecycle "github.com/artpar/pragma/internal/tools/lifecycle"
	toolmcp "github.com/artpar/pragma/internal/tools/mcp"
	toolnotebookedit "github.com/artpar/pragma/internal/tools/notebookedit"
	toolpowershell "github.com/artpar/pragma/internal/tools/powershell"
	toolremote "github.com/artpar/pragma/internal/tools/remote"
	toolrepl "github.com/artpar/pragma/internal/tools/repl"
	toolselftrace "github.com/artpar/pragma/internal/tools/selftrace"
	toolsendmsg "github.com/artpar/pragma/internal/tools/sendmsg"
	toolskill "github.com/artpar/pragma/internal/tools/skill"
	toolsleep "github.com/artpar/pragma/internal/tools/sleep"
	tooltaskcreate "github.com/artpar/pragma/internal/tools/taskcreate"
	tooltaskget "github.com/artpar/pragma/internal/tools/taskget"
	tooltasklist "github.com/artpar/pragma/internal/tools/tasklist"
	tooltaskoutput "github.com/artpar/pragma/internal/tools/taskoutput"
	tooltaskstop "github.com/artpar/pragma/internal/tools/taskstop"
	tooltaskupdate "github.com/artpar/pragma/internal/tools/taskupdate"
	toolteamcreate "github.com/artpar/pragma/internal/tools/teamcreate"
	toolteamdelete "github.com/artpar/pragma/internal/tools/teamdelete"
	tooltodo "github.com/artpar/pragma/internal/tools/todo"
	toolresultread "github.com/artpar/pragma/internal/tools/toolresultread"
	tooltoolsearch "github.com/artpar/pragma/internal/tools/toolsearch"
	toolwebfetch "github.com/artpar/pragma/internal/tools/webfetch"
	toolwebsearch "github.com/artpar/pragma/internal/tools/websearch"
	toolworktree "github.com/artpar/pragma/internal/tools/worktree"
)

// RegisterTools registers all tools on the registry. The agent tool needs the
// engine factory, which depends on the prompter — so it's built here.
func RegisterTools(d *Deps, prompter permission.Prompter, asker tool.Asker) (*query.Engine, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	d.EngineCfg.MCPServerStatuses = func() []query.MCPServerStatus {
		return mcpStatusesForQuery(d.McpManager)
	}

	engineFactory := func(forkedConv model.Conversation, scopedToolNames []string, modelOverride string) (*query.Engine, *app.StateStore) {
		subModel := activeModelForDeps(d)
		if modelOverride != "" {
			subModel = modelOverride
		}
		forkedConv.Model = subModel
		forkedConv.Provider = d.Cfg.Provider
		parentSessionID := ""
		if d.Store != nil {
			parentSessionID = d.Store.Snapshot().SessionID()
		}
		subStore := app.NewStateStore(app.AppState{
			Conversation:      forkedConv,
			CWD:               d.Cwd,
			Model:             subModel,
			Provider:          d.Cfg.Provider,
			MaxTokens:         d.Cfg.MaxTokens,
			Temperature:       d.Cfg.Temperature,
			ArtifactSessionID: parentSessionID,
		})
		subRegistry := tool.NewRegistry(d.Bus)
		for _, td := range baseTools(d, subStore) {
			if !shouldRegisterBuiltinTool(d, td.Name()) {
				observe.GlobalTrace("if: !shouldRegisterBuiltinTool(d, td.Name())")
				continue
			}
			_ = subRegistry.Register(td)
		}
		subRegistry.SetHidden(map[string]bool{toolapplypatch.LegacyToolName: true})
		if scopedToolNames != nil {
			subRegistry = subRegistry.Scoped(scopedToolNames)
		}
		subOrch := tool.NewOrchestrator(subRegistry, d.Checker, prompter, d.Bus)
		subCfg := d.EngineCfg
		subCfg.Model = subModel
		subEngine := query.NewEngine(d.Prov, subRegistry, subOrch, subStore, d.CostTracker, d.Bus, subCfg)
		if d.Engine != nil {
			subEngine.SetFileStateCache(d.Engine.FileStateCache())
		}
		return subEngine, subStore
	}

	for _, td := range BaseTools(d) {
		observe.GlobalTrace("range BaseTools(d)")
		if !shouldRegisterBuiltinTool(d, td.Name()) {
			observe.GlobalTrace("if: !shouldRegisterBuiltinTool(d, td.Name())")
			continue
		}
		if err := d.Registry.Register(td); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register tool %s: %w\", td.Name(), err)")
			return nil, fmt.Errorf("register tool %s: %w", td.Name(), err)
		}
	}
	d.Registry.SetHidden(map[string]bool{toolapplypatch.LegacyToolName: true})

	agentTool := &toolagent.Tool{
		EngineFactory:  engineFactory,
		Store:          d.Store,
		Tasks:          d.TaskReg,
		TaskContext:    d.TaskContext,
		Bus:            d.Bus,
		Provider:       d.Prov,
		SecondaryModel: SecondaryModelFor(d.Cfg.Provider),
	}
	if shouldRegisterBuiltinTool(d, agentTool.Name()) {
		observe.GlobalTrace("if: shouldRegisterBuiltinTool(d, agentTool.Name())")
		if err := d.Registry.Register(agentTool); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register agent tool: %w\", err)")
			return nil, fmt.Errorf("register agent tool: %w", err)
		}
	}
	askTool := &toolask.Tool{Asker: asker}
	if shouldRegisterBuiltinTool(d, askTool.Name()) {
		observe.GlobalTrace("if: shouldRegisterBuiltinTool(d, askTool.Name())")
		if err := d.Registry.Register(askTool); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register ask tool: %w\", err)")
			return nil, fmt.Errorf("register ask tool: %w", err)
		}
	}

	skillLoader := skill.NewLoader(d.Cwd)
	skillTool := &toolskill.Tool{
		Agent:  agentTool,
		Loader: skillLoader,
	}
	if shouldRegisterBuiltinTool(d, skillTool.Name()) {
		observe.GlobalTrace("if: shouldRegisterBuiltinTool(d, skillTool.Name())")
		if err := d.Registry.Register(skillTool); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register skill tool: %w\", err)")
			return nil, fmt.Errorf("register skill tool: %w", err)
		}
	}

	orchestrator := tool.NewOrchestrator(d.Registry, d.Checker, prompter, d.Bus)
	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		orchestrator.SetHookManager(d.HookMgr)
	}

	if os.Getenv("PRAGMA_REPL") == "1" && shouldRegisterBuiltinTool(d, "REPL") {
		observe.GlobalTrace("if: os.Getenv(\"PRAGMA_REPL\") == \"1\"")
		replTool := &toolrepl.Tool{Registry: d.Registry, Orchestrator: orchestrator, Bus: d.Bus}
		if err := d.Registry.Register(replTool); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register REPL tool: %w\", err)")
			return nil, fmt.Errorf("register REPL tool: %w", err)
		}
		d.Registry.SetHidden(toolrepl.PrimitiveToolNames)
	}

	lifecycleTool := &toollifecycle.Tool{
		Provider:       d.Prov,
		Orchestrator:   orchestrator,
		Registry:       d.Registry,
		Bus:            d.Bus,
		Store:          d.Store,
		SecondaryModel: SecondaryModelFor(d.Cfg.Provider),
	}
	if shouldRegisterBuiltinTool(d, lifecycleTool.Name()) {
		observe.GlobalTrace("if: shouldRegisterBuiltinTool(d, lifecycleTool.Name())")
		if err := d.Registry.Register(lifecycleTool); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register lifecycle tool: %w\", err)")
			return nil, fmt.Errorf("register lifecycle tool: %w", err)
		}
	}

	engine := query.NewEngine(d.Prov, d.Registry, orchestrator, d.Store, d.CostTracker, d.Bus, d.EngineCfg)
	d.Engine = engine
	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		engine.SetHookManager(d.HookMgr)
	}
	observe.GlobalTrace("return: engine, nil")
	return engine, nil
}

func RebindProviderBackedTools(d *Deps) {
	secondaryModel := SecondaryModelFor(d.Cfg.Provider)
	if desc, ok := d.Registry.Get("Agent"); ok {
		if tl, ok := desc.(*toolagent.Tool); ok {
			tl.Provider = d.Prov
			tl.SecondaryModel = secondaryModel
		}
	}
	if desc, ok := d.Registry.Get("LifecycleRun"); ok {
		if tl, ok := desc.(*toollifecycle.Tool); ok {
			tl.Provider = d.Prov
			tl.SecondaryModel = secondaryModel
		}
	}
	if desc, ok := d.Registry.Get("WebFetch"); ok {
		if tl, ok := desc.(*toolwebfetch.Tool); ok {
			tl.Provider = d.Prov
			tl.SecondaryModel = secondaryModel
		}
	}
}

func shouldRegisterBuiltinTool(d *Deps, name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d != nil && !d.ToolPolicy.Allows(name) {
		observe.GlobalTrace("if: d != nil && !d.ToolPolicy.Allows(name)")
		return false
	}
	if isRuntimeBuiltinTool(name) {
		observe.GlobalTrace("if: isRuntimeBuiltinTool(name)")
		observe.GlobalTrace("return: true")
		return true
	}
	if d == nil || d.Toolset == nil {
		observe.GlobalTrace("if: d == nil || d.Toolset == nil")
		observe.GlobalTrace("return: true")
		return true
	}
	if name == toolapplypatch.ToolName && d.Toolset.AllowBuiltinTool(toolapplypatch.LegacyToolName) {
		observe.GlobalTrace("if: name == toolapplypatch.ToolName && d.Toolset.AllowBuiltinTool(toolapplypatch....")
		observe.GlobalTrace("return: true")
		return true
	}
	if name == toolapplypatch.LegacyToolName && d.Toolset.AllowBuiltinTool(toolapplypatch.ToolName) {
		observe.GlobalTrace("if: name == toolapplypatch.LegacyToolName && d.Toolset.AllowBuiltinTool(toolapply...")
		observe.GlobalTrace("return: true")
		return true
	}
	observe.GlobalTrace("return: d.Toolset.AllowBuiltinTool(name)")
	return d.Toolset.AllowBuiltinTool(name)
}

type ToolExposurePolicy struct {
	HasAllowed bool
	Allowed    map[string]bool
	Disallowed map[string]bool
}

func toolExposurePolicyFromFlags(cmd *cobra.Command) ToolExposurePolicy {
	var policy ToolExposurePolicy
	if cmd == nil || cmd.Flags() == nil {
		return policy
	}
	if allowedStr, _ := cmd.Flags().GetString("allowed-tools"); allowedStr != "" {
		allowed := parseToolList(allowedStr)
		policy.HasAllowed = true
		policy.Allowed = make(map[string]bool, len(allowed))
		for _, name := range allowed {
			policy.Allowed[name] = true
		}
	}
	if disallowedStr, _ := cmd.Flags().GetString("disallowed-tools"); disallowedStr != "" {
		disallowed := parseToolList(disallowedStr)
		policy.Disallowed = make(map[string]bool, len(disallowed))
		for _, name := range disallowed {
			policy.Disallowed[name] = true
		}
	}
	return policy
}

func (p ToolExposurePolicy) Allows(name string) bool {
	if p.HasAllowed && !p.Allowed[name] {
		return false
	}
	if p.Disallowed[name] {
		return false
	}
	return true
}

func patchModeActive(d *Deps) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: shouldRegisterBuiltinTool(d, toolapplypatch.ToolName) || shouldRegisterBuilti...")
	return shouldRegisterBuiltinTool(d, toolapplypatch.ToolName) || shouldRegisterBuiltinTool(d, toolapplypatch.LegacyToolName)
}

func isRuntimeBuiltinTool(name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: name == \"tool_result.read\"")
	return name == "tool_result.read"
}

func mcpStatusesForQuery(mgr interface {
	ServerStatuses() []mcp.ServerStatusInfo
}) []query.MCPServerStatus {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if mgr == nil {
		observe.GlobalTrace("if: mgr == nil")
		observe.GlobalTrace("return: nil")
		return nil
	}
	statuses := mgr.ServerStatuses()
	out := make([]query.MCPServerStatus, 0, len(statuses))
	for _, st := range statuses {
		observe.GlobalTrace("range statuses")
		out = append(out, query.MCPServerStatus{Name: st.Name, Status: st.Status})
	}
	observe.GlobalTrace("return: out")
	return out
}

// BaseTools returns all tool descriptors except Agent and AskUserQuestion
// (which need the engine factory / asker).
func BaseTools(d *Deps) []tool.Descriptor {
	return baseTools(d, d.Store)
}

func baseTools(d *Deps, store *app.StateStore) []tool.Descriptor {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: []tool.Descriptor{\n\t&toolglob.Tool{},\n\t&toolgrep.Tool{},\n\t&toolapplypatch.Tool{...")
	tools := []tool.Descriptor{
		&toolglob.Tool{},
		&toolgrep.Tool{},
		&toolapplypatch.Tool{},
		&toolapplypatch.LegacyTool{},
		&toolfilewrite.Tool{PatchMode: patchModeActive(d)},
		&toolfileedit.Tool{PatchMode: patchModeActive(d)},
		&toolbash.Tool{PatchMode: patchModeActive(d)},
		&toolnotebookedit.Tool{},
		&toolwebfetch.Tool{Provider: d.Prov, Bus: d.Bus, SecondaryModel: SecondaryModelFor(d.Cfg.Provider)},
		&tooltaskcreate.Tool{Tasks: d.TaskReg},
		&tooltaskget.Tool{Tasks: d.TaskReg},
		&tooltasklist.Tool{Tasks: d.TaskReg},
		&tooltaskupdate.Tool{Tasks: d.TaskReg},
		&tooltaskstop.Tool{Tasks: d.TaskReg},
		&tooltaskoutput.Tool{Tasks: d.TaskReg},
		&toolsleep.Tool{},
		&tooltodo.Tool{Store: d.Store},
		&tooltoolsearch.Tool{
			Registry: d.Registry,
			PendingMCPServers: func() []string {
				if d.McpManager == nil {
					return nil
				}
				return d.McpManager.PendingServerNames()
			},
		},
		&toolresultread.Tool{},
		&toolworktree.EnterTool{Store: store},
		&toolworktree.ExitTool{Store: store},
		&toolcron.CreateTool{Scheduler: d.CronSched},
		&toolcron.DeleteTool{Scheduler: d.CronSched},
		&toolcron.ListTool{Scheduler: d.CronSched},
		&toolsendmsg.Tool{Tasks: d.TaskReg},
		&toolwebsearch.Tool{Token: d.Creds.CredentialFor("brave").APIKey},
		&toolbrief.Tool{Bus: d.Bus},
		&toolconfig.Tool{
			Store:             d.Store,
			WorkDir:           d.Cwd,
			ValidateLiveValue: validateLiveConfigValue(d),
			ApplyLiveValue:    applyLiveConfigValue(d),
		},
		&toolselftrace.Tool{LogFilePath: d.LogFilePath},
		&toolpowershell.Tool{},
	}

	if os.Getenv("PRAGMA_FEATURE_AGENT_TEAMS") == "1" {
		observe.GlobalTrace("if: os.Getenv(\"PRAGMA_FEATURE_AGENT_TEAMS\") == \"1\"")
		tools = append(tools,
			&toolteamcreate.Tool{Store: d.Store, Bus: d.Bus},
			&toolteamdelete.Tool{Store: d.Store, Tasks: d.TaskReg, Bus: d.Bus},
		)
	}

	if d.McpManager != nil {
		observe.GlobalTrace("if: d.McpManager != nil")
		tools = append(tools,
			&toolmcp.ListTool{Manager: d.McpManager},
		)
	}

	if os.Getenv("PRAGMA_FEATURE_REMOTE_TRIGGERS") == "1" {
		observe.GlobalTrace("if: os.Getenv(\"PRAGMA_FEATURE_REMOTE_TRIGGERS\") == \"1\"")
		client := anthropic.NewRemoteTriggerClient(&http.Client{Timeout: 20 * time.Second}, "https://api.anthropic.com",
			func() (string, error) {
				token := os.Getenv("ANTHROPIC_OAUTH_TOKEN")
				if token == "" {
					return "", fmt.Errorf("ANTHROPIC_OAUTH_TOKEN not set")
				}
				return token, nil
			},
			func() (string, error) {
				uuid := os.Getenv("ANTHROPIC_ORG_UUID")
				if uuid == "" {
					return "", fmt.Errorf("ANTHROPIC_ORG_UUID not set")
				}
				return uuid, nil
			},
		)
		tools = append(tools, &toolremote.Tool{
			Service: remote.NewService(client),
		})
	}
	observe.GlobalTrace("return: tools")

	return tools
}

func validateLiveConfigValue(d *Deps) func(string, any) error {
	return func(setting string, value any) error {
		switch setting {
		case "model":
			modelID, ok := value.(string)
			if !ok {
				return fmt.Errorf("model must be a string")
			}
			return validateActiveModel(d, modelID)
		default:
			return nil
		}
	}
}

func applyLiveConfigValue(d *Deps) func(string, any) {
	return func(setting string, value any) {
		switch setting {
		case "model":
			modelID, ok := value.(string)
			if !ok {
				return
			}
			if d.ModelSwitcher != nil {
				_ = d.ModelSwitcher(modelID)
				return
			}
			_ = switchActiveModel(d, modelID)
		}
	}
}

func validateActiveModel(d *Deps, modelID string) error {
	if d == nil || d.Prov == nil {
		return nil
	}
	if _, ok := d.Prov.ContextWindow(modelID); !ok {
		return fmt.Errorf("Unknown model: %s", modelID)
	}
	return nil
}

func switchActiveModel(d *Deps, modelID string) error {
	if err := validateActiveModel(d, modelID); err != nil {
		return err
	}
	if d != nil {
		d.Cfg.Model = modelID
		d.EngineCfg.Model = modelID
		if d.Engine != nil {
			d.Engine.SetModel(modelID)
		}
	}
	if d != nil && d.Store != nil {
		d.Store.Update(func(s *app.AppState) {
			s.Model = modelID
		})
	}
	if d != nil && d.Prov != nil && d.TokenMonitor != nil {
		if cw, ok := d.Prov.ContextWindow(modelID); ok {
			d.TokenMonitor.SetBudget(cw)
		}
	}
	return nil
}

func activeModelForDeps(d *Deps) string {
	if d == nil {
		return ""
	}
	if d.Store != nil {
		snap := d.Store.Snapshot()
		if snap.Model != "" {
			return snap.Model
		}
	}
	if d.EngineCfg.Model != "" {
		return d.EngineCfg.Model
	}
	return d.Cfg.Model
}
