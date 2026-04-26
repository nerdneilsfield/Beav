package safety

import (
	"path/filepath"
	"strings"
)

// systemRoots contains the default system directories that are allowed to be cleaned.
// systemRoots 包含允许清理的默认系统目录。
var systemRoots = []string{"/var/cache", "/var/log", "/tmp", "/var/tmp"}

// InsideAllowList checks whether a path falls within an allowed cleaning scope.
// InsideAllowList 检查路径是否在允许的清理范围内。
func InsideAllowList(path, home string) bool {
	clean := filepath.Clean(path)
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
	return strings.HasPrefix(p, prefix+string(filepath.Separator))
}
