package safety

import (
	"path/filepath"
	"strings"
)

// Policy controls configurable safety boundaries and bypass overrides.
// Policy 控制可配置的安全边界与绕过选项。
type Policy struct {
	// BypassAll disables all optional safety checks (except protecting literal "/").
	// BypassAll 禁用除保护根目录 "/" 之外的所有安全限制。
	BypassAll bool
	// AllowList holds extra allowed root path prefixes.
	// AllowList 保存额外允许清理的路径前缀。
	AllowList []string
	// ReplaceAllowList replaces the default allow-list with AllowList when true.
	// ReplaceAllowList 为 true 时使用 AllowList 替代默认允许列表。
	ReplaceAllowList bool
	// DisableAllowList skips allow-list checks completely.
	// DisableAllowList 完全跳过允许列表检查。
	DisableAllowList bool
	// Blacklist holds extra protected path prefixes.
	// Blacklist 保存额外保护的黑名单路径前缀。
	Blacklist []string
	// ReplaceBlacklist replaces the default blacklist with Blacklist when true.
	// ReplaceBlacklist 为 true 时使用 Blacklist 替代默认黑名单。
	ReplaceBlacklist bool
	// DisableBlacklist skips all blacklist checks (except literal "/").
	// DisableBlacklist 跳过除根目录 "/" 外的全部黑名单检查。
	DisableBlacklist bool
	// IgnoreRunningProcesses skips running process, busy lock, and open-FD checks.
	// IgnoreRunningProcesses 跳过运行中进程、锁文件与打开文件描述符检查。
	IgnoreRunningProcesses bool
	// BypassAgeFloor disables profile-level minimum age floors and allows min_age_days=0.
	// BypassAgeFloor 禁用内置清理策略的最小天数下限，允许按 0 天过滤或不过滤。
	BypassAgeFloor bool
	// FollowSymlinks allows cleaning through symbolic links and symlinked roots.
	// FollowSymlinks 允许遍历及清理符号链接和符号链接根目录。
	FollowSymlinks bool
	// AllowCrossFS allows crossing filesystem and mount boundaries during walks.
	// AllowCrossFS 允许在扫描与清理时跨越文件系统或挂载点边界。
	AllowCrossFS bool
	// AllowRootHome allows cleaning /root when running as root.
	// AllowRootHome 允许以 root 运行时清理 /root 目录。
	AllowRootHome bool
	// AllowCommandCleaners allows custom YAML cleaners with type: command.
	// AllowCommandCleaners 允许执行自定义 YAML 中的 type: command 清理器。
	AllowCommandCleaners bool
}

// DefaultPolicy returns the lightweight default safety policy.
// DefaultPolicy 返回轻量默认安全策略。
func DefaultPolicy() Policy {
	return Policy{}
}

// EffectiveDisableAllowList reports whether allow-list filtering is disabled.
// EffectiveDisableAllowList 报告是否禁用了允许列表检查。
func (p Policy) EffectiveDisableAllowList() bool {
	return p.BypassAll || p.DisableAllowList
}

// EffectiveDisableBlacklist reports whether blacklist filtering is disabled.
// EffectiveDisableBlacklist 报告是否禁用了黑名单检查。
func (p Policy) EffectiveDisableBlacklist() bool {
	return p.BypassAll || p.DisableBlacklist
}

// EffectiveIgnoreRunning reports whether running-process/busy checks are skipped.
// EffectiveIgnoreRunning 报告是否跳过运行中进程与占用检查。
func (p Policy) EffectiveIgnoreRunning() bool {
	return p.BypassAll || p.IgnoreRunningProcesses
}

// EffectiveBypassAgeFloor reports whether profile minimum age floors are bypassed.
// EffectiveBypassAgeFloor 报告是否绕过内置策略的最小留存天数下限。
func (p Policy) EffectiveBypassAgeFloor() bool {
	return p.BypassAll || p.BypassAgeFloor
}

// EffectiveFollowSymlinks reports whether symlinks are allowed.
// EffectiveFollowSymlinks 报告是否允许符号链接。
func (p Policy) EffectiveFollowSymlinks() bool {
	return p.BypassAll || p.FollowSymlinks
}

// EffectiveAllowCrossFS reports whether cross-filesystem traversal is allowed.
// EffectiveAllowCrossFS 报告是否允许跨文件系统遍历。
func (p Policy) EffectiveAllowCrossFS() bool {
	return p.BypassAll || p.AllowCrossFS
}

// EffectiveAllowRootHome reports whether /root cleaning is permitted.
// EffectiveAllowRootHome 报告是否允许清理 /root。
func (p Policy) EffectiveAllowRootHome() bool {
	return p.BypassAll || p.AllowRootHome
}

// EffectiveAllowCommandCleaners reports whether custom command cleaners are permitted.
// EffectiveAllowCommandCleaners 报告是否允许执行自定义 command 类型清理器。
func (p Policy) EffectiveAllowCommandCleaners() bool {
	return p.BypassAll || p.AllowCommandCleaners
}

// ExpandPath expands leading ~/ against home and cleans the resulting path.
// ExpandPath 将开头的 ~/ 按主目录展开并规范化路径。
func ExpandPath(raw, home string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if home != "" && (trimmed == "~" || strings.HasPrefix(trimmed, "~/")) {
		if trimmed == "~" {
			return filepath.Clean(home)
		}
		return filepath.Clean(filepath.Join(home, trimmed[2:]))
	}
	return filepath.Clean(trimmed)
}
