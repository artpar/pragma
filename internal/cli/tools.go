package cli

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/skill"
	"github.com/artpar/gogent/internal/tool"
	toolagent "github.com/artpar/gogent/internal/tools/agent"
	toolask "github.com/artpar/gogent/internal/tools/ask"
	toolbash "github.com/artpar/gogent/internal/tools/bash"
	toolbrief "github.com/artpar/gogent/internal/tools/brief"
	toolconfig "github.com/artpar/gogent/internal/tools/config"
	toolcron "github.com/artpar/gogent/internal/tools/cron"
	toolfileedit "github.com/artpar/gogent/internal/tools/fileedit"
	toolfileread "github.com/artpar/gogent/internal/tools/fileread"
	toolfilewrite "github.com/artpar/gogent/internal/tools/filewrite"
	toolglob "github.com/artpar/gogent/internal/tools/glob"
	toolgrep "github.com/artpar/gogent/internal/tools/grep"
	toollsp "github.com/artpar/gogent/internal/tools/lsp"
	toolmcp "github.com/artpar/gogent/internal/tools/mcp"
	toolnotebookedit "github.com/artpar/gogent/internal/tools/notebookedit"
	toolplan "github.com/artpar/gogent/internal/tools/plan"
	toolremote "github.com/artpar/gogent/internal/tools/remote"
	toolsendmsg "github.com/artpar/gogent/internal/tools/sendmsg"
	toolskill "github.com/artpar/gogent/internal/tools/skill"
	toolsleep "github.com/artpar/gogent/internal/tools/sleep"
	tooltaskcreate "github.com/artpar/gogent/internal/tools/taskcreate"
	tooltaskget "github.com/artpar/gogent/internal/tools/taskget"
	tooltasklist "github.com/artpar/gogent/internal/tools/tasklist"
	tooltaskoutput "github.com/artpar/gogent/internal/tools/taskoutput"
	tooltaskstop "github.com/artpar/gogent/internal/tools/taskstop"
	tooltaskupdate "github.com/artpar/gogent/internal/tools/taskupdate"
	tooltodo "github.com/artpar/gogent/internal/tools/todo"
	tooltoolsearch "github.com/artpar/gogent/internal/tools/toolsearch"
	toolwebfetch "github.com/artpar/gogent/internal/tools/webfetch"
	toolwebsearch "github.com/artpar/gogent/internal/tools/websearch"
	toolworktree "github.com/artpar/gogent/internal/tools/worktree"
)

// RegisterTools registers all tools on the registry. The agent tool needs the
// engine factory, which depends on the prompter — so it's built here.
func RegisterTools(d *Deps, prompter permission.Prompter, asker tool.Asker) (*query.Engine, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	engineFactory := func(forkedConv model.Conversation, scopedToolNames []string, modelOverride string) (*query.Engine, *app.StateStore) {
		subRegistry := tool.NewRegistry(d.Bus)
		for _, td := range BaseTools(d) {
			_ = subRegistry.Register(td)
		}
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
		if err := d.Registry.Register(td); err != nil {
			observe.GlobalTrace("if: err != nil")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register tool %s: %w\", td.Name(), err)")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register tool %s: %w\", td.Name(), err)")
			observe.GlobalTrace("return: nil, fmt.Errorf(\"register tool %s: %w\", td.Name(), err)")
			return nil, fmt.Errorf("register tool %s: %w", td.Name(), err)
		}
	}

	agentTool := &toolagent.Tool{EngineFactory: engineFactory, Store: d.Store, Tasks: d.TaskReg, Bus: d.Bus}
	if err := d.Registry.Register(agentTool); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"register agent tool: %w\", err)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"register agent tool: %w\", err)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"register agent tool: %w\", err)")
		return nil, fmt.Errorf("register agent tool: %w", err)
	}
	askTool := &toolask.Tool{Asker: asker}
	if err := d.Registry.Register(askTool); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"register ask tool: %w\", err)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"register ask tool: %w\", err)")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"register ask tool: %w\", err)")
		return nil, fmt.Errorf("register ask tool: %w", err)
	}

	skillLoader := skill.NewLoader(d.Cwd)
	skillTool := &toolskill.Tool{
		EngineFactory: engineFactory,
		Store:         d.Store,
		Bus:           d.Bus,
		Loader:        skillLoader,
	}
	if err := d.Registry.Register(skillTool); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: nil, fmt.Errorf(\"register skill tool: %w\", err)")
		return nil, fmt.Errorf("register skill tool: %w", err)
	}

	orchestrator := tool.NewOrchestrator(d.Registry, d.Checker, prompter, d.Bus)
	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		orchestrator.SetHookManager(d.HookMgr)
		orchestrator.SetPermPersister(&tool.PermPersister{
			WorkDir: d.Cwd,
			Persist: permission.PersistRule,
		})
	}
	engine := query.NewEngine(d.Prov, d.Registry, orchestrator, d.Store, d.CostTracker, d.Bus, d.EngineCfg)
	if d.HookMgr != nil {
		observe.GlobalTrace("if: d.HookMgr != nil")
		engine.SetHookManager(d.HookMgr)
	}
	observe.GlobalTrace("return: engine, nil")
	observe.GlobalTrace("return: engine, nil")
	observe.GlobalTrace("return: engine, nil")
	return engine, nil
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
		&toolfilewrite.Tool{LSP: d.LspManager},
		&toolfileedit.Tool{LSP: d.LspManager},
		&toolbash.Tool{},
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
		&tooltoolsearch.Tool{Registry: d.Registry},
		&toolmcp.ListTool{Manager: d.McpManager},
		&toolmcp.ReadTool{Manager: d.McpManager},
		&toolworktree.EnterTool{},
		&toolworktree.ExitTool{},
		&toolcron.CreateTool{Scheduler: d.CronSched},
		&toolcron.DeleteTool{Scheduler: d.CronSched},
		&toolcron.ListTool{Scheduler: d.CronSched},
		&toolsendmsg.Tool{Tasks: d.TaskReg},
		&toollsp.Tool{Manager: d.LspManager},
		&toolwebsearch.Tool{},
		&toolbrief.Tool{Bus: d.Bus},
		&toolconfig.Tool{Store: d.Store, WorkDir: d.Cwd},
	}

	if os.Getenv("GOGENT_FEATURE_REMOTE_TRIGGERS") == "1" {
		observe.GlobalTrace("if: os.Getenv(\"GOGENT_FEATURE_REMOTE_TRIGGERS\") == \"1\"")
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
	observe.GlobalTrace("return: tools")

	return tools
}
