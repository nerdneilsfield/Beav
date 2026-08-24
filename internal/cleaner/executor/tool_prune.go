package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

// ToolPruneTarget describes the account used for a user-scope tool.
// ToolPruneTarget 描述用户级工具使用的目标账户。
type ToolPruneTarget = CommandIdentity

// ToolPruneOption configures a ToolPruneExecutor.
// ToolPruneOption 配置 ToolPruneExecutor。
type ToolPruneOption func(*ToolPruneExecutor)

// WithToolSearchPath sets the profile-specific executable search path. It is
// deliberately independent of the caller's PATH.
// WithToolSearchPath 设置 profile 专用可执行文件搜索路径，不继承调用方 PATH。
func WithToolSearchPath(paths ...string) ToolPruneOption {
	return func(e *ToolPruneExecutor) {
		e.SearchPath = append([]string(nil), paths...)
	}
}

// WithToolIdentity selects a target UID. All other fields are ignored and
// re-resolved through NSS immediately before a user profile is considered.
// WithToolIdentity 仅选择目标 UID；其余字段全部忽略，并在用户 profile 判定前经 NSS 重解析。
func WithToolIdentity(identity CommandIdentity) ToolPruneOption {
	return func(e *ToolPruneExecutor) {
		identity.Groups = append([]uint32(nil), identity.Groups...)
		e.Target = identity
		e.HasTarget = true
	}
}

// WithToolEnvironment adds fixed test/integration environment entries. Core
// identity and XDG variables are always rebuilt after these values.
// WithToolEnvironment 添加固定测试/集成环境项；核心身份及 XDG 变量总会重建。
func WithToolEnvironment(values map[string]string) ToolPruneOption {
	return func(e *ToolPruneExecutor) {
		if e.ExtraEnv == nil {
			e.ExtraEnv = map[string]string{}
		}
		for k, v := range values {
			e.ExtraEnv[k] = v
		}
	}
}

// WithToolTimeout sets the command and capability-probe timeout.
// WithToolTimeout 设置命令及能力探测超时。
func WithToolTimeout(timeout time.Duration) ToolPruneOption {
	return func(e *ToolPruneExecutor) {
		e.Timeout = timeout
	}
}

// ToolPruneExecutor runs only closed, built-in tool profiles.
// ToolPruneExecutor 仅运行闭合集成 tool profile。
type ToolPruneExecutor struct {
	Home       string
	Whitelist  *safety.Whitelist
	SearchPath []string
	Target     CommandIdentity
	HasTarget  bool
	ExtraEnv   map[string]string
	Timeout    time.Duration
	// binaryResolver is an in-package test seam. Production system profiles
	// always use resolveTrustedProfileBinary and cannot inject search paths.
	binaryResolver func(toolProfile, []string) (string, error)
}

// NewToolPruneExecutor creates a profile-backed external-tool executor.
// NewToolPruneExecutor 创建按 profile 驱动的外部工具执行器。
func NewToolPruneExecutor(home string, whitelist *safety.Whitelist, options ...ToolPruneOption) *ToolPruneExecutor {
	cleanHome := ""
	if home != "" {
		cleanHome = filepath.Clean(home)
	}
	e := &ToolPruneExecutor{
		Home:      cleanHome,
		Whitelist: whitelist,
		Timeout:   5 * time.Minute,
	}
	for _, option := range options {
		if option != nil {
			option(e)
		}
	}
	return e
}

// NewToolPruneExecutorForTarget is a convenience constructor for callers that
// already resolved target credentials.
// NewToolPruneExecutorForTarget 供已解析目标凭据的调用方使用。
func NewToolPruneExecutorForTarget(home string, whitelist *safety.Whitelist, target CommandIdentity, options ...ToolPruneOption) *ToolPruneExecutor {
	options = append([]ToolPruneOption{WithToolIdentity(target)}, options...)
	return NewToolPruneExecutor(home, whitelist, options...)
}

// ResolveToolIdentity resolves account name, primary GID, and exact
// supplementary groups for a target home. Callers using sudo --all should
// pass the UID/home resolved from the sudo environment.
// ResolveToolIdentity 解析目标账户名、主 GID 及确切附加组；sudo --all 调用方应传入已验证 UID/主目录。
func ResolveToolIdentity(uid uint32, home string) (CommandIdentity, error) {
	if uid == 0 {
		return CommandIdentity{}, errors.New("target UID must be non-zero")
	}
	return lookupIdentity(uid, "", home)
}

type toolProfile struct {
	Binary             string
	NativeDryRun       bool
	System             bool
	RootResolver       func(home string) []string
	BuildArgs          func(root string, age int, dryRun bool) []string
	Busy               func(root string) (bool, error)
	ProbeArgs          []string
	Required           []string
	DryRunProbeArgs    []string
	DryRunRequired     []string
	DryRunVersionWords []string
	VersionWords       []string
	ExtraEnv           map[string]string
}

var toolProfiles = map[string]toolProfile{
	"pkg-homebrew": {
		Binary:       "brew",
		NativeDryRun: true,
		RootResolver: homeRoot(".cache", "Homebrew"),
		BuildArgs: func(_ string, age int, dry bool) []string {
			args := []string{"cleanup", "--prune=" + strconv.Itoa(age)}
			if dry {
				args = append(args, "--dry-run")
			}
			return args
		},
		VersionWords:    []string{"brew"},
		DryRunProbeArgs: []string{"cleanup", "--help"},
		DryRunRequired:  []string{"--dry-run"},
		ExtraEnv: map[string]string{
			"HOMEBREW_NO_AUTO_UPDATE": "1",
			"HOMEBREW_NO_ANALYTICS":   "1",
			"HOMEBREW_NO_AUTOREMOVE":  "1",
		},
	},
	"lang-uv": {
		Binary:       "uv",
		RootResolver: homeRoot(".cache", "uv"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"cache", "prune"} },
		VersionWords: []string{"uv"},
	},
	"lang-pnpm": {
		Binary:       "pnpm",
		RootResolver: homeRoot(".local", "share", "pnpm", "store"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"store", "prune"} },
		VersionWords: []string{"pnpm"},
	},
	"dev-pre-commit": {
		Binary:       "pre-commit",
		RootResolver: homeRoot(".cache", "pre-commit"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"gc"} },
		VersionWords: []string{"pre-commit"},
	},
	"lang-conda": {
		Binary:       "conda",
		NativeDryRun: true,
		RootResolver: homeRoot(".conda"),
		BuildArgs: func(_ string, _ int, dry bool) []string {
			args := []string{"clean", "--index-cache", "--tarballs", "--logfiles", "--yes"}
			if dry {
				args = append(args, "--dry-run", "--json")
			}
			return args
		},
		DryRunProbeArgs: []string{"clean", "--help"},
		DryRunRequired:  []string{"--dry-run", "--json"},
		VersionWords:    []string{"conda"},
	},
	"test-cypress": {
		Binary:       "cypress",
		RootResolver: homeRoot(".cache", "Cypress"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"cache", "prune"} },
		VersionWords: []string{"cypress"},
	},
	"build-ccache": {
		Binary:       "ccache",
		RootResolver: homeRoot(".cache", "ccache"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"--cleanup"} },
		VersionWords: []string{"ccache"},
	},
	"ai-huggingface-prune": {
		Binary:       "hf",
		NativeDryRun: true,
		RootResolver: homeRoot(".cache", "huggingface"),
		BuildArgs: func(root string, _ int, dry bool) []string {
			args := []string{"cache", "prune", "--cache-dir", root, "--yes"}
			if dry {
				args = append(args, "--dry-run")
			}
			return args
		},
		Busy:           huggingFaceRootBusy,
		ProbeArgs:      []string{"cache", "prune", "--help"},
		Required:       []string{"--cache-dir", "--yes"},
		DryRunRequired: []string{"--dry-run"},
		VersionWords:   []string{"hf", "huggingface"},
	},
	"dev-mise": {
		Binary:       "mise",
		RootResolver: homeRoot(".cache", "mise"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"prune", "--yes"} },
		ProbeArgs:    []string{"prune", "--help"},
		Required:     []string{"--yes"},
		VersionWords: []string{"mise"},
	},
	"lang-corepack": {
		Binary:       "corepack",
		RootResolver: homeRoot(".cache", "node", "corepack"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"cache", "clean"} },
		VersionWords: []string{"corepack"},
	},
	"sys-nix-gc": {
		Binary:       "nix-collect-garbage",
		NativeDryRun: true,
		System:       true,
		RootResolver: func(_ string) []string { return []string{"/nix"} },
		BuildArgs: func(_ string, _ int, dry bool) []string {
			if dry {
				return []string{"--dry-run"}
			}
			return nil
		},
		DryRunProbeArgs: []string{"--help"},
		DryRunRequired:  []string{"--dry-run"},
		VersionWords:    []string{"nix"},
	},
	"desktop-flatpak-user": {
		Binary:       "flatpak",
		RootResolver: homeRoot(".local", "share", "flatpak"),
		BuildArgs: func(_ string, _ int, _ bool) []string {
			return []string{"uninstall", "--user", "--unused", "--noninteractive"}
		},
		ProbeArgs:    []string{"uninstall", "--help"},
		Required:     []string{"--noninteractive"},
		VersionWords: []string{"flatpak"},
	},
	"desktop-flatpak-system": {
		Binary:       "flatpak",
		System:       true,
		RootResolver: func(_ string) []string { return []string{"/var/lib/flatpak"} },
		BuildArgs: func(_ string, _ int, _ bool) []string {
			return []string{"uninstall", "--system", "--unused", "--noninteractive"}
		},
		ProbeArgs:    []string{"uninstall", "--help"},
		Required:     []string{"--noninteractive"},
		VersionWords: []string{"flatpak"},
	},
	"lang-poetry-cache": {
		Binary:       "poetry",
		RootResolver: homeRoot(".cache", "pypoetry"),
		BuildArgs: func(_ string, _ int, _ bool) []string {
			return []string{"cache", "clear", "pypi", "--all", "--no-interaction"}
		},
		VersionWords: []string{"poetry"},
	},
	"test-playwright-browsers": {
		Binary:       "playwright",
		RootResolver: homeRoot(".cache", "ms-playwright"),
		BuildArgs:    func(_ string, _ int, _ bool) []string { return []string{"uninstall", "--all"} },
		VersionWords: []string{"playwright"},
	},
}

func homeRoot(parts ...string) func(string) []string {
	return func(home string) []string { return []string{filepath.Join(append([]string{home}, parts...)...)} }
}

// Run executes one closed tool profile. No profile accepts command or path
// data from YAML.
// Run 执行一个闭合 tool profile；任何 profile 均不接受 YAML 命令或路径。
func (e *ToolPruneExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := time.Now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	profileName := c.Profile()
	profile, ok := toolProfiles[profileName]
	if !ok {
		return e.finishToolError(emit, c, start, "unknown tool profile", nil)
	}
	if profile.System != (c.Scope == model.ScopeSystem) {
		return e.finishToolSkip(emit, c, start, "profile_scope_mismatch", "profile scope is fixed")
	}
	identity, err := e.identityFor(c, profile)
	if err != nil {
		return e.finishToolSkip(emit, c, start, "identity_unavailable", err.Error())
	}
	checkedRoots, err := e.resolveCheckedRoots(c, profile, identity)
	if err != nil {
		reason := "boundary_violation"
		if errors.Is(err, errKeepListIntersection) {
			reason = string(model.SkipReasonKeepListIntersection)
		} else if errors.Is(err, errRootUnavailable) {
			reason = "root_unavailable"
		}
		return e.finishToolSkip(emit, c, start, reason, err.Error())
	}
	roots := make([]string, 0, len(checkedRoots))
	for _, checkedRoot := range checkedRoots {
		roots = append(roots, checkedRoot.path)
	}
	if profile.Busy != nil {
		for _, root := range roots {
			busy, busyErr := profile.Busy(root)
			if busy || busyErr != nil {
				detail := "cache lock or open descriptor indicates activity"
				if busyErr != nil {
					detail = "cache inspection failed closed: " + busyErr.Error()
				}
				return e.finishToolSkip(emit, c, start, "cache_busy", detail)
			}
		}
	}
	if dryRun && !profile.NativeDryRun {
		return e.finishToolSkipIncomplete(emit, c, start, model.SkipReasonDryRunUnsupported, "profile has no native dry-run")
	}
	searchPath := e.searchPath(profile, identity.Home)
	binary, err := e.resolveBinary(profile, identity.Home)
	if err != nil {
		return e.finishToolSkip(emit, c, start, string(model.SkipReasonManagerNotInstalled), err.Error())
	}
	binaryState, err := snapshotPath(binary)
	if err != nil {
		return e.finishToolSkip(emit, c, start, string(model.SkipReasonManagerNotInstalled), err.Error())
	}
	if e.trustedBinary(profile) {
		if err := validateTrustedPath(binary); err != nil {
			return e.finishToolSkip(emit, c, start, string(model.SkipReasonManagerNotInstalled), err.Error())
		}
	}
	env := e.environment(identity, profile, searchPath)
	commandIdentity := e.executionIdentity(identity)
	if err := e.validateToolState(profile, checkedRoots, binary, binaryState); err != nil {
		return e.finishToolSkip(emit, c, start, "safety_changed", err.Error())
	}
	if err := e.probe(ctx, binary, profile, identity, commandIdentity, env, dryRun); err != nil {
		reason := "capability_unsupported"
		if errors.Is(err, errVersionMismatch) {
			reason = "version_mismatch"
		}
		return e.finishToolSkip(emit, c, start, reason, err.Error())
	}
	if err := e.validateToolState(profile, checkedRoots, binary, binaryState); err != nil {
		return e.finishToolSkip(emit, c, start, "safety_changed", err.Error())
	}
	args := profile.BuildArgs(roots[0], c.AgeOrDefault(14), dryRun)
	var errs int
	err = RunCommand(ctx, CommandRun{
		Executable:    binary,
		Args:          args,
		Env:           env,
		Cwd:           identity.Home,
		Identity:      commandIdentity,
		Timeout:       e.Timeout,
		DryRun:        dryRun,
		AffectedRoots: append([]string(nil), roots...),
		OnStdout: func(line string) {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: line, TS: time.Now()})
		},
		OnStderr: func(line string) {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stderr", Line: line, TS: time.Now()})
		},
	})
	if err != nil {
		errs = 1
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "command_failed", Detail: err.Error(), TS: time.Now()})
	}
	status := "ok"
	if errs != 0 {
		status = "error"
	}
	emitFinishWithMetrics(emit, c.ID, status, errs, start, true)
	return err
}

var (
	errKeepListIntersection = errors.New("keep-list intersects affected root")
	errRootUnavailable      = errors.New("affected root unavailable")
	errVersionMismatch      = errors.New("tool version mismatch")
)

func (e *ToolPruneExecutor) finishToolSkip(emit func(model.Event), c model.Cleaner, start time.Time, reason, detail string) error {
	emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: reason, Detail: detail, TS: time.Now()})
	emitFinish(emit, c.ID, "skipped", 0, start)
	return nil
}

func (e *ToolPruneExecutor) finishToolSkipIncomplete(emit func(model.Event), c model.Cleaner, start time.Time, reason model.SkipReason, detail string) error {
	emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: reason.String(), Detail: detail, TS: time.Now()})
	emitFinishWithMetrics(emit, c.ID, "skipped", 0, start, true)
	return nil
}

func (e *ToolPruneExecutor) finishToolError(emit func(model.Event), c model.Cleaner, start time.Time, detail string, err error) error {
	if err == nil {
		err = errors.New(detail)
	}
	emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: detail, TS: time.Now()})
	emitFinish(emit, c.ID, "error", 1, start)
	return err
}

func (e *ToolPruneExecutor) identityFor(c model.Cleaner, profile toolProfile) (CommandIdentity, error) {
	if profile.System {
		if os.Geteuid() != 0 {
			return CommandIdentity{}, errors.New("system profile requires root")
		}
		return CommandIdentity{UID: 0, GID: 0, Groups: []uint32{}, Name: "root", Home: "/root"}, nil
	}
	if e.Home == "" || !filepath.IsAbs(e.Home) || c.Scope != model.ScopeUser {
		return CommandIdentity{}, errors.New("user profile requires target home")
	}
	if e.HasTarget {
		if e.Target.UID == 0 {
			return CommandIdentity{}, errors.New("target UID must be non-zero")
		}
		if os.Geteuid() != 0 && e.Target.UID != uint32(os.Getuid()) {
			return CommandIdentity{}, errors.New("non-root process cannot target another UID")
		}
		// Target GID, groups, name, and home are mutable caller input. Resolve
		// every one from NSS; e.Home is the independently validated operation
		// boundary used by tests and by the caller's target-home selection.
		identity, err := lookupIdentity(e.Target.UID, "", e.Home)
		if err != nil {
			return CommandIdentity{}, err
		}
		if err := ensureCurrentIdentity(identity); err != nil {
			return CommandIdentity{}, err
		}
		return identity, nil
	}
	uid := uint32(os.Getuid())
	if uid == 0 {
		// A root process cleaning --all must carry an explicitly resolved target
		// identity. Falling back to root here would violate the sudo boundary.
		sudoUID := os.Getenv("SUDO_UID")
		sudoUser := os.Getenv("SUDO_USER")
		if sudoUID == "" || sudoUser == "" {
			return CommandIdentity{}, errors.New("target identity required for root user-scope tool")
		}
		parsed, parseErr := strconv.ParseUint(sudoUID, 10, 32)
		if parseErr != nil || parsed == 0 {
			return CommandIdentity{}, errors.New("SUDO_UID invalid or zero")
		}
		if targetUser, lookupErr := user.LookupId(strconv.FormatUint(parsed, 10)); lookupErr != nil || filepath.Clean(targetUser.HomeDir) != filepath.Clean(e.Home) {
			return CommandIdentity{}, errors.New("target home does not match SUDO target")
		}
		identity, lookupErr := lookupIdentity(uint32(parsed), sudoUser, e.Home)
		if lookupErr != nil {
			return CommandIdentity{}, lookupErr
		}
		return identity, nil
	}
	identity, err := lookupIdentity(uid, "", e.Home)
	if err != nil {
		return CommandIdentity{}, err
	}
	if err := ensureCurrentIdentity(identity); err != nil {
		return CommandIdentity{}, err
	}
	return identity, nil
}

func ensureCurrentIdentity(identity CommandIdentity) error {
	if os.Geteuid() == 0 {
		return nil
	}
	if uint32(os.Getuid()) != identity.UID || uint32(os.Getgid()) != identity.GID {
		return errors.New("current process credentials do not match target identity")
	}
	groups, err := unix.Getgroups()
	if err != nil {
		return fmt.Errorf("read current supplementary groups: %w", err)
	}
	actual := make([]uint32, len(groups))
	actual = actual[:0]
	for _, gid := range groups {
		if gid < 0 {
			return errors.New("current supplementary GID is invalid")
		}
		if uint32(gid) != identity.GID {
			actual = append(actual, uint32(gid))
		}
	}
	sort.Slice(actual, func(i, j int) bool { return actual[i] < actual[j] })
	want := append([]uint32(nil), identity.Groups...)
	sort.Slice(want, func(i, j int) bool { return want[i] < want[j] })
	if len(actual) != len(want) {
		return fmt.Errorf("current supplementary groups do not match target identity: got %v want %v", actual, want)
	}
	for i := range actual {
		if actual[i] != want[i] {
			return fmt.Errorf("current supplementary groups do not match target identity: got %v want %v", actual, want)
		}
	}
	return nil
}

func lookupIdentity(uid uint32, expectedName, home string) (CommandIdentity, error) {
	if uid == 0 {
		return CommandIdentity{}, errors.New("target UID must be non-zero")
	}
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return CommandIdentity{}, err
	}
	if expectedName != "" && u.Username != expectedName {
		return CommandIdentity{}, errors.New("target username does not match SUDO_USER")
	}
	gid64, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return CommandIdentity{}, err
	}
	if gid64 == 0 {
		return CommandIdentity{}, errors.New("target primary GID must be non-zero")
	}
	groups, err := u.GroupIds()
	if err != nil {
		return CommandIdentity{}, err
	}
	groupIDs := make([]uint32, 0, len(groups))
	for _, raw := range groups {
		gid, parseErr := strconv.ParseUint(raw, 10, 32)
		if parseErr != nil {
			return CommandIdentity{}, fmt.Errorf("invalid supplementary GID %q: %w", raw, parseErr)
		}
		if gid == 0 {
			return CommandIdentity{}, errors.New("target supplementary GID must be non-zero")
		}
		if uint32(gid) == uint32(gid64) {
			continue
		}
		groupIDs = append(groupIDs, uint32(gid))
	}
	if home == "" {
		home = u.HomeDir
	}
	identity := normalizeIdentity(CommandIdentity{UID: uid, GID: uint32(gid64), Groups: groupIDs, Name: u.Username, Home: home})
	if err := verifyTargetHome(identity); err != nil {
		return CommandIdentity{}, err
	}
	return identity, nil
}

func verifyTargetHome(identity CommandIdentity) error {
	if identity.Home == "" {
		return errors.New("target home is empty")
	}
	var st unix.Stat_t
	if err := unix.Lstat(identity.Home, &st); err != nil {
		return err
	}
	if st.Mode&unix.S_IFMT == unix.S_IFLNK || st.Mode&unix.S_IFMT != unix.S_IFDIR {
		return errors.New("target home is not a directory")
	}
	if st.Uid != identity.UID {
		return errors.New("target home is not target-owned")
	}
	return nil
}

func normalizeIdentity(identity CommandIdentity) CommandIdentity {
	seen := map[uint32]bool{}
	groups := make([]uint32, 0, len(identity.Groups))
	for _, gid := range identity.Groups {
		if gid == 0 || gid == identity.GID || seen[gid] {
			continue
		}
		seen[gid] = true
		groups = append(groups, gid)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i] < groups[j] })
	identity.Groups = groups
	return identity
}

func (e *ToolPruneExecutor) resolveAndCheckRoots(c model.Cleaner, profile toolProfile, identity CommandIdentity) ([]string, error) {
	checked, err := e.resolveCheckedRoots(c, profile, identity)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(checked))
	for _, root := range checked {
		roots = append(roots, root.path)
	}
	return roots, nil
}

func (e *ToolPruneExecutor) resolveCheckedRoots(c model.Cleaner, profile toolProfile, identity CommandIdentity) ([]checkedRoot, error) {
	roots := profile.RootResolver(identity.Home)
	if len(roots) == 0 {
		return nil, errRootUnavailable
	}
	checked := make([]checkedRoot, 0, len(roots))
	for _, root := range roots {
		root = filepath.Clean(root)
		if !filepath.IsAbs(root) {
			return nil, fmt.Errorf("affected root %q is not absolute", root)
		}
		if e.Whitelist != nil && e.Whitelist.Intersects(root) {
			return nil, fmt.Errorf("%w: %s", errKeepListIntersection, root)
		}
		if profile.System {
			if !fixedSystemRoot(profile.Binary, root) {
				return nil, fmt.Errorf("affected root %q is not fixed for profile", root)
			}
		} else {
			if !pathWithin(root, identity.Home) || root == filepath.Clean(identity.Home) || safety.Blacklisted(root, identity.Home) {
				return nil, fmt.Errorf("affected root %q is outside target home", root)
			}
		}
		fd, err := safety.OpenAnchoredDirFD(rootAnchor(identity.Home, profile.System), root)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", errRootUnavailable, root, err)
		}
		var st unix.Stat_t
		err = unix.Fstat(fd, &st)
		_ = unix.Close(fd)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %v", errRootUnavailable, root, err)
		}
		if st.Uid != identity.UID && !profile.System {
			return nil, fmt.Errorf("affected root %q is not target-owned", root)
		}
		if profile.System && st.Uid != 0 {
			return nil, fmt.Errorf("affected system root %q is not root-owned", root)
		}
		snapshot, snapshotErr := snapshotPath(root)
		if snapshotErr != nil {
			return nil, fmt.Errorf("%w: %s: %v", errRootUnavailable, root, snapshotErr)
		}
		if snapshot != snapshotFromStat(&st) {
			return nil, fmt.Errorf("%w: affected root %q changed during validation", errRootUnavailable, root)
		}
		if profile.System && snapshot.mode&0o022 != 0 {
			return nil, fmt.Errorf("affected system root %q is group/world writable", root)
		}
		checked = append(checked, checkedRoot{path: root, snapshot: snapshot})
	}
	return checked, nil
}

// AffectedRoots resolves and validates all roots declared by a profile before
// any external command is considered. It is useful to callers that want to
// display the safety decision without executing the profile.
// AffectedRoots 在考虑执行 profile 前解析并验证其声明的全部根。
func (e *ToolPruneExecutor) AffectedRoots(c model.Cleaner) ([]string, error) {
	profile, ok := toolProfiles[c.Profile()]
	if !ok {
		return nil, fmt.Errorf("unknown tool profile %q", c.Profile())
	}
	identity, err := e.identityFor(c, profile)
	if err != nil {
		return nil, err
	}
	return e.resolveAndCheckRoots(c, profile, identity)
}

func rootAnchor(home string, system bool) string {
	if system {
		return "/"
	}
	return filepath.Clean(home)
}

func fixedSystemRoot(binary, root string) bool {
	switch binary {
	case "nix-collect-garbage":
		return root == "/nix"
	case "flatpak":
		return root == "/var/lib/flatpak"
	default:
		return false
	}
}

func pathWithin(path, root string) bool {
	path, root = filepath.Clean(path), filepath.Clean(root)
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

type pathSnapshot struct {
	dev  uint64
	ino  uint64
	uid  uint32
	mode uint32
}

type checkedRoot struct {
	path     string
	snapshot pathSnapshot
}

func snapshotFromStat(st *unix.Stat_t) pathSnapshot {
	return pathSnapshot{dev: uint64(st.Dev), ino: uint64(st.Ino), uid: st.Uid, mode: st.Mode}
}

func snapshotPath(path string) (pathSnapshot, error) {
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil {
		return pathSnapshot{}, err
	}
	if st.Mode&unix.S_IFMT == unix.S_IFLNK {
		return pathSnapshot{}, fmt.Errorf("path %q is a symlink", path)
	}
	return snapshotFromStat(&st), nil
}

func (s pathSnapshot) revalidate(path string) error {
	current, err := snapshotPath(path)
	if err != nil {
		return err
	}
	if current != s {
		return fmt.Errorf("path %q changed after validation", path)
	}
	return nil
}

func (e *ToolPruneExecutor) searchPath(profile toolProfile, home string) []string {
	if profile.System {
		return defaultToolSearchPath("", true)
	}
	if len(e.SearchPath) > 0 {
		return cleanSearchPath(e.SearchPath)
	}
	return defaultToolSearchPath(home, profile.System)
}

func defaultToolSearchPath(home string, system bool) []string {
	paths := []string{}
	if !system && home != "" {
		paths = append(paths, filepath.Join(home, ".local", "bin"), filepath.Join(home, ".cargo", "bin"))
	}
	paths = append(paths,
		"/usr/local/bin",
		"/usr/bin",
		"/bin",
		"/usr/local/sbin",
		"/usr/sbin",
		"/sbin",
	)
	return cleanSearchPath(paths)
}

func cleanSearchPath(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		abs = filepath.Clean(abs)
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

func resolveProfileBinary(name string, searchPath []string) (string, error) {
	return resolveProfileBinaryWithTrust(name, searchPath, false)
}

func resolveTrustedProfileBinary(name string, searchPath []string) (string, error) {
	return resolveProfileBinaryWithTrust(name, searchPath, true)
}

func resolveProfileBinaryWithTrust(name string, searchPath []string, trusted bool) (string, error) {
	for _, dir := range searchPath {
		candidate := filepath.Join(dir, name)
		info, err := os.Lstat(candidate)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if trusted {
			if err := validateTrustedPath(abs); err != nil {
				continue
			}
		}
		return abs, nil
	}
	return "", fmt.Errorf("profile binary %q not found", name)
}

func (e *ToolPruneExecutor) resolveBinary(profile toolProfile, homes ...string) (string, error) {
	home := e.Home
	if len(homes) > 0 {
		home = homes[0]
	}
	searchPath := e.searchPath(profile, home)
	if e.binaryResolver != nil {
		return e.binaryResolver(profile, searchPath)
	}
	if profile.System {
		return resolveTrustedProfileBinary(profile.Binary, searchPath)
	}
	return resolveProfileBinary(profile.Binary, searchPath)
}

func (e *ToolPruneExecutor) trustedBinary(profile toolProfile) bool {
	return profile.System && e.binaryResolver == nil
}

func (e *ToolPruneExecutor) validateToolState(profile toolProfile, roots []checkedRoot, binary string, binaryState pathSnapshot) error {
	for _, root := range roots {
		if err := root.snapshot.revalidate(root.path); err != nil {
			return fmt.Errorf("affected root %q changed: %w", root.path, err)
		}
		if profile.System {
			current, err := snapshotPath(root.path)
			if err != nil {
				return fmt.Errorf("affected system root %q unavailable: %w", root.path, err)
			}
			if current.uid != 0 || current.mode&0o022 != 0 {
				return fmt.Errorf("affected system root %q lost trusted ownership/mode", root.path)
			}
		}
	}
	if err := binaryState.revalidate(binary); err != nil {
		return fmt.Errorf("profile binary %q changed: %w", binary, err)
	}
	if e.trustedBinary(profile) {
		if err := validateTrustedPath(binary); err != nil {
			return fmt.Errorf("profile binary %q lost trusted path: %w", binary, err)
		}
	}
	return nil
}

func validateTrustedPath(path string) error {
	path = filepath.Clean(path)
	if !filepath.IsAbs(path) {
		return errors.New("trusted path must be absolute")
	}
	rootInfo, err := os.Lstat(string(filepath.Separator))
	if err != nil {
		return err
	}
	rootStat, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok || rootStat.Uid != 0 || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm()&0o022 != 0 || !rootInfo.IsDir() {
		return errors.New("trusted path root is not secure")
	}
	current := string(filepath.Separator)
	for _, part := range strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" || part == "." || part == ".." {
			return errors.New("trusted path contains invalid component")
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("trusted path component %q is symlink", current)
		}
		if info.Mode().Perm()&0o022 != 0 {
			return fmt.Errorf("trusted path component %q is group/world writable", current)
		}
		stat, ok := info.Sys().(*syscall.Stat_t)
		if !ok || stat.Uid != 0 {
			return fmt.Errorf("trusted path component %q is not root-owned", current)
		}
		if current != path && !info.IsDir() {
			return fmt.Errorf("trusted path component %q is not a directory", current)
		}
	}
	return nil
}

func (e *ToolPruneExecutor) environment(identity CommandIdentity, profile toolProfile, searchPath []string) []string {
	home := identity.Home
	lang := os.Getenv("LANG")
	if lang == "" {
		lang = "C.UTF-8"
	}
	values := map[string]string{
		"HOME":            home,
		"USER":            identity.Name,
		"LOGNAME":         identity.Name,
		"XDG_CACHE_HOME":  filepath.Join(home, ".cache"),
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		"XDG_DATA_HOME":   filepath.Join(home, ".local", "share"),
		"XDG_STATE_HOME":  filepath.Join(home, ".local", "state"),
		"XDG_RUNTIME_DIR": filepath.Join("/run/user", strconv.FormatUint(uint64(identity.UID), 10)),
		"TMPDIR":          "/tmp",
		"PATH":            strings.Join(searchPath, string(os.PathListSeparator)),
		"LANG":            lang,
	}
	for key, value := range profile.ExtraEnv {
		values[key] = value
	}
	for key, value := range e.ExtraEnv {
		if _, reserved := values[key]; !reserved {
			values[key] = value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	env := make([]string, 0, len(keys))
	for _, key := range keys {
		env = append(env, key+"="+values[key])
	}
	return env
}

func (e *ToolPruneExecutor) executionIdentity(identity CommandIdentity) *CommandIdentity {
	// A non-root process cannot call setgroups, even with its current group
	// list. It already has the desired identity, so avoid a failing redundant
	// credential transition. sudo --all runs as root and takes this path.
	if os.Geteuid() != 0 {
		return nil
	}
	return &identity
}

func (e *ToolPruneExecutor) probe(ctx context.Context, binary string, profile toolProfile, identity CommandIdentity, commandIdentity *CommandIdentity, env []string, dryRun bool) error {
	args := profile.ProbeArgs
	required := profile.Required
	versionWords := profile.VersionWords
	if dryRun {
		if len(profile.DryRunProbeArgs) > 0 {
			args = profile.DryRunProbeArgs
		}
		required = profile.DryRunRequired
		versionWords = profile.DryRunVersionWords
	}
	if len(args) == 0 {
		args = []string{"--version"}
	}
	var (
		linesMu sync.Mutex
		lines   []string
	)
	err := RunCommand(ctx, CommandRun{
		Executable: binary,
		Args:       args,
		Env:        env,
		Cwd:        identity.Home,
		Identity:   commandIdentity,
		Timeout:    e.probeTimeout(),
		OnStdout: func(line string) {
			linesMu.Lock()
			lines = append(lines, line)
			linesMu.Unlock()
		},
		OnStderr: func(line string) {
			linesMu.Lock()
			lines = append(lines, line)
			linesMu.Unlock()
		},
	})
	if err != nil {
		return fmt.Errorf("capability probe failed: %w", err)
	}
	joined := strings.TrimSpace(strings.Join(lines, "\n"))
	if joined == "" {
		return errors.New("capability probe produced empty output")
	}
	if len(versionWords) > 0 {
		lowerJoined := strings.ToLower(joined)
		matched := false
		for _, word := range versionWords {
			if strings.Contains(lowerJoined, strings.ToLower(word)) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%w: %s", errVersionMismatch, joined)
		}
	}
	if len(required) > 0 {
		for _, flag := range required {
			if !strings.Contains(joined, flag) {
				return fmt.Errorf("required capability %q unavailable", flag)
			}
		}
	}
	return nil
}

func (e *ToolPruneExecutor) probeTimeout() time.Duration {
	if e.Timeout > 0 && e.Timeout < 10*time.Second {
		return e.Timeout
	}
	return 10 * time.Second
}

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
			link = strings.TrimSuffix(link, " (deleted)")
			if !pathWithin(filepath.Clean(link), root) {
				continue
			}
			if _, err := os.Stat(fdPath); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					continue
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
