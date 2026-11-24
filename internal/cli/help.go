package cli

import (
	"fmt"
	"strings"

	"github.com/dengqi/beav/internal/i18n"
	"github.com/spf13/cobra"
)

type helpTopic struct {
	Name  string
	Short func(*i18n.Translator) string
	Body  func(*i18n.Translator) string
}

var helpTopics = []helpTopic{
	{
		Name: "safety",
		Short: func(tr *i18n.Translator) string {
			return tr.Text("Configurable safety policy and bypass flags", "可配置安全策略与限制绕过说明")
		},
		Body: func(tr *i18n.Translator) string {
			return tr.Text(
				"Beav Safety Policy & Bypass Controls\n\n"+
					"Every restriction in Beav is configurable via ~/.config/beav/config.yaml (under `safety:`),\n"+
					"`beav config safety ...`, or per-run CLI flags. Only literal `/` is always protected.\n\n"+
					"CLI Bypass Flags:\n"+
					"  --unsafe, --force      Bypass all restrictions (heavy, age floors, blacklist, allow-list, process guards, commands)\n"+
					"  --allow-path <path>    Add extra allowed root paths for this run\n"+
					"  --bypass-allow-list    Allow paths outside default home/system roots\n"+
					"  --bypass-blacklist     Allow paths normally blocked by default blacklist (except /)\n"+
					"  --bypass-age-floor     Allow --min-age=0d even on managed caches and rustup\n"+
					"  --ignore-running       Skip running process and open-file checks\n"+
					"  --follow-symlinks      Allow traversing symlinks and symlinked tool binaries\n"+
					"  --allow-cross-fs       Allow crossing filesystem/mount boundaries\n"+
					"  --allow-root-home      Allow cleaning /root when running as root\n\n"+
					"Config Keys (`beav config safety set <key> <true|false>`):\n"+
					"  safety.bypass_all, safety.disable_allow_list, safety.replace_allow_list,\n"+
					"  safety.disable_blacklist, safety.replace_blacklist, safety.ignore_running_processes,\n"+
					"  safety.bypass_age_floor, safety.follow_symlinks, safety.allow_cross_fs,\n"+
					"  safety.allow_root_home, safety.allow_command_cleaners\n\n"+
					"Manage custom allow/deny prefixes:\n"+
					"  beav config safety allow-add /opt/cache\n"+
					"  beav config safety deny-add ~/important-cache\n",
				"Beav 安全策略与绕过控制\n\n"+
					"Beav 中的所有限制均可通过 ~/.config/beav/config.yaml（`safety:` 字段）、\n"+
					"`beav config safety ...` 或单次命令行参数配置与绕过；仅根目录 `/` 本身始终受保护。\n\n"+
					"命令行绕过选项：\n"+
					"  --unsafe, --force      绕过全部限制（重度项、年龄下限、黑名单、允许列表、进程检查、自定义命令）\n"+
					"  --allow-path <路径>    为本次运行临时追加允许清理的根路径\n"+
					"  --bypass-allow-list    允许清理默认主目录与系统目录之外的路径\n"+
					"  --bypass-blacklist     绕过默认黑名单保护（仍保护根目录 /）\n"+
					"  --bypass-age-floor     绕过内置策略最小天数下限（如允许 0d 清理托管缓存与 rustup）\n"+
					"  --ignore-running       忽略关联进程运行中与缓存占用检查\n"+
					"  --follow-symlinks      允许跟踪符号链接及符号链接命令\n"+
					"  --allow-cross-fs       允许跨越文件系统或挂载点边界\n"+
					"  --allow-root-home      以 root 运行时允许清理 /root\n\n"+
					"配置项（`beav config safety set <key> <true|false>`）：\n"+
					"  safety.bypass_all, safety.disable_allow_list, safety.replace_allow_list,\n"+
					"  safety.disable_blacklist, safety.replace_blacklist, safety.ignore_running_processes,\n"+
					"  safety.bypass_age_floor, safety.follow_symlinks, safety.allow_cross_fs,\n"+
					"  safety.allow_root_home, safety.allow_command_cleaners\n\n"+
					"管理自定义允许/禁止前缀：\n"+
					"  beav config safety allow-add /opt/cache\n"+
					"  beav config safety deny-add ~/important-cache\n",
			)
		},
	},
	{
		Name: "exit-codes",
		Short: func(tr *i18n.Translator) string {
			return tr.Text("Process exit codes for scripts and CI", "脚本与 CI 场景的进程退出码说明")
		},
		Body: func(tr *i18n.Translator) string {
			return tr.Text(
				"Exit Codes:\n"+
					"  0  Success (selected cleaners completed without errors)\n"+
					"  1  Invalid usage, unknown cleaner/tag, or refused privilege request\n"+
					"  2  Configuration, registry, or path validation failed\n"+
					"  3  One or more cleaners reported an execution error\n"+
					"  4  Interrupted by signal (SIGINT / SIGTERM)\n",
				"退出码说明：\n"+
					"  0  成功（所选清理项均完成且无错误）\n"+
					"  1  参数错误、未知清理项/标签或权限请求被拒绝\n"+
					"  2  配置文件、清理项定义或路径校验失败\n"+
					"  3  至少有一个清理项在执行时出错\n"+
					"  4  被信号中断（SIGINT / SIGTERM）\n",
			)
		},
	},
	{
		Name: "env",
		Short: func(tr *i18n.Translator) string {
			return tr.Text("Environment variables recognized by beav", "beav 识别的环境变量")
		},
		Body: func(tr *i18n.Translator) string {
			return tr.Text(
				"Environment Variables:\n"+
					"  BEAV_LANG       UI language override: auto, en, or zh\n"+
					"  BEAV_CONFIG_DIR Default configuration directory override\n"+
					"  BEAV_UNSAFE     Set to 1 or true to enable full bypass mode\n"+
					"  BEAV_NO_OPLOG   Set to any non-empty value to disable operations.log\n"+
					"  NO_COLOR        Disable colored terminal output when non-empty\n"+
					"  VISUAL / EDITOR Editor launched by `beav config edit`\n",
				"环境变量：\n"+
					"  BEAV_LANG       覆盖界面语言：auto、en 或 zh\n"+
					"  BEAV_CONFIG_DIR 覆盖默认配置目录\n"+
					"  BEAV_UNSAFE     设为 1 或 true 时默认启用全绕过模式\n"+
					"  BEAV_NO_OPLOG   非空时禁用写入 operations.log\n"+
					"  NO_COLOR        非空时禁用终端彩色输出\n"+
					"  VISUAL / EDITOR `beav config edit` 使用的文本编辑器\n",
			)
		},
	},
	{
		Name: "examples",
		Short: func(tr *i18n.Translator) string {
			return tr.Text("Common workflows and recipes", "常用场景示例")
		},
		Body: func(tr *i18n.Translator) string {
			return tr.Text(
				"Common Workflows:\n"+
					"  beav                                   # Launch full multi-tab TUI workbench\n"+
					"  beav doctor                            # Check disk usage, config, tools, and policy\n"+
					"  beav scan --only langs,ai              # Preview language & AI caches\n"+
					"  beav clean --only langs --yes          # Clean language caches non-interactively\n"+
					"  beav clean --unsafe --min-age=0d       # Bypass all limits and clean regardless of age\n"+
					"  beav purge ~/.cache/custom --dry-run   # Preview direct path cleanup\n"+
					"  beav history -n 20                     # Inspect recent cleanup operations\n"+
					"  sudo beav clean --all                  # Clean user home + system caches\n"+
					"  beav config safety set bypass_all true # Persist full bypass in config.yaml\n",
				"常用操作示例：\n"+
					"  beav                                   # 启动多视图交互工作台\n"+
					"  beav doctor                            # 检查磁盘空间、配置、工具可用性与安全策略\n"+
					"  beav scan --only langs,ai              # 只读预览语言工具与 AI 缓存\n"+
					"  beav clean --only langs --yes          # 免确认直接清理语言工具缓存\n"+
					"  beav clean --unsafe --min-age=0d       # 绕过全部限制并按 0 天直接清理\n"+
					"  beav purge ~/.cache/custom --dry-run   # 预览直接路径清理\n"+
					"  beav history -n 20                     # 查看最近 20 条清理操作记录\n"+
					"  sudo beav clean --all                  # 一次性清理用户目录与系统级缓存\n"+
					"  beav config safety set bypass_all true # 在 config.yaml 中持久化启用全绕过\n",
			)
		},
	},
}

func configureHelpAndUsage(root *cobra.Command, tr *i18n.Translator) {
	if root == nil || tr == nil {
		return
	}

	root.AddGroup(
		&cobra.Group{ID: "core", Title: tr.Text("Core & Cleanup Commands:", "核心清理命令：")},
		&cobra.Group{ID: "inspect", Title: tr.Text("Discovery & Diagnostics:", "浏览与诊断命令：")},
		&cobra.Group{ID: "util", Title: tr.Text("Configuration & Utilities:", "配置与工具命令：")},
	)
	root.SetHelpCommandGroupID("util")
	root.SetCompletionCommandGroupID("util")

	topicsSection := func() string {
		var b strings.Builder
		b.WriteString("\n" + tr.Text("Help Topics:", "帮助主题：") + "\n")
		for _, topic := range helpTopics {
			b.WriteString(fmt.Sprintf("  %-14s %s\n", topic.Name, topic.Short(tr)))
		}
		b.WriteString("\n" + tr.Text(
			`Use "beav help <command|topic>" or "beav <command> --help" for more information.`,
			`使用 "beav help <命令|主题>" 或 "beav <命令> --help" 查看详细说明。`,
		) + "\n")
		return b.String()
	}

	tmpl := buildUsageTemplate(tr, topicsSection())
	root.SetUsageTemplate(tmpl)

	helpCmd := &cobra.Command{
		Use:     "help [command|topic]",
		GroupID: "util",
		Short:   tr.Text("Help about any command or topic", "查看任意命令的帮助信息"),
		Long:    tr.Text("Display detailed help for any command or built-in topic (safety, exit-codes, env, examples).", "查看 Beav 任意命令或内置主题（safety、exit-codes、env、examples）的详细用法与选项说明。"),
		ValidArgsFunction: func(c *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			var completions []string
			if len(args) == 0 {
				for _, topic := range helpTopics {
					if strings.HasPrefix(topic.Name, toComplete) {
						completions = append(completions, fmt.Sprintf("%s\t%s", topic.Name, topic.Short(tr)))
					}
				}
			}
			cmd, _, e := c.Root().Find(args)
			if e != nil {
				return completions, cobra.ShellCompDirectiveNoFileComp
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
			localTr := translatorForCommand(c, tr)
			if len(args) == 1 {
				for _, topic := range helpTopics {
					if strings.EqualFold(args[0], topic.Name) {
						_, _ = fmt.Fprint(c.OutOrStdout(), topic.Body(localTr))
						return
					}
				}
			}
			target, _, e := c.Root().Find(args)
			if target == nil || e != nil {
				c.Printf("%s %#q\n", localTr.Text("Unknown help topic or command", "未知命令或帮助主题"), args)
				_ = c.Root().Usage()
				return
			}
			target.InitDefaultHelpFlag()
			target.InitDefaultVersionFlag()
			_ = target.Help()
		},
	}
	helpCmd.Flags().BoolP("help", "h", false, tr.Text("help for help", "显示 help 的帮助信息"))
	root.SetHelpCommand(helpCmd)

	var registerHelpFlag func(*cobra.Command)
	registerHelpFlag = func(c *cobra.Command) {
		if c.Flags().Lookup("help") == nil {
			c.Flags().BoolP("help", "h", false, tr.Textf("help for %s", "显示 %s 的帮助信息", c.Name()))
		}
		for _, sub := range c.Commands() {
			registerHelpFlag(sub)
		}
	}
	registerHelpFlag(root)
}

func buildUsageTemplate(tr *i18n.Translator, topicsFooter string) string {
	usageHdr := tr.Text("Usage:", "用法：")
	aliasesHdr := tr.Text("Aliases:", "别名：")
	examplesHdr := tr.Text("Examples:", "示例：")
	availCmdsHdr := tr.Text("Available Commands:", "可用命令：")
	addCmdsHdr := tr.Text("Additional Commands:", "其他命令：")
	flagsHdr := tr.Text("Flags:", "选项：")
	globalFlagsHdr := tr.Text("Global Flags:", "全局选项：")
	helpTopicsHdr := tr.Text("Additional help topics:", "其他帮助主题：")
	footer := tr.Text(
		`Use "{{.CommandPath}} [command] --help" for more information about a command.`,
		`使用 "{{.CommandPath}} [command] --help" 查看指定命令的详细说明。`,
	)

	var b strings.Builder
	b.WriteString(usageHdr + `{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

` + aliasesHdr + `
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

` + examplesHdr + `
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

` + availCmdsHdr + `{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

` + addCmdsHdr + `{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

` + flagsHdr + `
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

` + globalFlagsHdr + `
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

` + helpTopicsHdr + `{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}
`)
	b.WriteString("{{if not .HasParent}}" + topicsFooter + "{{else}}\n" + footer + "\n{{end}}")
	return b.String()
}
