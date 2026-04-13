package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/artpar/gogent/internal/background"
)

func sessionsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "sessions",
		Short:   "Manage background sessions",
		Aliases: []string{"ps"},
		RunE:    sessionsListRun, // default: list
	}
	cmd.AddCommand(sessionsListCommand())
	cmd.AddCommand(sessionsKillCommand())
	cmd.AddCommand(sessionsLogsCommand())
	return cmd
}

func sessionsListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List active background sessions",
		RunE:  sessionsListRun,
	}
}

func sessionsListRun(_ *cobra.Command, _ []string) error {
	reg, err := background.NewRegistry()
	if err != nil {
		return err
	}
	sessions, err := reg.List()
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Println("No active background sessions.")
		return nil
	}

	fmt.Printf("%-8s  %-10s  %-12s  %-10s  %s\n", "PID", "STATUS", "MODEL", "DURATION", "PROMPT")
	for _, s := range sessions {
		duration := time.Since(s.StartedAt).Truncate(time.Second)
		prompt := s.Prompt
		if len(prompt) > 60 {
			prompt = prompt[:60] + "..."
		}
		model := s.Model
		if len(model) > 12 {
			model = model[:12]
		}
		fmt.Printf("%-8d  %-10s  %-12s  %-10s  %s\n",
			s.PID, s.Status, model, duration, prompt)
	}
	return nil
}

func sessionsKillCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "kill <pid>",
		Short: "Kill a background session",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			pid, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid PID %q: %w", args[0], err)
			}
			reg, err := background.NewRegistry()
			if err != nil {
				return err
			}
			if err := reg.Kill(pid); err != nil {
				return err
			}
			fmt.Printf("Killed background session (PID %d)\n", pid)
			return nil
		},
	}
}

func sessionsLogsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs <pid>",
		Short: "Show logs for a background session",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			pid, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("invalid PID %q: %w", args[0], err)
			}
			reg, err := background.NewRegistry()
			if err != nil {
				return err
			}
			info, err := reg.Get(pid)
			if err != nil {
				return err
			}
			if info.LogPath == "" {
				return fmt.Errorf("no log path for session PID %d", pid)
			}

			follow, _ := cmd.Flags().GetBool("follow")
			if follow {
				return tailFollow(info.LogPath)
			}
			return tailLast(info.LogPath, 50)
		},
	}
	cmd.Flags().BoolP("follow", "f", false, "follow log output (like tail -f)")
	return cmd
}

// tailLast prints the last n lines of a file.
func tailLast(path string, n int) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) > n {
			lines = lines[1:]
		}
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return scanner.Err()
}

// tailFollow tails a file, printing new lines as they appear.
func tailFollow(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}
	defer f.Close()

	// Seek to end
	if _, err := f.Seek(0, io.SeekEnd); err != nil {
		return err
	}

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				time.Sleep(time.Second)
				continue
			}
			return err
		}
		fmt.Print(line)
	}
}
