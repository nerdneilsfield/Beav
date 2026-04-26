package model

type ExecutorType string

const (
	TypePaths          ExecutorType = "paths"
	TypeCommand        ExecutorType = "command"
	TypeJournalVacuum  ExecutorType = "journal_vacuum"
	TypePkgCache       ExecutorType = "pkg_cache"
	TypeContainerPrune ExecutorType = "container_prune"
)

func ParseExecutorType(s string) (ExecutorType, bool) {
	switch ExecutorType(s) {
	case TypePaths, TypeCommand, TypeJournalVacuum, TypePkgCache, TypeContainerPrune:
		return ExecutorType(s), true
	}
	return "", false
}

type Scope string

const (
	ScopeUser   Scope = "user"
	ScopeSystem Scope = "system"
)

func ParseScope(s string) (Scope, bool) {
	switch Scope(s) {
	case ScopeUser, ScopeSystem:
		return Scope(s), true
	}
	return "", false
}

type TimeField string

const (
	TimeMtime TimeField = "mtime"
	TimeCtime TimeField = "ctime"
)

type PathResolverRef struct {
	Resolver string   `yaml:"resolver"`
	Subpaths []string `yaml:"subpaths"`
}

type PkgCacheCfg struct {
	Manager string `yaml:"manager"`
}

type ContainerPruneCfg struct {
	Runtime string `yaml:"runtime"`
	Target  string `yaml:"target"`
}

type Cleaner struct {
	ID               string             `yaml:"id"`
	Name             string             `yaml:"name"`
	Description      string             `yaml:"description"`
	Scope            Scope              `yaml:"scope"`
	Type             ExecutorType       `yaml:"type"`
	Enabled          *bool              `yaml:"enabled"`
	MinAgeDays       *int               `yaml:"min_age_days"`
	NoAgeFilter      bool               `yaml:"no_age_filter"`
	TimeField        TimeField          `yaml:"time_field"`
	Paths            []string           `yaml:"paths"`
	PathResolvers    []PathResolverRef  `yaml:"path_resolvers"`
	Exclude          []string           `yaml:"exclude"`
	RunningProcesses []string           `yaml:"running_processes"`
	NeedsRoot        bool               `yaml:"needs_root"`
	Tags             []string           `yaml:"tags"`
	PkgCache         *PkgCacheCfg       `yaml:"pkg_cache"`
	ContainerPrune   *ContainerPruneCfg `yaml:"container_prune"`
}

func (c Cleaner) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

func (c Cleaner) AgeOrDefault(def int) int {
	if c.MinAgeDays == nil {
		return def
	}
	return *c.MinAgeDays
}
