package safety

import (
	"path/filepath"
	"strings"
)

// systemRoots contains the default system directories that are allowed to be cleaned.
// systemRoots 包含允许清理的默认系统目录。
var systemRoots = []string{"/var/cache", "/var/log", "/tmp", "/var/tmp"}

// DefaultAllowRoots returns the default allowed system root prefixes.
// DefaultAllowRoots 返回默认允许清理的系统根路径列表。
func DefaultAllowRoots() []string {
	out := make([]string, len(systemRoots))
	copy(out, systemRoots)
	return out
}

// InsideAllowList checks whether a path falls within the default allowed cleaning scope.
// InsideAllowList 检查路径是否在默认允许的清理范围内。
func InsideAllowList(path, home string) bool {
	return InsideAllowListWithPolicy(path, home, DefaultPolicy())
}

// InsideAllowListWithPolicy checks whether path is permitted by policy's allow-list configuration.
// InsideAllowListWithPolicy 根据给定 Policy 的 allow-list 配置检查路径是否被允许。
func InsideAllowListWithPolicy(path, home string, policy Policy) bool {
	clean := filepath.Clean(path)
	if clean == "/" {
		return false
	}
	if policy.EffectiveDisableAllowList() {
		return true
	}
	for _, entry := range policy.AllowList {
		exp := ExpandPath(entry, home)
		if exp == "" {
			continue
		}
		if hasPrefix(clean, exp) {
			return true
		}
	}
	if policy.ReplaceAllowList {
		return false
	}
	if home != "" && hasPrefix(clean, filepath.Clean(home)) {
		return true
	}
	for _, root := range systemRoots {
		if hasPrefix(clean, root) {
			return true
		}
	}
	return false
}

func hasPrefix(p, prefix string) bool {
	if p == prefix {
		return true
	}
	if prefix == "/" {
		return filepath.IsAbs(p)
	}
	return strings.HasPrefix(p, prefix+string(filepath.Separator))
}
