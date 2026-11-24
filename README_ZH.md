<div align="center">
  <h1>🦫 Beav</h1>
  <p><strong>安全、边界可配置的 Linux 磁盘缓存清理工具。</strong></p>
  <p>
    <a href="https://github.com/nerdneilsfield/Beav/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/nerdneilsfield/Beav/ci.yml?branch=master&style=flat-square&label=CI&logo=github" alt="CI"></a>
    <a href="https://github.com/nerdneilsfield/Beav/releases"><img src="https://img.shields.io/github/v/release/nerdneilsfield/Beav?style=flat-square&include_prereleases&label=release" alt="Release"></a>
    <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.24+">
    <img src="https://img.shields.io/badge/platform-Linux-FCC624?style=flat-square&logo=linux&logoColor=black" alt="Linux">
    <a href="LICENSE"><img src="https://img.shields.io/github/license/nerdneilsfield/Beav?style=flat-square" alt="License"></a>
    <a href="README.md"><img src="https://img.shields.io/badge/lang-English-blue?style=flat-square" alt="English"></a>
  </p>
</div>

---

Beav 用于扫描并清理 Linux 桌面、开发环境、AI/GPU、容器及系统级缓存。遇到自带缓存语义的工具（例如 `uv`、`npm`、`cargo`、`rustup`、Hugging Face、`nix`、Docker、Podman）时，Beav 会调用工具自身的清理接口或原子化缓存清理器，而不是直接粗暴删除目录。

默认策略会保护个人数据目录、凭据路径与正在运行的相关进程。同时，从 v0.3.0 起，除根目录 `/` 本身始终受保护外，允许列表、黑名单、年龄下限、进程锁、符号链接/跨挂载点校验以及自定义 `type: command` 清理项均可通过命令行参数、`config.yaml` 或交互界面自由配置和绕过。

## 核心特性

- **工具原生感知**：调用包管理器、语言工具链与容器引擎的原生清理接口，对 Hugging Face 和 Torch 缓存采用先重命名后删除的原子流程。
- **4 标签页 TUI 工作台**：集成了 `[1] Clean` 清理勾选、`[2] Analyze` 目录空间浏览与 `gdu` 联动、`[3] History` 操作历史审计、以及 `[4] Safety & Config` 安全开关实时编辑。
- **完备 CLI 命令集**：提供 `scan`、`clean`、`purge`、`analyze`、`doctor`、`history`、`list`、`info`、`tags`、`config`、`version`、`help` 与 Shell 补全。
- **可配置安全边界**：默认保守防误删，需要突破限制时可通过 `--allow-path`、`--bypass-*`、`--unsafe` 或 `config.yaml` 中的 `safety:` 节按需放开。
- **适配自动化与脚本**：支持 `spinner`、`plain`、`quiet`（`-q`）和 JSONL（`--output json`）输出及约定退出码（`0`-`4`）。

## 安装

通过 Homebrew 安装，或从 [Releases](https://github.com/nerdneilsfield/Beav/releases) 下载对应架构的静态二进制压缩包：

```bash
brew install --cask nerdneilsfield/apps/beav
```

安装后检查版本与环境状态：

```bash
beav version
beav doctor
```

## 快速上手

在交互终端（TTY）中直接运行 `beav`，会先执行只读扫描并进入 4 标签页 TUI 工作台：

```bash
beav               # 启动用户级 4 标签页 TUI 工作台
sudo beav --system # 启动系统级缓存交互界面
```

常用纯命令行流程：

```bash
beav scan                     # 严格只读扫描，预览可清理体积
beav clean                    # TTY 中先预览，确认后再删除
beav clean --only langs -y    # 跳过确认直接清理语言工具缓存
beav purge /tmp/build-* -y    # 无需写 YAML，直接清理指定路径
```

界面语言会按 `BEAV_LANG`、`LC_ALL`、`LC_MESSAGES`、`LANG` 自动在中英文间切换，也可通过 `--lang zh` 或 `--lang en` 显式指定。

## 4 标签页 TUI 工作台

使用 `1`-`4` 或 `[` / `]` 切换页面，按 `?` 查看全部快捷键，按 `q` 退出：

| 标签页 | 用途 | 常用快捷键 |
| --- | --- | --- |
| `[1] Clean` | 交互式清理清单，带磁盘占用条、体积占比条与可滚动详情栏 | `Space` 勾选、`a` 全选常规、`A` 全选含重度、`o` 排序、`t` 标签筛选、`m` 天数预设、`b` 实时切换绕过、`Enter`/`c` 执行 |
| `[2] Analyze` | 内置目录空间浏览器，支持直接定位与删除 | `Enter`/`Backspace` 进退目录、`o` 排序、`d` 删除、`g` 启动完整 `gdu` |
| `[3] History` | 检索并审计 `operations.log` 中的历史删除记录 | `/` 过滤、`r` 刷新、`X` 清空日志 |
| `[4] Safety & Config` | 交互修改安全策略布尔开关及允许/禁止路径前缀 | `Space` 切换开关、`+` 添加路径、`x` 移除路径、`w` 写入 `config.yaml`、`R` 应用并重新扫描 |

也可以直接指定进入某个标签页：`beav tui --tab analyze`（支持 `clean`、`history`、`config`）。

## 常用 CLI 命令一览

| 命令 | 别名 | 说明 |
| --- | --- | --- |
| `beav tui` | — | 启动多视图交互工作台（`--tab <name>`） |
| `beav scan` | — | 严格只读预览（`--only`、`--skip`、`--min-age`、`--output json`） |
| `beav clean` | — | 执行清理（`--dry-run`、`-y`、`--include-heavy`、`--system`、`--all`） |
| `beav purge <path...>` | — | 直接清理指定路径或通配符（`--force-no-age`、`--dry-run`、`-y`） |
| `beav analyze [path]` | — | 启动 `gdu` 磁盘分析界面（传入 `--allow-delete` 或 `--unsafe` 允许删文件） |
| `beav doctor` | `status`, `check` | 检查系统信息、权限、磁盘占用、配置合法性与外部工具状态（`--json`） |
| `beav history` | `log`, `logs` | 查看或清空删除操作记录（`-n`、`--cleaner`、`--all`、`--clear`、`--json`） |
| `beav list` | `ls` | 列出内置及自定义清理项（`--ids`、`--heavy`、`--scope`） |
| `beav info <id>` | — | 查看单个清理项的路径、执行方式、影响与备注 |
| `beav tags` | — | 列出所有标签及对应清理项数量（`--json`） |
| `beav config ...` | — | `init`、`show`、`edit`、`path`、`validate`、`reset`、`enable`、`disable`、`whitelist`、`safety` |
| `beav version` | `ver` | 显示版本、Commit、构建时间、Go 版本、平台、配置目录与清理项数量（`-s`、`--json`） |
| `beav help [topic]` | — | 分组命令帮助或主题说明（`safety`、`exit-codes`、`env`、`examples`） |

所有命令的参数、环境变量、JSONL 事件与退出码详表见 [CLI 与 TUI 完整参考](docs/cli-and-tui_zh.md)。

## 清理覆盖范围

| 范围 | 典型清理项 |
| --- | --- |
| 编辑器 | VS Code / Insiders / VSCodium、Cursor、Zed、JetBrains IDE、Neovim 缓存与日志 |
| 浏览器 | Firefox、Chrome、Chromium、Edge、Brave、Vivaldi 的缓存子目录（保留个人配置与登录态） |
| 语言工具链 | `npm`、`pnpm`、`yarn`、`bun`、`deno`、`node-gyp`、`pip`、`uv`、`Poetry`、`Conda`、`Cargo`、`rustup`、`Go`、`Gradle`、`Maven`、`Coursier` |
| 开发工具 | `pre-commit`、`ccache`、`sccache`、`clangd`、`zig`、`Bazel`、`Cypress`、`Playwright`、`mise` |
| AI 与 GPU | 未完成的 Hugging Face 下载、旧 HF Datasets Arrow 缓存、Torch Hub 权重、`vLLM`、`FlashInfer`、Mesa/NVIDIA 着色器缓存、`Triton`、`TorchInductor` |
| 桌面与云工具 | 回收站、缩略图、字体缓存、Flatpak 缓存与无用运行时、过期未完成下载、`minikube`、`helm`、`k9s`、`kubectl`、`gcloud`、Azure CLI 日志 |
| 容器与系统 | Docker、Podman（Rootless 与系统守护进程分开校验）、`apt`、`dnf`、`pacman`、`zypper`、`systemd` 日志、`/tmp`、`/var/tmp`、`/var/cache`、Nix GC |

## 可配置安全边界与限制绕过

默认情况下，Beav 会屏蔽个人目录（`Documents`、`Desktop`、`Downloads`、`Pictures`、`Videos`、`Music`、`.ssh`、`.gnupg`、`.password-store`）、系统核心目录（`/etc`、`/boot`、`/usr`、`/proc`、`/sys`、`/dev`、`/run` 与容器存储卷）、跨挂载点遍历、符号链接跟随、运行中进程占用的缓存、以 `root` 运行时的 `/root` 目录，以及敏感缓存的最小留存天数下限。

除根目录 `/` 之外，任何限制都可以按次或在配置中放开：

> [!WARNING]
> 使用 `--unsafe`、`--bypass-allow-list`、`--bypass-blacklist`、`--bypass-age-floor` 或 `--follow-symlinks` 会取消对应的防误删边界。执行真实删除前，先用 `beav scan` 或 `--dry-run` 核对目标范围。

```bash
# 本次临时追加允许清理的目录前缀
beav purge /opt/build-cache --allow-path /opt/build-cache --dry-run

# 本次绕过所有可配置限制
beav clean --unsafe --min-age=0d

# 通过命令行查看和持久化安全策略
beav config safety show
beav config safety set allow_command_cleaners true
beav config safety allow-add /opt/build-cache
```

配置文件位于 `~/.config/beav/config.yaml`，自定义规则放在 `~/.config/beav/cleaners.d/`（支持 `type: paths` 及需显式开启的 `type: command`）。完整配置项与自定义格式请阅读 [安全策略、绕过控制与配置指南](docs/safety-and-config_zh.md)。

## 文档导航

- [CLI 与 TUI 完整参考](docs/cli-and-tui_zh.md) — 全部子命令、参数、TUI 快捷键、环境变量、JSONL 与退出码
- [安全策略、绕过控制与配置指南](docs/safety-and-config_zh.md) — 默认边界、绕过参数、`config.yaml` `safety:` 完整字段与自定义清理项写法

## 参与开发

源码构建需要 Go 1.24 或更高版本：

```bash
git clone https://github.com/nerdneilsfield/Beav.git
cd Beav
make build
make test
make race
make lint
```

早期架构设计记录保留在 [设计记录](docs/superpowers/specs/2026-04-26-beav-design.md) 与 [实现计划](docs/superpowers/plans/2026-04-26-beav-implementation.md)。

## 致谢与许可证

Beav 的灵感来自 [Mole](https://github.com/tw93/mole)。磁盘占用分析基于 [gdu](https://github.com/dundee/gdu)，交互界面使用 [Charm](https://charm.sh/) 系列库。

项目基于 [MIT License](LICENSE) 开源。
