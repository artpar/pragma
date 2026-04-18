package slash

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/pragma/internal/app"
	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

// registerBuiltins registers all built-in slash commands.
func registerBuiltins(r *Registry) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	r.Register(Command{
		Name:        "compact",
		Description: "Clear conversation history but keep a summary in context",
		Handle:      handleCompact,
	})
	r.Register(Command{
		Name:        "clear",
		Aliases:     []string{"reset", "new"},
		Description: "Clear conversation history and start fresh",
		Handle:      handleClear,
	})
	r.Register(Command{
		Name:        "help",
		Aliases:     []string{"?"},
		Description: "Show available commands",
		Handle:      handleHelp,
	})
	r.Register(Command{
		Name:        "exit",
		Aliases:     []string{"quit"},
		Description: "Save session and exit",
		Handle:      handleExit,
	})
	r.Register(Command{
		Name:        "insights",
		Description: "Show current session statistics",
		Handle:      handleInsights,
	})

	r.Register(Command{
		Name:        "cost",
		Description: "Show session cost and token usage",
		Handle:      handleCost,
		Type:        TypeLocal,
		CLIUse:      "cost",
	})
	r.Register(Command{
		Name:        "model",
		Description: "Show or switch the active model",
		Handle:      handleModel,
		Type:        TypeLocal,
		CLIUse:      "model [name]",
	})
	r.Register(Command{
		Name:        "advisor",
		Description: "Show or set the advisor model",
		Handle:      handleAdvisor,
		Type:        TypeLocal,
		CLIUse:      "advisor [model]",
	})
	r.Register(Command{
		Name:        "doctor",
		Description: "Check environment and configuration health",
		Handle:      handleDoctor,
		Type:        TypeLocal,
		CLIUse:      "doctor",
	})

	r.Register(Command{
		Name:        "review",
		Description: "Review a pull request",
		Handle:      handleReview,
		Type:        TypePrompt,
		CLIUse:      "review [pr-number]",
	})
	r.Register(Command{
		Name:        "security-review",
		Aliases:     []string{"secreview"},
		Description: "Security review of pending branch changes",
		Handle:      handleSecurityReview,
		Type:        TypePrompt,
		CLIUse:      "security-review",
	})
	r.Register(Command{
		Name:        "commit",
		Description: "Create a git commit",
		Handle:      handleCommit,
		Type:        TypePrompt,
		CLIUse:      "commit",
		AllowedTools: []string{
			"Bash(git add:*)",
			"Bash(git status:*)",
			"Bash(git commit:*)",
		},
	})
	r.Register(Command{
		Name:        "init",
		Description: "Initialize AGENT.md with codebase documentation",
		Handle:      handleInit,
		Type:        TypePrompt,
		CLIUse:      "init",
	})
	r.Register(Command{
		Name:        "teams",
		Aliases:     []string{"teammates"},
		Description: "Manage active teammates",
		Handle:      handleTeams,
		Type:        TypeLocal,
	})
}

func handleCompact(ctx context.Context, args string, deps Deps) (Result, error) {
	observe.TraceCtx(ctx, "slash", "handleCompact", "enter")
	defer observe.TraceCtx(ctx, "slash", "handleCompact", "exit")
	if deps.Compactor == nil {
		observe.TraceCtx(ctx, "slash", "handleCompact", "if: deps.Compactor == nil")
		observe.TraceCtx(ctx, "slash", "handleCompact", "return: Result{DisplayText: \"Compaction is not available.\"}, nil")
		return Result{DisplayText: "Compaction is not available."}, nil
	}

	snap := deps.Store.Snapshot()
	msgs := snap.Conversation.APIMessages()

	result, err := deps.Compactor.Compact(ctx, msgs, snap.Conversation.System, strings.TrimSpace(args))
	if err != nil {
		observe.TraceCtx(ctx, "slash", "handleCompact", "if: err != nil")
		observe.TraceCtx(ctx, "slash", "handleCompact", "return: Result{}, fmt.Errorf(\"compaction failed: %w\", err)")
		return Result{}, fmt.Errorf("compaction failed: %w", err)
	}

	deps.Store.Update(func(s *app.AppState) {
		s.Conversation.Messages = result.ReplacementMessages
		s.Conversation.UpdatedAt = time.Now()
	})
	observe.TraceCtx(ctx, "slash", "handleCompact", "return: Result{\n\tDisplayText: fmt.Sprintf(\"Compacted: %d → %d tokens (%d messages r...")

	return Result{
		DisplayText: fmt.Sprintf("Compacted: %d → %d tokens (%d messages removed)",
			result.PreTokenCount, result.PostTokenCount, result.MessagesRemoved),
	}, nil
}

func handleClear(_ context.Context, _ string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	deps.Store.Update(func(s *app.AppState) {
		s.Conversation.Messages = nil
		s.Conversation.ID = model.NewUUID()
		s.Conversation.UpdatedAt = time.Now()
	})
	observe.GlobalTrace("return: Result{\n\tClearConversation:\ttrue,\n\tDisplayText:\t\t\"Conversation cleared.\",\n}, nil")

	return Result{
		ClearConversation: true,
		DisplayText:       "Conversation cleared.",
	}, nil
}

func handleCost(_ context.Context, _ string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	entries := deps.CostTracker.Snapshot()
	total := deps.CostTracker.TotalUSD()

	if len(entries) == 0 {
		observe.GlobalTrace("if: len(entries) == 0")
		observe.GlobalTrace("return: Result{DisplayText: \"No API calls yet. Session cost: $0.0000\"}, nil")
		return Result{DisplayText: "No API calls yet. Session cost: $0.0000"}, nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Session cost: $%.4f\n", total)
	fmt.Fprintf(&b, "Model: %s (provider: %s)\n\n", deps.ModelName, deps.Provider)

	// Aggregate by model
	type modelStats struct {
		input, output, cacheCreate, cacheRead int
		cost                                  float64
		calls                                 int
	}
	byModel := make(map[string]*modelStats)
	for _, e := range entries {
		observe.GlobalTrace("range entries")
		s, ok := byModel[e.Model]
		if !ok {
			observe.GlobalTrace("if: !ok")
			s = &modelStats{}
			byModel[e.Model] = s
		}
		s.input += e.Usage.InputTokens
		s.output += e.Usage.OutputTokens
		s.cacheCreate += e.Usage.CacheCreationInputTokens
		s.cacheRead += e.Usage.CacheReadInputTokens
		s.cost += e.CostUSD
		s.calls++
	}

	for m, s := range byModel {
		observe.GlobalTrace("range byModel")
		fmt.Fprintf(&b, "  %s: %d calls, $%.4f\n", m, s.calls, s.cost)
		fmt.Fprintf(&b, "    input: %d tokens, output: %d tokens\n", s.input, s.output)
		if s.cacheCreate > 0 || s.cacheRead > 0 {
			observe.GlobalTrace("if: s.cacheCreate > 0 || s.cacheRead > 0")
			fmt.Fprintf(&b, "    cache: %d created, %d read\n", s.cacheCreate, s.cacheRead)
		}
	}
	observe.GlobalTrace("return: Result{DisplayText: b.String()}, nil")

	return Result{DisplayText: b.String()}, nil
}

func handleHelp(_ context.Context, _ string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, cmd := range deps.Commands {
		observe.GlobalTrace("range deps.Commands")
		line := "  /" + cmd.Name
		if len(cmd.Aliases) > 0 {
			observe.GlobalTrace("if: len(cmd.Aliases) > 0")
			line += " (" + strings.Join(cmd.Aliases, ", ") + ")"
		}
		line += "  — " + cmd.Description
		b.WriteString(line)
		b.WriteString("\n")
	}
	observe.GlobalTrace("return: Result{DisplayText: strings.TrimSpace(b.String())}, nil")
	return Result{DisplayText: strings.TrimSpace(b.String())}, nil
}

func handleModel(_ context.Context, args string, deps Deps) (Result, error) {
	args = strings.TrimSpace(args)

	if args == "" {
		// Show current model + list available
		snap := deps.Store.Snapshot()
		modelName := snap.Model
		if modelName == "" {
			modelName = deps.ModelName
		}
		var b strings.Builder
		fmt.Fprintf(&b, "Current model: %s (provider: %s)", modelName, deps.Provider)
		if deps.ModelLister != nil {
			if models := deps.ModelLister(); len(models) > 0 {
				b.WriteString("\n\nAvailable models:")
				for _, m := range models {
					marker := "  "
					if m == modelName {
						marker = "* "
					}
					b.WriteString("\n  " + marker + m)
				}
			}
		}
		return Result{DisplayText: b.String()}, nil
	}

	// Validate model against provider's known models
	if deps.ContextWindowFunc != nil {
		if _, ok := deps.ContextWindowFunc(args); !ok {
			var b strings.Builder
			fmt.Fprintf(&b, "Unknown model: %s", args)
			if deps.ModelLister != nil {
				if models := deps.ModelLister(); len(models) > 0 {
					b.WriteString("\n\nAvailable models:")
					for _, m := range models {
						b.WriteString("\n  " + m)
					}
				}
			}
			return Result{DisplayText: b.String()}, nil
		}
	}

	deps.Store.Update(func(s *app.AppState) {
		s.Model = args
	})
	if deps.OnModelChanged != nil {
		deps.OnModelChanged(args)
	}

	msg := fmt.Sprintf("Model switched to: %s (takes effect on next turn)", args)
	if deps.ContextWindowFunc != nil {
		if cw, ok := deps.ContextWindowFunc(args); ok {
			msg = fmt.Sprintf("Model switched to: %s (context: %dk, takes effect on next turn)", args, cw/1000)
		}
	}
	return Result{DisplayText: msg}, nil
}

func handleExit(_ context.Context, _ string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if deps.SessionSave != nil {
		observe.GlobalTrace("if: deps.SessionSave != nil")
		deps.SessionSave()
	}
	observe.GlobalTrace("return: Result{Quit: true}, nil")
	return Result{Quit: true}, nil
}
