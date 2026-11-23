// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/config"
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

// TestCleanDryRunNeverDeletes verifies that --dry-run reports candidates but
// leaves every file on disk. This guards the dry-run integrity promise after a
// regression passed a hard-coded non-dry pass.
// TestCleanDryRunNeverDeletes 验证 --dry-run 只报告候选而不动磁盘上的任何文件。
// 此测试保护 dry-run 的完整性承诺。
func TestCleanDryRunNeverDeletes(t *testing.T) {
	target := t.TempDir()
	path := filepath.Join(target, "old.txt")
	if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -30)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}

	cfgDir := t.TempDir()
	cleanersDir := filepath.Join(cfgDir, "cleaners.d")
	if err := os.MkdirAll(cleanersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := "- id: drytest-cache\n  name: Drytest Cache\n  scope: user\n  type: paths\n  min_age_days: 7\n  paths:\n    - " + filepath.Join(target, "**") + "\n  tags: [drytest]\n"
	if err := os.WriteFile(filepath.Join(cleanersDir, "drytest.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"clean", "--dry-run", "--output", "json", "--only", "drytest-cache", "--config-dir", cfgDir, "--builtin-disabled", "--allow-root-home"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("err: %v out=%q", err, out.String())
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "keep" {
		t.Fatalf("dry run must not delete the file: read err=%v", err)
	}
	if !strings.Contains(out.String(), `"files_candidate":1`) {
		t.Fatalf("expected one candidate in the plan output: %q", out.String())
	}
}

// TestConfirmNeededSkipsDryRunYesAndJSON verifies which flag combinations
// bypass the interactive preview.
// TestConfirmNeededSkipsDryRunYesAndJSON 验证哪些标志组合会绕过交互式预演。
func TestConfirmNeededSkipsDryRunYesAndJSON(t *testing.T) {
	cfg := &config.Config{}
	if confirmNeeded(CleanFlags{DryRun: true}, cfg, &bytes.Buffer{}) {
		t.Fatal("dry run must not ask for confirmation")
	}
	if confirmNeeded(CleanFlags{Yes: true}, cfg, &bytes.Buffer{}) {
		t.Fatal("--yes must not ask for confirmation")
	}
	if confirmNeeded(CleanFlags{Output: "json"}, cfg, &bytes.Buffer{}) {
		t.Fatal("json output is for scripts and must not ask for confirmation")
	}
	if confirmNeeded(CleanFlags{}, cfg, &bytes.Buffer{}) {
		t.Fatal("non-terminal stdout must not ask for confirmation")
	}
}

// TestConfirmDeletionAcceptsExplicitYes verifies the prompt answer parsing.
// TestConfirmDeletionAcceptsExplicitYes 验证确认提示的回答解析。
func TestConfirmDeletionAcceptsExplicitYes(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  bool
	}{
		{"y", "y\n", true},
		{"yes", "YES\n", true},
		{"n", "n\n", false},
		{"empty default is no", "\n", false},
		{"eof is no", "", false},
		{"anything else is no", "why not\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			got := confirmDeletion(strings.NewReader(tc.input), &out, engine.Result{FilesCandidate: 3, BytesCandidate: 3000}, passInfo{native: []string{"lang-uv"}})
			if got != tc.want {
				t.Fatalf("confirmDeletion(%q) = %v, want %v", tc.input, got, tc.want)
			}
			if !strings.Contains(out.String(), "Proceed with deletion? [y/N]") {
				t.Fatalf("missing prompt: %q", out.String())
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
