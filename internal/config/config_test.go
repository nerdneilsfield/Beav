package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigOverridesAndWhitelist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`
defaults:
  min_age_days: 21
  output: spinner
overrides:
  editor-vscode:
    min_age_days: 3
    enabled: true
whitelist:
  - ~/.cache/keepme
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "whitelist.txt"), []byte("/tmp/keepme\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithHome(dir, "/home/target")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.MinAgeDays != 21 {
		t.Errorf("min_age_days got %d", cfg.Defaults.MinAgeDays)
	}
	ovr := cfg.Overrides["editor-vscode"]
	if ovr.MinAgeDays == nil || *ovr.MinAgeDays != 3 {
		t.Errorf("vscode override age missing: %+v", ovr)
	}
	wl := cfg.MergedWhitelist()
	want := []string{"/tmp/keepme", "/home/target/.cache/keepme"}
	if len(wl) != 2 || wl[0] != want[0] || wl[1] != want[1] {
		t.Errorf("whitelist got %v want %v", wl, want)
	}
}

// TestLoadWithHomeExpandsWhitelistForTargetHome verifies that ~/ paths in whitelist.txt are expanded using the provided home.
// TestLoadWithHomeExpandsWhitelistForTargetHome 验证 whitelist.txt 中的 ~/ 路径使用提供的主目录展开。
func TestLoadWithHomeExpandsWhitelistForTargetHome(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "whitelist.txt"), []byte("~/.cache/protected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadWithHome(dir, "/home/alice")
	if err != nil {
		t.Fatal(err)
	}
	got := cfg.MergedWhitelist()
	if len(got) != 1 || got[0] != "/home/alice/.cache/protected" {
		t.Fatalf("whitelist = %v", got)
	}
}

func TestLoadRejectsHomeRelativeWhitelistWithoutHome(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "whitelist.txt"), []byte("~/.cache/protected\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWithHome(dir, ""); err == nil {
		t.Fatal("expected ~/ whitelist to be rejected when no home is available")
	}
}

func TestLoadDefaultsToAutoOutput(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Output != "auto" {
		t.Fatalf("output default = %q, want auto", cfg.Defaults.Output)
	}
}

func TestWhitelistAddRemoveAndSavePreservesTilde(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("defaults:\n  min_age_days: 14\n  output: auto\nwhitelist:\n  - ~/.cache/keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "whitelist.txt"), []byte("/tmp/txt-keep\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadWithHome(dir, "/home/alice")
	if err != nil {
		t.Fatal(err)
	}
	added, err := cfg.AddWhitelist("~/.cache/keep", "/home/alice")
	if err != nil || added {
		t.Fatalf("duplicate tilde entry: added=%v err=%v", added, err)
	}
	added, err = cfg.AddWhitelist("~/.cache/second", "/home/alice")
	if err != nil || !added {
		t.Fatalf("new tilde entry: added=%v err=%v", added, err)
	}
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "~/.cache/keep") || !strings.Contains(string(saved), "~/.cache/second") {
		t.Fatalf("Save did not preserve ~/ prefixes: %q", string(saved))
	}
	if strings.Contains(string(saved), "/home/alice") {
		t.Fatalf("Save leaked expanded home path into config.yaml: %q", string(saved))
	}

	removed, inTxt := cfg.RemoveWhitelist("/home/alice/.cache/keep", "/home/alice")
	if !removed || inTxt {
		t.Fatalf("RemoveWhitelist by expanded path = (%v, %v), want (true, false)", removed, inTxt)
	}
	removed, inTxt = cfg.RemoveWhitelist("/tmp/txt-keep", "/home/alice")
	if removed || !inTxt {
		t.Fatalf("RemoveWhitelist for whitelist.txt entry = (%v, %v), want (false, true)", removed, inTxt)
	}
}
