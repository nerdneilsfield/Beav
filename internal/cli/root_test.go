package cli

import (
	"bytes"
	"strings"
	"testing"
)

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
