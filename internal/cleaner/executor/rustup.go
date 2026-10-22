package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

// rustupOwnerProcesses are the processes whose activity makes toolchain
// removal unsafe. A running rustc or cargo may hold toolchain files open.
var rustupOwnerProcesses = []string{"rustc", "cargo", "rustup", "rust-analyzer"}

// knownRustupArches tokens mark the start of the target triple in a
// toolchain directory name; everything before them is the channel spec.
var knownRustupArches = map[string]struct{}{
	"x86_64": {}, "i686": {}, "aarch64": {}, "arm": {}, "armv7": {},
	"mips": {}, "mips64": {}, "mips64el": {}, "powerpc": {}, "powerpc64": {},
	"powerpc64le": {}, "riscv64": {}, "s390x": {}, "wasm32": {}, "wasm64": {},
	"loongarch64": {}, "sparc64": {},
}

// RustupExecutor removes stale per-release-line Rust toolchains. Roots,
// channel parsing, and keep rules are closed; YAML only selects the profile.
type RustupExecutor struct {
	Home      string
	Whitelist *safety.Whitelist
	TargetUID uint32

	procRoot       string
	env            func(string) string
	processCheck   func([]string, string) (bool, error)
	procVisibility func(string, uint32) error
	fdIdentity     func(int) (managedIdentity, error)
	now            func() time.Time
	dryRunFlag     bool
}

// NewRustupExecutor creates a closed Rust toolchain executor. The optional
// UID lets callers bind cleanup to a resolved target account.
func NewRustupExecutor(home string, wl *safety.Whitelist, targetUID ...uint32) *RustupExecutor {
	uid := systemID(os.Getuid())
	if len(targetUID) > 0 {
		uid = targetUID[0]
	}
	return &RustupExecutor{
		Home:           filepath.Clean(home),
		Whitelist:      wl,
		TargetUID:      uid,
		procRoot:       "/proc",
		env:            os.Getenv,
		processCheck:   checkedManagedProcessRunning,
		procVisibility: safety.VerifyProcVisibility,
		fdIdentity:     readManagedFDIdentity,
		now:            time.Now,
	}
}

// Run executes the closed lang-rustup profile.
func (e *RustupExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	e.dryRunFlag = dryRun
	start := e.now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	finish := func(status string, files, bytes int64, errs int) {
		emit(model.Event{
			Event:          model.EvFinish,
			CleanerID:      c.ID,
			Status:         status,
			FilesDeleted:   files,
			BytesFreed:     bytes,
			FilesCandidate: files,
			BytesCandidate: bytes,
			Errors:         errs,
			DurationMs:     e.now().Sub(start).Milliseconds(),
			TS:             e.now(),
		})
	}
	skip := func(reason, detail string) error {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: reason, Detail: detail, TS: e.now()})
		finish("skipped", 0, 0, 0)
		return nil
	}
	fail := func(reason, detail string) error {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: reason, Detail: detail, TS: e.now()})
		finish("error", 0, 0, 1)
		return nil
	}

	if c.Profile() != "lang-rustup" {
		return fail("internal", "unknown rustup profile")
	}
	if c.Scope != model.ScopeUser {
		return skip("profile_scope_mismatch", "rustup profiles are user-scoped")
	}
	if c.NoAgeFilter {
		return skip("no_age_filter_unsupported", "rustup requires an age floor")
	}
	root := e.resolveToolchainsRoot()
	if root == "" || !managedPathWithin(root, filepath.Clean(e.Home)) {
		return skip("root_unavailable", "rustup toolchains root is outside target home")
	}
	if safety.Blacklisted(root, e.Home) {
		return skip("root_unavailable", "rustup root is blacklisted")
	}
	parentFD, rootFD, rootIdentity, defaultToolchain, err := e.openRustupRoot(root)
	if err != nil {
		reason := "root_unavailable"
		switch {
		case errors.Is(err, safety.ErrSymlink):
			reason = string(model.SkipReasonSymlink)
		case errors.Is(err, safety.ErrCrossFS):
			reason = string(model.SkipReasonCrossFS)
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
	if busy, checkErr := e.processCheck(rustupOwnerProcesses, e.procRoot); checkErr != nil {
		return skip("proc_inspection_failed", checkErr.Error())
	} else if busy {
		return skip(string(model.SkipReasonRunningProcess), "Rust toolchain owner process is running")
	}

	minAge := managedCacheMinAgeDays
	if requested := c.AgeOrDefault(managedCacheMinAgeDays); requested > minAge {
		minAge = requested
	}

	names, err := readManagedDirNames(rootFD)
	if err != nil {
		return fail("enumeration_failed", err.Error())
	}
	sort.Strings(names)

	units := make([]rustupUnit, 0, len(names))
	for _, leaf := range names {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !safeManagedLeaf(leaf) {
			continue
		}
		identity, statErr := managedStatAt(rootFD, leaf)
		if statErr != nil {
			if errors.Is(statErr, unix.ENOENT) {
				continue
			}
			emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: filepath.Join(root, leaf), Reason: "stat_failed", Detail: statErr.Error(), TS: e.now()})
			finish("error", 0, 0, 1)
			return nil
		}
		if identity.Mode == unix.S_IFLNK {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: filepath.Join(root, leaf), Reason: string(model.SkipReasonSymlink), TS: e.now()})
			continue
		}
		if identity.Mode != unix.S_IFDIR {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: filepath.Join(root, leaf), Reason: string(model.SkipReasonWrongType), TS: e.now()})
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
		units = append(units, rustupUnit{leaf: leaf, family: rustupFamilyKey(leaf), identity: identity})
	}

	stale := selectStaleRustupToolchains(units, defaultToolchain)
	var filesDeleted, bytesFreed, filesCandidate, bytesCandidate int64
	errs := 0
	for _, unit := range stale {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		abs := filepath.Join(root, unit.leaf)
		if e.Whitelist != nil && e.Whitelist.Intersects(abs) {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonWhitelisted), TS: e.now()})
			continue
		}
		if !managedIdentityOldEnough(unit.identity, minAge, e.now()) {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonAgeTooRecent), TS: e.now()})
			continue
		}
		files, bytes, unitErr := e.removeRustupToolchain(rootFD, rootIdentity, unit)
		if unitErr != nil {
			errs++
			reason := "partial_delete"
			if errors.Is(unitErr, safety.ErrChanged) {
				reason = string(model.SkipReasonToctouChanged)
			}
			emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: reason, Detail: unitErr.Error(), TS: e.now()})
			continue
		}
		if dryRun {
			// removeRustupToolchain counted candidates without deleting;
			// nothing to restore because deletion was skipped internally.
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonDryRun), Size: bytes, TS: e.now()})
			filesCandidate += files
			bytesCandidate += bytes
			continue
		}
		filesDeleted += files
		bytesFreed += bytes
		emit(model.Event{Event: model.EvDeleted, CleanerID: c.ID, Path: abs, Size: bytes, TS: e.now()})
	}

	status := "ok"
	if errs > 0 {
		status = "error"
	}
	emit(model.Event{
		Event:          model.EvFinish,
		CleanerID:      c.ID,
		Status:         status,
		FilesDeleted:   filesDeleted,
		BytesFreed:     bytesFreed,
		FilesCandidate: filesCandidate,
		BytesCandidate: bytesCandidate,
		Errors:         errs,
		DurationMs:     e.now().Sub(start).Milliseconds(),
		TS:             e.now(),
	})
	return nil
}

type rustupUnit struct {
	leaf     string
	family   string
	identity managedIdentity
}

// resolveToolchainsRoot locates <RUSTUP_HOME|~/.rustup>/toolchains.
func (e *RustupExecutor) resolveToolchainsRoot() string {
	home := filepath.Clean(e.Home)
	if value := e.getenv("RUSTUP_HOME"); filepath.IsAbs(value) {
		return filepath.Join(filepath.Clean(value), "toolchains")
	}
	return filepath.Join(home, ".rustup", "toolchains")
}

func (e *RustupExecutor) getenv(name string) string {
	if e.env == nil {
		return ""
	}
	return e.env(name)
}

// openRustupRoot anchors the rustup home below the target home, reads the
// default toolchain from settings.toml, then opens toolchains/.
func (e *RustupExecutor) openRustupRoot(root string) (int, int, managedIdentity, string, error) {
	home := filepath.Clean(e.Home)
	root = filepath.Clean(root)
	rustupHome := filepath.Dir(root)
	if !filepath.IsAbs(home) || !managedPathWithin(rustupHome, home) {
		return -1, -1, managedIdentity{}, "", fmt.Errorf("rustup home outside target home")
	}
	homeFD, err := safety.OpenAnchoredDirFD(home, home)
	if err != nil {
		return -1, -1, managedIdentity{}, "", fmt.Errorf("home: %v", err)
	}
	defer unix.Close(homeFD)
	homeIdentity, err := e.fdIdentity(homeFD)
	if err != nil {
		return -1, -1, managedIdentity{}, "", fmt.Errorf("home identity: %v", err)
	}
	if homeIdentity.Mode != unix.S_IFDIR || homeIdentity.UID != e.TargetUID {
		return -1, -1, managedIdentity{}, "", fmt.Errorf("target home ownership/type invalid")
	}
	rustupFD, err := safety.OpenAnchoredDirFDFromFD(homeFD, home, rustupHome)
	if err != nil {
		return -1, -1, managedIdentity{}, "", fmt.Errorf("rustup home: %v", err)
	}
	defer unix.Close(rustupFD)
	defaultToolchain := e.readDefaultToolchain(rustupFD)
	// parentFD addresses the toolchains directory itself; toolchain leaves
	// are unlinked relative to it.
	parentFD, err := safety.OpenAnchoredDirFDFromFD(rustupFD, rustupHome, root)
	if err != nil {
		return -1, -1, managedIdentity{}, "", fmt.Errorf("toolchains parent: %v", err)
	}
	closeParent := true
	defer func() {
		if closeParent {
			_ = unix.Close(parentFD)
		}
	}()
	opened, err := e.fdIdentity(parentFD)
	if err != nil {
		return -1, -1, managedIdentity{}, "", fmt.Errorf("toolchains identity: %v", err)
	}
	if opened.Mode != unix.S_IFDIR || opened.UID != e.TargetUID {
		return -1, -1, managedIdentity{}, "", fmt.Errorf("toolchains root ownership/type invalid")
	}
	if opened.Dev != homeIdentity.Dev || opened.MountID != homeIdentity.MountID {
		return -1, -1, managedIdentity{}, "", safety.ErrCrossFS
	}
	rootFD, err := unix.Dup(parentFD)
	if err != nil {
		return -1, -1, managedIdentity{}, "", fmt.Errorf("dup toolchains: %v", err)
	}
	closeParent = false
	return parentFD, rootFD, opened, defaultToolchain, nil
}

// readDefaultToolchain extracts default_toolchain from settings.toml with a
// deliberately small line scan; a missing file means no default protection.
func (e *RustupExecutor) readDefaultToolchain(rustupFD int) string {
	fd, err := unix.Openat(rustupFD, "settings.toml", unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return ""
	}
	defer unix.Close(fd)
	data, err := readAllFD(fd)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok || strings.TrimSpace(key) != "default_toolchain" {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			return value[1 : len(value)-1]
		}
	}
	return ""
}

// removeRustupToolchain validates the unit identity again, then walks the
// toolchain tree. When dryRun is false files are unlinked; the leaf directory
// itself is removed only on the real path.
func (e *RustupExecutor) removeRustupToolchain(rootFD int, rootIdentity managedIdentity, unit rustupUnit) (int64, int64, error) {
	current, err := managedStatAt(rootFD, unit.leaf)
	if err != nil {
		return 0, 0, err
	}
	if !sameManagedState(current, unit.identity) || current.UID != e.TargetUID {
		return 0, 0, safety.ErrChanged
	}
	unitFD, err := openManagedUnit(rootFD, unit.leaf, managedUnitDirectory)
	if err != nil {
		return 0, 0, err
	}
	defer unix.Close(unitFD)
	opened, err := e.fdIdentity(unitFD)
	if err != nil {
		return 0, 0, err
	}
	if !sameManagedIdentity(opened, unit.identity) || opened.Dev != rootIdentity.Dev || opened.MountID != rootIdentity.MountID || opened.UID != e.TargetUID {
		return 0, 0, safety.ErrChanged
	}
	dup, err := unix.Dup(unitFD)
	if err != nil {
		return 0, 0, err
	}
	walker, err := safety.OpenWalkerFD(dup, unit.leaf)
	if err != nil {
		_ = unix.Close(dup)
		return 0, 0, err
	}
	defer walker.Close()
	var files, bytes int64
	var walkErr error
	walkFn := func(entry safety.Entry) {
		if walkErr != nil {
			return
		}
		if entry.IsDir() {
			files++ // directory entries count toward the removed unit
			if err := walker.RemoveEmptyDirIfMatch(entry); err != nil {
				walkErr = err
			}
			return
		}
		if !entry.IsRegular() {
			return
		}
		files++
		bytes += entry.Size()
		if err := walker.UnlinkIfUnchanged(entry); err != nil {
			walkErr = err
		}
	}
	if e.dryRun() {
		// Walk without unlinking: count sizes for candidate metrics.
		if err := walker.Walk(func(entry safety.Entry) {
			if entry.IsDir() {
				files++
				return
			}
			if entry.IsRegular() {
				files++
				bytes += entry.Size()
			}
		}); err != nil {
			return 0, 0, err
		}
		return files, bytes, nil
	}
	if err := walker.Walk(walkFn); err != nil {
		return 0, 0, err
	}
	if walkErr != nil {
		return 0, 0, walkErr
	}
	names, err := readManagedDirNames(unitFD)
	if err != nil {
		return 0, 0, err
	}
	if len(names) != 0 {
		return files, bytes, fmt.Errorf("toolchain directory not empty after cleanup")
	}
	if err := unix.Unlinkat(rootFD, unit.leaf, unix.AT_REMOVEDIR); err != nil {
		return files, bytes, err
	}
	return files, bytes, nil
}

// dryRun is bound per-run through the executor field below.
func (e *RustupExecutor) dryRun() bool { return e.dryRunFlag }

// rustupFamilyKey groups toolchain names so the newest member of each
// release line survives: stable/beta/nightly share one slot per channel and
// target triple; versioned channels keep one slot per major.minor and triple.
func rustupFamilyKey(leaf string) string {
	parts := strings.Split(leaf, "-")
	archIdx := -1
	for i, part := range parts {
		if _, ok := knownRustupArches[part]; ok {
			archIdx = i
			break
		}
	}
	triple := ""
	channel := leaf
	if archIdx > 0 {
		channel = strings.Join(parts[:archIdx], "-")
		triple = strings.Join(parts[archIdx:], "-")
	}
	slot := channel
	if channel == "stable" || channel == "beta" || channel == "nightly" {
		slot = channel
	} else if base, _, ok := strings.Cut(channel, "-"); ok && base == "nightly" {
		// nightly-YYYY-MM-DD shares the nightly slot.
		slot = "nightly"
	} else if semver := rustupSemverPrefix(channel); semver != "" {
		slot = semver
	}
	return slot + "|" + triple
}

// rustupSemverPrefix returns the major.minor prefix of a versioned channel
// like "1.85" or "1.85.1"; it returns "" for non-versioned names.
func rustupSemverPrefix(channel string) string {
	pieces := strings.Split(channel, ".")
	if len(pieces) < 2 {
		return ""
	}
	for _, piece := range pieces {
		if _, err := strconv.Atoi(piece); err != nil {
			return ""
		}
	}
	return strings.Join(pieces[:2], ".")
}

// rustupVersionKey orders members inside one family slot. Dated nightlies
// sort by date; versioned channels by numeric components; everything else is
// equal, and the lexical leaf name breaks ties.
func rustupVersionKey(leaf string) string {
	parts := strings.Split(leaf, "-")
	for i, part := range parts {
		if _, ok := knownRustupArches[part]; ok {
			parts = parts[:i]
			break
		}
	}
	channel := strings.Join(parts, "-")
	if base, date, ok := strings.Cut(channel, "-"); ok && base == "nightly" && rustupIsDate(date) {
		return "d:" + date
	}
	if semver := rustupSemverPrefix(channel); semver != "" {
		pieces := strings.Split(channel, ".")
		nums := make([]string, 0, len(pieces))
		for _, piece := range pieces {
			n, _ := strconv.Atoi(piece)
			nums = append(nums, fmt.Sprintf("%04d", n))
		}
		return "v:" + strings.Join(nums, ".")
	}
	return "x:"
}

func rustupIsDate(s string) bool {
	pieces := strings.Split(s, "-")
	if len(pieces) != 3 {
		return false
	}
	for _, piece := range pieces {
		if _, err := strconv.Atoi(piece); err != nil {
			return false
		}
	}
	return true
}

// selectStaleRustupToolchains keeps the default toolchain and the newest
// member of each family slot; everything else is a removal candidate.
func selectStaleRustupToolchains(units []rustupUnit, defaultToolchain string) []rustupUnit {
	kept := map[string]rustupUnit{}
	for _, unit := range units {
		if unit.leaf == defaultToolchain {
			continue
		}
		key := unit.family
		current, ok := kept[key]
		if !ok || rustupVersionKey(unit.leaf) > rustupVersionKey(current.leaf) ||
			(rustupVersionKey(unit.leaf) == rustupVersionKey(current.leaf) && unit.leaf > current.leaf) {
			kept[key] = unit
		}
	}
	keepLeaves := map[string]bool{defaultToolchain: defaultToolchain != ""}
	for _, unit := range kept {
		keepLeaves[unit.leaf] = true
	}
	stale := make([]rustupUnit, 0, len(units))
	for _, unit := range units {
		if !keepLeaves[unit.leaf] {
			stale = append(stale, unit)
		}
	}
	return stale
}

// readAllFD reads a small regular file (settings.toml) fully.
func readAllFD(fd int) ([]byte, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return nil, err
	}
	if st.Size <= 0 || st.Size > 1<<20 {
		return nil, nil
	}
	buf := make([]byte, st.Size)
	var total int
	for total < len(buf) {
		n, err := unix.Read(fd, buf[total:])
		if err != nil {
			return nil, err
		}
		if n == 0 {
			break
		}
		total += n
	}
	return buf[:total], nil
}
