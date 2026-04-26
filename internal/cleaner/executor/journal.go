package executor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// JournalExecutor implements the Executor interface for journal log vacuuming.
// JournalExecutor 实现了用于日志清理的 Executor 接口。
type JournalExecutor struct{}

// NewJournalExecutor creates a new JournalExecutor.
// NewJournalExecutor 创建一个新的 JournalExecutor。
func NewJournalExecutor() *JournalExecutor { return &JournalExecutor{} }

// Run executes the journal vacuum operation, emitting events for each action taken.
// Run 执行日志清理操作，为每个操作发出事件。
func (j *JournalExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := time.Now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	if _, err := exec.LookPath("journalctl"); err != nil {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "manager_not_installed", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}
	age := c.AgeOrDefault(14)
	argv := []string{"journalctl", "--vacuum-time=" + fmt.Sprintf("%dd", age)}
	if dryRun {
		emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: "[dry-run] " + strings.Join(argv, " "), TS: time.Now()})
		emitFinish(emit, c.ID, "ok", 0, start)
		return nil
	}
	errs := 0
	err := RunCommand(ctx, CommandRun{
		Argv:    argv,
		Timeout: 5 * time.Minute,
		OnStdout: func(line string) {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: line, TS: time.Now()})
		},
		OnStderr: func(line string) {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stderr", Line: line, TS: time.Now()})
		},
	})
	if err != nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "command_failed", Detail: err.Error(), TS: time.Now()})
		errs++
	}
	status := "ok"
	if errs > 0 {
		status = "error"
	}
	emitFinish(emit, c.ID, status, errs, start)
	return nil
}
