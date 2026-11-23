package cli

import (
	"bytes"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestSpinnerProducesFinishLineWithFileCountAndClearSeq(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinner(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "x", Name: "Very Long Cleaner Display Name", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "x", Status: "ok", FilesDeleted: 2, BytesFreed: 1024})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("Close must be idempotent: %v", err)
	}
	got := buf.String()
	if !strings.Contains(got, "\x1b[2K\r") {
		t.Fatalf("expected ANSI line-clear sequence to avoid CR artifacts: %q", got)
	}
	if !strings.Contains(got, "(x)") || !strings.Contains(got, "2 files") {
		t.Fatalf("result line must carry ID and file count: %q", got)
	}
}

// TestSpinnerDryRunShowsPlanLine verifies that a dry run reports candidates as a plan.
// TestSpinnerDryRunShowsPlanLine 验证 dry run 将候选报告为计划行。
func TestSpinnerDryRunShowsPlanLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinner(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "lang-pip", Name: "pip wheel cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "lang-pip", Status: "ok", DryRun: true, FilesCandidate: 1011, BytesCandidate: 263580931})
	r.Render(model.Event{Event: model.EvSummary, DryRun: true, CleanersRun: 1, FilesCandidate: 1011, BytesCandidate: 263580931})

	got := buf.String()
	if !strings.Contains(got, "pip wheel cache (lang-pip)") {
		t.Fatalf("missing plan line name/ID: %q", got)
	}
	if strings.Contains(got, "freed") {
		t.Fatalf("dry run must not claim freed bytes: %q", got)
	}
}

// TestSpinnerMetricsIncompleteNotNoop verifies unmeasured native cleaners are shown rather than folded into nothing to do.
// TestSpinnerMetricsIncompleteNotNoop 验证未计量的原生命令清理器不会被折叠进无需清理行。
func TestSpinnerMetricsIncompleteNotNoop(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinner(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "pkg-apt-archives", Name: "apt downloaded packages", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "pkg-apt-archives", Status: "ok", MetricsIncomplete: true})
	r.Render(model.Event{Event: model.EvSummary, CleanersRun: 1, MetricsIncomplete: true, UnmeasuredCleaners: []string{"pkg-apt-archives"}})

	got := buf.String()
	if strings.Contains(got, "nothing to do") {
		t.Fatalf("unmeasured cleaner must not fall into nothing to do: %q", got)
	}
	if !strings.Contains(got, "pkg-apt-archives") || !strings.Contains(got, "size unmeasured") {
		t.Fatalf("missing unmeasured line: %q", got)
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
	r.Render(model.Event{Event: model.EvSummary, CleanersSkipped: 1})

	got := buf.String()
	if strings.Contains(got, "[ok]") || strings.Contains(got, "0 B freed") {
		t.Fatalf("skipped cleaner should not print an ok/freed line: %q", got)
	}
	if !strings.Contains(got, "containers are running") || !strings.Contains(got, "(docker-builder)") {
		t.Fatalf("grouped skip line missing phrase or ID: %q", got)
	}
}

func TestSpinnerAnimatedTickerRaceSafe(t *testing.T) {
	buf := &lockedBuffer{}
	r := NewSpinnerWithOptions(buf, Options{
		Translator: i18n.New(i18n.LangZH),
		Animate:    true,
	})
	r.Render(model.Event{Event: model.EvStart, CleanerID: "editor-vscode", Name: "VS Code Cache", TS: time.Now()})
	time.Sleep(150 * time.Millisecond)
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "editor-vscode", Status: "ok", FilesDeleted: 1, BytesFreed: 512})
	r.Render(model.Event{Event: model.EvSummary, CleanersRun: 1, FilesDeleted: 1, BytesFreed: 512})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	got := buf.String()
	if !strings.Contains(got, "VS Code 缓存与日志") || !strings.Contains(got, "(editor-vscode)") {
		t.Fatalf("expected localized output, got %q", got)
	}
	if !strings.Contains(got, "已释放 512 B（1 个文件）") {
		t.Fatalf("expected natural ZH freed word order, got %q", got)
	}
}

func TestSpinnerChineseErrorTag(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinnerWithOptions(&buf, Options{Translator: i18n.New(i18n.LangZH)})
	r.Render(model.Event{Event: model.EvStart, CleanerID: "lang-pip", Name: "pip wheel cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvError, CleanerID: "lang-pip", Detail: "permission denied"})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "lang-pip", Status: "error"})
	r.Render(model.Event{Event: model.EvSummary, CleanersErrored: 1})

	got := buf.String()
	if !strings.Contains(got, "[错误]") || strings.Contains(got, "[err]") {
		t.Fatalf("expected localized error tag [错误], got %q", got)
	}
}
