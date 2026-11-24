package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/spf13/cobra"
)

// NewRootCmd creates a new root CLI command using automatic locale detection.
// NewRootCmd 使用自动区域检测创建根 CLI 命令。
func NewRootCmd(version, commit, date string) *cobra.Command {
	return NewRootCmdWithLang(version, commit, date, i18n.Detect(""))
}

// NewRootCmdWithLang creates a new root CLI command localized to lang.
// NewRootCmdWithLang 创建一个指定语言的根 CLI 命令。
func NewRootCmdWithLang(version, commit, date string, lang i18n.Lang) *cobra.Command {
	tr := i18n.New(lang)
	var f CleanFlags
	var langFlag string
	var quietFlag bool
	var noColorFlag bool
	var versionFlag bool

	root := &cobra.Command{
		Use:           "beav",
		Short:         tr.Text("Fast, configurable Linux disk cache cleaner", "轻量、策略可配置的 Linux 磁盘缓存清理工具"),
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: tr.Text(
			"Beav cleans developer, browser, AI, container, and system caches with configurable safety policies.\n"+
				"Running `beav` with no subcommand in an interactive terminal launches the multi-view TUI workbench\n"+
				"(Clean, Disk Analyze, History, Safety & Config).\n"+
				"Or use subcommands directly: `beav scan`, `beav clean`, `beav purge`, `beav doctor`, `beav list`.",
			"Beav 用于清理开发工具、浏览器、AI、容器与系统缓存，所有安全边界均可按需配置或绕过。\n"+
				"在交互终端中直接运行 `beav` 会启动多视图交互工作台（清理 / 磁盘分析 / 历史 / 安全与配置）；\n"+
				"也可直接使用子命令：`beav scan`、`beav clean`、`beav purge`、`beav doctor`、`beav list`。",
		),
		Example: tr.Text(
			"  beav                       # interactive multi-view TUI workbench\n"+
				"  beav doctor                # check disk space, config, and available tools\n"+
				"  beav scan                  # read-only preview in CLI\n"+
				"  beav clean --only langs    # clean language caches (asks before deleting)\n"+
				"  beav clean --unsafe        # bypass all safety limits\n"+
				"  beav purge ~/.cache/custom # clean arbitrary paths directly\n"+
				"  sudo beav --system         # interactive TUI for system caches",
			"  beav                       # 启动多视图交互式工作台\n"+
				"  beav doctor                # 诊断磁盘占用、配置与可用工具状态\n"+
				"  beav scan                  # 在命令行只读预览可释放空间\n"+
				"  beav clean --only langs    # 清理语言工具缓存（删除前需确认）\n"+
				"  beav clean --unsafe        # 绕过全部安全限制直接清理\n"+
				"  beav purge ~/.cache/custom # 直接清理指定路径\n"+
				"  sudo beav --system         # 启动系统级缓存交互界面",
		),
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
			if noColorFlag {
				_ = os.Setenv("NO_COLOR", "1")
			}
			if langFlag == "" {
				return nil
			}
			if _, ok := i18n.ParseLang(langFlag); !ok {
				return ExitError{
					code: 1,
					err: fmt.Errorf("%s", tr.Textf(
						"unsupported language %q (valid: auto, en, zh)",
						"不支持的语言 %q（可选：auto、en、zh）",
						langFlag,
					)),
				}
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if versionFlag {
				_, err := fmt.Fprintf(cmd.OutOrStdout(), "beav %s (%s, %s)\n", version, commit, date)
				return err
			}
			localTr := translatorForCommand(cmd, tr)
			applyGlobalFlags(cmd, &f)
			if !isInteractiveTerminal(cmd.OutOrStdout()) {
				return cmd.Help()
			}
			return launchTUI(cmd.Context(), version, f, "clean", localTr)
		},
	}

	root.PersistentFlags().StringVar(&langFlag, "lang", "", tr.Text("interface language: auto, en, or zh", "界面语言：auto、en 或 zh"))
	root.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, tr.Text("suppress non-essential output", "静默模式，仅输出关键汇总与错误"))
	root.PersistentFlags().BoolVar(&noColorFlag, "no-color", false, tr.Text("disable colored terminal output", "禁用终端彩色输出"))

	root.Flags().BoolVarP(&versionFlag, "version", "V", false, tr.Text("print version and exit", "显示版本信息并退出"))
	root.Flags().BoolVar(&f.System, "system", false, tr.Text("launch TUI for system-scope caches (requires sudo)", "为系统级清理项启动交互界面（需 sudo）"))
	root.Flags().BoolVar(&f.All, "all", false, tr.Text("launch TUI for both user and system scopes (requires sudo)", "同时为用户级与系统级清理项启动交互界面（需 sudo）"))
	addScopeAndPolicyFlags(root, &f, tr)

	root.AddCommand(
		NewTUICmd(version, tr),
		NewScanCmd(tr),
		NewCleanCmd(tr),
		NewPurgeCmd(tr),
		NewAnalyzeCmd(tr),
		NewListCmd(tr),
		NewInfoCmd(tr),
		NewTagsCmd(tr),
		NewDoctorCmd(tr),
		NewHistoryCmd(tr),
		NewConfigCmd(tr),
		NewCompletionCmd(root, tr),
		NewVersionCmd(version, commit, date, tr),
	)
	configureHelpAndUsage(root, tr)
	return root
}

func isInteractiveTerminal(stdout io.Writer) bool {
	out, ok := stdout.(*os.File)
	if !ok || !sysinfo.IsTerminal(out) {
		return false
	}
	return sysinfo.IsTerminal(os.Stdin)
}
