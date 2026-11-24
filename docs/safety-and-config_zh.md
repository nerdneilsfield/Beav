# 安全策略、绕过控制与配置指南

默认情况下，Beav 会强制执行保守边界，防止常规缓存清理误伤个人目录或正在使用的缓存。从 v0.3.0 起，除根目录 `/` 本身始终受保护外，所有安全限制均可在 `~/.config/beav/config.yaml`、`[4] Safety & Config` TUI 标签页、`beav config safety` 子命令或单次命令行参数中自由配置与解除。

## 默认安全边界

在默认策略（`DefaultPolicy`）下：

- **允许列表（Allow-list）**：清理目标必须位于目标用户主目录，或 `/var/cache`、`/var/log`、`/tmp`、`/var/tmp` 下。
- **黑名单（Blacklist）**：禁止触碰系统目录（`/etc`、`/boot`、`/usr`、`/proc`、`/sys`、`/dev`、`/run`、`/var/lib/docker`、`/var/lib/containerd`、`/var/lib/kubelet`）及用户目录（`Documents`、`Desktop`、`Downloads`、`Pictures`、`Videos`、`Music`、`.ssh`、`.gnupg`、`.password-store`）。其中 `Downloads` 仅对过期的未完成下载后缀开放单层扫描。
- **目录遍历与文件身份校验**：通过 `openat` + `O_NOFOLLOW` 锚定遍历，禁止跨越挂载点，跳过 `.git` 目录，按文件级 `mtime` 判断年龄（并对 `managed_cache`、`debris`、`rustup` 设有最低天数下限），关联进程运行或存在打开文件描述符时保守跳过。
- **Root 主目录保护**：以 `root` 身份运行时默认拒绝清理 `/root`，除非显式传入 `--allow-root-home` 或配置 `safety.allow_root_home: true`。
- **重度清理项保护**：标记为 `impact: heavy` 的清理项不会自动执行，需要传入 `--include-heavy`、`--only <完整ID或标签>`、`--unsafe`，或在 TUI 中手动勾选。

> [!WARNING]
> 绕过允许列表、黑名单、年龄下限或符号链接校验会取消对应的数据防误删保护。在使用 `--unsafe` 或其他绕过参数实际删除前，建议先运行 `beav scan` 或 `--dry-run` 核对目标范围。

## 命令行绕过参数

以下参数可用于 `clean`、`scan`、`purge`、`tui` 或根命令 `beav` 的单次执行：

| 参数 | 作用 |
| --- | --- |
| `--unsafe`, `--force` | 开启全绕过模式（`bypass_all`）：跳过允许列表、黑名单（`/` 除外）、年龄下限、进程检查、重度限制、root 主目录限制，并允许运行 `type: command` 清理项 |
| `--allow-path <path>` | 为本次运行额外追加允许清理的根路径（可重复指定） |
| `--bypass-allow-list` | 允许清理用户主目录与默认系统目录之外的路径 |
| `--bypass-blacklist` | 允许清理命中默认或自定义黑名单的路径（仍保护根目录 `/`） |
| `--bypass-age-floor` | 允许对带有内置安全下限的清理项使用 `0d` 等更激进的留存天数 |
| `--ignore-running` | 跳过运行中进程、锁文件与打开文件描述符检查 |
| `--follow-symlinks` | 允许跟踪符号链接以及符号链接形式的外部命令 |
| `--allow-cross-fs` | 允许跨越文件系统与挂载点边界 |
| `--allow-root-home` | 以 `root` 身份运行时允许清理 `/root` |

设置环境变量 `BEAV_UNSAFE=1`（或 `true`）可在不传参数时默认启用全绕过模式。

## 配置文件（`config.yaml`）

默认路径为 `~/.config/beav/config.yaml`（可通过 `BEAV_CONFIG_DIR` 或 `--config-dir` 覆盖）。

生效优先级从高到低依次为：**命令行参数 > `config.yaml` > 清理项 YAML / 内置默认值**。

```yaml
defaults:
  min_age_days: 14
  output: auto # auto | spinner | plain | json

overrides:
  editor-vscode:
    min_age_days: 3
  browser-chromium:
    enabled: false

whitelist:
  - ~/.cache/keep-this

safety:
  bypass_all: false
  allow_list:
    - /opt/build-cache
  replace_allow_list: false
  disable_allow_list: false
  blacklist:
    - ~/important-work
  replace_blacklist: false
  disable_blacklist: false
  ignore_running_processes: false
  bypass_age_floor: false
  follow_symlinks: false
  allow_cross_fs: false
  allow_root_home: false
  allow_command_cleaners: false
```

### `safety` 配置项说明

| 字段 | 类型 | 默认值 | 说明 |
| --- | --- | --- | --- |
| `bypass_all` | `bool` | `false` | 一键总开关，绕过下方全部可配置限制（根目录 `/` 除外） |
| `allow_list` | `[]string` | `[]` | 追加自定义允许清理的根路径前缀 |
| `replace_allow_list` | `bool` | `false` | 设为 `true` 时，仅允许 `allow_list` 中的路径，不再包含默认主目录与系统目录 |
| `disable_allow_list` | `bool` | `false` | 完全关闭允许列表限制 |
| `blacklist` | `[]string` | `[]` | 在内置黑名单之外追加禁止路径前缀 |
| `replace_blacklist` | `bool` | `false` | 设为 `true` 时，用 `blacklist` 替换内置黑名单 |
| `disable_blacklist` | `bool` | `false` | 关闭黑名单检查（仍保护根目录 `/`） |
| `ignore_running_processes` | `bool` | `false` | 忽略关联进程、锁文件与文件占用检查 |
| `bypass_age_floor` | `bool` | `false` | 禁用内置清理策略的最小留存天数下限 |
| `follow_symlinks` | `bool` | `false` | 允许遍历符号链接并接受符号链接形式的外部命令 |
| `allow_cross_fs` | `bool` | `false` | 允许跨越挂载点与文件系统边界 |
| `allow_root_home` | `bool` | `false` | 允许在 `root` 身份下清理 `/root` |
| `allow_command_cleaners` | `bool` | `false` | 允许加载并执行 `cleaners.d/` 中的 `type: command` 自定义命令清理项 |

### 通过 CLI 管理安全策略

```bash
beav config safety show
beav config safety set allow_command_cleaners true
beav config safety allow-add /opt/build-cache
beav config safety deny-add ~/important-work
beav config safety allow-remove /opt/build-cache
beav config safety deny-remove ~/important-work
```

## 自定义清理项（`cleaners.d/`）

可将自定义 YAML 文件放入 `~/.config/beav/cleaners.d/` 目录。

### 路径型清理项（`type: paths`）

```yaml
- id: custom-zola
  name: Zola build cache
  scope: user
  type: paths
  min_age_days: 14
  paths:
    - ~/.cache/zola/**
  tags: [langs, static-site]
```

### 自定义命令清理项（`type: command`）

启用自定义命令清理项需要先在配置中开启 `safety.allow_command_cleaners: true`（或在运行时传入 `--unsafe`）。Beav 通过 `exec.CommandContext` 直接执行可执行文件，不经过 Shell 解释；如需管道或复合命令，可显式调用 `sh -c`。未填写 `dry_run_command` 时，`scan` 与 `--dry-run` 会跳过执行 `command`。

```yaml
- id: custom-ccache-zero
  name: Clear ccache stats and cache
  scope: user
  type: command
  command: ccache
  args: ["-C"]
  dry_run_command: ["ccache", "-s"]
  tags: [devtools]
```
