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

type managedTreeState map[string]managedIdentity

func captureManagedTree(fd int, abs string, kind managedUnitKind, root managedIdentity) (managedTreeState, error) {
	tree := managedTreeState{"": root}
	if kind != managedUnitDirectory {
		return tree, nil
	}
	dup, err := unix.Dup(fd)
	if err != nil {
		return nil, err
	}
	if _, err := unix.Seek(dup, 0, io.SeekStart); err != nil {
		_ = unix.Close(dup)
		return nil, err
	}
	walker, err := safety.OpenWalkerFD(dup, abs)
	if err != nil {
		return nil, err
	}
	defer walker.Close()
	if err := walker.Walk(func(entry safety.Entry) {
		mode := uint32(unix.S_IFREG)
		if entry.IsDir() {
			mode = unix.S_IFDIR
		}
		mtime := entry.MtimeTime()
		ctime := entry.CtimeTime()
		tree[entry.RelPath] = managedIdentity{
			Dev:       entry.Dev(),
			Ino:       entry.Ino(),
			MountID:   entry.MountID(),
			UID:       entry.UID(),
			Mode:      mode,
			Nlink:     entry.Nlink(),
			Size:      entry.Size(),
			MtimeSec:  mtime.Unix(),
			MtimeNsec: int64(mtime.Nanosecond()),
			CtimeSec:  ctime.Unix(),
			CtimeNsec: int64(ctime.Nanosecond()),
		}
	}); err != nil {
		return nil, err
	}
	return tree, nil
}

func sameManagedTree(a, b managedTreeState) bool {
	if len(a) != len(b) {
		return false
	}
	for path, left := range a {
		right, ok := b[path]
		if !ok || !sameManagedState(left, right) {
			return false
		}
	}
	return true
}

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
	st, err := managedStatx(fd, "", unix.AT_EMPTY_PATH)
	if err != nil {
		return managedIdentity{}, err
	}
	return managedIdentityFromStatx(st), nil
}

func managedStatAt(parentFD int, leaf string) (managedIdentity, error) {
	st, err := managedStatx(parentFD, leaf, unix.AT_SYMLINK_NOFOLLOW)
	if err != nil {
		return managedIdentity{}, err
	}
	return managedIdentityFromStatx(st), nil
}

func managedStatx(fd int, path string, flags int) (unix.Statx_t, error) {
	var st unix.Statx_t
	if err := unix.Statx(fd, path, flags, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &st); err != nil {
		return unix.Statx_t{}, err
	}
	if st.Mask&unix.STATX_MNT_ID == 0 || st.Mnt_id == 0 {
		return unix.Statx_t{}, errManagedMountUnavailable
	}
	return st, nil
}

func managedIdentityFromStatx(st unix.Statx_t) managedIdentity {
	return managedIdentity{
		Dev:       unix.Mkdev(st.Dev_major, st.Dev_minor),
		Ino:       st.Ino,
		MountID:   st.Mnt_id,
		UID:       st.Uid,
		Mode:      uint32(st.Mode & unix.S_IFMT),
		Nlink:     uint64(st.Nlink),
		Size:      int64(st.Size), //nolint:gosec // Linux statx size is bounded by signed off_t.
		MtimeSec:  st.Mtime.Sec,
		MtimeNsec: int64(st.Mtime.Nsec),
		CtimeSec:  st.Ctime.Sec,
		CtimeNsec: int64(st.Ctime.Nsec),
	}
}

func sameManagedIdentity(a, b managedIdentity) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.MountID == b.MountID && a.UID == b.UID && a.Mode == b.Mode
}

func sameManagedState(a, b managedIdentity) bool {
	return sameManagedIdentity(a, b) && a.Nlink == b.Nlink && a.Size == b.Size && a.MtimeSec == b.MtimeSec && a.MtimeNsec == b.MtimeNsec && a.CtimeSec == b.CtimeSec && a.CtimeNsec == b.CtimeNsec
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
	if !safeManagedLeaf(leaf) || !strings.HasPrefix(leaf, managedTombstonePrefix) {
		return false
	}
	suffix := strings.TrimPrefix(leaf, managedTombstonePrefix)
	if len(suffix) != hex.EncodedLen(16) {
		return false
	}
	for _, ch := range suffix {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
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
	switch {
	case errors.Is(err, safety.ErrSymlink):
		reason = string(model.SkipReasonSymlink)
	case errors.Is(err, safety.ErrCrossFS):
		reason = string(model.SkipReasonCrossFS)
	case errors.Is(err, unix.ENOTDIR):
		reason = string(model.SkipReasonWrongType)
	}
	emit(model.Event{Event: model.EvSkipped, CleanerID: cleanerID, Path: path, Reason: reason, Detail: err.Error(), TS: now()})
}
