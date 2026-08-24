package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ExecutorType represents the type of cleaner executor.
// ExecutorType 表示清理器执行器的类型。
type ExecutorType string

// TypePaths is a paths-based executor type.
// TypePaths 是基于路径的执行器类型。
const TypePaths ExecutorType = "paths"

// TypeCommand is a command-based executor type.
// TypeCommand 是基于命令的执行器类型。
const TypeCommand ExecutorType = "command"

// TypeToolPrune is a closed, profile-backed external tool executor.
// TypeToolPrune 是受限、按 profile 驱动的外部工具执行器。
const TypeToolPrune ExecutorType = "tool_prune"

// TypeDebris is a closed, profile-backed artifact executor.
// TypeDebris 是受限、按 profile 驱动的杂项产物执行器。
const TypeDebris ExecutorType = "debris"

// TypeManagedCache is a closed, profile-backed managed cache executor.
// TypeManagedCache 是受限、按 profile 驱动的托管缓存执行器。
const TypeManagedCache ExecutorType = "managed_cache"

// TypeJournalVacuum is a journal vacuum executor type.
// TypeJournalVacuum 是日志清理执行器类型。
const TypeJournalVacuum ExecutorType = "journal_vacuum"

// TypePkgCache is a package cache executor type.
// TypePkgCache 是包缓存清理执行器类型。
const TypePkgCache ExecutorType = "pkg_cache"

// TypeContainerPrune is a container prune executor type.
// TypeContainerPrune 是容器清理执行器类型。
const TypeContainerPrune ExecutorType = "container_prune"

// ParseExecutorType parses a string into an ExecutorType.
// ParseExecutorType 将字符串解析为 ExecutorType。
func ParseExecutorType(s string) (ExecutorType, bool) {
	switch ExecutorType(s) {
	case TypePaths, TypeJournalVacuum, TypePkgCache, TypeContainerPrune, TypeToolPrune, TypeDebris, TypeManagedCache:
		return ExecutorType(s), true
	}
	return "", false
}

// Impact describes how much user state a cleaner may affect.
// Impact 描述清理器可能影响的用户状态范围。
type Impact string

const (
	// ImpactSafe permits normal selection and execution.
	// ImpactSafe 可随普通选择及执行。
	ImpactSafe Impact = "safe"
	// ImpactHeavy requires explicit heavy-cleaner authorization.
	// ImpactHeavy 需显式授权重度清理器。
	ImpactHeavy Impact = "heavy"
)

// ParseImpact parses a configured impact value.
// ParseImpact 解析配置中的影响级别。
func ParseImpact(s string) (Impact, bool) {
	switch Impact(s) {
	case ImpactSafe, ImpactHeavy:
		return Impact(s), true
	case "":
		return ImpactSafe, true
	default:
		return "", false
	}
}

// Scope represents the scope of a cleaner.
// Scope 表示清理器的作用范围。
type Scope string

// ScopeUser is the user-level scope.
// ScopeUser 是用户级别的作用范围。
const ScopeUser Scope = "user"

// ScopeSystem is the system-level scope.
// ScopeSystem 是系统级别的作用范围。
const ScopeSystem Scope = "system"

// ScopeAll is the all-inclusive scope.
// ScopeAll 是包含所有的作用范围。
const ScopeAll Scope = "all"

// ParseScope parses a string into a Scope.
// ParseScope 将字符串解析为 Scope。
func ParseScope(s string) (Scope, bool) {
	switch Scope(s) {
	case ScopeUser, ScopeSystem:
		return Scope(s), true
	}
	return "", false
}

// TimeField represents the time field used for age calculation.
// TimeField 表示用于计算时间的字段。
type TimeField string

// TimeMtime uses modification time for age calculation.
// TimeMtime 使用修改时间进行时间计算。
const TimeMtime TimeField = "mtime"

// TimeCtime uses creation time for age calculation.
// TimeCtime 使用创建时间进行时间计算。
const TimeCtime TimeField = "ctime"

// PathResolverRef references a path resolver with optional subpaths.
// PathResolverRef 引用一个路径解析器并可选包含子路径。
// PathResolverRef references a path resolver with optional subpaths.
// PathResolverRef 引用一个路径解析器并可选包含子路径。
type PathResolverRef struct {
	// Resolver is the name of the path resolver.
	// Resolver 是路径解析器的名称。
	Resolver string `yaml:"resolver"`
	// Subpaths are the subpaths to resolve.
	// Subpaths 是要解析的子路径。
	Subpaths []string `yaml:"subpaths"`
}

// PkgCacheCfg holds package cache cleaner configuration.
// PkgCacheCfg 保存包缓存清理器的配置。
type PkgCacheCfg struct {
	// Manager is the package manager name.
	// Manager 是包管理器的名称。
	Manager string `yaml:"manager"`
}

// ContainerPruneCfg holds container prune configuration.
// ContainerPruneCfg 保存容器清理的配置。
type ContainerPruneCfg struct {
	// Runtime is the container runtime name.
	// Runtime 是容器运行时的名称。
	Runtime string `yaml:"runtime"`
	// Target is the prune target.
	// Target 是清理的目标。
	Target string `yaml:"target"`
}

// ToolPruneCfg selects one built-in external-tool profile.
// ToolPruneCfg 选择一个内置外部工具 profile。
type ToolPruneCfg struct {
	Profile string `yaml:"profile"`
}

// DebrisCfg selects one built-in debris profile.
// DebrisCfg 选择一个内置杂项产物 profile。
type DebrisCfg struct {
	Profile string `yaml:"profile"`
}

// ManagedCacheCfg selects one built-in managed-cache profile.
// ManagedCacheCfg 选择一个内置托管缓存 profile。
type ManagedCacheCfg struct {
	Profile string `yaml:"profile"`
}

// ToolPruneProfile is a compatibility name for ToolPruneCfg.
// ToolPruneProfile 是 ToolPruneCfg 的兼容名称。
type ToolPruneProfile = ToolPruneCfg

// DebrisProfile is a compatibility name for DebrisCfg.
// DebrisProfile 是 DebrisCfg 的兼容名称。
type DebrisProfile = DebrisCfg

// ManagedCacheProfile is a compatibility name for ManagedCacheCfg.
// ManagedCacheProfile 是 ManagedCacheCfg 的兼容名称。
type ManagedCacheProfile = ManagedCacheCfg

// Cleaner represents a cache cleaner configuration.
// Cleaner 表示一个缓存清理器的配置。
type Cleaner struct {
	// ID is the unique identifier for the cleaner.
	// ID 是清理器的唯一标识符。
	ID string `yaml:"id"`
	// Name is the display name of the cleaner.
	// Name 是清理器的显示名称。
	Name string `yaml:"name"`
	// Description is the detailed description of the cleaner.
	// Description 是清理器的详细描述。
	Description string `yaml:"description"`
	// Scope is the scope of the cleaner.
	// Scope 是清理器的作用范围。
	Scope Scope `yaml:"scope"`
	// Type is the executor type of the cleaner.
	// Type 是清理器的执行器类型。
	Type ExecutorType `yaml:"type"`
	// Impact is the declared impact level. Empty values default to safe.
	// Impact 是声明的影响级别；空值默认 safe。
	Impact Impact `yaml:"impact"`
	// Enabled indicates whether the cleaner is enabled.
	// Enabled 表示清理器是否已启用。
	Enabled *bool `yaml:"enabled"`
	// MinAgeDays is the minimum age in days before cleaning.
	// MinAgeDays 是清理前的最小天数。
	MinAgeDays *int `yaml:"min_age_days"`
	// NoAgeFilter indicates whether to skip age filtering.
	// NoAgeFilter 表示是否跳过时间过滤。
	NoAgeFilter bool `yaml:"no_age_filter"`
	// TimeField is the time field used for age calculation.
	// TimeField 是用于计算时间的字段。
	TimeField TimeField `yaml:"time_field"`
	// Paths are the paths to clean.
	// Paths 是要清理的路径。
	Paths []string `yaml:"paths"`
	// PathResolvers are the path resolver references.
	// PathResolvers 是路径解析器引用。
	PathResolvers []PathResolverRef `yaml:"path_resolvers"`
	// Exclude are the paths to exclude from cleaning.
	// Exclude 是要排除清理的路径。
	Exclude []string `yaml:"exclude"`
	// RunningProcesses are the processes that must not be running.
	// RunningProcesses 是必须未运行的进程。
	RunningProcesses []string `yaml:"running_processes"`
	// NeedsRoot indicates whether the cleaner requires root privileges.
	// NeedsRoot 表示清理器是否需要 root 权限。
	NeedsRoot bool `yaml:"needs_root"`
	// Tags are the tags associated with the cleaner.
	// Tags 是与清理器关联的标签。
	Tags []string `yaml:"tags"`
	// PkgCache is the package cache configuration.
	// PkgCache 是包缓存配置。
	PkgCache *PkgCacheCfg `yaml:"pkg_cache"`
	// ContainerPrune is the container prune configuration.
	// ContainerPrune 是容器清理配置。
	ContainerPrune *ContainerPruneCfg `yaml:"container_prune"`
	// ToolPrune is the closed external-tool profile configuration.
	// ToolPrune 是受限外部工具 profile 配置。
	ToolPrune *ToolPruneCfg `yaml:"tool_prune"`
	// Debris is the closed debris profile configuration.
	// Debris 是受限杂项产物 profile 配置。
	Debris *DebrisCfg `yaml:"debris"`
	// ManagedCache is the closed managed-cache profile configuration.
	// ManagedCache 是受限托管缓存 profile 配置。
	ManagedCache *ManagedCacheCfg `yaml:"managed_cache"`
}

// UnmarshalYAML preserves the safe default for legacy YAML that omits impact.
// UnmarshalYAML 为省略 impact 的旧 YAML 保留 safe 默认值。
func (c *Cleaner) UnmarshalYAML(value *yaml.Node) error {
	if err := validateCleanerYAMLFields(value); err != nil {
		return err
	}
	type plain Cleaner
	var decoded plain
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*c = Cleaner(decoded)
	if c.Impact == "" {
		c.Impact = ImpactSafe
	}
	if _, validImpact := ParseImpact(string(c.Impact)); validImpact && ProfileImpactFloor(c.Type, c.Profile()) == ImpactHeavy {
		c.Impact = ImpactHeavy
	}
	return nil
}

func validateCleanerYAMLFields(value *yaml.Node) error {
	if err := validateYAMLAliasGraph(value, map[*yaml.Node]bool{}); err != nil {
		return err
	}
	resolved, err := resolveYAMLAlias(value, map[*yaml.Node]bool{})
	if err != nil {
		return err
	}
	if resolved.Kind != yaml.MappingNode {
		return nil
	}
	allowed := map[string]struct{}{
		"id": {}, "name": {}, "description": {}, "scope": {}, "type": {}, "impact": {},
		"enabled": {}, "min_age_days": {}, "no_age_filter": {}, "time_field": {},
		"paths": {}, "path_resolvers": {}, "exclude": {}, "running_processes": {},
		"needs_root": {}, "tags": {}, "pkg_cache": {}, "container_prune": {},
		"tool_prune": {}, "debris": {}, "managed_cache": {},
	}
	for i := 0; i+1 < len(resolved.Content); i += 2 {
		keyNode, err := resolveYAMLAlias(resolved.Content[i], map[*yaml.Node]bool{})
		if err != nil {
			return err
		}
		key := keyNode.Value
		nested, err := resolveYAMLAlias(resolved.Content[i+1], map[*yaml.Node]bool{})
		if err != nil {
			return err
		}
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("field %s not found in type model.Cleaner", key)
		}
		var nestedAllowed map[string]struct{}
		switch key {
		case "pkg_cache":
			nestedAllowed = map[string]struct{}{"manager": {}}
		case "container_prune":
			nestedAllowed = map[string]struct{}{"runtime": {}, "target": {}}
		case "tool_prune", "debris", "managed_cache":
			nestedAllowed = map[string]struct{}{"profile": {}}
		}
		if nestedAllowed != nil && nested.Kind == yaml.MappingNode {
			for j := 0; j+1 < len(nested.Content); j += 2 {
				if _, ok := nestedAllowed[nested.Content[j].Value]; !ok {
					return fmt.Errorf("field %s not found in nested cleaner profile", nested.Content[j].Value)
				}
			}
		}
	}
	return nil
}

func validateYAMLAliasGraph(node *yaml.Node, stack map[*yaml.Node]bool) error {
	if node == nil {
		return fmt.Errorf("malformed YAML node")
	}
	if stack[node] {
		return fmt.Errorf("cyclic YAML alias")
	}
	stack[node] = true
	defer delete(stack, node)
	if node.Kind == yaml.AliasNode {
		if node.Alias == nil {
			return fmt.Errorf("malformed YAML alias")
		}
		return validateYAMLAliasGraph(node.Alias, stack)
	}
	for _, child := range node.Content {
		if err := validateYAMLAliasGraph(child, stack); err != nil {
			return err
		}
	}
	return nil
}

func resolveYAMLAlias(node *yaml.Node, seen map[*yaml.Node]bool) (*yaml.Node, error) {
	if node == nil {
		return nil, fmt.Errorf("malformed YAML node")
	}
	if node.Kind != yaml.AliasNode {
		return node, nil
	}
	if node.Alias == nil {
		return nil, fmt.Errorf("malformed YAML alias")
	}
	if seen[node] {
		return nil, fmt.Errorf("cyclic YAML alias")
	}
	seen[node] = true
	resolved, err := resolveYAMLAlias(node.Alias, seen)
	delete(seen, node)
	return resolved, err
}

// IsEnabled returns true if the cleaner is enabled.
// IsEnabled 返回清理器是否已启用。
func (c Cleaner) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// AgeOrDefault returns the minimum age days or a default value.
// AgeOrDefault 返回最小天数或默认值。
func (c Cleaner) AgeOrDefault(def int) int {
	if c.MinAgeDays == nil {
		return def
	}
	return *c.MinAgeDays
}

// ImpactOrDefault returns the declared impact, defaulting omitted YAML to safe.
// ImpactOrDefault 返回声明的影响级别，省略 YAML 时默认 safe。
func (c Cleaner) ImpactOrDefault() Impact {
	if c.Impact == "" {
		return ImpactSafe
	}
	return c.Impact
}

// EffectiveImpact applies the minimum impact required by the selected profile.
// EffectiveImpact 应用所选 profile 要求的最低影响级别。
func (c Cleaner) EffectiveImpact() Impact {
	declared := c.ImpactOrDefault()
	if ProfileImpactFloor(c.Type, c.Profile()) == ImpactHeavy {
		return ImpactHeavy
	}
	return declared
}

// ImpactFloor returns the minimum impact enforced by c's profile.
// ImpactFloor 返回 c 的 profile 强制的最低影响级别。
func (c Cleaner) ImpactFloor() Impact {
	return ProfileImpactFloor(c.Type, c.Profile())
}

// Profile returns the nested closed-profile name for c.
// Profile 返回 c 的嵌套受限 profile 名称。
func (c Cleaner) Profile() string {
	switch c.Type {
	case TypeToolPrune:
		if c.ToolPrune != nil {
			return c.ToolPrune.Profile
		}
	case TypeDebris:
		if c.Debris != nil {
			return c.Debris.Profile
		}
	case TypeManagedCache:
		if c.ManagedCache != nil {
			return c.ManagedCache.Profile
		}
	}
	return ""
}

// ProfileImpactFloor reports the minimum impact required by a closed profile.
// ProfileImpactFloor 返回受限 profile 要求的最低影响级别。
func ProfileImpactFloor(t ExecutorType, profile string) Impact {
	if t == TypeManagedCache {
		if _, ok := managedCacheProfiles[profile]; ok {
			return ImpactHeavy
		}
	}
	if t == TypeToolPrune {
		if impact, ok := toolPruneProfiles[profile]; ok {
			return impact
		}
	}
	return ImpactSafe
}

// KnownProfile reports whether profile is in the closed set for executor type t.
// KnownProfile 报告 profile 是否属于执行器类型 t 的受限集合。
func KnownProfile(t ExecutorType, profile string) bool {
	switch t {
	case TypeToolPrune:
		_, ok := toolPruneProfiles[profile]
		return ok
	case TypeDebris:
		_, ok := debrisProfiles[profile]
		return ok
	case TypeManagedCache:
		_, ok := managedCacheProfiles[profile]
		return ok
	default:
		return false
	}
}

var toolPruneProfiles = map[string]Impact{
	"pkg-homebrew":             ImpactSafe,
	"lang-uv":                  ImpactSafe,
	"lang-pnpm":                ImpactSafe,
	"dev-pre-commit":           ImpactSafe,
	"lang-conda":               ImpactSafe,
	"test-cypress":             ImpactSafe,
	"build-ccache":             ImpactSafe,
	"ai-huggingface-prune":     ImpactHeavy,
	"dev-mise":                 ImpactHeavy,
	"lang-corepack":            ImpactHeavy,
	"sys-nix-gc":               ImpactHeavy,
	"desktop-flatpak-user":     ImpactHeavy,
	"desktop-flatpak-system":   ImpactHeavy,
	"lang-poetry-cache":        ImpactHeavy,
	"test-playwright-browsers": ImpactHeavy,
}

var debrisProfiles = map[string]struct{}{
	"desktop-download-partials": {},
	"ai-huggingface-incomplete": {},
	"python-ruff-cache":         {},
	"python-mypy-cache":         {},
	"python-pyinstaller-cache":  {},
	"gpu-nvidia-compute-cache":  {},
	"gpu-triton-cache":          {},
	"gpu-torch-inductor-cache":  {},
	"cloud-gcloud-logs":         {},
	"cloud-azure-logs":          {},
}

var managedCacheProfiles = map[string]struct{}{
	"ai-huggingface-datasets-arrow": {},
	"ai-huggingface-datasets":       {},
	"ai-torch-hub":                  {},
	"ai-torch-hub-checkpoints":      {},
}
