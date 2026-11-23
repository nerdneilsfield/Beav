package cli

import (
	"fmt"

	"github.com/dengqi/beav/internal/i18n"
	"github.com/spf13/cobra"
)

// NewVersionCmd creates a command that prints the beav version info.
// NewVersionCmd 创建一个打印 beav 版本信息的命令。
func NewVersionCmd(version, commit, date string, tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	return &cobra.Command{
		Use:   "version",
		Short: tr.Text("Print beav version", "打印 beav 版本信息"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "beav %s (%s, %s)\n", version, commit, date)
			return err
		},
	}
}
