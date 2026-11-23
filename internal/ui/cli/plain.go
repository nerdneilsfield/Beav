package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dustin/go-humanize"
)

type Plain struct {
	mu      sync.Mutex
	w       io.Writer
	current map[string]model.Event
	noop    []string
	skips   map[string][]string
}

func NewPlain(w io.Writer) *Plain {
	return &Plain{w: w, current: map[string]model.Event{}, skips: map[string][]string{}}
}

// Render processes a cleaner event and writes the appropriate output.
// Render 处理清理器事件并写入相应输出。
func (p *Plain) Render(e model.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch e.Event {
	case model.EvStart:
		p.current[e.CleanerID] = e
	case model.EvFinish:
		start := p.current[e.CleanerID]
		name := start.Name
		if name == "" {
			name = e.CleanerID
		}
		delete(p.current, e.CleanerID)
		if e.Status == "skipped" || e.Status == "error" {
			return
		}
		switch {
		case e.DryRun && e.FilesCandidate > 0:
			fmt.Fprintf(p.w, "%s (%s) - plan - %s in %d files\n", name, e.CleanerID, humanize.Bytes(safeBytes(e.BytesCandidate)), e.FilesCandidate)
		case e.DryRun:
			p.noop = append(p.noop, name)
		case e.FilesDeleted > 0 || e.BytesFreed > 0:
			fmt.Fprintf(p.w, "%s (%s) - ok - %s freed (%d files)\n", name, e.CleanerID, humanize.Bytes(safeBytes(e.BytesFreed)), e.FilesDeleted)
		default:
			p.noop = append(p.noop, name)
		}
	case model.EvCleanerSkipped:
		start := p.current[e.CleanerID]
		name := start.Name
		if name == "" {
			name = e.CleanerID
		}
		p.skips[e.Reason] = append(p.skips[e.Reason], name)
	case model.EvError:
		start := p.current[e.CleanerID]
		name := start.Name
		if name == "" {
			name = e.CleanerID
		}
		fmt.Fprintf(p.w, "%s (%s) - error - %s\n", name, e.CleanerID, e.Detail)
	case model.EvSummary:
		p.flushGroups()
		if e.DryRun {
			fmt.Fprintf(p.w, "Total: %d ran - %s in %d files\n", e.CleanersRun, humanize.Bytes(safeBytes(e.BytesCandidate)), e.FilesCandidate)
			return
		}
		fmt.Fprintf(p.w, "Total: %d ran - %s freed\n", e.CleanersRun, humanize.Bytes(safeBytes(e.BytesFreed)))
	}
}

// flushGroups prints the quiet no-op cleaners and the grouped skip reasons.
// flushGroups 打印无事可做的清理器与按原因分组的跳过项。
func (p *Plain) flushGroups() {
	if len(p.noop) > 0 {
		fmt.Fprintf(p.w, "Nothing to do: %s\n", joinNames(p.noop))
		p.noop = nil
	}
	for _, reason := range sortedKeys(p.skips) {
		names := p.skips[reason]
		line := fmt.Sprintf("Skipped (%s): %s", skipPhrase(reason), joinNames(names))
		if remedy := skipRemedy(reason); remedy != "" {
			line += " - " + remedy
		}
		fmt.Fprintf(p.w, "%s\n", line)
	}
	p.skips = map[string][]string{}
}

func (p *Plain) Close() error {
	return nil
}

// joinNames lists up to four names plus an "and N more" tail.
// joinNames 最多列出四个名称，其余以 "and N more" 结尾。
func joinNames(names []string) string {
	const max = 4
	if len(names) <= max {
		return strings.Join(names, ", ")
	}
	return strings.Join(names[:max], ", ") + fmt.Sprintf(", and %d more", len(names)-max)
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
