// Package cli provides CLI-based renderers for cleaner events.
// Package cli 提供用于清理器事件的基于 CLI 的渲染器。
package cli

import (
	"fmt"
	"io"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dustin/go-humanize"
)

// Color styles for different event statuses.
// 不同事件状态的颜色样式。
var (
	styleOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	styleSkip  = lipgloss.NewStyle().Foreground(lipgloss.Color("#a1a1aa"))
	styleErr   = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
	stylePlan  = lipgloss.NewStyle().Foreground(lipgloss.Color("#eab308"))
	styleQuiet = lipgloss.NewStyle().Foreground(lipgloss.Color("#71717a"))
)

// Spinner renders cleaner events as an inline spinner with status updates.
// Spinner 将清理器事件渲染为带有状态更新的内联旋转指示器。
type Spinner struct {
	mu      sync.Mutex
	w       io.Writer
	current map[string]model.Event
	noop    []string
	skips   map[string][]string
}

// NewSpinner creates a new Spinner that writes to the given io.Writer.
// NewSpinner 创建一个新的 Spinner，写入给定的 io.Writer。
func NewSpinner(w io.Writer) *Spinner {
	return &Spinner{w: w, current: map[string]model.Event{}, skips: map[string][]string{}}
}

func (s *Spinner) Render(e model.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch e.Event {
	case model.EvStart:
		s.current[e.CleanerID] = e
		fmt.Fprintf(s.w, "  ... %s\r", e.Name)
	case model.EvFinish:
		start := s.current[e.CleanerID]
		name := start.Name
		if name == "" {
			name = e.CleanerID
		}
		delete(s.current, e.CleanerID)
		if e.Status == "skipped" || e.Status == "error" {
			return
		}
		switch {
		case e.DryRun && e.FilesCandidate > 0:
			fmt.Fprintf(s.w, "  %s %s (%s) - %s in %d files\n", stylePlan.Render("[plan]"), name, e.CleanerID, humanize.Bytes(safeBytes(e.BytesCandidate)), e.FilesCandidate)
		case e.DryRun:
			s.noop = append(s.noop, name)
		case e.FilesDeleted > 0 || e.BytesFreed > 0:
			fmt.Fprintf(s.w, "  %s %s (%s) - %s freed\n", styleOK.Render("[ok]"), name, e.CleanerID, humanize.Bytes(safeBytes(e.BytesFreed)))
		default:
			s.noop = append(s.noop, name)
		}
	case model.EvCleanerSkipped:
		start := s.current[e.CleanerID]
		name := start.Name
		if name == "" {
			name = e.CleanerID
		}
		s.skips[e.Reason] = append(s.skips[e.Reason], name)
	case model.EvError:
		start := s.current[e.CleanerID]
		name := start.Name
		if name == "" {
			name = e.CleanerID
		}
		fmt.Fprintf(s.w, "  %s %s (%s) - %s\n", styleErr.Render("[err]"), name, e.CleanerID, e.Detail)
	case model.EvSummary:
		s.flushGroups()
		if e.DryRun {
			fmt.Fprintf(s.w, "\nPlan: %s in %d files across %d cleaners.\n", humanize.Bytes(safeBytes(e.BytesCandidate)), e.FilesCandidate, e.CleanersRun)
			return
		}
		fmt.Fprintf(s.w, "\nFreed %s across %d cleaners.\n", humanize.Bytes(safeBytes(e.BytesFreed)), e.CleanersRun)
	}
}

// flushGroups prints the quiet no-op cleaners and the grouped skip reasons.
// flushGroups 打印无事可做的清理器与按原因分组的跳过项。
func (s *Spinner) flushGroups() {
	if len(s.noop) > 0 {
		fmt.Fprintf(s.w, "  %s nothing to do: %s\n", styleQuiet.Render("·"), joinNames(s.noop))
		s.noop = nil
	}
	for _, reason := range sortedKeys(s.skips) {
		names := s.skips[reason]
		line := fmt.Sprintf("%s - %s", skipPhrase(reason), joinNames(names))
		if remedy := skipRemedy(reason); remedy != "" {
			line += " - " + remedy
		}
		fmt.Fprintf(s.w, "  %s %s\n", styleSkip.Render("[skip]"), line)
	}
	s.skips = map[string][]string{}
}

func (s *Spinner) Close() error {
	return nil
}
