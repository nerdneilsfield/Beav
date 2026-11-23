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
}
