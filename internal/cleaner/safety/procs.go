package safety

import (
	"os"
	"path/filepath"
	"strings"
)

// AnyProcessRunning checks if any process with the given names is currently running.
// AnyProcessRunning 检查具有给定名称的任何进程是否正在运行。
func AnyProcessRunning(names []string) bool {
	if len(names) == 0 {
		return false
	}
	want := map[string]bool{}
	for _, n := range names {
		if n != "" {
			want[n] = true
		}
	}
	if len(want) == 0 {
		return false
	}
	dirs, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		pid := d.Name()
		if pid == "" || pid[0] < '0' || pid[0] > '9' {
			continue
		}
		if comm, err := os.ReadFile(filepath.Join("/proc", pid, "comm")); err == nil {
			if want[strings.TrimSpace(string(comm))] {
				return true
			}
		}
		if cmd, err := os.ReadFile(filepath.Join("/proc", pid, "cmdline")); err == nil {
			parts := strings.Split(string(cmd), "\x00")
			if len(parts) > 0 && want[filepath.Base(parts[0])] {
				return true
			}
			joined := strings.Join(parts, " ")
			for n := range want {
				if strings.Contains(n, "-") && strings.Contains(joined, n) {
					return true
				}
			}
		}
	}
	return false
}
