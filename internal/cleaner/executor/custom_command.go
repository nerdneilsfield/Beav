package executor

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

// CustomCommandExecutor runs user-defined type: command cleaners when allowed by policy.
// CustomCommandExecutor 在安全策略允许时执行用户自定义的 command 类型清理器。
type CustomCommandExecutor struct {
	Home   string
	Policy safety.Policy
}

// NewCustomCommandExecutor creates a CustomCommandExecutor with the given home and policy.
// NewCustomCommandExecutor 使用指定主目录和安全策略创建 CustomCommandExecutor。
func NewCustomCommandExecutor(home string, policy safety.Policy) *CustomCommandExecutor {
	return &CustomCommandExecutor{Home: home, Policy: policy}
}

// Run executes a custom command cleaner, honoring dryRun and Policy.
// Run 执行自定义命令清理器，遵循 dryRun 与安全策略。
func (e *CustomCommandExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := time.Now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})

	if !e.Policy.EffectiveAllowCommandCleaners() {
		emit(model.Event{
			Event:     model.EvCleanerSkipped,
			CleanerID: c.ID,
			Reason:    "command_disabled",
			Detail:    "set safety.allow_command_cleaners: true or pass --unsafe to run custom command cleaners",
			TS:        time.Now(),
		})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}
	if !e.Policy.EffectiveIgnoreRunning() && safety.AnyProcessRunning(c.RunningProcesses) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: string(model.SkipReasonRunningProcess), TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}
	if c.Command == "" {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: "missing command", TS: time.Now()})
		emitFinish(emit, c.ID, "error", 1, start)
		return nil
	}

	if dryRun {
		if c.DryRunCommand != "" {
			if err := e.runShellOrArgs(ctx, c.ID, c.DryRunCommand, nil, emit); err != nil {
				emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "command_failed", Detail: err.Error(), TS: time.Now()})
				emitFinishWithMetrics(emit, c.ID, "error", 1, start, true)
				return nil
			}
		} else {
			preview := c.Command
			if len(c.Args) > 0 {
				preview = c.Command + " " + strings.Join(c.Args, " ")
			}
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: "[dry-run] " + preview, TS: time.Now()})
		}
		emitFinishWithMetrics(emit, c.ID, "ok", 0, start, true)
		return nil
	}

	if err := e.runShellOrArgs(ctx, c.ID, c.Command, c.Args, emit); err != nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "command_failed", Detail: err.Error(), TS: time.Now()})
		emitFinishWithMetrics(emit, c.ID, "error", 1, start, true)
		return nil
	}
	emitFinishWithMetrics(emit, c.ID, "ok", 0, start, true)
	return nil
}

func (e *CustomCommandExecutor) runShellOrArgs(ctx context.Context, cleanerID, command string, args []string, emit func(model.Event)) error {
	var argv []string
	if len(args) > 0 {
		bin := command
		if resolved, err := exec.LookPath(command); err == nil {
			bin = resolved
		}
		argv = append([]string{bin}, args...)
	} else {
		argv = []string{"/bin/sh", "-c", command}
	}
	return RunCommand(ctx, CommandRun{
		Argv:    argv,
		Cwd:     e.Home,
		Timeout: 5 * time.Minute,
		OnStdout: func(line string) {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: cleanerID, Stream: "stdout", Line: line, TS: time.Now()})
		},
		OnStderr: func(line string) {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: cleanerID, Stream: "stderr", Line: line, TS: time.Now()})
		},
	})
}
