// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"bytes"
	"testing"
)

// TestVersionPrintsVersion verifies that the version command outputs correct format.
// TestVersionPrintsVersion 验证 version 命令输出正确的格式。
func TestVersionPrintsVersion(t *testing.T) {
	var out bytes.Buffer
	cmd := NewVersionCmd("0.1.0", "abcdef0", "2026-04-26")
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	want := "beav 0.1.0 (abcdef0, 2026-04-26)\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
