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

// TestPlainDoesNotPrintFinishLineAfterCleanerSkipped verifies that skipped cleaners don't produce a finish line.
// TestPlainDoesNotPrintFinishLineAfterCleanerSkipped 验证跳过的清理器不产生完成行。
func TestPlainDoesNotPrintFinishLineAfterCleanerSkipped(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "docker-builder", Name: "Docker builder cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvCleanerSkipped, CleanerID: "docker-builder", Reason: "runtime_busy", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "docker-builder", Status: "skipped", BytesFreed: 0, TS: time.Now()})

	got := buf.String()
	if strings.Count(got, "Docker builder cache") != 1 {
		t.Fatalf("got duplicate cleaner output: %q", got)
	}
	if strings.Contains(got, "0 B freed") {
		t.Fatalf("skipped cleaner should not print a freed line: %q", got)
	}
}

func TestSafeBytesClampsNegativeValues(t *testing.T) {
	if got := safeBytes(-1); got != 0 {
		t.Fatalf("safeBytes(-1) = %d, want 0", got)
	}
}
