package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/config"
	"github.com/artpar/pragma/internal/cron"
	"github.com/artpar/pragma/internal/observe"
)

// RunCronDaemon starts the explicit runtime owner for scheduled prompts.
func RunCronDaemon(cmd *cobra.Command, _ []string) error {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")

	pragmaHome, err := config.PragmaHome()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"resolve pragma home: %w\", err)")
		return fmt.Errorf("resolve pragma home: %w", err)
	}
	bus := observe.NewEventBus(1024)
	defer bus.Drain()

	store := cron.NewStore(filepath.Join(pragmaHome, "scheduled_tasks.json"))
	scheduler := cron.NewScheduler(bus, store)

	cwd, err := os.Getwd()
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: fmt.Errorf(\"get cwd: %w\", err)")
		return fmt.Errorf("get cwd: %w", err)
	}

	fmt.Fprintln(os.Stderr, "Cron scheduler running. Press Ctrl+C to stop.")
	scheduler.Start(cmd.Context(), func(job *cron.Job) error {
		if err := launchCronPrompt(cmd.Context(), cwd, job); err != nil {
			fmt.Fprintf(os.Stderr, "cron job %s failed to launch: %v\n", job.ID, err)
			return err
		}
		return nil
	})
	observe.GlobalTrace("return: nil")
	return nil
}

func launchCronPrompt(ctx context.Context, cwd string, job *cron.Job) error {
	observe.TraceCtx(ctx, "cli", "launchCronPrompt", "enter")
	defer observe.TraceCtx(ctx, "cli", "launchCronPrompt", "exit")
	if job == nil {
		observe.TraceCtx(ctx, "cli", "launchCronPrompt", "if: job == nil")
		observe.TraceCtx(ctx, "cli", "launchCronPrompt", "return: fmt.Errorf(\"nil cron job\")")
		return fmt.Errorf("nil cron job")
	}
	exe, err := os.Executable()
	if err != nil {
		observe.TraceCtx(ctx, "cli", "launchCronPrompt", "if: err != nil")
		observe.TraceCtx(ctx, "cli", "launchCronPrompt", "return: fmt.Errorf(\"resolve executable: %w\", err)")
		return fmt.Errorf("resolve executable: %w", err)
	}
	run := exec.CommandContext(ctx, exe, "--prompt", job.Prompt, "--bg")
	run.Dir = cwd
	run.Env = os.Environ()
	out, err := run.CombinedOutput()
	if err != nil {
		observe.TraceCtx(ctx, "cli", "launchCronPrompt", "if: err != nil")
		observe.TraceCtx(ctx, "cli", "launchCronPrompt", "return: fmt.Errorf(\"start background prompt: %s: %w\", string(out), err)")
		return fmt.Errorf("start background prompt: %s: %w", string(out), err)
	}
	if len(out) > 0 {
		observe.TraceCtx(ctx, "cli", "launchCronPrompt", "if: len(out) > 0")
		_, _ = os.Stderr.Write(out)
	}
	observe.TraceCtx(ctx, "cli", "launchCronPrompt", "return: nil")
	return nil
}
