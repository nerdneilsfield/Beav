package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/oplog"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
)

// NewHistoryCmd creates the `beav history` command for inspecting or clearing operation logs.
// NewHistoryCmd 创建 `beav history` 命令，用于查看或清空清理操作历史。
func NewHistoryCmd(tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	var limit int
	var cleanerFilter string
	var includeRotated bool
	var showJSON bool
	var clearLog bool
	var yes bool

	cmd := &cobra.Command{
		Use:     "history",
		Aliases: []string{"log", "logs"},
		GroupID: "inspect",
		Short:   tr.Text("Inspect or clear cleanup operation history", "查看或清空清理操作历史记录"),
		Example: "  beav history\n  beav history -n 50 --cleaner lang-pip\n  beav history --json\n  beav history --clear --yes",
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			home, _ := os.UserHomeDir()
			logPath := operationLogPath(home, os.Geteuid())
			if logPath == "" {
				fmt.Fprintln(cmd.OutOrStdout(), localTr.Text("Operation logging is disabled or unavailable.", "操作日志已禁用或不可用。"))
				return nil
			}
			out := cmd.OutOrStdout()

			if clearLog {
				if !yes {
					fmt.Fprintf(out, "%s", localTr.Textf("Clear operation history at %s? [y/N] ", "确认清空操作历史 %s？[y/N] ", logPath))
					line, err := bufio.NewReader(os.Stdin).ReadString('\n')
					if err != nil && line == "" {
						return nil
					}
					ans := strings.ToLower(strings.TrimSpace(line))
					if ans != "y" && ans != "yes" {
						fmt.Fprintln(out, localTr.Text("Canceled.", "已取消。"))
						return nil
					}
				}
				if err := oplog.Clear(logPath, 5); err != nil {
					return err
				}
				fmt.Fprintln(out, localTr.Text("Operation history cleared.", "操作历史已清空。"))
				return nil
			}

			entries, err := oplog.ReadEntries(logPath, includeRotated, 5)
			if err != nil {
				return err
			}
			filtered := make([]oplog.Entry, 0, len(entries))
			var totalBytes int64
			for _, ent := range entries {
				if cleanerFilter != "" && !strings.EqualFold(ent.CleanerID, cleanerFilter) && !strings.Contains(strings.ToLower(ent.CleanerID), strings.ToLower(cleanerFilter)) {
					continue
				}
				totalBytes += ent.Size
				filtered = append(filtered, ent)
			}
			totalMatched := len(filtered)
			if limit > 0 && len(filtered) > limit {
				filtered = filtered[:limit]
			}

			if showJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(map[string]any{
					"log_path":    logPath,
					"total_count": totalMatched,
					"total_bytes": totalBytes,
					"entries":     filtered,
				})
			}

			if len(filtered) == 0 {
				fmt.Fprintln(out, localTr.Text("No cleanup history recorded yet.", "暂无清理操作记录。"))
				return nil
			}

			rows := make([][]string, 0, len(filtered))
			for _, ent := range filtered {
				rows = append(rows, []string{
					ent.Timestamp.Local().Format("2006-01-02 15:04:05"),
					ent.CleanerID,
					humanize.Bytes(safeDoctorBytes(ent.Size)),
					ent.Path,
				})
			}
			writeAlignedTable(out, []string{
				localTr.Text("TIME", "时间"),
				localTr.Text("CLEANER", "清理项"),
				localTr.Text("SIZE", "大小"),
				localTr.Text("PATH", "路径"),
			}, rows)
			fmt.Fprintf(out, "\n%s\n", localTr.Textf(
				"Showing %d of %d operations (%s total freed).",
				"显示 %d / %d 条记录（累计释放 %s）。",
				len(filtered), totalMatched, humanize.Bytes(safeDoctorBytes(totalBytes)),
			))
			return nil
		},
	}

	cmd.Flags().IntVarP(&limit, "limit", "n", 30, tr.Text("maximum number of recent entries to display (0 for all)", "最多显示的最近记录条数（0 表示全部）"))
	cmd.Flags().StringVar(&cleanerFilter, "cleaner", "", tr.Text("filter entries by cleaner ID", "按清理项 ID 筛选"))
	cmd.Flags().BoolVar(&includeRotated, "all", false, tr.Text("include rotated backup log files", "包含轮转备份日志文件"))
	cmd.Flags().BoolVar(&showJSON, "json", false, tr.Text("output history as JSON", "输出 JSON 格式历史记录"))
	cmd.Flags().BoolVar(&clearLog, "clear", false, tr.Text("clear all recorded operation history", "清空所有已记录的操作历史"))
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, tr.Text("skip confirmation when clearing history", "清空历史时跳过确认"))
	return cmd
}
