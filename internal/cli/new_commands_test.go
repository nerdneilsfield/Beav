package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/oplog"
)

func TestDoctorTagsHistoryAndConfigCommands(t *testing.T) {
	cfgDir := t.TempDir()

	// config path
	var out bytes.Buffer
	cmd := NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"config", "--config-dir", cfgDir, "path"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), cfgDir) {
		t.Fatalf("config path missing %q: %q", cfgDir, out.String())
	}

	// config safety set & allow-add
	out.Reset()
	cmd = NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"config", "--config-dir", cfgDir, "safety", "set", "allow_command_cleaners", "true"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "allow_command_cleaners = true") {
		t.Fatalf("unexpected safety set output: %q", out.String())
	}

	out.Reset()
	cmd = NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"config", "--config-dir", cfgDir, "safety", "allow-add", "/opt/custom-cache"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	cmd = NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"config", "--config-dir", cfgDir, "safety", "show"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "/opt/custom-cache") {
		t.Fatalf("safety show missing custom allow_list: %q", out.String())
	}

	// config validate
	out.Reset()
	cmd = NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"config", "--config-dir", cfgDir, "validate"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "valid") {
		t.Fatalf("config validate output: %q", out.String())
	}

	// tags
	out.Reset()
	cmd = NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"tags", "--config-dir", cfgDir})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "langs") {
		t.Fatalf("tags output missing langs: %q", out.String())
	}

	// doctor --json
	out.Reset()
	cmd = NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"doctor", "--config-dir", cfgDir, "--json"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("doctor json error: %v", err)
	}
	if doc["config_valid"] != true {
		t.Fatalf("expected config_valid=true in doctor report: %+v", doc)
	}

	// history using temp HOME
	fakeHome := t.TempDir()
	t.Setenv("HOME", fakeHome)
	t.Setenv("BEAV_NO_OPLOG", "")
	logPath := filepath.Join(fakeHome, ".local", "state", "beav", "operations.log")
	logger, err := oplog.New(logPath, 1024*1024, 2)
	if err != nil {
		t.Fatal(err)
	}
	_ = logger.Write("delete", filepath.Join(fakeHome, ".cache", "a.bin"), 2048, "lang-pip")
	_ = logger.Close()

	out.Reset()
	cmd = NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"history"})
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "lang-pip") || !strings.Contains(out.String(), "2.0 kB") {
		t.Fatalf("history output unexpected: %q", out.String())
	}
}

func TestPurgeAndUnsafeCommandCleaner(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDir := filepath.Join(home, ".config", "beav")
	targetDir := filepath.Join(home, "Documents", "scratch-cache")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	targetFile := filepath.Join(targetDir, "temp.dat")
	if err := os.WriteFile(targetFile, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-24 * time.Hour)
	_ = os.Chtimes(targetFile, old, old)

	// Default purge rejects blacklisted ~/Documents path.
	var out bytes.Buffer
	cmd := NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"purge", targetFile, "--config-dir", cfgDir, "--yes"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected blacklisted ~/Documents path to be rejected without bypass")
	}

	// Purge with --unsafe deletes the file.
	out.Reset()
	cmd = NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"purge", targetFile, "--config-dir", cfgDir, "--unsafe", "--yes"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("purge --unsafe failed: %v out=%q", err, out.String())
	}
	if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
		t.Fatalf("expected targetFile to be deleted, stat err=%v", err)
	}

	// Custom type: command cleaner works with --unsafe.
	cleanersDir := filepath.Join(cfgDir, "cleaners.d")
	if err := os.MkdirAll(cleanersDir, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(home, "cmd-ran.txt")
	yamlDef := "- id: custom-echo\n  name: Custom Echo\n  scope: user\n  type: command\n  command: \"echo done > " + marker + "\"\n"
	if err := os.WriteFile(filepath.Join(cleanersDir, "cmd.yaml"), []byte(yamlDef), 0o600); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	cmd = NewRootCmdWithLang("0.2.2", "abc", "2026-11-24", i18n.LangEN)
	cmd.SetArgs([]string{"clean", "--config-dir", cfgDir, "--builtin-disabled", "--only", "custom-echo", "--unsafe", "--yes"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("custom command cleaner with --unsafe failed: %v out=%q", err, out.String())
	}
	if data, err := os.ReadFile(marker); err != nil || strings.TrimSpace(string(data)) != "done" {
		t.Fatalf("custom command did not run: err=%v data=%q", err, string(data))
	}
}
