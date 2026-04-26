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

// TestConfigShowPrintsCleaners verifies that config show outputs cleaner list.
// TestConfigShowPrintsCleaners 验证 config show 输出清理器列表。
func TestConfigShowPrintsCleaners(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "show", "--config-dir", t.TempDir()})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "cleaners") {
		t.Errorf("got %q", buf.String())
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
