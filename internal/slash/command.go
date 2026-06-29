package slash

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/compact"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/session"
	"github.com/artpar/pragma/internal/skill"
)

// ErrUnknownCommand is returned when no command matches the given name.
var ErrUnknownCommand = errors.New("unknown command")

// Result is the output of a slash command execution.
type Result struct {
	DisplayText       string // text to show in the interactive output surface
	ClearConversation bool   // true for /clear: presentation should reset visible conversation
	RewriteSession    bool   // true when conversation messages were replaced and the session log must be rewritten
	Quit              bool   // true for /exit: runtime should terminate after command handling
	InjectPrompt      string // if set, presentation feeds this as a user message to the engine
	Orchestrate       *OrchestrationRequest
	OpenModelPicker   bool // true for /model with no args: presentation opens model picker
	OpenResumePicker  bool // true for /resume with no args: presentation opens session picker
	ResumeCandidates  []ResumeCandidate
	ResumeScope       string
	ResumeSessionID   string // if set, presentation loads this session into conversation
}

type OrchestrationRequest struct {
	DefinitionPath string
	PersonaDir     string
	Prompt         string
	SeedArtifacts  map[string]string
	StartAtState   string
	StopAfterState string
}

// Handler is the function signature for a slash command handler.
type Handler func(ctx context.Context, args string, deps Deps) (Result, error)

// Deps bundles the dependencies available to slash command handlers.
type Deps struct {
	Store       *app.StateStore
	CostTracker *model.CostTracker
	Compactor   *compact.Service
	Bus         *observe.EventBus
	SessionSave func() error
	ModelName   string
	Provider    string
	Cwd         string // working directory for shell execution
	// Commands is set internally by Registry.Execute — not for external callers.
	Commands []Command

	// Model switching support — nil-safe (graceful degradation when unavailable).
	ModelLister       func() []string                  // returns available model names for current provider
	ContextWindowFunc func(modelID string) (int, bool) // validates model + returns context window
	ModelSwitcher     func(modelID string) error       // validates and applies a live model switch
	OnModelChanged    func(modelID string)             // callback: update budget + compaction on model switch

	// MCP status — nil-safe.
	McpStatus func() []McpServerStatus // returns configured MCP servers with connection state

	// Session + skill support — nil-safe.
	SessionStore *session.Store
	SkillLoader  *skill.Loader
	SkillCatalog skill.Catalog

	// Presentation-local support — nil-safe.
	LatestAssistantText func() string
	ClipboardWrite      func(string) error
}

type McpServerStatus struct {
	Name      string
	Status    string
	Error     string
	ToolCount int
	Transport string
}

// CommandType distinguishes how a command is executed.
type CommandType int

const (
	// TypeLocal commands need no engine — they produce a result directly (cost, model, doctor).
	TypeLocal CommandType = iota
	// TypePrompt commands inject a prompt into the engine for LLM processing (commit, review, init).
	TypePrompt
)

// Command describes a registered slash command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Handle      Handler

	// CLI metadata — zero-value CLIUse means "no CLI subcommand" (REPL-only).
	Type         CommandType // TypeLocal or TypePrompt
	CLIUse       string      // Cobra Use string (e.g., "commit", "review [pr-number]")
	CLIShort     string      // Override Description for CLI help (optional)
	AllowedTools []string    // Permission auto-approve list for prompt commands (nil = no special rules)
}

// Registry holds all registered slash commands.
// Exact match only — no fuzzy matching (GitHub issue #41828).
type Registry struct {
	commands map[string]*Command // name/alias → command
	ordered  []Command           // for /help listing
}

// NewRegistry creates a registry with all built-in commands registered.
func NewRegistry() *Registry {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	r := &Registry{
		commands: make(map[string]*Command),
	}
	registerBuiltins(r)
	observe.GlobalTrace("return: r")
	return r
}

// Register adds a command to the registry. Names and aliases are case-insensitive.
func (r *Registry) Register(cmd Command) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	c := &cmd
	r.commands[strings.ToLower(cmd.Name)] = c
	for _, alias := range cmd.Aliases {
		observe.GlobalTrace("range cmd.Aliases")
		r.commands[strings.ToLower(alias)] = c
	}
	r.ordered = append(r.ordered, cmd)
}

// Execute dispatches to the matching command handler and emits an observability event.
func (r *Registry) Execute(ctx context.Context, name, args string, deps Deps) (Result, error) {
	observe.TraceCtx(ctx, "slash", "Registry.Execute", "enter")
	defer observe.TraceCtx(ctx, "slash", "Registry.Execute", "exit")
	cmd, ok := r.commands[strings.ToLower(name)]
	if !ok {
		observe.TraceCtx(ctx, "slash", "Registry.Execute", "if: !ok")
		if result, found := skillCommandResult(name, args, deps); found {
			observe.TraceCtx(ctx, "slash", "Registry.Execute", "if: found")
			observe.TraceCtx(ctx, "slash", "Registry.Execute", "return: result, nil")
			return result, nil
		}
		observe.TraceCtx(ctx, "slash", "Registry.Execute", "if: !ok")
		observe.TraceCtx(ctx, "slash", "Registry.Execute", "return: Result{}, fmt.Errorf(\"%w: /%s\", ErrUnknownCommand, name)")
		return Result{}, fmt.Errorf("%w: /%s", ErrUnknownCommand, name)
	}
	if cmd.Handle == nil {
		observe.TraceCtx(ctx, "slash", "Registry.Execute", "if: cmd.Handle == nil")
		if result, found := skillCommandResult(name, args, deps); found {
			observe.TraceCtx(ctx, "slash", "Registry.Execute", "if: found")
			observe.TraceCtx(ctx, "slash", "Registry.Execute", "return: result, nil")
			return result, nil
		}
		observe.TraceCtx(ctx, "slash", "Registry.Execute", "return: Result{}, fmt.Errorf(\"%w: /%s\", ErrUnknownCommand, name)")
		return Result{}, fmt.Errorf("%w: /%s", ErrUnknownCommand, name)
	}
	deps.Commands = r.CommandsWithDeps(deps)
	start := time.Now()
	result, err := cmd.Handle(ctx, args, deps)
	if deps.Bus != nil {
		observe.TraceCtx(ctx, "slash", "Registry.Execute", "if: deps.Bus != nil")
		deps.Bus.Emit(observe.SlashCommandExecuted{
			EventHeader: observe.NewEventHeader("SlashCommandExecuted", "", "", ""),
			CommandName: cmd.Name,
			Args:        args,
			DurationMs:  time.Since(start).Milliseconds(),
			Success:     err == nil,
		})
	}
	observe.TraceCtx(ctx, "slash", "Registry.Execute", "return: result, err")
	return result, err
}

// Commands returns all registered commands sorted alphabetically.
func (r *Registry) Commands() []Command {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	sorted := make([]Command, len(r.ordered))
	copy(sorted, r.ordered)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})
	observe.GlobalTrace("return: sorted")
	return sorted
}

func (r *Registry) CommandsWithDeps(deps Deps) []Command {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	commands := r.Commands()
	catalog := deps.skillCatalog()
	if catalog == nil {
		observe.GlobalTrace("if: catalog == nil")
		observe.GlobalTrace("return: commands")
		return commands
	}
	skills, err := catalog.LoadAll()
	if err != nil || len(skills) == 0 {
		observe.GlobalTrace("if: err != nil || len(skills) == 0")
		observe.GlobalTrace("return: commands")
		return commands
	}
	seen := make(map[string]bool, len(commands)+len(skills))
	for _, cmd := range commands {
		observe.GlobalTrace("range commands")
		seen[strings.ToLower(cmd.Name)] = true
		for _, alias := range cmd.Aliases {
			observe.GlobalTrace("range cmd.Aliases")
			seen[strings.ToLower(alias)] = true
		}
	}
	for _, s := range skills {
		observe.GlobalTrace("range skills")
		name := strings.ToLower(strings.TrimSpace(s.Name))
		if name == "" || seen[name] {
			observe.GlobalTrace("if: name == \"\" || seen[name]")
			continue
		}
		commands = append(commands, Command{
			Name:        s.Name,
			Description: s.Description,
			Type:        TypePrompt,
		})
		seen[name] = true
	}
	sort.Slice(commands, func(i, j int) bool {
		return commands[i].Name < commands[j].Name
	})
	observe.GlobalTrace("return: commands")
	return commands
}

func (d Deps) skillCatalog() skill.Catalog {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if d.SkillCatalog != nil {
		observe.GlobalTrace("if: d.SkillCatalog != nil")
		observe.GlobalTrace("return: d.SkillCatalog")
		return d.SkillCatalog
	}
	if d.SkillLoader != nil {
		observe.GlobalTrace("if: d.SkillLoader != nil")
		observe.GlobalTrace("return: d.SkillLoader")
		return d.SkillLoader
	}
	observe.GlobalTrace("return: nil")
	return nil
}

func skillCommandResult(name, args string, deps Deps) (Result, bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	catalog := deps.skillCatalog()
	if catalog == nil {
		observe.GlobalTrace("if: catalog == nil")
		observe.GlobalTrace("return: Result{}, false")
		return Result{}, false
	}
	skillName := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(name), "/"))
	if skillName == "" {
		observe.GlobalTrace("if: skillName == \"\"")
		observe.GlobalTrace("return: Result{}, false")
		return Result{}, false
	}
	s, err := catalog.Load(skillName)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Result{}, false")
		return Result{}, false
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Invoke the %q skill", s.Name)
	if strings.TrimSpace(args) != "" {
		observe.GlobalTrace("if: strings.TrimSpace(args) != \"\"")
		fmt.Fprintf(&b, " with arguments: %s", strings.TrimSpace(args))
	}
	b.WriteString(".")
	observe.GlobalTrace("return: Result{InjectPrompt: b.String()}, true")
	return Result{InjectPrompt: b.String()}, true
}

// Parse checks if input is a slash command.
// Returns (command name, args, true) for valid commands.
// Returns ("", "", false) if input is not a slash command.
func Parse(input string) (name string, args string, ok bool) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		observe.GlobalTrace("if: !strings.HasPrefix(trimmed, \"/\")")
		observe.GlobalTrace("return: \"\", \"\", false")
		return "", "", false
	}

	withoutSlash := trimmed[1:]
	if withoutSlash == "" {
		observe.GlobalTrace("if: withoutSlash == \"\"")
		observe.GlobalTrace("return: \"\", \"\", false")
		return "", "", false
	}

	parts := strings.SplitN(withoutSlash, " ", 2)
	name = parts[0]
	if len(parts) > 1 {
		observe.GlobalTrace("if: len(parts) > 1")
		args = parts[1]
	}
	observe.GlobalTrace("return: name, args, true")

	return name, args, true
}
