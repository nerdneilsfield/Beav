package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewVersionCmd(version, commit, date string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print beav version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "beav %s (%s, %s)\n", version, commit, date)
			return err
		},
	}
}
