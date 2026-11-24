# Safety Policy, Bypass Controls, and Configuration

By default, Beav enforces conservative boundaries so routine cache cleanups do not touch personal data or live workspaces. Starting in v0.3.0, every restriction except literal root `/` protection is configurable in `~/.config/beav/config.yaml`, in the `[4] Safety & Config` TUI tab, via `beav config safety`, or per invocation with CLI flags.

## Default safety boundaries

With default settings (`DefaultPolicy`):

- **Allow-list roots**: Targets must reside under the resolved home directory or `/var/cache`, `/var/log`, `/tmp`, `/var/tmp`.
- **Blacklist prefixes**: System prefixes (`/etc`, `/boot`, `/usr`, `/proc`, `/sys`, `/dev`, `/run`, `/var/lib/docker`, `/var/lib/containerd`, `/var/lib/kubelet`) and home folders (`Documents`, `Desktop`, `Downloads`, `Pictures`, `Videos`, `Music`, `.ssh`, `.gnupg`, `.password-store`) are blocked. `Downloads` has a narrow exception for stale partial-download suffixes only.
- **Traversal and identity guards**: Directory walks use `openat` + `O_NOFOLLOW`, reject mount-point crossings, skip `.git` directories, enforce per-file `mtime` age thresholds (plus minimum age floors on `managed_cache`, `debris`, and `rustup`), and skip targets whose owning tool process or open file descriptor is active.
- **Root protection**: Running as `root` refuses `/root` unless `--allow-root-home` or `safety.allow_root_home: true` is set.
- **Heavy cleaners**: Cleaners marked `impact: heavy` are skipped unless authorized with `--include-heavy`, explicit `--only <id|tag>`, `--unsafe`, or manual selection in the TUI.

> [!WARNING]
> Bypassing the allow-list, blacklist, age floor, or symlink guards removes protections that prevent accidental deletion outside cache directories. Verify targets with `beav scan` or `--dry-run` before running real deletions under `--unsafe` or bypass flags.

## CLI bypass flags

Pass these flags to `clean`, `scan`, `purge`, `tui`, or root `beav` for a single run:

| Flag | Effect |
| --- | --- |
| `--unsafe`, `--force` | Enable full bypass (`bypass_all`): skips allow-list, blacklist (except `/`), age floors, process checks, heavy guard, root-home guard, and enables `type: command` cleaners |
| `--allow-path <path>` | Append a custom allowed root prefix for this run (repeatable) |
| `--bypass-allow-list` | Allow targets outside home and default system roots |
| `--bypass-blacklist` | Allow paths matching default or custom blacklist rules (literal `/` remains blocked) |
| `--bypass-age-floor` | Allow `0d` retention on profiles with built-in minimum age floors |
| `--ignore-running` | Skip running-process, lock-file, and open-FD checks |
| `--follow-symlinks` | Traverse symlinks and accept symlinked external tool binaries |
| `--allow-cross-fs` | Allow crossing mount-point / filesystem boundaries |
| `--allow-root-home` | Allow targeting `/root` when running as `root` |

Setting `BEAV_UNSAFE=1` (or `true`) enables `bypass_all` from the environment.

## Configuration file (`config.yaml`)

Default path: `~/.config/beav/config.yaml` (override with `BEAV_CONFIG_DIR` or `--config-dir`).

Precedence order (highest to lowest): **CLI flags -> `config.yaml` -> cleaner YAML / built-in defaults**.

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

### `safety` fields reference

| Key | Type | Default | Behavior |
| --- | --- | --- | --- |
| `bypass_all` | `bool` | `false` | Master bypass switch for all configurable checks below (literal `/` stays blocked) |
| `allow_list` | `[]string` | `[]` | Extra root prefixes allowed during cleanup |
| `replace_allow_list` | `bool` | `false` | When `true`, only paths in `allow_list` are permitted instead of extending defaults |
| `disable_allow_list` | `bool` | `false` | Disable allow-list restriction altogether |
| `blacklist` | `[]string` | `[]` | Custom deny prefixes added on top of built-in blacklist |
| `replace_blacklist` | `bool` | `false` | When `true`, replace built-in blacklist with `blacklist` |
| `disable_blacklist` | `bool` | `false` | Disable blacklist checks (literal `/` stays blocked) |
| `ignore_running_processes` | `bool` | `false` | Skip running-process, lock-file, and open-FD safety skips |
| `bypass_age_floor` | `bool` | `false` | Remove minimum retention age floors on protected profiles |
| `follow_symlinks` | `bool` | `false` | Allow symlink traversal and symlinked external tool paths |
| `allow_cross_fs` | `bool` | `false` | Allow walking across filesystem boundaries |
| `allow_root_home` | `bool` | `false` | Allow cleaning `/root` when executed as `root` |
| `allow_command_cleaners` | `bool` | `false` | Allow `type: command` cleaners loaded from `cleaners.d/` |

### Managing safety policy from CLI

```bash
beav config safety show
beav config safety set allow_command_cleaners true
beav config safety allow-add /opt/build-cache
beav config safety deny-add ~/important-work
beav config safety allow-remove /opt/build-cache
beav config safety deny-remove ~/important-work
```

## Custom cleaners (`cleaners.d/`)

Place custom YAML cleaner files inside `~/.config/beav/cleaners.d/`.

### Path cleaners (`type: paths`)

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

### Shell/command cleaners (`type: command`)

To define custom command-based cleaners, set `safety.allow_command_cleaners: true` (or run with `--unsafe`). Commands execute via `exec.CommandContext` without shell interpolation unless you explicitly invoke a shell such as `sh`. When `dry_run_command` is omitted, dry-run/scan preview skips running `command` instead of executing it.

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
