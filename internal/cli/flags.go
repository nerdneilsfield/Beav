// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"github.com/dengqi/beav/internal/i18n"
	"github.com/spf13/cobra"
)

// CleanFlags holds the command-line flags for the clean, scan, purge, and TUI commands.
// CleanFlags 保存 clean、scan、purge 与交互命令的命令行标志。
type CleanFlags struct {
	// System enables cleaning of system-scope cleaners.
	// System 启用系统范围清理器的清理。
	System bool
	// All enables cleaning of both user and system scopes.
	// All 启用用户和系统范围的清理。
	All bool
	// DryRun shows what would be removed without actually deleting.
	// DryRun 显示将被删除的内容但不实际删除。
	DryRun bool
	// Only runs only matching cleaner IDs, tags, or ID prefixes.
	// Only 仅运行匹配的清理器 ID、标签或 ID 前缀。
	Only []string
	// IncludeHeavy authorizes heavy-impact cleaners.
	// IncludeHeavy 授权重度影响清理器。
	IncludeHeavy bool
	// Skip skips matching cleaner IDs, tags, or ID prefixes.
	// Skip 跳过匹配的清理器 ID、标签或 ID 前缀。
	Skip []string
	// MinAge overrides the minimum age for files to be cleaned.
	// MinAge 覆盖要被清理文件的最小年龄。
	MinAge string
	// ForceNoAge allows cleaners that explicitly have no age filter.
	// ForceNoAge 允许明确没有年龄过滤器的清理器。
	ForceNoAge bool
	// Output sets the output mode: spinner, plain, or json.
	// Output 设置输出模式：spinner、plain 或 json。
	Output string
	// Yes skips interactive confirmations.
	// Yes 跳过交互式确认。
	Yes bool
	// Verbose prints candidate/deleted file samples and full skip details.
	// Verbose 输出候选/已删文件样本及完整跳过详情。
	Verbose bool
	// Quiet suppresses non-essential output.
	// Quiet 仅输出关键汇总或错误信息。
	Quiet bool
	// NoColor disables ANSI colors in CLI output.
	// NoColor 禁用命令行输出中的 ANSI 颜色。
	NoColor bool
	// Lang overrides the UI language (auto, en, or zh).
	// Lang 覆盖界面显示语言（auto、en 或 zh）。
	Lang string
	// AllowRootHome allows cleaning /root when running as root.
	// AllowRootHome 允许以 root 运行时清理 /root。
	AllowRootHome bool
	// Unsafe enables full bypass mode for all configurable safety checks.
	// Unsafe 启用全绕过模式，跳过所有可配置安全限制。
	Unsafe bool
	// AllowPaths appends custom allowed root prefixes for this run.
	// AllowPaths 为本次运行追加允许清理的路径前缀。
	AllowPaths []string
	// IgnoreRunning skips running-process, lock, and open-FD checks.
	// IgnoreRunning 跳过运行中进程、锁与打开文件描述符检查。
	IgnoreRunning bool
	// BypassBlacklist skips blacklist path checks (except literal "/").
	// BypassBlacklist 跳过除根目录 "/" 之外的黑名单检查。
	BypassBlacklist bool
	// BypassAllowList skips allow-list path checks.
	// BypassAllowList 跳过允许列表路径检查。
	BypassAllowList bool
	// BypassAgeFloor disables profile-level minimum age floors.
	// BypassAgeFloor 禁用内置策略的最小留存天数下限。
	BypassAgeFloor bool
	// FollowSymlinks allows following symbolic links.
	// FollowSymlinks 允许遍历并处理符号链接。
	FollowSymlinks bool
	// AllowCrossFS allows crossing filesystem/mount boundaries.
	// AllowCrossFS 允许跨文件系统与挂载点边界操作。
	AllowCrossFS bool
	// UserOverride specifies the target user for --all flag.
	// UserOverride 指定 --all 标志的目标用户。
	UserOverride string
	// ConfigDir overrides the default config directory.
	// ConfigDir 覆盖默认配置目录。
	ConfigDir string
	// BuiltinDisabled skips embedded cleaners when true.
	// BuiltinDisabled 为 true 时跳过内置清理器。
	BuiltinDisabled bool
}

func addScopeAndPolicyFlags(cmd *cobra.Command, f *CleanFlags, tr *i18n.Translator) {
	cmd.Flags().StringVar(&f.MinAge, "min-age", "", tr.Text("override min age, for example 14d or tag=3d", "覆盖最小保留天数，如 14d 或 langs=3d"))
	cmd.Flags().BoolVar(&f.AllowRootHome, "allow-root-home", false, tr.Text("allow cleaning /root when running as root", "以 root 运行时允许清理 /root 目录"))
	cmd.Flags().BoolVar(&f.Unsafe, "unsafe", false, tr.Text("bypass all configurable safety restrictions (heavy, age floors, blacklist, allow-list, running checks)", "绕过所有可配置安全限制（含重度项、年龄下限、黑名单、白/允许列表、进程检查等）"))
	cmd.Flags().BoolVar(&f.Unsafe, "force", false, tr.Text("alias for --unsafe", "--unsafe 的别名"))
	cmd.Flags().StringSliceVar(&f.AllowPaths, "allow-path", nil, tr.Text("extra allowed root paths for this run", "本次运行额外允许清理的根路径"))
	cmd.Flags().BoolVar(&f.IgnoreRunning, "ignore-running", false, tr.Text("skip running-process and busy-cache guards", "跳过运行中进程与缓存占用检查"))
	cmd.Flags().BoolVar(&f.BypassBlacklist, "bypass-blacklist", false, tr.Text("skip protected-path blacklist checks (except /)", "绕过黑名单保护检查（仍保护根目录 /）"))
	cmd.Flags().BoolVar(&f.BypassAllowList, "bypass-allow-list", false, tr.Text("skip allowed-root boundary checks", "绕过允许路径边界检查"))
	cmd.Flags().BoolVar(&f.BypassAgeFloor, "bypass-age-floor", false, tr.Text("ignore built-in profile minimum age floors", "绕过内置策略的最小留存天数下限"))
	cmd.Flags().BoolVar(&f.FollowSymlinks, "follow-symlinks", false, tr.Text("allow symbolic links during traversal and command lookup", "允许遍历符号链接与符号链接命令"))
	cmd.Flags().BoolVar(&f.AllowCrossFS, "allow-cross-fs", false, tr.Text("allow crossing mount/filesystem boundaries", "允许跨文件系统与挂载点边界"))
	cmd.Flags().StringVar(&f.UserOverride, "user", "", tr.Text("target user for --all", "配合 --all 指定目标用户名"))
	cmd.Flags().StringVar(&f.ConfigDir, "config-dir", "", tr.Text("override config directory", "覆盖默认配置目录"))
	cmd.Flags().BoolVar(&f.BuiltinDisabled, "builtin-disabled", false, "skip embedded cleaners")
	_ = cmd.Flags().MarkHidden("builtin-disabled")
}
