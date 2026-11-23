// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/executor"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/oplog"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui"
	uicli "github.com/dengqi/beav/internal/ui/cli"
	uijson "github.com/dengqi/beav/internal/ui/json"
	"github.com/spf13/cobra"
)

// NewCleanCmd creates a command that cleans caches, logs, and other reclaimable disk usage.
// NewCleanCmd 创建一个清理缓存、日志和其他可回收磁盘使用的命令。
func NewCleanCmd() *cobra.Command {
	var f CleanFlags
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Clean caches, logs, and other reclaimable disk usage",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runClean(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}
	cmd.Flags().BoolVar(&f.System, "system", false, "clean system-scope cleaners")
	cmd.Flags().BoolVar(&f.All, "all", false, "clean both user and system scopes")
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, "show what would be removed")
	cmd.Flags().StringSliceVar(&f.Only, "only", nil, "run matching cleaner IDs, tags, or ID prefixes; heavy requires an exact full ID or tag")
	cmd.Flags().BoolVar(&f.IncludeHeavy, "include-heavy", false, "explicitly authorize all heavy cleaners; an exact cleaner ID or tag in --only also authorizes one; --yes, --all, and prefixes do not")
	cmd.Flags().StringSliceVar(&f.Skip, "skip", nil, "skip matching cleaner IDs, tags, or ID prefixes")
	cmd.Flags().StringVar(&f.MinAge, "min-age", "", "override min age, for example 14d or tag=3d")
	cmd.Flags().BoolVar(&f.ForceNoAge, "force-no-age", false, "allow cleaners that explicitly have no age filter")
	cmd.Flags().StringVar(&f.Output, "output", "", "output mode: spinner, plain, or json")
	cmd.Flags().BoolVar(&f.Yes, "yes", false, "run deletion without the confirmation prompt; does not authorize heavy cleaners")
	cmd.Flags().BoolVar(&f.AllowRootHome, "allow-root-home", false, "allow cleaning /root")
	cmd.Flags().StringVar(&f.UserOverride, "user", "", "target user for --all")
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", "", "override config directory")
	cmd.Flags().BoolVar(&f.BuiltinDisabled, "builtin-disabled", false, "skip embedded cleaners")
	_ = cmd.Flags().MarkHidden("builtin-disabled")
	return cmd
}

// runClean executes the clean operation with the given flags.
// runClean 使用给定标志执行清理操作。
func runClean(ctx context.Context, stdout, stderr io.Writer, f CleanFlags) error {
	scope, home, targetUID, err := determineScope(f)
	if err != nil {
		return ExitError{code: 1, err: err}
	}

	cfgDir := f.ConfigDir
	if cfgDir == "" {
		configHome := home
		if scope == model.ScopeSystem || configHome == "" {
			configHome, _ = os.UserHomeDir()
		}
		cfgDir = filepath.Join(configHome, ".config", "beav")
	}
	cfg, err := config.LoadWithHome(cfgDir, home)
	if err != nil {
		return ExitError{code: 2, err: fmt.Errorf("config load: %w", err)}
	}

	var builtinList []registry.Loaded
	if !f.BuiltinDisabled {
		builtinList, err = registry.LoadBuiltin(cleaners.Builtin)
		if err != nil {
			return ExitError{code: 2, err: fmt.Errorf("builtin load: %w", err)}
		}
	}
	userList, err := registry.LoadUserDir(filepath.Join(cfgDir, "cleaners.d"))
	if err != nil {
		return ExitError{code: 2, err: fmt.Errorf("user cleaners: %w", err)}
	}
	merged, err := registry.MergeByID(builtinList, userList)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	merged, err = applyEffectiveConfig(merged, cfg, f)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	if err := validateCleanersForRun(merged, scope, home, f); err != nil {
		return ExitError{code: 2, err: err}
	}

	if confirmNeeded(f, cfg, stdout) {
		res, info, err := runCleanPass(ctx, merged, scope, home, targetUID, cfg, f, true, stdout)
		if err != nil {
			return passExitError(err)
		}
		if res.FilesCandidate == 0 && len(info.native) == 0 {
			fmt.Fprintln(stdout, "\nNothing to delete.")
			return nil
		}
		if !confirmDeletion(os.Stdin, stdout, res, info) {
			fmt.Fprintln(stdout, "\nCanceled. Nothing deleted.")
			return nil
		}
	}
	res, _, err := runCleanPass(ctx, merged, scope, home, targetUID, cfg, f, f.DryRun, stdout)
	if err != nil {
		return passExitError(err)
	}
	if res.CleanersErrored > 0 {
		return ExitError{code: 3, err: fmt.Errorf("errors in %d cleaners", res.CleanersErrored)}
	}
	_ = stderr
	return nil
}

// passInfo records what a preview pass learned about the upcoming real run.
// passInfo 记录预演对随后真实运行的了解。
type passInfo struct {
	native []string // cleaners that will run native commands and cannot be previewed
}

// runCleanPass wires executors, renders events, and runs one engine pass.
// runCleanPass 接线执行器、渲染事件并运行一次引擎处理。
func runCleanPass(ctx context.Context, merged []model.Cleaner, scope model.Scope, home string, targetUID int, cfg *config.Config, f CleanFlags, dry bool, stdout io.Writer) (engine.Result, passInfo, error) {
	info := passInfo{}
	renderer := chooseRenderer(f.Output, cfg.Defaults.Output, stdout)
	defer func() { _ = renderer.Close() }()

	var log *oplog.Logger
	if !dry && os.Getenv("BEAV_NO_OPLOG") == "" && shouldWriteOperationLog(targetUID) {
		logHome := home
		if logHome == "" {
			logHome, _ = os.UserHomeDir()
		}
		if logHome != "" {
			stateDir := filepath.Join(logHome, ".local", "state", "beav")
			if l, err := oplog.New(filepath.Join(stateDir, "operations.log"), 10*1024*1024, 5); err == nil {
				log = l
				defer func() { _ = log.Close() }()
			}
		}
	}

	eng := newCleanEngine(home, targetUID, safety.NewWhitelist(cfg.MergedWhitelist()))
	emit := func(ev model.Event) {
		if log != nil && ev.Event == model.EvDeleted {
			_ = log.Write("delete", ev.Path, ev.Size, ev.CleanerID)
		}
		if dry && ev.Event == model.EvCleanerSkipped && ev.Reason == string(model.SkipReasonDryRunUnsupported) {
			info.native = append(info.native, ev.CleanerID)
		}
		renderer.Render(ev)
	}
	res, err := eng.Run(ctx, merged, engine.Options{
		Scope:        scope,
		DryRun:       dry,
		Only:         f.Only,
		Skip:         f.Skip,
		IncludeHeavy: f.IncludeHeavy,
		Emitter:      emit,
	})
	return res, info, err
}

// confirmNeeded reports whether an interactive terminal user should see a
// preview and a deletion prompt before the real pass runs.
// confirmNeeded 判断交互终端用户在真实运行前是否应看到预演与删除确认。
func confirmNeeded(f CleanFlags, cfg *config.Config, stdout io.Writer) bool {
	if f.DryRun || f.Yes {
		return false
	}
	if outputMode(f.Output, cfg.Defaults.Output) == "json" {
		return false
	}
	out, ok := stdout.(*os.File)
	if !ok || !sysinfo.IsTerminal(out) {
		return false
	}
	return sysinfo.IsTerminal(os.Stdin)
}

// confirmDeletion asks the user to approve the deletion after the preview pass
// has printed its plan summary. It returns true only on an explicit yes.
// confirmDeletion 在预演 pass 打印计划后请求用户批准删除。只有明确回答 yes 才返回 true。
func confirmDeletion(stdin io.Reader, stdout io.Writer, res engine.Result, info passInfo) bool {
	if len(info.native) > 0 {
		fmt.Fprintf(stdout, "No preview available, these run their own cleanup command: %s.\n", strings.Join(info.native, ", "))
	}
	fmt.Fprint(stdout, "Proceed with deletion? [y/N] ")
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintln(stdout)
		return false
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// passExitError maps an engine error to the process exit code.
// passExitError 将引擎错误映射为进程退出码。
func passExitError(err error) error {
	if errors.Is(err, context.Canceled) {
		return ExitError{code: 4, err: err}
	}
	return ExitError{code: 3, err: err}
}

func shouldWriteOperationLog(targetUID int) bool {
	return targetUID == os.Geteuid()
}

// newCleanEngine wires all built-in executor types and binds target account
// identity for profile-backed cleaners. Closed executors never receive YAML
// command, root, or identity data.
func newCleanEngine(home string, targetUID int, whitelist *safety.Whitelist) *engine.Engine {
	uid := uint32(targetUID) //nolint:gosec // targetUID comes from the OS or the validated uint32 sudo resolver.
	toolOptions := []executor.ToolPruneOption{
		executor.WithToolIdentity(executor.CommandIdentity{UID: uid}),
	}
	return engine.New(
		engine.WithExecutor(model.TypePaths, executor.NewPathsExecutor(home, whitelist)),
		engine.WithExecutor(model.TypeJournalVacuum, executor.NewJournalExecutor()),
		engine.WithExecutor(model.TypePkgCache, executor.NewPkgCacheExecutor()),
		engine.WithExecutor(model.TypeContainerPrune, executor.NewContainerExecutor(executor.ContainerTarget{UID: targetUID, Home: home})),
		engine.WithExecutor(model.TypeToolPrune, executor.NewToolPruneExecutor(home, whitelist, toolOptions...)),
		engine.WithExecutor(model.TypeDebris, executor.NewDebrisExecutor(home, whitelist, uid)),
		engine.WithExecutor(model.TypeManagedCache, executor.NewManagedCacheExecutor(home, whitelist, uid)),
		engine.WithExecutor(model.TypeRustupPrune, executor.NewRustupExecutor(home, whitelist, uid)),
	)
}

// validateCleanersForRun validates cleaners for the given scope and flags before execution.
// validateCleanersForRun 在执行前验证给定范围和标志的清理器。
func validateCleanersForRun(cleaners []model.Cleaner, scope model.Scope, home string, f CleanFlags) error {
	for _, c := range cleaners {
		if err := registry.Validate(c); err != nil {
			return err
		}
		if !engine.Selected(c, scope, f.Only, f.Skip) {
			continue
		}
		validateHome := home
		if c.Scope == model.ScopeSystem {
			validateHome = ""
		}
		if err := registry.ValidatePaths(c, validateHome); err != nil {
			return err
		}
	}
	return nil
}

// applyEffectiveConfig applies configuration overrides and age settings to cleaners.
// applyEffectiveConfig 将配置覆盖和年龄设置应用到清理器。
func applyEffectiveConfig(cleaners []model.Cleaner, cfg *config.Config, f CleanFlags) ([]model.Cleaner, error) {
	globalAge, perTagAge, err := parseMinAge(f.MinAge)
	if err != nil {
		return nil, err
	}
	out := make([]model.Cleaner, 0, len(cleaners))
	for _, c := range cleaners {
		if override, ok := cfg.Overrides[c.ID]; ok {
			if override.Enabled != nil {
				enabled := *override.Enabled
				c.Enabled = &enabled
			}
		}
		if c.NoAgeFilter {
			if !f.ForceNoAge {
				enabled := false
				c.Enabled = &enabled
			}
			out = append(out, c)
			continue
		}

		age := -1
		for _, selector := range append([]string{c.ID}, c.Tags...) {
			if v, ok := perTagAge[selector]; ok {
				age = v
				break
			}
		}
		if age == -1 && globalAge >= 0 {
			age = globalAge
		}
		if age == -1 {
			if override, ok := cfg.Overrides[c.ID]; ok && override.MinAgeDays != nil {
				age = *override.MinAgeDays
			}
		}
		if age == -1 && c.MinAgeDays != nil {
			age = *c.MinAgeDays
		}
		if age == -1 {
			age = cfg.Defaults.MinAgeDays
		}
		c.MinAgeDays = &age
		out = append(out, c)
	}
	return out, nil
}

// parseMinAge parses the --min-age flag into global and per-tag age values.
// parseMinAge 将 --min-age 标志解析为全局和每标签的年龄值。
func parseMinAge(raw string) (int, map[string]int, error) {
	global := -1
	perTag := map[string]int{}
	if raw == "" {
		return global, perTag, nil
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if key, val, ok := strings.Cut(part, "="); ok {
			days, err := parseDays(val)
			if err != nil {
				return 0, nil, err
			}
			perTag[key] = days
			continue
		}
		days, err := parseDays(part)
		if err != nil {
			return 0, nil, err
		}
		global = days
	}
	return global, perTag, nil
}

// parseDays parses a day string (e.g., "14d") into an integer.
// parseDays 将天数字符串（如 "14d"）解析为整数。
func parseDays(raw string) (int, error) {
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "d")
	v, err := strconv.Atoi(raw)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("invalid age %q", raw)
	}
	return v, nil
}

// outputMode resolves the effective output mode from the flag and config default.
// outputMode 从标志与配置默认值解析有效的输出模式。
func outputMode(flag, cfgDefault string) string {
	if flag != "" {
		return flag
	}
	return cfgDefault
}

// chooseRenderer selects the appropriate output renderer based on flags and config.
// chooseRenderer 根据标志和配置选择相应的输出渲染器。
func chooseRenderer(flag, cfgDefault string, stdout io.Writer) ui.Renderer {
	switch mode := outputMode(flag, cfgDefault); mode {
	case "json":
		return uijson.New(stdout)
	case "plain":
		return uicli.NewPlain(stdout)
	case "spinner":
		return uicli.NewSpinner(stdout)
	default:
		if f, ok := stdout.(*os.File); ok && sysinfo.IsTerminal(f) {
			return uicli.NewSpinner(stdout)
		}
		return uicli.NewPlain(stdout)
	}
}

// determineScope determines the cleaning scope, home directory, and target UID from flags.
// determineScope 从标志确定清理范围、主目录和目标 UID。
func determineScope(f CleanFlags) (model.Scope, string, int, error) {
	if f.System && f.All {
		return "", "", 0, errors.New("--system and --all are mutually exclusive")
	}
	if !f.System && !f.All {
		if os.Getuid() == 0 && !f.AllowRootHome {
			return "", "", 0, errors.New("running as root without --system/--all; pass --allow-root-home to clean /root")
		}
		home, _ := os.UserHomeDir()
		return model.ScopeUser, home, os.Getuid(), nil
	}
	if os.Getuid() != 0 {
		return "", "", 0, errors.New("--system and --all require root; run with sudo")
	}
	if f.System {
		return model.ScopeSystem, "", os.Getuid(), nil
	}

	resolver := sysinfo.DefaultSudoUserResolver()
	env := sysinfo.EnvMap()
	if f.UserOverride != "" {
		uid, _, err := resolver.LookupByName(f.UserOverride)
		if err != nil {
			return "", "", 0, err
		}
		env["SUDO_USER"] = f.UserOverride
		env["SUDO_UID"] = strconv.FormatUint(uint64(uid), 10)
	}
	resolved, err := resolver.Resolve(env)
	if err != nil {
		return "", "", 0, fmt.Errorf("--all home resolution failed: %w", err)
	}
	return model.ScopeAll, resolved.Home, int(resolved.UID), nil
}

// ExitError wraps an error with the process exit code the CLI should return.
// ExitError 将错误与 CLI 应返回的进程退出代码包装在一起。
type ExitError struct {
	code int
	err  error
}

func (e ExitError) Error() string {
	return e.err.Error()
}

func (e ExitError) Code() int {
	return e.code
}

func (e ExitError) Unwrap() error {
	return e.err
}
