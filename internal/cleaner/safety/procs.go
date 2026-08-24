package safety

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// OpenDescriptorMatches scans procRoot's file descriptors for a descriptor
// referring to the supplied device and inode. The root argument is retained
// for callers that resolve a candidate relative to a trusted root; identity,
// rather than a pathname, is deliberately used for the comparison.
func OpenDescriptorMatches(_ string, dev, ino uint64, procRoot string) (bool, error) {
	processes, err := os.ReadDir(procRoot)
	if err != nil {
		return false, err
	}
	for _, process := range processes {
		if !process.IsDir() {
			continue
		}
		if _, err := strconv.ParseUint(process.Name(), 10, 32); err != nil {
			continue
		}
		fdRoot := filepath.Join(procRoot, process.Name(), "fd")
		fds, err := os.ReadDir(fdRoot)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return false, err
		}
		for _, fd := range fds {
			var st unix.Stat_t
			if err := unix.Stat(filepath.Join(fdRoot, fd.Name()), &st); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
				}
				return false, err
			}
			if uint64(st.Dev) == dev && uint64(st.Ino) == ino {
				return true, nil
			}
		}
	}
	return false, nil
}

// AnyProcessRunning checks if any process with the given names is currently running.
// AnyProcessRunning 检查具有给定名称的任何进程是否正在运行。
func AnyProcessRunning(names []string) bool {
	running, _ := AnyProcessRunningChecked(names, "/proc")
	return running
}

// AnyProcessRunningChecked performs the same process-name check against a
// caller-supplied proc tree and surfaces read errors. Deletion callers should
// treat an error as activity because proc races and permission failures are
// not proof that no owner is running.
func AnyProcessRunningChecked(names []string, procRoot string) (bool, error) {
	if len(names) == 0 {
		return false, nil
	}
	want := map[string]bool{}
	for _, n := range names {
		if n != "" {
			want[n] = true
		}
	}
	if len(want) == 0 {
		return false, nil
	}
	dirs, err := os.ReadDir(procRoot)
	if err != nil {
		return false, err
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		pid := d.Name()
		if pid == "" || pid[0] < '0' || pid[0] > '9' {
			continue
		}
		if comm, err := os.ReadFile(filepath.Join(procRoot, pid, "comm")); err == nil {
			if want[strings.TrimSpace(string(comm))] {
				return true, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		if cmd, err := os.ReadFile(filepath.Join(procRoot, pid, "cmdline")); err == nil {
			parts := strings.Split(string(cmd), "\x00")
			if len(parts) > 0 && want[filepath.Base(parts[0])] {
				return true, nil
			}
			joined := strings.Join(parts, " ")
			for n := range want {
				if strings.Contains(n, "-") && strings.Contains(joined, n) {
					return true, nil
				}
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}
