package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/i18n"
)

// TestConfigShowPrintsConfigOverview verifies that config show displays config paths, defaults, and whitelist.
// TestConfigShowPrintsConfigOverview 验证 config show 输出配置文件路径、默认值与白名单信息。
func TestConfigShowPrintsConfigOverview(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"config", "show", "--config-dir", t.TempDir()})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, want := range []string{"Config file:", "Defaults:", "Overrides:", "Whitelist:", "beav list"} {
		if !strings.Contains(got, want) {
			t.Errorf("config show missing %q in %q", want, got)
		}
	}
}

// TestConfigDisableAndEnablePersist verifies that enable/disable write an
// override that actually removes a cleaner from the next run.
// TestConfigDisableAndEnablePersist 验证 enable/disable 写入的覆盖能真正让清理器退出下一次运行。
func TestConfigDisableAndEnablePersist(t *testing.T) {
	dir := t.TempDir()

	var buf bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
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
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
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
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
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

// TestConfigDisableRejectsUnknownID verifies a typo cannot silently succeed and suggests fixes.
// TestConfigDisableRejectsUnknownID 验证拼错的 ID 不会静默成功并提供纠错提示。
func TestConfigDisableRejectsUnknownID(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"config", "disable", "lang-pipx", "--config-dir", t.TempDir()})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("unknown cleaner id must be rejected")
	}
	if !strings.Contains(err.Error(), "beav list") || !strings.Contains(err.Error(), "lang-pip") {
		t.Fatalf("error must point at beav list and suggest lang-pip: %q", err.Error())
	}
}

// TestConfigInitCreatesAutoDefault verifies that config init creates a commented default config.
// TestConfigInitCreatesAutoDefault 验证 config init 创建带注释的默认配置。
func TestConfigInitCreatesAutoDefault(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"config", "init", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), filepath.Join(dir, "config.yaml")) {
		t.Fatalf("expected created path message, got %q", buf.String())
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "output: auto") || !strings.Contains(string(data), "overrides:") {
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
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"config", "init", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected existing config to be refused")
	}
	if !strings.Contains(err.Error(), "--force") || !strings.Contains(err.Error(), "beav config edit") {
		t.Fatalf("expected helpful overwrite error, got %q", err.Error())
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
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

func TestConfigWhitelistAddListRemove(t *testing.T) {
	dir := t.TempDir()
	var buf bytes.Buffer

	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"config", "whitelist", "add", "~/.cache/keep-me", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "~/.cache/keep-me") {
		t.Fatalf("add output = %q", buf.String())
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"config", "whitelist", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "~/.cache/keep-me") {
		t.Fatalf("list output = %q", buf.String())
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"config", "whitelist", "remove", "~/.cache/keep-me", "--config-dir", dir})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Removed") {
		t.Fatalf("remove output = %q", buf.String())
	}
}
