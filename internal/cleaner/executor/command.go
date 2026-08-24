package executor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"golang.org/x/sys/unix"
)

// CommandIdentity describes the exact credentials a child process must use.
// CommandIdentity 描述子进程必须使用的确切凭据。
//
// Groups contains supplementary groups only; GID is always the primary group.
// Groups 只含附加组；GID 始终是主组。
type CommandIdentity struct {
	UID    uint32
	GID    uint32
	Groups []uint32
	Name   string
	Home   string
}

// TargetIdentity is a compatibility alias for CommandIdentity.
// TargetIdentity 是 CommandIdentity 的兼容别名。
type TargetIdentity = CommandIdentity

// CommandRun holds the configuration for running an external command.
// CommandRun 保存运行外部命令的配置。
type CommandRun struct {
	// Executable is the resolved absolute executable path. When set, it is
	// used for both argv[0] and exec, and Args are passed unchanged.
	// Executable 是已解析的绝对可执行文件路径；设置后同时用于 argv[0] 和 exec，Args 原样传递。
	Executable string
	// Args are arguments following Executable.
	// Args 是 Executable 后的参数。
	Args []string
	// Argv is the command and its arguments.
	// Argv 是命令及其参数。
	Argv []string
	// Env is the complete child environment. A non-nil value is never merged
	// with the parent environment.
	// Env 是子进程完整环境；非 nil 时绝不与父环境合并。
	Env []string
	// Environment is an alternate spelling retained for callers that prefer
	// the longer name.
	// Environment 是保留给偏好长名称调用方的别名。
	Environment []string
	// Cwd is the child working directory.
	// Cwd 是子进程工作目录。
	Cwd string
	// Identity, when non-nil, applies exact UID/GID/supplementary groups.
	// Identity 非 nil 时应用确切 UID/GID/附加组。
	Identity *CommandIdentity
	// TargetIdentity is an alternate spelling for Identity.
	// TargetIdentity 是 Identity 的别名字段。
	TargetIdentity *CommandIdentity
	// DryRun marks a native dry-run invocation. RunCommand does not infer or
	// alter argv from this flag; profile executors decide whether to invoke.
	// DryRun 标记原生试运行调用；RunCommand 不据此推断或修改 argv，由 profile executor 决定是否调用。
	DryRun bool
	// AffectedRoots are safety-checked by profile executors before invocation.
	// AffectedRoots 由 profile executor 在调用前执行安全检查。
	AffectedRoots []string
	// Timeout is the maximum duration for the command to run.
	// Timeout 是命令运行的最大持续时间。
	Timeout time.Duration
	// OnStdout is called for each line of stdout output.
	// OnStdout 为标准输出的每一行调用。
	OnStdout func(string)
	// OnStderr is called for each line of stderr output.
	// OnStderr 为标准错误输出的每一行调用。
	OnStderr func(string)
}

// RunCommand executes a command with the given configuration and streams output to callbacks.
// RunCommand 使用给定配置执行命令，并将输出流式传输到回调函数。
func RunCommand(ctx context.Context, r CommandRun) error {
	argv, executable, err := commandArgv(r)
	if err != nil {
		return err
	}
	if len(argv) == 0 || executable == "" {
		return errors.New("empty argv")
	}
	if r.Timeout == 0 {
		r.Timeout = 60 * time.Second
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	env := r.Env
	if env == nil {
		env = r.Environment
	}
	if env != nil {
		env = append([]string(nil), env...)
	} else {
		env = os.Environ()
	}
	devNull, err := os.OpenFile("/dev/null", os.O_RDONLY, 0)
	if err != nil {
		return fmt.Errorf("open /dev/null: %w", err)
	}
	defer devNull.Close()
	stdout, stdoutWriter, err := os.Pipe()
	if err != nil {
		return err
	}
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		return err
	}
	payload := commandHelperPayload{
		Executable: executable,
		Argv:       append([]string(nil), argv...),
		Env:        env,
		Cwd:        r.Cwd,
		Identity:   commandIdentity(r),
	}
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		_ = stderr.Close()
		_ = stderrWriter.Close()
		return fmt.Errorf("encode command helper payload: %w", err)
	}
	payloadReader, payloadWriter, err := os.Pipe()
	if err != nil {
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		_ = stderr.Close()
		_ = stderrWriter.Close()
		return err
	}
	if _, err := payloadWriter.Write(payloadBytes); err != nil {
		_ = payloadReader.Close()
		_ = payloadWriter.Close()
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		_ = stderr.Close()
		_ = stderrWriter.Close()
		return fmt.Errorf("write command helper payload: %w", err)
	}
	if err := payloadWriter.Close(); err != nil {
		_ = payloadReader.Close()
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		_ = stderr.Close()
		_ = stderrWriter.Close()
		return fmt.Errorf("close command helper payload: %w", err)
	}
	self, err := os.Executable()
	if err != nil {
		_ = payloadReader.Close()
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		_ = stderr.Close()
		_ = stderrWriter.Close()
		return fmt.Errorf("resolve command helper: %w", err)
	}
	cmd := exec.Command(self, "--beav-command-helper") // #nosec G204 -- self executable enters private helper mode.
	cmd.Env = []string{commandHelperEnv + "=1"}
	cmd.ExtraFiles = []*os.File{payloadReader}
	cmd.Stdin = devNull
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		_ = payloadReader.Close()
		_ = stdout.Close()
		_ = stdoutWriter.Close()
		_ = stderr.Close()
		_ = stderrWriter.Close()
		return err
	}
	_ = payloadReader.Close()
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	drainResults := make(chan drainResult, 2)
	go func() {
		drainResults <- drainResult{stream: "stdout", err: drain(stdout, r.OnStdout)}
	}()
	go func() {
		drainResults <- drainResult{stream: "stderr", err: drain(stderr, r.OnStderr)}
	}()
	wait := make(chan error, 1)
	go func() { wait <- cmd.Wait() }()
	var timedOut bool
	var canceled bool
	var waitErr error
	var waitReceived bool
	var drainErrors []error
	var drainsReceived int
	ctxDone := ctx.Done()
	var killErr error
	var closeOnce sync.Once
	closePipes := func() {
		closeOnce.Do(func() {
			_ = stdout.Close()
			_ = stderr.Close()
		})
	}
	for !waitReceived || drainsReceived < 2 {
		select {
		case waitErr = <-wait:
			waitReceived = true
		case result := <-drainResults:
			drainsReceived++
			if result.err != nil {
				drainErrors = append(drainErrors, fmt.Errorf("%s drain: %w", result.stream, result.err))
			}
		case <-ctxDone:
			canceled = true
			timedOut = errors.Is(ctx.Err(), context.DeadlineExceeded)
			ctxDone = nil
			killErr = killProcessGroup(cmd.Process.Pid)
			closePipes()
		}
	}
	closePipes()
	if timedOut {
		return commandTerminationError(fmt.Errorf("timeout after %s: %w", r.Timeout, context.DeadlineExceeded), killErr, drainErrors)
	}
	if canceled {
		return commandTerminationError(ctx.Err(), killErr, drainErrors)
	}
	return commandTerminationError(waitErr, nil, drainErrors)
}

type drainResult struct {
	stream string
	err    error
}

var killSyscall = unix.Kill

func commandTerminationError(commandErr, killErr error, drainErrors []error) error {
	errs := make([]error, 0, 2+len(drainErrors))
	if commandErr != nil {
		errs = append(errs, commandErr)
	}
	if killErr != nil {
		errs = append(errs, fmt.Errorf("kill process group: %w", killErr))
	}
	errs = append(errs, drainErrors...)
	return errors.Join(errs...)
}

func commandArgv(r CommandRun) ([]string, string, error) {
	if r.Executable != "" {
		if !filepath.IsAbs(r.Executable) {
			return nil, "", errors.New("executable must be absolute")
		}
		args := append([]string(nil), r.Args...)
		if len(args) == 0 && len(r.Argv) > 0 {
			if r.Argv[0] == r.Executable {
				args = append(args, r.Argv[1:]...)
			} else {
				args = append(args, r.Argv...)
			}
		}
		return append([]string{r.Executable}, args...), r.Executable, nil
	}
	if len(r.Argv) == 0 {
		return nil, "", errors.New("empty argv")
	}
	if filepath.IsAbs(r.Argv[0]) {
		return append([]string(nil), r.Argv...), r.Argv[0], nil
	}
	resolved, err := exec.LookPath(r.Argv[0])
	if err != nil {
		return nil, "", err
	}
	argv := append([]string{resolved}, r.Argv[1:]...)
	return argv, resolved, nil
}

func commandIdentity(r CommandRun) *CommandIdentity {
	if r.Identity != nil {
		return r.Identity
	}
	return r.TargetIdentity
}

func killProcessGroup(pid int) error {
	if pid <= 0 {
		return errors.New("invalid process group PID")
	}
	groupErr := killSyscall(-pid, unix.SIGKILL)
	if groupErr == nil {
		return nil
	}
	leaderErr := killSyscall(pid, unix.SIGKILL)
	errs := []error{fmt.Errorf("process group kill: %w", groupErr)}
	if leaderErr != nil {
		errs = append(errs, fmt.Errorf("leader cleanup: %w", leaderErr))
	}
	return errors.Join(errs...)
}

// drain reads from a reader line by line, passing each line to the callback function.
// drain 逐行从读取器读取，将每一行传递给回调函数。
func drain(r io.Reader, fn func(string)) error {
	if fn == nil {
		_, err := io.Copy(io.Discard, r)
		return err
	}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fn(scanner.Text())
	}
	return scanner.Err()
}

// emitFinish emits a finish event with the given status and error count.
// emitFinish 发出带有给定状态和错误计数的完成事件。
func emitFinish(emit func(model.Event), id, status string, errs int, start time.Time) {
	emitFinishWithMetrics(emit, id, status, errs, start, false)
}

func emitFinishWithMetrics(emit func(model.Event), id, status string, errs int, start time.Time, metricsIncomplete bool) {
	emit(model.Event{
		Event:             model.EvFinish,
		CleanerID:         id,
		Status:            status,
		Errors:            errs,
		MetricsIncomplete: metricsIncomplete,
		DurationMs:        time.Since(start).Milliseconds(),
		TS:                time.Now(),
	})
}
