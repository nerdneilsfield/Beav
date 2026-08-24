package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

const managedCacheMinAgeDays = 30

const managedTombstonePrefix = ".beav-managed-cache-"

var (
	errManagedRootUnavailable = errors.New("managed cache root unavailable")
	errManagedRootChanged     = errors.New("managed cache root identity changed")
)

type managedUnitKind uint8

const (
	managedUnitDirectory managedUnitKind = iota + 1
	managedUnitFile
)

type managedCacheProfile struct {
	minAgeDays     int
	kind           managedUnitKind
	resolveRoot    func(*ManagedCacheExecutor) (string, error)
	ownerProcesses []string
	complete       func(int, string) (bool, error)
}

type managedIdentity struct {
	Dev       uint64
	Ino       uint64
	UID       uint32
	Mode      uint32
	Size      int64
	MtimeSec  int64
	MtimeNsec int64
}

// ManagedCacheExecutor cleans only the closed, built-in managed-cache
// profiles. Roots and unit rules are never accepted from YAML.
type ManagedCacheExecutor struct {
	Home      string
	Whitelist *safety.Whitelist
	TargetUID uint32

	procRoot        string
	env             func(string) string
	processCheck    func([]string, string) (bool, error)
	procVisibility  func(string, uint32) error
	beforeRootOpen  func(string) error
	beforeRename    func(int, string, string) error
	fdIdentity      func(int) (managedIdentity, error)
	openDescriptor  func(managedIdentity, string) (bool, error)
	renameat2       func(int, string, int, string, uint) error
	randomTombstone func() (string, error)
	deleteUnit      func(int, string) error
	now             func() time.Time

	// journalPath is a test seam. The default is a fixed target-home state
	// path; callers cannot supply it through cleaner YAML.
	journalPath string
}

// NewManagedCacheExecutor creates a closed managed-cache executor. The
// optional UID lets callers bind cleanup to a resolved target account.
func NewManagedCacheExecutor(home string, wl *safety.Whitelist, targetUID ...uint32) *ManagedCacheExecutor {
	uid := uint32(os.Getuid())
	if len(targetUID) > 0 {
		uid = targetUID[0]
	}
	e := &ManagedCacheExecutor{
		Home:            filepath.Clean(home),
		Whitelist:       wl,
		TargetUID:       uid,
		procRoot:        "/proc",
		env:             os.Getenv,
		processCheck:    checkedManagedProcessRunning,
		procVisibility:  safety.VerifyProcVisibility,
		fdIdentity:      readManagedFDIdentity,
		renameat2:       unix.Renameat2,
		randomTombstone: randomManagedTombstone,
		now:             time.Now,
	}
	e.journalPath = filepath.Join(e.Home, ".local", "state", "beav", "managed-cache-recovery.jsonl")
	return e
}

type managedRunStats struct {
	filesDeleted   int64
	bytesFreed     int64
	filesCandidate int64
	bytesCandidate int64
	errors         int
}

// Run executes one closed managed-cache profile.
func (e *ManagedCacheExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := e.now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	finish := func(status string, stats managedRunStats) {
		emit(model.Event{
			Event:          model.EvFinish,
			CleanerID:      c.ID,
			Status:         status,
			FilesDeleted:   stats.filesDeleted,
			BytesFreed:     stats.bytesFreed,
			FilesCandidate: stats.filesCandidate,
			BytesCandidate: stats.bytesCandidate,
			Errors:         stats.errors,
			DurationMs:     e.now().Sub(start).Milliseconds(),
			TS:             e.now(),
		})
	}
	skip := func(reason, detail string) error {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: reason, Detail: detail, TS: e.now()})
		finish("skipped", managedRunStats{})
		return nil
	}
	fail := func(reason, detail string) error {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: reason, Detail: detail, TS: e.now()})
		finish("error", managedRunStats{errors: 1})
		return nil
	}

	profile, ok := managedCacheProfiles[c.Profile()]
	if !ok {
		return fail("internal", "unknown managed_cache profile")
	}
	if c.Scope != model.ScopeUser {
		return skip("profile_scope_mismatch", "managed cache profiles are user-scoped")
	}
	if c.NoAgeFilter {
		return skip("no_age_filter_unsupported", "managed cache requires an age floor")
	}
	root, err := profile.resolveRoot(e)
	if err != nil {
		return skip("root_unavailable", err.Error())
	}
	parentFD, rootFD, rootIdentity, err := e.openManagedRoot(root)
	if err != nil {
		reason := "root_unavailable"
		if errors.Is(err, safety.ErrSymlink) {
			reason = string(model.SkipReasonSymlink)
		} else if errors.Is(err, safety.ErrCrossFS) {
			reason = string(model.SkipReasonCrossFS)
		} else if errors.Is(err, errManagedRootChanged) {
			reason = "root_identity_changed"
		}
		return skip(reason, err.Error())
	}
	defer unix.Close(rootFD)
	defer unix.Close(parentFD)

	if e.procVisibility != nil {
		if err := e.procVisibility(e.procRoot, e.TargetUID); err != nil {
			return skip("proc_inspection_failed", err.Error())
		}
	}
	if busy, checkErr := e.checkOwner(profile.ownerProcesses); checkErr != nil {
		return skip("proc_inspection_failed", checkErr.Error())
	} else if busy {
		return skip(string(model.SkipReasonRunningProcess), "profile owner process is running")
	}

	var journal *managedJournal
	var records []managedJournalRecord
	if !dryRun {
		journal, err = e.openManagedJournal()
		if err != nil {
			return skip("journal_unavailable", err.Error())
		}
		defer journal.Close()
		records, err = loadManagedJournal(journal.fd)
		if err != nil {
			return fail("journal_corrupt", err.Error())
		}
	}

	stats := managedRunStats{}
	handled, resumeErrs := e.resumeManagedRecords(ctx, c, profile, root, rootFD, rootIdentity, journal, records, dryRun, emit, &stats)
	stats.errors += resumeErrs
	if ctx.Err() != nil {
		return ctx.Err()
	}

	names, err := readManagedDirNames(rootFD)
	if err != nil {
		return fail("enumeration_failed", err.Error())
	}
	sort.Strings(names)
	for _, leaf := range names {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if isManagedTombstone(leaf) || handled[leaf] {
			continue
		}
		identity, statErr := managedStatAt(rootFD, leaf)
		if statErr != nil {
			if errors.Is(statErr, unix.ENOENT) {
				continue
			}
			stats.errors++
			emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: filepath.Join(root, leaf), Reason: "stat_failed", Detail: statErr.Error(), TS: e.now()})
			continue
		}
		if identity.Dev != rootIdentity.Dev {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: filepath.Join(root, leaf), Reason: string(model.SkipReasonCrossFS), TS: e.now()})
			continue
		}
		if identity.UID != e.TargetUID {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: filepath.Join(root, leaf), Reason: "wrong_owner", TS: e.now()})
			continue
		}
		if !managedKindMatches(identity.Mode, profile.kind) {
			reason := string(model.SkipReasonWrongType)
			if identity.Mode == unix.S_IFLNK {
				reason = string(model.SkipReasonSymlink)
			}
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: filepath.Join(root, leaf), Reason: reason, TS: e.now()})
			continue
		}
		result := e.processManagedCandidate(ctx, c, profile, root, rootFD, rootIdentity, leaf, identity, nil, false, dryRun, journal, emit, &stats)
		if result == managedCandidateError {
			stats.errors++
		}
	}

	status := "ok"
	if stats.errors > 0 {
		status = "error"
	}
	finish(status, stats)
	return nil
}

type managedCandidateResult uint8

const (
	managedCandidateSkipped managedCandidateResult = iota
	managedCandidateDeleted
	managedCandidateError
)

func (e *ManagedCacheExecutor) processManagedCandidate(
	ctx context.Context,
	c model.Cleaner,
	profile managedCacheProfile,
	root string,
	parentFD int,
	rootIdentity managedIdentity,
	leaf string,
	identity managedIdentity,
	record *managedJournalRecord,
	renamed bool,
	dryRun bool,
	journal *managedJournal,
	emit func(model.Event),
	stats *managedRunStats,
) managedCandidateResult {
	originalLeaf := leaf
	if record != nil {
		originalLeaf = record.OriginalLeaf
	}
	abs := filepath.Join(root, originalLeaf)
	if e.Whitelist != nil && e.Whitelist.Intersects(abs) {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonWhitelisted), TS: e.now()})
		return managedCandidateSkipped
	}
	minAge := profile.minAgeDays
	if requested := c.AgeOrDefault(14); requested > minAge {
		minAge = requested
	}
	if !renamed && !managedIdentityOldEnough(identity, minAge, e.now()) {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonAgeTooRecent), TS: e.now()})
		return managedCandidateSkipped
	}
	if busy, checkErr := e.checkOwner(profile.ownerProcesses); checkErr != nil {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "proc_inspection_failed", Detail: checkErr.Error(), TS: e.now()})
		return managedCandidateSkipped
	} else if busy {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonRunningProcess), TS: e.now()})
		return managedCandidateSkipped
	}
	if open, checkErr := e.managedDescriptorBusy(identity, -1); checkErr != nil {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "proc_inspection_failed", Detail: checkErr.Error(), TS: e.now()})
		return managedCandidateSkipped
	} else if open {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "open_file", TS: e.now()})
		return managedCandidateSkipped
	}
	unitFD, err := openManagedUnit(parentFD, leaf, profile.kind)
	if err != nil {
		emitManagedOpenSkip(c.ID, abs, err, emit, e.now)
		return managedCandidateSkipped
	}
	defer unix.Close(unitFD)
	openedIdentity, err := e.fdIdentity(unitFD)
	if err != nil {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "stat_failed", Detail: err.Error(), TS: e.now()})
		return managedCandidateSkipped
	}
	if !sameManagedIdentity(identity, openedIdentity) || openedIdentity.Dev != rootIdentity.Dev || openedIdentity.UID != e.TargetUID {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonToctouChanged), TS: e.now()})
		return managedCandidateSkipped
	}
	if !renamed {
		complete, completeErr := profile.complete(unitFD, leaf)
		if completeErr != nil {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "completeness_unproven", Detail: completeErr.Error(), TS: e.now()})
			return managedCandidateSkipped
		}
		if !complete {
			return managedCandidateSkipped
		}
	}
	if dryRun {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonDryRun), Size: identity.Size, TS: e.now()})
		stats.filesCandidate++
		stats.bytesCandidate += identity.Size
		return managedCandidateSkipped
	}

	if record == nil {
		tombstone, randomErr := e.randomTombstone()
		if randomErr != nil {
			emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "tombstone_generation_failed", Detail: randomErr.Error(), TS: e.now()})
			return managedCandidateError
		}
		if !validManagedTombstone(tombstone) {
			emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "tombstone_generation_failed", Detail: "invalid tombstone leaf", TS: e.now()})
			return managedCandidateError
		}
		record = &managedJournalRecord{
			Profile:       c.Profile(),
			Root:          root,
			OriginalLeaf:  leaf,
			TombstoneLeaf: tombstone,
			UID:           e.TargetUID,
			Dev:           identity.Dev,
			Inode:         identity.Ino,
		}
		if err := journal.append(*record); err != nil {
			emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "journal_write_failed", Detail: err.Error(), TS: e.now()})
			return managedCandidateError
		}
	}
	if record.UID != e.TargetUID || record.Dev != openedIdentity.Dev || record.Inode != openedIdentity.Ino {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonToctouChanged), TS: e.now()})
		return managedCandidateSkipped
	}

	if !renamed {
		if e.renameat2 == nil {
			e.emitManagedRenameFailure(c.ID, abs, unix.ENOSYS, emit)
			return managedCandidateError
		}
		if e.beforeRename != nil {
			if hookErr := e.beforeRename(parentFD, record.OriginalLeaf, record.TombstoneLeaf); hookErr != nil {
				e.emitManagedRenameFailure(c.ID, abs, hookErr, emit)
				return managedCandidateError
			}
		}
		current, statErr := managedStatAt(parentFD, record.OriginalLeaf)
		if statErr != nil {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonToctouChanged), Detail: statErr.Error(), TS: e.now()})
			return managedCandidateSkipped
		}
		if !sameManagedIdentity(current, openedIdentity) {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonToctouChanged), TS: e.now()})
			return managedCandidateSkipped
		}
		if !sameManagedState(current, openedIdentity) || !managedIdentityOldEnough(current, minAge, e.now()) {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonToctouChanged), TS: e.now()})
			return managedCandidateSkipped
		}
		if busy, checkErr := e.checkOwner(profile.ownerProcesses); checkErr != nil {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "proc_inspection_failed", Detail: checkErr.Error(), TS: e.now()})
			return managedCandidateSkipped
		} else if busy {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonRunningProcess), TS: e.now()})
			return managedCandidateSkipped
		}
		if open, checkErr := e.managedDescriptorBusy(openedIdentity, unitFD); checkErr != nil {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "proc_inspection_failed", Detail: checkErr.Error(), TS: e.now()})
			return managedCandidateSkipped
		} else if open {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "open_file", TS: e.now()})
			return managedCandidateSkipped
		}
		if err := e.renameat2(parentFD, record.OriginalLeaf, parentFD, record.TombstoneLeaf, unix.RENAME_NOREPLACE); err != nil {
			e.emitManagedRenameFailure(c.ID, abs, err, emit)
			return managedCandidateError
		}
		renamed = true
	}

	tombstoneIdentity, err := managedStatAt(parentFD, record.TombstoneLeaf)
	if err != nil || !sameManagedIdentity(tombstoneIdentity, openedIdentity) {
		if err == nil {
			err = errManagedRootChanged
		}
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "rename_identity_changed", Detail: err.Error(), TS: e.now()})
		return managedCandidateError
	}
	if e.deleteUnit != nil {
		err = e.deleteUnit(unitFD, filepath.Join(root, record.TombstoneLeaf))
	} else if profile.kind == managedUnitDirectory {
		err = deleteManagedDirectory(unitFD, filepath.Join(root, record.TombstoneLeaf))
	}
	if err != nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "partial_delete", Detail: err.Error(), TS: e.now()})
		return managedCandidateError
	}
	if err := removeManagedTombstone(parentFD, unitFD, record.TombstoneLeaf, openedIdentity, profile.kind); err != nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "tombstone_remove_failed", Detail: err.Error(), TS: e.now()})
		return managedCandidateError
	}
	record.Complete = true
	if err := journal.append(*record); err != nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "journal_write_failed", Detail: err.Error(), TS: e.now()})
		return managedCandidateError
	}
	emit(model.Event{Event: model.EvDeleted, CleanerID: c.ID, Path: abs, Size: identity.Size, TS: e.now()})
	stats.filesDeleted++
	stats.bytesFreed += identity.Size
	return managedCandidateDeleted
}

func (e *ManagedCacheExecutor) resumeManagedRecords(
	ctx context.Context,
	c model.Cleaner,
	profile managedCacheProfile,
	root string,
	parentFD int,
	rootIdentity managedIdentity,
	journal *managedJournal,
	records []managedJournalRecord,
	dryRun bool,
	emit func(model.Event),
	stats *managedRunStats,
) (map[string]bool, int) {
	handled := make(map[string]bool)
	errs := 0
	for _, record := range records {
		if ctx.Err() != nil || record.Complete || record.Profile != c.Profile() || filepath.Clean(record.Root) != filepath.Clean(root) {
			continue
		}
		if safeManagedLeaf(record.OriginalLeaf) {
			handled[record.OriginalLeaf] = true
		}
		if record.UID != e.TargetUID || record.Dev == 0 || record.Inode == 0 || !safeManagedLeaf(record.OriginalLeaf) || !validManagedTombstone(record.TombstoneLeaf) {
			continue
		}
		original, originalErr := managedStatAt(parentFD, record.OriginalLeaf)
		tombstone, tombstoneErr := managedStatAt(parentFD, record.TombstoneLeaf)
		originalMatch := originalErr == nil && original.Dev == record.Dev && original.Ino == record.Inode && original.UID == record.UID
		tombstoneMatch := tombstoneErr == nil && tombstone.Dev == record.Dev && tombstone.Ino == record.Inode && tombstone.UID == record.UID
		if originalMatch && tombstoneErr != nil && errors.Is(tombstoneErr, unix.ENOENT) {
			result := e.processManagedCandidate(ctx, c, profile, root, parentFD, rootIdentity, record.OriginalLeaf, original, &record, false, dryRun, journal, emit, stats)
			if result == managedCandidateError {
				errs++
			}
			continue
		}
		if tombstoneMatch && originalErr != nil && errors.Is(originalErr, unix.ENOENT) {
			result := e.processManagedCandidate(ctx, c, profile, root, parentFD, rootIdentity, record.TombstoneLeaf, tombstone, &record, true, dryRun, journal, emit, stats)
			if result == managedCandidateError {
				errs++
			}
			continue
		}
		// Identity mismatch, both names present, or untrusted tombstone: leave
		// every pathname untouched and do not attempt a restore.
	}
	return handled, errs
}

func (e *ManagedCacheExecutor) checkOwner(names []string) (bool, error) {
	if e.processCheck == nil || len(names) == 0 {
		return false, nil
	}
	return e.processCheck(names, e.procRoot)
}

func (e *ManagedCacheExecutor) managedDescriptorBusy(identity managedIdentity, heldFD int) (bool, error) {
	if e.openDescriptor != nil {
		return e.openDescriptor(identity, e.procRoot)
	}
	want := []safety.FileIdentity{{Dev: identity.Dev, Ino: identity.Ino}}
	if heldFD >= 0 {
		return safety.OpenDescriptorMatchesExceptFD(e.procRoot, want, os.Getpid(), heldFD)
	}
	return safety.OpenDescriptorsMatch(e.procRoot, want)
}

func (e *ManagedCacheExecutor) emitManagedRenameFailure(cleanerID, path string, err error, emit func(model.Event)) {
	reason := "rename_failed"
	switch {
	case errors.Is(err, unix.EEXIST):
		reason = "rename_collision"
	case errors.Is(err, unix.EXDEV):
		reason = string(model.SkipReasonCrossFS)
	case errors.Is(err, unix.ENOSYS), errors.Is(err, unix.EINVAL), errors.Is(err, unix.EOPNOTSUPP), errors.Is(err, unix.EPERM):
		reason = "rename_unsupported"
	}
	emit(model.Event{Event: model.EvError, CleanerID: cleanerID, Path: path, Reason: reason, Detail: err.Error(), TS: e.now()})
}

func (e *ManagedCacheExecutor) openManagedRoot(root string) (int, int, managedIdentity, error) {
	home := filepath.Clean(e.Home)
	root = filepath.Clean(root)
	if !filepath.IsAbs(home) || !managedPathWithin(root, home) || root == home {
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: root outside target home", errManagedRootUnavailable)
	}
	homeFD, err := safety.OpenAnchoredDirFD(home, home)
	if err != nil {
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: home: %v", errManagedRootUnavailable, err)
	}
	defer unix.Close(homeFD)
	homeIdentity, err := e.fdIdentity(homeFD)
	if err != nil {
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: home identity: %v", errManagedRootUnavailable, err)
	}
	if homeIdentity.Mode != unix.S_IFDIR || homeIdentity.UID != e.TargetUID {
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: target home ownership/type invalid", errManagedRootUnavailable)
	}
	parentPath := filepath.Dir(root)
	parentFD, err := safety.OpenAnchoredDirFDFromFD(homeFD, home, parentPath)
	if err != nil {
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: root parent: %v", errManagedRootUnavailable, err)
	}
	closeParent := true
	defer func() {
		if closeParent {
			_ = unix.Close(parentFD)
		}
	}()
	leaf := filepath.Base(root)
	selected, err := managedStatAt(parentFD, leaf)
	if err != nil {
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: selected root: %v", errManagedRootUnavailable, err)
	}
	if selected.Mode == unix.S_IFLNK {
		return -1, -1, managedIdentity{}, safety.ErrSymlink
	}
	if selected.Mode != unix.S_IFDIR {
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: selected root is not a directory", errManagedRootUnavailable)
	}
	if selected.UID != e.TargetUID {
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: selected root owner differs", errManagedRootUnavailable)
	}
	if selected.Dev != homeIdentity.Dev {
		return -1, -1, managedIdentity{}, safety.ErrCrossFS
	}
	if e.beforeRootOpen != nil {
		if hookErr := e.beforeRootOpen(root); hookErr != nil {
			return -1, -1, managedIdentity{}, fmt.Errorf("%w: pre-open hook: %v", errManagedRootChanged, hookErr)
		}
	}
	rootFD, err := unix.Openat(parentFD, leaf, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: open root: %v", errManagedRootChanged, err)
	}
	opened, err := e.fdIdentity(rootFD)
	if err != nil {
		_ = unix.Close(rootFD)
		return -1, -1, managedIdentity{}, fmt.Errorf("%w: opened root identity: %v", errManagedRootChanged, err)
	}
	if !sameManagedIdentity(selected, opened) || opened.Dev != homeIdentity.Dev || opened.UID != e.TargetUID {
		_ = unix.Close(rootFD)
		return -1, -1, managedIdentity{}, errManagedRootChanged
	}
	closeParent = false
	return parentFD, rootFD, opened, nil
}
