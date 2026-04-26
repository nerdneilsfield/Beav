<div align="center">
  <h1>🦫 Beav</h1>
  <p><em>勤恳的河狸为你打理 Linux 磁盘——疏通缓存、清理日志、扫净构建残留。</em></p>
  <p>
    <a href="https://github.com/nerdneilsfield/Beav/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/nerdneilsfield/Beav/ci.yml?branch=master&style=flat-square&label=CI&logo=github" alt="CI"></a>
    <a href="https://github.com/nerdneilsfield/Beav/releases"><img src="https://img.shields.io/github/v/release/nerdneilsfield/Beav?style=flat-square&include_prereleases&label=release" alt="Release"></a>
    <a href="https://goreportcard.com/report/github.com/nerdneilsfield/Beav"><img src="https://img.shields.io/badge/go%20report-A-brightgreen?style=flat-square" alt="Go Report"></a>
    <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.24+">
    <img src="https://img.shields.io/badge/platform-Linux-FCC624?style=flat-square&logo=linux&logoColor=black" alt="Linux">
    <a href="LICENSE"><img src="https://img.shields.io/github/license/nerdneilsfield/Beav?style=flat-square" alt="License"></a>
    <a href="README.md"><img src="https://img.shields.io/badge/lang-English-blue?style=flat-square" alt="English"></a>
  </p>
</div>

---

> 🇬🇧 **English docs:** [README.md](README.md)

**Beav** 是一款单二进制、纯 Go 的 Linux 缓存清理工具。灵感来自 macOS 上的 [Mole](https://github.com/tw93/mole)，但 Beav 为 Linux 桌面与开发机纷杂的生态重新设计：浏览器、IDE、语言工具链、容器运行时、包管理器堆积的缓存，Beav 一并扫清——**到此为止**。**你的数据不是缓存。Beav 知道边界在哪里。**

## ✨ 为什么用 Beav？

- 🦫 **默认按"年龄"删除。** 文件只有在指定天数内未被触碰才会被清理。明天打开 IDE，缓存自然重建；这周还在跑的工作完全不动。
- 🛡️ **安全是契约，不是直觉。** 每次删除都要穿过一整套 `openat` 锚定关卡：白名单边界、硬黑名单、禁止跟随符号链接越界、禁止跨文件系统、`.git` 工作树保护、删前重 `stat` 防 TOCTOU。
- 🧰 **一切皆数据。** 清理项是 YAML，内置项随二进制嵌入；想加一条只需把 YAML 丢进 `~/.config/beav/cleaners.d/`。
- 🐙 **开箱即覆盖宽生态。** VS Code、JetBrains 全家桶、Neovim、Firefox、Chrome 系、npm/pnpm/yarn/bun、pip、cargo、Go build cache、Gradle、Maven、apt/dnf/pacman/zypper、journald、Docker、Podman、helm、k9s、kubectl、minikube。
- 🚦 **两种作用域，一个二进制。** `beav clean` 清家目录；`sudo beav clean --system` 清系统；`sudo beav clean --all` 一并清理，带多重校验的 `SUDO_USER` 解析。
- 🎨 **三种界面。** 终端里带 spinner，管道里走 plain，脚本/监控走 JSONL 事件流。
- 🚫 **绝不越界。** Beav 永不偷偷调用 `sudo`，不跟随 symlink 跑出安全区，不动 `~/.ssh`、`~/.gnupg`、`~/.kube/config`、`~/.docker/config.json`、`/var/lib/{docker,containerd,kubelet}`、`/etc`、`/boot`、`/usr`。

## 🚀 快速上手

```bash
# 从源码构建（需要 Go 1.24+）
git clone https://github.com/nerdneilsfield/Beav
cd Beav
make build
./bin/beav version
```

```bash
# 第一天的工作流
beav clean --dry-run                # 先看清要删什么——建议每次都从这里开始
beav clean                          # 清理 $HOME 下的缓存
sudo beav clean --system            # 清理 /var/cache、journal、包管理器缓存
sudo beav clean --all               # 一次清两个作用域
beav analyze                        # 交互式磁盘浏览器（背后是 gdu）
```

## 🎬 清理效果一览

```
$ beav clean
  ✓ VS Code Cache · 已释放 312.7 MB
  ✓ JetBrains IDE caches · 已释放 1.4 GB
  ✓ npm cache · 已释放 487.3 MB
  ✓ Go build cache · 已释放 2.1 GB
  ✓ Firefox cache · 已释放 145.0 MB
  ○ Chromium-family cache · 跳过 (running_process)

共释放 4.4 GB，运行了 5 个清理项。
```

<details>
<summary><strong>📋 完整命令参考</strong></summary>

```bash
beav                                 # 启动交互式 bubbletea 菜单（需 TTY）

beav clean                           # 用户作用域清理
beav clean --system                  # 系统作用域（非 root 拒绝）
beav clean --all                     # 用户 + 系统（root + 合法 SUDO_USER）
beav clean --dry-run                 # 仅预览，不删除
beav clean --only vscode,langs       # 按 ID 前缀或 tag 筛选
beav clean --skip browsers           # 反向筛选
beav clean --min-age=14d             # 全局年龄阈值覆盖
beav clean --min-age=langs=3d,logs=30d   # 按 tag 单独覆盖
beav clean --force-no-age            # 启用主动声明无年龄过滤的 cleaner
beav clean --output json             # stdout 输出 JSONL 事件
beav clean --yes                     # 跳过首次运行的 dry-run 提示
beav clean --user=alice --all        # 显式指定 SUDO_USER

beav analyze [PATH]                  # 交互式磁盘浏览 TUI（gdu）
beav config show                     # 打印生效配置 + cleaner 列表
beav config edit                     # 用 $EDITOR 打开 ~/.config/beav/config.yaml
beav completion {bash|zsh|fish}      # 生成 shell 补全脚本
beav version
```

**退出码语义**

| 码值 | 含义 |
|------|------|
| `0`  | 所选 cleaner 全部完成且无 error 事件 |
| `1`  | 用法错误（参数错、提权被拒） |
| `2`  | 配置 / registry / 路径安全校验失败 |
| `3`  | 至少一个 cleaner 触发了 error 事件 |
| `4`  | 信号中断（Ctrl-C） |

</details>

## 🎯 Beav 都清些什么？

| 类别 | 清理项 |
|------|--------|
| **桌面** | 回收站、缩略图缓存 |
| **编辑器** | VS Code（含 server / cpptools）、JetBrains 全家、Neovim 状态目录 |
| **浏览器** | Firefox、Chrome、Chromium、Edge ——只清 `Cache` / `Code Cache` / `GPUCache` 子目录（**绝不动 Profiles、cookies、登录态**） |
| **语言工具链** | npm、pnpm、yarn、bun、pip、cargo、Go build cache、Gradle、Maven |
| **Shell** | `~/.cache/zsh`、zcompdump |
| **Kubernetes** | minikube 镜像缓存、helm、k9s、kubectl 缓存（**绝不动 `~/.kube/config`**） |
| **容器（rootless）** | Docker / Podman 的 builder + image prune |
| **包管理器** | apt、dnf、pacman、zypper |
| **系统日志** | systemd journal（用 `journalctl --vacuum-time`，绝不直接 `rm`） |
| **系统 tmp** | `/tmp`、`/var/tmp`，以及 `/var/cache` 中的零散子目录 |
| **容器（rootful）** | Docker builder / image / container / network 各自 prune；Podman system prune |

<details>
<summary><strong>🔬 默认年龄阈值</strong></summary>

| 类别 | 默认 `min_age_days` | 理由 |
|------|---|---|
| 浏览器 / IDE 缓存 | 7 | 一周没碰，说明不在活跃流里 |
| 包管理器下载（apt/npm/pip/yarn/bun） | 14 | 中等保守；重下载成本低 |
| 构建产物（cargo、go、gradle、maven） | 30 | 重建成本不容忽视 |
| systemd journal | 14 | 走 `journalctl --vacuum-time=14d` |
| `/tmp`、`/var/tmp` | 10 | 与 systemd-tmpfiles 默认对齐 |
| `~/.local/share/Trash` | 30 | 留出后悔时间 |
| 容器 builder / image（rootful） | 14 / 30 | builder cache 激进、image 谨慎 |

可用 `--min-age=Nd` 全局覆盖、`--min-age=langs=3d,logs=30d` 按 tag 覆盖、或在 `~/.config/beav/config.yaml` 里按 cleaner ID 单独配置。

</details>

## 🛡️ 安全架构

Beav 把删除当成手术——每一刀都要复核两遍。

<details open>
<summary><strong>每个文件穿越的八道关卡</strong></summary>

1. **加载期白名单** — 每条静态路径与 resolver fallback 都必须落在 `$HOME`、`/var/cache`、`/var/log`、`/tmp`、`/var/tmp` 之内。指向其他位置的 YAML cleaner **加载时就被拒**，而非运行时。
2. **硬黑名单** — `/`、`/etc`、`/boot`、`/usr`、`/proc`、`/sys`、`/dev`、`/run`、`~/.ssh`、`~/.gnupg`、`~/.password-store`、`~/.docker/config.json`、`~/.kube/config`、`/var/lib/{docker,containerd,kubelet}`，以及 `~/{Documents,Desktop,Downloads,Pictures,Videos,Music}`，YAML 怎么写都不会动。
3. **锚定下行** — 每条路径都从安全根（safe root）出发，逐分量 `openat` + `O_NOFOLLOW`。任何中间分量是 symlink 即拒绝。
4. **跨文件系统守卫** — 在 root 抓取 `st_dev`，每个 entry 重新比对；挂载点直接跳过。
5. **`.git` 守卫** — 任何含 `.git` 子项的目录视为 git 工作树，整棵子树跳过。
6. **逐文件年龄过滤** — `now - mtime ≥ min_age_days` 是按**单个文件**判定，不是按父目录。空目录在子项删完后再自底向上删。
7. **TOCTOU 重新 stat** — 调用 `unlinkat` 之前再 stat 一次，`(ino, dev, size, mtim)` 任一字段漂移就放弃删除。
8. **进程守卫** — Beav 扫描 `/proc`，看到 `code`、`chromium`、`firefox`、`idea` 等正在运行就跳过对应 cleaner。

</details>

<details>
<summary><strong>权限模型：为什么 Beav 不偷偷 sudo</strong></summary>

| 调用方式 | 实际 UID | 清理范围 |
|---|---|---|
| `beav clean` | 非 root | 用户作用域（调用者的 `$HOME`） |
| `beav clean --system` | 非 root | 拒绝并打印 `sudo` 提示 |
| `sudo beav clean --system` | root | 仅系统 |
| `sudo beav clean --all` | root | 系统 + 原始用户的 `$HOME` |
| 直接以 root 登录跑 `beav clean` | root | 拒绝，除非显式 `--allow-root-home` |

`--all` 通过**三道独立校验**解析目标家目录——`$SUDO_UID`、`$SUDO_USER`、目录的 `lstat` owner——任何一处不一致就拒绝执行。`--user=name` 提供显式覆盖，但同样要过这三关。

</details>

<details>
<summary><strong>容器 rootless 校验</strong></summary>

用户作用域容器 cleaner 必须**全部满足**才会跑：

1. 运行时报告 rootless 模式（`docker info -f '{{.SecurityOptions}}'` / `podman info -f '{{.Host.Security.Rootless}}'`）。
2. 活跃 socket（通过 `docker context inspect` / `podman info` 查得）的 owner 是调用者 UID。
3. socket 路径在 `/run/user/$UID/`、`$XDG_RUNTIME_DIR/`、或 `$HOME/` 之下。

如果设置了 `DOCKER_HOST`，Beav 仅在它指向通过验证的 rootless socket 时才采纳；否则 cleaner 跳过并标记 `runtime_not_rootless`。系统作用域 cleaner 则反向：检测到 rootless daemon 时拒绝执行。

</details>

## 📜 配置

Beav 读取 `~/.config/beav/config.yaml`（如果存在）。**优先级：CLI flags > 配置文件 > 内置默认值。**

```yaml
# ~/.config/beav/config.yaml
defaults:
  min_age_days: 14
  output: spinner            # spinner | plain | json

overrides:
  editor-vscode:
    min_age_days: 3
    enabled: true
  browser-chromium:
    enabled: false

whitelist:
  - ~/.cache/keep-this
```

自定义 cleaner 放在 `~/.config/beav/cleaners.d/` 下，YAML 文件即可：

<details>
<summary><strong>例子：清理冷门工具的缓存</strong></summary>

```yaml
# ~/.config/beav/cleaners.d/zola.yaml
- id: lang-zola
  name: Zola 构建缓存
  scope: user
  type: paths
  min_age_days: 14
  paths:
    - ~/.cache/zola/**
  tags: [langs, static-site]
```

`beav config show | jq '.cleaners[] | select(.id=="lang-zola")'` 可验证已生效。

</details>

## 🧪 输出格式

<details>
<summary><strong>用 JSONL 串到流水线里</strong></summary>

```bash
beav clean --output json | jq -c 'select(.event=="finish")'
```

```json
{"event":"start","cleaner_id":"editor-vscode","name":"VS Code Cache","scope":"user","type":"paths","ts":"2026-04-27T10:00:00Z"}
{"event":"deleted","cleaner_id":"editor-vscode","path":"/home/dq/.config/Code/Cache/x","size":4096,"ts":"..."}
{"event":"skipped","cleaner_id":"editor-vscode","path":"/home/dq/.config/Code/Cache/y","reason":"age_too_recent","ts":"..."}
{"event":"finish","cleaner_id":"editor-vscode","status":"ok","files_deleted":42,"bytes_freed":12345678,"duration_ms":83,"ts":"..."}
{"event":"summary","cleaners_run":12,"bytes_freed":987654321,"duration_ms":4200,"ts":"..."}
```

每种事件类型只输出该类型相关的字段——无填充、schema 稳定、消费方友好。

</details>

## 🚧 v1 边界：明确不支持

Beav v1 故意保持窄。下面这些已记入 v1.x 路线：

- **`beav prune-kernels`** — 清旧内核会动 `/boot` 和包数据库，需要专门的强确认流程。
- **`beav prune-volumes`** — Docker / Podman volume 是持久化数据，不是缓存；而且 `docker volume prune` 不支持 `--filter until=`，需要基于 inspect 的专用 executor。
- **`beav prune-recents`** — 重写 `~/.local/share/recently-used.xbel` 需要 `transform` executor 类型，含备份 + 原子替换。
- **Flatpak / Snap / AppImage** — 沙箱语义需要单独打磨。
- **`beav status` / `beav optimize`** — 用 `btop`、`htop`、`systemctl` 即可，不重复造轮子。

## 🤝 贡献

加新清理项很容易——把 YAML 丢进 `cleaners/user/`（或 `cleaners/system/`）开 PR 即可。schema 见 [docs/superpowers/specs/2026-04-26-beav-design.md §5.1](docs/superpowers/specs/2026-04-26-beav-design.md)。提交前请确保 `go test ./...` 与 `golangci-lint run ./...` 通过。新增 executor 类型（`paths / command / journal_vacuum / pkg_cache / container_prune` 之外）需先经过设计讨论。

## 📖 文档

- [设计规范](docs/superpowers/specs/2026-04-26-beav-design.md) ——所有安全决策的来龙去脉。
- [实现计划](docs/superpowers/plans/2026-04-26-beav-implementation.md) ——9 个阶段 28 个 task，TDD 全程。

## 📜 许可证

MIT，详见 [LICENSE](LICENSE)。

## 🙏 致谢

- [Mole](https://github.com/tw93/mole) ——macOS 上的灵感来源。
- [gdu](https://github.com/dundee/gdu) ——`beav analyze` 的引擎。
- [Charm](https://charm.sh/) ——bubbletea、lipgloss、bubbles。

---

<div align="center">
<sub>献给所有经常 <code>du -sh ~/.cache/*</code> 的人。</sub>
</div>
