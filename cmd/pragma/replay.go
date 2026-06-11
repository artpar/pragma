package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/model"
	"github.com/artpar/pragma/internal/observe"
)

func replayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay <session-dir>",
		Short: "Replay a recorded session",
		Long: `Replay a previously recorded session from an events directory.

Modes:
  --events          Display event stream (read-only, no execution)
  --deterministic   Render recorded API responses (read-only, no tool execution)
  --until-turn=N    Replay N recorded turns`,
		Args: cobra.ExactArgs(1),
		RunE: replayRun,
	}
	cmd.Flags().Bool("events", false, "display event stream (read-only)")
	cmd.Flags().Bool("deterministic", false, "render recorded API responses without executing tools")
	cmd.Flags().Int("until-turn", 0, "replay up to N turns")
	cmd.AddCommand(replayExportCmd())
	cmd.AddCommand(replayRawHTTPCmd())
	return cmd
}

func replayRun(cmd *cobra.Command, args []string) error {
	dir := args[0]
	eventsMode, _ := cmd.Flags().GetBool("events")
	deterministicMode, _ := cmd.Flags().GetBool("deterministic")
	untilTurn, _ := cmd.Flags().GetInt("until-turn")

	engine, err := observe.LoadReplay(dir)
	if err != nil {
		return fmt.Errorf("load replay from %q: %w", dir, err)
	}

	if eventsMode {
		return replayEvents(engine)
	}

	if deterministicMode || untilTurn > 0 {
		return replayDeterministic(engine, untilTurn)
	}

	// Default: events mode
	return replayEvents(engine)
}

// replayEvents prints the event stream in a human-readable format.
func replayEvents(engine *observe.ReplayEngine) error {
	events := engine.Events()
	if len(events) == 0 {
		fmt.Println("No events in replay.")
		return nil
	}

	var prevTime time.Time
	for _, ev := range events {
		ts := ev.EventTimestamp()
		delta := ""
		if !prevTime.IsZero() {
			d := ts.Sub(prevTime)
			if d > time.Second {
				delta = fmt.Sprintf(" (+%s)", d.Truncate(time.Millisecond))
			}
		}
		prevTime = ts

		detail := formatEventDetail(ev)
		fmt.Printf("%s  %-30s  %s%s\n",
			ts.Format("15:04:05.000"),
			ev.EventKind(),
			detail,
			delta,
		)
	}

	fmt.Printf("\n%d events total\n", len(events))
	return nil
}

// formatEventDetail extracts key fields from an event for display.
func formatEventDetail(ev observe.Event) string {
	switch e := ev.(type) {
	case observe.APIRequestStarted:
		return fmt.Sprintf("model=%s messages=%d tools=%d", e.Model, e.MessageCount, e.ToolCount)
	case observe.APIRequestCompleted:
		return fmt.Sprintf("stop=%s input=%d output=%d %dms",
			e.StopReason, e.Usage.InputTokens, e.Usage.OutputTokens, e.DurationMs)
	case observe.APIRequestFailed:
		return fmt.Sprintf("error=%s retryable=%v", e.ErrorType, e.Retryable)
	case observe.ToolCallReceived:
		id := e.ToolCallID
		if len(id) > 8 {
			id = id[:8]
		}
		return fmt.Sprintf("tool=%s id=%s", e.ToolName, id)
	case observe.ToolExecutionCompleted:
		return fmt.Sprintf("tool=%s %dms error=%v", e.ToolName, e.DurationMs, e.IsError)
	case observe.ToolPermissionChecked:
		return fmt.Sprintf("tool=%s decision=%s", e.ToolName, e.Decision)
	case observe.CompactionStarted:
		return fmt.Sprintf("pre=%d budget=%d", e.PreTokenCount, e.BudgetTokens)
	case observe.CompactionCompleted:
		return fmt.Sprintf("post=%d summarized=%d %dms", e.PostTokenCount, e.SummarizedCount, e.DurationMs)
	case observe.MCPServerConnected:
		return fmt.Sprintf("server=%s tools=%d", e.ServerName, e.ToolCount)
	case observe.MCPHealthCheck:
		return fmt.Sprintf("server=%s status=%s", e.ServerName, e.Status)
	case observe.ErrorOccurred:
		return fmt.Sprintf("[%s] %s: %s", e.Severity, e.Component, e.ErrorMessage)
	case observe.SubAgentSpawned:
		return fmt.Sprintf("agent=%s model=%s", e.AgentName, e.Model)
	case observe.SubAgentCompleted:
		return fmt.Sprintf("turns=%d %dms", e.TurnCount, e.DurationMs)
	case observe.HookExecuted:
		return fmt.Sprintf("event=%s exit=%d outcome=%s", e.HookEvent, e.ExitCode, e.Outcome)
	case observe.FlowTrace:
		return fmt.Sprintf("%s.%s: %s", e.Component, e.Function, e.Message)
	default:
		return ""
	}
}

// replayDeterministic renders recorded responses without running the live engine.
func replayDeterministic(engine *observe.ReplayEngine, untilTurn int) error {
	return replayRecordedResponses(engine, untilTurn)
}

func replayRecordedResponses(engine *observe.ReplayEngine, untilTurn int) error {
	printed := 0
	for turn := 1; ; turn++ {
		if untilTurn > 0 && turn > untilTurn {
			break
		}
		resp, ok := engine.APIResponse(turn)
		if !ok {
			break
		}
		printed++
		if untilTurn != 1 {
			fmt.Fprintf(os.Stderr, "\n[replay turn %d]\n", turn)
		}
		if err := printReplayResponse(resp); err != nil {
			return err
		}
	}
	if printed == 0 {
		return fmt.Errorf("recording has no recorded API responses")
	}
	return nil
}

func printReplayResponse(resp model.Response) error {
	for _, part := range resp.Content {
		switch p := part.(type) {
		case model.TextPart:
			fmt.Print(p.Text)
		case model.ThinkingPart:
			if p.Text != "" {
				fmt.Fprint(os.Stderr, p.Text)
			}
		case model.ToolCallPart:
			input := strings.TrimSpace(string(p.Input))
			if input == "" {
				input = "{}"
			}
			fmt.Fprintf(os.Stderr, "\n[tool: %s %s]\n", p.Name, input)
		}
	}
	if len(resp.Content) > 0 {
		fmt.Println()
	}
	return nil
}
