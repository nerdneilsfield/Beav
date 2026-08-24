package executor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

func TestToolPruneHomebrewUsesResolvedBinaryAndNativeDryRun(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	cache := filepath.Join(home, ".cache", "Homebrew")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	brew := filepath.Join(binDir, "brew")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\nprintf '%s\\n' \"$0 $*\" >> \"$HOME/invocations\"\nprintf 'home=%s\\n' \"$HOME\" >> \"$HOME/env\"\nprintf 'path=%s\\n' \"$PATH\" >> \"$HOME/env\"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{
		ID:         "brew",
		Scope:      model.ScopeUser,
		Type:       model.TypeToolPrune,
		MinAgeDays: ptrInt(5),
		ToolPrune:  &model.ToolPruneCfg{Profile: "pkg-homebrew"},
	}
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir))
		if err := e.Run(context.Background(), c, true, emit); err != nil {
			t.Fatal(err)
		}
	})
	invocation, err := os.ReadFile(filepath.Join(home, "invocations"))
	if err != nil {
		t.Fatalf("invocation missing: %v; events=%+v", err, evs)
	}
	if !strings.Contains(string(invocation), brew+" cleanup --prune=5 --dry-run") {
		t.Fatalf("invocation = %q", invocation)
	}
	env, err := os.ReadFile(filepath.Join(home, "env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "path="+binDir) {
		t.Fatalf("environment = %q", env)
	}
	for _, ev := range evs {
		if ev.Event == model.EvFinish {
			if !ev.MetricsIncomplete {
				t.Fatalf("native dry-run should report incomplete metrics: %+v", ev)
			}
			return
		}
	}
	t.Fatal("missing finish event")
}

func TestToolPruneUnsupportedDryRunNeverExecutes(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "uv")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	uv := filepath.Join(binDir, "uv")
	if err := os.WriteFile(uv, []byte("#!/bin/sh\ntouch \"$HOME/executed\"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{ID: "uv", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}}
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir))
		if err := e.Run(context.Background(), c, true, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(home, "executed")); !os.IsNotExist(err) {
		t.Fatalf("unsupported dry-run executed tool: %v", err)
	}
	if !hasCleanerSkip(evs, string(model.SkipReasonDryRunUnsupported)) {
		t.Fatalf("events = %+v", evs)
	}
	for _, ev := range evs {
		if ev.Event == model.EvFinish && !ev.MetricsIncomplete {
			t.Fatalf("unsupported dry-run should report incomplete metrics: %+v", ev)
		}
	}
}

func TestToolPruneBlocksWholeCommandWhenKeepListIntersectsRoot(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "uv")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	uv := filepath.Join(binDir, "uv")
	if err := os.WriteFile(uv, []byte("#!/bin/sh\ntouch \"$HOME/executed\"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "keep")
	c := model.Cleaner{ID: "uv", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}}
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, safety.NewWhitelist([]string{keep}), WithToolSearchPath(binDir))
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(home, "executed")); !os.IsNotExist(err) {
		t.Fatalf("keep-list intersection executed tool: %v", err)
	}
	if !hasCleanerSkip(evs, "keep_list_intersection") {
		t.Fatalf("events = %+v", evs)
	}
}

func TestToolPruneCapabilityVersionMismatchSkipsBeforeCleanup(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cache", "Homebrew"), 0o755); err != nil {
		t.Fatal(err)
	}
	brew := filepath.Join(binDir, "brew")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo unrelated-tool 1.0; else touch \"$HOME/executed\"; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{ID: "brew", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "pkg-homebrew"}}
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir))
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(home, "executed")); !os.IsNotExist(err) {
		t.Fatalf("version mismatch executed cleanup: %v", err)
	}
	if !hasCleanerSkip(evs, "version_mismatch") {
		t.Fatalf("events = %+v", evs)
	}
}

func TestToolPruneCommandFailurePropagatesAfterSuccessfulProbe(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cache", "Homebrew"), 0o755); err != nil {
		t.Fatal(err)
	}
	brew := filepath.Join(binDir, "brew")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo brew 4.0; exit 0; fi\nexit 17\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{ID: "brew", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "pkg-homebrew"}}
	var gotErr error
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir))
		gotErr = e.Run(context.Background(), c, false, emit)
	})
	if gotErr == nil {
		t.Fatal("expected cleanup command failure to propagate")
	}
	if !hasCleanerError(evs, "command_failed") {
		t.Fatalf("events = %+v", evs)
	}
}

func hasCleanerError(events []model.Event, reason string) bool {
	for _, event := range events {
		if event.Event == model.EvError && event.Reason == reason {
			return true
		}
	}
	return false
}
