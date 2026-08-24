package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
)

func TestBuiltinRegistryEngineToolProfileIntegration(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("temporary user-profile identity seam requires non-root test user")
	}
	binDir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cache", "pypoetry"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".local", "share", "pnpm", "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	pnpm := filepath.Join(binDir, "pnpm")
	if err := os.WriteFile(pnpm, []byte("#!/bin/sh\nprintf 'args=%s\\n' \"$*\" >> \"$HOME/task5-tool.log\"\nprintf 'cwd=%s\\n' \"$(pwd)\" >> \"$HOME/task5-tool.log\"\nprintf 'home=%s\\n' \"$HOME\" >> \"$HOME/task5-tool.log\"\nprintf 'runtime=%s\\n' \"$XDG_RUNTIME_DIR\" >> \"$HOME/task5-tool.log\"\nprintf 'pnpm 9.0\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	poetry := filepath.Join(binDir, "poetry")
	if err := os.WriteFile(poetry, []byte("#!/bin/sh\nprintf 'args=%s\\n' \"$*\" >> \"$HOME/task5-tool.log\"\nprintf 'cwd=%s\\n' \"$(pwd)\" >> \"$HOME/task5-tool.log\"\nprintf 'home=%s\\n' \"$HOME\" >> \"$HOME/task5-tool.log\"\nprintf 'runtime=%s\\n' \"$XDG_RUNTIME_DIR\" >> \"$HOME/task5-tool.log\"\nif [ \"$1\" = \"--version\" ]; then printf 'Poetry (version 1.8)\\n'; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}

	var safeCleaner, heavyCleaner model.Cleaner
	for _, loaded := range mustLoadBuiltinForTask5(t) {
		switch loaded.Cleaner.ID {
		case "lang-pnpm":
			safeCleaner = loaded.Cleaner
		case "lang-poetry-cache":
			heavyCleaner = loaded.Cleaner
		}
	}
	if safeCleaner.ID == "" || heavyCleaner.ID == "" {
		t.Fatal("required safe/heavy builtin profile missing")
	}
	tool := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir), tempToolIdentityOption(t, home))
	eng := engine.New(engine.WithExecutor(model.TypeToolPrune, tool))
	var safeEvents []model.Event
	safeResult, err := eng.Run(context.Background(), []model.Cleaner{safeCleaner}, engine.Options{
		Scope:   model.ScopeUser,
		Emitter: func(event model.Event) { safeEvents = append(safeEvents, event) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if safeResult.CleanersRun != 1 || safeResult.CleanersErrored != 0 {
		t.Fatalf("safe result=%+v events=%+v", safeResult, safeEvents)
	}
	safeLog, err := os.ReadFile(filepath.Join(home, "task5-tool.log"))
	if err != nil {
		t.Fatal(err)
	}
	var dryRunEvents []model.Event
	dryRunResult, err := eng.Run(context.Background(), []model.Cleaner{safeCleaner}, engine.Options{
		Scope:  model.ScopeUser,
		DryRun: true,
		Emitter: func(event model.Event) {
			dryRunEvents = append(dryRunEvents, event)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if dryRunResult.CleanersSkipped != 1 || dryRunResult.CleanersRun != 0 {
		t.Fatalf("safe dry-run result=%+v events=%+v", dryRunResult, dryRunEvents)
	}
	postDryRunLog, err := os.ReadFile(filepath.Join(home, "task5-tool.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(postDryRunLog) != string(safeLog) {
		t.Fatalf("safe dry-run invoked fake tool: before=%q after=%q", safeLog, postDryRunLog)
	}

	cases := []struct {
		name         string
		scope        model.Scope
		only         []string
		includeHeavy bool
		wantRun      bool
	}{
		{name: "tag cannot authorize", scope: model.ScopeUser, only: []string{"tools"}},
		{name: "prefix cannot authorize", scope: model.ScopeUser, only: []string{"lang-poetry"}},
		{name: "all scope cannot authorize", scope: model.ScopeAll},
		{name: "exact full ID authorizes", scope: model.ScopeUser, only: []string{"lang-poetry-cache"}, wantRun: true},
		{name: "include heavy authorizes", scope: model.ScopeUser, includeHeavy: true, wantRun: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var events []model.Event
			result, err := eng.Run(context.Background(), []model.Cleaner{heavyCleaner}, engine.Options{
				Scope:        tc.scope,
				Only:         tc.only,
				IncludeHeavy: tc.includeHeavy,
				Emitter:      func(event model.Event) { events = append(events, event) },
			})
			if err != nil {
				t.Fatal(err)
			}
			if tc.wantRun {
				if result.CleanersRun != 1 || result.CleanersErrored != 0 {
					t.Fatalf("result=%+v events=%+v", result, events)
				}
				return
			}
			if result.CleanersSkipped != 1 {
				t.Fatalf("result=%+v events=%+v", result, events)
			}
			for _, event := range events {
				if event.Event == model.EvCleanerSkipped && event.Reason == string(model.SkipReasonHeavyNotAuthorized) {
					return
				}
			}
			t.Fatalf("events=%+v, missing heavy_not_authorized", events)
		})
	}

	log, err := os.ReadFile(filepath.Join(home, "task5-tool.log"))
	if err != nil {
		t.Fatal(err)
	}
	output := string(log)
	if !strings.Contains(output, "args=store prune") {
		t.Fatalf("safe fake tool argv = %q", output)
	}
	if !strings.Contains(output, "args=cache clear pypi --all --no-interaction") {
		t.Fatalf("fake tool argv = %q", output)
	}
	if !strings.Contains(output, "cwd="+home) || !strings.Contains(output, "home="+home) {
		t.Fatalf("fake tool identity/cwd = %q", output)
	}
	if !strings.Contains(output, fmt.Sprintf("runtime=/run/user/%d", os.Getuid())) {
		t.Fatalf("fake tool target UID environment = %q", output)
	}
}

func mustLoadBuiltinForTask5(t *testing.T) []registry.Loaded {
	t.Helper()
	loaded, err := registry.LoadBuiltin(cleaners.Builtin)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}
