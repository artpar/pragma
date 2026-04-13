package slash

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/artpar/gogent/internal/model"
	"github.com/artpar/gogent/internal/observe"
)

func handleInsights(_ context.Context, _ string, deps Deps) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	snap := deps.Store.Snapshot()
	msgs := snap.Conversation.Messages

	var userCount, assistantCount int
	toolCalls := make(map[string]int)

	for _, msg := range msgs {
		observe.GlobalTrace("range msgs")
		switch msg.Role {
		case model.RoleUser:
			userCount++
		case model.RoleAssistant:
			assistantCount++
		}
		for _, part := range msg.Content {
			if tc, ok := part.(model.ToolCallPart); ok {
				toolCalls[tc.Name]++
			}
		}
	}

	totalMsgs := userCount + assistantCount

	// Cost and token aggregation
	entries := deps.CostTracker.Snapshot()
	totalCost := deps.CostTracker.TotalUSD()
	var totalInput, totalOutput int
	type modelAgg struct {
		calls  int
		cost   float64
		input  int
		output int
	}
	byModel := make(map[string]*modelAgg)
	for _, e := range entries {
		observe.GlobalTrace("range entries")
		totalInput += e.Usage.InputTokens
		totalOutput += e.Usage.OutputTokens
		agg, ok := byModel[e.Model]
		if !ok {
			observe.GlobalTrace("if: !ok")
			agg = &modelAgg{}
			byModel[e.Model] = agg
		}
		agg.calls++
		agg.cost += e.CostUSD
		agg.input += e.Usage.InputTokens
		agg.output += e.Usage.OutputTokens
	}

	// Duration
	duration := time.Since(snap.Conversation.CreatedAt)
	durationStr := formatDuration(duration)

	var b strings.Builder
	b.WriteString("Session Insights\n")
	b.WriteString("─────────────────\n")
	fmt.Fprintf(&b, "Duration:  %s\n", durationStr)
	fmt.Fprintf(&b, "Messages:  %d (%d user, %d assistant)\n", totalMsgs, userCount, assistantCount)
	fmt.Fprintf(&b, "Tokens:    %s input, %s output\n", formatCount(totalInput), formatCount(totalOutput))
	fmt.Fprintf(&b, "Cost:      $%.4f\n", totalCost)

	if len(toolCalls) > 0 {
		observe.GlobalTrace("if: len(toolCalls) > 0")
		b.WriteString("\nTool Usage:\n")
		sorted := sortedToolCalls(toolCalls)
		for _, tc := range sorted {
			observe.GlobalTrace("range sorted")
			fmt.Fprintf(&b, "  %-14s %d calls\n", tc.name, tc.count)
		}
	}

	if len(byModel) > 0 {
		observe.GlobalTrace("if: len(byModel) > 0")
		b.WriteString("\nModels Used:\n")
		for name, agg := range byModel {
			observe.GlobalTrace("range byModel")
			fmt.Fprintf(&b, "  %-20s %d calls  $%.4f\n", name, agg.calls, agg.cost)
		}
	}

	observe.GlobalTrace("return: Result{DisplayText: b.String()}, nil")
	return Result{DisplayText: strings.TrimSpace(b.String())}, nil
}

type toolCallEntry struct {
	name  string
	count int
}

func sortedToolCalls(m map[string]int) []toolCallEntry {
	entries := make([]toolCallEntry, 0, len(m))
	for k, v := range m {
		entries = append(entries, toolCallEntry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count > entries[j].count
	})
	return entries
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	return fmt.Sprintf("%dh %dm", h, m)
}

func formatCount(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%d,%03d", n/1000, n%1000)
}
