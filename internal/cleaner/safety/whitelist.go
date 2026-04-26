package safety

import (
	"path/filepath"
	"strings"
)

// Whitelist holds a set of path prefixes that are explicitly allowed for cleaning.
// Whitelist 保存一组明确允许清理的路径前缀。
type Whitelist struct {
	prefixes []string
}

// NewWhitelist creates a new Whitelist from a slice of path prefixes.
// NewWhitelist 从路径前缀切片创建一个新的 Whitelist。
func NewWhitelist(prefixes []string) *Whitelist {
	cleaned := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		if p == "" {
			continue
		}
		cleaned = append(cleaned, filepath.Clean(p))
	}
	return &Whitelist{prefixes: cleaned}
}

// Match returns true if the given path matches any prefix in the whitelist.
// Match 如果给定路径与白名单中的任何前缀匹配，则返回 true。
func (w *Whitelist) Match(path string) bool {
	clean := filepath.Clean(path)
	for _, p := range w.prefixes {
		if clean == p || strings.HasPrefix(clean, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
