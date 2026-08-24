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

func TestSelectedHonorsScopeAndSelectors(t *testing.T) {
	cleaner := model.Cleaner{
		ID:    "browser-firefox",
		Scope: model.ScopeUser,
		Type:  model.TypePaths,
		Tags:  []string{"browser"},
	}

	if Selected(cleaner, model.ScopeSystem, nil, nil) {
		t.Fatal("system scope should not select a user cleaner")
	}
	if !Selected(cleaner, model.ScopeAll, []string{"browser"}, nil) {
		t.Fatal("tag selector should select cleaner in all scope")
	}
	if Selected(cleaner, model.ScopeAll, []string{"browser"}, []string{"browser-firefox"}) {
		t.Fatal("skip selector should override only selector")
	}
	if !Selected(cleaner, model.ScopeUser, []string{"browser"}, nil) {
		t.Fatal("matching scope and selector should select cleaner")
	}
}

func TestEngineHeavyAuthorizationTable(t *testing.T) {
	base := model.Cleaner{
		ID:        "sys-nix-gc",
		Scope:     model.ScopeUser,
		Type:      model.TypeToolPrune,
		Impact:    model.ImpactSafe,
		ToolPrune: &model.ToolPruneCfg{Profile: "sys-nix-gc"},
		Tags:      []string{"system"},
	}
	cases := []struct {
		name         string
		only         []string
		skip         []string
		includeHeavy bool
		cleaner      model.Cleaner
		wantRun      bool
		wantReason   string
	}{
		{name: "default denies heavy", wantReason: "heavy_not_authorized"},
		{name: "tag selector does not authorize", only: []string{"system"}, wantReason: "heavy_not_authorized"},
		{name: "prefix selector does not authorize", only: []string{"sys"}, wantReason: "heavy_not_authorized"},
		{name: "include heavy authorizes", includeHeavy: true, wantRun: true},
		{name: "exact id authorizes", only: []string{"sys-nix-gc"}, wantRun: true},
		{name: "skip overrides include", skip: []string{"sys-nix-gc"}, includeHeavy: true},
		{name: "disabled overrides include", includeHeavy: true, cleaner: model.Cleaner{ID: "sys-nix-gc", Scope: model.ScopeUser, Type: model.TypeToolPrune, Enabled: boolPtr(false), ToolPrune: &model.ToolPruneCfg{Profile: "sys-nix-gc"}}},
		{name: "wrong scope overrides include", includeHeavy: true, cleaner: model.Cleaner{ID: "sys-nix-gc", Scope: model.ScopeSystem, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "sys-nix-gc"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			if tc.cleaner.ID != "" {
				c = tc.cleaner
			}
			stub := &stubExecutor{}
			var events []model.Event
			e := New(WithExecutor(model.TypeToolPrune, stub))
			res, err := e.Run(context.Background(), []model.Cleaner{c}, Options{
				Scope:        model.ScopeUser,
				Only:         tc.only,
				Skip:         tc.skip,
				IncludeHeavy: tc.includeHeavy,
				Emitter:      func(ev model.Event) { events = append(events, ev) },
			})
			if err != nil {
				t.Fatal(err)
			}
			if (stub.ran > 0) != tc.wantRun {
				t.Fatalf("ran=%d, wantRun=%v result=%+v", stub.ran, tc.wantRun, res)
			}
			if tc.wantReason != "" {
				found := false
				for _, ev := range events {
					if ev.Event == model.EvCleanerSkipped && ev.Reason == tc.wantReason {
						found = true
					}
				}
				if !found {
					t.Fatalf("events=%+v, missing reason %q", events, tc.wantReason)
				}
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }

// TestEngineEmitsErrorForMissingExecutor verifies that the engine emits an error event
// when a cleaner has no registered executor.
// TestEngineEmitsErrorForMissingExecutor 验证当清理器没有注册执行器时引擎会发出错误事件。
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

func TestEngineSeparatesDryRunCandidatesAndUnmeasuredCleaners(t *testing.T) {
	var summary model.Event
	ex := executorFunc(func(_ context.Context, c model.Cleaner, _ bool, emit func(model.Event)) error {
		emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "ok", FilesCandidate: 3, BytesCandidate: 12, MetricsIncomplete: true})
		return nil
	})
	e := New(WithExecutor(model.TypePaths, ex))
	res, err := e.Run(context.Background(), []model.Cleaner{{ID: "paths", Scope: model.ScopeUser, Type: model.TypePaths}}, Options{
		Scope:  model.ScopeUser,
		DryRun: true,
		Emitter: func(ev model.Event) {
			if ev.Event == model.EvSummary {
				summary = ev
			}
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.FilesDeleted != 0 || res.BytesFreed != 0 {
		t.Fatalf("actual metrics = files %d bytes %d, want zero", res.FilesDeleted, res.BytesFreed)
	}
	if res.FilesCandidate != 3 || res.BytesCandidate != 12 {
		t.Fatalf("candidate metrics = files %d bytes %d, want 3/12", res.FilesCandidate, res.BytesCandidate)
	}
	if !res.MetricsIncomplete || len(res.UnmeasuredCleaners) != 1 || res.UnmeasuredCleaners[0] != "paths" {
		t.Fatalf("incomplete metrics = %+v", res)
	}
	if !summary.MetricsIncomplete || len(summary.UnmeasuredCleaners) != 1 || summary.UnmeasuredCleaners[0] != "paths" {
		t.Fatalf("summary = %+v", summary)
	}
}

type executorFunc func(context.Context, model.Cleaner, bool, func(model.Event)) error

func (f executorFunc) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	return f(ctx, c, dryRun, emit)
}

type stubExecutor struct{ ran int }

func (s *stubExecutor) Run(_ context.Context, c model.Cleaner, _ bool, emit func(model.Event)) error {
	s.ran++
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID})
	emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "ok"})
	return nil
}
