package main

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/cli"
	"github.com/artpar/pragma/internal/observe"
	"github.com/artpar/pragma/internal/permission"
	replayprov "github.com/artpar/pragma/internal/provider/replay"
	"github.com/artpar/pragma/internal/tui"
)

func replayCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "replay <session-dir>",
		Short: "Replay a recorded session",
		Long: `Replay a previously recorded session from an events directory.

Modes:
  --events          Display event stream (read-only, no execution)
  --deterministic   Replay with recorded API responses
  --until-turn=N    Replay N turns, then switch to live provider (requires --then-live)
  --then-live       Switch to real provider after --until-turn`,
		Args: cobra.ExactArgs(1),
		RunE: replayRun,
	}
	cmd.Flags().Bool("events", false, "display event stream (read-only)")
	cmd.Flags().Bool("deterministic", false, "replay with recorded API responses")
	cmd.Flags().Int("until-turn", 0, "replay up to N turns")
	cmd.Flags().Bool("then-live", false, "switch to live provider after --until-turn")
	return cmd
}

func replayRun(cmd *cobra.Command, args []string) error {
	dir := args[0]
	eventsMode, _ := cmd.Flags().GetBool("events")
	deterministicMode, _ := cmd.Flags().GetBool("deterministic")
	untilTurn, _ := cmd.Flags().GetInt("until-turn")
	thenLive, _ := cmd.Flags().GetBool("then-live")

	if thenLive && untilTurn == 0 {
		return fmt.Errorf("--then-live requires --until-turn=N")
	}

	engine, err := observe.LoadReplay(dir)
	if err != nil {
		return fmt.Errorf("load replay from %q: %w", dir, err)
	}

	if eventsMode {
		return replayEvents(engine)
	}

	if deterministicMode || untilTurn > 0 {
		return replayDeterministic(cmd, engine, untilTurn, thenLive)
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
	case observe.SessionStarted:
		return fmt.Sprintf("session=%s", e.SessionID)
	case observe.SessionEnded:
		return fmt.Sprintf("turns=%d cost=$%.4f", e.TurnCount, e.TotalCostUSD)
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

// replayDeterministic runs the engine with recorded API responses.
func replayDeterministic(cmd *cobra.Command, engine *observe.ReplayEngine, untilTurn int, thenLive bool) error {
	d, err := cli.SetupDeps(cmd)
	if err != nil {
		return err
	}
	if d.Cleanup != nil {
		defer d.Cleanup()
	}

	// Build replay provider, optionally with fallback to real provider
	var rp *replayprov.Provider
	if thenLive && untilTurn > 0 {
		rp = replayprov.New(engine, d.Prov, untilTurn)
	} else {
		rp = replayprov.New(engine, nil, 0)
	}

	// Swap provider to replay — all downstream consumers use this
	d.Prov = rp

	// Build engine using standard tool registration path
	prompter := &permission.NonInteractivePrompter{}
	asker := &tui.NonInteractiveAsker{}
	queryEngine, err := cli.RegisterTools(d, prompter, asker)
	if err != nil {
		return fmt.Errorf("register tools for replay: %w", err)
	}

	firstPrompt := extractFirstUserPrompt(engine)
	if firstPrompt == "" {
		return fmt.Errorf("no user message found in recorded events")
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	events := queryEngine.Run(cmd.Context(), firstPrompt)
	return cli.ConsumeEngineEvents(events, verbose)
}

// extractFirstUserPrompt scans recorded events for the first user message.
func extractFirstUserPrompt(engine *observe.ReplayEngine) string {
	for _, ev := range engine.Events() {
		if ma, ok := ev.(observe.MessageAppended); ok && ma.Role == "user" {
			// The actual text isn't in the event — return a generic prompt
			// that will be paired with the recorded API response
			return "Replay: continue from recorded session"
		}
	}
	return ""
}
