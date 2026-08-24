package executor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func huggingFaceRootBusy(root string) (bool, error) {
	return inspectHuggingFaceBusy(root, "/proc")
}

func inspectHuggingFaceBusy(root, procRoot string) (bool, error) {
	root = filepath.Clean(root)
	if info, err := os.Stat(root); err != nil {
		return false, err
	} else if !info.IsDir() {
		return false, errors.New("cache root is not a directory")
	}
	busy := false
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry == nil {
			return errors.New("cache walk returned nil entry")
		}
		if entry.IsDir() && (entry.Name() == ".locks" || entry.Name() == "locks") {
			children, readErr := os.ReadDir(path)
			if readErr != nil {
				return readErr
			}
			if len(children) > 0 {
				busy = true
			}
			return filepath.SkipDir
		}
		name := strings.ToLower(entry.Name())
		if strings.HasSuffix(name, ".lock") || name == "lock" {
			busy = true
			return filepath.SkipAll
		}
		return nil
	}); err != nil {
		return false, fmt.Errorf("walk cache root: %w", err)
	}
	if busy {
		return true, nil
	}
	return inspectOpenDescriptorUnder(root, procRoot)
}

func inspectOpenDescriptorUnder(root, procRoot string) (bool, error) {
	root = filepath.Clean(root)
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return false, err
	}
	for _, process := range entries {
		if !process.IsDir() || process.Name() == "" || process.Name()[0] < '0' || process.Name()[0] > '9' {
			continue
		}
		fdRoot := filepath.Join(procRoot, process.Name(), "fd")
		fds, err := os.ReadDir(fdRoot)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return false, fmt.Errorf("read %s: %w", fdRoot, err)
		}
		for _, fd := range fds {
			fdPath := filepath.Join(fdRoot, fd.Name())
			link, err := os.Readlink(fdPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return false, fmt.Errorf("readlink %s: %w", fdPath, err)
			}
			rawLink := filepath.Clean(link)
			link = strings.TrimSuffix(rawLink, " (deleted)")
			if !pathWithin(link, root) {
				continue
			}
			if strings.HasSuffix(rawLink, " (deleted)") {
				return true, nil
			}
			if _, err := os.Stat(fdPath); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return true, nil
				}
				return false, fmt.Errorf("stat %s: %w", fdPath, err)
			}
			return true, nil
		}
	}
	return false, nil
}

// openDescriptorUnder preserves the historical boolean helper for callers in
// this package; any inspection error is conservatively treated as busy.
func openDescriptorUnder(root string) bool {
	busy, err := inspectOpenDescriptorUnder(root, "/proc")
	return busy || err != nil
}
