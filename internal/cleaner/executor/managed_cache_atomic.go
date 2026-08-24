package executor

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

func managedIdentityOldEnough(identity managedIdentity, minAge int, now time.Time) bool {
	if minAge < managedCacheMinAgeDays || now.IsZero() {
		return false
	}
	stamp := time.Unix(identity.MtimeSec, identity.MtimeNsec)
	return !stamp.After(now.Add(-time.Duration(minAge) * 24 * time.Hour))
}

func openManagedUnit(parentFD int, leaf string, kind managedUnitKind) (int, error) {
	if !safeManagedLeaf(leaf) {
		return -1, safety.ErrNotFound
	}
	flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_CLOEXEC
	if kind == managedUnitDirectory {
		flags |= unix.O_DIRECTORY
	}
	return unix.Openat(parentFD, leaf, flags, 0)
}

func deleteManagedDirectory(fd int, abs string) error {
	dup, err := unix.Dup(fd)
	if err != nil {
		return err
	}
	if _, err := unix.Seek(dup, 0, io.SeekStart); err != nil {
		_ = unix.Close(dup)
		return err
	}
	walker, err := safety.OpenWalkerFD(dup, abs)
	if err != nil {
		return err
	}
	defer walker.Close()
	var firstErr error
	if err := walker.Walk(func(entry safety.Entry) {
		if firstErr != nil {
			return
		}
		if entry.IsDir() {
			if err := walker.RemoveEmptyDirIfMatch(entry); err != nil {
				firstErr = err
			}
			return
		}
		if entry.IsRegular() {
			if err := walker.UnlinkIfUnchanged(entry); err != nil {
				firstErr = err
			}
		}
	}); err != nil {
		return err
	}
	return firstErr
}

func removeManagedTombstone(parentFD, unitFD int, leaf string, expected managedIdentity, kind managedUnitKind) error {
	current, err := managedStatAt(parentFD, leaf)
	if err != nil {
		return err
	}
	held, err := readManagedFDIdentity(unitFD)
	if err != nil {
		return err
	}
	if !sameManagedIdentity(current, expected) || !sameManagedIdentity(held, expected) {
		return errManagedRootChanged
	}
	if kind == managedUnitDirectory {
		names, err := readManagedDirNames(unitFD)
		if err != nil {
			return err
		}
		if len(names) != 0 {
			return fmt.Errorf("directory not empty: %s", strings.Join(names, ","))
		}
		if err := unix.Unlinkat(parentFD, leaf, unix.AT_REMOVEDIR); err != nil {
			return err
		}
	} else if err := unix.Unlinkat(parentFD, leaf, 0); err != nil {
		return err
	}
	return unix.Fsync(parentFD)
}

func readManagedDirNames(fd int) ([]string, error) {
	dup, err := unix.Dup(fd)
	if err != nil {
		return nil, err
	}
	// dup shares directory stream offset with held fd; rewind duplicate so
	// post-walk revalidation observes complete contents.
	if _, err := unix.Seek(dup, 0, io.SeekStart); err != nil {
		_ = unix.Close(dup)
		return nil, err
	}
	file := os.NewFile(uintptr(dup), "<managed-cache-dir>")
	if file == nil {
		_ = unix.Close(dup)
		return nil, errors.New("failed to wrap directory fd")
	}
	defer file.Close()
	return file.Readdirnames(-1)
}

func readManagedFDIdentity(fd int) (managedIdentity, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return managedIdentity{}, err
	}
	return managedIdentity{
		Dev:       uint64(st.Dev),
		Ino:       uint64(st.Ino),
		UID:       st.Uid,
		Mode:      uint32(st.Mode & unix.S_IFMT),
		Size:      st.Size,
		MtimeSec:  st.Mtim.Sec,
		MtimeNsec: st.Mtim.Nsec,
	}, nil
}

func managedStatAt(parentFD int, leaf string) (managedIdentity, error) {
	var st unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return managedIdentity{}, err
	}
	return managedIdentity{
		Dev:       uint64(st.Dev),
		Ino:       uint64(st.Ino),
		UID:       st.Uid,
		Mode:      uint32(st.Mode & unix.S_IFMT),
		Size:      st.Size,
		MtimeSec:  st.Mtim.Sec,
		MtimeNsec: st.Mtim.Nsec,
	}, nil
}

func sameManagedIdentity(a, b managedIdentity) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.UID == b.UID && a.Mode == b.Mode
}

func sameManagedState(a, b managedIdentity) bool {
	return sameManagedIdentity(a, b) && a.Size == b.Size && a.MtimeSec == b.MtimeSec && a.MtimeNsec == b.MtimeNsec
}

func managedKindMatches(mode uint32, kind managedUnitKind) bool {
	if kind == managedUnitDirectory {
		return mode == unix.S_IFDIR
	}
	return mode == unix.S_IFREG
}

func managedPathWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	return path != root && strings.HasPrefix(path, root+string(filepath.Separator))
}

func safeManagedLeaf(leaf string) bool {
	return leaf != "" && leaf != "." && leaf != ".." && filepath.Base(leaf) == leaf && !strings.ContainsAny(leaf, `/\\`)
}

func isManagedTombstone(leaf string) bool {
	return strings.HasPrefix(leaf, managedTombstonePrefix) || strings.HasPrefix(leaf, ".beav-tombstone-")
}

func validManagedTombstone(leaf string) bool {
	// Recovery may act only on names this executor can generate. Broader
	// tombstone-looking names are skipped during enumeration but never become
	// trusted through a forged journal record.
	return safeManagedLeaf(leaf) && strings.HasPrefix(leaf, managedTombstonePrefix)
}

func randomManagedTombstone() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return managedTombstonePrefix + hex.EncodeToString(raw[:]), nil
}

func checkedManagedProcessRunning(names []string, procRoot string) (bool, error) {
	return safety.AnyProcessRunningChecked(names, procRoot)
}

func emitManagedOpenSkip(cleanerID, path string, err error, emit func(model.Event), now func() time.Time) {
	reason := "permission_denied"
	if errors.Is(err, safety.ErrSymlink) {
		reason = string(model.SkipReasonSymlink)
	} else if errors.Is(err, safety.ErrCrossFS) {
		reason = string(model.SkipReasonCrossFS)
	} else if errors.Is(err, unix.ENOTDIR) {
		reason = string(model.SkipReasonWrongType)
	}
	emit(model.Event{Event: model.EvSkipped, CleanerID: cleanerID, Path: path, Reason: reason, Detail: err.Error(), TS: now()})
}
