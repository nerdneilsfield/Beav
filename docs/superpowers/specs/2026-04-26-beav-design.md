# Beav — Linux Cache Cleaner Design

**Date:** 2026-04-26
**Status:** Draft, awaiting user review
**Inspired by:** [Mole](https://github.com/tw93/mole) (macOS, bash) — Beav is a pure-Go re-imagining for Linux.

---

## 1. Goal

A single-binary, pure-Go, Linux-first system cleaner. v1 ships two commands: `beav clean` (cache cleanup, age-aware) and `beav analyze` (interactive disk usage explorer). Status / optimize / uninstall stay out of v1.

## 2. Non-Goals

- macOS / Windows support (v2+).
- Real-time system monitor (use btop/htop).
- App uninstaller (Linux apps are package-managed; not Mole's macOS use case).
- Any operation that modifies files outside cache/log/tmp domains.
- Any auto-elevation: Beav never silently calls `sudo`.

## 3. Privilege Model

Single binary, two scopes selected by the `--system` flag and the effective UID:

| Invocation | Effective UID | Scope cleaned |
|---|---|---|
| `beav clean` | non-root | user (caller's `$HOME`) |
| `beav clean --system` | non-root | refuse, print `sudo beav clean --system` hint |
| `sudo beav clean --system` | root | system only (`/var/cache`, `/var/log`, `/tmp`, journal, pkg cache) |
| `sudo beav clean --all` | root | system + the original user's `$HOME` (resolved via `$SUDO_USER`) |
| `beav clean` as root login | root | refuses unless `--allow-root-home`; the only home is `/root` and that's usually not what's wanted |

After a non-root `beav clean`, if `/var/cache` etc. show non-trivial size, print a one-line hint: `system caches hold ~12 GiB; run 'sudo beav clean --system' to reclaim`.

## 4. Architecture

```
cmd/beav/main.go                  cobra entrypoint
internal/
  cli/                            cobra subcommands: clean, analyze, config, completion, version
  cleaner/
    registry/                     loads embedded + ~/.config/beav/cleaners.d YAML, merges by ID
    model/                        Cleaner, ExecutorType, AgeFilter structs
    executor/
      paths.go                    walk + age-filter + safe delete
      command.go                  whitelisted external command (apt, journalctl, npm, ...)
      journal.go                  journalctl --vacuum-time wrapper
      pkgcache.go                 apt clean / dnf clean / pacman -Sc / zypper clean
    safety/
      blacklist.go                hard-deny path prefixes
      bounds.go                   $HOME/$VARCACHE/$VARLOG/$TMP boundary check
      fs.go                       O_NOFOLLOW, st_dev cross-fs guard, lstat type filter
      procs.go                    "is VS Code / Chrome running?" guard
      whitelist.go                user ~/.config/beav/whitelist.txt
  ui/
    renderer.go                   Renderer interface (Plain, Spinner, JSON)
    cli/spinner.go                Mole-style line output (default for non-TTY → plain)
    tui/menu.go                   bubbletea main menu (no-args entry)
    tui/analyze.go                bubbletea wrapper around gdu lib
    json/json.go                  --output json renderer
  config/                         ~/.config/beav/config.yaml loader (yaml.v3, no viper)
  sysinfo/                        distro detect, SUDO_USER, isatty, GOCACHE/XDG resolution
  oplog/                          ~/.local/state/beav/operations.log (rolling 10MB×5)
cleaners/                         //go:embed YAML data
  user/{vscode,jetbrains,browsers,desktop,shells}.yaml
  user/langs/{npm,pnpm,yarn,bun,pip,cargo,go,gradle,maven}.yaml
  system/{apt,dnf,pacman,zypper,journal,varcache,tmp,kernels}.yaml
docs/
testdata/
Makefile
go.mod (go 1.24)
```

## 5. Cleaner Registry (YAML)

All cleaners — built-in and user — share one schema. Built-ins live in `cleaners/` and are embedded via `//go:embed`. User cleaners live in `~/.config/beav/cleaners.d/*.yaml` and **merge by ID**: a user file with the same `id` overrides the built-in's mutable fields (`enabled`, `min_age_days`, additional `paths`). Structural fields (`type`, `command`) are not overridable for safety.

### 5.1 Schema

```yaml
id: vscode-cache              # unique, kebab-case, namespaced
name: VS Code Cache
description: Editor blob and code cache; rebuilt on next launch.
scope: user                   # user | system
type: paths                   # paths | command | journal_vacuum | pkg_cache
enabled: true
min_age_days: 7               # null means "no age filter — requires --force-no-age"
time_field: mtime             # mtime | ctime (default mtime)
paths:                        # for type: paths — globs relative to $HOME or absolute
  - ~/.config/Code/Cache/*
  - ~/.config/Code/CachedData/*
  - ~/.config/Code/logs/*
  - ~/.cache/vscode-cpptools/**
  - ~/.vscode-server/data/CachedExtensionVSIXs/*
exclude:                      # globs, evaluated after paths
  - "**/keybindings.json"
running_processes:            # skip cleaning if any of these are running
  - code
  - code-insiders
needs_root: false
tags: [ide, editor]           # for --only / --skip filters
```

For `type: command`:

```yaml
id: pkg-apt-archives
name: APT downloaded packages
scope: system
type: pkg_cache
pkg_cache:
  manager: apt                # apt | dnf | pacman | zypper
needs_root: true
# min_age_days N/A for `apt clean`; use `apt-get autoclean` for "expired" semantics
```

For `type: journal_vacuum`:

```yaml
id: sys-journal
name: systemd journal
scope: system
type: journal_vacuum
min_age_days: 14
needs_root: true
```

### 5.2 Executor Whitelist

`type: command` is **not** an arbitrary shell hatch. The executor maps `pkg_cache.manager` to a hardcoded `exec.Command` invocation; YAML cannot specify the binary path or extra args. Same for `journal_vacuum` (always `/usr/bin/journalctl --vacuum-time=Nd`). This keeps user-provided YAML from being an RCE vector.

## 6. Safety Layers

Every path slated for deletion passes through these gates in order. Failing any gate skips the file (logged, not aborted).

1. **Allow-list boundary** — must be inside `$HOME`, `/var/cache`, `/var/log`, `/tmp`, `/var/tmp`. Anything else → refuse the entire cleaner with a load-time error.
2. **Hard blacklist** — never touched even if YAML asks: `/`, `/etc`, `/boot`, `/usr` (except explicitly listed `/usr/share/man/cache`), `/home/*` (except resolved caller home), `$HOME` itself, `$HOME/{Documents,Desktop,Downloads,Pictures,Videos,Music}`, `$HOME/.ssh`, `$HOME/.gnupg`, `$HOME/.password-store`, any directory containing a `.git` directory.
3. **No symlink follow** — open with `O_NOFOLLOW`, refuse symlinks pointing outside the parent.
4. **No cross-fs** — compare `st_dev` to the registry root; refuse if different.
5. **Type filter** — `lstat` must report regular file or directory; sockets/FIFOs/device nodes skipped.
6. **Age filter** — `now - mtime ≥ min_age_days` (or ctime per `time_field`). Always-on unless `--force-no-age`.
7. **Process guard** — if any name in `running_processes` matches a live process (`/proc` scan), the cleaner is skipped with a clear message.
8. **User whitelist** — `~/.config/beav/whitelist.txt` prefixes are excluded path-by-path.

Operations log (`~/.local/state/beav/operations.log`) records every delete with timestamp, path, size, cleaner id. Disable with `BEAV_NO_OPLOG=1`.

## 7. CLI Surface

```
beav                                interactive bubbletea menu (TTY only)
beav clean                          user-scope clean
beav clean --system                 system-scope (refuses without root)
beav clean --all                    user + system (root only)
beav clean --dry-run                no deletes, print what would happen
beav clean --only vscode,npm        run only matching cleaners (id or tag)
beav clean --skip browsers          inverse
beav clean --min-age=14d            global override
beav clean --min-age=cache=3d,logs=30d   per-tag override
beav clean --force-no-age           allow cleaners without age filter to run
beav clean --output json            machine-readable
beav clean --yes                    skip first-run dry-run hint

beav analyze [PATH]                 bubbletea wrapper around gdu library
beav config show                    print effective merged config + cleaner list
beav config edit                    open $EDITOR on ~/.config/beav/config.yaml
beav completion {bash|zsh|fish}     shell completion
beav version
```

## 8. UI Behavior

- **No args + TTY** → bubbletea main menu (Clean / Analyze / List cleaners / Quit).
- **`beav clean` + TTY** → spinner renderer (Mole-style line output). Each cleaner gets one line: spinner → `✓ <name> · <freed>` or `⚠ <name> · skipped (<reason>)`.
- **`beav clean` + non-TTY** (pipe, CI, SSH stdout to file) → plain renderer (no ANSI, no spinner, one line per cleaner).
- **`beav clean --output json`** → JSONL events: one object per cleaner finished, plus a final summary.
- **`beav analyze`** → always TUI; refuses if non-TTY (suggests `du`/`gdu` directly).

## 9. Default Cleaners (v1 Registry)

### User scope

| ID prefix | Covers | Default age |
|---|---|---|
| `desktop-trash` | `~/.local/share/Trash` | 30d |
| `desktop-thumbnails` | `~/.cache/thumbnails` | 30d |
| `desktop-recent` | `~/.local/share/recently-used.xbel` (rewrite, not delete) | 30d |
| `editor-vscode` | VS Code Cache, CachedData, logs, cpptools, vscode-server VSIXs | 7d |
| `editor-jetbrains` | `~/.cache/JetBrains/<Product><Version>` | 30d |
| `editor-nvim` | `~/.local/state/nvim/{log,swap,undo}` | 30d |
| `browser-firefox` | per-profile `cache2/`, `startupCache`, `OfflineCache` | 7d |
| `browser-chromium` | Chrome/Chromium/Edge `Cache`, `Code Cache`, `GPUCache` | 7d |
| `lang-npm` | `$npm_config_cache` (`_cacache`, `_npx`, `_logs`, `_prebuilds`) | 14d |
| `lang-pnpm` | `pnpm store path` | 14d |
| `lang-yarn` | `~/.cache/yarn` | 14d |
| `lang-bun` | `~/.bun/install/cache` | 14d |
| `lang-pip` | `~/.cache/pip` | 14d |
| `lang-cargo` | `$CARGO_HOME/registry/cache`, `$CARGO_HOME/git` | 30d |
| `lang-go` | `$GOCACHE` (paths mode, not `go clean -cache`) | 30d |
| `lang-gradle` | `~/.gradle/caches` | 30d |
| `lang-maven` | `~/.m2/repository/.cache` | 30d |
| `shell-zsh` | `~/.cache/zsh/*`, zcompdump | 30d |
| `cache-generic` | other `~/.cache/*` (anything not claimed above) | 30d |

### System scope

| ID prefix | Covers | Default age |
|---|---|---|
| `pkg-apt` | `apt clean` + `apt autoclean` (Debian/Ubuntu) | command-driven |
| `pkg-dnf` | `dnf clean all` | command-driven |
| `pkg-pacman` | `pacman -Sc --noconfirm` (keeps current versions) | command-driven |
| `pkg-zypper` | `zypper clean -a` | command-driven |
| `sys-journal` | `journalctl --vacuum-time=14d` | 14d |
| `sys-varcache` | `/var/cache/man`, `/var/cache/fontconfig`, etc. (allow-list per distro) | 30d |
| `sys-tmp` | `/tmp`, `/var/tmp` regular files | 10d |
| `sys-kernels` | keep latest 2 kernels (apt/dnf only; not pacman to avoid breaking dual-boot users) | N/A |

Cleaners detect their distro / package manager at runtime; an unsupported manager is silently skipped.

## 10. Safety Decisions Confirmed

- Mtime is the default time field; per-cleaner `time_field: ctime` override available.
- No "move to trash" mode — caches don't belong in trash.
- No first-run dry-run enforcement; just a one-line "first run? try `--dry-run`" hint, suppressed after first successful run via `~/.local/state/beav/state.json`.
- No transaction log / resume — cleaners are idempotent, Ctrl+C is safe.

## 11. Configuration

`~/.config/beav/config.yaml`:

```yaml
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
  - ~/.cache/important-thing
```

CLI flags > config file > built-in defaults.

`~/.config/beav/whitelist.txt` (one prefix per line) is the simpler always-skip list.

## 12. Dependencies

- Go 1.24
- `github.com/spf13/cobra` — CLI
- `github.com/charmbracelet/bubbletea` + `github.com/charmbracelet/lipgloss` + `github.com/charmbracelet/bubbles` — TUI
- `github.com/dundee/gdu/v5` — vendored as library for `analyze`
- `gopkg.in/yaml.v3` — config + cleaner registry
- `github.com/dustin/go-humanize` — size display
- `github.com/mattn/go-isatty` — TTY detection
- `log/slog` (stdlib) — logging

No viper, no zap, no third-party process libs (we read `/proc` directly).

## 13. Testing Strategy

- **Unit**: each safety gate has table-driven tests with fake `os.Stat` results; YAML loader + merger has fixtures for built-in vs override interaction.
- **Integration**: a `testdata/` filesystem skeleton (`testdata/fakehome`, `testdata/fakeroot`) — tests run cleaners against it with `--dry-run` and assert the planned-delete set, then run live and diff.
- **Cross-distro**: GitHub Actions matrix `{ubuntu-22.04, ubuntu-24.04, fedora-40, archlinux-latest}` running the system cleaners against a freshly-installed package set inside a container.
- **Coverage gate**: 80%+ on `internal/cleaner/safety/**` (non-negotiable; this is where mistakes delete data) and 70%+ overall.
- **Fuzz**: YAML loader fuzzed against malicious inputs (path traversal, command injection in command-type extra fields).
- **Manual**: a `make demo-vm` target spins up a Vagrant Ubuntu/Fedora/Arch VM with a synthetic dirty home, runs `beav clean --dry-run` and `beav clean`.

## 14. Out of Scope (v1.x roadmap)

- `beav status` (live system dashboard).
- `beav optimize` (rebuild caches, restart services).
- Flatpak / Snap / AppImage cleanup (sandboxed semantics need extra care).
- Browser profile-aware cleaning (cookies, localStorage opt-in).
- TUI editor for `cleaners.d/` YAML.
- Telemetry / metrics export.

## 15. Open Risks

- **Distro detection fragility**: `/etc/os-release` parsing is reliable but pkg-mgr presence ≠ active pkg mgr (e.g., `apt` exists on a system that primarily uses `nala`). Mitigation: detect via `command -v` and refuse cleanly when unsure.
- **VS Code Server on remote dev hosts**: cleaning `~/.vscode-server` while a remote session is live can break the session. Process guard catches local `code` but not the remote tunnel; we conservatively check for `node` processes whose cmdline contains `vscode-server`.
- **Browser profiles vary by channel**: Chrome stable / beta / canary all share path patterns; YAML uses globs (`~/.config/google-chrome*/*/Cache`) — verified before enabling per channel.
- **Snap / Flatpak shadow caches**: even if we don't clean them, they can re-fill `~/.cache` under sandbox-specific paths. Document, don't act.
