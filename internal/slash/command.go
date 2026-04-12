package slash

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/compact"
	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

// ErrUnknownCommand is returned when no command matches the given name.
var ErrUnknownCommand = errors.New("unknown command")

// Result is the output of a slash command execution.
type Result struct {
	DisplayText       string // text to show in the TUI viewport
	ClearConversation bool   // true for /clear — TUI should reset display
	Quit              bool   // true for /exit — TUI should exit
}

// Handler is the function signature for a slash command handler.
type Handler func(ctx context.Context, args string, deps Deps) (Result, error)

// Deps bundles the dependencies available to slash command handlers.
type Deps struct {
	Store       *app.StateStore
	CostTracker *model.CostTracker
	Compactor   *compact.Service
	Bus         *observe.EventBus
	SessionSave func()
	ModelName   string
	Provider    string
	// Commands is set internally by Registry.Execute — not for external callers.
	Commands []Command
}

// Command describes a registered slash command.
type Command struct {
	Name        string
	Aliases     []string
	Description string
	Handle      Handler
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
		observe.TraceCtx(ctx, "slash", "Registry.Execute", "return: Result{}, fmt.Errorf(\"%w: /%s\", ErrUnknownCommand, name)")
		observe.TraceCtx(ctx, "slash", "Registry.Execute", "return: Result{}, fmt.Errorf(\"%w: /%s\", ErrUnknownCommand, name)")
		return Result{}, fmt.Errorf("%w: /%s", ErrUnknownCommand, name)
	}
	deps.Commands = r.Commands()
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
	observe.GlobalTrace("return: sorted")
	return sorted
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
		observe.GlobalTrace("return: \"\", \"\", false")
		return "", "", false
	}

	withoutSlash := trimmed[1:]
	if withoutSlash == "" {
		observe.GlobalTrace("if: withoutSlash == \"\"")
		observe.GlobalTrace("return: \"\", \"\", false")
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
	observe.GlobalTrace("return: name, args, true")

	return name, args, true
}
