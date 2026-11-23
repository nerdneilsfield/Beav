<div align="center">
  <h1>🦫 Beav</h1>
  <p><strong>安全清理 Linux 缓存，不靠猜哪些文件能删。</strong></p>
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

Beav 是一款 Linux 磁盘清理工具，用于清理缓存、过期下载、日志、构建产物以及其他可重新生成的文件。遇到 Homebrew、uv、Hugging Face、系统包管理器或容器运行时，它会调用对应工具自身的清理接口；这些目录不能简单地直接 `rm`。

个人文件不归 Beav 管。浏览器配置与登录数据、凭据、项目目录、容器数据卷，以及 `Downloads` 中的普通文件都不会被当成缓存处理。

## 通过 Homebrew / Scoop 安装

```bash
# macOS / Linux
brew install --cask nerdneilsfield/apps/beav
```

## 快速上手

可以从 [Releases](https://github.com/nerdneilsfield/Beav/releases) 下载 Linux 压缩包，解压后把 `beav` 放进 `PATH`。如果选择源码构建，需要 Go 1.24 或更新版本：

```bash
git clone https://github.com/nerdneilsfield/Beav.git
cd Beav
make build
./bin/beav version
```

在交互终端里直接运行 `beav`，会自动进入全屏只读扫描与勾选工作台：

```bash
beav               # 先执行只读扫描，查看体积与候选文件样本，自主勾选并二次确认后才清理
sudo beav --system # 在系统作用域启动交互工作台
```

如果习惯纯命令行，可以先运行只读扫描预览：

```bash
beav scan          # 严格只读预览，绝不删除任何文件
beav clean         # 终端中：先预览，确认 y 后才删除
beav clean --yes   # 跳过确认直接清理常规清理项
```

管道和 CI 没有交互终端，`beav clean` 在那里保持直接执行，脚本行为不变。

Beav 不会自行调用 `sudo`；系统级清理需要明确使用：

```bash
sudo beav clean --system
```

想在一次运行中同时处理原用户和系统作用域，可以用：

```bash
sudo beav clean --all
```

想以只读界面查看各目录磁盘占用（基于 `gdu`，按 `q` 退出）：

```bash
beav analyze [PATH]
```

界面语言会自动根据环境变量（`BEAV_LANG`、`LC_ALL`、`LC_MESSAGES`、`LANG`）在中英文间切换，也可以通过 `--lang zh` 或 `--lang en` 手动指定。

## 能清理哪些内容

内置清理项覆盖了常见的 Linux 桌面和开发环境：

| 范围 | 例子 |
|---|---|
| 编辑器 | VS Code / Insiders / VSCodium、Cursor、Zed、JetBrains IDE、Neovim 缓存与日志 |
| 浏览器 | Firefox、Chrome、Chromium、Edge、Brave、Vivaldi 的缓存目录；个人配置与登录状态保留 |
| 语言工具 | npm、pnpm、yarn（Classic / Berry）、bun、deno、node-gyp、Electron、pip、uv、Poetry、Conda、PDM/Hatch/pipx、Corepack、Cargo、Go（`GOCACHE`、gopls、`GOMODCACHE`）、Gradle、Maven、Coursier |
| 开发工具 | pre-commit、ccache、sccache、clangd、zig、Bazel、Cypress、Playwright、mise |
| AI 与 GPU | 未完成的 Hugging Face 下载、旧 HF Datasets Arrow 缓存、Torch Hub 权重、vLLM 与 FlashInfer、Mesa 与 NVIDIA 着色器缓存、NVIDIA 计算缓存、Triton、TorchInductor |
| 桌面 | 回收站、缩略图缓存、字体缓存、Flatpak 应用缓存与无用运行时、过期的未完成下载 |
| Kubernetes | minikube 镜像缓存、Helm、k9s、kubectl 接口缓存 |
| 云工具 | 过期的 gcloud 与 Azure CLI 日志 |
| 容器 | Docker、Podman 清理；Rootless 与系统守护进程分开校验 |
| 系统 | apt、dnf、pacman、zypper、systemd 日志（轮转与清理）、`/tmp`、`/var/tmp`、`/var/cache`（man、fontconfig、PackageKit）、Nix GC |

查看本机二进制内置与自定义的全部清理项、影响级别（常规 / 重度）、实际生效保留期以及单条详情：

```bash
beav list
beav list --heavy
beav info lang-rustup
```

工具没安装、相关进程仍在运行、文件太新，或者 Beav 无法确认目标安全时，对应清理项会自动跳过。跳过不等于删除失败。

## 删除边界

缓存目录未必固定。环境变量可能改过它，符号链接可能在运行中被替换，`sudo` 也可能指向错误的用户目录。Beav 会先核对这些条件，再决定能不能删。

- 路径必须落在允许的根目录下，例如目标用户的家目录、`/var/cache`、`/var/log`、`/tmp` 或 `/var/tmp`。
- 已知凭据位置和个人数据目录在硬黑名单中，包括 `~/.ssh`、`~/.gnupg`、`~/.password-store`、`~/.kube/config` 和 `~/.docker/config.json`。内置浏览器清理项只指向缓存子目录，不指向用户配置。
- 普通清理项不能进入 `Documents`、`Desktop`、`Downloads`、`Pictures`、`Videos`、`Music`。Downloads 是唯一的窄例外：只检查一层，并且只识别未完成下载的后缀。
- 年龄按单个文件计算。父目录很旧，不代表里面的新文件可以一起删。
- 目录遍历由 `openat` 和 `O_NOFOLLOW` 锚定。遇到跨挂载点、符号链接替换或文件身份变化，删除会立即停止。
- 相关程序、打开的文件描述符和锁文件都会影响判断。目标用户进程的检查一旦报错，Beav 会保守跳过。
- 重度清理项需要 `--include-heavy`、在 `--only` 中写出完整清理项 ID 或标签，或在交互界面中手动按空格键勾选。`--yes`、前缀和 `--all` 都不会自动启用重度清理项。

Beav 没有隐式提权。普通用户请求 `--system` 时只会收到 sudo 提示；root 登录若想清理 `/root`，还要明确传入 `--allow-root-home`。在 `sudo --all` 下，Beav 会交叉检查 `SUDO_UID`、`SUDO_USER` 和家目录所有者，然后才确定用户作用域。

<details>
<summary><strong>文件删除前具体检查什么</strong></summary>

对于路径型清理项，Beav 从可信目录描述符开始逐级打开路径。每个文件都会经过符号链接检查、文件系统设备检查、`.git` 目录保护和保留期判断；调用 `unlinkat` 前，还会再次比对 `(inode, device, size, mtime)`。

外部工具清理方案以闭集形式编译进 Beav。YAML 只能选择已知 profile，不能注入命令、参数、环境变量或任意根目录。Hugging Face 与 Torch 的托管缓存采用先重命名再删除的方式，并写入恢复日志；即使中途退出，下一次运行也无需相信过期路径。

Docker、Podman 的 Rootless 清理还会核对守护进程模式、套接字所有者和套接字位置。系统清理项不会连接 Rootless 守护进程。

</details>

## 常用命令

```bash
# 只读预览语言工具相关清理项及候选文件样本
beav scan --only langs --verbose

# 查看单个清理项的说明、路径与示例命令
beav info lang-uv

# 运行一个指定清理项
beav clean --only lang-uv

# 本次跳过浏览器缓存
beav clean --skip browser

# 全局改为 30 天
beav clean --min-age=30d

# 分别覆盖标签保留期
beav clean --min-age=langs=14d,logs=30d

# 允许已启用的重度清理项
beav clean --include-heavy

# 给脚本输出 JSONL
beav scan --output json
```

`--only` 和 `--skip` 支持清理项 ID、ID 前缀或标签。重度清理项更严格：只有完整 ID 或完整标签才能在 `--only` 中启用。敏感 profile 还有内置最低保留天数，命令行参数不能把未完成下载或托管缓存的门槛降到安全线以下。

完整参数以程序帮助为准：

```bash
beav clean --help
```

## 配置

查看当前生效配置、生成带注释的初始配置文件、用 `$VISUAL`/`$EDITOR` 编辑，或管理白名单保护路径：

```bash
beav config show
beav config init
beav config edit
beav config whitelist add ~/.cache/keep-this
beav config whitelist remove ~/.cache/keep-this
```

某个清理项不想再清理，一行命令即可永久禁用或启用，不需要手改 YAML：

```bash
beav config disable lang-pip
beav config enable lang-pip
```

默认配置文件是 `~/.config/beav/config.yaml`。优先级很直接：命令行参数 > 本配置文件 > 清理项内置默认值。

```yaml
defaults:
  min_age_days: 14
  output: spinner # auto | spinner | plain | json

overrides:
  editor-vscode:
    min_age_days: 3
  browser-chromium:
    enabled: false

whitelist:
  - ~/.cache/keep-this
```

额外的自定义清理项 YAML 放在 `~/.config/beav/cleaners.d/`。一个简单的路径型清理项如下：

```yaml
- id: lang-zola
  name: Zola build cache
  scope: user
  type: paths
  min_age_days: 14
  paths:
    - ~/.cache/zola/**
  tags: [langs, static-site]
```

自定义 YAML 仍要通过注册表和路径安全校验。`type: command` 会被拒绝；`tool_prune`、`debris`、`managed_cache` 这类闭集执行器只接受内置 profile 名称。

## 输出与脚本

stdout 是终端时，默认输出动态 spinner；接到管道后会改用 plain。让其他程序读取时，建议显式选择 JSONL：

```bash
beav scan --output json | jq -c 'select(.event == "finish" or .event == "summary")'
```

只读扫描的候选数量写入 `files_candidate` 和 `bytes_candidate`，实际删除字段仍为零。部分原生工具无法给出准确字节数。遇到这种情况，summary 会设置 `metrics_incomplete` 并列出未计量的清理项，而不是编一个数字。

脚本可以依赖以下退出码：

| 退出码 | 含义 |
|---:|---|
| `0` | 所选清理项结束，没有错误事件 |
| `1` | 参数无效、未知 ID/选择器或提权请求被拒绝 |
| `2` | 配置、注册表或路径校验失败 |
| `3` | 至少一个清理项产生错误事件 |
| `4` | 收到信号后中断 |

## 明确不清理的内容

Beav 不删除旧内核、容器数据卷、Downloads 中的普通文件、浏览器历史、最近文件记录或远程缓存。对于缓存语义还不能安全确定的应用数据，目前也选择不碰，比如 Ollama 和 ModelScope。

Nix 只调用原生 GC，不传删除 generation 的参数。Conda 限于索引缓存、tarball 和日志。Rust 工具链清理保留默认工具链、每个发布分支（stable、beta、nightly 及每个 `major.minor`）的最新版本，以及项目链接的工具链。项目没有通用 shell 命令清理器。

这些限制是有意留下的。一个目录只要可能存着用户工作的唯一副本，就应该单独设计清理流程，而不是再补一条通配符规则。

## 参与开发

新增路径型清理项时，先从 YAML 开始，并给出保守的保留天数。涉及原生工具或托管缓存时，安全规则应写进 Go 的闭集 profile，同时配测试，不应变成用户传入的参数。

提交前运行：

```bash
make test
make race
make lint
```

[设计记录](docs/superpowers/specs/2026-04-26-beav-design.md) 解释了最初的安全模型；[实现计划](docs/superpowers/plans/2026-04-26-beav-implementation.md) 保留了首个版本的开发过程。

## 致谢与许可证

Beav 的灵感来自 [Mole](https://github.com/tw93/mole)。磁盘浏览由 [gdu](https://github.com/dundee/gdu) 提供，终端界面使用 [Charm](https://charm.sh/) 系列库。

项目采用 [MIT License](LICENSE)。
