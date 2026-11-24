package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dustin/go-humanize"
)

// Plain renders cleaner events as plain, non-ANSI text suitable for pipes, logs, or --output plain.
// Plain 将清理器事件渲染为纯文本（无 ANSI 控制符），适用于管道、日志或 --output plain。
type Plain struct {
	mu      sync.Mutex
	w       io.Writer
	tr      *i18n.Translator
	verbose bool
	current map[string]*cleanerState
	noop    []string
	skips   map[string][]skipEntry
}

// NewPlain creates a Plain renderer with default English concise options.
// NewPlain 创建一个使用默认英文简洁选项的 Plain 渲染器。
func NewPlain(w io.Writer) *Plain {
	return NewPlainWithOptions(w, Options{})
}

// NewPlainWithOptions creates a Plain renderer configured with the given Options.
// NewPlainWithOptions 创建一个按给定 Options 配置的 Plain 渲染器。
func NewPlainWithOptions(w io.Writer, opts Options) *Plain {
	return &Plain{
		w:       w,
		tr:      resolveTranslator(opts.Translator),
		verbose: opts.Verbose,
		current: map[string]*cleanerState{},
		skips:   map[string][]skipEntry{},
	}
}

// Render processes a cleaner event and writes the appropriate output.
// Render 处理清理器事件并写入相应输出。
func (p *Plain) Render(e model.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch e.Event {
	case model.EvStart:
		p.current[e.CleanerID] = &cleanerState{start: e}
	case model.EvDeleted:
		if st := p.current[e.CleanerID]; st != nil && p.verbose {
			st.sampleTotal++
			if len(st.samples) < verboseDetailLimit {
				st.samples = append(st.samples, fmt.Sprintf("%s (%s)", e.Path, humanize.Bytes(safeBytes(e.Size))))
			}
		}
	case model.EvSkipped:
		if e.Reason == string(model.SkipReasonDryRun) {
			if st := p.current[e.CleanerID]; st != nil && p.verbose {
				st.sampleTotal++
				if len(st.samples) < verboseDetailLimit {
					st.samples = append(st.samples, fmt.Sprintf("%s (%s)", e.Path, humanize.Bytes(safeBytes(e.Size))))
				}
			}
		}
	case model.EvCommandOutput:
		if st := p.current[e.CleanerID]; st != nil {
			line := strings.TrimSpace(e.Line)
			if line != "" {
				st.cmdTotal++
				if len(st.cmdLines) < verboseDetailLimit {
					st.cmdLines = append(st.cmdLines, line)
				}
			}
		}
	case model.EvFinish:
		st := p.current[e.CleanerID]
		var startName string
		if st != nil {
			startName = st.start.Name
		}
		name := displayName(p.tr, e.CleanerID, startName)
		delete(p.current, e.CleanerID)
		if e.Status == "skipped" || e.Status == "error" {
			return
		}
		p.renderFinish(e, st, name)
	case model.EvCleanerSkipped:
		st := p.current[e.CleanerID]
		var startName string
		if st != nil {
			startName = st.start.Name
		}
		name := displayName(p.tr, e.CleanerID, startName)
		p.skips[e.Reason] = append(p.skips[e.Reason], skipEntry{
			ID:     e.CleanerID,
			Name:   name,
			Detail: e.Detail,
		})
	case model.EvError:
		st := p.current[e.CleanerID]
		var startName string
		if st != nil {
			startName = st.start.Name
		}
		name := displayName(p.tr, e.CleanerID, startName)
		fmt.Fprintf(p.w, "%s", p.tr.Textf("%s (%s) - error - %s\n", "%s (%s) - 错误 - %s\n", name, e.CleanerID, e.Detail))
	case model.EvSummary:
		p.flushGroups()
		p.renderSummary(e)
	}
}

func (p *Plain) renderFinish(e model.Event, st *cleanerState, name string) {
	switch {
	case e.DryRun && e.FilesCandidate > 0:
		partial := ""
		if e.MetricsIncomplete {
			partial = p.tr.Text(" (partial)", "（部分估算）")
		}
		fmt.Fprintf(p.w, "%s\n",
			p.tr.Textf("%s (%s) - plan - %s in %s%s",
				"%s (%s) - 预览 - %s（%s）%s",
				name, e.CleanerID,
				humanize.Bytes(safeBytes(e.BytesCandidate)),
				p.tr.Plural(e.FilesCandidate, "file", "files", "个文件"),
				partial,
			),
		)
		p.printDetails(st, verboseDetailLimit)
	case e.DryRun && e.MetricsIncomplete:
		fmt.Fprintf(p.w, "%s (%s) - %s - %s\n",
			name, e.CleanerID, p.tr.Text("plan", "预览"),
			p.tr.Text("native command (size unmeasured)", "原生命令（无法估算大小）"),
		)
		limit := previewCmdLimit
		if p.verbose {
			limit = verboseDetailLimit
		}
		p.printCommandLines(st, limit)
	case e.DryRun:
		p.noop = append(p.noop, name)
	case e.FilesDeleted > 0 || e.BytesFreed > 0:
		partial := ""
		if e.MetricsIncomplete {
			partial = p.tr.Text(" (partial)", "（部分估算）")
		}
		fmt.Fprintf(p.w, "%s\n",
			p.tr.Textf("%s (%s) - ok - %s freed (%s)%s",
				"%s (%s) - 完成 - 已释放 %s（%s）%s",
				name, e.CleanerID,
				humanize.Bytes(safeBytes(e.BytesFreed)),
				p.tr.Plural(e.FilesDeleted, "file", "files", "个文件"),
				partial,
			),
		)
		p.printDetails(st, verboseDetailLimit)
	case e.MetricsIncomplete:
		fmt.Fprintf(p.w, "%s (%s) - %s - %s\n",
			name, e.CleanerID, p.tr.Text("ok", "完成"),
			p.tr.Text("native command completed (size unmeasured)", "原生命令已执行（未统计大小）"),
		)
		if p.verbose {
			p.printCommandLines(st, verboseDetailLimit)
		}
	default:
		p.noop = append(p.noop, name)
	}
}

func (p *Plain) printDetails(st *cleanerState, limit int) {
	if !p.verbose || st == nil {
		return
	}
	if len(st.samples) > 0 {
		for _, line := range st.samples {
			fmt.Fprintf(p.w, "  %s\n", line)
		}
		if st.sampleTotal > len(st.samples) {
			fmt.Fprintf(p.w, "  %s\n", p.tr.Textf("... and %d more files", "... 另有 %d 个文件", st.sampleTotal-len(st.samples)))
		}
	}
	p.printCommandLines(st, limit)
}

func (p *Plain) printCommandLines(st *cleanerState, limit int) {
	if st == nil || len(st.cmdLines) == 0 {
		return
	}
	n := len(st.cmdLines)
	if n > limit {
		n = limit
	}
	for _, line := range st.cmdLines[:n] {
		fmt.Fprintf(p.w, "  %s\n", line)
	}
	if st.cmdTotal > n {
		fmt.Fprintf(p.w, "  %s\n", p.tr.Textf("... and %d more lines", "... 另有 %d 行输出", st.cmdTotal-n))
	}
}

func (p *Plain) flushGroups() {
	if len(p.noop) > 0 {
		fmt.Fprintf(p.w, "%s", p.tr.Textf("Nothing to do: %s\n", "无需清理：%s\n", joinNames(p.tr, p.noop, p.verbose)))
		p.noop = nil
	}
	for _, reason := range sortedSkipReasons(p.skips, false) {
		entries := p.skips[reason]
		line := p.tr.Textf("Skipped (%s): %s", "已跳过（%s）：%s", skipPhrase(p.tr, reason), joinSkipEntries(p.tr, entries, p.verbose))
		if remedy := skipRemedy(p.tr, reason); remedy != "" {
			line += p.tr.Text(" - ", "；") + remedy
		}
		fmt.Fprintln(p.w, line)
		if p.verbose {
			for _, ent := range entries {
				if ent.Detail != "" {
					fmt.Fprintf(p.w, "  %s: %s\n", ent.ID, ent.Detail)
				}
			}
		}
	}

	quietReasons := sortedSkipReasons(p.skips, true)
	if len(quietReasons) > 0 {
		if !p.verbose {
			total := 0
			for _, r := range quietReasons {
				total += len(p.skips[r])
			}
			fmt.Fprintf(p.w, "%s\n",
				p.tr.Textf("Skipped (not installed / no cache): %s - use --verbose to list",
					"已跳过（未安装或无缓存）：%s（加 --verbose 查看明细）",
					p.tr.Plural(int64(total), "cleaner", "cleaners", "项")),
			)
		} else {
			for _, reason := range quietReasons {
				entries := p.skips[reason]
				fmt.Fprintf(p.w, "%s\n",
					p.tr.Textf("Skipped (%s): %s", "已跳过（%s）：%s", skipPhrase(p.tr, reason), joinSkipEntries(p.tr, entries, true)),
				)
				for _, ent := range entries {
					if ent.Detail != "" {
						fmt.Fprintf(p.w, "  %s: %s\n", ent.ID, ent.Detail)
					}
				}
			}
		}
	}
	p.skips = map[string][]skipEntry{}
}

func (p *Plain) renderSummary(e model.Event) {
	tail := formatSummaryTail(p.tr, e)
	if e.DryRun {
		fmt.Fprintf(p.w, "%s\n",
			p.tr.Textf("Total: %s ran - %s in %s%s",
				"总计：扫描 %s，预计可释放 %s（%s）%s",
				p.tr.Plural(int64(e.CleanersRun), "cleaner", "cleaners", "项"),
				humanize.Bytes(safeBytes(e.BytesCandidate)),
				p.tr.Plural(e.FilesCandidate, "file", "files", "个文件"),
				tail,
			),
		)
	} else {
		fmt.Fprintf(p.w, "%s\n",
			p.tr.Textf("Total: %s ran - %s freed (%s)%s",
				"总计：处理 %s，已释放 %s（%s）%s",
				p.tr.Plural(int64(e.CleanersRun), "cleaner", "cleaners", "项"),
				humanize.Bytes(safeBytes(e.BytesFreed)),
				p.tr.Plural(e.FilesDeleted, "file", "files", "个文件"),
				tail,
			),
		)
	}
	if e.MetricsIncomplete && len(e.UnmeasuredCleaners) > 0 {
		fmt.Fprintf(p.w, "%s\n",
			p.tr.Textf("Note: exact byte sizes unavailable for native commands: %s",
				"提示：以下原生命令无法精确统计释放空间：%s",
				strings.Join(e.UnmeasuredCleaners, ", "),
			),
		)
	}
}

// Close satisfies the ui.Renderer interface.
// Close 实现 ui.Renderer 接口。
func (p *Plain) Close() error {
	return nil
}

// Quiet renders only error events and the final summary line.
// Quiet 仅输出错误事件与最终汇总行。
type Quiet struct {
	mu sync.Mutex
	w  io.Writer
	tr *i18n.Translator
}

// NewQuietRenderer creates a minimal renderer for --quiet mode.
// NewQuietRenderer 创建一个用于 --quiet 模式的极简渲染器。
func NewQuietRenderer(w io.Writer, tr *i18n.Translator) *Quiet {
	return &Quiet{w: w, tr: resolveTranslator(tr)}
}

// Render prints errors and final summary only.
// Render 仅打印错误与最终汇总。
func (q *Quiet) Render(e model.Event) {
	q.mu.Lock()
	defer q.mu.Unlock()
	switch e.Event {
	case model.EvError:
		fmt.Fprintf(q.w, "%s\n", q.tr.Textf("%s - error - %s", "%s - 错误 - %s", e.CleanerID, e.Detail))
	case model.EvSummary:
		p := &Plain{w: q.w, tr: q.tr}
		p.renderSummary(e)
	}
}

// Close satisfies the ui.Renderer interface.
// Close 实现 ui.Renderer 接口。
func (q *Quiet) Close() error { return nil }
