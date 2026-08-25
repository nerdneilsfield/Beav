package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

func TestManagedIdentityRejectsSameDeviceInodeAcrossMounts(t *testing.T) {
	base := managedIdentity{Dev: 7, Ino: 11, UID: 1000, Mode: unix.S_IFREG, MountID: 42}
	otherMount := base
	otherMount.MountID = 43
	if sameManagedIdentity(base, otherMount) {
		t.Fatal("same dev/inode on different mount must not match")
	}
}

func TestManagedCacheDeletesCompleteHuggingFaceArrowUnitAfterThirtyDays(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "huggingface", "datasets", "dataset")
	writeManagedAged(t, filepath.Join(unit, "dataset.arrow"), -31*24*time.Hour)
	setManagedDirAge(t, unit, -31*24*time.Hour)

	exec := newManagedTestExecutor(t, home)
	c := managedCleaner("ai-huggingface-datasets-arrow", 0)
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("complete arrow unit remains: %v", err)
	}
	if !hasDelete(events, unit) {
		t.Fatalf("missing unit deletion event: %+v", events)
	}
}

func TestManagedCacheDeletesCompleteNestedHuggingFaceArrowUnit(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "huggingface", "datasets", "nested-dataset")
	writeManagedAged(t, filepath.Join(unit, "dataset.arrow"), -31*24*time.Hour)
	writeManagedAged(t, filepath.Join(unit, "tables", "part.arrow"), -31*24*time.Hour)
	setManagedDirAge(t, filepath.Join(unit, "tables"), -31*24*time.Hour)
	setManagedDirAge(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("legal nested Arrow unit was not removed: %v", err)
	}
}

func TestManagedCacheRejectsBlacklistedEnvironmentRoot(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".ssh")
	unit := filepath.Join(root, "dataset")
	writeManagedAged(t, filepath.Join(unit, "dataset.arrow"), -31*24*time.Hour)
	setManagedDirAge(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.env = func(name string) string {
		if name == "HF_DATASETS_CACHE" {
			return root
		}
		return ""
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("blacklisted managed-cache unit was removed: %v", err)
	}
	if !hasCleanerSkip(events, "root_unavailable") {
		t.Fatalf("blacklisted managed-cache root was not rejected: %+v", events)
	}
}

func TestManagedCacheDirectoryMetricsAreMarkedIncomplete(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "huggingface", "datasets", "dataset")
	writeManagedAged(t, filepath.Join(unit, "dataset.arrow"), -31*24*time.Hour)
	setManagedDirAge(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	var finish model.Event
	for _, event := range events {
		if event.Event == model.EvFinish {
			finish = event
		}
	}
	if !finish.MetricsIncomplete {
		t.Fatalf("directory deletion metrics claimed complete: %+v", finish)
	}
	if finish.BytesFreed != 0 {
		t.Fatalf("directory st_size reported as freed bytes: %d", finish.BytesFreed)
	}
}

func TestManagedCacheDryRunDoesNotCreateJournalOrDelete(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -365*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), true, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("dry run removed unit: %v", err)
	}
	if _, err := os.Stat(exec.journalPath); !os.IsNotExist(err) {
		t.Fatalf("dry run created or changed journal: %v", err)
	}
}

func TestManagedCacheEnforcesThirtyDayFloor(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -29*24*time.Hour)

	exec := newManagedTestExecutor(t, home)
	c := managedCleaner("ai-torch-hub", 0)
	if err := exec.Run(context.Background(), c, false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("unit newer than mandatory floor was removed: %v", err)
	}
}

func TestManagedCacheSkipsIncompleteCheckpointSuffix(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt.part")
	writeManagedAged(t, unit, -365*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("incomplete checkpoint suffix was removed: %v", err)
	}
}

func TestManagedCacheSkipsIncompleteArrowUnitName(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "huggingface", "datasets", "dataset.incomplete")
	writeManagedAged(t, filepath.Join(unit, "dataset.arrow"), -365*24*time.Hour)
	setManagedDirAge(t, unit, -365*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("incomplete arrow unit name was removed: %v", err)
	}
}

func TestManagedCacheRejectsNoAgeFilter(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -365*24*time.Hour)

	exec := newManagedTestExecutor(t, home)
	c := managedCleaner("ai-torch-hub", 0)
	c.NoAgeFilter = true
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "no_age_filter_unsupported") {
		t.Fatalf("missing no_age_filter rejection: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("no_age_filter rejection removed unit: %v", err)
	}
}

func TestManagedCacheRenameCollisionLeavesOriginalAndCollision(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	collision := filepath.Join(filepath.Dir(unit), ".beav-managed-cache-00000000000000000000000000000000")
	writeManagedAged(t, collision, -31*24*time.Hour)

	exec := newManagedTestExecutor(t, home)
	exec.randomTombstone = func() (string, error) { return filepath.Base(collision), nil }
	c := managedCleaner("ai-torch-hub", 0)
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("rename collision removed original: %v", err)
	}
	if _, err := os.Stat(collision); err != nil {
		t.Fatalf("rename collision removed existing tombstone: %v", err)
	}
	if !hasManagedError(events, "rename_collision") {
		t.Fatalf("missing collision error: %+v", events)
	}
}

func TestManagedCacheDoesNotDeleteForgedTombstone(t *testing.T) {
	home := t.TempDir()
	forged := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", ".beav-managed-cache-forged")
	writeManagedAged(t, filepath.Join(forged, "payload"), -365*24*time.Hour)

	exec := newManagedTestExecutor(t, home)
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(forged); err != nil {
		t.Fatalf("forged tombstone was touched: %v", err)
	}
}

func TestManagedCacheIgnoresGenericForgedTombstoneName(t *testing.T) {
	home := t.TempDir()
	forged := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", ".beav-tombstone-forged")
	writeManagedAged(t, forged, -365*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(forged); err != nil {
		t.Fatalf("generic forged tombstone was touched: %v", err)
	}
}

func TestManagedCacheRevalidatesPathBeforeRename(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)

	exec := newManagedTestExecutor(t, home)
	exec.beforeRename = func(parentFD int, original, _ string) error {
		moved := original + ".replacement"
		if err := unix.Renameat(parentFD, original, parentFD, moved); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(filepath.Dir(unit), filepath.Base(original)), []byte("replacement"), 0o600)
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasManagedSkip(events, "toctou_changed") {
		t.Fatalf("missing path revalidation skip: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("replacement path missing: %v", err)
	}
	if _, err := os.Stat(unit + ".replacement"); err != nil {
		t.Fatalf("original unit was not preserved: %v", err)
	}
}

func TestManagedCacheRevalidatesAgeBeforeRename(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.beforeRename = func(int, string, string) error {
		now := time.Now()
		return os.Chtimes(unit, now, now)
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasManagedSkip(events, string(model.SkipReasonToctouChanged)) {
		t.Fatalf("mtime race was not rejected: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("mtime race removed unit: %v", err)
	}
}

func TestManagedCachePartialDeletionLeavesTrustedTombstoneForResume(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "huggingface", "datasets", "dataset")
	writeManagedAged(t, filepath.Join(unit, "dataset.arrow"), -31*24*time.Hour)
	setManagedDirAge(t, unit, -31*24*time.Hour)

	exec := newManagedTestExecutor(t, home)
	const tombstone = ".beav-managed-cache-11111111111111111111111111111111"
	exec.randomTombstone = func() (string, error) { return tombstone, nil }
	exec.deleteUnit = func(int, string) error { return errors.New("injected partial deletion") }
	firstEvents := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	for _, event := range firstEvents {
		if event.Event == model.EvFinish && !event.MetricsIncomplete {
			t.Fatalf("partial deletion reported complete metrics: %+v", event)
		}
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("original path should remain absent after rename: %v", err)
	}
	resumed := filepath.Join(filepath.Dir(unit), tombstone)
	if _, err := os.Stat(resumed); err != nil {
		t.Fatalf("incomplete trusted tombstone missing: %v", err)
	}

	exec = newManagedTestExecutor(t, home)
	if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(resumed); !os.IsNotExist(err) {
		t.Fatalf("trusted tombstone was not resumed: %v", err)
	}
}

func TestManagedCacheRemoveTombstoneFailureMarksMetricsIncomplete(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	tombstone := ".beav-managed-cache-33333333333333333333333333333333"
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.randomTombstone = func() (string, error) { return tombstone, nil }
	exec.removeTombstone = func(int, int, string, managedIdentity, managedUnitKind) error {
		return errors.New("injected tombstone removal failure")
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	for _, event := range events {
		if event.Event == model.EvFinish && !event.MetricsIncomplete {
			t.Fatalf("tombstone removal failure reported complete metrics: %+v", event)
		}
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(unit), tombstone)); err != nil {
		t.Fatalf("failed tombstone was not left for recovery: %v", err)
	}
}

func TestManagedCacheCompletionJournalFailureMarksKnownDeletionIncomplete(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	info, err := os.Stat(unit)
	if err != nil {
		t.Fatal(err)
	}
	exec := newManagedTestExecutor(t, home)
	appendCalls := 0
	exec.journalAppend = func(journal *managedJournal, record managedJournalRecord) error {
		appendCalls++
		if appendCalls == 2 {
			return errors.New("injected completion journal failure")
		}
		return journal.append(record)
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	var finish model.Event
	for _, event := range events {
		if event.Event == model.EvFinish {
			finish = event
		}
	}
	if !finish.MetricsIncomplete || finish.FilesDeleted != 1 || finish.BytesFreed != info.Size() {
		t.Fatalf("completion journal failure metrics = %+v", finish)
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("known-deleted unit remains after completion journal failure: %v", err)
	}
}

func TestManagedCacheRechecksOwnerImmediatelyBeforeRename(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	checks := 0
	exec := newManagedTestExecutor(t, home)
	exec.processCheck = func([]string, string) (bool, error) {
		checks++
		return checks > 2, nil
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasManagedSkip(events, "running_process") {
		t.Fatalf("missing immediate owner recheck: %+v", events)
	}
	if checks != 3 {
		t.Fatalf("owner checks = %d, want startup, candidate, and pre-rename checks", checks)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("owner recheck removed unit: %v", err)
	}
}

func TestManagedCacheDoesNotTreatGenericPythonAsOwner(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	procRoot := t.TempDir()
	processRoot := filepath.Join(procRoot, "123")
	if err := os.MkdirAll(filepath.Join(processRoot, "fd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(processRoot, "comm"), []byte("python3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(processRoot, "cmdline"), []byte("python3\x00worker.py\x00"), 0o600); err != nil {
		t.Fatal(err)
	}
	exec := newManagedTestExecutor(t, home)
	exec.procRoot = procRoot
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); !os.IsNotExist(err) {
		t.Fatalf("unrelated python process blocked managed-cache cleanup: %v", err)
	}
}

func TestManagedCacheSkipsSymlinkUnit(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "torch", "hub", "checkpoints")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(home, "outside-checkpoint")
	writeManagedAged(t, target, -365*24*time.Hour)
	symlink := filepath.Join(root, "model.pt")
	if err := os.Symlink(target, symlink); err != nil {
		t.Fatal(err)
	}
	exec := newManagedTestExecutor(t, home)
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasManagedSkip(events, string(model.SkipReasonSymlink)) {
		t.Fatalf("symlink unit was not skipped: %+v", events)
	}
	if _, err := os.Lstat(symlink); err != nil {
		t.Fatalf("symlink unit changed: %v", err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("symlink target changed: %v", err)
	}
}

func TestManagedCacheOpenDescriptorGuardLeavesUnit(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.openDescriptor = func(managedIdentity, string) (bool, error) { return true, nil }
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasManagedSkip(events, "open_file") {
		t.Fatalf("open descriptor guard missing: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("open unit was removed: %v", err)
	}
}

func TestManagedCacheOpenDescriptorGuardCoversArrowDescendants(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "huggingface", "datasets", "dataset")
	child := filepath.Join(unit, "dataset.arrow")
	writeManagedAged(t, child, -31*24*time.Hour)
	setManagedDirAge(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	called := false
	exec.openDescriptors = func(identities []managedIdentity, _ string, _ int) (bool, error) {
		called = true
		if len(identities) < 2 {
			t.Fatalf("subtree descriptor scan identities = %d, want unit and descendant", len(identities))
		}
		return true, nil
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !called {
		t.Fatal("subtree descriptor guard was not invoked")
	}
	if !hasManagedSkip(events, "open_file") {
		t.Fatalf("descendant open descriptor was not rejected: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("open descendant caused unit removal: %v", err)
	}
}

func TestManagedCacheProcVisibilityFailureLeavesUnit(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	if err := os.WriteFile(filepath.Join(exec.procRoot, "mounts"), []byte("proc /proc proc rw,nosuid,hidepid=2 0 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "proc_inspection_failed") {
		t.Fatalf("hidepid proc visibility was not rejected: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("hidepid inspection failure removed unit: %v", err)
	}
}

func TestManagedCacheSkipsHardlinkedArrowChild(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "huggingface", "datasets", "dataset")
	child := filepath.Join(unit, "dataset.arrow")
	outside := filepath.Join(home, "elsewhere.arrow")
	writeManagedAged(t, child, -31*24*time.Hour)
	if err := os.Link(child, outside); err != nil {
		t.Fatal(err)
	}
	setManagedDirAge(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("hardlinked Arrow child allowed unit removal: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("hardlink target changed: %v", err)
	}
}

func TestManagedCacheRejectsTorchHardlinkAddedBeforeRename(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	outside := filepath.Join(home, "elsewhere.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.beforeRename = func(parentFD int, original, _ string) error {
		return unix.Linkat(parentFD, original, unix.AT_FDCWD, outside, 0)
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("hardlink race removed original: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("hardlink race target missing: %v", err)
	}
}

func TestManagedCacheRejectsArrowHardlinkAddedBeforeRename(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "huggingface", "datasets", "dataset")
	child := filepath.Join(unit, "dataset.arrow")
	outside := filepath.Join(home, "elsewhere.arrow")
	writeManagedAged(t, child, -31*24*time.Hour)
	setManagedDirAge(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.beforeRename = func(parentFD int, original, _ string) error {
		return unix.Linkat(parentFD, filepath.Join(original, "dataset.arrow"), unix.AT_FDCWD, outside, 0)
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("Arrow child hardlink race removed original: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("Arrow hardlink race target missing: %v", err)
	}
}

func TestManagedCacheRenameUnsupportedLeavesUnit(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.renameat2 = func(int, string, int, string, uint) error { return unix.ENOSYS }
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasManagedError(events, "rename_unsupported") {
		t.Fatalf("unsupported rename was not reported: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("unsupported rename removed unit: %v", err)
	}
}

func TestManagedCacheNilRenameImplementationFailsClosed(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.renameat2 = nil
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasManagedError(events, "rename_unsupported") {
		t.Fatalf("nil rename implementation was not rejected: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("nil rename implementation removed unit: %v", err)
	}
}

func TestManagedCacheHeldFDProtectsAgainstPostRenamePathReopen(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "huggingface", "datasets", "dataset")
	writeManagedAged(t, filepath.Join(unit, "dataset.arrow"), -31*24*time.Hour)
	setManagedDirAge(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.renameat2 = func(oldDir int, old string, newDir int, newName string, flags uint) error {
		if err := unix.Renameat2(oldDir, old, newDir, newName, flags); err != nil {
			return err
		}
		return os.Mkdir(filepath.Join(filepath.Dir(unit), old), 0o755)
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-huggingface-datasets-arrow", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("replacement original path missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unit, "dataset.arrow")); !os.IsNotExist(err) {
		t.Fatalf("held unit content was reopened through replacement path: %v", err)
	}
}

func TestManagedCacheJournalBindsRecoveryToIdentity(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	if err := os.MkdirAll(filepath.Dir(exec.journalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := `{"profile":"ai-torch-hub","root":"` + filepath.Dir(unit) + `","original_leaf":"model.pt","tombstone_leaf":".beav-managed-cache-forged","uid":` + fmt.Sprint(os.Getuid()) + `,"dev":1,"inode":1}` + "\n"
	if err := os.WriteFile(exec.journalPath, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("identity-mismatched record removed current unit: %v", err)
	}
}

func TestManagedCacheMalformedRecoveryRecordDoesNotAuthorizeCleanup(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	if err := os.MkdirAll(filepath.Dir(exec.journalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	bad := `{"profile":"ai-torch-hub","root":"` + filepath.Dir(unit) + `","original_leaf":"model.pt","tombstone_leaf":"not-a-tombstone","uid":` + fmt.Sprint(os.Getuid()) + `,"dev":1,"inode":1}` + "\n"
	if err := os.WriteFile(exec.journalPath, []byte(bad), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("malformed record authorized deletion: %v", err)
	}
}

func TestManagedCacheGenericTombstoneRecoveryNameIsUntrusted(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "torch", "hub", "checkpoints")
	tombstone := filepath.Join(root, ".beav-tombstone-forged")
	writeManagedAged(t, tombstone, -31*24*time.Hour)
	var st unix.Stat_t
	if err := unix.Stat(tombstone, &st); err != nil {
		t.Fatal(err)
	}
	exec := newManagedTestExecutor(t, home)
	if err := os.MkdirAll(filepath.Dir(exec.journalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf(`{"profile":"ai-torch-hub","root":%q,"original_leaf":"missing.pt","tombstone_leaf":".beav-tombstone-forged","uid":%d,"dev":%d,"inode":%d}`+"\n", root, os.Getuid(), st.Dev, st.Ino)
	if err := os.WriteFile(exec.journalPath, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tombstone); err != nil {
		t.Fatalf("generic forged recovery tombstone was touched: %v", err)
	}
}

func TestManagedCacheForgedManagedTombstoneShapeIsUntrusted(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "torch", "hub", "checkpoints")
	tombstone := filepath.Join(root, ".beav-managed-cache-forged")
	writeManagedAged(t, tombstone, -31*24*time.Hour)
	var st unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, tombstone, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &st); err != nil {
		t.Fatal(err)
	}
	exec := newManagedTestExecutor(t, home)
	if err := os.MkdirAll(filepath.Dir(exec.journalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf(`{"profile":"ai-torch-hub","root":%q,"original_leaf":"missing.pt","tombstone_leaf":".beav-managed-cache-forged","uid":%d,"dev":%d,"inode":%d,"mount_id":%d}`+"\n", root, os.Getuid(), unix.Mkdev(st.Dev_major, st.Dev_minor), st.Ino, st.Mnt_id)
	if err := os.WriteFile(exec.journalPath, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tombstone); err != nil {
		t.Fatalf("forged managed tombstone was touched: %v", err)
	}
}

func TestManagedCacheJournalParentFsyncPrecedesRename(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	synced := false
	exec.journalParentSync = func(fd int) error {
		synced = true
		return unix.Fsync(fd)
	}
	exec.renameat2 = func(oldDir int, old string, newDir int, newName string, flags uint) error {
		if !synced {
			t.Fatal("rename started before journal containing directory fsync")
		}
		return unix.Renameat2(oldDir, old, newDir, newName, flags)
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if !synced {
		t.Fatal("journal containing directory was not fsynced")
	}
}

func TestEnsureManagedOwnerRepairsMismatchedOwner(t *testing.T) {
	called := false
	err := ensureManagedOwner(12, 0, 1000, func(fd, uid, gid int) error {
		called = true
		if fd != 12 || uid != 1000 || gid != -1 {
			t.Fatalf("fchown args = (%d, %d, %d)", fd, uid, gid)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("mismatched journal owner was not repaired")
	}
}

func TestManagedCacheJournalParentFsyncFailureFailsClosed(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.journalParentSync = func(int) error { return errors.New("injected journal parent fsync failure") }
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "journal_unavailable") {
		t.Fatalf("journal fsync failure was not fail-closed: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("journal fsync failure removed unit: %v", err)
	}
}

func TestManagedCacheJournalFsyncsEveryCreatedParent(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.journalPath = filepath.Join(home, ".one", "two", "three", "recovery.jsonl")
	syncs := 0
	exec.journalDirSync = func(int) error {
		syncs++
		return nil
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if syncs != 3 {
		t.Fatalf("created journal parent fsyncs = %d, want one per created level", syncs)
	}
}

func TestManagedCacheJournalDirectoryFsyncFailureFailsClosed(t *testing.T) {
	home := t.TempDir()
	unit := filepath.Join(home, ".cache", "torch", "hub", "checkpoints", "model.pt")
	writeManagedAged(t, unit, -31*24*time.Hour)
	exec := newManagedTestExecutor(t, home)
	exec.journalPath = filepath.Join(home, ".one", "two", "recovery.jsonl")
	exec.journalDirSync = func(int) error { return errors.New("injected parent directory fsync failure") }
	events := captureEvents(t, func(emit func(model.Event)) {
		if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, emit); err != nil {
			t.Fatal(err)
		}
	})
	if !hasCleanerSkip(events, "journal_unavailable") {
		t.Fatalf("parent directory fsync failure was not fail-closed: %+v", events)
	}
	if _, err := os.Stat(unit); err != nil {
		t.Fatalf("parent directory fsync failure removed unit: %v", err)
	}
}

func TestManagedCacheTornJournalRecordDoesNotAuthorizeRecovery(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".cache", "torch", "hub", "checkpoints")
	tombstone := filepath.Join(root, ".beav-managed-cache-22222222222222222222222222222222")
	writeManagedAged(t, tombstone, -31*24*time.Hour)
	var st unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, tombstone, unix.AT_SYMLINK_NOFOLLOW, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &st); err != nil {
		t.Fatal(err)
	}
	exec := newManagedTestExecutor(t, home)
	if err := os.MkdirAll(filepath.Dir(exec.journalPath), 0o700); err != nil {
		t.Fatal(err)
	}
	record := fmt.Sprintf(`{"profile":"ai-torch-hub","root":%q,"original_leaf":"missing.pt","tombstone_leaf":".beav-managed-cache-22222222222222222222222222222222","uid":%d,"dev":%d,"inode":%d,"mount_id":%d}`, root, os.Getuid(), unix.Mkdev(st.Dev_major, st.Dev_minor), st.Ino, st.Mnt_id)
	if err := os.WriteFile(exec.journalPath, []byte(record), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := exec.Run(context.Background(), managedCleaner("ai-torch-hub", 0), false, func(model.Event) {}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(tombstone); err != nil {
		t.Fatalf("torn journal record authorized cleanup: %v", err)
	}
}

func newManagedTestExecutor(t *testing.T, home string) *ManagedCacheExecutor {
	t.Helper()
	exec := NewManagedCacheExecutor(home, safety.NewWhitelist(nil), uint32(os.Getuid()))
	exec.procRoot = t.TempDir()
	exec.journalPath = filepath.Join(home, ".state", "managed-cache-recovery.jsonl")
	exec.env = func(string) string { return "" }
	return exec
}

func managedCleaner(profile string, age int) model.Cleaner {
	return model.Cleaner{
		ID:         "managed-" + strings.ReplaceAll(profile, ".", "-"),
		Name:       profile,
		Scope:      model.ScopeUser,
		Type:       model.TypeManagedCache,
		MinAgeDays: ptrInt(age),
		ManagedCache: &model.ManagedCacheCfg{
			Profile: profile,
		},
	}
}

func writeManagedAged(t *testing.T, path string, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		if err := os.WriteFile(path, []byte("managed-cache-fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	when := time.Now().Add(age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func setManagedDirAge(t *testing.T, path string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(age)
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func hasManagedError(events []model.Event, reason string) bool {
	for _, event := range events {
		if event.Event == model.EvError && event.Reason == reason {
			return true
		}
	}
	return false
}

func hasManagedSkip(events []model.Event, reason string) bool {
	for _, event := range events {
		if event.Event == model.EvSkipped && event.Reason == reason {
			return true
		}
	}
	return false
}
