package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/dengqi/beav/internal/cleaner/safety"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/oplog"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui/tui"
	"github.com/spf13/cobra"
)

// NewTUICmd creates the explicit `beav tui` subcommand to launch the interactive workbench.
// NewTUICmd 创建显式的 `beav tui` 子命令，用于启动多视图交互工作台。
func NewTUICmd(version string, tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	var f CleanFlags
	var tabName string

	cmd := &cobra.Command{
		Use:     "tui",
		GroupID: "core",
		Short:   tr.Text("Launch the interactive multi-view TUI workbench", "启动交互式多视图 TUI 工作台"),
		Example: "  beav tui\n  beav tui --tab analyze\n  beav tui --unsafe\n  sudo beav tui --system",
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			applyGlobalFlags(cmd, &f)
			if !isInteractiveTerminal(cmd.OutOrStdout()) {
				return ExitError{
					code: 1,
					err:  fmt.Errorf("%s", localTr.Text("tui requires an interactive terminal (TTY)", "tui 需要交互式终端（TTY）")),
				}
			}
			return launchTUI(cmd.Context(), version, f, tabName, localTr)
		},
	}
	cmd.Flags().BoolVar(&f.System, "system", false, tr.Text("launch TUI for system-scope caches (requires sudo)", "为系统级清理项启动交互界面（需 sudo）"))
	cmd.Flags().BoolVar(&f.All, "all", false, tr.Text("launch TUI for both user and system scopes (requires sudo)", "同时为用户级与系统级清理项启动交互界面（需 sudo）"))
	cmd.Flags().StringVar(&tabName, "tab", "clean", tr.Text("initial tab: clean, analyze, history, or config", "初始标签页：clean、analyze、history 或 config"))
	addScopeAndPolicyFlags(cmd, &f, tr)
	return cmd
}

func parseInitialTab(tabName string) int {
	switch tabName {
	case "analyze":
		return 1
	case "history":
		return 2
	case "config", "safety":
		return 3
	default:
		return 0
	}
}

func launchTUI(ctx context.Context, version string, f CleanFlags, tabName string, tr *i18n.Translator) error {
	lc, err := loadCleanContext(f, tr)
	if err != nil {
		return err
	}
	logPath := operationLogPath(lc.Home, lc.TargetUID)
	if logPath == "" {
		home, _ := os.UserHomeDir()
		logPath = operationLogPath(home, os.Geteuid())
	}

	opts := tui.Options{
		Translator: tr,
		Version:    version,
		ConfigDir:  lc.CfgDir,
		Config:     lc.Cfg,
		Policy:     lc.Policy,
		LogPath:    logPath,
		InitialTab: parseInitialTab(tabName),
		Scope: tui.ScopeInfo{
			Scope:  lc.Scope,
			Home:   lc.Home,
			IsRoot: os.Getuid() == 0,
		},
		Hooks: tui.Hooks{
			StartScan: func(runCtx context.Context) (<-chan tui.ScanProgress, context.CancelFunc) {
				return runReadOnlyScanWithPolicy(runCtx, lc.Merged, lc.Scope, lc.Home, lc.TargetUID, lc.Cfg, lc.Policy)
			},
			StartExec: func(runCtx context.Context, selectedIDs []string) (<-chan tui.ExecProgress, context.CancelFunc) {
				return runTUIExecWithPolicy(runCtx, lc.Merged, lc.Scope, lc.Home, lc.TargetUID, lc.Cfg, lc.Policy, f, selectedIDs)
			},
			StartScanWithOptions: func(runCtx context.Context, policy safety.Policy, minAgeOverride string) (<-chan tui.ScanProgress, context.CancelFunc) {
				runFlags := f
				if minAgeOverride != "" {
					runFlags.MinAge = minAgeOverride
				}
				if policy.BypassAll {
					runFlags.ForceNoAge = true
				}
				eff, effErr := applyEffectiveConfig(lc.Merged, lc.Cfg, runFlags, tr)
				if effErr != nil {
					eff = lc.Merged
				}
				return runReadOnlyScanWithPolicy(runCtx, eff, lc.Scope, lc.Home, lc.TargetUID, lc.Cfg, policy)
			},
			StartExecWithOptions: func(runCtx context.Context, policy safety.Policy, minAgeOverride string, selectedIDs []string) (<-chan tui.ExecProgress, context.CancelFunc) {
				runFlags := f
				if minAgeOverride != "" {
					runFlags.MinAge = minAgeOverride
				}
				if policy.BypassAll {
					runFlags.ForceNoAge = true
				}
				eff, effErr := applyEffectiveConfig(lc.Merged, lc.Cfg, runFlags, tr)
				if effErr != nil {
					eff = lc.Merged
				}
				return runTUIExecWithPolicy(runCtx, eff, lc.Scope, lc.Home, lc.TargetUID, lc.Cfg, policy, runFlags, selectedIDs)
			},
			SaveConfig: func(cfg *config.Config) error {
				if cfg == nil {
					return nil
				}
				return cfg.Save()
			},
			ReadHistory: func() ([]oplog.Entry, error) {
				if logPath == "" {
					return nil, nil
				}
				return oplog.ReadEntries(logPath, true, 5)
			},
			ClearHistory: func() error {
				if logPath == "" {
					return nil
				}
				return oplog.Clear(logPath, 5)
			},
		},
	}
	_ = sysinfo.IsTerminal
	return tui.Run(ctx, opts)
}
