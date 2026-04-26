package registry

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/resolver"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

func ValidatePaths(c model.Cleaner, home string) error {
	if c.Type != model.TypePaths {
		return nil
	}
	check := func(p string) error {
		expanded := expandHome(p, home)
		base := globPrefix(expanded)
		if !safety.InsideAllowList(base, home) {
			return fmt.Errorf("cleaner %q: path %q outside allow-list", c.ID, p)
		}
		if safety.Blacklisted(base, home) {
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

func globPrefix(p string) string {
	for i, r := range p {
		if r == '*' || r == '?' || r == '[' {
			return filepath.Clean(p[:i])
		}
	}
	return filepath.Clean(p)
}
