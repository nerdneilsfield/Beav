package model

// ExecutorType represents the type of cleaner executor.
// ExecutorType 表示清理器执行器的类型。
type ExecutorType string

// TypePaths is a paths-based executor type.
// TypePaths 是基于路径的执行器类型。
const TypePaths ExecutorType = "paths"

// TypeCommand is a command-based executor type.
// TypeCommand 是基于命令的执行器类型。
const TypeCommand ExecutorType = "command"

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
	case TypePaths, TypeCommand, TypeJournalVacuum, TypePkgCache, TypeContainerPrune:
		return ExecutorType(s), true
	}
	return "", false
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
	Resolver string   `yaml:"resolver"`
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
	Target  string `yaml:"target"`
}

// Cleaner represents a cache cleaner configuration.
// Cleaner 表示一个缓存清理器的配置。
type Cleaner struct {
	// ID is the unique identifier for the cleaner.
	// ID 是清理器的唯一标识符。
	ID               string             `yaml:"id"`
	// Name is the display name of the cleaner.
	// Name 是清理器的显示名称。
	Name             string             `yaml:"name"`
	// Description is the detailed description of the cleaner.
	// Description 是清理器的详细描述。
	Description      string             `yaml:"description"`
	// Scope is the scope of the cleaner.
	// Scope 是清理器的作用范围。
	Scope            Scope              `yaml:"scope"`
	// Type is the executor type of the cleaner.
	// Type 是清理器的执行器类型。
	Type             ExecutorType       `yaml:"type"`
	// Enabled indicates whether the cleaner is enabled.
	// Enabled 表示清理器是否已启用。
	Enabled          *bool              `yaml:"enabled"`
	// MinAgeDays is the minimum age in days before cleaning.
	// MinAgeDays 是清理前的最小天数。
	MinAgeDays       *int               `yaml:"min_age_days"`
	// NoAgeFilter indicates whether to skip age filtering.
	// NoAgeFilter 表示是否跳过时间过滤。
	NoAgeFilter      bool               `yaml:"no_age_filter"`
	// TimeField is the time field used for age calculation.
	// TimeField 是用于计算时间的字段。
	TimeField        TimeField          `yaml:"time_field"`
	// Paths are the paths to clean.
	// Paths 是要清理的路径。
	Paths            []string           `yaml:"paths"`
	// PathResolvers are the path resolver references.
	// PathResolvers 是路径解析器引用。
	PathResolvers    []PathResolverRef  `yaml:"path_resolvers"`
	// Exclude are the paths to exclude from cleaning.
	// Exclude 是要排除清理的路径。
	Exclude          []string           `yaml:"exclude"`
	// RunningProcesses are the processes that must not be running.
	// RunningProcesses 是必须未运行的进程。
	RunningProcesses []string           `yaml:"running_processes"`
	// NeedsRoot indicates whether the cleaner requires root privileges.
	// NeedsRoot 表示清理器是否需要 root 权限。
	NeedsRoot        bool               `yaml:"needs_root"`
	// Tags are the tags associated with the cleaner.
	// Tags 是与清理器关联的标签。
	Tags             []string           `yaml:"tags"`
	// PkgCache is the package cache configuration.
	// PkgCache 是包缓存配置。
	PkgCache         *PkgCacheCfg       `yaml:"pkg_cache"`
	// ContainerPrune is the container prune configuration.
	// ContainerPrune 是容器清理配置。
	ContainerPrune   *ContainerPruneCfg `yaml:"container_prune"`
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
