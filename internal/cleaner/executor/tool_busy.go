package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

func inspectHuggingFaceBusy(root, procRoot string) (bool, error) {
	w, err := safety.OpenWalker(root)
	if err != nil {
		return false, err
	}
	fd, err := w.DupRootFD()
	_ = w.Close()
	if err != nil {
		return false, err
	}
	defer unix.Close(fd)
	return inspectHuggingFaceBusyFD(fd, filepath.Clean(root), procRoot)
}

// inspectHuggingFaceBusyFD enumerates cache through held root fd and compares
// process descriptors by device/inode identity.
func inspectHuggingFaceBusyFD(rootFD int, root, procRoot string) (bool, error) {
	var rootStat unix.Stat_t
	if err := unix.Fstat(rootFD, &rootStat); err != nil {
		return false, err
	}
	if rootStat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return false, fmt.Errorf("cache root fd is not a directory")
	}
	identities := []safety.FileIdentity{{Dev: uint64(rootStat.Dev), Ino: uint64(rootStat.Ino)}}
	lockFD, err := unix.Openat(rootFD, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, err
	}
	unix.CloseOnExec(lockFD)
	lockBusy, lockErr := scanHuggingFaceLockMarkers(lockFD, "")
	_ = unix.Close(lockFD)
	if lockErr != nil {
		return false, fmt.Errorf("scan cache locks: %w", lockErr)
	}
	if lockBusy {
		return true, nil
	}

	// Open a fresh file description beneath the held root. A plain dup shares
	// directory offsets with rootFD, which could make the later cleanup walk
	// miss entries after this guard scan.
	scanFD, err := unix.Openat(rootFD, ".", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return false, err
	}
	unix.CloseOnExec(scanFD)
	w, err := safety.OpenWalkerFD(scanFD, root)
	if err != nil {
		return false, err
	}
	walkErr := w.Walk(func(entry safety.Entry) {
		identities = append(identities, safety.FileIdentity{Dev: entry.Dev(), Ino: entry.Ino()})
	})
	closeErr := w.Close()
	if walkErr != nil {
		return false, fmt.Errorf("walk cache root: %w", walkErr)
	}
	if closeErr != nil {
		return false, fmt.Errorf("close cache walk: %w", closeErr)
	}
	deletedBusy, err := inspectDeletedDescriptorUnder(root, procRoot, uint32(os.Geteuid()))
	if err != nil {
		return false, err
	}
	if deletedBusy {
		return true, nil
	}
	busy, err := safety.OpenDescriptorMatchesExceptFD(procRoot, identities, os.Getpid(), rootFD)
	if err != nil {
		return false, err
	}
	if busy {
		return true, nil
	}
	return false, nil
}

// scanHuggingFaceLockMarkers enumerates every directory entry through a held
// descriptor. fstatat with AT_SYMLINK_NOFOLLOW is deliberate: lock marker
// symlinks are busy and must never be followed into another tree.
func scanHuggingFaceLockMarkers(dirFD int, rel string) (bool, error) {
	dupFD, err := unix.Dup(dirFD)
	if err != nil {
		return false, err
	}
	dir := os.NewFile(uintptr(dupFD), "<hf-locks>")
	names, readErr := dir.Readdirnames(-1)
	closeErr := dir.Close()
	if readErr != nil {
		return false, readErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	for _, name := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(dirFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return false, err
		}
		entryRel := filepath.Join(rel, name)
		if st.Mode&unix.S_IFMT != unix.S_IFDIR && huggingFaceLockMarker(entryRel) {
			return true, nil
		}
		if st.Mode&unix.S_IFMT != unix.S_IFDIR {
			continue
		}
		childFD, err := unix.Openat(dirFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			return false, err
		}
		unix.CloseOnExec(childFD)
		busy, walkErr := scanHuggingFaceLockMarkers(childFD, entryRel)
		_ = unix.Close(childFD)
		if walkErr != nil || busy {
			return busy, walkErr
		}
	}
	return false, nil
}

func huggingFaceLockMarker(rel string) bool {
	parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
	for i, part := range parts {
		name := strings.ToLower(part)
		if i == len(parts)-1 && (name == ".locks" || name == "locks" || strings.HasSuffix(name, ".lock") || name == "lock") {
			return true
		}
	}
	return false
}

func inspectDeletedDescriptorUnder(root, procRoot string, effectiveUID uint32) (bool, error) {
	root = filepath.Clean(root)
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return false, err
	}
	for _, process := range entries {
		if !process.IsDir() || process.Name() == "" || process.Name()[0] < '0' || process.Name()[0] > '9' {
			continue
		}
		processRoot := filepath.Join(procRoot, process.Name())
		inspect, err := safety.ShouldInspectProcessFDs(processRoot, effectiveUID)
		if err != nil {
			return false, err
		}
		if !inspect {
			continue
		}
		fdRoot := filepath.Join(processRoot, "fd")
		fds, err := os.ReadDir(fdRoot)
		if err != nil {
			return false, fmt.Errorf("read %s: %w", fdRoot, err)
		}
		for _, fd := range fds {
			fdPath := filepath.Join(fdRoot, fd.Name())
			link, err := os.Readlink(fdPath)
			if err != nil {
				return false, fmt.Errorf("readlink %s: %w", fdPath, err)
			}
			rawLink := filepath.Clean(link)
			cleanLink := strings.TrimSuffix(rawLink, " (deleted)")
			if strings.HasSuffix(rawLink, " (deleted)") && pathWithin(cleanLink, root) {
				return true, nil
			}
		}
	}
	return false, nil
}
