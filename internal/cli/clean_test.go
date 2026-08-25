// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/model"
	uicli "github.com/dengqi/beav/internal/ui/cli"
)

// TestCleanDryRunNoCleanersExitsZero verifies that clean with no cleaners exits zero.
// TestCleanDryRunNoCleanersExitsZero 验证没有清理器时 clean 命令正常退出。
func TestCleanDryRunNoCleanersExitsZero(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd("test", "test", "test")
	cmd.SetArgs([]string{"clean", "--dry-run", "--output", "json", "--config-dir", t.TempDir(), "--builtin-disabled", "--allow-root-home"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("err: %v out=%q", err, out.String())
	}
}

// TestChooseRendererAutoUsesPlainForNonTTY verifies renderer selection for non-TTY output.
// TestChooseRendererAutoUsesPlainForNonTTY 验证非 TTY 输出的渲染器选择。
func TestChooseRendererAutoUsesPlainForNonTTY(t *testing.T) {
	var out bytes.Buffer
	if _, ok := chooseRenderer("", "auto", &out).(*uicli.Plain); !ok {
		t.Fatal("auto output on non-TTY should use plain renderer")
	}
	if _, ok := chooseRenderer("", "spinner", &out).(*uicli.Spinner); !ok {
		t.Fatal("explicit config spinner should use spinner renderer")
	}
}

func TestOperationLogOnlyWritesForEffectiveTarget(t *testing.T) {
	if !shouldWriteOperationLog(os.Geteuid()) {
		t.Fatal("same-UID cleanup should keep operation logging")
	}
	if shouldWriteOperationLog(os.Geteuid() + 1) {
		t.Fatal("cross-UID cleanup must not create root-owned state in target home")
	}
}

// TestValidateCleanersForRunSkipsWrongScopeBeforePathValidation verifies scope-based filtering.
// TestValidateCleanersForRunSkipsWrongScopeBeforePathValidation 验证基于范围的过滤。
func TestValidateCleanersForRunSkipsWrongScopeBeforePathValidation(t *testing.T) {
	cleaners := []model.Cleaner{
		{
			ID:    "browser-firefox",
			Scope: model.ScopeUser,
			Type:  model.TypePaths,
			Paths: []string{"~/.cache/mozilla/firefox/*/cache2/**"},
		},
		{
			ID:    "system-cache",
			Scope: model.ScopeSystem,
			Type:  model.TypePaths,
			Paths: []string{"/tmp/beav-system-cache-*"},
		},
	}

	if err := validateCleanersForRun(cleaners, model.ScopeSystem, "", CleanFlags{}); err != nil {
		t.Fatal(err)
	}
}

func TestCleanCommandExposesIncludeHeavyFlag(t *testing.T) {
	cmd := NewCleanCmd()
	if cmd.Flags().Lookup("include-heavy") == nil {
		t.Fatal("missing --include-heavy flag")
	}
	if usage := cmd.Flags().Lookup("include-heavy").Usage; !strings.Contains(usage, "explicit") {
		t.Fatalf("heavy help must explain explicit authorization: %q", usage)
	}
	if usage := cmd.Flags().Lookup("only").Usage; !strings.Contains(usage, "exact full") {
		t.Fatalf("--only help must explain exact full-ID heavy authorization: %q", usage)
	}
	if usage := cmd.Flags().Lookup("yes").Usage; !strings.Contains(usage, "does not authorize heavy") {
		t.Fatalf("--yes help must state that it does not authorize heavy: %q", usage)
	}
}

func TestValidateCleanersForRunRejectsInvalidUnselectedCleaner(t *testing.T) {
	cleaners := []model.Cleaner{
		{ID: "valid", Scope: model.ScopeUser, Type: model.TypePaths, Paths: []string{"~/.cache/valid/*"}},
		{ID: "bad-command", Scope: model.ScopeSystem, Type: model.TypeCommand},
	}
	err := validateCleanersForRun(cleaners, model.ScopeUser, t.TempDir(), CleanFlags{Only: []string{"valid"}})
	if err == nil || !strings.Contains(err.Error(), "bad-command") {
		t.Fatalf("err = %v, want invalid unselected cleaner error", err)
	}
}

func TestCleanEngineRegistersClosedExecutors(t *testing.T) {
	home := t.TempDir()
	eng := newCleanEngine(home, os.Getuid(), nil)
	cases := []model.Cleaner{
		{ID: "tool", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}},
		{ID: "debris", Scope: model.ScopeUser, Type: model.TypeDebris, Debris: &model.DebrisCfg{Profile: "python-ruff-cache"}},
		{ID: "managed", Scope: model.ScopeUser, Type: model.TypeManagedCache, ManagedCache: &model.ManagedCacheCfg{Profile: "ai-torch-hub"}},
	}
	for _, cleaner := range cases {
		t.Run(string(cleaner.Type), func(t *testing.T) {
			var events []model.Event
			result, err := eng.Run(context.Background(), []model.Cleaner{cleaner}, engine.Options{
				Scope:   model.ScopeUser,
				Emitter: func(event model.Event) { events = append(events, event) },
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.CleanersErrored != 0 {
				t.Fatalf("registered executor errored: %+v events=%+v", result, events)
			}
			for _, event := range events {
				if event.Event == model.EvError && event.Reason == "internal" {
					t.Fatalf("executor missing for %q: %+v", cleaner.Type, events)
				}
			}
		})
	}
}

func TestConfigShowListsClosedExecutorTypes(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd("test", "test", "test")
	cmd.SetArgs([]string{"config", "show", "--config-dir", t.TempDir()})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, typ := range []string{"tool_prune", "debris", "managed_cache"} {
		if !strings.Contains(out.String(), typ) {
			t.Fatalf("cleaner listing missing %q", typ)
		}
	}
}
