// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestAnalyzeHelp verifies that the analyze command shows help text.
// TestAnalyzeHelp 验证 analyze 命令显示帮助文本。
func TestAnalyzeHelp(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"analyze", "--help"})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "Analyze") {
		t.Errorf("got %q", buf.String())
	}
}
