package safety

import (
	"path/filepath"
	"strings"
)

var systemRoots = []string{"/var/cache", "/var/log", "/tmp", "/var/tmp"}

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
