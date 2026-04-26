package config

import (
	"os"
	"path/filepath"
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

func TestLoadDefaultsToAutoOutput(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.Output != "auto" {
		t.Fatalf("output default = %q, want auto", cfg.Defaults.Output)
	}
}
