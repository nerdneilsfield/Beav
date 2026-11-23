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
	for _, sub := range []string{"beav", "scan", "clean", "list", "info", "config", "analyze"} {
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
