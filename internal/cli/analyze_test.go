package cli

import (
	"bytes"
	"strings"
	"testing"
)

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
