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
	if !strings.Contains(buf.String(), "(x)") {
		t.Fatalf("result line must carry the cleaner ID: %q", buf.String())
	}
}

// TestSpinnerDryRunShowsPlanLine verifies that a dry run reports candidates as a plan.
// TestSpinnerDryRunShowsPlanLine 验证 dry run 将候选报告为计划行。
func TestSpinnerDryRunShowsPlanLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinner(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "lang-pip", Name: "pip wheel cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "lang-pip", Status: "ok", DryRun: true, FilesCandidate: 1011, BytesCandidate: 263580931})
	r.Render(model.Event{Event: model.EvSummary, DryRun: true})

	got := buf.String()
	if !strings.Contains(got, "pip wheel cache (lang-pip)") {
		t.Fatalf("missing plan line name/ID: %q", got)
	}
	if strings.Contains(got, "freed") {
		t.Fatalf("dry run must not claim freed bytes: %q", got)
	}
}

// TestSpinnerAggregatesNoopCleaners verifies that zero-result cleaners stay in
// one summary line.
// TestSpinnerAggregatesNoopCleaners 验证零结果清理器只出现在一行汇总中。
func TestSpinnerAggregatesNoopCleaners(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinner(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "a", Name: "Cache A", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "a", Status: "ok"})
	r.Render(model.Event{Event: model.EvStart, CleanerID: "b", Name: "Cache B", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "b", Status: "ok"})
	r.Render(model.Event{Event: model.EvSummary, CleanersRun: 2})

	got := buf.String()
	if !strings.Contains(got, "nothing to do: Cache A, Cache B") {
		t.Fatalf("missing aggregated no-op line: %q", got)
	}
}

// TestSpinnerDoesNotPrintOKLineAfterCleanerSkipped verifies that skipped cleaners don't produce an [ok] line.
// TestSpinnerDoesNotPrintOKLineAfterCleanerSkipped 验证跳过的清理器不产生 [ok] 行。
func TestSpinnerDoesNotPrintOKLineAfterCleanerSkipped(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinner(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "docker-builder", Name: "Docker builder cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvCleanerSkipped, CleanerID: "docker-builder", Reason: "runtime_busy"})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "docker-builder", Status: "skipped", BytesFreed: 0})
	r.Render(model.Event{Event: model.EvSummary})

	got := buf.String()
	if strings.Contains(got, "[ok]") || strings.Contains(got, "0 B freed") {
		t.Fatalf("skipped cleaner should not print an ok/freed line: %q", got)
	}
	if !strings.Contains(got, "runtime busy") {
		t.Fatalf("grouped skip line missing: %q", got)
	}
}
