package cli

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/i18n"
	uicli "github.com/dengqi/beav/internal/ui/cli"
	"github.com/dengqi/beav/internal/ui/tui"
)

// TestCleanDryRunNoCleanersExitsZero verifies that clean with no cleaners exits zero.
// TestCleanDryRunNoCleanersExitsZero 验证没有清理器时 clean 命令正常退出。
func TestCleanDryRunNoCleanersExitsZero(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmdWithLang("test", "test", "test", i18n.LangEN)
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
	r := chooseRenderer("", "spinner", &out)
	sp, ok := r.(*uicli.Spinner)
	if !ok {
		t.Fatal("explicit config spinner should use spinner renderer")
	}
	_ = sp.Close()
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
	cmd := NewCleanCmd(i18n.New(i18n.LangEN))
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
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
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

func TestScanCommandIsAlwaysDryRunAndNeverDeletes(t *testing.T) {
	target := t.TempDir()
	path := filepath.Join(target, "stale.bin")
	if err := os.WriteFile(path, []byte("keep-me"), 0o644); err != nil {
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
	def := "- id: scantest-cache\n  name: Scantest Cache\n  scope: user\n  type: paths\n  min_age_days: 7\n  paths:\n    - " + filepath.Join(target, "**") + "\n  tags: [scantest]\n"
	if err := os.WriteFile(filepath.Join(cleanersDir, "scantest.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"scan", "--only", "scantest-cache", "--verbose", "--config-dir", cfgDir, "--builtin-disabled", "--allow-root-home"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("err: %v out=%q", err, out.String())
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "keep-me" {
		t.Fatalf("scan must never delete the file: err=%v", err)
	}
	got := out.String()
	if !strings.Contains(got, "scantest-cache") || !strings.Contains(got, "stale.bin") {
		t.Fatalf("scan verbose output missing expected cleaner or sample file: %q", got)
	}
}

func TestDiagnoseSelectorsUnknownWrongScopeAndDisabled(t *testing.T) {
	disabled := false
	cleaners := []model.Cleaner{
		{ID: "lang-pip", Name: "pip wheel cache", Scope: model.ScopeUser, Tags: []string{"langs", "python"}},
		{ID: "pkg-apt-archives", Name: "apt", Scope: model.ScopeSystem, Tags: []string{"pkg"}},
		{ID: "editor-vscode", Name: "VS Code", Scope: model.ScopeUser, Enabled: &disabled},
	}
	tr := i18n.New(i18n.LangEN)

	_, err := diagnoseSelectors(cleaners, model.ScopeUser, []string{"lang-pipx"}, tr)
	var cliErr ExitError
	if !errors.As(err, &cliErr) || cliErr.Code() != 1 {
		t.Fatalf("expected ExitError(1) for unknown selector, got %v", err)
	}
	if !strings.Contains(err.Error(), "lang-pip") || !strings.Contains(err.Error(), "beav list") {
		t.Fatalf("expected suggestion and list hint, got %q", err.Error())
	}

	_, err = diagnoseSelectors(cleaners, model.ScopeUser, []string{"pkg-apt-archives"}, tr)
	if !errors.As(err, &cliErr) || cliErr.Code() != 1 || !strings.Contains(err.Error(), "--system") {
		t.Fatalf("expected --system hint for wrong scope, got %v", err)
	}

	warnings, err := diagnoseSelectors(cleaners, model.ScopeUser, []string{"editor-vscode"}, tr)
	if err != nil || len(warnings) != 1 || !strings.Contains(warnings[0], "beav config enable editor-vscode") {
		t.Fatalf("expected disabled warning, got err=%v warnings=%v", err, warnings)
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
			got := confirmDeletion(strings.NewReader(tc.input), &out, engine.Result{FilesCandidate: 3, BytesCandidate: 3000}, passInfo{
				native:     []string{"lang-uv"},
				unmeasured: []string{"pkg-homebrew"},
			})
			if got != tc.want {
				t.Fatalf("confirmDeletion(%q) = %v, want %v", tc.input, got, tc.want)
			}
			if !strings.Contains(out.String(), "Proceed with deletion? [y/N]") {
				t.Fatalf("missing prompt: %q", out.String())
			}
			if !strings.Contains(out.String(), "pkg-homebrew") {
				t.Fatalf("missing unmeasured native warning: %q", out.String())
			}
		})
	}
}

func TestFilterOutNativeExcludesDuplicates(t *testing.T) {
	got := filterOutNative([]string{"lang-uv", "pkg-homebrew"}, []string{"lang-uv"})
	if len(got) != 1 || got[0] != "pkg-homebrew" {
		t.Fatalf("filterOutNative = %v, want [pkg-homebrew]", got)
	}
}

func TestShouldAbortEarlyAccountsForMetricsIncomplete(t *testing.T) {
	if !shouldAbortEarly(engine.Result{}, passInfo{}) {
		t.Fatal("empty result with no native/unmeasured cleaners should abort early")
	}
	if shouldAbortEarly(engine.Result{MetricsIncomplete: true}, passInfo{}) {
		t.Fatal("metrics_incomplete native cleaner must not abort early as Nothing to delete")
	}
	if shouldAbortEarly(engine.Result{}, passInfo{native: []string{"lang-uv"}}) {
		t.Fatal("native cleaner without preview must not abort early")
	}
}

func TestRunReadOnlyScanStreamsCandidatesWithoutDeleting(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, ".cache", "demo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	safeFile := filepath.Join(target, "safe.cache")
	if err := os.WriteFile(safeFile, []byte("123456"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().AddDate(0, 0, -20)
	if err := os.Chtimes(safeFile, old, old); err != nil {
		t.Fatal(err)
	}

	minAge := 7
	merged := []model.Cleaner{
		{
			ID:         "safe-demo",
			Name:       "Safe Demo",
			Scope:      model.ScopeUser,
			Type:       model.TypePaths,
			Impact:     model.ImpactSafe,
			MinAgeDays: &minAge,
			Paths:      []string{filepath.Join(target, "**")},
		},
		{
			ID:         "heavy-demo",
			Name:       "Heavy Demo",
			Scope:      model.ScopeUser,
			Type:       model.TypePaths,
			Impact:     model.ImpactHeavy,
			MinAgeDays: &minAge,
			Paths:      []string{filepath.Join(target, "**")},
		},
	}

	ch, cancel := runReadOnlyScan(context.Background(), merged, model.ScopeUser, home, os.Getuid(), &config.Config{})
	defer cancel()

	var items []tui.CleanerScan
	var done bool
	for p := range ch {
		if p.Item != nil {
			items = append(items, *p.Item)
		}
		if p.Done {
			done = true
		}
	}
	if !done || len(items) != 2 {
		t.Fatalf("done=%v items=%d, want 2 scanned items", done, len(items))
	}
	for _, it := range items {
		if it.FilesCandidate != 1 || it.BytesCandidate != 6 {
			t.Fatalf("unexpected candidate metrics for %s: %+v", it.Cleaner.ID, it)
		}
	}
	if _, err := os.Stat(safeFile); err != nil {
		t.Fatalf("read-only scan must not delete file: %v", err)
	}
}

func TestListListsClosedExecutorTypes(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmdWithLang("test", "test", "test", i18n.LangEN)
	cmd.SetArgs([]string{"list", "--config-dir", t.TempDir()})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, id := range []string{"lang-uv", "python-ruff-cache", "ai-torch-hub", "lang-rustup"} {
		if !strings.Contains(got, id) {
			t.Fatalf("cleaner listing missing %q", id)
		}
	}
	if !strings.Contains(got, "IMPACT") || !strings.Contains(got, "heavy") {
		t.Fatalf("list output missing IMPACT column or heavy badge: %q", got)
	}
}
