package cli

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/mcp"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	"github.com/artpar/pragma/internal/query"
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
	toolfileread "github.com/artpar/pragma/internal/tools/fileread"
	toolfilewrite "github.com/artpar/pragma/internal/tools/filewrite"
	toolglob "github.com/artpar/pragma/internal/tools/glob"
	toolgrep "github.com/artpar/pragma/internal/tools/grep"
	toollifecycle "github.com/artpar/pragma/internal/tools/lifecycle"
	toolmcp "github.com/artpar/pragma/internal/tools/mcp"
	toolnotebookedit "github.com/artpar/pragma/internal/tools/notebookedit"
	toolplan "github.com/artpar/pragma/internal/tools/plan"
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
		subRegistry := tool.NewRegistry(d.Bus)
		for _, td := range BaseTools(d) {
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
		subStore := app.NewStateStore(app.AppState{
			Conversation: forkedConv,
			CWD:          d.Cwd,
			Model:        d.Cfg.Model,
			Provider:     d.Cfg.Provider,
			MaxTokens:    d.Cfg.MaxTokens,
			Temperature:  d.Cfg.Temperature,
		})
		subOrch := tool.NewOrchestrator(subRegistry, d.Checker, prompter, d.Bus)
		subCfg := d.EngineCfg
		if modelOverride != "" {
			subCfg.Model = modelOverride
		}
		return query.NewEngine(d.Prov, subRegistry, subOrch, subStore, d.CostTracker, d.Bus, subCfg), subStore
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
		EngineFactory: engineFactory,
		Store:         d.Store,
		Bus:           d.Bus,
		Loader:        skillLoader,
	}
	if shouldRegisterBuiltinTool(d, skillTool.Name()) {
		observe.GlobalTrace("if: shouldRegisterBuiltinTool(d, skillTool.Name())")
		if err := d.Registry.Register(skillTool); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register skill tool: %w\", err)")
			return nil, fmt.Errorf("register skill tool: %w", err)
		}
	}

	if os.Getenv("PRAGMA_REPL") == "1" && shouldRegisterBuiltinTool(d, "REPL") {
		observe.GlobalTrace("if: os.Getenv(\"PRAGMA_REPL\") == \"1\"")
		replTool := &toolrepl.Tool{Registry: d.Registry, Bus: d.Bus}
		if err := d.Registry.Register(replTool); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register REPL tool: %w\", err)")
			return nil, fmt.Errorf("register REPL tool: %w", err)
		}
		d.Registry.SetHidden(toolrepl.PrimitiveToolNames)
	}

	orchestrator := tool.NewOrchestrator(d.Registry, d.Checker, prompter, d.Bus)
	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		orchestrator.SetHookManager(d.HookMgr)
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
	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		engine.SetHookManager(d.HookMgr)
	}
	observe.GlobalTrace("return: engine, nil")
	return engine, nil
}

func shouldRegisterBuiltinTool(d *Deps, name string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
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
		return true
	}
	if name == toolapplypatch.LegacyToolName && d.Toolset.AllowBuiltinTool(toolapplypatch.ToolName) {
		return true
	}
	observe.GlobalTrace("return: d.Toolset.AllowBuiltinTool(name)")
	return d.Toolset.AllowBuiltinTool(name)
}

func patchModeActive(d *Deps) bool {
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: []tool.Descriptor{\n\t&toolglob.Tool{},\n\t&toolgrep.Tool{},\n\t&toolfileread.Tool{...")
	tools := []tool.Descriptor{
		&toolglob.Tool{},
		&toolgrep.Tool{},
		&toolfileread.Tool{},
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
		&toolplan.EnterTool{Store: d.Store},
		&toolplan.ExitTool{Store: d.Store},
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
		&toolworktree.EnterTool{},
		&toolworktree.ExitTool{},
		&toolcron.CreateTool{Scheduler: d.CronSched},
		&toolcron.DeleteTool{Scheduler: d.CronSched},
		&toolcron.ListTool{Scheduler: d.CronSched},
		&toolsendmsg.Tool{Tasks: d.TaskReg},
		&toolwebsearch.Tool{Token: d.Creds.CredentialFor("brave").APIKey},
		&toolbrief.Tool{Bus: d.Bus},
		&toolconfig.Tool{Store: d.Store, WorkDir: d.Cwd},
		&toolselftrace.Tool{LogFilePath: d.LogFilePath},
		&toolpowershell.Tool{},
	}

	if os.Getenv("PRAGMA_FEATURE_AGENT_TEAMS") == "1" {
		observe.GlobalTrace("if: os.Getenv(\"PRAGMA_FEATURE_AGENT_TEAMS\") == \"1\"")
		tools = append(tools,
			&toolteamcreate.Tool{Store: d.Store, Bus: d.Bus},
			&toolteamdelete.Tool{Store: d.Store, Bus: d.Bus},
		)
	}

	if d.McpManager != nil {
		observe.GlobalTrace("if: d.McpManager != nil")
		tools = append(tools,
			&toolmcp.ListTool{Manager: d.McpManager},
			&toolmcp.ReadTool{Manager: d.McpManager},
		)
	}

	if os.Getenv("PRAGMA_FEATURE_REMOTE_TRIGGERS") == "1" {
		observe.GlobalTrace("if: os.Getenv(\"PRAGMA_FEATURE_REMOTE_TRIGGERS\") == \"1\"")
		tools = append(tools, &toolremote.Tool{
			HTTPClient: &http.Client{Timeout: 20 * time.Second},
			BaseURL:    "https://api.anthropic.com",
			TokenSource: func() (string, error) {
				token := os.Getenv("ANTHROPIC_OAUTH_TOKEN")
				if token == "" {
					return "", fmt.Errorf("ANTHROPIC_OAUTH_TOKEN not set")
				}
				return token, nil
			},
			OrgUUID: func() (string, error) {
				uuid := os.Getenv("ANTHROPIC_ORG_UUID")
				if uuid == "" {
					return "", fmt.Errorf("ANTHROPIC_ORG_UUID not set")
				}
				return uuid, nil
			},
		})
	}
	observe.GlobalTrace("return: tools")

	return tools
}
