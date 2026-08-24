package executor

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

func TestDebrisDownloadsHonorsSuffixDepthAndProfileAgeFloor(t *testing.T) {
	home := t.TempDir()
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	old := []string{"a.crdownload", "b.opdownload", "c.part", "d.partial", "e.download", "f.aria2"}
	for _, name := range old {
		writeDebrisAged(t, filepath.Join(downloads, name), -48*time.Hour)
	}
	writeDebrisAged(t, filepath.Join(downloads, "keep.tmp"), -48*time.Hour)
	writeDebrisAged(t, filepath.Join(downloads, "fresh.part"), -2*time.Hour)
	writeDebrisAged(t, filepath.Join(downloads, "nested", "deep.part"), -48*time.Hour)

	minAge := 0
	c := model.Cleaner{
		ID:          "downloads",
		Name:        "downloads",
		Scope:       model.ScopeUser,
		Type:        model.TypeDebris,
		MinAgeDays:  &minAge,
		NoAgeFilter: true,
		Debris:      &model.DebrisCfg{Profile: "desktop-download-partials"},
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		e := NewDebrisExecutor(home, safety.NewWhitelist(nil))
		e.procRoot = t.TempDir()
		e.env = func(string) string { return "" }
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	for _, name := range old {
		if !hasDelete(events, filepath.Join(downloads, name)) {
			t.Errorf("old allowed suffix %q was not deleted", name)
		}
	}
	for _, name := range []string{"keep.tmp", "fresh.part", filepath.Join("nested", "deep.part")} {
		if hasDelete(events, filepath.Join(downloads, name)) {
			t.Errorf("%q should be preserved", name)
		}
		if _, err := os.Stat(filepath.Join(downloads, name)); err != nil {
			t.Errorf("%q missing: %v", name, err)
		}
	}
}

// TestDownloadsRootRejectsUnsafeEnvBeforeUserDirsFallback verifies that an
// absolute environment value outside target home cannot bypass the safe
// user-dirs parser.
func TestDownloadsRootRejectsUnsafeEnvBeforeUserDirsFallback(t *testing.T) {
	home := t.TempDir()
	parsed := filepath.Join(home, "Fetch")
	if err := os.MkdirAll(parsed, 0o755); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, ".config", "user-dirs.dirs")
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, []byte("XDG_DOWNLOAD_DIR=\"$HOME/Fetch\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := NewDebrisExecutor(home, nil)
	e.env = func(name string) string {
		if name == "XDG_DOWNLOAD_DIR" {
			return "/tmp/not-target-home"
		}
		return ""
	}
	root, err := e.resolveDownloadsRoot()
	if err != nil {
		t.Fatal(err)
	}
	if root != parsed {
		t.Fatalf("root = %q, want parser value %q", root, parsed)
	}
}

// TestDownloadsPreservesPayloadWhenDeletingAriaSidecar verifies that a
// downloader payload without a partial suffix is never selected with its
// .aria2 sidecar.
func TestDownloadsPreservesPayloadWhenDeletingAriaSidecar(t *testing.T) {
	home := t.TempDir()
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(downloads, "archive.iso")
	sidecar := payload + ".aria2"
	writeDebrisAged(t, payload, -48*time.Hour)
	writeDebrisAged(t, sidecar, -48*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	minAge := 0
	c := model.Cleaner{ID: "downloads", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "desktop-download-partials"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasDelete(events, sidecar) {
		t.Fatal("aria sidecar should emit deletion event")
	}
	if _, err := os.Stat(payload); err != nil {
		t.Fatalf("payload should remain: %v", err)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Fatalf("aria sidecar should be deleted, stat = %v", err)
	}
}

// TestDownloadsSkipsHardlinksAndOpenFiles verifies that only regular
// single-link, non-open candidates are removed.
func TestDownloadsSkipsHardlinksAndOpenFiles(t *testing.T) {
	home := t.TempDir()
	downloads := filepath.Join(home, "Downloads")
	if err := os.MkdirAll(downloads, 0o755); err != nil {
		t.Fatal(err)
	}
	hard := filepath.Join(downloads, "hard.part")
	hardLink := filepath.Join(downloads, "hard-copy")
	openPath := filepath.Join(downloads, "open.part")
	single := filepath.Join(downloads, "single.part")
	writeDebrisAged(t, hard, -48*time.Hour)
	if err := os.Link(hard, hardLink); err != nil {
		t.Fatal(err)
	}
	writeDebrisAged(t, openPath, -48*time.Hour)
	writeDebrisAged(t, single, -48*time.Hour)
	f, err := os.Open(openPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	procRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(procRoot, "123", "fd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(openPath, filepath.Join(procRoot, "123", "fd", "7")); err != nil {
		t.Fatal(err)
	}
	e := NewDebrisExecutor(home, nil)
	e.procRoot = procRoot
	e.env = func(string) string { return "" }
	minAge := 0
	c := model.Cleaner{ID: "downloads", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "desktop-download-partials"}}
	if err := e.Run(context.Background(), c, false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(single); !os.IsNotExist(err) {
		t.Fatalf("single-link candidate should be deleted, stat = %v", err)
	}
	for _, path := range []string{hard, hardLink, openPath} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("protected candidate %q missing: %v", path, err)
		}
	}
}

// TestHuggingFaceIncompleteDeletesOnlyOldIncompleteArtifacts verifies the
// profile's closed suffix filter and mandatory 24-hour floor.
func TestHuggingFaceIncompleteDeletesOnlyOldIncompleteArtifacts(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "huggingface")
	old := filepath.Join(root, "hub", "model.incomplete")
	complete := filepath.Join(root, "hub", "model.safetensors")
	fresh := filepath.Join(root, "hub", "new.incomplete")
	writeDebrisAged(t, old, -48*time.Hour)
	writeDebrisAged(t, complete, -48*time.Hour)
	writeDebrisAged(t, fresh, -2*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	minAge := 0
	c := model.Cleaner{ID: "hf", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "ai-huggingface-incomplete"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasDelete(events, old) {
		t.Fatal("old incomplete artifact should be deleted")
	}
	for _, path := range []string{complete, fresh} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("protected artifact %q missing: %v", path, err)
		}
	}
}

// TestHuggingFaceIncompleteSkipsWhenLockExists verifies that any cache lock
// blocks the whole profile before deletion starts.
func TestHuggingFaceIncompleteSkipsWhenLockExists(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "huggingface")
	candidate := filepath.Join(root, "hub", "model.incomplete")
	lock := filepath.Join(root, ".locks", "model.lock")
	writeDebrisAged(t, candidate, -48*time.Hour)
	writeDebrisAged(t, lock, -48*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	c := model.Cleaner{ID: "hf", Scope: model.ScopeUser, Type: model.TypeDebris, Debris: &model.DebrisCfg{Profile: "ai-huggingface-incomplete"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "cache_busy") {
		t.Fatalf("lock should skip cleaner; events: %+v", events)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("candidate should remain while lock exists: %v", err)
	}
}

// TestRuffCacheRequiresExplicitHomeBoundRoot verifies that the profile never
// discovers project caches by recursively scanning target home.
func TestRuffCacheRequiresExplicitHomeBoundRoot(t *testing.T) {
	home := t.TempDir()
	projectCache := filepath.Join(home, "project", ".ruff_cache")
	writeDebrisAged(t, filepath.Join(projectCache, "content", "entry"), -48*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	c := model.Cleaner{ID: "ruff", Scope: model.ScopeUser, Type: model.TypeDebris, Debris: &model.DebrisCfg{Profile: "python-ruff-cache"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "root_unavailable") {
		t.Fatalf("missing explicit root should skip; events: %+v", events)
	}
	if _, err := os.Stat(filepath.Join(projectCache, "content", "entry")); err != nil {
		t.Fatalf("project cache should not be discovered: %v", err)
	}
}

// TestRuffCacheDeletesOnlyWithinExplicitEnvironmentRoot verifies that an
// explicit cache root under home is accepted and cleaned.
func TestRuffCacheDeletesOnlyWithinExplicitEnvironmentRoot(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, ".ruff-cache")
	candidate := filepath.Join(cache, "content", "entry")
	writeDebrisAged(t, candidate, -48*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(name string) string {
		if name == "RUFF_CACHE_DIR" {
			return cache
		}
		return ""
	}
	minAge := 0
	c := model.Cleaner{ID: "ruff", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "python-ruff-cache"}}
	if err := e.Run(context.Background(), c, false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(candidate); !os.IsNotExist(err) {
		t.Fatalf("explicit cache candidate should be deleted, stat = %v", err)
	}
}

// TestGPUCacheEnforcesSevenDayFloor verifies that a CLI age lower than the
// profile floor cannot remove recent GPU artifacts.
func TestGPUCacheEnforcesSevenDayFloor(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".nv", "ComputeCache")
	recent := filepath.Join(root, "recent.bin")
	old := filepath.Join(root, "old.bin")
	writeDebrisAged(t, recent, -3*24*time.Hour)
	writeDebrisAged(t, old, -8*24*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	e.processCheck = func([]string, string) (bool, error) { return false, nil }
	minAge := 0
	c := model.Cleaner{ID: "nvidia", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "gpu-nvidia-compute-cache"}}
	if err := e.Run(context.Background(), c, false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old GPU artifact should be deleted, stat = %v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent GPU artifact should remain: %v", err)
	}
}

// TestGPUCacheSkipsWhenOwnerProcessRuns verifies process guards prevent any
// deletion while a profile owner is active.
func TestGPUCacheSkipsWhenOwnerProcessRuns(t *testing.T) {
	home := t.TempDir()
	candidate := filepath.Join(home, ".triton", "cache", "kernel.bin")
	writeDebrisAged(t, candidate, -8*24*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	e.processCheck = func([]string, string) (bool, error) { return true, nil }
	c := model.Cleaner{ID: "triton", Scope: model.ScopeUser, Type: model.TypeDebris, Debris: &model.DebrisCfg{Profile: "gpu-triton-cache"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, string(model.SkipReasonRunningProcess)) {
		t.Fatalf("running owner should skip; events: %+v", events)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("candidate should remain while owner runs: %v", err)
	}
}

// TestCloudLogsEnforceThirtyDayFloor verifies cloud log profiles retain
// recent logs even when configuration asks for a lower age.
func TestCloudLogsEnforceThirtyDayFloor(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".config", "gcloud", "logs")
	recent := filepath.Join(root, "recent.log")
	old := filepath.Join(root, "old.log")
	writeDebrisAged(t, recent, -14*24*time.Hour)
	writeDebrisAged(t, old, -31*24*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	minAge := 0
	c := model.Cleaner{ID: "gcloud", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "cloud-gcloud-logs"}}
	if err := e.Run(context.Background(), c, false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old cloud log should be deleted, stat = %v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent cloud log should remain: %v", err)
	}
}

// TestDebrisProfilesRejectSystemScope verifies home-bound debris profiles
// cannot be relabeled as system cleaners.
func TestDebrisProfilesRejectSystemScope(t *testing.T) {
	home := t.TempDir()
	candidate := filepath.Join(home, ".cache", "pyinstaller", "old.bin")
	writeDebrisAged(t, candidate, -48*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	c := model.Cleaner{ID: "pyinstaller", Scope: model.ScopeSystem, Type: model.TypeDebris, Debris: &model.DebrisCfg{Profile: "python-pyinstaller-cache"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "profile_scope_mismatch") {
		t.Fatalf("system-scope debris should skip; events: %+v", events)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("candidate should remain: %v", err)
	}
}

// TestDebrisHonorsKeepList verifies that a matching whitelist entry remains
// protected inside a dedicated profile root.
func TestDebrisHonorsKeepList(t *testing.T) {
	home := t.TempDir()
	candidate := filepath.Join(home, "Downloads", "keep.part")
	writeDebrisAged(t, candidate, -48*time.Hour)
	e := NewDebrisExecutor(home, safety.NewWhitelist([]string{candidate}))
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	minAge := 0
	c := model.Cleaner{ID: "downloads", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "desktop-download-partials"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasSkip(events, string(model.SkipReasonWhitelisted)) {
		t.Fatalf("keep-list candidate should be skipped; events: %+v", events)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("keep-list candidate should remain: %v", err)
	}
}

// TestDebrisUsesConservativeDefaultAge verifies profiles without a dedicated
// floor still inherit the cleaner's 14-day default instead of deleting all
// existing files.
func TestDebrisUsesConservativeDefaultAge(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "pyinstaller")
	recent := filepath.Join(root, "recent.bin")
	old := filepath.Join(root, "old.bin")
	writeDebrisAged(t, recent, -2*24*time.Hour)
	writeDebrisAged(t, old, -15*24*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	c := model.Cleaner{ID: "pyinstaller", Scope: model.ScopeUser, Type: model.TypeDebris, Debris: &model.DebrisCfg{Profile: "python-pyinstaller-cache"}}
	if err := e.Run(context.Background(), c, false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old file should be deleted, stat = %v", err)
	}
	if _, err := os.Stat(recent); err != nil {
		t.Fatalf("recent file should remain: %v", err)
	}
}

// TestDebrisRejectsSymlinkRoot verifies profile roots are opened without
// following a symlink, even when its target lies under target home.
func TestDebrisRejectsSymlinkRoot(t *testing.T) {
	home := t.TempDir()
	realRoot := filepath.Join(home, "real-cache")
	linkRoot := filepath.Join(home, ".triton", "cache")
	if err := os.MkdirAll(realRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(linkRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Fatal(err)
	}
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	c := model.Cleaner{ID: "triton", Scope: model.ScopeUser, Type: model.TypeDebris, Debris: &model.DebrisCfg{Profile: "gpu-triton-cache"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, string(model.SkipReasonSymlink)) {
		t.Fatalf("symlink root should skip; events: %+v", events)
	}
}

// TestDebrisRejectsRootSwapBeforeOpen verifies that replacing the selected
// root before opening it cannot redirect cleanup to a different directory.
func TestDebrisRejectsRootSwapBeforeOpen(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "pyinstaller")
	candidate := filepath.Join(root, "old.bin")
	writeDebrisAged(t, candidate, -48*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	minAge := 0
	c := model.Cleaner{ID: "pyinstaller", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "python-pyinstaller-cache"}}
	e.beforeRootOpen = func(selected string) error {
		moved := selected + ".moved"
		if err := os.Rename(selected, moved); err != nil {
			return err
		}
		return os.Mkdir(selected, 0o755)
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "root_identity_changed") {
		t.Fatalf("root replacement should fail closed; events: %+v", events)
	}
	if _, err := os.Stat(filepath.Join(root+".moved", "old.bin")); err != nil {
		t.Fatalf("original candidate should remain in moved directory: %v", err)
	}
}

// TestDebrisRejectsMountIDMismatch verifies that a mount identity change is
// rejected even when device, inode, and owner remain otherwise acceptable.
func TestDebrisRejectsMountIDMismatch(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "pyinstaller")
	writeDebrisAged(t, filepath.Join(root, "old.bin"), -48*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	fdCalls := 0
	e.fdIdentity = func(fd int) (debrisIdentity, error) {
		identity, err := readDebrisFDIdentity(fd)
		if err != nil {
			return debrisIdentity{}, err
		}
		fdCalls++
		if fdCalls == 2 {
			identity.MntID++
		}
		return identity, nil
	}
	minAge := 0
	c := model.Cleaner{ID: "pyinstaller", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "python-pyinstaller-cache"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "root_identity_changed") {
		t.Fatalf("mount identity mismatch should fail closed; events: %+v", events)
	}
}

// TestDebrisRejectsHidepidProcVisibility verifies that hidepid=2 cannot be
// treated as a complete fd view for deletion safety.
func TestDebrisRejectsHidepidProcVisibility(t *testing.T) {
	home := t.TempDir()
	candidate := filepath.Join(home, "Downloads", "old.part")
	writeDebrisAged(t, candidate, -48*time.Hour)
	procRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(procRoot, "mounts"), []byte("proc /proc proc rw,nosuid,hidepid=2 0 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := NewDebrisExecutor(home, nil)
	e.procRoot = procRoot
	e.env = func(string) string { return "" }
	minAge := 0
	c := model.Cleaner{ID: "downloads", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "desktop-download-partials"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "proc_inspection_failed") {
		t.Fatalf("hidepid proc view should skip; events: %+v", events)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("candidate should remain: %v", err)
	}
}

// TestHuggingFaceBusyDetectsHardlinkAlias verifies that an open descriptor
// reached through an alias outside the cache root still matches by identity.
func TestHuggingFaceBusyDetectsHardlinkAlias(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "huggingface")
	blob := filepath.Join(root, "hub", "blob")
	alias := filepath.Join(home, "outside-alias")
	writeDebrisAged(t, blob, -48*time.Hour)
	if err := os.Link(blob, alias); err != nil {
		t.Fatal(err)
	}
	procRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(procRoot, "123", "fd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(alias, filepath.Join(procRoot, "123", "fd", "7")); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(root, "hub", "model.incomplete")
	writeDebrisAged(t, candidate, -48*time.Hour)
	e := NewDebrisExecutor(home, nil)
	e.procRoot = procRoot
	e.env = func(string) string { return "" }
	minAge := 0
	c := model.Cleaner{ID: "hf", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "ai-huggingface-incomplete"}}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "cache_busy") {
		t.Fatalf("hardlink alias should mark HF cache busy; events: %+v", events)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("candidate should remain: %v", err)
	}
}

func TestHuggingFaceDebrisIgnoresCleanerHeldRootDescriptor(t *testing.T) {
	home := t.TempDir()
	candidate := filepath.Join(home, ".cache", "huggingface", "hub", "model.incomplete")
	writeDebrisAged(t, candidate, -48*time.Hour)
	procRoot := t.TempDir()
	fdRoot := filepath.Join(procRoot, strconv.Itoa(os.Getpid()), "fd")
	if err := os.MkdirAll(fdRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	reserved, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer reserved.Close()
	rootFD := int(reserved.Fd()) + 2
	root := filepath.Join(home, ".cache", "huggingface")
	if err := os.Symlink(root, filepath.Join(fdRoot, strconv.Itoa(rootFD))); err != nil {
		t.Fatal(err)
	}
	e := NewDebrisExecutor(home, nil)
	e.procRoot = procRoot
	e.env = func(string) string { return "" }
	minAge := 0
	c := model.Cleaner{
		ID:         "hf",
		Scope:      model.ScopeUser,
		Type:       model.TypeDebris,
		MinAgeDays: &minAge,
		Debris:     &model.DebrisCfg{Profile: "ai-huggingface-incomplete"},
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(candidate); !os.IsNotExist(err) {
		t.Fatalf("HF debris candidate should be deleted; stat = %v, events=%+v", err, events)
	}
	if hasCleanerSkip(events, "cache_busy") {
		t.Fatalf("cleaner's own held root descriptor marked debris busy: %+v", events)
	}
}

// TestDebrisAgeBoundaryUsesNanoseconds verifies that an artifact one fraction
// of a second newer than the threshold is not deleted early.
func TestDebrisAgeBoundaryUsesNanoseconds(t *testing.T) {
	home := t.TempDir()
	candidate := filepath.Join(home, "Downloads", "boundary.part")
	writeDebrisAged(t, candidate, -48*time.Hour)
	now := time.Unix(2_000_000_000, 900_000_000)
	boundary := now.Add(-24 * time.Hour).Add(10 * time.Millisecond)
	if err := os.Chtimes(candidate, boundary, boundary); err != nil {
		t.Fatal(err)
	}
	e := NewDebrisExecutor(home, nil)
	e.procRoot = t.TempDir()
	e.env = func(string) string { return "" }
	e.now = func() time.Time { return now }
	minAge := 0
	c := model.Cleaner{ID: "downloads", Scope: model.ScopeUser, Type: model.TypeDebris, MinAgeDays: &minAge, Debris: &model.DebrisCfg{Profile: "desktop-download-partials"}}
	if err := e.Run(context.Background(), c, false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(candidate); err != nil {
		t.Fatalf("subsecond-new candidate should remain: %v", err)
	}
}

func writeDebrisAged(t *testing.T, path string, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}
