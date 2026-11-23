// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConfigShowPrintsCleaners verifies that config show outputs a table of cleaners.
// TestConfigShowPrintsCleaners 验证 config show 输出清理器表格。
func TestConfigShowPrintsCleaners(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "show", "--config-dir", t.TempDir()})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "ID") || !strings.Contains(got, "STATE") {
		t.Errorf("missing table header: %q", got)
	}
	if !strings.Contains(got, "lang-pip") {
		t.Errorf("table must list builtin cleaner IDs so users can target them: %q", got)
	}
}

// TestConfigDisableAndEnablePersist verifies that enable/disable write an
// override that actually removes a cleaner from the next run.
// TestConfigDisableAndEnablePersist 验证 enable/disable 写入的覆盖能真正让清理器退出下一次运行。
func TestConfigDisableAndEnablePersist(t *testing.T) {
	dir := t.TempDir()

	var buf bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "disable", "lang-pip", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "lang-pip is now disabled by default") {
		t.Fatalf("got %q", buf.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "enabled: false") {
		t.Fatalf("override not persisted: %q", string(data))
	}

	buf.Reset()
	cmd = NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"clean", "--dry-run", "--output", "json", "--only", "lang-pip", "--config-dir", dir, "--allow-root-home"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), `"cleaner_id":"lang-pip"`) {
		t.Fatalf("disabled cleaner must not run: %q", buf.String())
	}

	buf.Reset()
	cmd = NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "enable", "lang-pip", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "enabled: true") {
		t.Fatalf("override not flipped back: %q", string(data))
	}
}

// TestConfigDisableRejectsUnknownID verifies a typo cannot silently succeed.
// TestConfigDisableRejectsUnknownID 验证拼错的 ID 不会静默成功。
func TestConfigDisableRejectsUnknownID(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "disable", "lang-typo", "--config-dir", t.TempDir()})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err == nil {
		t.Fatal("unknown cleaner id must be rejected")
	}
	if !strings.Contains(buf.String(), "beav config show") {
		t.Fatalf("error must point at the ID list: %q", buf.String())
	}
}

// TestConfigInitCreatesAutoDefault verifies that config init creates a default config.
// TestConfigInitCreatesAutoDefault 验证 config init 创建默认配置。
func TestConfigInitCreatesAutoDefault(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "init", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "output: auto") {
		t.Fatalf("config.yaml = %q", string(data))
	}
}

// TestConfigInitRefusesExistingWithoutForce verifies that init refuses to overwrite existing config.
// TestConfigInitRefusesExistingWithoutForce 验证 init 拒绝覆盖现有配置。
func TestConfigInitRefusesExistingWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("custom: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "init", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected existing config to be refused")
	}

	buf.Reset()
	cmd = NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "init", "--config-dir", dir, "--force"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "output: auto") {
		t.Fatalf("config.yaml = %q", string(data))
	}
}
