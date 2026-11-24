package cli

import (
	"github.com/dengqi/beav/internal/i18n"
	"github.com/spf13/cobra"
)

// NewCompletionCmd creates a command that generates shell completion scripts.
// NewCompletionCmd 创建一个生成 shell 补全脚本的命令。
func NewCompletionCmd(root *cobra.Command, tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish]",
		GroupID:   "util",
		Short:     tr.Text("Generate shell completion script", "生成 Shell 自动补全脚本"),
		ValidArgs: []string{"bash", "zsh", "fish"},
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":
				return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return root.GenFishCompletion(cmd.OutOrStdout(), true)
			}
			return nil
		},
	}
}
