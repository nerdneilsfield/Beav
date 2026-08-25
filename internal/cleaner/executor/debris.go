package executor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

const (
	debrisDefaultAgeDays = 1
	debrisGPUAgeDays     = 7
	debrisCloudAgeDays   = 30
)

type debrisProfile struct {
	minAgeDays  int
	depth       int
	resolveRoot func(*DebrisExecutor) (string, error)
	match       func(string) bool
	processes   []string
	locks       bool
}

type debrisIdentity struct {
	Dev   uint64
	Ino   uint64
	UID   uint32
	MntID uint64
	Mode  uint32
}

// DebrisExecutor removes only artifacts named by the closed debris profile
// registry. It never accepts roots or matching rules from YAML.
type DebrisExecutor struct {
	Home      string
	Whitelist *safety.Whitelist
	TargetUID uint32

	procRoot       string
	env            func(string) string
	userDirsPath   string
	processCheck   func([]string, string) (bool, error)
	procVisibility func(string, uint32) error
	beforeRootOpen func(string) error
	pathIdentity   func(string) (debrisIdentity, error)
	fdIdentity     func(int) (debrisIdentity, error)
	now            func() time.Time
}

// NewDebrisExecutor creates a profile-backed debris executor. The optional
// UID exists for callers that already resolved a target account.
func NewDebrisExecutor(home string, wl *safety.Whitelist, targetUID ...uint32) *DebrisExecutor {
	uid := uint32(os.Getuid())
	if len(targetUID) > 0 {
		uid = targetUID[0]
	}
	return &DebrisExecutor{
		Home:           filepath.Clean(home),
		Whitelist:      wl,
		TargetUID:      uid,
		procRoot:       "/proc",
		env:            os.Getenv,
		processCheck:   checkedProcessRunning,
		procVisibility: safety.VerifyProcVisibility,
		pathIdentity:   readDebrisPathIdentity,
		fdIdentity:     readDebrisFDIdentity,
		now:            time.Now,
	}
}

var debrisProfileRegistry = map[string]debrisProfile{
	"desktop-download-partials": {
		minAgeDays:  debrisDefaultAgeDays,
		depth:       1,
		resolveRoot: (*DebrisExecutor).resolveDownloadsRoot,
		match:       downloadPartialName,
	},
	"ai-huggingface-incomplete": {
		minAgeDays:  debrisDefaultAgeDays,
		resolveRoot: (*DebrisExecutor).resolveHuggingFaceRoot,
		match:       func(name string) bool { return strings.HasSuffix(name, ".incomplete") },
		locks:       true,
	},
	"python-ruff-cache": {
		resolveRoot: func(e *DebrisExecutor) (string, error) { return e.resolveExplicitEnvRoot("RUFF_CACHE_DIR") },
		match:       anyDebrisName,
	},
	"python-mypy-cache": {
		resolveRoot: func(e *DebrisExecutor) (string, error) { return e.resolveExplicitEnvRoot("MYPY_CACHE_DIR") },
		match:       anyDebrisName,
	},
	"python-pyinstaller-cache": {
		resolveRoot: (*DebrisExecutor).resolvePyInstallerRoot,
		match:       anyDebrisName,
	},
	"gpu-nvidia-compute-cache": {
		minAgeDays:  debrisGPUAgeDays,
		resolveRoot: (*DebrisExecutor).resolveNVIDIARoot,
		match:       anyDebrisName,
		processes:   []string{"nvidia-persistenced", "nvidia-cuda-mps-server"},
	},
	"gpu-triton-cache": {
		minAgeDays:  debrisGPUAgeDays,
		resolveRoot: (*DebrisExecutor).resolveTritonRoot,
		match:       anyDebrisName,
		processes:   []string{"triton", "tritonserver"},
	},
	"gpu-torch-inductor-cache": {
		minAgeDays:  debrisGPUAgeDays,
		resolveRoot: (*DebrisExecutor).resolveTorchInductorRoot,
		match:       anyDebrisName,
		processes:   []string{"torchrun", "torch_shm_manager"},
	},
	"cloud-gcloud-logs": {
		minAgeDays:  debrisCloudAgeDays,
		resolveRoot: (*DebrisExecutor).resolveGCloudLogsRoot,
		match:       anyDebrisName,
	},
	"cloud-azure-logs": {
		minAgeDays:  debrisCloudAgeDays,
		resolveRoot: (*DebrisExecutor).resolveAzureLogsRoot,
		match:       anyDebrisName,
	},
}

// Run executes one closed debris profile.
func (e *DebrisExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := e.now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	profile, ok := debrisProfileRegistry[c.Profile()]
	if !ok {
		return e.finishDebrisError(emit, c, start, "unknown debris profile")
	}
	if c.Scope != model.ScopeUser {
		return e.finishDebrisSkip(emit, c, start, "profile_scope_mismatch", "debris profiles are user-scoped")
	}
	root, err := profile.resolveRoot(e)
	if err != nil {
		return e.finishDebrisSkip(emit, c, start, "root_unavailable", err.Error())
	}
	if c.Profile() != "desktop-download-partials" && safety.Blacklisted(root, e.Home) {
		return e.finishDebrisSkip(emit, c, start, "boundary_violation", "debris root is blacklisted")
	}
	rootFD, err := e.openDebrisRoot(root)
	if err != nil {
		reason := "boundary_violation"
		if errors.Is(err, errDebrisRootUnavailable) {
			reason = "root_unavailable"
		} else if errors.Is(err, errDebrisRootChanged) {
			reason = "root_identity_changed"
		} else if errors.Is(err, safety.ErrSymlink) {
			reason = string(model.SkipReasonSymlink)
		} else if errors.Is(err, safety.ErrCrossFS) {
			reason = string(model.SkipReasonCrossFS)
		}
		return e.finishDebrisSkip(emit, c, start, reason, err.Error())
	}
	if e.procVisibility != nil {
		if visibilityErr := e.procVisibility(e.procRoot, e.TargetUID); visibilityErr != nil {
			_ = unix.Close(rootFD)
			return e.finishDebrisSkip(emit, c, start, "proc_inspection_failed", visibilityErr.Error())
		}
	}
	if len(profile.processes) > 0 {
		busy, checkErr := e.processCheck(profile.processes, e.procRoot)
		if checkErr != nil {
			_ = unix.Close(rootFD)
			return e.finishDebrisSkip(emit, c, start, "proc_inspection_failed", checkErr.Error())
		}
		if busy {
			_ = unix.Close(rootFD)
			return e.finishDebrisSkip(emit, c, start, string(model.SkipReasonRunningProcess), "profile owner process is running")
		}
	}
	if profile.locks {
		busy, checkErr := inspectHuggingFaceBusyFD(rootFD, root, e.procRoot)
		if busy || checkErr != nil {
			_ = unix.Close(rootFD)
			detail := "cache lock or open descriptor indicates activity"
			if checkErr != nil {
				detail = "cache inspection failed closed: " + checkErr.Error()
			}
			return e.finishDebrisSkip(emit, c, start, "cache_busy", detail)
		}
	}

	w, err := safety.OpenWalkerFD(rootFD, root)
	if err != nil {
		return e.finishDebrisSkip(emit, c, start, "root_unavailable", err.Error())
	}
	defer w.Close()
	minAge := profile.minAgeDays
	if requested := c.AgeOrDefault(14); requested > minAge {
		minAge = requested
	}
	threshold := e.now().Add(-time.Duration(minAge) * 24 * time.Hour)
	field := safety.TimeFieldMtime
	if c.TimeField == model.TimeCtime {
		field = safety.TimeFieldCtime
	}
	var filesDeleted, bytesFreed, filesCandidate, bytesCandidate int64
	errs := 0
	walkErr := w.Walk(func(entry safety.Entry) {
		if ctx.Err() != nil {
			return
		}
		if profile.depth > 0 && debrisDepth(entry.RelPath) != profile.depth {
			return
		}
		if !entry.IsRegular() || entry.Nlink() != 1 || !profile.match(filepath.Base(entry.RelPath)) {
			return
		}
		stamp := entry.MtimeTime()
		if field == safety.TimeFieldCtime {
			stamp = entry.CtimeTime()
		}
		// Profile floors are mandatory; NoAgeFilter cannot lower them.
		if stamp.After(threshold) {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: filepath.Join(root, entry.RelPath), Reason: string(model.SkipReasonAgeTooRecent), TS: e.now()})
			return
		}
		abs := filepath.Join(root, entry.RelPath)
		if e.Whitelist != nil && e.Whitelist.Match(abs) {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonWhitelisted), TS: e.now()})
			return
		}
		open, openErr := safety.OpenDescriptorMatches(abs, entry.Dev(), entry.Ino(), e.procRoot)
		if openErr != nil {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "proc_inspection_failed", Detail: openErr.Error(), TS: e.now()})
			return
		}
		if open {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "open_file", TS: e.now()})
			return
		}
		if dryRun {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonDryRun), Size: entry.Size(), TS: e.now()})
			filesCandidate++
			bytesCandidate += entry.Size()
			return
		}
		if unlinkErr := w.UnlinkIfUnchanged(entry); unlinkErr != nil {
			if errors.Is(unlinkErr, safety.ErrChanged) {
				emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: string(model.SkipReasonToctouChanged), TS: e.now()})
				return
			}
			errs++
			emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "unlink_failed", Detail: unlinkErr.Error(), TS: e.now()})
			return
		}
		emit(model.Event{Event: model.EvDeleted, CleanerID: c.ID, Path: abs, Size: entry.Size(), TS: e.now()})
		filesDeleted++
		bytesFreed += entry.Size()
	})
	if walkErr != nil {
		errs++
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: root, Reason: "walk_failed", Detail: walkErr.Error(), TS: e.now()})
	}
	if ctx.Err() != nil {
		errs++
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: ctx.Err().Error(), TS: e.now()})
	}
	status := "ok"
	if errs > 0 {
		status = "error"
	}
	emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: status, FilesDeleted: filesDeleted, BytesFreed: bytesFreed, FilesCandidate: filesCandidate, BytesCandidate: bytesCandidate, Errors: errs, DurationMs: e.now().Sub(start).Milliseconds(), TS: e.now()})
	return nil
}

var (
	errDebrisRootUnavailable = errors.New("debris root unavailable")
	errDebrisRootChanged     = errors.New("debris root identity changed")
)

func (e *DebrisExecutor) finishDebrisSkip(emit func(model.Event), c model.Cleaner, start time.Time, reason, detail string) error {
	emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: reason, Detail: detail, TS: e.now()})
	emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "skipped", DurationMs: e.now().Sub(start).Milliseconds(), TS: e.now()})
	return nil
}

func (e *DebrisExecutor) finishDebrisError(emit func(model.Event), c model.Cleaner, start time.Time, detail string) error {
	emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: detail, TS: e.now()})
	emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "error", Errors: 1, DurationMs: e.now().Sub(start).Milliseconds(), TS: e.now()})
	return errors.New(detail)
}

func anyDebrisName(string) bool { return true }

func downloadPartialName(name string) bool {
	if name == "" || strings.HasSuffix(name, ".tmp") {
		return false
	}
	for _, suffix := range []string{".crdownload", ".opdownload", ".part", ".partial", ".download", ".aria2"} {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

func debrisDepth(rel string) int {
	clean := filepath.Clean(rel)
	if clean == "." || clean == string(filepath.Separator) {
		return 0
	}
	return 1 + strings.Count(clean, string(filepath.Separator))
}

func checkedProcessRunning(names []string, procRoot string) (bool, error) {
	return safety.AnyProcessRunningChecked(names, procRoot)
}

func (e *DebrisExecutor) getenv(name string) string {
	if e.env == nil {
		return ""
	}
	return e.env(name)
}

func (e *DebrisExecutor) resolveDownloadsRoot() (string, error) {
	if value := e.getenv("XDG_DOWNLOAD_DIR"); filepath.IsAbs(value) && debrisPathWithin(value, e.Home) && !strings.ContainsAny(value, "\x00$;`|&()") {
		return filepath.Clean(value), nil
	}
	if parsed, err := e.parseUserDirs(); err == nil && parsed != "" {
		return parsed, nil
	}
	return filepath.Join(e.Home, "Downloads"), nil
}

func (e *DebrisExecutor) parseUserDirs() (string, error) {
	path := e.userDirsPath
	if path == "" {
		configHome := e.getenv("XDG_CONFIG_HOME")
		if filepath.IsAbs(configHome) && debrisPathWithin(configHome, e.Home) {
			path = filepath.Join(configHome, "user-dirs.dirs")
		} else {
			path = filepath.Join(e.Home, ".config", "user-dirs.dirs")
		}
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !strings.HasPrefix(line, "XDG_DOWNLOAD_DIR") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) != "XDG_DOWNLOAD_DIR" {
			return "", errors.New("malformed XDG_DOWNLOAD_DIR")
		}
		value := strings.TrimSpace(parts[1])
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		if strings.ContainsAny(value, "\\;`|&()\n\r\t") {
			return "", errors.New("unsafe XDG_DOWNLOAD_DIR value")
		}
		if strings.HasPrefix(value, "$HOME/") {
			return filepath.Join(e.Home, value[len("$HOME/"):]), scanner.Err()
		}
		if filepath.IsAbs(value) {
			return filepath.Clean(value), scanner.Err()
		}
		return "", errors.New("XDG_DOWNLOAD_DIR must be absolute or $HOME-relative")
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("XDG_DOWNLOAD_DIR not found")
}

func (e *DebrisExecutor) resolveExplicitEnvRoot(name string) (string, error) {
	value := e.getenv(name)
	if !filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return "", fmt.Errorf("%s must be an absolute path", name)
	}
	return filepath.Clean(value), nil
}

func (e *DebrisExecutor) xdgCacheRoot() string {
	value := e.getenv("XDG_CACHE_HOME")
	if filepath.IsAbs(value) && debrisPathWithin(value, e.Home) {
		return filepath.Clean(value)
	}
	return filepath.Join(e.Home, ".cache")
}

func (e *DebrisExecutor) resolveHuggingFaceRoot() (string, error) {
	if value := e.getenv("HF_HOME"); filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Join(e.xdgCacheRoot(), "huggingface"), nil
}

func (e *DebrisExecutor) resolvePyInstallerRoot() (string, error) {
	return filepath.Join(e.xdgCacheRoot(), "pyinstaller"), nil
}

func (e *DebrisExecutor) resolveNVIDIARoot() (string, error) {
	if value := e.getenv("CUDA_CACHE_PATH"); filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Join(e.Home, ".nv", "ComputeCache"), nil
}

func (e *DebrisExecutor) resolveTritonRoot() (string, error) {
	if value := e.getenv("TRITON_CACHE_DIR"); filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Join(e.Home, ".triton", "cache"), nil
}

func (e *DebrisExecutor) resolveTorchInductorRoot() (string, error) {
	if value := e.getenv("TORCHINDUCTOR_CACHE_DIR"); filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Join(e.xdgCacheRoot(), "torch", "inductor"), nil
}

func (e *DebrisExecutor) resolveGCloudLogsRoot() (string, error) {
	if value := e.getenv("CLOUDSDK_LOG_DIR"); filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	return filepath.Join(e.Home, ".config", "gcloud", "logs"), nil
}

func (e *DebrisExecutor) resolveAzureLogsRoot() (string, error) {
	if value := e.getenv("AZURE_CONFIG_DIR"); filepath.IsAbs(value) {
		return filepath.Join(filepath.Clean(value), "commands"), nil
	}
	return filepath.Join(e.Home, ".azure", "commands"), nil
}

func readDebrisPathIdentity(path string) (debrisIdentity, error) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return debrisIdentity{}, err
	}
	var sx unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, unix.AT_SYMLINK_NOFOLLOW|unix.AT_STATX_SYNC_AS_STAT|unix.AT_NO_AUTOMOUNT, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &sx); err != nil {
		return debrisIdentity{}, fmt.Errorf("statx %q: %w", path, err)
	}
	if sx.Mnt_id == 0 {
		return debrisIdentity{}, errors.New("statx did not provide mount identity")
	}
	return debrisIdentity{Dev: uint64(st.Dev), Ino: uint64(st.Ino), UID: st.Uid, MntID: sx.Mnt_id, Mode: uint32(st.Mode & unix.S_IFMT)}, nil
}

func readDebrisFDIdentity(fd int) (debrisIdentity, error) {
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		return debrisIdentity{}, err
	}
	var sx unix.Statx_t
	if err := unix.Statx(fd, "", unix.AT_EMPTY_PATH|unix.AT_STATX_SYNC_AS_STAT|unix.AT_NO_AUTOMOUNT, unix.STATX_BASIC_STATS|unix.STATX_MNT_ID, &sx); err != nil {
		return debrisIdentity{}, fmt.Errorf("statx fd %d: %w", fd, err)
	}
	if sx.Mnt_id == 0 {
		return debrisIdentity{}, errors.New("statx fd did not provide mount identity")
	}
	return debrisIdentity{Dev: uint64(st.Dev), Ino: uint64(st.Ino), UID: st.Uid, MntID: sx.Mnt_id, Mode: uint32(st.Mode & unix.S_IFMT)}, nil
}

func sameDebrisIdentity(a, b debrisIdentity) bool {
	return a.Dev == b.Dev && a.Ino == b.Ino && a.UID == b.UID && a.MntID == b.MntID && a.Mode == b.Mode
}

func (e *DebrisExecutor) openDebrisRoot(root string) (int, error) {
	home := filepath.Clean(e.Home)
	root = filepath.Clean(root)
	if home == "." || !filepath.IsAbs(home) || !debrisPathWithin(root, home) || root == home {
		return -1, fmt.Errorf("debris root is outside target home")
	}
	homeFD, err := safety.OpenAnchoredDirFD(home, home)
	if err != nil {
		return -1, fmt.Errorf("%w: target home: %v", errDebrisRootUnavailable, err)
	}
	defer unix.Close(homeFD)
	homeID, err := e.fdIdentity(homeFD)
	if err != nil {
		return -1, fmt.Errorf("%w: target home identity: %v", errDebrisRootUnavailable, err)
	}
	if homeID.Mode != unix.S_IFDIR || homeID.UID != e.TargetUID || homeID.MntID == 0 {
		return -1, fmt.Errorf("debris target home ownership/type/mount identity invalid")
	}
	selected, err := e.pathIdentity(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return -1, fmt.Errorf("%w: %v", errDebrisRootUnavailable, err)
		}
		return -1, fmt.Errorf("%w: selected root identity: %v", errDebrisRootChanged, err)
	}
	if selected.Mode == unix.S_IFLNK {
		return -1, safety.ErrSymlink
	}
	if selected.Mode != unix.S_IFDIR {
		return -1, fmt.Errorf("debris root is not a directory")
	}
	if selected.UID != e.TargetUID {
		return -1, fmt.Errorf("debris root owner differs from target UID")
	}
	if selected.Dev != homeID.Dev {
		return -1, safety.ErrCrossFS
	}
	if selected.MntID == 0 || selected.MntID != homeID.MntID {
		return -1, errDebrisRootChanged
	}
	if e.beforeRootOpen != nil {
		if err := e.beforeRootOpen(root); err != nil {
			return -1, fmt.Errorf("%w: pre-open hook: %v", errDebrisRootChanged, err)
		}
	}
	fd, err := safety.OpenAnchoredDirFDFromFD(homeFD, home, root)
	if err != nil {
		return -1, err
	}
	opened, err := e.fdIdentity(fd)
	if err != nil {
		_ = unix.Close(fd)
		return -1, fmt.Errorf("%w: opened root identity: %v", errDebrisRootChanged, err)
	}
	if !sameDebrisIdentity(selected, opened) || opened.MntID != homeID.MntID {
		_ = unix.Close(fd)
		return -1, errDebrisRootChanged
	}
	return fd, nil
}

func debrisPathWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	return path != root && strings.HasPrefix(path, root+string(filepath.Separator))
}
