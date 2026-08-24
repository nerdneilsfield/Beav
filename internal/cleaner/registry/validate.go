package registry

import (
	"fmt"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func Validate(c model.Cleaner) error {
	if c.ID == "" {
		return fmt.Errorf("cleaner missing id")
	}
	if _, ok := model.ParseExecutorType(string(c.Type)); !ok {
		return fmt.Errorf("cleaner %q: invalid type %q", c.ID, c.Type)
	}
	if _, ok := model.ParseScope(string(c.Scope)); !ok {
		return fmt.Errorf("cleaner %q: invalid scope %q", c.ID, c.Scope)
	}
	if c.Impact != "" {
		if _, ok := model.ParseImpact(string(c.Impact)); !ok {
			return fmt.Errorf("cleaner %q: invalid impact %q", c.ID, c.Impact)
		}
	}
	switch c.Type {
	case model.TypePaths:
		if len(c.Paths) == 0 && len(c.PathResolvers) == 0 {
			return fmt.Errorf("cleaner %q: paths type requires paths or path_resolvers", c.ID)
		}
	case model.TypePkgCache:
		if c.PkgCache == nil || c.PkgCache.Manager == "" {
			return fmt.Errorf("cleaner %q: pkg_cache type requires pkg_cache.manager", c.ID)
		}
	case model.TypeContainerPrune:
		if c.ContainerPrune == nil {
			return fmt.Errorf("cleaner %q: container_prune type requires container_prune block", c.ID)
		}
		if !validContainerTarget(c.ContainerPrune.Runtime, c.ContainerPrune.Target) {
			return fmt.Errorf("cleaner %q: unsupported container_prune runtime/target %q/%q", c.ID, c.ContainerPrune.Runtime, c.ContainerPrune.Target)
		}
		if c.MinAgeDays == nil {
			return fmt.Errorf("cleaner %q: container_prune requires min_age_days", c.ID)
		}
	case model.TypeToolPrune:
		if c.ToolPrune == nil || c.ToolPrune.Profile == "" {
			return fmt.Errorf("cleaner %q: tool_prune type requires tool_prune.profile", c.ID)
		}
		if !model.KnownProfile(model.TypeToolPrune, c.ToolPrune.Profile) {
			return fmt.Errorf("cleaner %q: unknown tool_prune profile %q", c.ID, c.ToolPrune.Profile)
		}
	case model.TypeDebris:
		if c.Debris == nil || c.Debris.Profile == "" {
			return fmt.Errorf("cleaner %q: debris type requires debris.profile", c.ID)
		}
		if !model.KnownProfile(model.TypeDebris, c.Debris.Profile) {
			return fmt.Errorf("cleaner %q: unknown debris profile %q", c.ID, c.Debris.Profile)
		}
	case model.TypeManagedCache:
		if c.ManagedCache == nil || c.ManagedCache.Profile == "" {
			return fmt.Errorf("cleaner %q: managed_cache type requires managed_cache.profile", c.ID)
		}
		if !model.KnownProfile(model.TypeManagedCache, c.ManagedCache.Profile) {
			return fmt.Errorf("cleaner %q: unknown managed_cache profile %q", c.ID, c.ManagedCache.Profile)
		}
	}
	return nil
}

// validContainerTarget checks if a container prune runtime/target combination is supported.
// validContainerTarget 检查容器修剪运行时/目标组合是否受支持。
func validContainerTarget(runtime, target string) bool {
	switch runtime {
	case "docker":
		switch target {
		case "builder", "container", "image", "network":
			return true
		}
	case "podman":
		switch target {
		case "builder", "container", "image", "network", "system":
			return true
		}
	}
	return false
}
