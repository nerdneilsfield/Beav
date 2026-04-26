package cli

import (
	"bytes"
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
