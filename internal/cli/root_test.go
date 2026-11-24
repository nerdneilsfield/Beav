package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/i18n"
)

// TestNoArgsNonTTYPrintsHelp verifies that running without args in non-TTY prints help.
// TestNoArgsNonTTYPrintsHelp 验证在非 TTY 环境中无参数运行时打印帮助信息。
func TestNoArgsNonTTYPrintsHelp(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	for _, sub := range []string{"beav", "tui", "scan", "clean", "purge", "list", "info", "tags", "doctor", "history", "config", "analyze"} {
		if !strings.Contains(got, sub) {
			t.Errorf("help missing %q in %q", sub, got)
		}
	}
}

func TestRootHelpAndInvalidLangFlag(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangZH)
	cmd.SetArgs([]string{})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "交互") {
		t.Fatalf("expected ZH help output, got %q", buf.String())
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"list", "--lang", "invalid"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unsupported language") {
		t.Fatalf("expected invalid --lang error, got %v", err)
	}
}

func TestLocalizedCobraHelpTreeZH(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangZH)
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	rootHelp := buf.String()
	for _, want := range []string{"用法：", "核心清理命令：", "浏览与诊断命令：", "配置与工具命令：", "帮助主题：", "选项：", "查看任意命令的帮助信息", "显示 beav 的帮助信息"} {
		if !strings.Contains(rootHelp, want) {
			t.Fatalf("ZH root help missing %q in:\n%s", want, rootHelp)
		}
	}
	for _, forbidden := range []string{"Usage:", "Available Commands:", "Help about any command", "help for beav"} {
		if strings.Contains(rootHelp, forbidden) {
			t.Fatalf("ZH root help must not contain untranslated Cobra string %q in:\n%s", forbidden, rootHelp)
		}
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangZH)
	cmd.SetArgs([]string{"clean", "--help"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	cleanHelp := buf.String()
	for _, want := range []string{"用法：", "选项：", "全局选项：", "显示 clean 的帮助信息"} {
		if !strings.Contains(cleanHelp, want) {
			t.Fatalf("ZH clean help missing %q in:\n%s", want, cleanHelp)
		}
	}
	for _, forbidden := range []string{"Usage:", "Flags:", "Global Flags:", "help for clean"} {
		if strings.Contains(cleanHelp, forbidden) {
			t.Fatalf("ZH clean help must not contain untranslated Cobra string %q in:\n%s", forbidden, cleanHelp)
		}
	}

	buf.Reset()
	cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangZH)
	cmd.SetArgs([]string{"help", "clean"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "显示 clean 的帮助信息") {
		t.Fatalf("ZH `help clean` missing localized help flag description: %s", buf.String())
	}

	for _, topic := range []string{"safety", "exit-codes", "env", "examples"} {
		buf.Reset()
		cmd = NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
		cmd.SetArgs([]string{"help", topic})
		cmd.SetOut(&buf)
		cmd.SetErr(&buf)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("help %s failed: %v", topic, err)
		}
		if len(strings.TrimSpace(buf.String())) == 0 {
			t.Fatalf("help %s produced empty output", topic)
		}
	}
}
