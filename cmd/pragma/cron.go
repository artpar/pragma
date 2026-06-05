package main

import (
	"github.com/spf13/cobra"

	"github.com/artpar/pragma/internal/cli"
)

func cronCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cron",
		Short: "Run scheduled cron jobs",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run the cron scheduler until interrupted",
		RunE:  cli.RunCronDaemon,
	})
	return cmd
}
