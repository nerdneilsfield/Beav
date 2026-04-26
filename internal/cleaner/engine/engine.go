package engine

import (
	"context"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

type Executor interface {
	Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error
}

type Options struct {
	Scope   model.Scope
	DryRun  bool
	Only    []string
	Skip    []string
	Emitter func(model.Event)
}

type Result struct {
	CleanersRun     int
	CleanersSkipped int
	CleanersErrored int
	BytesFreed      int64
	FilesDeleted    int64
	Errors          int
}

type Engine struct {
	executors map[model.ExecutorType]Executor
}

type Option func(*Engine)

func WithExecutor(t model.ExecutorType, ex Executor) Option {
	return func(e *Engine) {
		e.executors[t] = ex
	}
}

func New(opts ...Option) *Engine {
	e := &Engine{executors: map[model.ExecutorType]Executor{}}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func (e *Engine) Run(ctx context.Context, cleaners []model.Cleaner, opt Options) (Result, error) {
	res := Result{}
	for _, c := range cleaners {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if !c.IsEnabled() || (opt.Scope != "" && c.Scope != opt.Scope) || !match(c, opt.Only, opt.Skip) {
			continue
		}
		ex, ok := e.executors[c.Type]
		if !ok {
			continue
		}

		var status string
		var bytesFreed int64
		var filesDeleted int64
		var errs int
		emit := func(ev model.Event) {
			ev.DryRun = opt.DryRun
			if opt.Emitter != nil {
				opt.Emitter(ev)
			}
			if ev.Event != model.EvFinish {
				return
			}
			status = ev.Status
			bytesFreed = ev.BytesFreed
			filesDeleted = ev.FilesDeleted
			errs = ev.Errors
		}

		if err := ex.Run(ctx, c, opt.DryRun, emit); err != nil {
			return res, err
		}
		if err := ctx.Err(); err != nil {
			return res, err
		}
		switch status {
		case "ok":
			res.CleanersRun++
		case "skipped":
			res.CleanersSkipped++
		case "error":
			res.CleanersErrored++
			res.Errors += errs
		default:
			res.CleanersRun++
		}
		res.BytesFreed += bytesFreed
		res.FilesDeleted += filesDeleted
	}
	if opt.Emitter != nil {
		opt.Emitter(model.Event{
			Event:           model.EvSummary,
			DryRun:          opt.DryRun,
			CleanersRun:     res.CleanersRun,
			CleanersSkipped: res.CleanersSkipped,
			CleanersErrored: res.CleanersErrored,
			BytesFreed:      res.BytesFreed,
			FilesDeleted:    res.FilesDeleted,
			Errors:          res.Errors,
			TS:              time.Now(),
		})
	}
	return res, nil
}

func match(c model.Cleaner, only, skip []string) bool {
	if len(only) > 0 && !matchesAny(c, only) {
		return false
	}
	return !matchesAny(c, skip)
}

func matchesAny(c model.Cleaner, selectors []string) bool {
	for _, selector := range selectors {
		if selector == c.ID {
			return true
		}
		for _, tag := range c.Tags {
			if selector == tag {
				return true
			}
		}
		if strings.HasPrefix(c.ID, selector+"-") {
			return true
		}
	}
	return false
}
