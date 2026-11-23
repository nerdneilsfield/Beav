package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dustin/go-humanize"
)

// Color styles for different event statuses.
// 不同事件状态的颜色样式。
var (
	styleOK    = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	styleSkip  = lipgloss.NewStyle().Foreground(lipgloss.Color("#a1a1aa"))
	styleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
	stylePlan  = lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308"))
	styleQuiet = lipgloss.NewStyle().Foreground(lipgloss.Color("#71717a"))
)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Spinner renders cleaner events as an inline animated spinner with clean line clearing.
// Spinner 将清理器事件渲染为带清行控制的动态终端指示器。
type Spinner struct {
	mu         sync.Mutex
	w          io.Writer
	tr         *i18n.Translator
	verbose    bool
	animate    bool
	current    map[string]*cleanerState
	noop       []string
	skips      map[string][]skipEntry
	inProgress bool
	activeName string
	closed     bool
	stopCh     chan struct{}
	stopOnce   sync.Once
	wg         sync.WaitGroup
}

// NewSpinner creates a new Spinner with deterministic non-animated output.
// NewSpinner 创建一个使用确定性非定时动画输出的 Spinner。
func NewSpinner(w io.Writer) *Spinner {
	return NewSpinnerWithOptions(w, Options{})
}

// NewSpinnerWithOptions creates a Spinner configured with the given Options.
// NewSpinnerWithOptions 创建一个按给定 Options 配置的 Spinner。
func NewSpinnerWithOptions(w io.Writer, opts Options) *Spinner {
	s := &Spinner{
		w:       w,
		tr:      resolveTranslator(opts.Translator),
		verbose: opts.Verbose,
		animate: opts.Animate,
		current: map[string]*cleanerState{},
		skips:   map[string][]skipEntry{},
	}
	if opts.Animate {
		s.stopCh = make(chan struct{})
		s.wg.Add(1)
		go s.runTicker()
	}
	return s
}

func (s *Spinner) runTicker() {
	defer s.wg.Done()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	frame := 0
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.mu.Lock()
			if s.inProgress && !s.closed {
				glyph := spinnerFrames[frame%len(spinnerFrames)]
				frame++
				fmt.Fprintf(s.w, "\x1b[2K\r  %s %s", stylePlan.Render(glyph), s.activeName)
			}
			s.mu.Unlock()
		}
	}
}

func (s *Spinner) clearProgressLocked() {
	if s.inProgress {
		fmt.Fprint(s.w, "\x1b[2K\r")
		s.inProgress = false
	}
}

// Render processes one cleaner event and updates the terminal output.
// Render 处理单个清理器事件并更新终端输出。
func (s *Spinner) Render(e model.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}

	switch e.Event {
	case model.EvStart:
		s.current[e.CleanerID] = &cleanerState{start: e}
		s.activeName = displayName(s.tr, e.CleanerID, e.Name)
		s.inProgress = true
		if s.animate {
			fmt.Fprintf(s.w, "\x1b[2K\r  %s %s", stylePlan.Render(spinnerFrames[0]), s.activeName)
		} else {
			fmt.Fprintf(s.w, "\x1b[2K\r  ... %s\r", s.activeName)
		}
	case model.EvDeleted:
		if st := s.current[e.CleanerID]; st != nil && s.verbose {
			st.sampleTotal++
			if len(st.samples) < verboseDetailLimit {
				st.samples = append(st.samples, fmt.Sprintf("%s (%s)", e.Path, humanize.Bytes(safeBytes(e.Size))))
			}
		}
	case model.EvSkipped:
		if e.Reason == string(model.SkipReasonDryRun) {
			if st := s.current[e.CleanerID]; st != nil && s.verbose {
				st.sampleTotal++
				if len(st.samples) < verboseDetailLimit {
					st.samples = append(st.samples, fmt.Sprintf("%s (%s)", e.Path, humanize.Bytes(safeBytes(e.Size))))
				}
			}
		}
	case model.EvCommandOutput:
		if st := s.current[e.CleanerID]; st != nil {
			line := strings.TrimSpace(e.Line)
			if line != "" {
				st.cmdTotal++
				if len(st.cmdLines) < verboseDetailLimit {
					st.cmdLines = append(st.cmdLines, line)
				}
			}
		}
	case model.EvFinish:
		st := s.current[e.CleanerID]
		var startName string
		if st != nil {
			startName = st.start.Name
		}
		name := displayName(s.tr, e.CleanerID, startName)
		delete(s.current, e.CleanerID)
		if e.Status == "skipped" || e.Status == "error" {
			s.clearProgressLocked()
			return
		}
		s.renderFinishLocked(e, st, name)
	case model.EvCleanerSkipped:
		st := s.current[e.CleanerID]
		var startName string
		if st != nil {
			startName = st.start.Name
		}
		name := displayName(s.tr, e.CleanerID, startName)
		s.skips[e.Reason] = append(s.skips[e.Reason], skipEntry{
			ID:     e.CleanerID,
			Name:   name,
			Detail: e.Detail,
		})
	case model.EvError:
		s.clearProgressLocked()
		st := s.current[e.CleanerID]
		var startName string
		if st != nil {
			startName = st.start.Name
		}
		name := displayName(s.tr, e.CleanerID, startName)
		fmt.Fprintf(s.w, "  %s %s (%s) - %s\n", styleErr.Render("[err]"), name, e.CleanerID, e.Detail)
	case model.EvSummary:
		s.clearProgressLocked()
		s.flushGroups()
		s.renderSummary(e)
	}
}

func (s *Spinner) renderFinishLocked(e model.Event, st *cleanerState, name string) {
	switch {
	case e.DryRun && e.FilesCandidate > 0:
		s.clearProgressLocked()
		partial := ""
		if e.MetricsIncomplete {
			partial = s.tr.Text(" (partial)", "（部分估算）")
		}
		fmt.Fprintf(s.w, "  %s %s\n",
			stylePlan.Render(s.tr.Text("[plan]", "[预演]")),
			s.tr.Textf("%s (%s) - %s in %s%s",
				"%s (%s) - %s（%s）%s",
				name, e.CleanerID,
				humanize.Bytes(safeBytes(e.BytesCandidate)),
				s.tr.Plural(e.FilesCandidate, "file", "files", "个文件"),
				partial,
			),
		)
		s.printDetailsLocked(st, verboseDetailLimit)
	case e.DryRun && e.MetricsIncomplete:
		s.clearProgressLocked()
		fmt.Fprintf(s.w, "  %s %s (%s) - %s\n",
			stylePlan.Render(s.tr.Text("[plan]", "[预演]")),
			name, e.CleanerID,
			s.tr.Text("native command (size unmeasured)", "原生命令（无法预估大小）"),
		)
		limit := previewCmdLimit
		if s.verbose {
			limit = verboseDetailLimit
		}
		s.printCommandLinesLocked(st, limit)
	case e.DryRun:
		s.clearProgressLocked()
		s.noop = append(s.noop, name)
	case e.FilesDeleted > 0 || e.BytesFreed > 0:
		s.clearProgressLocked()
		partial := ""
		if e.MetricsIncomplete {
			partial = s.tr.Text(" (partial)", "（部分估算）")
		}
		fmt.Fprintf(s.w, "  %s %s (%s) - %s %s (%s)%s\n",
			styleOK.Render(s.tr.Text("[ok]", "[完成]")),
			name, e.CleanerID,
			humanize.Bytes(safeBytes(e.BytesFreed)),
			s.tr.Text("freed", "已释放"),
			s.tr.Plural(e.FilesDeleted, "file", "files", "个文件"),
			partial,
		)
		s.printDetailsLocked(st, verboseDetailLimit)
	case e.MetricsIncomplete:
		s.clearProgressLocked()
		fmt.Fprintf(s.w, "  %s %s (%s) - %s\n",
			styleOK.Render(s.tr.Text("[ok]", "[完成]")),
			name, e.CleanerID,
			s.tr.Text("native command completed (size unmeasured)", "原生命令已执行（未计量字节数）"),
		)
		if s.verbose {
			s.printCommandLinesLocked(st, verboseDetailLimit)
		}
	default:
		s.clearProgressLocked()
		s.noop = append(s.noop, name)
	}
}

func (s *Spinner) printDetailsLocked(st *cleanerState, limit int) {
	if !s.verbose || st == nil {
		return
	}
	if len(st.samples) > 0 {
		for _, line := range st.samples {
			fmt.Fprintf(s.w, "      %s\n", styleQuiet.Render(line))
		}
		if st.sampleTotal > len(st.samples) {
			fmt.Fprintf(s.w, "      %s\n", styleQuiet.Render(s.tr.Textf("... and %d more files", "... 另有 %d 个文件", st.sampleTotal-len(st.samples))))
		}
	}
	s.printCommandLinesLocked(st, limit)
}

func (s *Spinner) printCommandLinesLocked(st *cleanerState, limit int) {
	if st == nil || len(st.cmdLines) == 0 {
		return
	}
	n := len(st.cmdLines)
	if n > limit {
		n = limit
	}
	for _, line := range st.cmdLines[:n] {
		fmt.Fprintf(s.w, "      %s\n", styleQuiet.Render(line))
	}
	if st.cmdTotal > n {
		fmt.Fprintf(s.w, "      %s\n", styleQuiet.Render(s.tr.Textf("... and %d more lines", "... 另有 %d 行输出", st.cmdTotal-n)))
	}
}

func (s *Spinner) flushGroups() {
	if len(s.noop) > 0 {
		fmt.Fprintf(s.w, "  %s %s: %s\n",
			styleQuiet.Render("·"),
			s.tr.Text("nothing to do", "无需清理"),
			joinNames(s.tr, s.noop, s.verbose),
		)
		s.noop = nil
	}
	skipTag := styleSkip.Render(s.tr.Text("[skip]", "[跳过]"))
	for _, reason := range sortedSkipReasons(s.skips, false) {
		entries := s.skips[reason]
		line := fmt.Sprintf("%s - %s", skipPhrase(s.tr, reason), joinSkipEntries(s.tr, entries, s.verbose))
		if remedy := skipRemedy(s.tr, reason); remedy != "" {
			line += " - " + remedy
		}
		fmt.Fprintf(s.w, "  %s %s\n", skipTag, line)
		if s.verbose {
			for _, ent := range entries {
				if ent.Detail != "" {
					fmt.Fprintf(s.w, "      %s: %s\n", styleQuiet.Render(ent.ID), styleQuiet.Render(ent.Detail))
				}
			}
		}
	}

	quietReasons := sortedSkipReasons(s.skips, true)
	if len(quietReasons) > 0 {
		if !s.verbose {
			total := 0
			for _, r := range quietReasons {
				total += len(s.skips[r])
			}
			line := fmt.Sprintf("%s - %s",
				s.tr.Text("not installed / no cache", "未安装或无缓存"),
				s.tr.Textf("%s - use --verbose to list", "%s - 加 --verbose 查看明细",
					s.tr.Plural(int64(total), "cleaner", "cleaners", "项")),
			)
			fmt.Fprintf(s.w, "  %s %s\n", skipTag, line)
		} else {
			for _, reason := range quietReasons {
				entries := s.skips[reason]
				line := fmt.Sprintf("%s - %s", skipPhrase(s.tr, reason), joinSkipEntries(s.tr, entries, true))
				fmt.Fprintf(s.w, "  %s %s\n", skipTag, line)
				for _, ent := range entries {
					if ent.Detail != "" {
						fmt.Fprintf(s.w, "      %s: %s\n", styleQuiet.Render(ent.ID), styleQuiet.Render(ent.Detail))
					}
				}
			}
		}
	}
	s.skips = map[string][]skipEntry{}
}

func (s *Spinner) renderSummary(e model.Event) {
	tail := formatSummaryTail(s.tr, e)
	if e.DryRun {
		fmt.Fprintf(s.w, "\n%s\n",
			s.tr.Textf("Plan: %s in %s across %s%s.",
				"预演计划: 共 %s（%s），涉及 %s%s。",
				humanize.Bytes(safeBytes(e.BytesCandidate)),
				s.tr.Plural(e.FilesCandidate, "file", "files", "个文件"),
				s.tr.Plural(int64(e.CleanersRun), "cleaner", "cleaners", "项"),
				tail,
			),
		)
	} else {
		fmt.Fprintf(s.w, "\n%s\n",
			s.tr.Textf("Freed %s (%s) across %s%s.",
				"已释放 %s（%s），共处理 %s%s。",
				humanize.Bytes(safeBytes(e.BytesFreed)),
				s.tr.Plural(e.FilesDeleted, "file", "files", "个文件"),
				s.tr.Plural(int64(e.CleanersRun), "cleaner", "cleaners", "项"),
				tail,
			),
		)
	}
	if e.MetricsIncomplete && len(e.UnmeasuredCleaners) > 0 {
		fmt.Fprintf(s.w, "%s\n",
			s.tr.Textf("Note: exact byte sizes unavailable for native commands: %s.",
				"提示: 以下原生命令无法精确统计字节数: %s。",
				strings.Join(e.UnmeasuredCleaners, ", "),
			),
		)
	}
}

// Close stops any active spinner animation and clears leftover progress text. It is safe to call repeatedly.
// Close 停止动态旋转动画并清除残留进度行，可安全重复调用。
func (s *Spinner) Close() error {
	s.stopOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.clearProgressLocked()
		s.mu.Unlock()
		if s.stopCh != nil {
			close(s.stopCh)
		}
		s.wg.Wait()
	})
	return nil
}
