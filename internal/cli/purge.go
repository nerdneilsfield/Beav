package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/oplog"
	"github.com/spf13/cobra"
)

// NewPurgeCmd creates the `beav purge <path...>` command for direct path cleanup.
// NewPurgeCmd 创建 `beav purge <path...>` 命令，直接清理指定的路径或目录。
func NewPurgeCmd(tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	var f CleanFlags

	cmd := &cobra.Command{
		Use:     "purge <path...>",
		GroupID: "core",
		Short:   tr.Text("Directly clean one or more specified paths or globs", "直接清理一个或多个指定路径或通配符目标"),
		Long: tr.Text(
			"Clean arbitrary file or directory targets directly without creating a YAML cleaner.\n"+
				"Applies your active safety policy (allow-list, blacklist, whitelist, min-age) unless bypassed via --unsafe or policy flags.",
			"直接清理指定的目录或文件路径，无需预先编写 YAML 清理项。\n"+
				"默认仍遵循当前安全策略（允许列表、黑名单、白名单与留存天数），也可通过 --unsafe 或绕过参数解除限制。",
		),
		Example: "  beav purge ~/.cache/my-app --dry-run\n  beav purge /tmp/build-* --min-age=0d --yes\n  beav purge /opt/old-cache --unsafe --min-age=0d",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			applyGlobalFlags(cmd, &f)
			return runPurge(cmd, args, f, localTr)
		},
	}
	cmd.Flags().BoolVar(&f.DryRun, "dry-run", false, tr.Text("preview matching files without deleting", "仅预览可清理文件而不执行删除"))
	cmd.Flags().BoolVarP(&f.Yes, "yes", "y", false, tr.Text("skip confirmation prompt", "跳过确认提示直接删除"))
	cmd.Flags().BoolVarP(&f.Verbose, "verbose", "v", false, tr.Text("show matched file details", "显示匹配文件详情"))
	cmd.Flags().StringVar(&f.Output, "output", "", tr.Text("output mode: spinner, plain, or json", "输出格式：spinner、plain 或 json"))
	cmd.Flags().BoolVar(&f.ForceNoAge, "force-no-age", false, tr.Text("delete regardless of file age", "忽略文件修改时间过滤"))
	addScopeAndPolicyFlags(cmd, &f, tr)
	return cmd
}

func runPurge(cmd *cobra.Command, targets []string, f CleanFlags, tr *i18n.Translator) error {
	home, _ := os.UserHomeDir()
	cfgDir := resolveConfigDir(f.ConfigDir)
	cfg, err := config.LoadWithHome(cfgDir, home)
	if err != nil {
		return ExitError{code: 2, err: fmt.Errorf("%s: %w", tr.Text("config load", "加载配置失败"), err)}
	}
	policy := effectivePolicy(f, cfg)
	// When user explicitly specifies paths on CLI, allow them through allow-list unless replace_allow_list is active without allow_path.
	absTargets := make([]string, 0, len(targets))
	for _, t := range targets {
		exp := safety.ExpandPath(t, home)
		if !filepath.IsAbs(exp) {
			if abs, absErr := filepath.Abs(exp); absErr == nil {
				exp = abs
			}
		}
		if exp == "/" {
			return ExitError{code: 1, err: fmt.Errorf("%s", tr.Text("refusing to purge literal root '/'", "拒绝直接清理根目录 '/'"))}
		}
		absTargets = append(absTargets, exp)
	}
	// If user passed --bypass-allow-list or --unsafe, it's already disabled; otherwise keep policy check.
	// Allow explicit target paths if policy isn't locked down or user added them via --allow-path.
	minAge := 0
	noAge := f.ForceNoAge || f.MinAge == ""
	if f.MinAge != "" {
		globalAge, _, parseErr := parseMinAge(f.MinAge, tr)
		if parseErr != nil {
			return ExitError{code: 1, err: parseErr}
		}
		if globalAge >= 0 {
			minAge = globalAge
			noAge = false
		}
	}

	cleaner := model.Cleaner{
		ID:          "cli-purge",
		Name:        tr.Text("Direct Path Purge", "直接路径清理"),
		Scope:       model.ScopeUser,
		Type:        model.TypePaths,
		Impact:      model.ImpactSafe,
		MinAgeDays:  &minAge,
		NoAgeFilter: noAge,
		Paths:       absTargets,
	}

	// Check paths against policy before running.
	for _, p := range absTargets {
		base := p
		if !safety.InsideAllowListWithPolicy(base, home, policy) {
			return ExitError{code: 2, err: fmt.Errorf("%s", tr.Textf("path %q outside allow-list (pass --bypass-allow-list, --allow-path, or --unsafe)", "路径 %q 超出允许范围（可加 --bypass-allow-list、--allow-path 或 --unsafe）", p))}
		}
		if safety.BlacklistedWithPolicy(base, home, policy) {
			return ExitError{code: 2, err: fmt.Errorf("%s", tr.Textf("path %q is protected by blacklist (pass --bypass-blacklist or --unsafe)", "路径 %q 受黑名单保护（可加 --bypass-blacklist 或 --unsafe）", p))}
		}
	}

	stdout := cmd.OutOrStdout()
	mode := outputMode(f.Output, cfg.Defaults.Output)
	if mode != "json" && !f.Quiet {
		printRunHeader(stdout, model.ScopeUser, home, f.DryRun, confirmNeeded(f, cfg, stdout), policy.BypassAll, tr)
	}

	runPass := func(preview bool) (engine.Result, passInfo, error) {
		dry := preview || f.DryRun
		renderer := chooseRendererWithOptions(f, cfg.Defaults.Output, stdout, tr)
		defer func() { _ = renderer.Close() }()

		var log *oplog.Logger
		if !dry {
			if p := operationLogPath(home, os.Geteuid()); p != "" {
				if l, lErr := oplog.New(p, 10*1024*1024, 5); lErr == nil {
					log = l
					defer func() { _ = log.Close() }()
				}
			}
		}

		eng := newCleanEngineWithPolicy(home, os.Geteuid(), safety.NewWhitelist(cfg.MergedWhitelist()), policy)
		res, runErr := eng.Run(cmd.Context(), []model.Cleaner{cleaner}, engine.Options{
			Scope:        model.ScopeUser,
			DryRun:       dry,
			IncludeHeavy: true,
			BypassAll:    policy.BypassAll,
			Emitter: func(ev model.Event) {
				if log != nil && ev.Event == model.EvDeleted {
					_ = log.Write("delete", ev.Path, ev.Size, ev.CleanerID)
				}
				renderer.Render(ev)
			},
		})
		return res, passInfo{}, runErr
	}

	if confirmNeeded(f, cfg, stdout) {
		res, info, err := runPass(true)
		if err != nil {
			return passExitError(err)
		}
		if shouldAbortEarly(res, info) {
			if !f.Quiet {
				fmt.Fprintln(stdout, "\n"+tr.Text("Nothing to delete.", "没有发现可清理的文件。"))
			}
			return nil
		}
		if !confirmDeletionLocalized(os.Stdin, stdout, res, info, tr) {
			fmt.Fprintln(stdout, "\n"+tr.Text("Canceled. Nothing deleted.", "已取消，未删除任何文件。"))
			return nil
		}
		fmt.Fprintf(stdout, "\n%s\n", tr.Text("Cleaning...", "开始执行清理..."))
	}

	res, _, err := runPass(false)
	if err != nil {
		return passExitError(err)
	}
	if res.CleanersErrored > 0 {
		return ExitError{code: 3, err: fmt.Errorf("%s", tr.Text("purge encountered errors", "路径清理过程中出现错误"))}
	}
	return nil
}
