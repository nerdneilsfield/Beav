<div align="center">
  <h1>🦫 Beav</h1>
  <p><strong>清理Linux缓存，不靠猜哪些文件能删。</strong></p>
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

Beav是一款Linux磁盘清理工具，处理缓存、过期下载、日志、构建产物和其他可重新生成的文件。遇到Homebrew、uv、Hugging Face、系统包管理器或容器运行时，它会调用对应工具的清理接口；这些地方不能简单地对着目录执行`rm`。

个人文件不归Beav管。浏览器资料、凭据、项目目录、容器卷，以及`Downloads`里的普通文件都不会被当成缓存。

## 第一次运行，先 dry-run

可以从[Releases](https://github.com/nerdneilsfield/Beav/releases)下载Linux压缩包，解压后把`beav`放进`PATH`。如果选择源码构建，需要Go 1.24或更新版本：

```bash
git clone https://github.com/nerdneilsfield/Beav.git
cd Beav
make build
./bin/beav version
```

新机器上不要急着删。先看计划：

```bash
beav clean --dry-run
```

确认输出符合预期，再去掉 `--dry-run`：

```bash
beav clean
```

这条命令只处理当前用户的缓存。Beav 不会自行调用 `sudo`；系统清理需要明确执行：

```bash
sudo beav clean --system
```

想在一次运行中同时处理原用户和系统作用域，可以用：

```bash
sudo beav clean --all
```

直接运行 `beav` 会打开终端菜单。`beav analyze [PATH]` 则进入随程序提供的磁盘占用浏览器。

## 能清理哪些内容

内置清理项覆盖了常见的Linux桌面和开发环境：

| 范围 | 例子 |
|---|---|
| 编辑器 | VS Code、VS Code Server、JetBrains IDE、Neovim 缓存与状态目录 |
| 浏览器 | Firefox、Chrome、Chromium、Edge 的缓存目录；用户资料和登录数据保留 |
| 语言工具 | npm、pnpm、yarn、bun、pip、uv、Poetry、Conda、Corepack、Go、Gradle、Maven |
| 开发工具 | pre-commit、ccache、Cypress、Playwright、mise |
| AI 与 GPU | 未完成的 Hugging Face 下载、旧 HF Datasets Arrow 单元、Torch Hub checkpoints、NVIDIA compute cache、Triton、TorchInductor |
| 桌面 | 回收站、缩略图缓存、过期的未完成下载、Flatpak 无用数据 |
| Kubernetes | minikube 镜像缓存、Helm、k9s、kubectl 缓存 |
| 云工具 | 过期的gcloud与Azure CLI日志 |
| 容器 | Docker、Podman清理；无根模式与系统守护进程分开校验 |
| 系统 | apt、dnf、pacman、zypper、systemd journal、`/tmp`、`/var/tmp`、选定的 `/var/cache` 内容、Nix GC |

你安装的版本究竟带了哪些清理项，直接问本机二进制最准确：

```bash
beav config show
```

工具没安装、相关进程仍在运行、文件太新，或者Beav无法证明目标安全时，对应清理项会跳过。跳过不等于删除失败。

## 删除边界

缓存目录未必固定。环境变量可能改过它，符号链接可能在运行中被替换，`sudo`也可能指向错误的家目录。Beav会先核对这些条件，再决定能不能删。

- 路径必须落在允许的根目录下，例如目标用户的 home、`/var/cache`、`/var/log`、`/tmp` 或 `/var/tmp`。
- 已知凭据位置和个人数据目录在硬黑名单中，包括`~/.ssh`、`~/.gnupg`、`~/.password-store`、`~/.kube/config`和`~/.docker/config.json`。内置浏览器清理项只指向缓存子目录，不指向用户资料。
- 普通清理项不能进入 `Documents`、`Desktop`、`Downloads`、`Pictures`、`Videos`、`Music`。Downloads 是唯一的窄例外：只检查一层，并且只识别未完成下载的后缀。
- 年龄按单个文件计算。父目录很旧，不代表里面的新文件可以一起删。
- 目录遍历由 `openat` 和 `O_NOFOLLOW` 锚定。遇到跨挂载点、符号链接替换或文件身份变化，删除会停止。
- 相关程序、打开的文件描述符和锁文件都会影响判断。目标用户进程的检查一旦报错，Beav 会保守跳过。
- heavy 清理项需要 `--include-heavy`，或者在 `--only` 中写出完整 cleaner ID 或 tag。`--yes`、前缀和 `--all` 都不能代替这次授权。

Beav没有隐式提权。普通用户请求`--system`时只会收到sudo提示；root登录若想清理`/root`，还要明确传入`--allow-root-home`。在`sudo --all`下，Beav会交叉检查`SUDO_UID`、`SUDO_USER`和家目录所有者，然后才确定用户作用域。

<details>
<summary><strong>文件删除前具体检查什么</strong></summary>

对于路径型清理项，Beav 从可信目录描述符开始逐级打开路径。每个文件都会经过符号链接检查、文件系统设备检查、`.git` 目录保护和年龄判断；调用 `unlinkat` 前，还会再次比较 `(inode, device, size, mtime)`。

外部工具清理方案以闭集形式编译进Beav。YAML只能选择已知profile，不能注入命令、参数、环境变量或任意根目录。Hugging Face与Torch的托管缓存采用先重命名再删除的方式，并写入恢复日志；即使中途退出，下一次运行也无需相信过期路径。

Docker、Podman的无根清理还会核对守护进程模式、套接字所有者和套接字位置。系统清理项不会连接无根守护进程。

</details>

## 常用命令

```bash
# 只预览语言工具相关清理项
beav clean --dry-run --only langs

# 运行一个指定清理项
beav clean --only lang-uv

# 本次跳过浏览器缓存
beav clean --skip browser

# 全局改为30天
beav clean --min-age=30d

# 分别覆盖 tag
beav clean --min-age=langs=14d,logs=30d

# 允许已启用的heavy清理项
beav clean --include-heavy

# 给脚本输出 JSONL
beav clean --dry-run --output json
```

`--only`和`--skip`支持cleaner ID、ID前缀或tag。heavy清理项更严格：只有完整ID才能在`--only`中顺带授权。敏感profile还有内置最低年龄，命令行参数不能把未完成下载或托管缓存的门槛降到安全线以下。

完整参数以程序帮助为准：

```bash
beav clean --help
```

## 配置

下面三条命令分别用于生成初始配置、查看合并结果和打开编辑器：

```bash
beav config init
beav config show
beav config edit
```

默认配置文件是 `~/.config/beav/config.yaml`。优先级很直接：命令行覆盖配置文件，配置文件覆盖内置默认值。

```yaml
defaults:
  min_age_days: 14
  output: spinner # spinner | plain | json

overrides:
  editor-vscode:
    min_age_days: 3
  browser-chromium:
    enabled: false

whitelist:
  - ~/.cache/keep-this
```

额外的 cleaner YAML 放在 `~/.config/beav/cleaners.d/`。一个简单的路径型清理项如下：

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

自定义YAML仍要通过清理项注册表和路径安全校验。`type: command`会被拒绝；`tool_prune`、`debris`、`managed_cache`这类闭集执行器只接受内置profile名称。

## 输出与脚本

stdout 是终端时，默认输出 spinner；接到管道后会改用 plain。让其他程序读取时，建议显式选择 JSONL：

```bash
beav clean --dry-run --output json | jq -c 'select(.event == "finish" or .event == "summary")'
```

dry-run 的候选数量写入 `files_candidate` 和 `bytes_candidate`，实际删除字段仍为零。部分原生工具无法给出准确字节数。遇到这种情况，summary 会设置 `metrics_incomplete` 并列出未计量的 cleaner，而不是编一个数字。

脚本可以依赖以下退出码：

| 退出码 | 含义 |
|---:|---|
| `0` | 所选清理项结束，没有错误事件 |
| `1` | 参数无效或提权请求被拒绝 |
| `2` | 配置、registry 或路径校验失败 |
| `3` | 至少一个清理项产生错误事件 |
| `4` | 收到信号后中断 |

## 明确不清理的内容

Beav不删除旧内核、容器卷、Downloads中的普通文件、浏览器历史、最近文件记录或远程缓存。对于缓存语义还不能安全确定的应用数据，目前也选择不碰，比如Ollama和ModelScope。

Nix只调用原生GC，不传删除generation的参数。Conda限于索引缓存、tarball和日志。项目没有通用shell command cleaner。

这些限制是有意留下的。一个目录只要可能存着用户工作的唯一副本，就应该单独设计清理流程，而不是再补一条 glob。

## 参与开发

新增路径型cleaner时，先从YAML开始，并给出保守的年龄。涉及原生工具或托管缓存时，安全规则应写进Go的闭集profile，同时配测试，不应变成用户传入的参数。

提交前运行：

```bash
make test
make race
make lint
```

[设计记录](docs/superpowers/specs/2026-04-26-beav-design.md) 解释了最初的安全模型；[实现计划](docs/superpowers/plans/2026-04-26-beav-implementation.md) 保留了首个版本的开发过程。

## 致谢与许可证

Beav的灵感来自[Mole](https://github.com/tw93/mole)。磁盘浏览由[gdu](https://github.com/dundee/gdu)提供，终端界面使用[Charm](https://charm.sh/)系列库。

项目采用 [MIT License](LICENSE)。
