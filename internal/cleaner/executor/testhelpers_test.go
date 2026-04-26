package executor

import (
	"context"
	"sync"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// captureEvents collects all events emitted by a function under test.
// captureEvents 收集被测函数发出的所有事件。
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

// hasDelete checks if any event indicates a deletion of the given path.
// hasDelete 检查是否有任何事件指示删除了给定路径。
func hasDelete(evs []model.Event, path string) bool {
	for _, e := range evs {
		if e.Event == model.EvDeleted && e.Path == path {
			return true
		}
	}
	return false
}

// hasCleanerSkip checks if any event indicates the cleaner was skipped for the given reason.
// hasCleanerSkip 检查是否有任何事件指示清理器因给定原因被跳过。
func hasCleanerSkip(evs []model.Event, reason string) bool {
	for _, e := range evs {
		if e.Event == model.EvCleanerSkipped && e.Reason == reason {
			return true
		}
	}
	return false
}

// hasSkip checks if any event indicates a file was skipped for the given reason.
// hasSkip 检查是否有任何事件指示文件因给定原因被跳过。
func hasSkip(evs []model.Event, reason string) bool {
	for _, e := range evs {
		if e.Event == model.EvSkipped && e.Reason == reason {
			return true
		}
	}
	return false
}

// ptrInt returns a pointer to the given int value, useful for test fixtures.
// ptrInt 返回指向给定 int 值的指针，用于测试夹具。
func ptrInt(v int) *int { return &v }

// testContext returns a context that is cancelled when the test completes.
// testContext 返回一个在测试完成时被取消的上下文。
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	return ctx
}
