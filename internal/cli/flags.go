// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

// CleanFlags holds the command-line flags for the clean command.
// CleanFlags 保存 clean 命令的命令行标志。
type CleanFlags struct {
	// System enables cleaning of system-scope cleaners.
	// System 启用系统范围清理器的清理。
	System          bool
	// All enables cleaning of both user and system scopes.
	// All 启用用户和系统范围的清理。
	All             bool
	// DryRun shows what would be removed without actually deleting.
	// DryRun 显示将被删除的内容但不实际删除。
	DryRun          bool
	// Only runs only matching cleaner IDs, tags, or ID prefixes.
	// Only 仅运行匹配的清理器 ID、标签或 ID 前缀。
	Only            []string
	// Skip skips matching cleaner IDs, tags, or ID prefixes.
	// Skip 跳过匹配的清理器 ID、标签或 ID 前缀。
	Skip            []string
	// MinAge overrides the minimum age for files to be cleaned.
	// MinAge 覆盖要被清理文件的最小年龄。
	MinAge          string
	// ForceNoAge allows cleaners that explicitly have no age filter.
	// ForceNoAge 允许明确没有年龄过滤器的清理器。
	ForceNoAge      bool
	// Output sets the output mode: spinner, plain, or json.
	// Output 设置输出模式：spinner、plain 或 json。
	Output          string
	// Yes skips interactive confirmations.
	// Yes 跳过交互式确认。
	Yes             bool
	// AllowRootHome allows cleaning /root when running as root.
	// AllowRootHome 允许以 root 运行时清理 /root。
	AllowRootHome   bool
	// UserOverride specifies the target user for --all flag.
	// UserOverride 指定 --all 标志的目标用户。
	UserOverride    string
	// ConfigDir overrides the default config directory.
	// ConfigDir 覆盖默认配置目录。
	ConfigDir       string
	// BuiltinDisabled skips embedded cleaners when true.
	// BuiltinDisabled 为 true 时跳过内置清理器。
	BuiltinDisabled bool
}
