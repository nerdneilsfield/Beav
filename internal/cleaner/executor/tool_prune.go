package executor

import (
	"context"
	"errors"
	"path/filepath"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

// ToolPruneTarget describes the account used for a user-scope tool.
// ToolPruneTarget 描述用户级工具使用的目标账户。
type ToolPruneTarget = CommandIdentity

// ToolPruneOption configures a ToolPruneExecutor.
// ToolPruneOption 配置 ToolPruneExecutor。
type ToolPruneOption func(*ToolPruneExecutor)

type toolIdentityResolver func(uid uint32) (CommandIdentity, error)

// withToolIdentityResolverForTest is intentionally package-private. It is the
// only seam allowing temporary fixture homes; production constructors cannot
// select alternate HOME values through public options or configuration.
func withToolIdentityResolverForTest(resolver toolIdentityResolver) ToolPruneOption {
	return func(e *ToolPruneExecutor) {
		e.identityResolver = resolver
	}
}

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
	// procRoot is fixed to /proc in production; package tests use a synthetic
	// tree to exercise fail-closed descriptor inspection deterministically.
	procRoot         string
	identityResolver toolIdentityResolver
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
		procRoot:  "/proc",
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
			busy, busyErr := profile.Busy(root, e.procRoot)
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
