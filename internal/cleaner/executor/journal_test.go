package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// TestJournalVacuumSkipsWhenJournalctlMissing verifies that the cleaner skips when journalctl is not installed.
// TestJournalVacuumSkipsWhenJournalctlMissing 验证当 journalctl 未安装时清理器会跳过。
func TestJournalVacuumSkipsWhenJournalctlMissing(t *testing.T) {
	if _, err := exec.LookPath("journalctl"); err == nil {
		t.Skip("journalctl present; skipping negative test")
	}
	c := model.Cleaner{
		ID:         "j",
		Name:       "journal",
		Scope:      model.ScopeSystem,
		Type:       model.TypeJournalVacuum,
		MinAgeDays: ptrInt(7),
	}
	evs := captureEvents(t, func(emit func(model.Event)) {
		_ = NewJournalExecutor().Run(context.Background(), c, false, emit)
	})
	if !hasCleanerSkip(evs, "manager_not_installed") {
		t.Fatalf("expected cleaner_skipped/manager_not_installed; got %+v", evs)
	}
}

func TestJournalExecutorRotatesAndVacuumsWithMetricsIncomplete(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "journal.log")
	bin := filepath.Join(dir, "journalctl")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho \"$*\" >> "+logFile+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	c := model.Cleaner{
		ID:         "sys-journal",
		Name:       "systemd journal",
		Scope:      model.ScopeSystem,
		Type:       model.TypeJournalVacuum,
		MinAgeDays: ptrInt(7),
	}

	dryEvs := captureEvents(t, func(emit func(model.Event)) {
		if err := NewJournalExecutor().Run(context.Background(), c, true, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(logFile); !os.IsNotExist(err) {
		t.Fatalf("dry-run must not invoke journalctl")
	}
	var sawRotatePreview, sawVacuumPreview bool
	for _, ev := range dryEvs {
		if ev.Event == model.EvCommandOutput {
			if strings.Contains(ev.Line, "--rotate") {
				sawRotatePreview = true
			}
			if strings.Contains(ev.Line, "--vacuum-time=7d") {
				sawVacuumPreview = true
			}
		}
	}
	if !sawRotatePreview || !sawVacuumPreview {
		t.Fatalf("dry-run preview missing rotate/vacuum lines: %+v", dryEvs)
	}

	realEvs := captureEvents(t, func(emit func(model.Event)) {
		if err := NewJournalExecutor().Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 || lines[0] != "--rotate" || lines[1] != "--vacuum-time=7d" {
		t.Fatalf("journalctl calls = %q, want [--rotate, --vacuum-time=7d]", lines)
	}
	for _, ev := range realEvs {
		if ev.Event == model.EvFinish {
			if !ev.MetricsIncomplete || ev.Status != "ok" {
				t.Fatalf("journal finish must report ok + MetricsIncomplete=true: %+v", ev)
			}
			return
		}
	}
	t.Fatal("missing finish event")
}
