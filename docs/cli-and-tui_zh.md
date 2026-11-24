# CLI 与 TUI 完整参考

本文档说明 `beav` 的全部子命令、参数选项、4 标签页 TUI 快捷键、环境变量、JSONL 输出格式与进程退出码。

## 根命令与全局参数

在交互终端（TTY）中直接运行 `beav` 时，默认进入 `[1] Clean` 标签页启动 4 标签页 TUI 工作台。如果标准输出不是交互终端，则输出命令行帮助信息。

### 全局参数（所有子命令均可用）

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `--lang <auto\|en\|zh>` | `auto` | 覆盖界面语言（否则按 `BEAV_LANG`、`LC_ALL`、`LC_MESSAGES`、`LANG` 自动检测） |
| `-q`, `--quiet` | `false` | 静默输出，仅保留关键汇总或错误信息 |
| `--no-color` | `false` | 禁用终端 ANSI 彩色输出 |

### 根命令参数

| 参数 | 说明 |
| --- | --- |
| `-V`, `--version` | 输出 `beav <version> (<commit>, <date>)` 并退出 |
| `--system` | 在系统作用域启动 TUI（需 `sudo`） |
| `--all` | 同时在用户与系统作用域启动 TUI（需 `sudo`） |
| 策略与作用域参数 | 与 `tui`、`scan`、`clean` 共用同一组作用域和安全绕过参数 |

## 子命令一览

### 核心清理与分析

| 命令 | 别名 | 说明 |
| --- | --- | --- |
| `beav tui` | — | 显式启动多视图 TUI 工作台（`--tab clean\|analyze\|history\|config`） |
| `beav scan` | — | 严格只读扫描，预览匹配清理项与候选空间占用 |
| `beav clean` | — | 执行清理（TTY 中默认先预览并二次确认，加 `-y`/`--yes` 跳过确认） |
| `beav purge <path...>` | — | 无需编写 YAML，直接清理指定路径或通配符（`--dry-run`、`-y`/`--yes`、`-v`/`--verbose`、`--output`、`--force-no-age`） |
| `beav analyze [path]` | — | 启动内置 `gdu` 磁盘分析器（默认只读；传入 `--allow-delete` 或 `--unsafe` 允许在界面内删除） |

#### `scan` 与 `clean` 常用参数

| 参数 | 适用命令 | 说明 |
| --- | --- | --- |
| `--system` | `scan`, `clean`, `tui` | 启用系统级清理项 |
| `--all` | `scan`, `clean`, `tui` | 同时运行用户级与系统级清理项 |
| `--user <name>` | `scan`, `clean`, `tui` | 配合 `--all` 指定目标用户名 |
| `--only <selector>` | `scan`, `clean` | 仅运行匹配的清理项 ID、ID 前缀或标签（可重复传入） |
| `--skip <selector>` | `scan`, `clean` | 跳过匹配的清理项 ID、ID 前缀或标签（可重复传入） |
| `--include-heavy` | `scan`, `clean`, `tui` | 授权执行重度（`impact: heavy`）清理项 |
| `--min-age <rule>` | `scan`, `clean`, `purge`, `tui` | 覆盖留存天数（如 `30d` 或 `langs=14d,logs=30d`） |
| `--force-no-age` | `scan`, `clean`, `purge`, `tui` | 忽略文件修改时间过滤，直接匹配所有年龄文件 |
| `--output <mode>` | `scan`, `clean`, `purge` | 输出模式：`auto`、`spinner`、`plain` 或 `json` |
| `-v`, `--verbose` | `scan`, `clean`, `purge` | 显示候选/已删文件样本以及完整跳过详情 |
| `--dry-run` | `clean`, `purge` | 预览匹配文件但不实际删除 |
| `-y`, `--yes` | `clean`, `purge` | 在 TTY 中跳过确认提示 |
| `--config-dir <dir>` | `scan`, `clean`, `purge`, `tui`, `list`, `info` | 覆盖默认配置目录 |

关于 `--unsafe` / `--force`、`--allow-path`、`--bypass-*` 等策略绕过参数，请参阅 [安全策略与配置指南](safety-and-config_zh.md)。

### 查看与诊断

| 命令 | 别名 | 参数 | 说明 |
| --- | --- | --- | --- |
| `beav list` | `ls` | `--ids`, `--heavy`, `--scope`, `--config-dir` | 列出内置及自定义清理项、影响级别、生效留存期与路径 |
| `beav info <cleaner-id>` | — | `--config-dir` | 查看单个清理项的详细说明、路径、影响与安全策略备注 |
| `beav tags` | — | `--scope`, `--json`, `--config-dir` | 列出所有可用标签及对应清理项数量 |
| `beav doctor` | `status`, `check` | `--json`, `--config-dir` | 检查系统环境、权限、磁盘占用、配置合法性、安全开关及外部工具可用性 |
| `beav history` | `log`, `logs` | `-n`/`--limit`, `--cleaner`, `--all`, `--json`, `--clear`, `-y`/`--yes` | 查看或清空 `operations.log` 清理记录（`--all` 包含轮转历史文件） |

### 配置与工具命令

| 命令 | 别名 | 说明 |
| --- | --- | --- |
| `beav config init` | — | 生成带注释的初始 `~/.config/beav/config.yaml`（`--force` 覆盖已有文件） |
| `beav config show` | — | 查看当前生效配置（支持 `--json`） |
| `beav config edit` | — | 调用 `$VISUAL` 或 `$EDITOR`（默认 `vi`）编辑 `config.yaml` |
| `beav config path` | — | 打印配置目录、`config.yaml`、`cleaners.d`、`whitelist.txt` 与日志路径 |
| `beav config validate` | — | 校验 `config.yaml`、白名单以及 `cleaners.d` 中的自定义清理项 |
| `beav config reset` | — | 重置 `config.yaml` 为默认模板（`-y`/`--yes` 跳过确认） |
| `beav config enable <id>` | — | 将 `<id>` 写入配置默认启用 |
| `beav config disable <id>` | — | 将 `<id>` 写入配置默认禁用 |
| `beav config whitelist [add\|remove <path>]` | — | 查看、添加或删除白名单保护路径 |
| `beav config safety ...` | — | 查看或修改可配置安全开关以及自定义允许/禁止路径前缀 |
| `beav version` | `ver` | 显示版本、提交哈希、构建时间、Go 版本、系统发行版、配置目录与内置清理项数量（`-s`/`--short`、`--json`） |
| `beav help [command\|topic]` | — | 显示分组帮助或主题说明（`safety`、`exit-codes`、`env`、`examples`） |
| `beav completion [bash\|zsh\|fish]` | — | 输出 Shell 自动补全脚本 |

## TUI 多视图工作台

使用数字键 `1`-`4` 或 `[` / `]` 在四个标签页之间切换；任意页面按 `q` 或 `Ctrl+C` 退出，按 `?` 查看快捷键帮助浮层。

### `[1] Clean`（清理视图）

- `↑` / `↓`、`j` / `k`、`PgUp` / `PgDn`、`g` / `G`：上下移动清理项光标
- `Space`：勾选或取消当前清理项
- `a`：全选或取消全选常规（非重度）候选项
- `A`：全选所有候选项（包含重度清理项）
- `o`：切换排序方式（体积降序 -> 名称 -> 文件数降序 -> 影响级别）
- `t`：按标签筛选
- `m`：循环切换留存天数预设（默认、`0d`、`3d`、`7d`、`14d`、`30d`）并重新扫描
- `b`：实时切换 `bypass_all` 绕过模式并重新扫描
- `s`：显示或隐藏已跳过及零字节清理项
- `/`：按 ID、名称或标签搜索过滤
- `Tab` / `J` / `K`：聚焦并滚动右侧详情面板
- `r`：重新执行只读扫描
- `Enter` / `c`：打开确认弹窗并执行已勾选清理项

### `[2] Analyze`（磁盘分析视图）

- 内置目录体积浏览器，按 `o` 切换体积、文件数或名称排序
- `Enter`：进入选中目录；`Backspace` / `h`：返回上级目录
- `r`：刷新当前目录统计
- `d`：删除当前选中的文件或目录（带确认弹窗）
- `g`：挂起工作台并在当前路径启动完整的 `gdu` 浏览器

### `[3] History`（操作历史视图）

- `/`：按清理项 ID、作用域或路径过滤记录
- `r`：重新加载 `operations.log`
- `X`：清空操作历史（带确认弹窗）

### `[4] Safety & Config`（安全策略与配置视图）

- `↑` / `↓` 或 `j` / `k`：在安全布尔开关、自定义允许前缀列表、自定义禁止前缀列表间移动
- `Space`：切换选中的布尔开关状态
- `+`：向允许列表或禁止列表追加路径前缀
- `x` / `Delete`：删除选中的自定义路径条目
- `w`：将当前安全策略写入 `config.yaml`
- `R`：立即应用修改并触发清理列表重新扫描

## 环境变量

| 变量 | 作用 |
| --- | --- |
| `BEAV_LANG` | 覆盖界面语言：`auto`、`en` 或 `zh` |
| `BEAV_CONFIG_DIR` | 覆盖 `~/.config/beav` 默认配置目录 |
| `BEAV_UNSAFE` | 设为 `1` 或 `true` 时默认启用全绕过模式 |
| `BEAV_NO_OPLOG` | 设为任意非空值时禁用写入 `operations.log` |
| `NO_COLOR` | 设为任意非空值时禁用终端彩色输出 |
| `VISUAL`, `EDITOR` | `beav config edit` 启动的编辑器 |

## JSONL 输出与退出码

为 `scan`、`clean` 或 `purge` 指定 `--output json` 时，Beav 逐行输出 JSON 事件（`start`、`result`、`skip`、`error`、`summary`）。在只读扫描或 `--dry-run` 下，候选统计写入 `files_candidate` 和 `bytes_candidate`，实际删除计数保持为 `0`。当部分原生工具自身不提供精确字节统计时，`summary` 会设置 `metrics_incomplete: true` 并列出未计量清理项。

| 退出码 | 含义 |
| ---: | --- |
| `0` | 所选清理项全部完成，未发生错误事件 |
| `1` | 参数或选择器无效、缺少必需的 TTY，或未提权请求 `--system` |
| `2` | `config.yaml`、YAML 清理项定义或路径安全校验失败 |
| `3` | 至少有一个清理项在执行期间产生错误事件 |
| `4` | 进程收到 `SIGINT` 或 `SIGTERM` 后中断 |
