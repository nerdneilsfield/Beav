package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/i18n"
)

func TestListReflectsAgeOverridesAndFilters(t *testing.T) {
	dir := t.TempDir()
	cfg := "defaults:\n  min_age_days: 14\noverrides:\n  lang-pip:\n    min_age_days: 3\n  editor-vscode:\n    enabled: false\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"list", "--config-dir", dir, "--tag", "python"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "lang-pip") || !strings.Contains(got, "3d") {
		t.Fatalf("expected overridden 3d age for lang-pip: %q", got)
	}
	if strings.Contains(got, "editor-vscode") {
		t.Fatalf("--tag python must exclude editor-vscode: %q", got)
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"list", "--config-dir", dir, "--disabled"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "editor-vscode") || strings.Contains(buf.String(), "lang-pip") {
		t.Fatalf("--disabled filter mismatch: %q", buf.String())
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"list", "--config-dir", dir, "--heavy", "--lang", "zh"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "lang-rustup") || !strings.Contains(buf.String(), "重度") {
		t.Fatalf("--heavy --lang zh output mismatch: %q", buf.String())
	}
}

func TestInfoShowsDetailsAndSuggestsOnTypo(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer

	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"info", "lang-rustup", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "lang-rustup") || !strings.Contains(got, "heavy") || !strings.Contains(got, "beav scan --only lang-rustup") {
		t.Fatalf("info output incomplete: %q", got)
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangZH)
	cmd.SetArgs([]string{"info", "lang-rustu", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for unknown cleaner ID")
	}
	if !strings.Contains(err.Error(), "lang-rustup") || !strings.Contains(err.Error(), "beav list") {
		t.Fatalf("expected suggestion for typo, got %q", err.Error())
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangZH)
	cmd.SetArgs([]string{"info", "lang-rustup", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	zhInfo := buf.String()
	if !strings.Contains(zhInfo, "Rust 工具链 (rustup_prune)") || !strings.Contains(zhInfo, "修改时间 mtime") {
		t.Fatalf("ZH info missing localized executor type or time field: %q", zhInfo)
	}
	if strings.Contains(zhInfo, "中英文说明") {
		t.Fatalf("ZH info must not contain stale phrasing: %q", zhInfo)
	}
}

func TestScanCommandHonorsLangFlag(t *testing.T) {
	target := t.TempDir()
	path := filepath.Join(target, "stale.bin")
	if err := os.WriteFile(path, []byte("keep-me"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfgDir := t.TempDir()
	cleanersDir := filepath.Join(cfgDir, "cleaners.d")
	if err := os.MkdirAll(cleanersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	def := "- id: scantest-cache\n  name: Scantest Cache\n  scope: user\n  type: paths\n  min_age_days: 0\n  paths:\n    - " + filepath.Join(target, "**") + "\n"
	if err := os.WriteFile(filepath.Join(cleanersDir, "scantest.yaml"), []byte(def), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"scan", "--lang", "zh", "--only", "scantest-cache", "--config-dir", cfgDir, "--builtin-disabled", "--allow-root-home"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("err: %v out=%q", err, out.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("scan must never delete files: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "只读扫描") || !strings.Contains(got, "总计：") {
		t.Fatalf("scan --lang zh output not localized: %q", got)
	}
}
