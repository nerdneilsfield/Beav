package engine

import (
	"context"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// Executor defines the interface for running a cleaner.
// Executor 定义了运行清理器的接口。
type Executor interface {
	Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error
}

// Options holds the configuration for running the engine.
// Options 保存运行引擎的配置。
type Options struct {
	Scope   model.Scope
	DryRun  bool
	Only    []string
	Skip    []string
	Emitter func(model.Event)
}

// Result aggregates statistics from a cleaning run.
// Result 聚合清理运行的统计信息。
type Result struct {
	CleanersRun     int
	CleanersSkipped int
	CleanersErrored int
	BytesFreed      int64
	FilesDeleted    int64
	Errors          int
}

// Engine orchestrates the execution of cleaners with registered executors.
// Engine 协调已注册执行器来运行清理器。
type Engine struct {
	executors map[model.ExecutorType]Executor
}

// Option configures an Engine instance.
// Option 用于配置 Engine 实例。
type Option func(*Engine)

// WithExecutor registers an executor for a given executor type.
// WithExecutor 为指定的执行器类型注册一个执行器。
func WithExecutor(t model.ExecutorType, ex Executor) Option {
	return func(e *Engine) {
		e.executors[t] = ex
	}
}

// New creates a new Engine with the provided options.
// New 使用提供的选项创建一个新的 Engine。
func New(opts ...Option) *Engine {
	e := &Engine{executors: map[model.ExecutorType]Executor{}}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run executes the given cleaners according to the options and returns aggregated results.
// Run 根据选项执行给定的清理器并返回聚合结果。
func (e *Engine) Run(ctx context.Context, cleaners []model.Cleaner, opt Options) (Result, error) {
	res := Result{}
	for _, c := range cleaners {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		if !Selected(c, opt.Scope, opt.Only, opt.Skip) {
			continue
		}
		ex, ok := e.executors[c.Type]
		if !ok {
			if opt.Emitter != nil {
				opt.Emitter(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: opt.DryRun, TS: time.Now()})
				opt.Emitter(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: "missing executor for type " + string(c.Type), TS: time.Now()})
				opt.Emitter(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "error", Errors: 1, TS: time.Now()})
			}
			res.CleanersErrored++
			res.Errors++
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

// Selected reports whether c should run for the requested scope and selectors.
// Selected 报告清理器 c 是否应在请求的范围和选择器下运行。
func Selected(c model.Cleaner, scope model.Scope, only, skip []string) bool {
	if !c.IsEnabled() {
		return false
	}
	if scope != "" && scope != model.ScopeAll && c.Scope != scope {
		return false
	}
	return match(c, only, skip)
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
