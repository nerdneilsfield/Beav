package executor

import (
	"context"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

func TestToolPruneHomebrewUsesResolvedBinaryAndNativeDryRun(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	cache := filepath.Join(home, ".cache", "Homebrew")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	brew := filepath.Join(binDir, "brew")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\nif [ \"$1\" = cleanup ] && [ \"$2\" = --help ]; then printf '%s\\n' 'brew cleanup --dry-run'; exit 0; fi\nprintf '%s\\n' \"$0 $*\" >> \"$HOME/invocations\"\nprintf 'home=%s\\n' \"$HOME\" >> \"$HOME/env\"\nprintf 'path=%s\\n' \"$PATH\" >> \"$HOME/env\"\nprintf 'brew 4.0\\n'\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{
		ID:         "brew",
		Scope:      model.ScopeUser,
		Type:       model.TypeToolPrune,
		MinAgeDays: ptrInt(5),
		ToolPrune:  &model.ToolPruneCfg{Profile: "pkg-homebrew"},
	}
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir), tempToolIdentityOption(t, home))
		if err := e.Run(context.Background(), c, true, emit); err != nil {
			t.Fatal(err)
		}
	})
	invocation, err := os.ReadFile(filepath.Join(home, "invocations"))
	if err != nil {
		t.Fatalf("invocation missing: %v; events=%+v", err, evs)
	}
	if !strings.Contains(string(invocation), brew+" cleanup --prune=5 --dry-run") {
		t.Fatalf("invocation = %q", invocation)
	}
	env, err := os.ReadFile(filepath.Join(home, "env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(env), "path="+binDir) {
		t.Fatalf("environment = %q", env)
	}
	for _, ev := range evs {
		if ev.Event == model.EvFinish {
			if !ev.MetricsIncomplete {
				t.Fatalf("native dry-run should report incomplete metrics: %+v", ev)
			}
			return
		}
	}
	t.Fatal("missing finish event")
}

func tempToolIdentityOption(t *testing.T, home string) ToolPruneOption {
	t.Helper()
	uid := uint32(os.Getuid())
	canonical, err := ResolveToolIdentity(uid, "")
	if err != nil {
		t.Fatal(err)
	}
	canonical.Home = home
	return withToolIdentityResolverForTest(func(got uint32) (CommandIdentity, error) {
		if got != uid {
			return CommandIdentity{}, fmt.Errorf("resolver UID %d, want %d", got, uid)
		}
		return canonical, nil
	})
}

func TestToolPruneUnsupportedDryRunNeverExecutes(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "uv")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	uv := filepath.Join(binDir, "uv")
	if err := os.WriteFile(uv, []byte("#!/bin/sh\ntouch \"$HOME/executed\"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{ID: "uv", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}}
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir), tempToolIdentityOption(t, home))
		if err := e.Run(context.Background(), c, true, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(home, "executed")); !os.IsNotExist(err) {
		t.Fatalf("unsupported dry-run executed tool: %v", err)
	}
	if !hasCleanerSkip(evs, string(model.SkipReasonDryRunUnsupported)) {
		t.Fatalf("events = %+v", evs)
	}
	for _, ev := range evs {
		if ev.Event == model.EvFinish && !ev.MetricsIncomplete {
			t.Fatalf("unsupported dry-run should report incomplete metrics: %+v", ev)
		}
	}
}

func TestToolPruneBlocksWholeCommandWhenKeepListIntersectsRoot(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "uv")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	uv := filepath.Join(binDir, "uv")
	if err := os.WriteFile(uv, []byte("#!/bin/sh\ntouch \"$HOME/executed\"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(root, "keep")
	c := model.Cleaner{ID: "uv", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}}
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, safety.NewWhitelist([]string{keep}), WithToolSearchPath(binDir), tempToolIdentityOption(t, home))
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(home, "executed")); !os.IsNotExist(err) {
		t.Fatalf("keep-list intersection executed tool: %v", err)
	}
	if !hasCleanerSkip(evs, "keep_list_intersection") {
		t.Fatalf("events = %+v", evs)
	}
}

func TestToolPruneCapabilityVersionMismatchSkipsBeforeCleanup(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cache", "Homebrew"), 0o755); err != nil {
		t.Fatal(err)
	}
	brew := filepath.Join(binDir, "brew")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo unrelated-tool 1.0; else touch \"$HOME/executed\"; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{ID: "brew", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "pkg-homebrew"}}
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir), tempToolIdentityOption(t, home))
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(home, "executed")); !os.IsNotExist(err) {
		t.Fatalf("version mismatch executed cleanup: %v", err)
	}
	if !hasCleanerSkip(evs, "version_mismatch") {
		t.Fatalf("events = %+v", evs)
	}
}

func TestToolPruneCommandFailurePropagatesAfterSuccessfulProbe(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".cache", "Homebrew"), 0o755); err != nil {
		t.Fatal(err)
	}
	brew := filepath.Join(binDir, "brew")
	if err := os.WriteFile(brew, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then echo brew 4.0; exit 0; fi\nexit 17\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{ID: "brew", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "pkg-homebrew"}}
	var gotErr error
	evs := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir), tempToolIdentityOption(t, home))
		gotErr = e.Run(context.Background(), c, false, emit)
	})
	if gotErr == nil {
		t.Fatal("expected cleanup command failure to propagate")
	}
	if !hasCleanerError(evs, "command_failed") {
		t.Fatalf("events = %+v", evs)
	}
}

func hasCleanerError(events []model.Event, reason string) bool {
	for _, event := range events {
		if event.Event == model.EvError && event.Reason == reason {
			return true
		}
	}
	return false
}

func TestResolveToolIdentityReturnsExactSupplementaryGroups(t *testing.T) {
	uid := uint32(os.Getuid())
	account, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ResolveToolIdentity(uid, account.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	if identity.UID != uid || identity.Name != account.Username || identity.Home != account.HomeDir {
		t.Fatalf("identity = %+v", identity)
	}
	for _, gid := range identity.Groups {
		if gid == identity.GID {
			t.Fatalf("primary GID %d leaked into supplementary groups %+v", identity.GID, identity.Groups)
		}
	}
}

func TestWithToolIdentityUsesCanonicalNSSIdentity(t *testing.T) {
	uid := uint32(os.Getuid())
	account, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		t.Fatal(err)
	}
	e := NewToolPruneExecutor(account.HomeDir, nil, WithToolIdentity(CommandIdentity{
		UID:    uid,
		GID:    0,
		Groups: []uint32{0},
		Name:   "root",
		Home:   "/root",
	}))
	c := model.Cleaner{Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}}
	identity, err := e.identityFor(c, toolProfiles["lang-uv"])
	if err != nil {
		t.Fatal(err)
	}
	if identity.UID != uid || identity.Name != account.Username || identity.Home != account.HomeDir {
		t.Fatalf("identity = %+v; want canonical NSS identity", identity)
	}
	if identity.GID == 0 {
		t.Fatalf("primary GID = 0: %+v", identity)
	}
	for _, gid := range identity.Groups {
		if gid == 0 {
			t.Fatalf("root supplementary group leaked: %+v", identity)
		}
	}
}

func TestWithToolIdentityRejectsZeroUID(t *testing.T) {
	e := NewToolPruneExecutor("/tmp", nil, WithToolIdentity(CommandIdentity{UID: 0}))
	c := model.Cleaner{Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}}
	if _, err := e.identityFor(c, toolProfiles["lang-uv"]); err == nil {
		t.Fatal("UID 0 identity accepted")
	}
}

func TestUserIdentityRejectsAlternateOwnedHome(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root user requires explicit sudo target")
	}
	account, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}
	alternate := t.TempDir()
	if filepath.Clean(alternate) == filepath.Clean(account.HomeDir) {
		t.Fatal("temporary home unexpectedly equals NSS home")
	}
	e := NewToolPruneExecutor(alternate, nil)
	c := model.Cleaner{Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}}
	if _, err := e.identityFor(c, toolProfiles["lang-uv"]); err == nil {
		t.Fatal("alternate owned home accepted instead of authoritative NSS HOME")
	}
}

func TestInjectedIdentityResolverIsExplicitTempHomeSeam(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root user requires explicit sudo target")
	}
	home := t.TempDir()
	uid := uint32(os.Getuid())
	canonical, err := ResolveToolIdentity(uid, "")
	if err != nil {
		t.Fatal(err)
	}
	canonical.Home = home
	e := NewToolPruneExecutor(home, nil, withToolIdentityResolverForTest(func(got uint32) (CommandIdentity, error) {
		if got != uid {
			t.Fatalf("resolver UID = %d, want %d", got, uid)
		}
		return canonical, nil
	}))
	c := model.Cleaner{Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}}
	identity, err := e.identityFor(c, toolProfiles["lang-uv"])
	if err != nil {
		t.Fatal(err)
	}
	if identity.Home != home || identity.UID != uid {
		t.Fatalf("identity = %+v, want injected temp home", identity)
	}
}

func TestSystemProfileRejectsCallerSearchPathRelaxation(t *testing.T) {
	e := NewToolPruneExecutor("", nil, WithToolSearchPath(t.TempDir()))
	paths := e.searchPath(toolProfiles["sys-nix-gc"], "/root")
	for _, path := range paths {
		if path == filepath.Clean(e.SearchPath[0]) || strings.Contains(path, "linuxbrew") || strings.HasPrefix(path, "/home/") {
			t.Fatalf("system search path contains untrusted directory %q: %v", path, paths)
		}
	}
}

func TestSystemProfileFakeBinaryRequiresExplicitResolverSeam(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "nix-collect-garbage")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	e := NewToolPruneExecutor("", nil, WithToolSearchPath(filepath.Dir(fake)))
	e.binaryResolver = func(profile toolProfile, _ []string) (string, error) {
		if profile.Binary != "nix-collect-garbage" {
			t.Fatalf("profile binary = %q", profile.Binary)
		}
		return fake, nil
	}
	resolved, err := e.resolveBinary(toolProfiles["sys-nix-gc"])
	if err != nil {
		t.Fatal(err)
	}
	if resolved != fake {
		t.Fatalf("resolved = %q, want fake %q", resolved, fake)
	}
}

func TestTrustedSystemBinaryRejectsUntrustedPathComponents(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "tool")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := validateTrustedPath(binary); err == nil {
		t.Fatal("caller-owned temporary binary accepted as trusted system executable")
	}
}

func TestCapabilityProbeUsesDryRunSpecificCondaAndNixFlags(t *testing.T) {
	home := t.TempDir()
	for _, profileName := range []string{"lang-conda", "sys-nix-gc"} {
		profile := toolProfiles[profileName]
		binary := filepath.Join(t.TempDir(), profile.Binary)
		if err := os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  echo "+profile.VersionWords[0]+" 1.0\nelse\n  echo --dry-run --json\nfi\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		e := NewToolPruneExecutor(home, nil)
		identity := CommandIdentity{UID: uint32(os.Getuid()), Name: "test", Home: home}
		if err := e.probe(context.Background(), binary, profile, identity, nil, []string{"HOME=" + home, "PATH=/bin:/usr/bin"}, false); err != nil {
			t.Fatalf("%s normal probe: %v", profileName, err)
		}
		if err := e.probe(context.Background(), binary, profile, identity, nil, []string{"HOME=" + home, "PATH=/bin:/usr/bin"}, true); err != nil {
			t.Fatalf("%s dry-run probe: %v", profileName, err)
		}
	}
}

func TestCapabilityProbeRejectsEmptyOutput(t *testing.T) {
	home := t.TempDir()
	binary := filepath.Join(t.TempDir(), "uv")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	profile := toolProfiles["lang-uv"]
	e := NewToolPruneExecutor(home, nil)
	identity := CommandIdentity{UID: uint32(os.Getuid()), Name: "test", Home: home}
	if err := e.probe(context.Background(), binary, profile, identity, nil, []string{"HOME=" + home, "PATH=/bin:/usr/bin"}, false); err == nil {
		t.Fatal("empty capability output accepted")
	}
}

func TestToolPruneRevalidatesBinaryAfterCapabilityProbe(t *testing.T) {
	home := t.TempDir()
	binDir := t.TempDir()
	root := filepath.Join(home, ".cache", "Homebrew")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(binDir, "brew")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nif [ \"$1\" = cleanup ] && [ \"$2\" = --help ]; then /bin/mv \"$0\" \"$0.moved\"; printf '%s\\n' 'brew cleanup --dry-run'; exit 0; fi\n/bin/touch \"$HOME/executed\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{ID: "brew", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "pkg-homebrew"}}
	var events []model.Event
	e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir), tempToolIdentityOption(t, home))
	if err := e.Run(context.Background(), c, true, func(event model.Event) { events = append(events, event) }); err != nil {
		t.Fatalf("err=%v events=%+v", err, events)
	}
	if _, err := os.Stat(filepath.Join(home, "executed")); !os.IsNotExist(err) {
		t.Fatalf("cleanup executed after binary replacement: %v", err)
	}
	if !hasCleanerSkip(events, "safety_changed") {
		t.Fatalf("events = %+v, want safety_changed skip", events)
	}
}

func TestHuggingFaceBusyInspectionFailsClosedOnProcReadError(t *testing.T) {
	root := t.TempDir()
	procRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(procRoot, "123"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(procRoot, "123", "fd"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	busy, err := inspectHuggingFaceBusy(root, procRoot)
	if err == nil || busy {
		t.Fatalf("busy=%v err=%v, want fail-closed inspection error", busy, err)
	}
}

func TestHuggingFaceBusyInspectionTreatsDeletedCacheDescriptorAsBusy(t *testing.T) {
	root := t.TempDir()
	procRoot := t.TempDir()
	fdRoot := filepath.Join(procRoot, "123", "fd")
	if err := os.MkdirAll(fdRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	deletedTarget := filepath.Join(root, "blob") + " (deleted)"
	if err := os.Symlink(deletedTarget, filepath.Join(fdRoot, "4")); err != nil {
		t.Fatal(err)
	}
	busy, err := inspectHuggingFaceBusy(root, procRoot)
	if err != nil || !busy {
		t.Fatalf("busy=%v err=%v, want deleted cache descriptor to be busy", busy, err)
	}
}

func TestDeletedDescriptorInspectionSkipsForeignProcessFDs(t *testing.T) {
	root := t.TempDir()
	procRoot := t.TempDir()
	processRoot := filepath.Join(procRoot, "123")
	if err := os.MkdirAll(processRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(processRoot, "fd"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	foreignUID := uint32(os.Geteuid()) + 1
	if _, err := inspectDeletedDescriptorUnder(root, procRoot, foreignUID); err != nil {
		t.Fatalf("foreign process descriptors should be skipped: %v", err)
	}
	if _, err := inspectDeletedDescriptorUnder(root, procRoot, uint32(os.Geteuid())); err == nil {
		t.Fatal("target-user descriptor error must remain fail-closed")
	}
}

// TestHuggingFaceBusyInspectionTreatsSymlinkLockMarkerAsBusy verifies lock
// names are inspected without following symlinks, even though the walker
// deliberately skips symlink entries.
func TestHuggingFaceBusyInspectionTreatsSymlinkLockMarkerAsBusy(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "lock-target")
	if err := os.Symlink(outside, filepath.Join(root, "model.lock")); err != nil {
		t.Fatal(err)
	}
	busy, err := inspectHuggingFaceBusy(root, t.TempDir())
	if err != nil || !busy {
		t.Fatalf("busy=%v err=%v, want symlink lock marker to be busy", busy, err)
	}
}

func TestHuggingFaceBusyInspectionAllowsEmptyLocksDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".locks"), 0o700); err != nil {
		t.Fatal(err)
	}
	busy, err := inspectHuggingFaceBusy(root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if busy {
		t.Fatal("empty .locks directory should not mark cache busy")
	}
}

// TestHuggingFaceBusyInspectionTreatsOpenRootDirectoryAsBusy verifies an FD
// to the cache root itself blocks cleanup, not only FDs to descendants.
func TestHuggingFaceBusyInspectionTreatsOpenRootDirectoryAsBusy(t *testing.T) {
	root := t.TempDir()
	opened, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	procRoot := t.TempDir()
	fdRoot := filepath.Join(procRoot, "123", "fd")
	if err := os.MkdirAll(fdRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(fdRoot, "4")); err != nil {
		t.Fatal(err)
	}
	busy, err := inspectHuggingFaceBusy(root, procRoot)
	if err != nil || !busy {
		t.Fatalf("busy=%v err=%v, want open root directory to be busy", busy, err)
	}
}

func TestHuggingFaceBusyInspectionIgnoresCurrentPIDHeldRoot(t *testing.T) {
	root := t.TempDir()
	held, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	procRoot := t.TempDir()
	fdRoot := filepath.Join(procRoot, strconv.Itoa(os.Getpid()), "fd")
	if err := os.MkdirAll(fdRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(fdRoot, strconv.Itoa(int(held.Fd())))); err != nil {
		t.Fatal(err)
	}
	busy, err := inspectHuggingFaceBusyFD(int(held.Fd()), root, procRoot)
	if err != nil {
		t.Fatalf("busy=%v err=%v, want synthetic proc inspection to succeed", busy, err)
	}
	if busy {
		t.Fatal("cleaner's own held root descriptor should not mark HF cache busy")
	}
}

func TestHuggingFaceBusyInspectionKeepsOtherCurrentPIDCacheDescriptor(t *testing.T) {
	root := t.TempDir()
	held, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	other, err := os.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	procRoot := t.TempDir()
	fdRoot := filepath.Join(procRoot, strconv.Itoa(os.Getpid()), "fd")
	if err := os.MkdirAll(fdRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, fd := range []uintptr{held.Fd(), other.Fd()} {
		if err := os.Symlink(root, filepath.Join(fdRoot, strconv.Itoa(int(fd)))); err != nil {
			t.Fatal(err)
		}
	}
	busy, err := inspectHuggingFaceBusyFD(int(held.Fd()), root, procRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !busy {
		t.Fatal("other same-process cache descriptor should remain busy")
	}
}

func TestToolPruneHuggingFaceProfileIgnoresCleanerHeldRootDescriptor(t *testing.T) {
	binDir := t.TempDir()
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "huggingface")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
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
	if err := os.Symlink(root, filepath.Join(fdRoot, strconv.Itoa(rootFD))); err != nil {
		t.Fatal(err)
	}
	hf := filepath.Join(binDir, "hf")
	if err := os.WriteFile(hf, []byte("#!/bin/sh\nif [ \"$1\" = \"cache\" ] && [ \"$2\" = \"prune\" ] && [ \"$3\" = \"--help\" ]; then printf '%s\\n' 'hf cache prune --cache-dir ROOT --yes'; exit 0; fi\nif [ \"$1\" = \"cache\" ] && [ \"$2\" = \"prune\" ]; then /bin/touch \"$HOME/hf-ran\"; exit 0; fi\nprintf '%s\\n' 'hf 1.0'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	c := model.Cleaner{
		ID:        "hf",
		Scope:     model.ScopeUser,
		Type:      model.TypeToolPrune,
		ToolPrune: &model.ToolPruneCfg{Profile: "ai-huggingface-prune"},
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		e := NewToolPruneExecutor(home, nil, WithToolSearchPath(binDir), tempToolIdentityOption(t, home))
		e.procRoot = procRoot
		if err := e.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(filepath.Join(home, "hf-ran")); err != nil {
		t.Fatalf("HF tool should run after ignoring cleaner-held root descriptor: %v, events=%+v", err, events)
	}
	if hasCleanerSkip(events, "cache_busy") {
		t.Fatalf("cleaner's own held root descriptor marked tool profile busy: %+v", events)
	}
}

func TestHuggingFaceBusyInspectionFailsClosedOnWalkError(t *testing.T) {
	busy, err := inspectHuggingFaceBusy(filepath.Join(t.TempDir(), "missing"), t.TempDir())
	if err == nil || busy {
		t.Fatalf("busy=%v err=%v, want walk inspection error", busy, err)
	}
}

func TestToolProfilesDeclareExactCommandSafetyContract(t *testing.T) {
	type profileContract struct {
		binary       string
		normalArgs   []string
		dryArgs      []string
		nativeDryRun bool
		system       bool
		rootSuffix   string
		required     []string
		dryRequired  []string
	}
	wants := map[string]profileContract{
		"pkg-homebrew":             {"brew", []string{"cleanup", "--prune=5"}, []string{"cleanup", "--prune=5", "--dry-run"}, true, false, filepath.Join(".cache", "Homebrew"), nil, []string{"--dry-run"}},
		"lang-uv":                  {"uv", []string{"cache", "prune"}, []string{"cache", "prune"}, false, false, filepath.Join(".cache", "uv"), nil, nil},
		"lang-pnpm":                {"pnpm", []string{"store", "prune"}, []string{"store", "prune"}, false, false, filepath.Join(".local", "share", "pnpm", "store"), nil, nil},
		"dev-pre-commit":           {"pre-commit", []string{"gc"}, []string{"gc"}, false, false, filepath.Join(".cache", "pre-commit"), nil, nil},
		"lang-conda":               {"conda", []string{"clean", "--index-cache", "--tarballs", "--logfiles", "--yes"}, []string{"clean", "--index-cache", "--tarballs", "--logfiles", "--yes", "--dry-run", "--json"}, true, false, ".conda", nil, []string{"--dry-run", "--json"}},
		"test-cypress":             {"cypress", []string{"cache", "prune"}, []string{"cache", "prune"}, false, false, filepath.Join(".cache", "Cypress"), nil, nil},
		"build-ccache":             {"ccache", []string{"--cleanup"}, []string{"--cleanup"}, false, false, filepath.Join(".cache", "ccache"), nil, nil},
		"ai-huggingface-prune":     {"hf", []string{"cache", "prune", "--cache-dir", "/tmp/home/.cache/huggingface", "--yes"}, []string{"cache", "prune", "--cache-dir", "/tmp/home/.cache/huggingface", "--yes", "--dry-run"}, true, false, filepath.Join(".cache", "huggingface"), []string{"--cache-dir", "--yes"}, []string{"--dry-run"}},
		"dev-mise":                 {"mise", []string{"prune", "--yes"}, []string{"prune", "--yes"}, false, false, filepath.Join(".cache", "mise"), []string{"--yes"}, nil},
		"lang-corepack":            {"corepack", []string{"cache", "clean"}, []string{"cache", "clean"}, false, false, filepath.Join(".cache", "node", "corepack"), nil, nil},
		"sys-nix-gc":               {"nix-collect-garbage", nil, []string{"--dry-run"}, true, true, "/nix", nil, []string{"--dry-run"}},
		"desktop-flatpak-user":     {"flatpak", []string{"uninstall", "--user", "--unused", "--noninteractive"}, []string{"uninstall", "--user", "--unused", "--noninteractive"}, false, false, filepath.Join(".local", "share", "flatpak"), []string{"--noninteractive"}, nil},
		"desktop-flatpak-system":   {"flatpak", []string{"uninstall", "--system", "--unused", "--noninteractive"}, []string{"uninstall", "--system", "--unused", "--noninteractive"}, false, true, "/var/lib/flatpak", []string{"--noninteractive"}, nil},
		"lang-poetry-cache":        {"poetry", []string{"cache", "clear", "pypi", "--all", "--no-interaction"}, []string{"cache", "clear", "pypi", "--all", "--no-interaction"}, false, false, filepath.Join(".cache", "pypoetry"), nil, nil},
		"test-playwright-browsers": {"playwright", []string{"uninstall", "--all"}, []string{"uninstall", "--all"}, false, false, filepath.Join(".cache", "ms-playwright"), nil, nil},
	}
	if len(wants) != len(toolProfiles) {
		t.Fatalf("contract table covers %d profiles, registry has %d", len(wants), len(toolProfiles))
	}
	for name, want := range wants {
		profile, ok := toolProfiles[name]
		if !ok {
			t.Errorf("missing profile %q", name)
			continue
		}
		if profile.Binary != want.binary || profile.NativeDryRun != want.nativeDryRun || profile.System != want.system {
			t.Errorf("%s metadata = binary %q nativeDryRun %v system %v", name, profile.Binary, profile.NativeDryRun, profile.System)
		}
		if got := profile.BuildArgs("/tmp/home/.cache/huggingface", 5, false); !reflect.DeepEqual(got, want.normalArgs) {
			t.Errorf("%s normal argv = %#v, want %#v", name, got, want.normalArgs)
		}
		if got := profile.BuildArgs("/tmp/home/.cache/huggingface", 5, true); !reflect.DeepEqual(got, want.dryArgs) {
			t.Errorf("%s dry argv = %#v, want %#v", name, got, want.dryArgs)
		}
		identity := CommandIdentity{Home: "/tmp/home"}
		roots := profile.RootResolver(identity.Home)
		if len(roots) != 1 || roots[0] != filepath.Join(identity.Home, want.rootSuffix) && roots[0] != want.rootSuffix {
			t.Errorf("%s roots = %#v, want suffix %q", name, roots, want.rootSuffix)
		}
		if !reflect.DeepEqual(profile.Required, want.required) || !reflect.DeepEqual(profile.DryRunRequired, want.dryRequired) {
			t.Errorf("%s required flags = %#v/%#v, want %#v/%#v", name, profile.Required, profile.DryRunRequired, want.required, want.dryRequired)
		}
	}
}
