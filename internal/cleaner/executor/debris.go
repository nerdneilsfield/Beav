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

// DebrisExecutor removes only artifacts named by the closed debris profile
// registry. It never accepts roots or matching rules from YAML.
type DebrisExecutor struct {
	Home      string
	Whitelist *safety.Whitelist
	TargetUID uint32

	procRoot     string
	env          func(string) string
	userDirsPath string
	processCheck func([]string, string) (bool, error)
	now          func() time.Time
}

// NewDebrisExecutor creates a profile-backed debris executor. The optional
// UID exists for callers that already resolved a target account.
func NewDebrisExecutor(home string, wl *safety.Whitelist, targetUID ...uint32) *DebrisExecutor {
	uid := uint32(os.Getuid())
	if len(targetUID) > 0 {
		uid = targetUID[0]
	}
	return &DebrisExecutor{
		Home:         filepath.Clean(home),
		Whitelist:    wl,
		TargetUID:    uid,
		procRoot:     "/proc",
		env:          os.Getenv,
		processCheck: checkedProcessRunning,
		now:          time.Now,
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
		processes:   []string{"nvidia-persistenced", "nvidia-cuda-mps-server", "python", "python3"},
	},
	"gpu-triton-cache": {
		minAgeDays:  debrisGPUAgeDays,
		resolveRoot: (*DebrisExecutor).resolveTritonRoot,
		match:       anyDebrisName,
		processes:   []string{"triton", "tritonserver", "python", "python3"},
	},
	"gpu-torch-inductor-cache": {
		minAgeDays:  debrisGPUAgeDays,
		resolveRoot: (*DebrisExecutor).resolveTorchInductorRoot,
		match:       anyDebrisName,
		processes:   []string{"torchrun", "torch_shm_manager", "python", "python3"},
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
	if err := e.validateDebrisRoot(root); err != nil {
		reason := "boundary_violation"
		if errors.Is(err, errDebrisRootUnavailable) {
			reason = "root_unavailable"
		} else if errors.Is(err, safety.ErrSymlink) {
			reason = string(model.SkipReasonSymlink)
		} else if errors.Is(err, safety.ErrCrossFS) {
			reason = string(model.SkipReasonCrossFS)
		}
		return e.finishDebrisSkip(emit, c, start, reason, err.Error())
	}
	if len(profile.processes) > 0 {
		busy, checkErr := e.processCheck(profile.processes, e.procRoot)
		if checkErr != nil {
			return e.finishDebrisSkip(emit, c, start, "proc_inspection_failed", checkErr.Error())
		}
		if busy {
			return e.finishDebrisSkip(emit, c, start, string(model.SkipReasonRunningProcess), "profile owner process is running")
		}
	}
	if profile.locks {
		busy, checkErr := inspectHuggingFaceBusy(root, e.procRoot)
		if busy || checkErr != nil {
			detail := "cache lock or open descriptor indicates activity"
			if checkErr != nil {
				detail = "cache inspection failed closed: " + checkErr.Error()
			}
			return e.finishDebrisSkip(emit, c, start, "cache_busy", detail)
		}
	}

	dirFD, err := safety.OpenAnchoredDirFD(e.Home, root)
	if err != nil {
		return e.finishDebrisSkip(emit, c, start, "root_unavailable", err.Error())
	}
	w, err := safety.OpenWalkerFD(dirFD, root)
	if err != nil {
		return e.finishDebrisSkip(emit, c, start, "root_unavailable", err.Error())
	}
	defer w.Close()
	minAge := profile.minAgeDays
	if requested := c.AgeOrDefault(14); requested > minAge {
		minAge = requested
	}
	threshold := e.now().Add(-time.Duration(minAge) * 24 * time.Hour).Unix()
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
		stamp := entry.Mtime()
		if field == safety.TimeFieldCtime {
			stamp = entry.Ctime()
		}
		// Profile floors are mandatory; NoAgeFilter cannot lower them.
		if stamp > threshold {
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

var errDebrisRootUnavailable = errors.New("debris root unavailable")

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

func (e *DebrisExecutor) validateDebrisRoot(root string) error {
	home := filepath.Clean(e.Home)
	root = filepath.Clean(root)
	if home == "." || !filepath.IsAbs(home) || !debrisPathWithin(root, home) || root == home {
		return fmt.Errorf("debris root is outside target home")
	}
	var homeSt, rootSt unix.Stat_t
	if err := unix.Lstat(home, &homeSt); err != nil {
		return fmt.Errorf("%w: target home: %v", errDebrisRootUnavailable, err)
	}
	if homeSt.Mode&unix.S_IFMT != unix.S_IFDIR || homeSt.Uid != e.TargetUID {
		return fmt.Errorf("debris target home ownership/type invalid")
	}
	if err := unix.Lstat(root, &rootSt); err != nil {
		return fmt.Errorf("%w: %v", errDebrisRootUnavailable, err)
	}
	if rootSt.Mode&unix.S_IFMT == unix.S_IFLNK {
		return safety.ErrSymlink
	}
	if rootSt.Mode&unix.S_IFMT != unix.S_IFDIR {
		return fmt.Errorf("debris root is not a directory")
	}
	if rootSt.Uid != e.TargetUID {
		return fmt.Errorf("debris root owner differs from target UID")
	}
	if rootSt.Dev != homeSt.Dev {
		return safety.ErrCrossFS
	}
	fd, err := safety.OpenAnchoredDirFD(home, root)
	if err != nil {
		return err
	}
	return unix.Close(fd)
}

func debrisPathWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	return path != root && strings.HasPrefix(path, root+string(filepath.Separator))
}
