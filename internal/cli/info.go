package cli

import (
	"fmt"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/spf13/cobra"
)

// NewInfoCmd creates the `beav info <cleaner-id>` command showing detailed cleaner metadata.
// NewInfoCmd 创建 `beav info <cleaner-id>` 命令，用于查看单个清理器的详细说明与规则。
func NewInfoCmd(tr *i18n.Translator) *cobra.Command {
	var configDir string
	var builtinDisabled bool

	cmd := &cobra.Command{
		Use:   "info <cleaner-id>",
		Short: tr.Text("Inspect details, safety rules, and targets for one cleaner", "查看单个清理器的详情、安全规则与清理目标"),
		Long: tr.Text(
			"Print the full description, effective age threshold, impact level, target paths or native command profile, and copy-ready commands for one cleaner.",
			"显示单个清理器的中英文说明、实际生效的保留天数、影响级别、目标路径或原生命令配置以及可直接使用的示例命令。",
		),
		Example: tr.Text(
			"  beav info lang-pip\n"+
				"  beav info lang-rustup",
			"  beav info lang-pip\n"+
				"  beav info lang-rustup",
		),
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			_, effective, _, err := loadCatalog(configDir, CleanFlags{BuiltinDisabled: builtinDisabled})
			if err != nil {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var ids []string
			for _, c := range effective {
				if strings.HasPrefix(c.ID, toComplete) {
					ids = append(ids, c.ID)
				}
			}
			return ids, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			id := args[0]

			cfg, effective, _, err := loadCatalog(configDir, CleanFlags{BuiltinDisabled: builtinDisabled})
			if err != nil {
				return err
			}

			var target *model.Cleaner
			for i := range effective {
				if effective[i].ID == id {
					target = &effective[i]
					break
				}
			}
			if target == nil {
				suggestions := suggestCleanerIDs(effective, id, 3)
				if len(suggestions) > 0 {
					return ExitError{
						code: 1,
						err: fmt.Errorf("%s", localTr.Textf(
							"unknown cleaner id %q (did you mean: %s?); run `beav list` to see all IDs",
							"未找到清理器 ID %q（是否指: %s？）；运行 `beav list` 查看全部 ID",
							id, strings.Join(suggestions, ", "),
						)),
					}
				}
				return ExitError{
					code: 1,
					err: fmt.Errorf("%s", localTr.Textf(
						"unknown cleaner id %q; run `beav list` to see all IDs",
						"未找到清理器 ID %q；运行 `beav list` 查看全部 ID",
						id,
					)),
				}
			}

			c := *target
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s (%s)\n", localTr.CleanerName(c), c.ID)
			if desc := localTr.CleanerDesc(c); desc != "" {
				fmt.Fprintf(out, "%s\n", desc)
			}
			fmt.Fprintln(out)

			scopeText := localTr.ScopeLabel(c.Scope)
			if c.Scope == model.ScopeSystem {
				scopeText += localTr.Text(" (requires sudo)", "（需要 sudo 权限）")
			}
			fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Scope:", "作用域:"), scopeText)

			impactText := localTr.ImpactLabel(c.EffectiveImpact())
			if note := localTr.HeavyNote(c); note != "" {
				impactText += " - " + note
			}
			fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Impact:", "影响级别:"), impactText)

			stateText := localTr.Text("enabled", "启用")
			if !c.IsEnabled() {
				stateText = localTr.Text("disabled", "禁用")
			}
			if ov, ok := cfg.Overrides[c.ID]; ok && ov.Enabled != nil {
				stateText += localTr.Text(" (overridden in config.yaml)", "（已在 config.yaml 中覆盖）")
			}
			fmt.Fprintf(out, "%-16s %s\n", localTr.Text("State:", "当前状态:"), stateText)

			ageText := localTr.Text("command-managed (no fixed day filter)", "由原生工具策略控制（无固定天数）")
			if c.MinAgeDays != nil {
				tf := string(c.TimeField)
				if tf == "" {
					tf = string(model.TimeMtime)
				}
				ageText = fmt.Sprintf("%s (%s)", localTr.Days(*c.MinAgeDays), tf)
				if ov, ok := cfg.Overrides[c.ID]; ok && ov.MinAgeDays != nil {
					ageText += localTr.Text(" (overridden in config.yaml)", "（已在 config.yaml 中覆盖）")
				}
			}
			fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Min age:", "保留天数:"), ageText)

			if len(c.Tags) > 0 {
				fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Tags:", "标签:"), strings.Join(c.Tags, ", "))
			}
			fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Executor:", "执行器:"), string(c.Type))

			if prof := c.Profile(); prof != "" {
				fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Profile:", "内置配置:"), prof)
			}
			if c.PkgCache != nil {
				fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Manager:", "包管理器:"), c.PkgCache.Manager)
			}
			if c.ContainerPrune != nil {
				fmt.Fprintf(out, "%-16s %s (%s)\n", localTr.Text("Container:", "容器目标:"), c.ContainerPrune.Runtime, c.ContainerPrune.Target)
			}
			if len(c.Paths) > 0 {
				fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Paths:", "目标路径:"), strings.Join(c.Paths, ", "))
			}
			if len(c.PathResolvers) > 0 {
				var resolvers []string
				for _, r := range c.PathResolvers {
					if len(r.Subpaths) > 0 {
						resolvers = append(resolvers, fmt.Sprintf("%s/[%s]", r.Resolver, strings.Join(r.Subpaths, ",")))
					} else {
						resolvers = append(resolvers, r.Resolver)
					}
				}
				fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Resolvers:", "动态解析:"), strings.Join(resolvers, ", "))
			}
			if len(c.Exclude) > 0 {
				fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Excludes:", "排除规则:"), strings.Join(c.Exclude, ", "))
			}
			if len(c.RunningProcesses) > 0 {
				fmt.Fprintf(out, "%-16s %s\n", localTr.Text("Process guard:", "进程保护:"), strings.Join(c.RunningProcesses, ", "))
			}

			fmt.Fprintln(out)
			fmt.Fprintln(out, localTr.Text("Commands:", "常用命令:"))
			prefix := "beav"
			scopeFlag := ""
			if c.Scope == model.ScopeSystem {
				prefix = "sudo beav"
				scopeFlag = " --system"
			}
			fmt.Fprintf(out, "  %s scan%s --only %s\n", prefix, scopeFlag, c.ID)
			fmt.Fprintf(out, "  %s clean%s --only %s\n", prefix, scopeFlag, c.ID)
			if c.IsEnabled() {
				fmt.Fprintf(out, "  beav config disable %s\n", c.ID)
			} else {
				fmt.Fprintf(out, "  beav config enable %s\n", c.ID)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&configDir, "config-dir", "", tr.Text("override config directory", "覆盖默认配置目录"))
	cmd.Flags().BoolVar(&builtinDisabled, "builtin-disabled", false, "skip embedded cleaners")
	_ = cmd.Flags().MarkHidden("builtin-disabled")
	return cmd
}
