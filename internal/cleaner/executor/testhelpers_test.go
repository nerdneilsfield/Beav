package executor

import (
	"context"
	"sync"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func captureEvents(t *testing.T, fn func(emit func(model.Event))) []model.Event {
	t.Helper()
	var mu sync.Mutex
	var evs []model.Event
	fn(func(e model.Event) {
		mu.Lock()
		defer mu.Unlock()
		evs = append(evs, e)
	})
	return evs
}

func hasDelete(evs []model.Event, path string) bool {
	for _, e := range evs {
		if e.Event == model.EvDeleted && e.Path == path {
			return true
		}
	}
	return false
}

func hasCleanerSkip(evs []model.Event, reason string) bool {
	for _, e := range evs {
		if e.Event == model.EvCleanerSkipped && e.Reason == reason {
			return true
		}
	}
	return false
}

func hasSkip(evs []model.Event, reason string) bool {
	for _, e := range evs {
		if e.Event == model.EvSkipped && e.Reason == reason {
			return true
		}
	}
	return false
}

func ptrInt(v int) *int { return &v }

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}
