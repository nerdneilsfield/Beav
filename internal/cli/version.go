// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewVersionCmd creates a command that prints the beav version info.
// NewVersionCmd 创建一个打印 beav 版本信息的命令。
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
