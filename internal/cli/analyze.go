package cli

import (
	"errors"
	"os"

	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui/tui"
	"github.com/spf13/cobra"
)

// NewAnalyzeCmd creates a command that launches the bundled read-only disk usage explorer.
// NewAnalyzeCmd 创建一个启动内置只读磁盘空间浏览器的命令。
func NewAnalyzeCmd(tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	return &cobra.Command{
		Use:   "analyze [path]",
		Short: tr.Text("Explore disk usage in a read-only TUI (powered by gdu)", "以只读 TUI 界面分析目录磁盘占用（基于 gdu）"),
		Long: tr.Text(
			"Launch the bundled read-only disk usage explorer (powered by gdu) for the given directory (defaults to current directory).\n"+
				"Deletion and shell spawning are disabled for safety. Press q to quit.",
			"启动内置的只读磁盘占用浏览器（基于 gdu），分析指定目录（默认为当前目录）的空间分布。\n"+
				"为保证安全已禁用删除与外部 Shell 功能；按 q 键退出。",
		),
		Example: "  beav analyze\n  beav analyze ~/.cache",
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
			return tui.RunAnalyze(path)
		},
	}
}
