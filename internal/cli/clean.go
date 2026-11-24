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

	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/executor"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/oplog"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui"
	uicli "github.com/dengqi/beav/internal/ui/cli"
	uijson "github.com/dengqi/beav/internal/ui/json"
	"github.com/spf13/cobra"
)

// NewCleanCmd creates a command that cleans caches, logs, and other reclaimable disk usage.
// NewCleanCmd 创建一个清理缓存、日志和其他可回收磁盘使用的命令。
func NewCleanCmd(tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	var f CleanFlags
	cmd := &cobra.Command{
		Use:     "clean",
		GroupID: "core",
		Short:   tr.Text("Clean caches, logs, and other reclaimable disk usage", "清理缓存、日志及其他可释放磁盘空间"),
		Long: tr.Text(
			"Clean caches, logs, and build artifacts safely.\n"+
				"In an interactive terminal, Beav previews the cleanup plan first and asks before deleting unless --yes is passed.\n"+
				"Heavy cleaners require --include-heavy or an exact full cleaner ID/tag in --only. Run `beav list` or `beav info <id>` to inspect cleaners.",
			"安全清理缓存、日志与可再生构建产物。\n"+
				"在交互终端中默认先执行只读扫描并等待确认后才删除（除非传入 --yes）。\n"+
				"重度清理项需要 --include-heavy 或在 --only 中指定完整 ID 或标签；运行 `beav list` 或 `beav info <ID>` 可查看全部清理项。",
		),
		Example: tr.Text(
			"  beav clean --dry-run\n"+
				"  beav clean --only langs,browser\n"+
				"  beav clean --only lang-rustup --yes\n"+
				"  sudo beav clean --system\n"+
				"  sudo beav clean --all",
			"  beav clean --dry-run\n"+
				"  beav clean --only langs,browser\n"+
				"  beav clean --only lang-rustup --yes\n"+
				"  sudo beav clean --system\n"+
				"  sudo beav clean --all",
		),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			applyGlobalFlags(cmd, &f)
			return runCleanWithTranslator(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), f, localTr)
		},
	}
	cmd.Flags().BoolVar(&f.System, "system", false, tr.Text("clean system-scope cleaners (requires sudo)", "清理系统级清理项（需 sudo）"))
	cmd.Flags().BoolVar(&f.All, "all", false, tr.Text("clean both user and system scopes (requires sudo)", "同时清理用户级与系统级清理项（需 sudo）"))
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, tr.Text("preview what would be removed without deleting", "仅预览可清理内容而不执行删除"))
	cmd.Flags().StringSliceVar(&f.Only, "only", nil, tr.Text("run matching cleaner IDs, tags, or ID prefixes; heavy requires an exact full ID or tag", "仅运行匹配的清理项 ID、标签或 ID 前缀；重度项需指定完整 ID 或标签"))
	cmd.Flags().BoolVar(&f.IncludeHeavy, "include-heavy", false, tr.Text("explicitly authorize all heavy cleaners; an exact cleaner ID or tag in --only also authorizes one; --yes, --all, and prefixes do not", "启用全部重度清理项；在 --only 中指定精确 ID 或标签也可启用（--yes、--all 与前缀不会自动启用）"))
	cmd.Flags().StringSliceVar(&f.Skip, "skip", nil, tr.Text("skip matching cleaner IDs, tags, or ID prefixes", "跳过匹配的清理项 ID、标签或 ID 前缀"))
	cmd.Flags().BoolVar(&f.ForceNoAge, "force-no-age", false, tr.Text("allow cleaners that explicitly have no age filter", "允许执行未设置保留天数过滤的清理项"))
	cmd.Flags().StringVar(&f.Output, "output", "", tr.Text("output mode: spinner, plain, or json", "输出格式：spinner、plain 或 json"))
	cmd.Flags().BoolVarP(&f.Yes, "yes", "y", false, tr.Text("run deletion without the confirmation prompt; does not authorize heavy cleaners", "跳过交互确认直接执行；不会自动启用重度清理项"))
	cmd.Flags().BoolVarP(&f.Verbose, "verbose", "v", false, tr.Text("show file samples, command output, and all skipped cleaners", "显示文件样本、原生命令输出与全部跳过项明细"))
	addScopeAndPolicyFlags(cmd, &f, tr)
	return cmd
}

func runCleanWithTranslator(ctx context.Context, stdout, stderr io.Writer, f CleanFlags, tr *i18n.Translator) error {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	lc, err := loadCleanContext(f, tr)
	if err != nil {
		return err
	}
	warnings, diagErr := diagnoseSelectors(lc.Merged, lc.Scope, f.Only, tr)
	if diagErr != nil {
		return diagErr
	}

	mode := outputMode(f.Output, lc.Cfg.Defaults.Output)
	if mode != "json" && !f.Quiet {
		for _, w := range warnings {
			fmt.Fprintf(stderr, "%s\n", tr.Textf("Warning: %s", "警告：%s", w))
		}
		printRunHeader(stdout, lc.Scope, lc.Home, f.DryRun, confirmNeeded(f, lc.Cfg, stdout), lc.Policy.BypassAll, tr)
	}

	if confirmNeeded(f, lc.Cfg, stdout) {
		res, info, err := runCleanPassWithOptions(ctx, lc.Merged, lc.Scope, lc.Home, lc.TargetUID, lc.Cfg, lc.Policy, f, true, stdout, tr)
		if err != nil {
			return passExitError(err)
		}
		if shouldAbortEarly(res, info) {
			if !f.Quiet {
				fmt.Fprintln(stdout, "\n"+tr.Text("Nothing to delete.", "没有发现可清理的缓存。"))
			}
			return nil
		}
		info.unmeasured = filterOutNative(res.UnmeasuredCleaners, info.native)
		if !confirmDeletionLocalized(os.Stdin, stdout, res, info, tr) {
			fmt.Fprintln(stdout, "\n"+tr.Text("Canceled. Nothing deleted.", "已取消，未删除任何文件。"))
			return nil
		}
		fmt.Fprintf(stdout, "\n%s\n", tr.Text("Cleaning...", "开始执行清理..."))
	}
	res, _, err := runCleanPassWithOptions(ctx, lc.Merged, lc.Scope, lc.Home, lc.TargetUID, lc.Cfg, lc.Policy, f, false, stdout, tr)
	if err != nil {
		return passExitError(err)
	}
	if res.CleanersErrored > 0 {
		return ExitError{
			code: 3,
			err:  fmt.Errorf("%s", tr.Textf("errors in %d cleaners", "有 %d 个清理项执行出错", res.CleanersErrored)),
		}
	}
	return nil
}

func printRunHeader(w io.Writer, scope model.Scope, home string, dryRun, previewBeforeConfirm, bypassAll bool, tr *i18n.Translator) {
	phase := tr.Text("Clean", "执行清理")
	if dryRun {
		phase = tr.Text("Preview (read-only)", "只读扫描（不会删除文件）")
	} else if previewBeforeConfirm {
		phase = tr.Text("Preview before clean", "清理前预览")
	}
	scopeText := tr.ScopeLabel(scope)
	if home != "" && scope != model.ScopeSystem {
		scopeText += " (" + home + ")"
	}
	if bypassAll {
		scopeText += tr.Text(" [UNSAFE BYPASS]", " [已绕过安全限制]")
	}
	fmt.Fprint(w, tr.Textf("%s | scope: %s\n", "%s | 作用域：%s\n", phase, scopeText))
}

func shouldAbortEarly(res engine.Result, info passInfo) bool {
	return res.FilesCandidate == 0 && len(info.native) == 0 && !res.MetricsIncomplete
}

func filterOutNative(unmeasured, native []string) []string {
	if len(unmeasured) == 0 {
		return nil
	}
	nativeSet := make(map[string]struct{}, len(native))
	for _, id := range native {
		nativeSet[id] = struct{}{}
	}
	out := make([]string, 0, len(unmeasured))
	for _, id := range unmeasured {
		if _, ok := nativeSet[id]; !ok {
			out = append(out, id)
		}
	}
	return out
}

// passInfo records what a preview pass learned about the upcoming real run.
// passInfo 记录预演对随后真实运行的了解。
type passInfo struct {
	native     []string // cleaners that refuse dry-run preview
	unmeasured []string // native cleaners that previewed without exact byte totals
}

func runCleanPassWithOptions(ctx context.Context, merged []model.Cleaner, scope model.Scope, home string, targetUID int, cfg *config.Config, policy safety.Policy, f CleanFlags, preview bool, stdout io.Writer, tr *i18n.Translator) (engine.Result, passInfo, error) {
	renderer := chooseRendererWithOptions(f, cfg.Defaults.Output, stdout, tr)
	defer func() { _ = renderer.Close() }()
	return runCleanPassWithSink(ctx, merged, scope, home, targetUID, cfg, policy, f, preview, renderer.Render)
}

// runCleanPassWithSink wires executors, emits events to sink, and runs one engine pass.
// Dryness is always derived here from CleanFlags and preview so no caller can pass a literal dry value.
// runCleanPassWithSink 接线执行器、将事件发送给 sink 并运行一次引擎处理；
// dry 在此严格从 CleanFlags 与 preview 派生，调用方无法传入字面量 dry 值。
func runCleanPassWithSink(ctx context.Context, merged []model.Cleaner, scope model.Scope, home string, targetUID int, cfg *config.Config, policy safety.Policy, f CleanFlags, preview bool, sink func(model.Event)) (engine.Result, passInfo, error) {
	dry := preview || f.DryRun
	info := passInfo{}

	var log *oplog.Logger
	if !dry {
		if p := operationLogPath(home, targetUID); p != "" {
			if l, err := oplog.New(p, 10*1024*1024, 5); err == nil {
				log = l
				defer func() { _ = log.Close() }()
			}
		}
	}

	eng := newCleanEngineWithPolicy(home, targetUID, safety.NewWhitelist(cfg.MergedWhitelist()), policy)
	emit := func(ev model.Event) {
		if log != nil && ev.Event == model.EvDeleted {
			_ = log.Write("delete", ev.Path, ev.Size, ev.CleanerID)
		}
		if dry && ev.Event == model.EvCleanerSkipped && ev.Reason == string(model.SkipReasonDryRunUnsupported) {
			info.native = append(info.native, ev.CleanerID)
		}
		if sink != nil {
			sink(ev)
		}
	}
	res, err := eng.Run(ctx, merged, engine.Options{
		Scope:        scope,
		DryRun:       dry,
		Only:         f.Only,
		Skip:         f.Skip,
		IncludeHeavy: f.IncludeHeavy || policy.BypassAll,
		BypassAll:    policy.BypassAll,
		Emitter:      emit,
	})
	return res, info, err
}

func operationLogPath(home string, targetUID int) string {
	if os.Getenv("BEAV_NO_OPLOG") != "" || !shouldWriteOperationLog(targetUID) {
		return ""
	}
	logHome := home
	if logHome == "" {
		logHome, _ = os.UserHomeDir()
	}
	if logHome == "" {
		return ""
	}
	return filepath.Join(logHome, ".local", "state", "beav", "operations.log")
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

func confirmDeletion(stdin io.Reader, stdout io.Writer, res engine.Result, info passInfo) bool {
	return confirmDeletionLocalized(stdin, stdout, res, info, i18n.New(i18n.LangEN))
}

func confirmDeletionLocalized(stdin io.Reader, stdout io.Writer, _ engine.Result, info passInfo, tr *i18n.Translator) bool {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	if len(info.native) > 0 {
		fmt.Fprintf(stdout, "%s\n", tr.Textf(
			"No preview available, these run their own cleanup command: %s.",
			"以下清理项不支持预览估算，将直接调用原生命令执行：%s。",
			strings.Join(info.native, ", "),
		))
	}
	if len(info.unmeasured) > 0 {
		fmt.Fprintf(stdout, "%s\n", tr.Textf(
			"Native commands with unmeasured byte totals: %s.",
			"以下原生命令在执行前无法估算释放空间：%s。",
			strings.Join(info.unmeasured, ", "),
		))
	}
	fmt.Fprint(stdout, tr.Text("Proceed with deletion? [y/N] ", "确认执行删除？[y/N] "))
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
	return newCleanEngineWithPolicy(home, targetUID, whitelist, safety.DefaultPolicy())
}

func newCleanEngineWithPolicy(home string, targetUID int, whitelist *safety.Whitelist, policy safety.Policy) *engine.Engine {
	uid := uint32(targetUID) //nolint:gosec // targetUID comes from the OS or the validated uint32 sudo resolver.
	toolOptions := []executor.ToolPruneOption{
		executor.WithToolIdentity(executor.CommandIdentity{UID: uid}),
		executor.WithToolPolicy(policy),
	}
	return engine.New(
		engine.WithExecutor(model.TypePaths, executor.NewPathsExecutorWithPolicy(home, whitelist, policy)),
		engine.WithExecutor(model.TypeCommand, executor.NewCustomCommandExecutor(home, policy)),
		engine.WithExecutor(model.TypeJournalVacuum, executor.NewJournalExecutor()),
		engine.WithExecutor(model.TypePkgCache, executor.NewPkgCacheExecutor()),
		engine.WithExecutor(model.TypeContainerPrune, executor.NewContainerExecutorWithPolicy(executor.ContainerTarget{UID: targetUID, Home: home}, policy)),
		engine.WithExecutor(model.TypeToolPrune, executor.NewToolPruneExecutor(home, whitelist, toolOptions...)),
		engine.WithExecutor(model.TypeDebris, executor.NewDebrisExecutorWithPolicy(home, whitelist, policy, uid)),
		engine.WithExecutor(model.TypeManagedCache, executor.NewManagedCacheExecutorWithPolicy(home, whitelist, policy, uid)),
		engine.WithExecutor(model.TypeRustupPrune, executor.NewRustupExecutorWithPolicy(home, whitelist, policy, uid)),
	)
}

// validateCleanersForRun validates cleaners for the given scope and flags before execution.
// validateCleanersForRun 在执行前验证给定范围和标志的清理器。
func validateCleanersForRun(cleaners []model.Cleaner, scope model.Scope, home string, f CleanFlags) error {
	return validateCleanersForRunWithPolicy(cleaners, scope, home, f, safety.DefaultPolicy())
}

func validateCleanersForRunWithPolicy(cleaners []model.Cleaner, scope model.Scope, home string, f CleanFlags, policy safety.Policy) error {
	for _, c := range cleaners {
		if err := registry.ValidateWithPolicy(c, policy); err != nil {
			return err
		}
		if !engine.Selected(c, scope, f.Only, f.Skip) {
			continue
		}
		validateHome := home
		if c.Scope == model.ScopeSystem {
			validateHome = ""
		}
		if err := registry.ValidatePathsWithPolicy(c, validateHome, policy); err != nil {
			return err
		}
	}
	return nil
}

// applyEffectiveConfig applies configuration overrides and age settings to cleaners.
// applyEffectiveConfig 将配置覆盖和保留期设置应用到清理项。
func applyEffectiveConfig(cleaners []model.Cleaner, cfg *config.Config, f CleanFlags, tr *i18n.Translator) ([]model.Cleaner, error) {
	globalAge, perTagAge, err := parseMinAge(f.MinAge, tr)
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
// parseMinAge 将 --min-age 标志解析为全局和每标签的保留天数。
func parseMinAge(raw string, tr *i18n.Translator) (int, map[string]int, error) {
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
			days, err := parseDays(val, tr)
			if err != nil {
				return 0, nil, err
			}
			perTag[key] = days
			continue
		}
		days, err := parseDays(part, tr)
		if err != nil {
			return 0, nil, err
		}
		global = days
	}
	return global, perTag, nil
}

// parseDays parses a day string (e.g., "14d") into an integer.
// parseDays 将天数字符串（如 "14d"）解析为整数。
func parseDays(raw string, tr *i18n.Translator) (int, error) {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	stripped := strings.TrimSuffix(strings.TrimSpace(raw), "d")
	v, err := strconv.Atoi(stripped)
	if err != nil || v < 0 {
		return 0, fmt.Errorf("%s", tr.Textf("invalid age %q (example: 14d or langs=3d)", "无效的保留天数 %q（示例：14d 或 langs=3d）", raw))
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

func chooseRenderer(flag, cfgDefault string, stdout io.Writer) ui.Renderer {
	return chooseRendererWithOptions(CleanFlags{Output: flag}, cfgDefault, stdout, i18n.New(i18n.LangEN))
}

func chooseRendererWithOptions(f CleanFlags, cfgDefault string, stdout io.Writer, tr *i18n.Translator) ui.Renderer {
	if f.Quiet {
		return uicli.NewQuietRenderer(stdout, tr)
	}
	isTTY := false
	if file, ok := stdout.(*os.File); ok && sysinfo.IsTerminal(file) {
		isTTY = true
	}
	noColor := f.NoColor || os.Getenv("NO_COLOR") != ""
	mode := outputMode(f.Output, cfgDefault)
	if noColor && (mode == "" || mode == "auto" || mode == "spinner") {
		mode = "plain"
	}
	switch mode {
	case "json":
		return uijson.New(stdout)
	case "plain":
		return uicli.NewPlainWithOptions(stdout, uicli.Options{Translator: tr, Verbose: f.Verbose})
	case "spinner":
		return uicli.NewSpinnerWithOptions(stdout, uicli.Options{Translator: tr, Verbose: f.Verbose, Animate: isTTY})
	default:
		if isTTY {
			return uicli.NewSpinnerWithOptions(stdout, uicli.Options{Translator: tr, Verbose: f.Verbose, Animate: true})
		}
		return uicli.NewPlainWithOptions(stdout, uicli.Options{Translator: tr, Verbose: f.Verbose})
	}
}

// determineScope determines the cleaning scope, home directory, and target UID from flags.
// determineScope 从标志确定清理范围、主目录和目标 UID。
func determineScope(f CleanFlags, tr *i18n.Translator) (model.Scope, string, int, error) {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	if f.System && f.All {
		return "", "", 0, errors.New(tr.Text("--system and --all are mutually exclusive", "--system 与 --all 不能同时使用"))
	}
	if !f.System && !f.All {
		if os.Getuid() == 0 && !f.AllowRootHome {
			return "", "", 0, errors.New(tr.Text(
				"running as root without --system/--all; pass --allow-root-home to clean /root",
				"当前以 root 身份运行但未指定 --system 或 --all；若需要处理 /root，请加上 --allow-root-home",
			))
		}
		home, _ := os.UserHomeDir()
		return model.ScopeUser, home, os.Getuid(), nil
	}
	if os.Getuid() != 0 {
		return "", "", 0, errors.New(tr.Text("--system and --all require root; run with sudo", "--system 和 --all 需要管理员权限，请使用 sudo 运行"))
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
		return "", "", 0, fmt.Errorf("%s: %w", tr.Text("--all home resolution failed", "--all 解析用户主目录失败"), err)
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
