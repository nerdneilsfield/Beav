package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestPlainOneLinePerCleaner(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "demo", Name: "Demo Cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "demo", Status: "ok", FilesDeleted: 3, BytesFreed: 1500})
	r.Render(model.Event{Event: model.EvSummary, CleanersRun: 1, BytesFreed: 1500})

	got := buf.String()
	if !strings.Contains(got, "Demo Cache") {
		t.Errorf("missing name: %q", got)
	}
	if !strings.Contains(got, "ok") || !strings.Contains(got, "1.5 kB") {
		t.Errorf("missing status/size: %q", got)
	}
}

func TestSafeBytesClampsNegativeValues(t *testing.T) {
	if got := safeBytes(-1); got != 0 {
		t.Fatalf("safeBytes(-1) = %d, want 0", got)
	}
}
