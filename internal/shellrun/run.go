package shellrun

import (
	"context"
	"errors"
	"fmt"
	"github.com/artpar/pragma/internal/observe"
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
	// OnLiveOutput, when set, is invoked with the tail of output-so-far
	// while the foreground command runs (TUI-004), at most once per
	// LiveOutputInterval and only when the output has grown.
	OnLiveOutput       func(soFar string)
	LiveOutputInterval time.Duration
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
	observe.TraceCtx(ctx, "shellrun", "Execute", "enter")
	defer observe.TraceCtx(ctx, "shellrun", "Execute", "exit")
	if opts.Timeout <= 0 {
		observe.TraceCtx(ctx, "shellrun", "Execute", "if: opts.Timeout <= 0")
		observe.TraceCtx(ctx, "shellrun", "Execute", "return: Result{}, fmt.Errorf(\"timeout is required\")")
		return Result{}, fmt.Errorf("timeout is required")
	}
	if opts.ForegroundWait <= 0 {
		observe.TraceCtx(ctx, "shellrun", "Execute", "if: opts.ForegroundWait <= 0")
		opts.ForegroundWait = opts.Timeout
	}
	if opts.RunningOutputLines <= 0 {
		observe.TraceCtx(ctx, "shellrun", "Execute", "if: opts.RunningOutputLines <= 0")
		opts.RunningOutputLines = 100
	}
	files, err := NewFiles(opts.BaseDirName, opts.Command)
	if err != nil {
		observe.TraceCtx(ctx, "shellrun", "Execute", "if: err != nil")
		observe.TraceCtx(ctx, "shellrun", "Execute", "return: Result{}, err")
		return Result{}, err
	}
	result := Result{Files: files, ExitCode: -1}
	if opts.Background {
		observe.TraceCtx(ctx, "shellrun", "Execute", "if: opts.Background")
		observe.TraceCtx(ctx, "shellrun", "Execute", "return: startBackground(opts, files)")
		return startBackground(opts, files)
	}

	cmdCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	cancelOnReturn := true
	defer func() {
		if cancelOnReturn {
			observe.TraceCtx(ctx, "shellrun", "Execute", "if: cancelOnReturn")
			cancel()
		}
	}()

	cmd, consoleFile, err := startCommand(cmdCtx, opts, files)
	if err != nil {
		observe.TraceCtx(ctx, "shellrun", "Execute", "if: err != nil")
		cancel()
		observe.TraceCtx(ctx, "shellrun", "Execute", "return: result, err")
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
	// TUI-004: while the foreground command runs, a ticker reads the
	// console file and invokes OnLiveOutput with the bounded tail whenever
	// the output has grown. The nil-channel case never fires when the
	// callback is unset.
	var liveCh <-chan time.Time
	var lastLive string
	if opts.OnLiveOutput != nil {
		observe.TraceCtx(ctx, "shellrun", "Execute", "if: opts.OnLiveOutput != nil")
		interval := opts.LiveOutputInterval
		if interval <= 0 {
			observe.TraceCtx(ctx, "shellrun", "Execute", "if: interval <= 0")
			interval = 250 * time.Millisecond
		}
		liveTicker := time.NewTicker(interval)
		defer liveTicker.Stop()
		liveCh = liveTicker.C
	}
	fgExpiry := time.After(opts.ForegroundWait)
waitLoop:
	for {
		select {
		case waitErr = <-done:
			observe.TraceCtx(ctx, "shellrun", "Execute", "select: waitErr = <-done")
			cancel()
			break waitLoop
		case <-fgExpiry:
			observe.TraceCtx(ctx, "shellrun", "Execute", "select: <-fgExpiry")
			output := TailLines(ReadOutput(files), opts.RunningOutputLines)
			result.Output = RunningContent("Command is still running after "+opts.ForegroundWait.String()+".", result.PID, files, output, opts.RunningOutputLines)
			result.Running = true
			cancelOnReturn = false
			return result, nil
		case <-cmdCtx.Done():
			observe.TraceCtx(ctx, "shellrun", "Execute", "select: <-cmdCtx.Done()")
			result.TimedOut = cmdCtx.Err() == context.DeadlineExceeded
			waitErr = <-done
			break waitLoop
		case <-liveCh:
			observe.TraceCtx(ctx, "shellrun", "Execute", "select: <-liveCh")
			soFar := ReadOutput(files)
			if soFar != lastLive {
				observe.TraceCtx(ctx, "shellrun", "Execute", "if: soFar != lastLive")
				lastLive = soFar
				opts.OnLiveOutput(TailLines(soFar, opts.RunningOutputLines))
			}
		}
	}

	result.Output = ReadOutput(files)
	result.ExitCode = exitCode(waitErr)
	result.Err = waitErr
	if result.TimedOut {
		observe.TraceCtx(ctx, "shellrun", "Execute", "if: result.TimedOut")
		result.ExitCode = -1
	}
	observe.TraceCtx(ctx, "shellrun", "Execute", "return: result, nil")
	return result, nil
}

func startBackground(opts Options, files Files) (Result, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	cmdCtx, cancel := context.WithTimeout(context.Background(), opts.Timeout)
	cmd, consoleFile, err := startCommand(cmdCtx, opts, files)
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		cancel()
		observe.GlobalTrace("return: Result{Files: files, ExitCode: -1}, err")
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
	observe.GlobalTrace("return: result, nil")
	return result, nil
}

func startCommand(ctx context.Context, opts Options, files Files) (*exec.Cmd, *os.File, error) {
	observe.TraceCtx(ctx, "shellrun", "startCommand", "enter")
	defer observe.TraceCtx(ctx, "shellrun", "startCommand", "exit")
	command := opts.Command
	args := []string{"-c", commandForShellRun(command, opts.Background)}
	if opts.UseErrexit {
		observe.TraceCtx(ctx, "shellrun", "startCommand", "if: opts.UseErrexit")
		args = append([]string{"-e"}, args...)
	}
	if opts.UsePipefail {
		observe.TraceCtx(ctx, "shellrun", "startCommand", "if: opts.UsePipefail")
		args = append([]string{"-o", "pipefail"}, args...)
	}
	cmd := exec.CommandContext(ctx, "bash", args...)
	cmd.Dir = opts.WorkDir
	cmd.WaitDelay = 5 * time.Second
	setProcAttr(cmd)

	consoleFile, err := os.OpenFile(files.Console, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		observe.TraceCtx(ctx, "shellrun", "startCommand", "if: err != nil")
		observe.TraceCtx(ctx, "shellrun", "startCommand", "return: nil, nil, fmt.Errorf(\"open console log: %w\", err)")
		return nil, nil, fmt.Errorf("open console log: %w", err)
	}
	cmd.Stdout = consoleFile
	cmd.Stderr = consoleFile
	if err := cmd.Start(); err != nil {
		observe.TraceCtx(ctx, "shellrun", "startCommand", "if: err != nil")
		consoleFile.Close()
		observe.TraceCtx(ctx, "shellrun", "startCommand", "return: nil, nil, fmt.Errorf(\"start command: %w\", err)")
		return nil, nil, fmt.Errorf("start command: %w", err)
	}
	_ = os.WriteFile(files.Status, []byte(fmt.Sprintf("running\npid=%d\nstarted=%s\n", cmd.Process.Pid, time.Now().Format(time.RFC3339))), 0o644)
	observe.TraceCtx(ctx, "shellrun", "startCommand", "return: cmd, consoleFile, nil")
	return cmd, consoleFile, nil
}

func NewFiles(baseDirName, command string) (Files, error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if baseDirName == "" {
		observe.GlobalTrace("if: baseDirName == \"\"")
		baseDirName = "pragma-bash"
	}
	base := filepath.Join(os.TempDir(), baseDirName)
	if err := os.MkdirAll(base, 0o755); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Files{}, fmt.Errorf(\"create shell command log directory: %w\", err)")
		return Files{}, fmt.Errorf("create shell command log directory: %w", err)
	}
	dir, err := os.MkdirTemp(base, "cmd-")
	if err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Files{}, fmt.Errorf(\"create shell command directory: %w\", err)")
		return Files{}, fmt.Errorf("create shell command directory: %w", err)
	}
	files := Files{
		Dir:     dir,
		Command: filepath.Join(dir, "command.sh"),
		Console: filepath.Join(dir, "console.log"),
		Status:  filepath.Join(dir, "status.txt"),
	}
	if err := os.WriteFile(files.Command, []byte(command), 0o644); err != nil {
		observe.GlobalTrace("if: err != nil")
		observe.GlobalTrace("return: Files{}, fmt.Errorf(\"write command log: %w\", err)")
		return Files{}, fmt.Errorf("write command log: %w", err)
	}
	observe.GlobalTrace("return: files, nil")
	return files, nil
}

func writeStatus(files Files, pid int, timeout time.Duration, cmdCtx context.Context, err error) {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	status := "exited"
	detail := "exit_code=0"
	if cmdCtx.Err() == context.DeadlineExceeded {
		observe.GlobalTrace("if: cmdCtx.Err() == context.DeadlineExceeded")
		status = "timeout"
		detail = fmt.Sprintf("timeout_ms=%d", timeout.Milliseconds())
	} else if err != nil {
		observe.GlobalTrace("else-if: err != nil")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if output != "" {
		observe.GlobalTrace("if: output != \"\"")
		output = fmt.Sprintf("Console tail (last %d lines):\n%s\n\n", maxLines, output)
	}
	observe.GlobalTrace("return: output + fmt.Sprintf(`%s\nPID: %d\nCommand: %s\nStatus: %s\nConsole: %s\n\nActive p...")
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
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	output = strings.TrimRight(output, "\n")
	if output == "" || maxLines <= 0 {
		observe.GlobalTrace("if: output == \"\" || maxLines <= 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	lines := strings.Split(output, "\n")
	if len(lines) <= maxLines {
		observe.GlobalTrace("if: len(lines) <= maxLines")
		observe.GlobalTrace("return: output")
		return output
	}
	observe.GlobalTrace("return: strings.Join(lines[len(lines)-maxLines:], \"\\n\")")
	return strings.Join(lines[len(lines)-maxLines:], "\n")
}

func ReadOutput(files Files) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	output, err := os.ReadFile(files.Console)
	if err != nil || len(output) == 0 {
		observe.GlobalTrace("if: err != nil || len(output) == 0")
		observe.GlobalTrace("return: \"\"")
		return ""
	}
	observe.GlobalTrace("return: string(output)")
	return string(output)
}

func ProcessGroupSummary(pid int) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	out, err := exec.Command("ps", "-o", "pid,ppid,pgid,stat,etime,comm,args", "-g", strconv.Itoa(pid)).Output()
	if err != nil || len(out) == 0 {
		observe.GlobalTrace("if: err != nil || len(out) == 0")
		observe.GlobalTrace("return: fmt.Sprintf(\"pid=%d\", pid)")
		return fmt.Sprintf("pid=%d", pid)
	}
	observe.GlobalTrace("return: strings.TrimRight(string(out), \"\\n\")")
	return strings.TrimRight(string(out), "\n")
}

func CommandMayUseBackground(command string) bool {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	observe.GlobalTrace("return: strings.Contains(command, \"&\")")
	return strings.Contains(command, "&")
}

func commandForShellRun(command string, detached bool) string {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if detached || !CommandMayUseBackground(command) {
		observe.GlobalTrace("if: detached || !CommandMayUseBackground(command)")
		observe.GlobalTrace("return: command")
		return command
	}
	observe.GlobalTrace("return: command + \"\\n__pragma_status=$?\\ndisown -a 2>/dev/null || true\\nexit $__pragm...")
	return command + "\n__pragma_status=$?\ndisown -a 2>/dev/null || true\nexit $__pragma_status"
}

func exitCode(err error) int {
	observe.GlobalTrace("enter")
	defer observe.GlobalTrace("exit")
	if err == nil {
		observe.GlobalTrace("if: err == nil")
		observe.GlobalTrace("return: 0")
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		observe.GlobalTrace("if: errors.As(err, &exitErr)")
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			observe.GlobalTrace("if: ok && status.Signaled()")
			observe.GlobalTrace("return: -int(status.Signal())")
			return -int(status.Signal())
		}
		observe.GlobalTrace("return: exitErr.ExitCode()")
		return exitErr.ExitCode()
	}
	observe.GlobalTrace("return: -1")
	return -1
}
