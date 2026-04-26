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
	styleSkip = lipgloss.NewStyle().Foreground(lipgloss.Color("#a1a1aa"))
	styleErr  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
)

// Spinner renders cleaner events as an inline spinner with status updates.
// Spinner 将清理器事件渲染为带有状态更新的内联旋转指示器。
type Spinner struct {
	mu      sync.Mutex
	w       io.Writer
	current map[string]model.Event
}

// NewSpinner creates a new Spinner that writes to the given io.Writer.
// NewSpinner 创建一个新的 Spinner，写入给定的 io.Writer。
func NewSpinner(w io.Writer) *Spinner {
	return &Spinner{w: w, current: map[string]model.Event{}}
}

func (s *Spinner) Render(e model.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch e.Event {
	case model.EvStart:
		s.current[e.CleanerID] = e
		_, _ = fmt.Fprintf(s.w, "  ... %s\r", e.Name)
	case model.EvFinish:
		start := s.current[e.CleanerID]
		name := start.Name
		if name == "" {
			name = e.CleanerID
		}
		if e.Status == "skipped" || e.Status == "error" {
			delete(s.current, e.CleanerID)
			return
		}
		_, _ = fmt.Fprintf(s.w, "  %s %s - %s freed\n", styleOK.Render("[ok]"), name, humanize.Bytes(safeBytes(e.BytesFreed)))
		delete(s.current, e.CleanerID)
	case model.EvCleanerSkipped:
		start := s.current[e.CleanerID]
		name := start.Name
		if name == "" {
			name = e.CleanerID
		}
		_, _ = fmt.Fprintf(s.w, "  %s %s - skipped (%s)\n", styleSkip.Render("[skip]"), name, e.Reason)
	case model.EvError:
		_, _ = fmt.Fprintf(s.w, "  %s %s - %s\n", styleErr.Render("[err]"), e.CleanerID, e.Detail)
	case model.EvSummary:
		_, _ = fmt.Fprintf(s.w, "\nFreed %s across %d cleaners.\n", humanize.Bytes(safeBytes(e.BytesFreed)), e.CleanersRun)
	}
}

func (s *Spinner) Close() error {
	return nil
}
