package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/i18n"
)

// TestVersionPrintsVersion verifies that the version command outputs rich details, --short, --json, and root -V.
// TestVersionPrintsVersion 验证 version 命令输出完整详情、--short、--json 以及根命令 -V/--version。
func TestVersionPrintsVersion(t *testing.T) {
	var out bytes.Buffer
	cmd := NewVersionCmd("0.1.0", "abcdef0", "2026-04-26", i18n.New(i18n.LangEN))
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"beav 0.1.0", "abcdef0", "2026-04-26", "Go:", "Platform:", "Cleaners:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("version output missing %q in:\n%s", want, got)
		}
	}

	out.Reset()
	cmd = NewVersionCmd("0.1.0", "abcdef0", "2026-04-26", i18n.New(i18n.LangEN))
	cmd.SetArgs([]string{"--short"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "0.1.0\n" {
		t.Fatalf("short version = %q, want %q", out.String(), "0.1.0\n")
	}

	out.Reset()
	cmd = NewVersionCmd("0.1.0", "abcdef0", "2026-04-26", i18n.New(i18n.LangEN))
	cmd.SetArgs([]string{"--json"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		t.Fatalf("version json invalid: %v\n%s", err, out.String())
	}
	if parsed["version"] != "0.1.0" || parsed["commit"] != "abcdef0" {
		t.Fatalf("unexpected json payload: %+v", parsed)
	}

	out.Reset()
	root := NewRootCmdWithLang("0.1.0", "abcdef0", "2026-04-26", i18n.LangEN)
	root.SetArgs([]string{"-V"})
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "beav 0.1.0 (abcdef0, 2026-04-26)\n" {
		t.Fatalf("root -V = %q", out.String())
	}
}
