package cli

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/spf13/cobra"
)

type tagSummary struct {
	Tag        string   `json:"tag"`
	Count      int      `json:"count"`
	HeavyCount int      `json:"heavy_count"`
	Scopes     []string `json:"scopes"`
	SampleIDs  []string `json:"sample_ids"`
}

// NewTagsCmd creates the `beav tags` command listing available cleaner tags.
// NewTagsCmd 创建 `beav tags` 命令，列出全部可用清理标签及统计信息。
func NewTagsCmd(tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	var scopeFilter string
	var showJSON bool
	var configDir string

	cmd := &cobra.Command{
		Use:     "tags",
		GroupID: "inspect",
		Short:   tr.Text("List all cleaner tags and matching cleaner counts", "列出所有清理项标签及包含的项目数量"),
		Example: "  beav tags\n  beav tags --scope user\n  beav tags --json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			if scopeFilter != "" && scopeFilter != "user" && scopeFilter != "system" && scopeFilter != "all" {
				return ExitError{code: 1, err: fmt.Errorf("%s", localTr.Textf("invalid --scope %q (valid: user, system, all)", "无效的 --scope 参数 %q（可选：user、system、all）", scopeFilter))}
			}
			_, effective, _, err := loadCatalog(configDir, CleanFlags{}, localTr)
			if err != nil {
				return err
			}
			byTag := map[string]*tagSummary{}
			scopeSeen := map[string]map[string]bool{}
			for _, c := range effective {
				if scopeFilter != "" && scopeFilter != "all" && string(c.Scope) != scopeFilter {
					continue
				}
				for _, tag := range c.Tags {
					ts, ok := byTag[tag]
					if !ok {
						ts = &tagSummary{Tag: tag}
						byTag[tag] = ts
						scopeSeen[tag] = map[string]bool{}
					}
					ts.Count++
					if c.EffectiveImpact() == model.ImpactHeavy {
						ts.HeavyCount++
					}
					if !scopeSeen[tag][string(c.Scope)] {
						scopeSeen[tag][string(c.Scope)] = true
						ts.Scopes = append(ts.Scopes, string(c.Scope))
					}
					if len(ts.SampleIDs) < 4 {
						ts.SampleIDs = append(ts.SampleIDs, c.ID)
					}
				}
			}
			list := make([]tagSummary, 0, len(byTag))
			for _, ts := range byTag {
				sort.Strings(ts.Scopes)
				list = append(list, *ts)
			}
			sort.Slice(list, func(i, j int) bool {
				if list[i].Count != list[j].Count {
					return list[i].Count > list[j].Count
				}
				return list[i].Tag < list[j].Tag
			})

			out := cmd.OutOrStdout()
			if showJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(list)
			}
			if len(list) == 0 {
				fmt.Fprintln(out, localTr.Text("No tags found.", "未找到任何标签。"))
				return nil
			}
			rows := make([][]string, 0, len(list))
			for _, ts := range list {
				rows = append(rows, []string{
					ts.Tag,
					strconv.Itoa(ts.Count),
					strconv.Itoa(ts.HeavyCount),
					strings.Join(ts.Scopes, ","),
					strings.Join(ts.SampleIDs, ", "),
				})
			}
			writeAlignedTable(out, []string{
				localTr.Text("TAG", "标签"),
				localTr.Text("COUNT", "数量"),
				localTr.Text("HEAVY", "重度"),
				localTr.Text("SCOPES", "作用域"),
				localTr.Text("EXAMPLES", "示例 ID"),
			}, rows)
			return nil
		},
	}
	cmd.Flags().StringVar(&scopeFilter, "scope", "", tr.Text("filter by scope: user, system, or all", "按作用域筛选：user、system 或 all"))
	cmd.Flags().BoolVar(&showJSON, "json", false, tr.Text("output tags as JSON", "输出 JSON 格式"))
	cmd.Flags().StringVar(&configDir, "config-dir", "", tr.Text("override config directory", "覆盖默认配置目录"))
	return cmd
}
