package executor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

func TestPathsExecutorDeletesAgedFiles(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, ".cache", "demo")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(cache, "old.bin")
	newer := filepath.Join(cache, "new.bin")
	if err := os.WriteFile(old, make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, make([]byte, 1024), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(old, when, when); err != nil {
		t.Fatal(err)
	}

	c := model.Cleaner{
		ID:         "demo",
		Name:       "demo",
		Scope:      model.ScopeUser,
		Type:       model.TypePaths,
		MinAgeDays: ptrInt(7),
		Paths:      []string{cache},
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		exec := NewPathsExecutor(home, safety.NewWhitelist(nil))
		if err := exec.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})

	if !hasDelete(events, old) {
		t.Errorf("expected old file to be deleted; events: %+v", events)
	}
	if hasDelete(events, newer) {
		t.Errorf("new file should not be deleted")
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old file still exists or unexpected stat error: %v", err)
	}
	if _, err := os.Stat(newer); err != nil {
		t.Errorf("new file should still exist: %v", err)
	}
}

func TestRunRootHandlesSafeRootItself(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old.bin")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(old, when, when); err != nil {
		t.Fatal(err)
	}

	c := model.Cleaner{ID: "demo", Scope: model.ScopeSystem, Type: model.TypePaths, MinAgeDays: ptrInt(7)}
	exec := NewPathsExecutor("", safety.NewWhitelist(nil))
	var processed []string
	process := func(w *safety.Walker, entries []safety.Entry) {
		for _, e := range entries {
			processed = append(processed, filepath.Join(w.Root(), e.RelPath))
		}
	}
	var errs int
	exec.runRoot(c, root, root, 7, safety.TimeFieldMtime, process, func(model.Event) {}, &errs)
	if errs != 0 {
		t.Fatalf("errs = %d", errs)
	}
	if len(processed) != 1 || processed[0] != old {
		t.Fatalf("processed = %v, want [%s]", processed, old)
	}
}

func TestPathsExecutorMarksExcludeAsExcluded(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, ".cache", "demo")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(cache, "keep.log")
	if err := os.WriteFile(old, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(old, when, when); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{
		ID:         "demo",
		Scope:      model.ScopeUser,
		Type:       model.TypePaths,
		MinAgeDays: ptrInt(7),
		Paths:      []string{cache},
		Exclude:    []string{"*.log"},
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := NewPathsExecutor(home, safety.NewWhitelist(nil)).Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasSkip(events, "excluded") {
		t.Fatalf("expected excluded skip; events: %+v", events)
	}
}

func TestPathsExecutorSkipsCleanerWhenGlobHasNoMatches(t *testing.T) {
	home := t.TempDir()
	c := model.Cleaner{
		ID:         "demo",
		Scope:      model.ScopeUser,
		Type:       model.TypePaths,
		MinAgeDays: ptrInt(7),
		Paths:      []string{"~/.cache/missing/*"},
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := NewPathsExecutor(home, safety.NewWhitelist(nil)).Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "no_matches") {
		t.Fatalf("expected no_matches cleaner skip; events: %+v", events)
	}
}

func TestExpandRootsReturnsInvalidGlobError(t *testing.T) {
	home := t.TempDir()
	_, err := NewPathsExecutor(home, nil).expandRoots(model.Cleaner{
		ID:    "bad",
		Scope: model.ScopeUser,
		Type:  model.TypePaths,
		Paths: []string{"~/.cache/["},
	})
	if err == nil || !strings.Contains(err.Error(), "invalid glob") {
		t.Fatalf("err = %v, want invalid glob", err)
	}
	if errors.Is(err, errNoMatches) {
		t.Fatalf("invalid glob should not be reported as no matches")
	}
}
