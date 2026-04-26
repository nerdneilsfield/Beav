package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestSpinnerProducesFinishLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinner(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "x", Name: "X", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "x", Status: "ok", BytesFreed: 1024})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if buf.Len() == 0 {
		t.Fatal("no output")
	}
}

func TestSpinnerDoesNotPrintOKLineAfterCleanerSkipped(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinner(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "docker-builder", Name: "Docker builder cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvCleanerSkipped, CleanerID: "docker-builder", Reason: "runtime_busy", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "docker-builder", Status: "skipped", BytesFreed: 0, TS: time.Now()})

	got := buf.String()
	if strings.Count(got, "Docker builder cache") != 2 {
		t.Fatalf("expected start + skip output only, got: %q", got)
	}
	if strings.Contains(got, "[ok]") || strings.Contains(got, "0 B freed") {
		t.Fatalf("skipped cleaner should not print an ok/freed line: %q", got)
	}
}
