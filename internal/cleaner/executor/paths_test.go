package executor

import (
	"context"
	"os"
	"path/filepath"
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
