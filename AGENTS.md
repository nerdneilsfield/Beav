# Beav — Agent Instructions

Linux 磁盘清理工具，Go 编写。删除安全内核在 `internal/cleaner/{engine,executor,safety}`；交互与呈现层在 `internal/cli` 与 `internal/ui`。涉及删除路径或多 pass 逻辑的改动，先读下方 Gotchas。

## Counterweight

Coding implementation and modification tasks follow these rules:

1. Do not add an abstraction, dependency, config knob, fallback, retry, cache,
   or compatibility layer without a current requirement, a reachable caller, or
   an observed failure.
2. Do not write a plan, design document, or task list for work whose correct
   diff is already apparent.
3. Do not add a permanent test unless the behavior deserves future protection
   and its expected result comes from outside the implementation.
4. Do not put throwaway scripts, reproducers, or fixtures in the project's test
   tree. Use /tmp or an uncommitted scratch path.
5. Do not drop a security, accessibility, data-loss, or trust-boundary guard in
   pursuit of a smaller diff.

These rules yield to an explicit user request for more process. Language and
framework skills may be combined with them freely. Do not enable a general
planning or orchestration workflow by default.

## Gotchas

- 改动 `runClean` 的多 pass 逻辑后，必须先在 /tmp 隔离夹具（自定义 cleaner YAML + 临时目录）上验证 dry 与 real 两条路径，才允许任何接触真实 home 的运行。原因：一次调用方硬编码 `dry=false` 绕过了 `--dry-run`，验证时真实删除了 14.86 GB 缓存。dry 只能在 `runCleanPass` 内由 `f.DryRun` 派生，禁止调用方传字面量；`TestCleanDryRunNeverDeletes` 守护这一行为。

## Development

- 构建 `make build`（产出 `bin/beav`）；检查 `go test ./...` 与 `make race`；`make lint` 需要本机安装 golangci-lint。
- 文档注释是双语惯例：先英文一句，再对应中文（见 `internal/cli`、`internal/cleaner/model`）。新代码沿用。
- 新增 paths 型 cleaner 从 `cleaners/user/` 下的 YAML 开始并给出保守年龄；原生工具、托管缓存、Rust 工具链的安全规则写进 Go 闭集 profile（`internal/cleaner/model/cleaner.go`），不由 YAML 参数注入。
- 终端输出契约：结果行携带 cleaner ID、dry-run 显示 plan、零结果与 skip 汇总分组。改渲染器时同步维护 `internal/ui/cli` 的 plain 与 spinner 及其测试。
