package cli

import (
	"bytes"
	"testing"
)

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
