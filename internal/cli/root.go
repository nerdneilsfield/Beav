package cli

import (
	"os"

	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui/tui"
	"github.com/spf13/cobra"
)

func NewRootCmd(version, commit, date string) *cobra.Command {
	root := &cobra.Command{
		Use:   "beav",
		Short: "Linux cache cleaner",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if out, ok := cmd.OutOrStdout().(*os.File); !ok || !sysinfo.IsTerminal(out) {
				return cmd.Help()
			}
			choice, err := tui.RunMenu()
			if err != nil {
				return err
			}
			switch choice {
			case "clean":
				return runClean(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), CleanFlags{})
			case "analyze":
				return tui.RunAnalyze(".")
			case "list":
				return runConfigShow(cmd, "")
			}
			return nil
		},
	}
	root.AddCommand(NewVersionCmd(version, commit, date))
	root.AddCommand(NewCleanCmd())
	root.AddCommand(NewConfigCmd())
	root.AddCommand(NewAnalyzeCmd())
	root.AddCommand(NewCompletionCmd(root))
	return root
}
