<div align="center">
  <h1>🦫 Beav</h1>
  <p><em>The hardworking beaver that grooms your Linux disk — dredges caches, vacuums logs, fells build artifacts.</em></p>
  <p>
    <a href="https://github.com/nerdneilsfield/Beav/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/nerdneilsfield/Beav/ci.yml?branch=master&style=flat-square&label=CI&logo=github" alt="CI"></a>
    <a href="https://github.com/nerdneilsfield/Beav/releases"><img src="https://img.shields.io/github/v/release/nerdneilsfield/Beav?style=flat-square&include_prereleases&label=release" alt="Release"></a>
    <a href="https://goreportcard.com/report/github.com/nerdneilsfield/Beav"><img src="https://img.shields.io/badge/go%20report-A-brightgreen?style=flat-square" alt="Go Report"></a>
    <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.24+">
    <img src="https://img.shields.io/badge/platform-Linux-FCC624?style=flat-square&logo=linux&logoColor=black" alt="Linux">
    <a href="LICENSE"><img src="https://img.shields.io/github/license/nerdneilsfield/Beav?style=flat-square" alt="License"></a>
    <a href="README_ZH.md"><img src="https://img.shields.io/badge/lang-中文-red?style=flat-square" alt="中文"></a>
  </p>
</div>

---

> 🇨🇳 **中文文档：** [README_ZH.md](README_ZH.md)

**Beav** is a single-binary, pure-Go cache cleaner for Linux. Inspired by [Mole](https://github.com/tw93/mole) on macOS, it re-imagines the workflow for the messier, more diverse world of Linux desktops and dev boxes. It clears the rubble around browsers, IDEs, language toolchains, container runtimes, and package managers — and stops there. **Your data is not cache. Beav knows the difference.**

## ✨ Why Beav?

- 🦫 **Age-aware by default.** Files are deleted only after they've been untouched for *N* days. Reopen your IDE tomorrow and the cache rebuilds itself; this week's work is never at risk.
- 🛡️ **Safety as a contract, not a vibe.** Every deletion goes through a stack of openat-anchored gates: allow-list bounds, hard-deny blacklist, no-symlink-cross, no-cross-filesystem, `.git`-tree guard, and a pre-unlink TOCTOU re-stat.
- 🧰 **Everything is data.** Cleaners are YAML files. Built-ins are embedded in the binary; drop your own into `~/.config/beav/cleaners.d/` to extend.
- 🐙 **Wide ecosystem coverage.** VS Code, JetBrains, Neovim, Firefox, Chromium-family, npm/pnpm/yarn/bun, pip, cargo, Go build cache, Gradle, Maven, apt/dnf/pacman/zypper, journald, Docker, Podman, helm, k9s, kubectl, minikube. Out of the box.
- 🚦 **Two scopes, one binary.** `beav clean` for your home; `sudo beav clean --system` for the box; `sudo beav clean --all` for both, with bullet-proof `SUDO_USER` resolution.
- 🎨 **Three faces.** A spinner UI in your terminal, plain text on a pipe, and JSONL events for scripts and dashboards.
- 🚫 **No surprises.** Beav never silently calls `sudo`, never follows symlinks out of bounds, never touches `~/.ssh`, `~/.gnupg`, `~/.kube/config`, `~/.docker/config.json`, `/var/lib/{docker,containerd,kubelet}`, `/etc`, `/boot`, or `/usr`.

## 🚀 Quick start

```bash
# Build from source (requires Go 1.24+)
git clone https://github.com/nerdneilsfield/Beav
cd Beav
make build
./bin/beav version
```

```bash
# Day-one workflow
beav clean --dry-run                # see what would happen — start here, always
beav clean                          # clean your $HOME caches
sudo beav clean --system            # clean /var/cache, journal, package mgr cache
sudo beav clean --all               # both, in one go
beav analyze                        # interactive disk explorer (gdu under the hood)
```

## 🎬 Cleanup at a glance

```
$ beav clean
  ✓ VS Code Cache · 312.7 MB freed
  ✓ JetBrains IDE caches · 1.4 GB freed
  ✓ npm cache · 487.3 MB freed
  ✓ Go build cache · 2.1 GB freed
  ✓ Firefox cache · 145.0 MB freed
  ○ Chromium-family cache · skipped (running_process)

Freed 4.4 GB across 5 cleaners.
```

> Always run `--dry-run` first on a new machine. The snippet above shows the real thing once you know what to expect.

<details>
<summary><strong>📋 Full command reference</strong></summary>

```bash
beav                                 # launch the interactive bubbletea menu (TTY only)

beav clean                           # user-scope clean
beav clean --system                  # system-scope (refuses without root)
beav clean --all                     # user + system (root + valid SUDO_USER)
beav clean --dry-run                 # plan only; no deletions
beav clean --only vscode,langs       # filter by ID prefix or tag
beav clean --skip browsers           # inverse filter
beav clean --min-age=14d             # global age override
beav clean --min-age=langs=3d,logs=30d   # per-tag override
beav clean --force-no-age            # enable cleaners that opt out of age filtering
beav clean --output json             # JSONL events on stdout
beav clean --yes                     # skip first-run dry-run hint
beav clean --user=alice --all        # explicit SUDO_USER override

beav analyze [PATH]                  # interactive TUI disk explorer (gdu)
beav config show                     # print effective merged config + cleaner list
beav config edit                     # open $EDITOR on ~/.config/beav/config.yaml
beav completion {bash|zsh|fish}      # shell completion
beav version
```

**Exit codes**

| Code | Meaning |
|------|---------|
| `0`  | All selected cleaners finished without error events |
| `1`  | Usage error (bad flags, refused privilege escalation) |
| `2`  | Config / registry / path-safety validation failed |
| `3`  | At least one cleaner emitted an `error` event |
| `4`  | Aborted by signal (Ctrl-C) |

</details>

## 🎯 What does Beav clean?

| Domain | Cleaners |
|--------|----------|
| **Desktop** | Trash, thumbnail cache |
| **Editors** | VS Code (incl. server / cpptools), JetBrains family, Neovim state |
| **Browsers** | Firefox, Chrome, Chromium, Edge — `Cache`, `Code Cache`, `GPUCache` only (never profiles, cookies, or logins) |
| **Languages** | npm, pnpm, yarn, bun, pip, cargo, Go build cache, Gradle, Maven |
| **Shells** | `~/.cache/zsh`, zcompdump |
| **Kubernetes** | minikube image cache, helm, k9s, `kubectl` cache (never `~/.kube/config`) |
| **Containers (rootless)** | Docker / Podman builder + image prune |
| **Package managers** | apt, dnf, pacman, zypper |
| **System logs** | systemd journal (via `journalctl --vacuum-time`, never raw `rm`) |
| **System tmp** | `/tmp`, `/var/tmp`, stragglers in `/var/cache` |
| **Containers (rootful)** | Docker builder / image / container / network; Podman system prune |

<details>
<summary><strong>🔬 Default age thresholds</strong></summary>

| Category | Default `min_age_days` | Rationale |
|----------|---|---|
| Browser & IDE caches | 7 | A week without a click means it's not in your active loop |
| Package manager downloads (apt/npm/pip/yarn/bun) | 14 | Mid-conservative; cheap to redownload |
| Build artifacts (cargo, go, gradle, maven) | 30 | Rebuild cost is real |
| systemd journal | 14 | `journalctl --vacuum-time=14d` |
| `/tmp`, `/var/tmp` | 10 | Matches systemd-tmpfiles defaults |
| `~/.local/share/Trash` | 30 | Give yourself time to undo |
| Container builder/image (rootful) | 14 / 30 | Aggressive on builder cache, careful with images |

Override globally with `--min-age=Nd`, per tag with `--min-age=langs=3d,logs=30d`, or per cleaner ID via `~/.config/beav/config.yaml`.

</details>

## 🛡️ Safety architecture

Beav treats deletion the way a surgeon treats incisions — every cut is checked twice.

<details open>
<summary><strong>The eight gates every entry passes through</strong></summary>

1. **Load-time allow-list** — every static path and every resolver fallback must lie under `$HOME`, `/var/cache`, `/var/log`, `/tmp`, or `/var/tmp`. A YAML cleaner pointing anywhere else is *refused at load*, not at runtime.
2. **Hard blacklist** — `/`, `/etc`, `/boot`, `/usr`, `/proc`, `/sys`, `/dev`, `/run`, `~/.ssh`, `~/.gnupg`, `~/.password-store`, `~/.docker/config.json`, `~/.kube/config`, `/var/lib/{docker,containerd,kubelet}`, plus `~/{Documents,Desktop,Downloads,Pictures,Videos,Music}` are never touched, even if a YAML asks.
3. **Anchored descent** — every path is opened by walking from the safe-root with `openat` + `O_NOFOLLOW` at every component. A symlink anywhere in the path → refused.
4. **Cross-filesystem guard** — `st_dev` is captured at the root and re-checked on every entry; mounted volumes are skipped.
5. **`.git` guard** — any directory containing a `.git` child is treated as a working tree and skipped wholesale, including the directory itself.
6. **Per-file age filter** — `now - mtime ≥ min_age_days` is checked on **each individual file**, not the parent directory. Empty parents are removed bottom-up afterwards.
7. **TOCTOU re-stat** — immediately before `unlinkat`, the entry is re-stat'd; any drift in `(ino, dev, size, mtim)` aborts the delete.
8. **Process guard** — Beav scans `/proc` for `code`, `chromium`, `firefox`, `idea`, etc. — running editors and browsers stay untouched.

</details>

<details>
<summary><strong>Privilege model: why Beav never auto-sudoes</strong></summary>

| Invocation | Effective UID | Scope cleaned |
|---|---|---|
| `beav clean` | non-root | user (caller's `$HOME`) |
| `beav clean --system` | non-root | refuses with a `sudo` hint |
| `sudo beav clean --system` | root | system only |
| `sudo beav clean --all` | root | system + the original user's `$HOME` |
| `beav clean` as root login | root | refuses unless `--allow-root-home` |

`--all` resolves the target home through **three independent checks** — `$SUDO_UID`, `$SUDO_USER`, and `lstat`-verified ownership of the home directory — and refuses if any disagree. `--user=name` provides an explicit override that still passes all three checks.

</details>

<details>
<summary><strong>Container rootless verification</strong></summary>

For user-scope container cleaners, Beav refuses unless **all** hold:

1. The runtime reports rootless mode (`docker info -f '{{.SecurityOptions}}'` / `podman info -f '{{.Host.Security.Rootless}}'`).
2. The active socket (`docker context inspect` / `podman info`) is owned by the invoking UID.
3. The socket lives under `/run/user/$UID/`, `$XDG_RUNTIME_DIR/`, or `$HOME/`.

If `DOCKER_HOST` is set, Beav honours it only if it points at a verified rootless socket; otherwise the cleaner skips with `runtime_not_rootless`. System-scope cleaners invert this — they refuse to talk to a rootless daemon.

</details>

## 📜 Configuration

Beav reads `~/.config/beav/config.yaml` if present. CLI flags > config file > built-in defaults.

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

Custom cleaners live as YAML in `~/.config/beav/cleaners.d/`:

<details>
<summary><strong>Example: clean an obscure tool's cache</strong></summary>

```yaml
# ~/.config/beav/cleaners.d/zola.yaml
- id: lang-zola
  name: Zola build cache
  scope: user
  type: paths
  min_age_days: 14
  paths:
    - ~/.cache/zola/**
  tags: [langs, static-site]
```

`beav config show | jq '.cleaners[] | select(.id=="lang-zola")'` to verify.

</details>

## 🧪 Output formats

<details>
<summary><strong>JSONL events for pipelines</strong></summary>

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

Each event type emits only the fields it owns — no padding, stable schema, easy to consume.

</details>

## 🚧 Out of scope (for now)

Beav v1 is intentionally narrow. The following are filed as v1.x:

- **`beav prune-kernels`** — old-kernel removal touches `/boot` and the package DB; needs its own confirmation UX.
- **`beav prune-volumes`** — Docker/Podman volumes are persistent state, not cache. `docker volume prune` doesn't even support `--filter until=`, so a proper inspect-based executor is required.
- **`beav prune-recents`** — rewriting `~/.local/share/recently-used.xbel` would need a `transform` executor type with backup + atomic-replace.
- **Flatpak / Snap / AppImage** — sandboxed semantics need careful design.
- **`beav status` / `beav optimize`** — out of scope; use `btop`, `htop`, `systemctl`.

## 🤝 Contributing

Custom cleaners are easy to add — drop a YAML in `cleaners/user/` (or `cleaners/system/`) and open a PR. The schema lives in [docs/superpowers/specs/2026-04-26-beav-design.md §5.1](docs/superpowers/specs/2026-04-26-beav-design.md). Make sure `go test ./...` and `golangci-lint run ./...` are green. New executor types (anything beyond `paths / command / journal_vacuum / pkg_cache / container_prune`) require a design discussion first.

## 📖 Docs

- [Design spec](docs/superpowers/specs/2026-04-26-beav-design.md) — every safety decision documented.
- [Implementation plan](docs/superpowers/plans/2026-04-26-beav-implementation.md) — 28 tasks across 9 phases, TDD.

## 📜 License

MIT. See [LICENSE](LICENSE).

## 🙏 Credits

- [Mole](https://github.com/tw93/mole) — the macOS muse.
- [gdu](https://github.com/dundee/gdu) — powering `beav analyze`.
- [Charm](https://charm.sh/) — bubbletea, lipgloss, bubbles.

---

<div align="center">
<sub>Made with care for the people who type <code>du -sh ~/.cache/*</code> too often.</sub>
</div>
