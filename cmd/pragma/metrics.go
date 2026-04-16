package main

import (
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/observe"
)

func metricsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "metrics <events-path>",
		Short: "Show session metrics from recorded events",
		Long: `Display aggregated metrics from a recorded session.

The path can be:
  - A .jsonl file (e.g., pragma-recording.jsonl)
  - A replay directory containing events.jsonl`,
		Args: cobra.ExactArgs(1),
		RunE: metricsRun,
	}
}

func metricsRun(_ *cobra.Command, args []string) error {
	events, err := observe.LoadEvents(args[0])
	if err != nil {
		return fmt.Errorf("load events from %q: %w", args[0], err)
	}

	if len(events) == 0 {
		fmt.Println("No events found.")
		return nil
	}

	m := observe.NewMetrics()
	for _, ev := range events {
		m.HandleEvent(ev)
	}

	snap := m.Snapshot()

	fmt.Println("Session Metrics")
	fmt.Println("===============")

	if snap.SessionDurationMs > 0 {
		d := time.Duration(snap.SessionDurationMs) * time.Millisecond
		fmt.Printf("Duration:      %s\n", d.Truncate(time.Second))
	}
	fmt.Printf("Turns:         %d\n", snap.TurnCount)
	if snap.APIErrorCount > 0 {
		fmt.Printf("API Calls:     %d (%d errors)\n", snap.APICallCount, snap.APIErrorCount)
	} else {
		fmt.Printf("API Calls:     %d\n", snap.APICallCount)
	}
	if snap.APICallCount > 0 {
		fmt.Printf("Avg Latency:   %dms\n", snap.AvgAPILatencyMs)
	}
	fmt.Printf("Compactions:   %d\n", snap.Compactions)

	fmt.Println()
	fmt.Println("Tokens")
	fmt.Println("------")
	fmt.Printf("Input:         %d\n", snap.TokenUsage.InputTokens)
	fmt.Printf("Output:        %d\n", snap.TokenUsage.OutputTokens)
	if snap.TokenUsage.CacheCreationInputTokens > 0 {
		fmt.Printf("Cache Create:  %d\n", snap.TokenUsage.CacheCreationInputTokens)
	}
	if snap.TokenUsage.CacheReadInputTokens > 0 {
		fmt.Printf("Cache Read:    %d\n", snap.TokenUsage.CacheReadInputTokens)
	}

	if len(snap.ToolStats) > 0 {
		fmt.Println()
		fmt.Println("Tool Breakdown")
		fmt.Println("--------------")

		// Sort tools by call count descending
		names := make([]string, 0, len(snap.ToolStats))
		for name := range snap.ToolStats {
			names = append(names, name)
		}
		sort.Slice(names, func(i, j int) bool {
			return snap.ToolStats[names[i]].Calls > snap.ToolStats[names[j]].Calls
		})

		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		fmt.Fprintln(w, "TOOL\tCALLS\tERRORS\tAVG DURATION")
		for _, name := range names {
			ts := snap.ToolStats[name]
			fmt.Fprintf(w, "%s\t%d\t%d\t%dms\n", name, ts.Calls, ts.Errors, ts.AvgDurMs)
		}
		w.Flush()
	}

	fmt.Printf("\n%d events processed\n", len(events))
	return nil
}
