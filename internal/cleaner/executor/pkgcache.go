package executor

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

type PkgCacheExecutor struct{}

func NewPkgCacheExecutor() *PkgCacheExecutor { return &PkgCacheExecutor{} }

var pkgArgv = map[string][][]string{
	"apt":    {{"apt-get", "clean"}, {"apt-get", "autoclean"}},
	"dnf":    {{"dnf", "clean", "all"}},
	"pacman": {{"pacman", "-Sc", "--noconfirm"}},
	"zypper": {{"zypper", "clean", "-a"}},
}

func (p *PkgCacheExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := time.Now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	if c.PkgCache == nil || c.PkgCache.Manager == "" {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: "pkg_cache config missing", TS: time.Now()})
		emitFinish(emit, c.ID, "error", 1, start)
		return nil
	}

	commands, ok := pkgArgv[c.PkgCache.Manager]
	if !ok {
		if _, err := exec.LookPath(c.PkgCache.Manager); err != nil {
			emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "manager_not_installed", TS: time.Now()})
			emitFinish(emit, c.ID, "skipped", 0, start)
			return nil
		}
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: fmt.Sprintf("unknown manager %q", c.PkgCache.Manager), TS: time.Now()})
		emitFinish(emit, c.ID, "error", 1, start)
		return nil
	}
	if _, err := exec.LookPath(commands[0][0]); err != nil {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "manager_not_installed", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}
	if dryRun {
		for _, argv := range commands {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: "[dry-run] " + strings.Join(argv, " "), TS: time.Now()})
		}
		emitFinish(emit, c.ID, "ok", 0, start)
		return nil
	}

	errs := 0
	for _, argv := range commands {
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
	}
	status := "ok"
	if errs > 0 {
		status = "error"
	}
	emitFinish(emit, c.ID, status, errs, start)
	return nil
}
