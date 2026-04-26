package safety

import (
	"path/filepath"
	"strings"
)

type Whitelist struct {
	prefixes []string
}

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

func (w *Whitelist) Match(path string) bool {
	clean := filepath.Clean(path)
	for _, p := range w.prefixes {
		if clean == p || strings.HasPrefix(clean, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
