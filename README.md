<div align="center">
  <h1>🦫 Beav</h1>
  <p><strong>Clean Linux developer, AI, container, and system caches with configurable safety policies.</strong></p>
  <p>
    <a href="https://github.com/nerdneilsfield/Beav/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/nerdneilsfield/Beav/ci.yml?branch=master&style=flat-square&label=CI&logo=github" alt="CI"></a>
    <a href="https://github.com/nerdneilsfield/Beav/releases"><img src="https://img.shields.io/github/v/release/nerdneilsfield/Beav?style=flat-square&include_prereleases&label=release" alt="Release"></a>
    <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.24+">
    <img src="https://img.shields.io/badge/platform-Linux-FCC624?style=flat-square&logo=linux&logoColor=black" alt="Linux">
    <a href="LICENSE"><img src="https://img.shields.io/github/license/nerdneilsfield/Beav?style=flat-square" alt="License"></a>
    <a href="README_ZH.md"><img src="https://img.shields.io/badge/lang-%E7%AE%80%E4%BD%93%E4%B8%AD%E6%96%87-blue?style=flat-square" alt="简体中文"></a>
  </p>
</div>

---

Beav scans and reclaims disk space across Linux desktop, developer, GPU/AI, container, and system caches. When a cache belongs to a tool with its own store semantics—such as `uv`, `npm`, `cargo`, `rustup`, Hugging Face, `nix`, Docker, or Podman—Beav invokes the tool's native cleanup interface or an atomic cache cleaner rather than blindly removing directories.

By default, personal directories, credentials, and live tool processes are protected. Every safety boundary (allow-lists, blacklists, age floors, process locks, symlink/mount rules, and custom `type: command` cleaners) can also be configured or bypassed via CLI flags, `config.yaml`, or the TUI—only literal `/` remains unconditionally protected.

## Highlights

- **Tool-aware cleanup**: Native prune support for package managers, runtimes, container engines, and atomic cleanup for Hugging Face and Torch caches.
- **4-tab TUI workbench**: Scan and clean (`[1] Clean`), browse directory sizes with `gdu` integration (`[2] Analyze`), inspect operation logs (`[3] History`), and edit safety rules live (`[4] Safety & Config`).
- **Complete CLI suite**: `scan`, `clean`, `purge`, `analyze`, `doctor`, `history`, `list`, `info`, `tags`, `config`, `version`, `help`, and shell completion.
- **Configurable safety**: Conservative defaults out of the box, with per-flag overrides (`--allow-path`, `--bypass-*`, `--unsafe`) and persistent rules under `safety:` in `config.yaml`.
- **Script-friendly output**: `spinner`, `plain`, `quiet` (`-q`), and JSON Lines (`--output json`) modes with predictable exit codes (`0`-`4`).

## Installation

Install via Homebrew or download a static binary archive from [Releases](https://github.com/nerdneilsfield/Beav/releases):

```bash
brew install --cask nerdneilsfield/apps/beav
```

To verify your installation:

```bash
beav version
beav doctor
```

## Quick start

In an interactive terminal (TTY), running `beav` with no arguments starts the read-only scan and 4-tab TUI workbench:

```bash
beav               # launch 4-tab TUI workbench (user scope)
sudo beav --system # launch TUI for system caches
```

For direct CLI workflows:

```bash
beav scan                     # read-only preview of matching caches
beav clean                    # preview and prompt before deleting in TTY
beav clean --only langs -y    # clean language caches without prompting
beav purge /tmp/build-* -y    # clean arbitrary paths without creating a cleaner YAML
```

Language switches automatically between English and Chinese (`BEAV_LANG`, `LC_ALL`, `LC_MESSAGES`, `LANG`), or can be set per command with `--lang en` or `--lang zh`.

## 4-tab TUI workbench

Switch tabs with `1`-`4` or `[` / `]`, press `?` for full keybindings, and press `q` to quit:

| Tab | Purpose | Key actions |
| --- | --- | --- |
| `[1] Clean` | Interactive cleaner list with disk bar, proportional sizes, and scrollable detail pane | `Space` select, `a` safe all, `A` all incl. heavy, `o` sort, `t` tag filter, `m` age preset, `b` toggle bypass, `Enter`/`c` run |
| `[2] Analyze` | Built-in directory size browser with optional deletion | `Enter`/`Backspace` navigate, `o` sort, `d` delete, `g` launch full `gdu` explorer |
| `[3] History` | Browse and filter recorded deletions from `operations.log` | `/` filter, `r` reload, `X` clear logs |
| `[4] Safety & Config` | Toggle safety policy flags and edit custom allow/deny prefixes live | `Space` toggle, `+` add path, `x` remove path, `w` save `config.yaml`, `R` apply and rescan |

You can also open directly on a specific tab with `beav tui --tab analyze` (or `clean`, `history`, `config`).

## Essential CLI commands

| Command | Aliases | Description |
| --- | --- | --- |
| `beav tui` | — | Launch multi-view TUI workbench (`--tab <name>`) |
| `beav scan` | — | Strictly read-only preview (`--only`, `--skip`, `--min-age`, `--output json`) |
| `beav clean` | — | Run selected cleaners (`--dry-run`, `-y`, `--include-heavy`, `--system`, `--all`) |
| `beav purge <path...>` | — | Clean specified paths or globs directly (`--force-no-age`, `--dry-run`, `-y`) |
| `beav analyze [path]` | — | Launch `gdu` disk explorer (`--allow-delete` / `--unsafe` enables deletion) |
| `beav doctor` | `status`, `check` | Diagnose system environment, disk usage, config state, and tool availability (`--json`) |
| `beav history` | `log`, `logs` | Inspect or clear deletion history (`-n`, `--cleaner`, `--all`, `--clear`, `--json`) |
| `beav list` | `ls` | List cleaners (`--ids`, `--heavy`, `--scope`) |
| `beav info <id>` | — | Show cleaner paths, profile, impact, and notes |
| `beav tags` | — | List tags and cleaner counts (`--json`) |
| `beav config ...` | — | `init`, `show`, `edit`, `path`, `validate`, `reset`, `enable`, `disable`, `whitelist`, `safety` |
| `beav version` | `ver` | Show build metadata, runtime, config dir, and cleaner count (`-s`, `--json`) |
| `beav help [topic]` | — | Grouped command list or topic (`safety`, `exit-codes`, `env`, `examples`) |

See [CLI and TUI Reference](docs/cli-and-tui.md) for all flags, environment variables, JSONL events, and exit codes.

## What Beav cleans

| Scope | Examples |
| --- | --- |
| Editors | VS Code / Insiders / VSCodium, Cursor, Zed, JetBrains IDEs, Neovim caches and logs |
| Browsers | Cache subdirectories for Firefox, Chrome, Chromium, Edge, Brave, Vivaldi (profiles and logins preserved) |
| Language tools | `npm`, `pnpm`, `yarn`, `bun`, `deno`, `node-gyp`, `pip`, `uv`, `Poetry`, `Conda`, `Cargo`, `rustup`, `Go`, `Gradle`, `Maven`, `Coursier` |
| Dev tools | `pre-commit`, `ccache`, `sccache`, `clangd`, `zig`, `Bazel`, `Cypress`, `Playwright`, `mise` |
| AI & GPU | Incomplete Hugging Face downloads, old HF Datasets Arrow caches, Torch Hub weights, `vLLM`, `FlashInfer`, Mesa/NVIDIA shader caches, `Triton`, `TorchInductor` |
| Desktop & Cloud | Trash, thumbnails, font caches, Flatpak caches/unused runtimes, partial downloads, `minikube`, `helm`, `k9s`, `kubectl`, `gcloud`, Azure CLI logs |
| Containers & System | Docker, Podman (rootless and system verified separately), `apt`, `dnf`, `pacman`, `zypper`, `systemd` journals, `/tmp`, `/var/tmp`, `/var/cache`, Nix GC |

## Configurable safety and bypass

Default rules protect personal directories (`Documents`, `Desktop`, `Downloads`, `Pictures`, `Videos`, `Music`, `.ssh`, `.gnupg`, `.password-store`), system directories (`/etc`, `/boot`, `/usr`, `/proc`, `/sys`, `/dev`, `/run`, container runtime storage), mount boundaries, symlink traversal, active processes, `/root` when running as `root`, and age floors on sensitive caches.

You can relax or replace any restriction (except literal `/`) per command or persistently:

> [!WARNING]
> Bypassing safety checks (`--unsafe`, `--bypass-allow-list`, `--bypass-blacklist`, `--bypass-age-floor`, `--follow-symlinks`) removes protections against unintended deletion. Preview targets with `beav scan` or `--dry-run` before running destructive commands.

```bash
# Append a custom allowed root for one run
beav purge /opt/build-cache --allow-path /opt/build-cache --dry-run

# Bypass all configurable checks for one run
beav clean --unsafe --min-age=0d

# Configure persistent safety rules via CLI
beav config safety show
beav config safety set allow_command_cleaners true
beav config safety allow-add /opt/build-cache
```

Configuration lives in `~/.config/beav/config.yaml` and custom cleaners live in `~/.config/beav/cleaners.d/` (both `type: paths` and gated `type: command` are supported). Read [Safety Policy, Bypass Controls, and Configuration](docs/safety-and-config.md) for the complete schema and policy reference.

## Documentation index

- [CLI and TUI Reference](docs/cli-and-tui.md) — subcommands, flags, 4-tab shortcuts, env vars, JSONL output, exit codes
- [Safety and Configuration](docs/safety-and-config.md) — default bounds, bypass flags, `config.yaml` `safety:` schema, custom `paths` and `command` cleaners

## Development

Requires Go 1.24+:

```bash
git clone https://github.com/nerdneilsfield/Beav.git
cd Beav
make build
make test
make race
make lint
```

Design notes from the initial architecture remain in [design spec](docs/superpowers/specs/2026-04-26-beav-design.md) and [implementation plan](docs/superpowers/plans/2026-04-26-beav-implementation.md).

## Acknowledgments and license

Inspired by [Mole](https://github.com/tw93/mole). Disk usage exploration uses [gdu](https://github.com/dundee/gdu), and the terminal UI is built with [Charm](https://charm.sh/).

Licensed under the [MIT License](LICENSE).
