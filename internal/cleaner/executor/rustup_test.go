package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

func rustupUnitFor(leaf string) rustupUnit {
	return rustupUnit{leaf: leaf, family: rustupFamilyKey(leaf)}
}

func TestRustupFamilyKeyGroupsReleaseLines(t *testing.T) {
	cases := []struct {
		leaf string
		want string
	}{
		{"stable-x86_64-unknown-linux-gnu", "stable|x86_64-unknown-linux-gnu"},
		{"nightly-x86_64-unknown-linux-gnu", "nightly|x86_64-unknown-linux-gnu"},
		{"nightly-2024-01-01-x86_64-unknown-linux-gnu", "nightly|x86_64-unknown-linux-gnu"},
		{"1.85-x86_64-unknown-linux-gnu", "1.85|x86_64-unknown-linux-gnu"},
		{"1.85.1-x86_64-unknown-linux-gnu", "1.85|x86_64-unknown-linux-gnu"},
		{"1.84.0-x86_64-unknown-linux-gnu", "1.84|x86_64-unknown-linux-gnu"},
		{"stable-x86_64-unknown-linux-musl", "stable|x86_64-unknown-linux-musl"},
	}
	for _, tc := range cases {
		if got := rustupFamilyKey(tc.leaf); got != tc.want {
			t.Errorf("rustupFamilyKey(%q) = %q, want %q", tc.leaf, got, tc.want)
		}
	}
}

func TestSelectStaleRustupKeepsNewestPerLineAndDefault(t *testing.T) {
	units := []rustupUnit{
		rustupUnitFor("stable-x86_64-unknown-linux-gnu"),
		rustupUnitFor("nightly-2024-01-01-x86_64-unknown-linux-gnu"),
		rustupUnitFor("nightly-2024-06-01-x86_64-unknown-linux-gnu"),
		rustupUnitFor("1.85.0-x86_64-unknown-linux-gnu"),
		rustupUnitFor("1.85.1-x86_64-unknown-linux-gnu"),
		rustupUnitFor("1.84.0-x86_64-unknown-linux-gnu"),
	}
	stale := selectStaleRustupToolchains(units, "1.84.0-x86_64-unknown-linux-gnu")
	got := map[string]bool{}
	for _, u := range stale {
		got[u.leaf] = true
	}
	for _, want := range []string{
		"nightly-2024-01-01-x86_64-unknown-linux-gnu",
		"1.85.0-x86_64-unknown-linux-gnu",
	} {
		if !got[want] {
			t.Errorf("stale missing %q; got %v", want, got)
		}
	}
	for _, keep := range []string{
		"stable-x86_64-unknown-linux-gnu",
		"nightly-2024-06-01-x86_64-unknown-linux-gnu",
		"1.85.1-x86_64-unknown-linux-gnu",
		"1.84.0-x86_64-unknown-linux-gnu", // default
	} {
		if got[keep] {
			t.Errorf("default/newest %q must be kept; stale=%v", keep, got)
		}
	}
}

func TestSelectStaleRustupVersionOrderNumericNotLexical(t *testing.T) {
	// 1.9 and 1.10 are different release lines and both stay; numeric
	// ordering only applies within one line (1.9.9 vs 1.9.10).
	units := []rustupUnit{
		rustupUnitFor("1.9.9-x86_64-unknown-linux-gnu"),
		rustupUnitFor("1.9.10-x86_64-unknown-linux-gnu"),
	}
	stale := selectStaleRustupToolchains(units, "")
	if len(stale) != 1 || stale[0].leaf != "1.9.9-x86_64-unknown-linux-gnu" {
		t.Fatalf("stale=%+v, want only 1.9.9 removed", stale)
	}
}

// newRustupTestExecutor builds an executor against a fixture home with the
// usual proc/env seams pointed at inert fakes.
func newRustupTestExecutor(t *testing.T, home string) *RustupExecutor {
	t.Helper()
	e := NewRustupExecutor(home, safety.NewWhitelist(nil), systemID(os.Getuid()))
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	e.processCheck = func([]string, string) (bool, error) { return false, nil }
	e.procVisibility = func(string, uint32) error { return nil }
	return e
}

func writeRustupToolchain(t *testing.T, root, name string, age time.Duration) {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Join(dir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeDebrisAged(t, filepath.Join(dir, "bin", "rustc"), age)
	// Age the directory itself; content removal checks the dir mtime.
	when := time.Now().Add(age)
	if err := os.Chtimes(dir, when, when); err != nil {
		t.Fatal(err)
	}
}

func rustupTestCleaner() model.Cleaner {
	minAge := 30
	return model.Cleaner{
		ID:          "lang-rustup",
		Name:        "rust",
		Scope:       model.ScopeUser,
		Type:        model.TypeRustupPrune,
		MinAgeDays:  &minAge,
		RustupPrune: &model.RustupCfg{Profile: "lang-rustup"},
	}
}

func TestRustupRemovesStaleKeepsDefaultAndNewest(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".rustup", "toolchains")
	writeRustupToolchain(t, root, "stable-x86_64-unknown-linux-gnu", -720*time.Hour)
	writeRustupToolchain(t, root, "1.85.0-x86_64-unknown-linux-gnu", -720*time.Hour)
	writeRustupToolchain(t, root, "1.85.1-x86_64-unknown-linux-gnu", -720*time.Hour)

	settings := filepath.Join(home, ".rustup", "settings.toml")
	if err := os.WriteFile(settings, []byte(`default_toolchain = "stable-x86_64-unknown-linux-gnu"
`), 0o600); err != nil {
		t.Fatal(err)
	}

	c := rustupTestCleaner()
	events := captureEvents(t, func(emit func(model.Event)) {
		e := newRustupTestExecutor(t, home)
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	stableDir := filepath.Join(root, "stable-x86_64-unknown-linux-gnu")
	oldDir := filepath.Join(root, "1.85.0-x86_64-unknown-linux-gnu")
	keepDir := filepath.Join(root, "1.85.1-x86_64-unknown-linux-gnu")
	if _, err := os.Stat(stableDir); err != nil {
		t.Errorf("default toolchain removed: %v", err)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Errorf("stale 1.85.0 should be removed, stat err=%v", err)
	}
	if _, err := os.Stat(keepDir); err != nil {
		t.Errorf("newest 1.85.1 removed: %v", err)
	}
	if !hasDelete(events, oldDir) {
		t.Errorf("missing delete event for %q; events=%+v", oldDir, events)
	}
	if hasDelete(events, stableDir) || hasDelete(events, keepDir) {
		t.Errorf("delete event touched a kept toolchain; events=%+v", events)
	}
}

func TestRustupDryRunKeepsEverythingAndCountsCandidates(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".rustup", "toolchains")
	writeRustupToolchain(t, root, "1.85.0-x86_64-unknown-linux-gnu", -720*time.Hour)
	writeRustupToolchain(t, root, "1.85.1-x86_64-unknown-linux-gnu", -720*time.Hour)

	c := rustupTestCleaner()
	var result model.Event
	events := captureEvents(t, func(emit func(model.Event)) {
		e := newRustupTestExecutor(t, home)
		if err := e.Run(context.Background(), c, true, emit); err != nil {
			t.Fatal(err)
		}
	})
	for _, ev := range events {
		if ev.Event == model.EvFinish {
			result = ev
		}
	}
	if result.FilesCandidate == 0 || result.BytesCandidate == 0 {
		t.Errorf("dry-run candidates not counted: %+v", result)
	}
	for _, leaf := range []string{"1.85.0-x86_64-unknown-linux-gnu", "1.85.1-x86_64-unknown-linux-gnu"} {
		if _, err := os.Stat(filepath.Join(root, leaf)); err != nil {
			t.Errorf("dry-run removed %q: %v", leaf, err)
		}
	}
	if !hasSkip(events, string(model.SkipReasonDryRun)) {
		t.Errorf("missing dry_run skip events: %+v", events)
	}
}

func TestRustupSkipsSymlinkedToolchain(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".rustup", "toolchains")
	writeRustupToolchain(t, root, "1.85.1-x86_64-unknown-linux-gnu", -720*time.Hour)
	external := filepath.Join(home, "external-toolchain")
	writeRustupToolchain(t, external, "bin-placeholder", -720*time.Hour)
	link := filepath.Join(root, "custom-x86_64-unknown-linux-gnu")
	if err := os.Symlink(external, link); err != nil {
		t.Fatal(err)
	}

	c := rustupTestCleaner()
	events := captureEvents(t, func(emit func(model.Event)) {
		e := newRustupTestExecutor(t, home)
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasSkip(events, string(model.SkipReasonSymlink)) {
		t.Errorf("linked toolchain not reported as symlink skip: %+v", events)
	}
	if _, err := os.Lstat(link); err != nil {
		t.Errorf("linked toolchain removed: %v", err)
	}
	if _, err := os.Stat(external); err != nil {
		t.Errorf("external toolchain content removed through link: %v", err)
	}
}

func TestRustupSkipsRecentToolchain(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".rustup", "toolchains")
	// Three members of the 1.85 line: 1.85.2 is default-protected, 1.85.1 is
	// the newest member, so 1.85.0 is the only removal candidate; making it
	// recent must hit the age gate.
	writeRustupToolchain(t, root, "1.85.0-x86_64-unknown-linux-gnu", -1*time.Hour)
	writeRustupToolchain(t, root, "1.85.1-x86_64-unknown-linux-gnu", -720*time.Hour)
	writeRustupToolchain(t, root, "1.85.2-x86_64-unknown-linux-gnu", -720*time.Hour)
	settings := filepath.Join(home, ".rustup", "settings.toml")
	if err := os.WriteFile(settings, []byte("default_toolchain = \"1.85.2-x86_64-unknown-linux-gnu\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	c := rustupTestCleaner()
	events := captureEvents(t, func(emit func(model.Event)) {
		e := newRustupTestExecutor(t, home)
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	recent := filepath.Join(root, "1.85.0-x86_64-unknown-linux-gnu")
	if _, err := os.Stat(recent); err != nil {
		t.Errorf("recent toolchain removed: %v", err)
	}
	if !hasSkip(events, string(model.SkipReasonAgeTooRecent)) {
		t.Errorf("missing age_too_recent skip: %+v", events)
	}
	if hasDelete(events, recent) {
		t.Errorf("recent toolchain got a delete event: %+v", events)
	}
}
