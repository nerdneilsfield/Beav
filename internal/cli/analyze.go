// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"errors"
	"os"

	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui/tui"
	"github.com/spf13/cobra"
)

// NewAnalyzeCmd creates a command that analyzes disk usage in a TUI.
// NewAnalyzeCmd 创建一个在 TUI 中分析磁盘使用的命令。
func NewAnalyzeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "analyze [path]",
		Short: "Analyze disk usage in a TUI",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if !sysinfo.IsTerminal(os.Stdout) {
				return errors.New("analyze requires a TTY; try `du -sh *` or `gdu` for pipe-friendly disk analysis")
			}
			path := "."
			if len(args) == 1 {
				path = args[0]
			}
			return tui.RunAnalyze(path)
		},
	}
}
