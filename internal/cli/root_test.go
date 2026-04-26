// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"bytes"
	"strings"
	"testing"
)

// TestNoArgsNonTTYPrintsHelp verifies that running without args in non-TTY prints help.
// TestNoArgsNonTTYPrintsHelp 验证在非 TTY 环境中无参数运行时打印帮助信息。
func TestNoArgsNonTTYPrintsHelp(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "beav") {
		t.Errorf("got %q", buf.String())
	}
}
