package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui/tui"
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

	root := &cobra.Command{
		Use:           "beav",
		Short:         tr.Text("Safe, age-aware Linux cache cleaner", "安全、按留存天数过滤的 Linux 缓存清理工具"),
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: tr.Text(
			"Beav safely cleans developer, browser, container, and system caches without touching personal files.\n"+
				"Running `beav` with no subcommand in an interactive terminal launches a full-screen read-only scan and selection workbench.\n"+
				"Or use subcommands directly: `beav scan` to preview, `beav clean` to clean, `beav list` to browse cleaners.",
			"Beav 用于安全清理开发工具、浏览器、容器与系统缓存，绝不触碰个人文件与凭据。\n"+
				"在交互终端中直接运行 `beav` 会启动全屏只读扫描与勾选工作台；\n"+
				"也可以直接使用子命令：`beav scan` 只读预览、`beav clean` 命令行清理、`beav list` 浏览所有清理项。",
		),
		Example: tr.Text(
			"  beav                       # interactive scan & clean TUI\n"+
				"  beav scan                  # read-only preview in CLI\n"+
				"  beav clean --only langs    # clean language caches (asks before deleting)\n"+
				"  beav list                  # list all cleaners and IDs\n"+
				"  sudo beav --system         # interactive TUI for system caches",
			"  beav                       # 启动交互式只读扫描与清理界面\n"+
				"  beav scan                  # 在命令行只读预览可释放空间\n"+
				"  beav clean --only langs    # 清理语言工具缓存（删除前需确认）\n"+
				"  beav list                  # 列出所有清理项、影响级别与 ID\n"+
				"  sudo beav --system         # 启动系统级缓存交互界面",
		),
		PersistentPreRunE: func(_ *cobra.Command, _ []string) error {
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
			localTr := translatorForCommand(cmd, tr)
			f.Lang = langFlag
			if !isInteractiveTerminal(cmd.OutOrStdout()) {
				return cmd.Help()
			}
			lc, err := loadCleanContext(f, localTr)
			if err != nil {
				return err
			}
			opts := tui.Options{
				Translator: localTr,
				Scope: tui.ScopeInfo{
					Scope:  lc.Scope,
					Home:   lc.Home,
					IsRoot: os.Getuid() == 0,
				},
				Hooks: tui.Hooks{
					StartScan: func(ctx context.Context) (<-chan tui.ScanProgress, context.CancelFunc) {
						return runReadOnlyScan(ctx, lc.Merged, lc.Scope, lc.Home, lc.TargetUID, lc.Cfg)
					},
					StartExec: func(ctx context.Context, selectedIDs []string) (<-chan tui.ExecProgress, context.CancelFunc) {
						return runTUIExec(ctx, lc.Merged, lc.Scope, lc.Home, lc.TargetUID, lc.Cfg, f, selectedIDs)
					},
				},
			}
			return tui.Run(cmd.Context(), opts)
		},
	}

	root.PersistentFlags().StringVar(&langFlag, "lang", "", tr.Text("interface language: auto, en, or zh", "界面语言：auto、en 或 zh"))
	root.Flags().BoolVar(&f.System, "system", false, tr.Text("launch TUI for system-scope caches (requires sudo)", "为系统级清理项启动交互界面（需 sudo）"))
	root.Flags().BoolVar(&f.All, "all", false, tr.Text("launch TUI for both user and system scopes (requires sudo)", "同时为用户级与系统级清理项启动交互界面（需 sudo）"))
	root.Flags().StringVar(&f.MinAge, "min-age", "", tr.Text("override min age, for example 14d or tag=3d", "覆盖最小保留天数，如 14d 或 langs=3d"))
	root.Flags().BoolVar(&f.AllowRootHome, "allow-root-home", false, tr.Text("allow scanning /root when running as root", "以 root 运行时允许扫描 /root 目录"))
	root.Flags().StringVar(&f.UserOverride, "user", "", tr.Text("target user for --all", "配合 --all 指定目标用户名"))
	root.Flags().StringVar(&f.ConfigDir, "config-dir", "", tr.Text("override config directory", "覆盖默认配置目录"))
	root.Flags().BoolVar(&f.BuiltinDisabled, "builtin-disabled", false, "skip embedded cleaners")
	_ = root.Flags().MarkHidden("builtin-disabled")

	root.AddCommand(
		NewScanCmd(tr),
		NewCleanCmd(tr),
		NewListCmd(tr),
		NewInfoCmd(tr),
		NewConfigCmd(tr),
		NewAnalyzeCmd(tr),
		NewCompletionCmd(root, tr),
		NewVersionCmd(version, commit, date, tr),
	)
	localizeCobraTree(root, tr)
	return root
}

func localizeCobraTree(root *cobra.Command, tr *i18n.Translator) {
	if root == nil || tr == nil || !tr.IsZH() {
		return
	}
	replacer := strings.NewReplacer(
		"Global Flags:", "全局选项：",
		"Additional Commands:", "其他命令：",
		"Available Commands:", "可用命令：",
		"Additional help topics:", "其他帮助主题：",
		"Usage:", "用法：",
		"Aliases:", "别名：",
		"Examples:", "示例：",
		"Flags:", "选项：",
		`Use "{{.CommandPath}} [command] --help" for more information about a command.`,
		`使用 "{{.CommandPath}} [command] --help" 查看指定命令的详细说明。`,
	)
	root.SetUsageTemplate(replacer.Replace(root.UsageTemplate()))

	helpCmd := &cobra.Command{
		Use:   "help [command]",
		Short: "查看任意命令的帮助信息",
		Long:  "查看 Beav 任意命令的详细用法与选项说明。",
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var completions []string
			cmd, _, e := c.Root().Find(args)
			if e != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			if cmd == nil {
				cmd = c.Root()
			}
			for _, sub := range cmd.Commands() {
				if sub.IsAvailableCommand() && strings.HasPrefix(sub.Name(), toComplete) {
					completions = append(completions, fmt.Sprintf("%s\t%s", sub.Name(), sub.Short))
				}
			}
			return completions, cobra.ShellCompDirectiveNoFileComp
		},
		Run: func(c *cobra.Command, args []string) {
			target, _, e := c.Root().Find(args)
			if target == nil || e != nil {
				c.Printf("未知命令或帮助主题 %#q\n", args)
				_ = c.Root().Usage()
				return
			}
			target.InitDefaultHelpFlag()
			target.InitDefaultVersionFlag()
			_ = target.Help()
		},
	}
	helpCmd.Flags().BoolP("help", "h", false, "显示 help 的帮助信息")
	root.SetHelpCommand(helpCmd)

	var registerHelpFlag func(*cobra.Command)
	registerHelpFlag = func(c *cobra.Command) {
		if c.Flags().Lookup("help") == nil {
			c.Flags().BoolP("help", "h", false, fmt.Sprintf("显示 %s 的帮助信息", c.Name()))
		}
		for _, sub := range c.Commands() {
			registerHelpFlag(sub)
		}
	}
	registerHelpFlag(root)
}

func isInteractiveTerminal(stdout io.Writer) bool {
	out, ok := stdout.(*os.File)
	if !ok || !sysinfo.IsTerminal(out) {
		return false
	}
	return sysinfo.IsTerminal(os.Stdin)
}
