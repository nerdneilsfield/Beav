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

type CommandRun struct {
	Argv     []string
	Timeout  time.Duration
	OnStdout func(string)
	OnStderr func(string)
}

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
