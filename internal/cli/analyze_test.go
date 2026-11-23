package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/i18n"
)

// TestAnalyzeHelp verifies that the analyze command shows help text.
// TestAnalyzeHelp 验证 analyze 命令显示帮助文本。
func TestAnalyzeHelp(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmdWithLang("t", "t", "t", i18n.LangEN)
	cmd.SetArgs([]string{"analyze", "--help"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "read-only") {
		t.Errorf("expected read-only notice in analyze help, got %q", buf.String())
	}
}
