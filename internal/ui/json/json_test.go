package json

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestJSONLOnePerLine(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "x", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "x", Status: "ok"})
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines: %q", len(lines), buf.String())
	}
	for _, line := range lines {
		var e model.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Errorf("invalid JSON %q: %v", line, err)
		}
	}
}

func TestJSONLIncludesZeroValuedStableCounters(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf)
	r.Render(model.Event{Event: model.EvSummary, TS: time.Now()})

	line := strings.TrimSpace(buf.String())
	for _, field := range []string{"files_deleted", "bytes_freed", "errors", "cleaners_run", "cleaners_skipped", "cleaners_errored"} {
		if !strings.Contains(line, `"`+field+`":`) {
			t.Fatalf("summary JSON missing %s: %s", field, line)
		}
	}
}

func TestJSONLOmitsIrrelevantZeroCountersOnSkippedEntry(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf)
	r.Render(model.Event{Event: model.EvSkipped, CleanerID: "x", Path: "/tmp/x", Reason: "age_too_recent", TS: time.Now()})
	line := strings.TrimSpace(buf.String())
	for _, field := range []string{"files_deleted", "bytes_freed", "errors", "cleaners_run", "duration_ms", "dry_run"} {
		if strings.Contains(line, `"`+field+`":`) {
			t.Fatalf("skipped entry should omit %s: %s", field, line)
		}
	}
}

func TestJSONLStartIncludesDryRunFalse(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "x", DryRun: false, TS: time.Now()})
	line := strings.TrimSpace(buf.String())
	if !strings.Contains(line, `"dry_run":false`) {
		t.Fatalf("start should include dry_run false: %s", line)
	}
}
