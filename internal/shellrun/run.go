package shellrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Options struct {
	Command            string
	WorkDir            string
	Timeout            time.Duration
	ForegroundWait     time.Duration
	Background         bool
	BaseDirName        string
	UsePipefail        bool
	UseErrexit         bool
	RunningOutputLines int
}

type Files struct {
	Dir     string
	Command string
	Console string
	Status  string
}

type Result struct {
	ExitCode int
	Output   string
	Files    Files
	PID      int
	Running  bool
	TimedOut bool
	Err      error
}

func Execute(ctx context.Context, opts Options) (Result, error) {
	if opts.Timeout <= 0 {
		return Result{}, fmt.Errorf("timeout is required")
	}
	if opts.ForegroundWait <= 0 {
		opts.ForegroundWait = opts.Timeout
	}
	if opts.RunningOutputLines <= 0 {
		opts.RunningOutputLines = 100
	}
	files, err := NewFiles(opts.BaseDirName, opts.Command)
	if err != nil {
		return Result{}, err
	}
	result := Result{Files: files, ExitCode: -1}
	if opts.Background {
		return startBackground(opts, files)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	cancelOnReturn := true
	defer func() {
		if cancelOnReturn {
			cancel()
		}
	}()

	cmd, consoleFile, err := startCommand(cmdCtx, opts, files)
	if err != nil {
		cancel()
		return result, err
	}
	result.PID = cmd.Process.Pid

	done := make(chan error, 1)
	go func() {
		defer cancel()
		err := cmd.Wait()
		consoleFile.Close()
		writeStatus(files, result.PID, opts.Timeout, cmdCtx, err)
		done <- err
	}()

	var waitErr error
	select {
	case waitErr = <-done:
		cancel()
	case <-time.After(opts.ForegroundWait):
		output := TailLines(ReadOutput(files), opts.RunningOutputLines)
		result.Output = RunningContent("Command is still running after "+opts.ForegroundWait.String()+".", result.PID, files, output, opts.RunningOutputLines)
		result.Running = true
		cancelOnReturn = false
		return result, nil
	case <-cmdCtx.Done():
		result.TimedOut = cmdCtx.Err() == context.DeadlineExceeded
		waitErr = <-done
	}

	result.Output = ReadOutput(files)
	result.ExitCode = exitCode(waitErr)
	result.Err = waitErr
	if result.TimedOut {
		result.ExitCode = -1
	}
	return result, nil
}

func startBackground(opts Options, files Files) (Result, error) {
	cmdCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	cmd, consoleFile, err := startCommand(cmdCtx, opts, files)
	if err != nil {
		cancel()
		return Result{Files: files, ExitCode: -1}, err
	}
	result := Result{
		ExitCode: -1,
		Files:    files,
		PID:      cmd.Process.Pid,
		Running:  true,
	}
	go func() {
		defer cancel()
		defer consoleFile.Close()
		err := cmd.Wait()
		writeStatus(files, result.PID, opts.Timeout, cmdCtx, err)
	}()
	result.Output = RunningContent("Started background command.", result.PID, files, "", opts.RunningOutputLines)
	return result, nil
}

func startCommand(ctx context.Context, opts Options, files Files) (*exec.Cmd, *os.File, error) {
	command := opts.Command
	args := []string{"-c", commandForShellRun(command, opts.Background)}
	if opts.UseErrexit {
		args = append([]string{"-e"}, args...)
	}
	if opts.UsePipefail {
		args = append([]string{"-o", "pipefail"}, args...)
	}
	cmd := exec.CommandContext(ctx, "bash", args...)
	cmd.Dir = opts.WorkDir
	cmd.WaitDelay = 5 * time.Second
	setProcAttr(cmd)

	consoleFile, err := os.OpenFile(files.Console, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, fmt.Errorf("open console log: %w", err)
	}
	cmd.Stdout = consoleFile
	cmd.Stderr = consoleFile
	if err := cmd.Start(); err != nil {
		consoleFile.Close()
		return nil, nil, fmt.Errorf("start command: %w", err)
	}
	_ = os.WriteFile(files.Status, []byte(fmt.Sprintf("running\npid=%d\nstarted=%s\n", cmd.Process.Pid, time.Now().Format(time.RFC3339))), 0o644)
	return cmd, consoleFile, nil
}

func NewFiles(baseDirName, command string) (Files, error) {
	if baseDirName == "" {
		baseDirName = "pragma-bash"
	}
	base := filepath.Join(os.TempDir(), baseDirName)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return Files{}, fmt.Errorf("create shell command log directory: %w", err)
	}
	dir, err := os.MkdirTemp(base, "cmd-")
	if err != nil {
		return Files{}, fmt.Errorf("create shell command directory: %w", err)
	}
	files := Files{
		Dir:     dir,
		Command: filepath.Join(dir, "command.sh"),
		Console: filepath.Join(dir, "console.log"),
		Status:  filepath.Join(dir, "status.txt"),
	}
	if err := os.WriteFile(files.Command, []byte(command), 0o644); err != nil {
		return Files{}, fmt.Errorf("write command log: %w", err)
	}
	return files, nil
}

func writeStatus(files Files, pid int, timeout time.Duration, cmdCtx context.Context, err error) {
	status := "exited"
	detail := "exit_code=0"
	if cmdCtx.Err() == context.DeadlineExceeded {
		status = "timeout"
		detail = fmt.Sprintf("timeout_ms=%d", timeout.Milliseconds())
	} else if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			detail = fmt.Sprintf("exit_code=%d", exitErr.ExitCode())
		} else {
			status = "error"
			detail = fmt.Sprintf("error=%s", err.Error())
		}
	}
	_ = os.WriteFile(files.Status, []byte(fmt.Sprintf("%s\npid=%d\n%s\nended=%s\n", status, pid, detail, time.Now().Format(time.RFC3339))), 0o644)
}

func RunningContent(prefix string, pid int, files Files, output string, maxLines int) string {
	if output != "" {
		output = fmt.Sprintf("Console tail (last %d lines):\n%s\n\n", maxLines, output)
	}
	return output + fmt.Sprintf(`%s
PID: %d
Command: %s
Status: %s
Console: %s

Active processes:
%s

Poll with: cat %q
Inspect logs with: tail -100 %q`, prefix, pid, files.Command, files.Status, files.Console, ProcessGroupSummary(pid), files.Status, files.Console)
}

func TailLines(output string, maxLines int) string {
	output = strings.TrimRight(output, "\n")
	if output == "" || maxLines <= 0 {
		return ""
	}
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		return output
	}
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func ReadOutput(files Files) string {
	output, err := os.ReadFile(files.Console)
	if err != nil || len(output) == 0 {
		return ""
	}
	return string(output)
}

func ProcessGroupSummary(pid int) string {
	out, err := exec.Command("ps", "-o", "pid,ppid,pgid,stat,etime,comm,args", "-g", strconv.Itoa(pid)).Output()
	if err != nil || len(out) == 0 {
		return fmt.Sprintf("pid=%d", pid)
	}
	return strings.TrimRight(string(out), "\n")
}

func CommandMayUseBackground(command string) bool {
	return strings.Contains(command, "&")
}

func commandForShellRun(command string, detached bool) string {
	if detached || !CommandMayUseBackground(command) {
		return command
	}
	return command + "\n__pragma_status=$?\ndisown -a 2>/dev/null || true\nexit $__pragma_status"
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return -int(status.Signal())
		}
		return exitErr.ExitCode()
	}
	return -1
}
