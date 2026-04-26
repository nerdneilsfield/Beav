package resolver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Resolver func(home string) string

var resolvers = map[string]Resolver{
	"xdg_cache":        xdg("XDG_CACHE_HOME", ".cache"),
	"xdg_state":        xdg("XDG_STATE_HOME", ".local/state"),
	"xdg_data":         xdg("XDG_DATA_HOME", ".local/share"),
	"npm_cache":        cmdResolver([]string{"npm", "config", "get", "cache"}, "$HOME/.npm"),
	"pnpm_store":       cmdResolver([]string{"pnpm", "store", "path"}, "$HOME/.local/share/pnpm/store"),
	"yarn_cache":       cmdResolver([]string{"yarn", "cache", "dir"}, "$HOME/.cache/yarn"),
	"bun_cache":        envResolver("BUN_INSTALL_CACHE_DIR", "$HOME/.bun/install/cache"),
	"pip_cache":        cmdResolver([]string{"pip", "cache", "dir"}, "$HOME/.cache/pip"),
	"cargo_home":       envResolver("CARGO_HOME", "$HOME/.cargo"),
	"gocache":          cmdResolver([]string{"go", "env", "GOCACHE"}, "$HOME/.cache/go-build"),
	"gradle_home":      envResolver("GRADLE_USER_HOME", "$HOME/.gradle"),
	"maven_local_repo": cmdResolver([]string{"mvn", "help:evaluate", "-Dexpression=settings.localRepository", "-q", "-DforceStdout"}, "$HOME/.m2/repository"),
}

// Resolve looks up a resolver by name and returns the resolved absolute path.
// Resolve 按名称查找解析器并返回解析后的绝对路径。
func Resolve(name, home string) (string, error) {
	r, ok := resolvers[name]
	if !ok {
		return "", fmt.Errorf("unknown resolver %q", name)
	}
	out := filepath.Clean(r(home))
	if !filepath.IsAbs(out) {
		out = filepath.Clean(filepath.Join(home, out))
	}
	return out, nil
}

func MustResolve(name, home string) string {
	out, err := Resolve(name, home)
	if err != nil {
		panic(err)
	}
	return out
}

func xdg(envVar, fallbackRel string) Resolver {
	return func(home string) string {
		if v := os.Getenv(envVar); v != "" {
			return v
		}
		return filepath.Join(home, fallbackRel)
	}
}

func envResolver(envVar, fallback string) Resolver {
	return func(home string) string {
		if v := os.Getenv(envVar); v != "" {
			return v
		}
		return expandFallback(fallback, home)
	}
}

func cmdResolver(argv []string, fallback string) Resolver {
	return func(home string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, argv[0], argv[1:]...) // #nosec G204 -- resolver argv comes from the closed built-in resolver table.
		out, err := c.Output()
		if err == nil {
			s := strings.TrimSpace(string(out))
			if filepath.IsAbs(s) {
				return s
			}
		}
		return expandFallback(fallback, home)
	}
}

func expandFallback(fallback, home string) string {
	out := os.Expand(fallback, func(k string) string {
		if k == "HOME" {
			return home
		}
		return os.Getenv(k)
	})
	if !filepath.IsAbs(out) {
		out = filepath.Join(home, out)
	}
	return out
}
