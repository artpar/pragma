package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/observe"
)

func auditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "audit <events-path>",
		Short: "Show permission audit trail from recorded events",
		Long: `Display the permission audit trail from a recorded session.

The path can be:
  - A .jsonl file (e.g., gogent-recording.jsonl)
  - A replay directory containing events.jsonl`,
		Args: cobra.ExactArgs(1),
		RunE: auditRun,
	}
}

func auditRun(_ *cobra.Command, args []string) error {
	events, err := observe.LoadEvents(args[0])
	if err != nil {
		return fmt.Errorf("load events from %q: %w", args[0], err)
	}

	auditor := observe.NewAuditor()
	for _, ev := range events {
		auditor.HandleEvent(ev)
	}

	trail := auditor.Trail()
	if len(trail) == 0 {
		fmt.Println("No permission events found.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "TIMESTAMP\tTOOL\tDECISION\tRESPONSE\tRULE\tSOURCE")
	for _, e := range trail {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			e.Timestamp.Format("15:04:05.000"),
			e.ToolName,
			e.Decision,
			e.UserResponse,
			e.RuleMatched,
			e.RuleSource,
		)
	}
	w.Flush()

	violations := auditor.Violations()
	if len(violations) > 0 {
		fmt.Fprintf(os.Stdout, "\nVIOLATIONS: %d denial(s) were ignored (tool executed despite deny)\n", len(violations))
		for _, v := range violations {
			fmt.Fprintf(os.Stdout, "  %s  %s  (call %s)\n",
				v.Timestamp.Format("15:04:05.000"), v.ToolName, v.ToolCallID)
		}
		return fmt.Errorf("%d permission violation(s) detected", len(violations))
	}

	fmt.Printf("\n%d permission checks, 0 violations\n", len(trail))
	return nil
}
