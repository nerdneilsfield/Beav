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

// WithToolIdentity sets the exact UID, primary GID, and supplementary groups.
// WithToolIdentity 设置确切 UID、主 GID 及附加组。
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

type toolProfile struct {
	Binary       string
	NativeDryRun bool
	System       bool
	RootResolver func(home string) []string
	BuildArgs    func(root string, age int, dryRun bool) []string
	Busy         func(root string) bool
	ProbeArgs    []string
	Required     []string
	VersionWords []string
	ExtraEnv     map[string]string
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
		VersionWords: []string{"brew"},
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
		Required:     []string{"--dry-run", "--json"},
		VersionWords: []string{"conda"},
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
		Busy:         huggingFaceRootBusy,
		ProbeArgs:    []string{"cache", "prune", "--help"},
		Required:     []string{"--cache-dir", "--yes", "--dry-run"},
		VersionWords: []string{"hf", "huggingface"},
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
		System:       true,
		RootResolver: func(_ string) []string { return []string{"/nix"} },
		BuildArgs: func(_ string, _ int, dry bool) []string {
			if dry {
				return []string{"--dry-run"}
			}
			return nil
		},
		Required:     []string{"--dry-run"},
		VersionWords: []string{"nix"},
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
	roots, err := e.resolveAndCheckRoots(c, profile, identity)
	if err != nil {
		reason := "boundary_violation"
		if errors.Is(err, errKeepListIntersection) {
			reason = string(model.SkipReasonKeepListIntersection)
		} else if errors.Is(err, errRootUnavailable) {
			reason = "root_unavailable"
		}
		return e.finishToolSkip(emit, c, start, reason, err.Error())
	}
	if profile.Busy != nil {
		for _, root := range roots {
			if profile.Busy(root) {
				return e.finishToolSkip(emit, c, start, "cache_active", "cache lock or open descriptor indicates activity")
			}
		}
	}
	if dryRun && !profile.NativeDryRun {
		return e.finishToolSkipIncomplete(emit, c, start, model.SkipReasonDryRunUnsupported, "profile has no native dry-run")
	}
	searchPath := e.searchPath(profile, identity.Home)
	binary, err := resolveProfileBinary(profile.Binary, searchPath)
	if err != nil {
		return e.finishToolSkip(emit, c, start, string(model.SkipReasonManagerNotInstalled), err.Error())
	}
	env := e.environment(identity, profile, searchPath)
	commandIdentity := e.executionIdentity(identity)
	if err := e.probe(ctx, binary, profile, identity, commandIdentity, env); err != nil {
		reason := "capability_unsupported"
		if errors.Is(err, errVersionMismatch) {
			reason = "version_mismatch"
		}
		return e.finishToolSkip(emit, c, start, reason, err.Error())
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
		identity := e.Target
		if os.Geteuid() != 0 && identity.UID != uint32(os.Getuid()) {
			return CommandIdentity{}, errors.New("non-root process cannot target another UID")
		}
		if identity.Home == "" {
			identity.Home = e.Home
		}
		if identity.Name == "" {
			if targetUser, lookupErr := user.LookupId(strconv.FormatUint(uint64(identity.UID), 10)); lookupErr == nil {
				identity.Name = targetUser.Username
			}
		}
		if err := verifyTargetHome(identity); err != nil {
			return CommandIdentity{}, err
		}
		return normalizeIdentity(identity), nil
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
	return identity, nil
}

func lookupIdentity(uid uint32, expectedName, home string) (CommandIdentity, error) {
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
	groups, err := u.GroupIds()
	if err != nil {
		return CommandIdentity{}, err
	}
	groupIDs := make([]uint32, 0, len(groups))
	for _, raw := range groups {
		gid, parseErr := strconv.ParseUint(raw, 10, 32)
		if parseErr != nil || uint32(gid) == uint32(gid64) {
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
		if gid == identity.GID || seen[gid] {
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
	roots := profile.RootResolver(identity.Home)
	if len(roots) == 0 {
		return nil, errRootUnavailable
	}
	checked := make([]string, 0, len(roots))
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
		checked = append(checked, root)
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

func (e *ToolPruneExecutor) searchPath(profile toolProfile, home string) []string {
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
		"/home/linuxbrew/.linuxbrew/bin",
		"/opt/linuxbrew/bin",
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
	for _, dir := range searchPath {
		candidate := filepath.Join(dir, name)
		info, err := os.Stat(candidate)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		return abs, nil
	}
	return "", fmt.Errorf("profile binary %q not found", name)
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

func (e *ToolPruneExecutor) probe(ctx context.Context, binary string, profile toolProfile, identity CommandIdentity, commandIdentity *CommandIdentity, env []string) error {
	args := profile.ProbeArgs
	if len(args) == 0 {
		args = []string{"--version"}
	}
	var lines []string
	err := RunCommand(ctx, CommandRun{
		Executable: binary,
		Args:       args,
		Env:        env,
		Cwd:        identity.Home,
		Identity:   commandIdentity,
		Timeout:    e.probeTimeout(),
		OnStdout:   func(line string) { lines = append(lines, line) },
		OnStderr:   func(line string) { lines = append(lines, line) },
	})
	if err != nil {
		return fmt.Errorf("capability probe failed: %w", err)
	}
	if len(profile.VersionWords) > 0 && len(lines) > 0 {
		joined := strings.ToLower(strings.Join(lines, "\n"))
		matched := false
		for _, word := range profile.VersionWords {
			if strings.Contains(joined, strings.ToLower(word)) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%w: %s", errVersionMismatch, strings.TrimSpace(joined))
		}
	}
	if len(profile.Required) > 0 {
		joined := strings.Join(lines, "\n")
		for _, flag := range profile.Required {
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

func huggingFaceRootBusy(root string) bool {
	busy := false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry == nil {
			return nil
		}
		if entry.IsDir() && (entry.Name() == ".locks" || entry.Name() == "locks") {
			children, readErr := os.ReadDir(path)
			if readErr == nil && len(children) > 0 {
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
	})
	if busy {
		return true
	}
	return openDescriptorUnder(root)
}

func openDescriptorUnder(root string) bool {
	root = filepath.Clean(root)
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, process := range entries {
		if !process.IsDir() || process.Name() == "" || process.Name()[0] < '0' || process.Name()[0] > '9' {
			continue
		}
		fds, err := os.ReadDir(filepath.Join("/proc", process.Name(), "fd"))
		if err != nil {
			continue
		}
		for _, fd := range fds {
			link, err := os.Readlink(filepath.Join("/proc", process.Name(), "fd", fd.Name()))
			if err != nil || !pathWithin(filepath.Clean(link), root) {
				continue
			}
			return true
		}
	}
	return false
}
