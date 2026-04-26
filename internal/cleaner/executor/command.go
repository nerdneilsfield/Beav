package executor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// CommandRun holds the configuration for running an external command.
// CommandRun 保存运行外部命令的配置。
type CommandRun struct {
	// Argv is the command and its arguments.
	// Argv 是命令及其参数。
	Argv []string
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
	if len(r.Argv) == 0 {
		return errors.New("empty argv")
	}
	if r.Timeout == 0 {
		r.Timeout = 60 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, r.Argv[0], r.Argv[1:]...) // #nosec G204 -- argv is selected by built-in executors, not shell-expanded user input.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan struct{}, 2)
	go func() {
		drain(stdout, r.OnStdout)
		done <- struct{}{}
	}()
	go func() {
		drain(stderr, r.OnStderr)
		done <- struct{}{}
	}()
	err = cmd.Wait()
	<-done
	<-done
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timeout after %s", r.Timeout)
	}
	return err
}

// drain reads from a reader line by line, passing each line to the callback function.
// drain 逐行从读取器读取，将每一行传递给回调函数。
func drain(r io.Reader, fn func(string)) {
	if fn == nil {
		_, _ = io.Copy(io.Discard, r)
		return
	}
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fn(scanner.Text())
	}
}

// emitFinish emits a finish event with the given status and error count.
// emitFinish 发出带有给定状态和错误计数的完成事件。
func emitFinish(emit func(model.Event), id, status string, errs int, start time.Time) {
	emit(model.Event{
		Event:      model.EvFinish,
		CleanerID:  id,
		Status:     status,
		Errors:     errs,
		DurationMs: time.Since(start).Milliseconds(),
		TS:         time.Now(),
	})
}
