package slash

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/app"
	"github.com/artpar/gogent/internal/model"
)

// registerBuiltins registers all built-in slash commands.
func registerBuiltins(r *Registry) {
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
		Name:        "cost",
		Description: "Show session cost and token usage",
		Handle:      handleCost,
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
}

func handleCompact(ctx context.Context, args string, deps Deps) (Result, error) {
	if deps.Compactor == nil {
		return Result{DisplayText: "Compaction is not available."}, nil
	}

	snap := deps.Store.Snapshot()
	msgs := snap.Conversation.APIMessages()

	result, err := deps.Compactor.Compact(ctx, msgs, snap.Conversation.System, strings.TrimSpace(args))
	if err != nil {
		return Result{}, fmt.Errorf("compaction failed: %w", err)
	}

	// Apply result atomically (#40316)
	deps.Store.Update(func(s *app.AppState) {
		s.Conversation.Messages = result.ReplacementMessages
		s.Conversation.UpdatedAt = time.Now()
	})

	return Result{
		DisplayText: fmt.Sprintf("Compacted: %d → %d tokens (%d messages removed)",
			result.PreTokenCount, result.PostTokenCount, result.MessagesRemoved),
	}, nil
}

func handleClear(_ context.Context, _ string, deps Deps) (Result, error) {
	// Regenerate conversation ID for full isolation (matches TS behavior)
	deps.Store.Update(func(s *app.AppState) {
		s.Conversation.Messages = nil
		s.Conversation.ID = model.NewUUID()
		s.Conversation.UpdatedAt = time.Now()
	})

	return Result{
		ClearConversation: true,
		DisplayText:       "Conversation cleared.",
	}, nil
}

func handleCost(_ context.Context, _ string, deps Deps) (Result, error) {
	entries := deps.CostTracker.Snapshot()
	total := deps.CostTracker.TotalUSD()

	if len(entries) == 0 {
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
		s, ok := byModel[e.Model]
		if !ok {
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
		fmt.Fprintf(&b, "  %s: %d calls, $%.4f\n", m, s.calls, s.cost)
		fmt.Fprintf(&b, "    input: %d tokens, output: %d tokens\n", s.input, s.output)
		if s.cacheCreate > 0 || s.cacheRead > 0 {
			fmt.Fprintf(&b, "    cache: %d created, %d read\n", s.cacheCreate, s.cacheRead)
		}
	}

	return Result{DisplayText: b.String()}, nil
}

func handleHelp(_ context.Context, _ string, deps Deps) (Result, error) {
	var b strings.Builder
	b.WriteString("Available commands:\n")
	for _, cmd := range deps.Commands {
		line := "  /" + cmd.Name
		if len(cmd.Aliases) > 0 {
			line += " (" + strings.Join(cmd.Aliases, ", ") + ")"
		}
		line += "  — " + cmd.Description
		b.WriteString(line)
		b.WriteString("\n")
	}
	return Result{DisplayText: strings.TrimSpace(b.String())}, nil
}

func handleExit(_ context.Context, _ string, deps Deps) (Result, error) {
	if deps.SessionSave != nil {
		deps.SessionSave()
	}
	return Result{Quit: true}, nil
}
