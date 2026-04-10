package cli

import (
	"fmt"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/permission"
	"github.com/artpar/gogent/internal/query"
	"github.com/artpar/gogent/internal/tool"
	toolagent "github.com/artpar/gogent/internal/tools/agent"
	toolask "github.com/artpar/gogent/internal/tools/ask"
	toolbash "github.com/artpar/gogent/internal/tools/bash"
	toolfileedit "github.com/artpar/gogent/internal/tools/fileedit"
	toolfileread "github.com/artpar/gogent/internal/tools/fileread"
	toolfilewrite "github.com/artpar/gogent/internal/tools/filewrite"
	toolglob "github.com/artpar/gogent/internal/tools/glob"
	toolgrep "github.com/artpar/gogent/internal/tools/grep"
	toolnotebookedit "github.com/artpar/gogent/internal/tools/notebookedit"
	toolplan "github.com/artpar/gogent/internal/tools/plan"
	toolsearch "github.com/artpar/gogent/internal/tools/search"
	toolsleep "github.com/artpar/gogent/internal/tools/sleep"
	tooltaskcreate "github.com/artpar/gogent/internal/tools/taskcreate"
	tooltaskget "github.com/artpar/gogent/internal/tools/taskget"
	tooltasklist "github.com/artpar/gogent/internal/tools/tasklist"
	tooltaskstop "github.com/artpar/gogent/internal/tools/taskstop"
	tooltaskupdate "github.com/artpar/gogent/internal/tools/taskupdate"
	tooltodo "github.com/artpar/gogent/internal/tools/todo"
	toolwebfetch "github.com/artpar/gogent/internal/tools/webfetch"
)

// RegisterTools registers all tools on the registry. The agent tool needs the
// engine factory, which depends on the prompter — so it's built here.
func RegisterTools(d *Deps, prompter permission.Prompter, asker tool.Asker) (*query.Engine, error) {
	// Sub-agent engine factory
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

	// Register all base tools
	for _, td := range BaseTools(d) {
		if err := d.Registry.Register(td); err != nil {
			return nil, fmt.Errorf("register tool %s: %w", td.Name(), err)
		}
	}

	// Register tools that need the prompter/asker
	agentTool := &toolagent.Tool{EngineFactory: engineFactory, Store: d.Store, Tasks: d.TaskReg, Bus: d.Bus}
	if err := d.Registry.Register(agentTool); err != nil {
		return nil, fmt.Errorf("register agent tool: %w", err)
	}
	askTool := &toolask.Tool{Asker: asker}
	if err := d.Registry.Register(askTool); err != nil {
		return nil, fmt.Errorf("register ask tool: %w", err)
	}

	// Create main orchestrator and engine
	orchestrator := tool.NewOrchestrator(d.Registry, d.Checker, prompter, d.Bus)
	engine := query.NewEngine(d.Prov, d.Registry, orchestrator, d.Store, d.CostTracker, d.Bus, d.EngineCfg)
	return engine, nil
}

// BaseTools returns all tool descriptors except Agent and AskUserQuestion
// (which need the engine factory / asker).
func BaseTools(d *Deps) []tool.Descriptor {
	return []tool.Descriptor{
		&toolglob.Tool{},
		&toolgrep.Tool{},
		&toolfileread.Tool{},
		&toolfilewrite.Tool{},
		&toolfileedit.Tool{},
		&toolbash.Tool{},
		&toolnotebookedit.Tool{},
		&toolwebfetch.Tool{Provider: d.Prov, Bus: d.Bus, SecondaryModel: SecondaryModelFor(d.Cfg.Provider)},
		&tooltaskcreate.Tool{Tasks: d.TaskReg},
		&tooltaskget.Tool{Tasks: d.TaskReg},
		&tooltasklist.Tool{Tasks: d.TaskReg},
		&tooltaskupdate.Tool{Tasks: d.TaskReg},
		&tooltaskstop.Tool{Tasks: d.TaskReg},
		&toolsleep.Tool{},
		&tooltodo.Tool{Store: d.Store},
		&toolplan.EnterTool{Store: d.Store},
		&toolplan.ExitTool{Store: d.Store},
		&toolsearch.Tool{Registry: d.Registry},
	}
}
