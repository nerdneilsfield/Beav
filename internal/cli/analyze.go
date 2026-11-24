package cli

import (
	"errors"
	"os"

	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui/tui"
	"github.com/spf13/cobra"
)

// NewAnalyzeCmd creates a command that launches the bundled disk usage explorer.
// NewAnalyzeCmd 创建一个启动内置磁盘空间浏览器的命令。
func NewAnalyzeCmd(tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	var allowDelete bool
	var unsafe bool

	cmd := &cobra.Command{
		Use:     "analyze [path]",
		GroupID: "core",
		Short:   tr.Text("Explore disk usage in a TUI (powered by gdu)", "以交互界面分析目录磁盘占用（基于 gdu）"),
		Long: tr.Text(
			"Launch the bundled disk usage explorer (powered by gdu) for the given directory (defaults to current directory).\n"+
				"Default mode is read-only; pass --allow-delete or --unsafe to enable deletion inside gdu. Press q to quit.",
			"启动内置的磁盘占用浏览器（基于 gdu），分析指定目录（默认为当前目录）的空间分布。\n"+
				"默认以只读模式运行；加上 --allow-delete 或 --unsafe 可在界面中直接删除文件。按 q 键退出。",
		),
		Example: "  beav analyze\n  beav analyze ~/.cache\n  beav analyze ~/.cache --allow-delete",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			if !sysinfo.IsTerminal(os.Stdout) {
				return ExitError{
					code: 1,
					err: errors.New(localTr.Text(
						"analyze requires an interactive TTY; try `du -sh *` or `gdu` for pipe-friendly disk analysis",
						"analyze 需要交互式终端（TTY）；管道或脚本环境请改用 `du -sh *` 或 `gdu`",
					)),
				}
			}
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			return tui.RunAnalyzeWithOptions(path, allowDelete || unsafe)
		},
	}
	cmd.Flags().BoolVar(&allowDelete, "allow-delete", false, tr.Text("enable file deletion inside gdu", "允许在 gdu 界面中直接删除文件"))
	cmd.Flags().BoolVar(&unsafe, "unsafe", false, tr.Text("enable deletion and shell inside gdu", "允许在 gdu 界面中执行删除与 Shell"))
	return cmd
}
