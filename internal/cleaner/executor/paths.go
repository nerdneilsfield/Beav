package executor

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/resolver"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

// errNoMatches is returned when glob patterns match no paths.
// errNoMatches 在 glob 模式未匹配到任何路径时返回。
var errNoMatches = errors.New("no matching paths")

// PathsExecutor implements the Executor interface for cleaning file paths.
// PathsExecutor 实现了用于清理文件路径的 Executor 接口。
type PathsExecutor struct {
	// Home is the user's home directory for expanding ~ prefixes.
	// Home 是用于展开 ~ 前缀的用户主目录。
	Home string
	// Whitelist contains paths that must never be deleted.
	// Whitelist 包含绝不能被删除的路径。
	Whitelist *safety.Whitelist
}

// NewPathsExecutor creates a new PathsExecutor with the given home directory and whitelist.
// NewPathsExecutor 使用给定的主目录和白名单创建一个新的 PathsExecutor。
func NewPathsExecutor(home string, wl *safety.Whitelist) *PathsExecutor {
	return &PathsExecutor{Home: home, Whitelist: wl}
}

// Run executes the path cleaning operation, emitting events for each action taken.
// Run 执行路径清理操作，为每个操作发出事件。
func (p *PathsExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := time.Now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})

	if safety.AnyProcessRunning(c.RunningProcesses) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "running_process", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}

	roots, err := p.expandRoots(c)
	if err != nil {
		reason := "boundary_violation"
		if errors.Is(err, errNoMatches) {
			reason = "no_matches"
		}
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: reason, Detail: err.Error(), TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}

	field := safety.TimeFieldMtime
	if c.TimeField == model.TimeCtime {
		field = safety.TimeFieldCtime
	}
	age := c.AgeOrDefault(14)
	excludes := compileGlobs(c.Exclude)
	var bytesFreed int64
	var filesDeleted int64
	var errs int

	process := func(w *safety.Walker, entries []safety.Entry) {
		for _, e := range entries {
			select {
			case <-ctx.Done():
				errs++
				emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: ctx.Err().Error(), TS: time.Now()})
				return
			default:
			}
			abs := filepath.Join(w.Root(), e.RelPath)
			if p.Whitelist != nil && p.Whitelist.Match(abs) {
				emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "whitelisted", TS: time.Now()})
				continue
			}
			if matchAny(excludes, abs, e.RelPath) {
				emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "excluded", TS: time.Now()})
				continue
			}
			if dryRun {
				emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "dry_run", Size: e.Size(), TS: time.Now()})
				bytesFreed += e.Size()
				filesDeleted++
				continue
			}
			var unlinkErr error
			if e.IsDir() {
				unlinkErr = w.RemoveEmptyDirIfMatch(e)
			} else {
				unlinkErr = w.UnlinkIfUnchanged(e)
			}
			if unlinkErr != nil {
				if errors.Is(unlinkErr, safety.ErrChanged) {
					emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "toctou_changed", TS: time.Now()})
					continue
				}
				emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "unlink_failed", Detail: unlinkErr.Error(), TS: time.Now()})
				errs++
				continue
			}
			emit(model.Event{Event: model.EvDeleted, CleanerID: c.ID, Path: abs, Size: e.Size(), TS: time.Now()})
			bytesFreed += e.Size()
			filesDeleted++
		}
	}

	for _, root := range roots {
		if ctx.Err() != nil {
			break
		}
		safeRoot := determineSafeRoot(root, p.Home)
		if safeRoot == "" {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: root, Reason: "blacklisted", TS: time.Now()})
			continue
		}
		p.runRoot(c, root, safeRoot, age, field, process, emit, &errs)
	}

	status := "ok"
	if errs > 0 {
		status = "error"
	}
	emit(model.Event{
		Event:        model.EvFinish,
		CleanerID:    c.ID,
		Status:       status,
		FilesDeleted: filesDeleted,
		BytesFreed:   bytesFreed,
		Errors:       errs,
		DurationMs:   time.Since(start).Milliseconds(),
		TS:           time.Now(),
	})
	return nil
}

func (p *PathsExecutor) runRoot(c model.Cleaner, root, safeRoot string, age int, field safety.TimeField, process func(*safety.Walker, []safety.Entry), emit func(model.Event), errs *int) {
	parent := filepath.Dir(root)
	leaf := filepath.Base(root)
	if filepath.Clean(root) == filepath.Clean(safeRoot) {
		parent = safeRoot
		leaf = "."
	}
	parentFD, err := safety.OpenAnchoredDirFD(safeRoot, parent)
	if err != nil {
		emitOpenSkip(c.ID, root, err, emit)
		return
	}
	defer unix.Close(parentFD)

	var st unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		emitOpenSkip(c.ID, root, err, emit)
		return
	}
	mode := st.Mode & unix.S_IFMT
	if mode == unix.S_IFLNK {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: root, Reason: "symlink", TS: time.Now()})
		return
	}
	if mode == unix.S_IFREG {
		w, e, err := safety.OpenAnchoredFile(safeRoot, root)
		if err != nil {
			emitOpenSkip(c.ID, root, err, emit)
			return
		}
		defer w.Close()
		if c.NoAgeFilter || entryOldEnough(e, age, field, time.Now()) {
			process(w, []safety.Entry{e})
		} else {
			emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: root, Reason: "age_too_recent", TS: time.Now()})
		}
		return
	}
	if mode != unix.S_IFDIR {
		emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: root, Reason: "wrong_type", TS: time.Now()})
		return
	}

	dirFD, err := safety.OpenAnchoredDirFD(safeRoot, root)
	if err != nil {
		emitOpenSkip(c.ID, root, err, emit)
		return
	}
	w, err := safety.OpenWalkerFD(dirFD, root)
	if err != nil {
		*errs++
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: root, Reason: "walk_failed", Detail: err.Error(), TS: time.Now()})
		return
	}
	defer w.Close()
	plan := safety.AgePlan(w, age, field, time.Now(), c.NoAgeFilter)
	process(w, plan.Ordered())
}

func entryOldEnough(e safety.Entry, age int, field safety.TimeField, now time.Time) bool {
	ts := e.Mtime()
	if field == safety.TimeFieldCtime {
		ts = e.Ctime()
	}
	return ts <= now.Add(-time.Duration(age)*24*time.Hour).Unix()
}

func emitOpenSkip(cleanerID, path string, err error, emit func(model.Event)) {
	reason := "permission_denied"
	switch {
	case errors.Is(err, safety.ErrSymlink):
		reason = "symlink"
	case errors.Is(err, safety.ErrCrossFS):
		reason = "cross_fs"
	case errors.Is(err, safety.ErrNotInsideRoot):
		reason = "blacklisted"
	case errors.Is(err, safety.ErrNotDir):
		reason = "wrong_type"
	}
	emit(model.Event{Event: model.EvSkipped, CleanerID: cleanerID, Path: path, Reason: reason, Detail: err.Error(), TS: time.Now()})
}

// expandRoots expands path patterns and resolvers into a validated list of root paths.
// expandRoots 将路径模式和解析器展开为经过验证的根路径列表。
func (p *PathsExecutor) expandRoots(c model.Cleaner) ([]string, error) {
	var raws []string
	for _, pat := range c.Paths {
		raws = append(raws, expandHome(pat, p.Home))
	}
	for _, ref := range c.PathResolvers {
		base, err := resolver.Resolve(ref.Resolver, p.Home)
		if err != nil {
			continue
		}
		if len(ref.Subpaths) == 0 {
			raws = append(raws, base)
			continue
		}
		for _, sp := range ref.Subpaths {
			raws = append(raws, filepath.Join(base, sp))
		}
	}

	out := make([]string, 0, len(raws))
	hadMatch := false
	for _, r := range raws {
		matches := []string{r}
		if hasGlobMeta(r) {
			var err error
			matches, err = filepath.Glob(r)
			if err != nil {
				return nil, fmt.Errorf("invalid glob %q: %w", r, err)
			}
			if len(matches) == 0 {
				continue
			}
		}
		for _, m := range matches {
			hadMatch = true
			clean := filepath.Clean(m)
			if !safety.InsideAllowList(clean, p.Home) || safety.Blacklisted(clean, p.Home) {
				continue
			}
			out = append(out, clean)
		}
	}
	if len(out) == 0 {
		if !hadMatch {
			return nil, errNoMatches
		}
		return nil, errors.New("no valid roots after safety check")
	}
	return out, nil
}

func hasGlobMeta(p string) bool {
	return strings.ContainsAny(p, "*?[")
}

func expandHome(p, home string) string {
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

// globSet holds compiled glob patterns for exclusion matching.
// globSet 保存用于排除匹配的编译后的 glob 模式。
type globSet []string

func compileGlobs(patterns []string) globSet { return globSet(patterns) }

func matchAny(set globSet, absPath, relPath string) bool {
	for _, pattern := range set {
		if ok, _ := filepath.Match(pattern, filepath.Base(absPath)); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, relPath); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, absPath); ok {
			return true
		}
	}
	return false
}

// determineSafeRoot returns the safe root directory for a given path, or empty string if blacklisted.
// determineSafeRoot 返回给定路径的安全根目录，如果被列入黑名单则返回空字符串。
func determineSafeRoot(path, home string) string {
	clean := filepath.Clean(path)
	if home != "" {
		hc := filepath.Clean(home)
		if clean == hc || strings.HasPrefix(clean, hc+string(filepath.Separator)) {
			return hc
		}
	}
	for _, r := range []string{"/var/cache", "/var/log", "/tmp", "/var/tmp"} {
		if clean == r || strings.HasPrefix(clean, r+string(filepath.Separator)) {
			return r
		}
	}
	return ""
}
