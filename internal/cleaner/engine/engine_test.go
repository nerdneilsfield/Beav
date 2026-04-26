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

func TestEngineScopeAllRunsUserAndSystem(t *testing.T) {
	cleaners := []model.Cleaner{
		{ID: "u", Scope: model.ScopeUser, Type: model.TypePaths},
		{ID: "s", Scope: model.ScopeSystem, Type: model.TypePaths},
	}
	stub := &stubExecutor{}
	e := New(WithExecutor(model.TypePaths, stub))
	res, err := e.Run(context.Background(), cleaners, Options{Scope: model.ScopeAll})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanersRun != 2 || stub.ran != 2 {
		t.Fatalf("result=%+v ran=%d, want both scopes", res, stub.ran)
	}
}

func TestEngineEmitsErrorForMissingExecutor(t *testing.T) {
	var events []model.Event
	e := New()
	res, err := e.Run(context.Background(), []model.Cleaner{{ID: "x", Scope: model.ScopeUser, Type: model.TypePaths}}, Options{
		Scope: model.ScopeUser,
		Emitter: func(e model.Event) {
			events = append(events, e)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.CleanersErrored != 1 {
		t.Fatalf("result=%+v, want one errored cleaner", res)
	}
	for _, e := range events {
		if e.Event == model.EvError && e.Reason == "internal" {
			return
		}
	}
	t.Fatalf("missing executor error event not emitted: %+v", events)
}

type stubExecutor struct{ ran int }

func (s *stubExecutor) Run(_ context.Context, c model.Cleaner, _ bool, emit func(model.Event)) error {
	s.ran++
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID})
	emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "ok"})
	return nil
}
