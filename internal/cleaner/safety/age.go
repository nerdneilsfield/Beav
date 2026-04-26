package safety

import (
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type TimeField int

const (
	TimeFieldMtime TimeField = iota
	TimeFieldCtime
)

type Plan struct {
	root     string
	delete   map[string]Entry
	keepDirs map[string]bool
	ordered  []Entry
}

func (p *Plan) WillDelete(absPath string) bool {
	_, ok := p.delete[absPath]
	return ok
}

func (p *Plan) Ordered() []Entry {
	return p.ordered
}

func AgePlan(w *Walker, minAgeDays int, field TimeField, now time.Time, noAgeFilter bool) *Plan {
	threshold := now.Add(-time.Duration(minAgeDays) * 24 * time.Hour).Unix()
	var all []ageInfo
	rootAbs := w.Root()
	_ = w.Walk(func(e Entry) {
		passed := true
		if !noAgeFilter {
			ts := e.Mtime()
			if field == TimeFieldCtime {
				ts = e.Ctime()
			}
			passed = ts <= threshold
		}
		all = append(all, ageInfo{entry: e, passed: passed, absPath: filepath.Join(rootAbs, e.RelPath)})
	})

	plan := &Plan{root: rootAbs, delete: map[string]Entry{}, keepDirs: map[string]bool{}}
	for _, it := range all {
		if !it.passed {
			dir := it.absPath
			if !it.entry.IsDir() {
				dir = filepath.Dir(it.absPath)
			}
			for dir != rootAbs && dir != "/" && dir != "." {
				plan.keepDirs[dir] = true
				dir = filepath.Dir(dir)
			}
		}
	}
	for _, it := range all {
		if it.entry.IsRegular() && it.passed {
			plan.delete[it.absPath] = it.entry
		}
	}
	for _, it := range all {
		if it.entry.IsDir() && it.passed && !plan.keepDirs[it.absPath] {
			plan.delete[it.absPath] = it.entry
		}
	}
	plan.ordered = orderForDeletion(all, plan.delete)
	return plan
}

type ageInfo struct {
	entry   Entry
	passed  bool
	absPath string
}

func orderForDeletion(all []ageInfo, set map[string]Entry) []Entry {
	type pair struct {
		depth int
		e     Entry
		abs   string
	}
	var ps []pair
	for _, it := range all {
		if e, ok := set[it.absPath]; ok {
			ps = append(ps, pair{depth: pathDepth(it.absPath), e: e, abs: it.absPath})
		}
	}
	sort.SliceStable(ps, func(i, j int) bool {
		if ps[i].depth != ps[j].depth {
			return ps[i].depth > ps[j].depth
		}
		if ps[i].e.IsRegular() != ps[j].e.IsRegular() {
			return ps[i].e.IsRegular()
		}
		return ps[i].abs < ps[j].abs
	})
	out := make([]Entry, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.e)
	}
	return out
}

func pathDepth(p string) int {
	return strings.Count(filepath.Clean(p), string(filepath.Separator))
}
