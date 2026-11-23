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
	if !strings.Contains(got, "(demo)") {
		t.Errorf("result line must carry the cleaner ID for --only/--skip: %q", got)
	}
	if !strings.Contains(got, "ok") || !strings.Contains(got, "1.5 kB") {
		t.Errorf("missing status/size: %q", got)
	}
}

// TestPlainDryRunShowsPlanLine verifies that a dry run reports candidates as a plan.
// TestPlainDryRunShowsPlanLine 验证 dry run 将候选报告为计划行。
func TestPlainDryRunShowsPlanLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "lang-pip", Name: "pip wheel cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "lang-pip", Status: "ok", DryRun: true, FilesCandidate: 1011, BytesCandidate: 263580931})
	r.Render(model.Event{Event: model.EvSummary, DryRun: true, FilesCandidate: 1011, BytesCandidate: 263580931})

	got := buf.String()
	if !strings.Contains(got, "pip wheel cache (lang-pip) - plan - 264 MB in 1011 files") {
		t.Errorf("missing dry-run plan line: %q", got)
	}
	if strings.Contains(got, " - ok - ") {
		t.Errorf("dry run must not print an ok/freed cleaner line: %q", got)
	}
}

// TestPlainAggregatesNoopCleaners verifies that cleaners with nothing to do
// only appear in one summary line.
// TestPlainAggregatesNoopCleaners 验证无事可做的清理器只出现在一行汇总中。
func TestPlainAggregatesNoopCleaners(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "a", Name: "Cache A", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "a", Status: "ok"})
	r.Render(model.Event{Event: model.EvStart, CleanerID: "b", Name: "Cache B", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "b", Status: "ok"})
	r.Render(model.Event{Event: model.EvSummary, CleanersRun: 2})

	got := buf.String()
	if strings.Contains(got, "Cache A - ok") || strings.Contains(got, "Cache B - ok") {
		t.Fatalf("zero-result cleaners must not get their own lines: %q", got)
	}
	if !strings.Contains(got, "Nothing to do: Cache A, Cache B") {
		t.Fatalf("missing aggregated no-op line: %q", got)
	}
}

// TestPlainGroupsSkipsWithRemedy verifies that skips are grouped with a human
// phrase and the action that would unblock them.
// TestPlainGroupsSkipsWithRemedy 验证跳过项按原因分组并附带补救提示。
func TestPlainGroupsSkipsWithRemedy(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "docker-builder", Name: "Docker builder cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvCleanerSkipped, CleanerID: "docker-builder", Reason: "runtime_busy"})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "docker-builder", Status: "skipped"})
	r.Render(model.Event{Event: model.EvSummary})

	got := buf.String()
	if strings.Contains(got, "Docker builder cache - ok") {
		t.Fatalf("skipped cleaner must not print a freed line: %q", got)
	}
	if !strings.Contains(got, "Skipped (runtime busy): Docker builder cache") {
		t.Fatalf("missing grouped skip line: %q", got)
	}
	if !strings.Contains(got, "pass --include-heavy") && !strings.Contains(got, "runtime busy") {
		t.Fatalf("skip groups should stay readable: %q", got)
	}
}

// TestPlainHeavySkipNamesRemedy verifies the heavy skip carries the unlock flag.
// TestPlainHeavySkipNamesRemedy 验证 heavy 跳过行附带解锁标志。
func TestPlainHeavySkipNamesRemedy(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "rustup", Name: "Rust toolchains", TS: time.Now()})
	r.Render(model.Event{Event: model.EvCleanerSkipped, CleanerID: "rustup", Reason: "heavy_not_authorized"})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "rustup", Status: "skipped"})
	r.Render(model.Event{Event: model.EvSummary})

	got := buf.String()
	if !strings.Contains(got, "pass --include-heavy or put the exact ID in --only") {
		t.Fatalf("heavy skip must tell the user how to authorize it: %q", got)
	}
}

func TestSafeBytesClampsNegativeValues(t *testing.T) {
	if got := safeBytes(-1); got != 0 {
		t.Fatalf("safeBytes(-1) = %d, want 0", got)
	}
}
