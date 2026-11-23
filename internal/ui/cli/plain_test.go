package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
)

func TestPlainOneLinePerCleaner(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "demo", Name: "Demo Cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "demo", Status: "ok", FilesDeleted: 3, BytesFreed: 1500})
	r.Render(model.Event{Event: model.EvSummary, CleanersRun: 1, FilesDeleted: 3, BytesFreed: 1500})

	got := buf.String()
	if !strings.Contains(got, "Demo Cache") {
		t.Errorf("missing name: %q", got)
	}
	if !strings.Contains(got, "(demo)") {
		t.Errorf("result line must carry the cleaner ID for --only/--skip: %q", got)
	}
	if !strings.Contains(got, "ok") || !strings.Contains(got, "1.5 kB") || !strings.Contains(got, "3 files") {
		t.Errorf("missing status/size/count: %q", got)
	}
	if !strings.Contains(got, "1 cleaner") {
		t.Errorf("expected singular '1 cleaner', got %q", got)
	}
}

// TestPlainDryRunShowsPlanLine verifies that a dry run reports candidates as a plan.
// TestPlainDryRunShowsPlanLine 验证 dry run 将候选报告为计划行。
func TestPlainDryRunShowsPlanLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "lang-pip", Name: "pip wheel cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "lang-pip", Status: "ok", DryRun: true, FilesCandidate: 1011, BytesCandidate: 263580931})
	r.Render(model.Event{Event: model.EvSummary, DryRun: true, CleanersRun: 1, FilesCandidate: 1011, BytesCandidate: 263580931})

	got := buf.String()
	if !strings.Contains(got, "pip wheel cache (lang-pip) - plan - 264 MB in 1011 files") {
		t.Errorf("missing dry-run plan line: %q", got)
	}
	if strings.Contains(got, " - ok - ") {
		t.Errorf("dry run must not print an ok/freed cleaner line: %q", got)
	}
}

// TestPlainMetricsIncompleteNotTreatedAsNoop verifies native cleaners with unmeasured metrics are printed explicitly.
// TestPlainMetricsIncompleteNotTreatedAsNoop 验证未计量的原生命令清理器不会被误归为无需清理。
func TestPlainMetricsIncompleteNotTreatedAsNoop(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "lang-uv", Name: "uv cache prune", TS: time.Now()})
	r.Render(model.Event{Event: model.EvCommandOutput, CleanerID: "lang-uv", Stream: "stdout", Line: "[dry-run] uv cache prune"})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "lang-uv", Status: "ok", DryRun: true, MetricsIncomplete: true})
	r.Render(model.Event{Event: model.EvSummary, DryRun: true, CleanersRun: 1, MetricsIncomplete: true, UnmeasuredCleaners: []string{"lang-uv"}})

	got := buf.String()
	if strings.Contains(got, "Nothing to do") {
		t.Fatalf("native incomplete cleaner must not appear in Nothing to do: %q", got)
	}
	if !strings.Contains(got, "uv cache prune (lang-uv)") || !strings.Contains(got, "size unmeasured") {
		t.Fatalf("missing native unmeasured plan line: %q", got)
	}
	if !strings.Contains(got, "[dry-run] uv cache prune") {
		t.Fatalf("missing command preview line: %q", got)
	}
	if !strings.Contains(got, "lang-uv") {
		t.Fatalf("summary note must mention unmeasured cleaner ID: %q", got)
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

// TestPlainGroupsSkipsWithRemedyAndID verifies that actionable skips include IDs and quiet skips collapse by default.
// TestPlainGroupsSkipsWithRemedyAndID 验证可干预的跳过项附带清理器 ID，而常规未安装/无缓存项默认折叠汇总。
func TestPlainGroupsSkipsWithRemedyAndID(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)

	r.Render(model.Event{Event: model.EvStart, CleanerID: "lang-rustup", Name: "Rust toolchains", TS: time.Now()})
	r.Render(model.Event{Event: model.EvCleanerSkipped, CleanerID: "lang-rustup", Reason: "heavy_not_authorized"})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "lang-rustup", Status: "skipped"})

	r.Render(model.Event{Event: model.EvStart, CleanerID: "lang-conda", Name: "Conda index and archive cleanup", TS: time.Now()})
	r.Render(model.Event{Event: model.EvCleanerSkipped, CleanerID: "lang-conda", Reason: "manager_not_installed"})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "lang-conda", Status: "skipped"})
	r.Render(model.Event{Event: model.EvSummary, CleanersSkipped: 2})

	got := buf.String()
	if !strings.Contains(got, "Rust toolchains (lang-rustup)") {
		t.Fatalf("heavy skip must carry cleaner ID: %q", got)
	}
	if !strings.Contains(got, "pass --include-heavy or --only <id>") {
		t.Fatalf("heavy skip must tell user how to authorize: %q", got)
	}
	if strings.Contains(got, "install the tool") {
		t.Fatalf("must not advise user to install missing tools: %q", got)
	}
	if !strings.Contains(got, "not installed / no cache") || !strings.Contains(got, "--verbose") {
		t.Fatalf("quiet skips should collapse with --verbose hint: %q", got)
	}
}

func TestPlainChineseAndVerbose(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlainWithOptions(&buf, Options{
		Translator: i18n.New(i18n.LangZH),
		Verbose:    true,
	})

	r.Render(model.Event{Event: model.EvStart, CleanerID: "lang-pip", Name: "pip wheel cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvSkipped, CleanerID: "lang-pip", Path: "/tmp/a.whl", Reason: "dry_run", Size: 2048})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "lang-pip", Status: "ok", DryRun: true, FilesCandidate: 1, BytesCandidate: 2048})
	r.Render(model.Event{Event: model.EvSummary, DryRun: true, CleanersRun: 1, FilesCandidate: 1, BytesCandidate: 2048})

	got := buf.String()
	if !strings.Contains(got, "pip Wheel 缓存 (lang-pip)") {
		t.Fatalf("expected localized ZH cleaner name: %q", got)
	}
	if !strings.Contains(got, "/tmp/a.whl") {
		t.Fatalf("expected verbose file sample: %q", got)
	}
	if !strings.Contains(got, "总计: 扫描 1 项 - 可回收 2.0 kB（1 个文件）") {
		t.Fatalf("expected ZH summary: %q", got)
	}
}

func TestSafeBytesClampsNegativeValues(t *testing.T) {
	if got := safeBytes(-1); got != 0 {
		t.Fatalf("safeBytes(-1) = %d, want 0", got)
	}
}
