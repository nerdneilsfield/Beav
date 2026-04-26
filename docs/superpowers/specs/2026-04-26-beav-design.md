# Beav — Linux Cache Cleaner Design

**Date:** 2026-04-26
**Status:** Draft, awaiting user review
**Inspired by:** [Mole](https://github.com/tw93/mole) (macOS, bash) — Beav is a pure-Go re-imagining for Linux.

---

## 1. Goal

A single-binary, pure-Go, Linux-first system cleaner.

**Primary v1 commands:** `beav clean` (cache cleanup, age-aware) and `beav analyze` (interactive disk usage explorer).

**Auxiliary v1 commands** (also shipped, but trivial): `beav config`, `beav completion`, `beav version`.

Status / optimize / uninstall stay out of v1.

## 2. Non-Goals

- macOS / Windows support (v2+).
- Real-time system monitor (use btop/htop).
- App uninstaller (Linux apps are package-managed; not Mole's macOS use case).
- Any operation that modifies files outside cache/log/tmp domains, **with one explicit exception**: container runtime garbage collection via whitelisted CLI commands (`docker`/`podman` prune subcommands — see §5.4). These touch container-runtime-managed state (build cache, dangling/unused images, stopped containers, unused networks) but never touch persistent user data and never directly modify `/var/lib/{docker,containerd,kubelet}` paths. This exception **does not** extend to:
  - `/boot`, package databases, bootloader state — old-kernel removal is not in v1 (see §14).
  - `docker volume prune` — volumes are persistent state, not cache; volume cleanup is moved to v1.x (see §14).
- In-place file rewriting / transformation (e.g., editing `recently-used.xbel`) — v1 only deletes whole files/directories.
- Any auto-elevation: Beav never silently calls `sudo`.

## 3. Privilege Model

Single binary, two scopes selected by the `--system` flag and the effective UID:

| Invocation | Effective UID | Scope cleaned |
|---|---|---|
| `beav clean` | non-root | user (caller's `$HOME`) |
| `beav clean --system` | non-root | refuse, print `sudo beav clean --system` hint |
| `sudo beav clean --system` | root | system only (`/var/cache`, `/var/log`, `/tmp`, journal, pkg cache) |
| `sudo beav clean --all` | root | system + the original user's `$HOME` (resolved per §3.1) |
| `beav clean` as root login | root | refuses unless `--allow-root-home`; the only home is `/root` and that's usually not what's wanted |

After a non-root `beav clean`, if `/var/cache` etc. show non-trivial size, print a one-line hint: `system caches hold ~12 GiB; run 'sudo beav clean --system' to reclaim`.

### 3.1 Resolving the target home for `--all`

`$SUDO_USER` alone is not trustworthy (`su -`, `doas`, `sudo -E` from a hostile env, root login shells can all corrupt or clear it). Beav resolves the target home by **all three** of the following, and refuses if any disagree or are unavailable:

1. `$SUDO_UID` is set, parses to a non-zero numeric uid, and `getpwuid(uid)` succeeds.
2. `$SUDO_USER` is set, `getpwnam(name)` succeeds, and the resulting uid matches step 1.
3. The home directory from `getpwuid` exists, is owned by that uid (via `lstat` + `st_uid` check), and is not a symlink.

If any step fails, `--all` aborts with a clear error: "cannot determine invoking user safely; run `beav clean --system` and `beav clean` separately as that user." `--all` may also be given an explicit `--user=<name>` to bypass `$SUDO_*` entirely; the same three checks still apply to the named user.

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
      container.go                docker/podman {builder,image,container,network,system} prune --filter until=
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
  user/k8s/{minikube,helm,k9s,kubectl}.yaml
  user/containers/{docker-rootless,podman-rootless}.yaml
  system/{apt,dnf,pacman,zypper,journal,varcache,tmp}.yaml
  system/containers/{docker,podman}.yaml
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
type: paths                   # paths | command | journal_vacuum | pkg_cache | container_prune
enabled: true
min_age_days: 7               # null means "no age filter — requires --force-no-age"
time_field: mtime             # mtime | ctime (default mtime)
paths:                        # for type: paths — static globs (~ or absolute only)
  - ~/.config/Code/Cache/*
  - ~/.config/Code/CachedData/*
  - ~/.config/Code/logs/*
  - ~/.cache/vscode-cpptools/**
  - ~/.vscode-server/data/CachedExtensionVSIXs/*
path_resolvers:               # dynamic paths resolved at runtime by built-in resolvers (see §5.3)
  - resolver: npm_cache
    subpaths: [_cacache, _npx, _logs, _prebuilds]
exclude:                      # globs, evaluated after paths and path_resolvers
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

For `type: container_prune`:

```yaml
id: docker-builder-cache
name: Docker BuildKit cache
scope: system                 # rootful docker; rootless variant has scope: user
type: container_prune
container_prune:
  runtime: docker             # docker | podman
  target: builder             # builder | image | container | network | system  (volume is v1.x)
min_age_days: 14              # mapped to "--filter until=${age*24}h" — required
needs_root: true              # rootful only; rootless: false
```

### 5.2 Executor Whitelist

`type: command` is **not** an arbitrary shell hatch. The executor maps `pkg_cache.manager` to a hardcoded `exec.Command` invocation; YAML cannot specify the binary path or extra args. Same for `journal_vacuum` (always `/usr/bin/journalctl --vacuum-time=Nd`) and `container_prune` (see §5.4). This keeps user-provided YAML from being an RCE vector.

### 5.4 Container Runtime Cleaners

`type: container_prune` invokes `docker` or `podman` with a **fixed per-target argv** chosen from the table below. YAML chooses `runtime` (enum: `docker | podman`) and `target` (enum: `builder | image | container | network | system`); the executor builds the argv from this table — YAML cannot inject extra args.

`min_age_days` is **required** (`null` rejected at registry load) and converted to `${age*24}h` for the `until` filter where supported.

| runtime | target | argv | Notes |
|---|---|---|---|
| `docker` | `builder` | `docker builder prune -f --filter until=Nh` | `until` supported per [docker builder prune](https://docs.docker.com/reference/cli/docker/builder/prune/) |
| `docker` | `image` | `docker image prune -af --filter until=Nh` | `-a` removes all unused (not just dangling); `until` supported |
| `docker` | `container` | `docker container prune -f --filter until=Nh` | only stopped; `until` supported |
| `docker` | `network` | `docker network prune -f --filter until=Nh` | unused user-defined; `until` supported |
| `docker` | `system` | not in v1 | `docker system prune` already lacks `--all` for images by default; we expose individual targets instead for clarity |
| `podman` | `builder` | `podman builder prune -f --filter until=Nh` | matches docker semantics |
| `podman` | `image` | `podman image prune -af --filter until=Nh` | |
| `podman` | `container` | `podman container prune -f --filter until=Nh` | |
| `podman` | `network` | `podman network prune -f --filter until=Nh` | |
| `podman` | `system` | `podman system prune -f --filter until=Nh` | per [podman-system-prune docs](https://docs.podman.io/en/stable/markdown/podman-system-prune.1.html); does **not** prune volumes by default, which is the safety property we want |
| any | `volume` | **not in v1** — see §14 | `docker volume prune` only supports `label` filter, not `until`; needs a different executor design |

The registry loader rejects any `runtime`/`target` combination not in the table.

**Daemon guard:** before running, the executor calls `<runtime> info --format '{{.ServerVersion}}'` with a 3 s timeout. If the daemon is unreachable, the cleaner is skipped with reason `runtime_unavailable` — not an error.

**In-flight build/run guard:** before `target: builder | image`, the executor runs `<runtime> ps -q --filter status=running` and counts. If non-zero, those targets are skipped with reason `runtime_busy`; `container | network | system` still proceed (they only touch stopped/unused objects by definition).

**Rootless verification (user-scope cleaners):** the cleaner refuses to run unless **all** of the following hold (otherwise skipped with reason `runtime_not_rootless`):

1. `<runtime> info --format '{{.SecurityOptions}}'` (docker) / `--format '{{.Host.Security.Rootless}}'` (podman) reports rootless mode.
2. The active socket path (`docker context inspect` for docker, `podman info --format '{{.Host.RemoteSocket.Path}}'` for podman) is **owned by the invoking UID** (verified by `lstat`).
3. The socket path is under `/run/user/$UID/`, `$XDG_RUNTIME_DIR/`, or `$HOME/`.

If `DOCKER_HOST` is set in the environment, it is honored only if it points at a rootless socket meeting (2) and (3); otherwise the cleaner skips. Beav never overrides a user's explicit `DOCKER_HOST`.

For system-scope cleaners (`needs_root: true`, run under `sudo beav clean --system`), the daemon is assumed rootful and Beav uses the default socket; rootless verification is inverted (skip if rootless).

**No `--include-dangerous` flag, no `dangerous` tag in v1.** Every container_prune cleaner shipped in v1 operates only on caches/dangling-or-unused objects. Volume pruning, which is the only cleaner that could touch persistent state, is in v1.x (§14) behind a separate command with proper UX.

### 5.3 Path Resolvers

Many language toolchains store caches at user-configurable paths. YAML cannot embed shell calls, so dynamic paths are resolved by a **closed enum of built-in resolvers**:

| Resolver | Source | Fallback |
|---|---|---|
| `npm_cache` | `npm config get cache` (2s timeout) | `$HOME/.npm` |
| `pnpm_store` | `pnpm store path` (2s timeout) | `$HOME/.local/share/pnpm/store` |
| `yarn_cache` | `yarn cache dir` (2s timeout) | `$HOME/.cache/yarn` |
| `bun_cache` | `$BUN_INSTALL_CACHE_DIR` env | `$HOME/.bun/install/cache` |
| `pip_cache` | `pip cache dir` (2s timeout) | `$HOME/.cache/pip` |
| `cargo_home` | `$CARGO_HOME` env | `$HOME/.cargo` |
| `gocache` | `go env GOCACHE` (2s timeout) | `$HOME/.cache/go-build` |
| `gradle_home` | `$GRADLE_USER_HOME` env | `$HOME/.gradle` |
| `maven_local_repo` | settings.xml `<localRepository>` (2s timeout via `mvn help:evaluate`) | `$HOME/.m2/repository` |
| `xdg_cache` | `$XDG_CACHE_HOME` env | `$HOME/.cache` |
| `xdg_state` | `$XDG_STATE_HOME` env | `$HOME/.local/state` |
| `xdg_data` | `$XDG_DATA_HOME` env | `$HOME/.local/share` |

Resolver semantics:

- **Resolution failure** (binary missing, returns non-zero, times out, returns empty/non-absolute path): use the fallback. Failure is logged at debug level; the cleaner still runs against the fallback.
- **Boundary check after resolution**: every resolved path is re-validated against §6 layer 1 (allow-list) and layer 2 (blacklist). A resolver returning, say, `/etc/foo` is rejected at runtime — the cleaner skips with an error.
- **No env-var injection in YAML**: `path_resolvers` lists resolver names from the table above; arbitrary `${VAR}` interpolation in `paths:` is not supported.
- **Subpaths**: `subpaths: [a, b]` joined under the resolved root; each subpath is treated as a glob pattern under that root.

User-supplied YAML can use these resolvers but cannot define new ones; adding a resolver requires a Go-level change (gated by code review).

## 6. Safety Layers

### 6.1 Per-cleaner gates (load time)

Refusing a whole cleaner before it touches disk:

1. **Allow-list boundary** — every static `path` and every resolver fallback must be inside `$HOME`, `/var/cache`, `/var/log`, `/tmp`, `/var/tmp`. Anything else → refuse the entire cleaner at registry load time with a clear error.
2. **Hard blacklist** — refusal at registry load: `/`, `/etc`, `/boot`, `/usr` (no exceptions in v1), `/home/*` (except the resolved caller home), `$HOME` itself, `$HOME/{Documents,Desktop,Downloads,Pictures,Videos,Music}`, `$HOME/.ssh`, `$HOME/.gnupg`, `$HOME/.password-store`, `$HOME/.docker/config.json`, `$HOME/.kube/config` (and any non-`cache`/non-`http-cache` child of `$HOME/.kube/`), `/var/lib/docker` (must be cleaned via `container_prune`, never raw-pathed), `/var/lib/containerd`, `/var/lib/kubelet`. A directory containing `.git` is checked at runtime per-entry, not at load.

### 6.2 Per-entry gates (runtime, recursive)

The executor walks the matched tree using **`openat`-style descent with a directory file descriptor at every level** (Go: `golang.org/x/sys/unix.Openat` + `os.NewFile` wrapped in a `*Dir`). It never resolves a full path string with `os.Open` after the initial root open, so an attacker swapping a parent into a symlink mid-walk cannot redirect us.

For every entry encountered during the walk:

3. **lstat each level** — at every directory descent and before any unlink, call `fstatat` with `AT_SYMLINK_NOFOLLOW`. A symlink is never followed; if the entry is a symlink, it is skipped (or unlinked only if the cleaner explicitly opts into `delete_symlinks: true`, which v1 cleaners do not).
4. **No cross-fs** — `st_dev` of every entry must equal the `st_dev` captured when the cleaner's root was first opened. Mismatch → skip subtree.
5. **Type filter** — only regular files and directories. Sockets/FIFOs/character/block devices are skipped silently.
6. **`.git` guard** — if a directory's children include `.git`, the entire subtree is skipped (it's a git work tree, not cache).
7. **Recursive age filter** — `now - mtime(entry) ≥ min_age_days` is checked **per individual file**, not per directory. A directory itself is considered for removal **only after** its children have been processed and only if it is then empty *and* its own mtime also meets the threshold. Whole-tree `RemoveAll` is forbidden inside the executor; deletion is always a bottom-up walk.
8. **Re-stat before unlink (TOCTOU)** — immediately before calling `unlinkat`, re-`fstatat` the entry; if `(st_ino, st_dev, st_size, st_mtim)` differs from the value captured during the walk, the entry is skipped. This narrows the TOCTOU window to a single syscall.
9. **Process guard** — before the cleaner starts, scan `/proc/*/comm` and `/proc/*/cmdline` for any name in `running_processes`. If matched, the cleaner is skipped with a clear message. (Re-checked once mid-walk for long-running cleaners.)
10. **User whitelist** — entries whose absolute path has any prefix listed in `~/.config/beav/whitelist.txt` or `config.yaml`'s `whitelist:` array are skipped.

The two whitelist sources (txt + yaml) are merged into a single in-memory prefix set; they are not redundant in code, but the user-facing surface is one concept. We still document both, since `whitelist.txt` is convenient for ops scripts and `config.yaml` is convenient for declarative dotfile management.

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
- **`beav clean --output json`** → JSONL events on stdout, one JSON object per line (see §8.1 for schema). Logs go to stderr.
- **`beav analyze`** → always TUI; refuses if non-TTY (suggests `du`/`gdu` directly).

### 8.1 JSONL output schema

Stable across v1.x (additive only). Every line is one of:

```json
{"event":"start","cleaner_id":"editor-vscode","name":"VS Code Cache","scope":"user","type":"paths","dry_run":false,"ts":"2026-04-26T12:00:00Z"}
{"event":"deleted","cleaner_id":"editor-vscode","path":"/home/dq/.config/Code/Cache/x","size":4096,"ts":"..."}
{"event":"skipped","cleaner_id":"editor-vscode","path":"/home/dq/.config/Code/Cache/y","reason":"age_too_recent","ts":"..."}
{"event":"cleaner_skipped","cleaner_id":"container-docker-builder","reason":"runtime_unavailable","ts":"..."}
{"event":"command_output","cleaner_id":"pkg-apt","stream":"stdout","line":"Reading package lists...","ts":"..."}
{"event":"error","cleaner_id":"sys-journal","path":null,"reason":"command_failed","detail":"journalctl exited 1: failed to vacuum","ts":"..."}
{"event":"finish","cleaner_id":"editor-vscode","status":"ok","files_deleted":42,"bytes_freed":12345678,"errors":0,"duration_ms":83}
{"event":"summary","cleaners_run":12,"cleaners_skipped":1,"cleaners_errored":1,"files_deleted":1024,"bytes_freed":987654321,"errors":2,"duration_ms":4200}
```

**Per-entry skip `reason` enum** (event `skipped`):
`age_too_recent`, `whitelisted`, `excluded`, `cross_fs`, `symlink`, `wrong_type`, `inside_git`, `blacklisted`, `permission_denied`, `toctou_changed`, `dry_run`.

**Whole-cleaner skip `reason` enum** (event `cleaner_skipped`):
`disabled`, `not_selected`, `wrong_scope`, `running_process`, `runtime_unavailable`, `runtime_busy`, `runtime_not_rootless`, `manager_not_installed`, `boundary_violation`, `resolver_failed`, `no_matches`, `user_declined`.

**Error event schema** (event `error`): emitted when a cleaner encounters a non-skip failure during execution — distinct from `skipped` (intentional) and `cleaner_skipped` (precondition not met). Fields:

```json
{"event":"error",
 "cleaner_id":"<id>",
 "path":"<absolute path or null>",
 "reason":"<enum>",
 "detail":"<human-readable string>",
 "ts":"<RFC3339>"}
```

Error `reason` enum: `unlink_failed`, `walk_failed`, `command_failed`, `command_timeout`, `oplog_write_failed`, `internal`. Each cleaner may emit multiple `error` events; the `finish` event then carries `status: "error"` and the count of errors encountered.

**`finish.status` enum:** `ok` | `skipped` | `error`. `ok` if no `error` events were emitted; `skipped` if the cleaner emitted `cleaner_skipped` and never started work; `error` if at least one `error` event was emitted.

### 8.2 Exit codes

- `0` — every selected cleaner finished without an `error` event. Expected skips — whether per-entry (age, whitelist, blacklist, TOCTOU) or whole-cleaner precondition skips (`running_process`, `runtime_unavailable`, `runtime_busy`, `manager_not_installed`, etc.) — do **not** cause non-zero; they are normal.
- `1` — usage error (bad flags, unknown cleaner id, refused privilege escalation request).
- `2` — config / registry load error (malformed YAML, allow-list violation in YAML, missing required field).
- `3` — at least one cleaner reported an `error` event during execution (permission denied on a path that should have been writable, syscall error, command-type cleaner exited non-zero). Other cleaners still completed; output reflects partial success.
- `4` — aborted by signal (Ctrl+C). Already-deleted entries remain deleted; oplog is flushed.

`--dry-run` exits `0` unless flag, config, or registry validation fails (in which case `1` or `2` applies as above).

## 9. Default Cleaners (v1 Registry)

### User scope

| ID prefix | Covers | Default age |
|---|---|---|
| `desktop-trash` | `~/.local/share/Trash` | 30d |
| `desktop-thumbnails` | `~/.cache/thumbnails` | 30d |
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
| `k8s-minikube-cache` | `~/.minikube/cache/` | 30d |
| `k8s-helm-cache` | `~/.cache/helm/` | 30d |
| `k8s-k9s-cache` | `~/.cache/k9s/` | 30d |
| `k8s-kubectl-cache` | `~/.kube/cache/`, `~/.kube/http-cache/` (never `~/.kube/config`) | 30d |
| `container-rootless-docker` | rootless: `docker {builder,image,container,network} prune --filter until=` | 14d |
| `container-rootless-podman` | rootless: `podman {builder,image,container,network} prune --filter until=` | 14d |

> Note: there is intentionally **no** "catch-all `~/.cache/*` cleaner" in v1. `~/.cache` legitimately holds anything an app's author decided to put there, including data the app cannot regenerate. v1 cleans only known-safe subdirectories explicitly listed by ID. Users wanting to clean an unlisted directory add a tiny YAML to `~/.config/beav/cleaners.d/`.

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
| `container-docker-builder` | rootful: `docker builder prune --filter until=` | 14d |
| `container-docker-images` | rootful: `docker image prune -af --filter until=` (unused images) | 30d |
| `container-docker-containers` | rootful: `docker container prune --filter until=` (stopped only) | 7d |
| `container-docker-networks` | rootful: `docker network prune --filter until=` | 30d |
| `container-podman-system` | rootful podman: `podman system prune --filter until=` (no volumes by default) | 14d |

Cleaners detect their distro / package manager at runtime; an unsupported manager is silently skipped.

Old-kernel removal is **not** in v1: it touches `/boot`, the package DB, and the bootloader, all of which are outside the cache/log/tmp domain declared in §2. It is filed as a v1.x feature behind an explicit `beav prune-kernels` command with strong confirmation (see §14).

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

**Whitelist sources** (merged into a single in-memory prefix set):
- `~/.config/beav/config.yaml` `whitelist:` array — declarative, dotfile-managed.
- `~/.config/beav/whitelist.txt` — one prefix per line, easy for ad-hoc / scripted appends.

The two are functionally equivalent; we ship both because the cost is one file reader and the user-facing concept is identical.

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
- `beav prune-kernels` — dedicated maintenance command for old-kernel removal. Touches `/boot` and the package DB; needs explicit confirmation, dry-run-first UX, and per-distro logic (apt vs dnf vs pacman). Out of scope for v1's cache-only domain.
- `beav prune-recents` — rewrites `~/.local/share/recently-used.xbel`. Requires a `transform` executor type (read → filter → atomic-replace with backup), which v1's executor enum does not include.
- `beav prune-volumes` — Docker/Podman volume pruning. Volumes are persistent data, not cache, and `docker volume prune` only supports the `label` filter (no `until`), so age-based deletion is not directly available. Needs a dedicated executor: `inspect` each volume, compute "last referenced" age via container references / `Mountpoint` mtime, then `volume rm` selectively. Will live behind a separate command with confirm-by-typing UX, never enabled by default.
- Flatpak / Snap / AppImage cleanup (sandboxed semantics need extra care).
- Browser profile-aware cleaning (cookies, localStorage opt-in).
- TUI editor for `cleaners.d/` YAML.
- Telemetry / metrics export.

## 15. Open Risks

- **Distro detection fragility**: `/etc/os-release` parsing is reliable but pkg-mgr presence ≠ active pkg mgr (e.g., `apt` exists on a system that primarily uses `nala`). Mitigation: detect via `command -v` and refuse cleanly when unsure.
- **VS Code Server on remote dev hosts**: cleaning `~/.vscode-server` while a remote session is live can break the session. Process guard catches local `code` but not the remote tunnel; we conservatively check for `node` processes whose cmdline contains `vscode-server`.
- **Browser profiles vary by channel**: Chrome stable / beta / canary all share path patterns; YAML uses globs (`~/.config/google-chrome*/*/Cache`) — verified before enabling per channel.
- **Snap / Flatpak shadow caches**: even if we don't clean them, they can re-fill `~/.cache` under sandbox-specific paths. Document, don't act.
- **Docker / Podman daemon availability**: `container_prune` cleaners depend on a running daemon and a usable socket. We treat unreachable daemons as "skip with reason `runtime_unavailable`", never as an error, so machines without Docker installed produce a clean run. False negatives (daemon up but socket permission denied because the user is not in the `docker` group) are reported with the actual error.
- **Docker image churn on dev hosts**: `docker image prune -a --filter until=720h` (30 d) will remove any image not used by a container in 30 days. On dev hosts that pull large base images for occasional builds, this can cause unexpected re-pulls. Mitigation: default tag is `dev-only`; users with this concern can set `enabled: false` in config.
- **k8s control-plane / node hosts**: cleaners assume a workstation, not a node. `/var/lib/kubelet` and `/var/lib/containerd` are blacklisted in §6.1; an admin running Beav on a node by mistake will not damage workloads. We do not attempt `crictl` cleanup in v1.
