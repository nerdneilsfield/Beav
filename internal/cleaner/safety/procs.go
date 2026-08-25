package safety

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
)

// VerifyProcVisibility rejects proc mounts whose hidepid setting prevents a
// complete file-descriptor view. A synthetic proc root without a mounts file
// is an explicit test fixture seam; the real /proc must provide mount data.
func VerifyProcVisibility(procRoot string, _ uint32) error {
	data, err := os.ReadFile(filepath.Join(procRoot, "mounts"))
	if err != nil {
		if procRoot != "/proc" && errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	found := false
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || fields[1] != "/proc" || fields[2] != "proc" {
			continue
		}
		found = true
		options := fields[3]
		for i, field := range fields {
			if field == "-" && i+3 < len(fields) {
				options += "," + fields[i+3]
				break
			}
		}
		for _, option := range strings.Split(options, ",") {
			if !strings.HasPrefix(option, "hidepid") {
				continue
			}
			parts := strings.SplitN(option, "=", 2)
			if len(parts) != 2 {
				return fmt.Errorf("proc mount has malformed %s", option)
			}
			value, parseErr := strconv.Atoi(parts[1])
			if parseErr != nil || value != 0 {
				return fmt.Errorf("proc mount hides process descriptors: %s", option)
			}
		}
	}
	if !found && procRoot == "/proc" {
		return errors.New("proc mount visibility could not be proven")
	}
	return nil
}

// FileIdentity identifies a filesystem object independent of pathname.
type FileIdentity struct {
	Dev uint64
	Ino uint64
}

// OpenDescriptorMatches scans procRoot's file descriptors for a descriptor
// referring to the supplied device and inode. The root argument is retained
// for callers that resolve a candidate relative to a trusted root; identity,
// rather than a pathname, is deliberately used for the comparison.
func OpenDescriptorMatches(_ string, dev, ino uint64, procRoot string) (bool, error) {
	return OpenDescriptorsMatch(procRoot, []FileIdentity{{Dev: dev, Ino: ino}})
}

// OpenDescriptorsMatch scans every visible process descriptor and compares
// fstat device/inode identities against candidates. It never uses descriptor
// symlink pathnames to decide whether an object is open.
func OpenDescriptorsMatch(procRoot string, identities []FileIdentity) (bool, error) {
	return openDescriptorsMatchForUID(procRoot, identities, -1, -1, effectiveUID())
}

// OpenDescriptorMatchesExceptPID scans visible process descriptors while
// excluding one known process ID. Cleanup code holds an object descriptor
// across rename; excluding its own PID keeps that deliberate hold distinct
// from an owner descriptor discovered through proc.
func OpenDescriptorMatchesExceptPID(procRoot string, identities []FileIdentity, excludePID int) (bool, error) {
	return openDescriptorsMatchForUID(procRoot, identities, excludePID, -1, effectiveUID())
}

// OpenDescriptorMatchesExceptFD scans visible descriptors while excluding one
// deliberate descriptor held by the caller. Other descriptors from same PID
// remain visible, so an owner descriptor cannot hide behind process identity.
func OpenDescriptorMatchesExceptFD(procRoot string, identities []FileIdentity, excludePID, excludeFD int) (bool, error) {
	return openDescriptorsMatchForUID(procRoot, identities, excludePID, excludeFD, effectiveUID())
}

func effectiveUID() uint32 {
	return uint32(os.Geteuid()) //nolint:gosec // Linux UID values originate from uint32 kernel IDs.
}

func openDescriptorsMatchForUID(procRoot string, identities []FileIdentity, excludePID, excludeFD int, effectiveUID uint32) (bool, error) {
	if err := VerifyProcVisibility(procRoot, effectiveUID); err != nil {
		return false, err
	}
	want := make(map[FileIdentity]struct{}, len(identities))
	for _, identity := range identities {
		want[identity] = struct{}{}
	}
	processes, err := os.ReadDir(procRoot)
	if err != nil {
		return false, err
	}
	for _, process := range processes {
		if !process.IsDir() {
			continue
		}
		pid, err := strconv.ParseUint(process.Name(), 10, 32)
		if err != nil {
			continue
		}
		if excludePID >= 0 && int(pid) == excludePID && excludeFD < 0 {
			continue
		}
		processRoot := filepath.Join(procRoot, process.Name())
		inspect, err := ShouldInspectProcessFDs(processRoot, effectiveUID)
		if err != nil {
			return false, err
		}
		if !inspect {
			continue
		}
		fdRoot := filepath.Join(processRoot, "fd")
		fds, err := os.ReadDir(fdRoot)
		if err != nil {
			return false, err
		}
		for _, fd := range fds {
			fdNumber, parseErr := strconv.Atoi(fd.Name())
			if parseErr == nil && int(pid) == excludePID && fdNumber == excludeFD {
				continue
			}
			identity, err := statProcFD(filepath.Join(fdRoot, fd.Name()))
			if err != nil {
				return false, err
			}
			if _, ok := want[identity]; ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// ShouldInspectProcessFDs reports whether a proc process directory belongs to
// the effective user whose descriptors can be inspected. Root inspects all.
func ShouldInspectProcessFDs(processRoot string, effectiveUID uint32) (bool, error) {
	if effectiveUID == 0 {
		return true, nil
	}
	var st unix.Stat_t
	if err := unix.Stat(processRoot, &st); err != nil {
		if errors.Is(err, unix.ENOENT) {
			return false, nil
		}
		return false, err
	}
	return st.Uid == effectiveUID, nil
}

// statProcFD opens a visible proc descriptor link and fstats the resulting
// object. O_PATH permits identity inspection without requiring read/write
// access to the target file and avoids trusting the link's pathname text.
func statProcFD(path string) (FileIdentity, error) {
	fd, err := unix.Open(path, unix.O_PATH|unix.O_CLOEXEC, 0)
	if err != nil {
		return FileIdentity{}, err
	}
	defer unix.Close(fd)
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return FileIdentity{}, err
	}
	return FileIdentity{Dev: uint64(st.Dev), Ino: uint64(st.Ino)}, nil
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
		} else {
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
		} else {
			return false, err
		}
	}
	return false, nil
}
