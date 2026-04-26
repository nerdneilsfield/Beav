package engine

import (
	"context"
	"errors"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestEngineRunsSelectedAndAggregates(t *testing.T) {
	cleaners := []model.Cleaner{
		{ID: "a", Name: "A", Scope: model.ScopeUser, Type: model.TypePaths},
		{ID: "b", Name: "B", Scope: model.ScopeUser, Type: model.TypePaths, Tags: []string{"langs"}},
	}
	stub := &stubExecutor{}
	e := New(WithExecutor(model.TypePaths, stub))
	res, err := e.Run(context.Background(), cleaners, Options{Only: []string{"langs"}})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanersRun != 1 {
		t.Errorf("ran %d", res.CleanersRun)
	}
	if stub.ran != 1 {
		t.Errorf("executor ran %d times", stub.ran)
	}
}

func TestEngineReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := New(WithExecutor(model.TypePaths, &stubExecutor{}))
	_, err := e.Run(ctx, []model.Cleaner{{ID: "a", Scope: model.ScopeUser, Type: model.TypePaths}}, Options{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

type stubExecutor struct{ ran int }

func (s *stubExecutor) Run(_ context.Context, c model.Cleaner, _ bool, emit func(model.Event)) error {
	s.ran++
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID})
	emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "ok"})
	return nil
}
