package cli

import (
	"errors"
	"os"

	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui/tui"
	"github.com/spf13/cobra"
)

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
