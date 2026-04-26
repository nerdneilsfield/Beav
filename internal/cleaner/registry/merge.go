package registry

import (
	"fmt"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// MergeByID merges builtin and user cleaner definitions by ID, applying user overrides.
// MergeByID 按 ID 合并内置和用户清理器定义，应用用户覆盖。
func MergeByID(builtin, user []Loaded) ([]model.Cleaner, error) {
	byID := map[string]model.Cleaner{}
	order := []string{}
	for _, l := range builtin {
		if _, ok := byID[l.Cleaner.ID]; !ok {
			order = append(order, l.Cleaner.ID)
		}
		byID[l.Cleaner.ID] = l.Cleaner
	}
	for _, l := range user {
		base, ok := byID[l.Cleaner.ID]
		if !ok {
			byID[l.Cleaner.ID] = l.Cleaner
			order = append(order, l.Cleaner.ID)
			continue
		}
		if l.Cleaner.Type != "" && l.Cleaner.Type != base.Type {
			return nil, fmt.Errorf("cleaner %q: cannot override type", l.Cleaner.ID)
		}
		if l.Cleaner.Scope != "" && l.Cleaner.Scope != base.Scope {
			return nil, fmt.Errorf("cleaner %q: cannot override scope", l.Cleaner.ID)
		}
		if l.Cleaner.Enabled != nil {
			base.Enabled = l.Cleaner.Enabled
		}
		if l.Cleaner.MinAgeDays != nil {
			base.MinAgeDays = l.Cleaner.MinAgeDays
		}
		if l.Cleaner.NoAgeFilter {
			base.NoAgeFilter = true
		}
		if l.Cleaner.TimeField != "" {
			base.TimeField = l.Cleaner.TimeField
		}
		base.Paths = append(base.Paths, l.Cleaner.Paths...)
		base.Exclude = append(base.Exclude, l.Cleaner.Exclude...)
		base.PathResolvers = append(base.PathResolvers, l.Cleaner.PathResolvers...)
		base.RunningProcesses = append(base.RunningProcesses, l.Cleaner.RunningProcesses...)
		base.Tags = append(base.Tags, l.Cleaner.Tags...)
		byID[l.Cleaner.ID] = base
	}
	out := make([]model.Cleaner, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, nil
}
