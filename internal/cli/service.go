package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/ui/tui"
	"github.com/spf13/cobra"
)

type cleanContext struct {
	Scope     model.Scope
	Home      string
	TargetUID int
	CfgDir    string
	Cfg       *config.Config
	Merged    []model.Cleaner
}

func translatorFromFlag(flagLang string) *i18n.Translator {
	return i18n.New(i18n.Detect(flagLang))
}

func translatorForCommand(cmd *cobra.Command, fallback *i18n.Translator) *i18n.Translator {
	if cmd != nil {
		if f := cmd.Flags().Lookup("lang"); f != nil && f.Changed {
			return i18n.New(i18n.Detect(f.Value.String()))
		}
	}
	if fallback != nil {
		return fallback
	}
	return i18n.New(i18n.Detect(""))
}

func resolveConfigDirForScope(override string, scope model.Scope, home string) string {
	if override != "" {
		return override
	}
	configHome := home
	if scope == model.ScopeSystem || configHome == "" {
		configHome, _ = os.UserHomeDir()
	}
	return filepath.Join(configHome, ".config", "beav")
}

func loadCleanContext(f CleanFlags, tr *i18n.Translator) (*cleanContext, error) {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	scope, home, targetUID, err := determineScope(f, tr)
	if err != nil {
		return nil, ExitError{code: 1, err: err}
	}
	cfgDir := resolveConfigDirForScope(f.ConfigDir, scope, home)
	cfg, err := config.LoadWithHome(cfgDir, home)
	if err != nil {
		return nil, ExitError{code: 2, err: fmt.Errorf("%s: %w", tr.Text("config load", "加载配置失败"), err)}
	}
	var builtinList []registry.Loaded
	if !f.BuiltinDisabled {
		builtinList, err = registry.LoadBuiltin(cleaners.Builtin)
		if err != nil {
			return nil, ExitError{code: 2, err: fmt.Errorf("%s: %w", tr.Text("builtin load", "加载内置清理项失败"), err)}
		}
	}
	userList, err := registry.LoadUserDir(filepath.Join(cfgDir, "cleaners.d"))
	if err != nil {
		return nil, ExitError{code: 2, err: fmt.Errorf("%s: %w", tr.Text("user cleaners", "加载自定义清理项失败"), err)}
	}
	merged, err := registry.MergeByID(builtinList, userList)
	if err != nil {
		return nil, ExitError{code: 2, err: err}
	}
	merged, err = applyEffectiveConfig(merged, cfg, f, tr)
	if err != nil {
		return nil, ExitError{code: 2, err: err}
	}
	if err := validateCleanersForRun(merged, scope, home, f); err != nil {
		return nil, ExitError{code: 2, err: err}
	}
	return &cleanContext{
		Scope:     scope,
		Home:      home,
		TargetUID: targetUID,
		CfgDir:    cfgDir,
		Cfg:       cfg,
		Merged:    merged,
	}, nil
}

func loadCatalog(cfgDir string, f CleanFlags, tr *i18n.Translator) (*config.Config, []model.Cleaner, string, error) {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	resolvedDir := resolveConfigDir(cfgDir)
	cfg, err := config.Load(resolvedDir)
	if err != nil {
		return nil, nil, resolvedDir, ExitError{code: 2, err: fmt.Errorf("%s: %w", tr.Text("config load", "加载配置失败"), err)}
	}
	var builtin []registry.Loaded
	if !f.BuiltinDisabled {
		builtin, err = registry.LoadBuiltin(cleaners.Builtin)
		if err != nil {
			return nil, nil, resolvedDir, ExitError{code: 2, err: fmt.Errorf("%s: %w", tr.Text("builtin load", "加载内置清理项失败"), err)}
		}
	}
	user, err := registry.LoadUserDir(filepath.Join(resolvedDir, "cleaners.d"))
	if err != nil {
		return nil, nil, resolvedDir, ExitError{code: 2, err: fmt.Errorf("%s: %w", tr.Text("user cleaners", "加载自定义清理项失败"), err)}
	}
	merged, err := registry.MergeByID(builtin, user)
	if err != nil {
		return nil, nil, resolvedDir, ExitError{code: 2, err: err}
	}
	effective, err := applyEffectiveConfig(merged, cfg, f, tr)
	if err != nil {
		return nil, nil, resolvedDir, ExitError{code: 2, err: err}
	}
	return cfg, effective, resolvedDir, nil
}

func selectorMatchesCleaner(c model.Cleaner, selector string) bool {
	if selector == c.ID {
		return true
	}
	for _, tag := range c.Tags {
		if selector == tag {
			return true
		}
	}
	return strings.HasPrefix(c.ID, selector+"-")
}

func diagnoseSelectors(merged []model.Cleaner, scope model.Scope, only []string, tr *i18n.Translator) (error, []string) {
	if len(only) == 0 {
		return nil, nil
	}
	var warnings []string
	for _, sel := range only {
		var anyMatch []model.Cleaner
		var scopeMatch []model.Cleaner
		var enabledMatch []model.Cleaner
		for _, c := range merged {
			if !selectorMatchesCleaner(c, sel) {
				continue
			}
			anyMatch = append(anyMatch, c)
			if scope == "" || scope == model.ScopeAll || c.Scope == scope {
				scopeMatch = append(scopeMatch, c)
				if c.IsEnabled() {
					enabledMatch = append(enabledMatch, c)
				}
			}
		}
		if len(anyMatch) == 0 {
			suggestions := suggestCleanerIDs(merged, sel, 3)
			if len(suggestions) > 0 {
				return ExitError{
					code: 1,
					err: fmt.Errorf("%s", tr.Textf(
						"unknown cleaner or tag %q (did you mean: %s?); run `beav list` to inspect all IDs",
						"未找到清理项或标签 %q（是否指：%s？）；运行 `beav list` 查看全部 ID",
						sel, strings.Join(suggestions, ", "),
					)),
				}, nil
			}
			return ExitError{
				code: 1,
				err: fmt.Errorf("%s", tr.Textf(
					"unknown cleaner or tag %q; run `beav list` to inspect all IDs",
					"未找到清理项或标签 %q；运行 `beav list` 查看全部 ID",
					sel,
				)),
			}, nil
		}
		if len(scopeMatch) == 0 {
			if anyMatch[0].Scope == model.ScopeSystem {
				return ExitError{
					code: 1,
					err: fmt.Errorf("%s", tr.Textf(
						"%q matches system-scope cleaners; run `sudo beav clean --system --only %s`",
						"%q 属于系统级清理项；请运行 `sudo beav clean --system --only %s`",
						sel, sel,
					)),
				}, nil
			}
			return ExitError{
				code: 1,
				err: fmt.Errorf("%s", tr.Textf(
					"%q matches user-scope cleaners; run `beav clean --only %s` (or `--all`) instead of `--system`",
					"%q 属于用户级清理项；请改用 `beav clean --only %s`（或 `--all`）",
					sel, sel,
				)),
			}, nil
		}
		if len(enabledMatch) == 0 {
			warnings = append(warnings, tr.Textf(
				"%q is currently disabled in config; run `beav config enable %s` to re-enable",
				"%q 当前在配置中处于禁用状态；运行 `beav config enable %s` 可重新启用",
				sel, scopeMatch[0].ID,
			))
		}
	}
	return nil, warnings
}

func suggestCleanerIDs(cleaners []model.Cleaner, query string, max int) []string {
	type scored struct {
		id    string
		score int
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return nil
	}
	candidates := make([]scored, 0, len(cleaners))
	for _, c := range cleaners {
		id := strings.ToLower(c.ID)
		dist := levenshtein(q, id)
		switch {
		case strings.Contains(id, q) || strings.Contains(q, id):
			candidates = append(candidates, scored{id: c.ID, score: dist / 2})
		case dist <= maxInt(3, len(q)/2):
			candidates = append(candidates, scored{id: c.ID, score: 10 + dist})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score < candidates[j].score
		}
		return candidates[i].id < candidates[j].id
	})
	if len(candidates) > max {
		candidates = candidates[:max]
	}
	out := make([]string, 0, len(candidates))
	for _, cand := range candidates {
		out = append(out, cand.id)
	}
	return out
}

func levenshtein(a, b string) int {
	ra := []rune(a)
	rb := []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			m := prev[j] + 1
			if curr[j-1]+1 < m {
				m = curr[j-1] + 1
			}
			if prev[j-1]+cost < m {
				m = prev[j-1] + cost
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)]
}

func sortCleaners(list []model.Cleaner) {
	sort.Slice(list, func(i, j int) bool {
		if list[i].Scope != list[j].Scope {
			return list[i].Scope == model.ScopeUser
		}
		ti, tj := "", ""
		if len(list[i].Tags) > 0 {
			ti = list[i].Tags[0]
		}
		if len(list[j].Tags) > 0 {
			tj = list[j].Tags[0]
		}
		if ti != tj {
			if ti == "" {
				return false
			}
			if tj == "" {
				return true
			}
			return ti < tj
		}
		return list[i].ID < list[j].ID
	})
}

func padLabel(label string, width int) string {
	pad := width - lipgloss.Width(label)
	if pad < 1 {
		pad = 1
	}
	return label + strings.Repeat(" ", pad)
}

func writeAlignedTable(w io.Writer, headers []string, rows [][]string) {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = lipgloss.Width(h)
	}
	for _, r := range rows {
		for i := range headers {
			if i < len(r) {
				if cw := lipgloss.Width(r[i]); cw > widths[i] {
					widths[i] = cw
				}
			}
		}
	}
	printRow := func(cols []string) {
		var b strings.Builder
		for i := range headers {
			val := ""
			if i < len(cols) {
				val = cols[i]
			}
			b.WriteString(val)
			if i < len(headers)-1 {
				pad := widths[i] - lipgloss.Width(val) + 2
				if pad < 2 {
					pad = 2
				}
				b.WriteString(strings.Repeat(" ", pad))
			}
		}
		fmt.Fprintln(w, b.String())
	}
	printRow(headers)
	for _, r := range rows {
		printRow(r)
	}
}

// runReadOnlyScan executes a strictly read-only preview pass across all enabled
// cleaners in the target scope and streams progress for the TUI workbench.
// runReadOnlyScan 在目标作用域内对所有启用清理器执行严格只读的预演扫描，并向 TUI 流式发送进度。
func runReadOnlyScan(ctx context.Context, merged []model.Cleaner, scope model.Scope, home string, targetUID int, cfg *config.Config) (<-chan tui.ScanProgress, context.CancelFunc) {
	scanCtx, cancel := context.WithCancel(ctx)
	ch := make(chan tui.ScanProgress, 16)

	eligible := make([]model.Cleaner, 0, len(merged))
	byID := make(map[string]model.Cleaner, len(merged))
	for _, c := range merged {
		if engine.Selected(c, scope, nil, nil) {
			eligible = append(eligible, c)
			byID[c.ID] = c
		}
	}

	send := func(p tui.ScanProgress) {
		select {
		case ch <- p:
		case <-scanCtx.Done():
		}
	}

	go func() {
		defer close(ch)
		total := len(eligible)
		send(tui.ScanProgress{Total: total, Completed: 0})
		if total == 0 {
			send(tui.ScanProgress{Total: 0, Completed: 0, Done: true})
			return
		}

		eng := newCleanEngine(home, targetUID, safety.NewWhitelist(cfg.MergedWhitelist()))
		completed := 0
		var activeSamples []tui.FileSample
		var activeCmd []string
		var skipReason, skipDetail, errDetail string

		emit := func(ev model.Event) {
			switch ev.Event {
			case model.EvStart:
				activeSamples = nil
				activeCmd = nil
				skipReason = ""
				skipDetail = ""
				errDetail = ""
				send(tui.ScanProgress{
					Total:       total,
					Completed:   completed,
					CurrentID:   ev.CleanerID,
					CurrentName: ev.Name,
				})
			case model.EvSkipped:
				if ev.Reason == string(model.SkipReasonDryRun) {
					activeSamples = insertSample(activeSamples, tui.FileSample{Path: ev.Path, Size: ev.Size}, 20)
				}
			case model.EvCommandOutput:
				line := strings.TrimSpace(ev.Line)
				if line != "" && len(activeCmd) < 30 {
					activeCmd = append(activeCmd, line)
				}
			case model.EvCleanerSkipped:
				skipReason = ev.Reason
				skipDetail = ev.Detail
			case model.EvError:
				if errDetail == "" {
					errDetail = ev.Detail
				} else {
					errDetail += "; " + ev.Detail
				}
			case model.EvFinish:
				completed++
				scan := &tui.CleanerScan{
					Cleaner:           byID[ev.CleanerID],
					Status:            ev.Status,
					FilesCandidate:    ev.FilesCandidate,
					BytesCandidate:    ev.BytesCandidate,
					MetricsIncomplete: ev.MetricsIncomplete,
					SkipReason:        skipReason,
					SkipDetail:        skipDetail,
					ErrorDetail:       errDetail,
					Samples:           append([]tui.FileSample(nil), activeSamples...),
					CommandPreview:    append([]string(nil), activeCmd...),
				}
				send(tui.ScanProgress{
					Total:     total,
					Completed: completed,
					Item:      scan,
				})
			}
		}

		_, runErr := eng.Run(scanCtx, eligible, engine.Options{
			Scope:        scope,
			DryRun:       true, // Read-only TUI scan is unconditionally dry-run.
			IncludeHeavy: true,
			Emitter:      emit,
		})
		if errors.Is(runErr, context.Canceled) {
			return
		}
		send(tui.ScanProgress{
			Total:     total,
			Completed: completed,
			Done:      true,
			Err:       runErr,
		})
	}()

	return ch, cancel
}

func insertSample(samples []tui.FileSample, s tui.FileSample, limit int) []tui.FileSample {
	if len(samples) < limit {
		samples = append(samples, s)
		sort.Slice(samples, func(i, j int) bool { return samples[i].Size > samples[j].Size })
		return samples
	}
	if s.Size <= samples[len(samples)-1].Size {
		return samples
	}
	samples[len(samples)-1] = s
	sort.Slice(samples, func(i, j int) bool { return samples[i].Size > samples[j].Size })
	return samples
}

func runTUIExec(ctx context.Context, merged []model.Cleaner, scope model.Scope, home string, targetUID int, cfg *config.Config, f CleanFlags, selectedIDs []string) (<-chan tui.ExecProgress, context.CancelFunc) {
	execCtx, cancel := context.WithCancel(ctx)
	ch := make(chan tui.ExecProgress, 16)

	execFlags := f
	execFlags.DryRun = false
	execFlags.Only = append([]string(nil), selectedIDs...)
	execFlags.IncludeHeavy = false

	send := func(p tui.ExecProgress) {
		select {
		case ch <- p:
		case <-execCtx.Done():
		}
	}

	go func() {
		defer close(ch)
		total := len(selectedIDs)
		completed := 0
		var errDetail string
		logPath := operationLogPath(home, targetUID)

		res, _, runErr := runCleanPassWithSink(execCtx, merged, scope, home, targetUID, cfg, execFlags, false, func(ev model.Event) {
			switch ev.Event {
			case model.EvStart:
				errDetail = ""
				send(tui.ExecProgress{
					Total:       total,
					Completed:   completed,
					CurrentID:   ev.CleanerID,
					CurrentName: ev.Name,
				})
			case model.EvError:
				if errDetail == "" {
					errDetail = ev.Detail
				} else {
					errDetail += "; " + ev.Detail
				}
			case model.EvFinish:
				completed++
				send(tui.ExecProgress{
					Total:             total,
					Completed:         completed,
					CleanerID:         ev.CleanerID,
					Status:            ev.Status,
					FilesDeleted:      ev.FilesDeleted,
					BytesFreed:        ev.BytesFreed,
					MetricsIncomplete: ev.MetricsIncomplete,
					ErrorDetail:       errDetail,
				})
			}
		})
		if errors.Is(runErr, context.Canceled) {
			return
		}
		send(tui.ExecProgress{
			Total:      total,
			Completed:  completed,
			Done:       true,
			TotalFreed: res.BytesFreed,
			TotalFiles: res.FilesDeleted,
			Unmeasured: append([]string(nil), res.UnmeasuredCleaners...),
			Errored:    res.CleanersErrored,
			LogPath:    logPath,
			Err:        runErr,
		})
	}()

	return ch, cancel
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
