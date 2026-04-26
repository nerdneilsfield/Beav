package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/executor"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

func TestFakeHomeEndToEnd(t *testing.T) {
	home := t.TempDir()
	mk := func(rel, content string, age time.Duration) string {
		t.Helper()
		p := filepath.Join(home, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		when := time.Now().Add(age)
		if err := os.Chtimes(p, when, when); err != nil {
			t.Fatal(err)
		}
		return p
	}

	old := mk(".cache/Code/Cache/old", strings.Repeat("x", 4096), -30*24*time.Hour)
	newer := mk(".cache/Code/Cache/new", "x", -1*time.Hour)

	minAge := 7
	cleaners := []model.Cleaner{{
		ID:         "editor-vscode",
		Name:       "vscode",
		Scope:      model.ScopeUser,
		Type:       model.TypePaths,
		MinAgeDays: &minAge,
		Paths:      []string{filepath.Join(home, ".cache", "Code", "Cache", "*")},
	}}
	for _, c := range cleaners {
		if err := registry.Validate(c); err != nil {
			t.Fatal(err)
		}
	}

	en := engine.New(engine.WithExecutor(model.TypePaths, executor.NewPathsExecutor(home, safety.NewWhitelist(nil))))
	res, err := en.Run(context.Background(), cleaners, engine.Options{
		Scope:   model.ScopeUser,
		Emitter: func(model.Event) {},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanersRun != 1 {
		t.Errorf("ran %d", res.CleanersRun)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Errorf("old still exists: %v", err)
	}
	if _, err := os.Stat(newer); err != nil {
		t.Errorf("new should exist: %v", err)
	}
}
