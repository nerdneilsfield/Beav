package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigShowPrintsCleaners(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "show", "--config-dir", t.TempDir()})
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "cleaners") {
		t.Errorf("got %q", buf.String())
	}
}

func TestConfigInitCreatesAutoDefault(t *testing.T) {
	dir := t.TempDir()
	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "init", "--config-dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "output: auto") {
		t.Fatalf("config.yaml = %q", string(data))
	}
}

func TestConfigInitRefusesExistingWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("custom: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "init", "--config-dir", dir})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected existing config to be refused")
	}

	cmd = NewRootCmd("t", "t", "t")
	cmd.SetArgs([]string{"config", "init", "--config-dir", dir, "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "output: auto") {
		t.Fatalf("config.yaml = %q", string(data))
	}
}
