package registry

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/resolver"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

// ValidatePaths verifies that all paths of a cleaner satisfy default safety boundaries.
// ValidatePaths 验证清理器的所有路径是否满足默认安全边界。
func ValidatePaths(c model.Cleaner, home string) error {
	return ValidatePathsWithPolicy(c, home, safety.DefaultPolicy())
}

// ValidatePathsWithPolicy verifies that all paths of a cleaner satisfy the given safety Policy.
// ValidatePathsWithPolicy 根据指定安全策略验证清理器的所有路径。
func ValidatePathsWithPolicy(c model.Cleaner, home string, policy safety.Policy) error {
	if c.Type != model.TypePaths {
		return nil
	}
	check := func(p string) error {
		expanded := expandHome(p, home)
		base := globPrefix(expanded)
		if !safety.InsideAllowListWithPolicy(base, home, policy) {
			return fmt.Errorf("cleaner %q: path %q outside allow-list", c.ID, p)
		}
		if safety.BlacklistedWithPolicy(base, home, policy) {
			return fmt.Errorf("cleaner %q: path %q in blacklist", c.ID, p)
		}
		return nil
	}
	for _, p := range c.Paths {
		if err := check(p); err != nil {
			return err
		}
	}
	for _, ref := range c.PathResolvers {
		base, err := resolver.Resolve(ref.Resolver, home)
		if err != nil {
			return fmt.Errorf("cleaner %q: %w", c.ID, err)
		}
		if err := check(base); err != nil {
			return err
		}
		for _, sp := range ref.Subpaths {
			if err := check(filepath.Join(base, sp)); err != nil {
				return err
			}
		}
	}
	return nil
}

func expandHome(p, home string) string {
	if home != "" && strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// globPrefix returns the non-glob prefix of a path, stopping at the first wildcard character.
// globPrefix 返回路径的非通配符前缀，在第一个通配符字符处停止。
func globPrefix(p string) string {
	for i, r := range p {
		if r == '*' || r == '?' || r == '[' {
			return filepath.Clean(p[:i])
		}
	}
	return filepath.Clean(p)
}
