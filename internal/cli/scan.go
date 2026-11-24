package cli

import (
	"github.com/dengqi/beav/internal/i18n"
	"github.com/spf13/cobra"
)

// NewScanCmd creates a strictly read-only preview command that reports reclaimable space.
// NewScanCmd 创建一个严格只读的扫描预览命令，用于查看可回收空间。
func NewScanCmd(tr *i18n.Translator) *cobra.Command {
	var f CleanFlags
	cmd := &cobra.Command{
		Use:     "scan",
		GroupID: "core",
		Short:   tr.Text("Scan and preview reclaimable caches without deleting anything", "只读扫描并预览可清理缓存（绝不删除文件）"),
		Long: tr.Text(
			"Scan caches in read-only mode and display estimated reclaimable sizes.\n"+
				"Nothing is modified or deleted. Run `beav list` to inspect all cleaner IDs and tags.",
			"以严格只读模式扫描可清理缓存并估算可释放空间，不会修改或删除任何文件。\n"+
				"如需查看全部清理项 ID 与标签，请运行 `beav list`。",
		),
		Example: tr.Text(
			"  beav scan\n"+
				"  beav scan --only langs,browser\n"+
				"  beav scan --include-heavy --verbose\n"+
				"  sudo beav scan --system",
			"  beav scan\n"+
				"  beav scan --only langs,browser\n"+
				"  beav scan --include-heavy --verbose\n"+
				"  sudo beav scan --system",
		),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			applyGlobalFlags(cmd, &f)
			f.DryRun = true
			return runCleanWithTranslator(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), f, localTr)
		},
	}
	cmd.Flags().BoolVar(&f.System, "system", false, tr.Text("scan system-scope cleaners (requires sudo)", "扫描系统级清理项（需 sudo）"))
	cmd.Flags().BoolVar(&f.All, "all", false, tr.Text("scan both user and system scopes (requires sudo)", "同时扫描用户级与系统级清理项（需 sudo）"))
	cmd.Flags().StringSliceVar(&f.Only, "only", nil, tr.Text("scan matching cleaner IDs, tags, or ID prefixes; heavy requires an exact full ID or tag", "仅扫描匹配的清理项 ID、标签或 ID 前缀；重度清理项需完整 ID 或标签"))
	cmd.Flags().BoolVar(&f.IncludeHeavy, "include-heavy", false, tr.Text("include heavy cleaners in scan", "在扫描中包含重度清理项"))
	cmd.Flags().StringSliceVar(&f.Skip, "skip", nil, tr.Text("skip matching cleaner IDs, tags, or ID prefixes", "跳过匹配的清理项 ID、标签或 ID 前缀"))
	cmd.Flags().BoolVar(&f.ForceNoAge, "force-no-age", false, tr.Text("allow cleaners that explicitly have no age filter", "允许执行未设置保留天数过滤的清理项"))
	cmd.Flags().StringVar(&f.Output, "output", "", tr.Text("output mode: spinner, plain, or json", "输出格式：spinner、plain 或 json"))
	cmd.Flags().BoolVarP(&f.Verbose, "verbose", "v", false, tr.Text("show candidate file samples and all skipped cleaners", "显示候选文件样本与全部跳过项明细"))
	addScopeAndPolicyFlags(cmd, &f, tr)
	return cmd
}
