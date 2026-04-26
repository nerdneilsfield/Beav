package cli

import (
	"fmt"
	"io"
	"sync"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dustin/go-humanize"
)

type Plain struct {
	mu      sync.Mutex
	w       io.Writer
	current map[string]model.Event
}

func NewPlain(w io.Writer) *Plain {
	return &Plain{w: w, current: map[string]model.Event{}}
}

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
		if e.Status == "skipped" || e.Status == "error" {
			delete(p.current, e.CleanerID)
			return
		}
		_, _ = fmt.Fprintf(p.w, "%s - %s - %s freed (%d files)\n", name, e.Status, humanize.Bytes(safeBytes(e.BytesFreed)), e.FilesDeleted)
		delete(p.current, e.CleanerID)
	case model.EvCleanerSkipped:
		start := p.current[e.CleanerID]
		name := start.Name
		if name == "" {
			name = e.CleanerID
		}
		_, _ = fmt.Fprintf(p.w, "%s - skipped (%s)\n", name, e.Reason)
	case model.EvError:
		_, _ = fmt.Fprintf(p.w, "%s - error - %s: %s\n", e.CleanerID, e.Reason, e.Detail)
	case model.EvSummary:
		_, _ = fmt.Fprintf(p.w, "Total: %d cleaners - %s freed\n", e.CleanersRun, humanize.Bytes(safeBytes(e.BytesFreed)))
	}
}

func (p *Plain) Close() error {
	return nil
}
