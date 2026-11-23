package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/spf13/cobra"
)

// NewListCmd creates the `beav list` command that prints all available cleaners and their effective settings.
// NewListCmd 创建 `beav list` 命令，展示全部清理器及其实际生效的设置。
func NewListCmd(tr *i18n.Translator) *cobra.Command {
	var scopeFilter string
	var tagFilter string
	var onlyHeavy bool
	var onlyDisabled bool
	var showJSON bool
	var configDir string
	var builtinDisabled bool

	cmd := &cobra.Command{
		Use:   "list",
		Short: tr.Text("List all cleaners with IDs, impact levels, effective age, and state", "列出全部清理器及其 ID、影响级别、生效保留天数和状态"),
		Long: tr.Text(
			"Display built-in and custom cleaners along with their effective retention age and enabled state.\n"+
				"Use the cleaner ID or any tag with `beav scan --only`, `beav clean --only`, or `beav info <id>`.",
			"展示所有内置与自定义清理器，并反映配置文件覆盖后的实际保留天数与启用状态。\n"+
				"可将列表中的 ID 或标签用于 `beav scan --only`、`beav clean --only` 或 `beav info <ID>`。",
		),
		Example: tr.Text(
			"  beav list\n"+
				"  beav list --scope user --tag langs\n"+
				"  beav list --heavy\n"+
				"  beav list --json",
			"  beav list\n"+
				"  beav list --scope user --tag langs\n"+
				"  beav list --heavy\n"+
				"  beav list --json",
		),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			if scopeFilter != "" && scopeFilter != "user" && scopeFilter != "system" && scopeFilter != "all" {
				return ExitError{
					code: 1,
					err: fmt.Errorf("%s", localTr.Textf("invalid --scope %q (valid: user, system, all)", "无效的 --scope 参数 %q（可选: user、system、all）", scopeFilter)),
				}
			}

			cfg, effective, _, err := loadCatalog(configDir, CleanFlags{BuiltinDisabled: builtinDisabled})
			if err != nil {
				return err
			}
			sortCleaners(effective)

			filtered := make([]model.Cleaner, 0, len(effective))
			for _, c := range effective {
				if scopeFilter != "" && scopeFilter != "all" && string(c.Scope) != scopeFilter {
					continue
				}
				if tagFilter != "" && !hasTag(c.Tags, tagFilter) {
					continue
				}
				if onlyHeavy && c.EffectiveImpact() != model.ImpactHeavy {
					continue
				}
				if onlyDisabled && c.IsEnabled() {
					continue
				}
				filtered = append(filtered, c)
			}

			if showJSON {
				out := map[string]any{
					"defaults": cfg.Defaults,
					"cleaners": filtered,
				}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				return enc.Encode(out)
			}

			if len(filtered) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), localTr.Text("No cleaners match the given filters.", "没有符合当前筛选条件的清理器。"))
				return nil
			}

			headers := []string{
				"ID",
				localTr.Text("NAME", "名称"),
				localTr.Text("SCOPE", "作用域"),
				localTr.Text("IMPACT", "影响"),
				localTr.Text("AGE", "保留期"),
				localTr.Text("TAGS", "标签"),
				localTr.Text("STATE", "状态"),
			}
			rows := make([][]string, 0, len(filtered))
			for _, c := range filtered {
				age := "-"
				if c.MinAgeDays != nil {
					age = localTr.Days(*c.MinAgeDays)
				}
				state := localTr.Text("enabled", "启用")
				if !c.IsEnabled() {
					state = localTr.Text("disabled", "禁用")
				}
				rows = append(rows, []string{
					c.ID,
					localTr.CleanerName(c),
					localTr.ScopeLabel(c.Scope),
					localTr.ImpactLabel(c.EffectiveImpact()),
					age,
					strings.Join(c.Tags, ","),
					state,
				})
			}
			writeAlignedTable(cmd.OutOrStdout(), headers, rows)
			fmt.Fprintf(cmd.OutOrStdout(), "\n%s\n", localTr.Textf(
				"%s shown. Inspect details with `beav info <id>` or preview with `beav scan --only <id>`.",
				"共显示 %s。查看详情请运行 `beav info <ID>`，预演扫描请运行 `beav scan --only <ID>`。",
				localTr.Plural(int64(len(filtered)), "cleaner", "cleaners", "项清理器"),
			))
			return nil
		},
	}

	cmd.Flags().StringVar(&scopeFilter, "scope", "", tr.Text("filter by scope: user, system, or all", "按作用域筛选：user、system 或 all"))
	cmd.Flags().StringVar(&tagFilter, "tag", "", tr.Text("filter by tag (for example langs, ai, browser)", "按标签筛选（例如 langs、ai、browser）"))
	cmd.Flags().BoolVar(&onlyHeavy, "heavy", false, tr.Text("show only heavy cleaners", "仅显示重度清理项"))
	cmd.Flags().BoolVar(&onlyDisabled, "disabled", false, tr.Text("show only disabled cleaners", "仅显示已禁用的清理项"))
	cmd.Flags().BoolVar(&showJSON, "json", false, tr.Text("output machine-readable JSON", "输出机器可读 JSON"))
	cmd.Flags().StringVar(&configDir, "config-dir", "", tr.Text("override config directory", "覆盖默认配置目录"))
	cmd.Flags().BoolVar(&builtinDisabled, "builtin-disabled", false, "skip embedded cleaners")
	_ = cmd.Flags().MarkHidden("builtin-disabled")
	return cmd
}

func hasTag(tags []string, want string) bool {
	for _, t := range tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
}
