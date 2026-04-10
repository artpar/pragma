package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/spf13/cobra"

	"github.com/artpar/gogent/internal/cli"
)

func main() {
	root := &cobra.Command{
		Use:           "gogent",
		Short:         "AI coding assistant",
		Long:          "gogent is a CLI AI coding assistant powered by LLMs.",
		RunE:          cli.RunDispatcher,
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	cli.RegisterFlags(root)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
