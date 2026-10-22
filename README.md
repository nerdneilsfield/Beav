<div align="center">
  <h1>🦫 Beav</h1>
  <p><strong>Clean Linux caches without guessing what is safe to delete.</strong></p>
  <p>
    <a href="https://github.com/nerdneilsfield/Beav/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/nerdneilsfield/Beav/ci.yml?branch=master&style=flat-square&label=CI&logo=github" alt="CI"></a>
    <a href="https://github.com/nerdneilsfield/Beav/releases"><img src="https://img.shields.io/github/v/release/nerdneilsfield/Beav?style=flat-square&include_prereleases&label=release" alt="Release"></a>
    <img src="https://img.shields.io/badge/Go-1.24%2B-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go 1.24+">
    <img src="https://img.shields.io/badge/platform-Linux-FCC624?style=flat-square&logo=linux&logoColor=black" alt="Linux">
    <a href="LICENSE"><img src="https://img.shields.io/github/license/nerdneilsfield/Beav?style=flat-square" alt="License"></a>
    <a href="README_ZH.md"><img src="https://img.shields.io/badge/lang-中文-red?style=flat-square" alt="中文"></a>
  </p>
</div>

---

Beav is a Linux disk cleaner for caches, stale downloads, logs, build output, and other files that can be recreated. It knows about the tools that produced those files, so it can use Homebrew, uv, Hugging Face, package-manager, and container cleanup commands when a plain directory sweep would be unsafe.

Personal files are outside its job. Browser profiles, credentials, project trees, container volumes, and ordinary files in `Downloads` are not cleanup targets.

## Start with a dry run

Download a Linux archive from [Releases](https://github.com/nerdneilsfield/Beav/releases), extract `beav`, and place it somewhere on your `PATH`. To build from source instead, use Go 1.24 or newer:

```bash
git clone https://github.com/nerdneilsfield/Beav.git
cd Beav
make build
./bin/beav version
```

Before deleting anything on a new machine, inspect the plan:

```bash
beav clean --dry-run
```

If the result looks right, run the same cleanup without `--dry-run`:

```bash
beav clean
```

This cleans the current user's caches. Beav does not invoke `sudo` for you. System cleanup is a separate, explicit command:

```bash
sudo beav clean --system
```

To clean the original user's home and the system in one run:

```bash
sudo beav clean --all
```

Running `beav` with no subcommand opens the terminal menu. `beav analyze [PATH]` starts the bundled disk-usage browser.

## What Beav cleans

The built-in registry covers these areas:

| Area | Examples |
|---|---|
| Editors | VS Code, VS Code Server, JetBrains IDEs, Neovim cache and state |
| Browsers | Firefox, Chrome, Chromium, and Edge cache directories; profiles and login data stay in place |
| Language tools | npm, pnpm, yarn, bun, pip, uv, Poetry, Conda, Corepack, Go, Gradle, Maven |
| Developer tools | pre-commit, ccache, Cypress, Playwright, mise |
| AI and GPU | interrupted Hugging Face downloads, old HF Datasets Arrow units, Torch Hub checkpoints, NVIDIA compute cache, Triton, TorchInductor |
| Desktop | Trash, thumbnail cache, old partial-download files, Flatpak unused data |
| Kubernetes | minikube image cache, Helm, k9s, and kubectl cache |
| Cloud tools | old gcloud and Azure CLI logs |
| Containers | Docker and Podman prune operations, with rootless and system daemons kept separate |
| System | apt, dnf, pacman, zypper, systemd journal, `/tmp`, `/var/tmp`, selected `/var/cache` entries, Nix garbage collection |

The exact list on your installed version is available locally:

```bash
beav config show
```

Some entries are skipped when their tool is missing, the relevant process is running, the files are too recent, or Beav cannot prove that the target is safe. A skip is not treated as a deletion failure.

## Safety boundaries

Cache cleanup is easy until a path comes from an environment variable, a symlink changes during the run, or `sudo` points at the wrong home directory. Beav checks those cases before it removes a file.

- Every path must stay below an approved root such as the target home, `/var/cache`, `/var/log`, `/tmp`, or `/var/tmp`.
- Known credential locations and personal-data directories are blacklisted. This includes `~/.ssh`, `~/.gnupg`, `~/.password-store`, `~/.kube/config`, and `~/.docker/config.json`. Built-in browser cleaners point only at cache subdirectories, not profiles.
- Generic cleaners cannot enter `Documents`, `Desktop`, `Downloads`, `Pictures`, `Videos`, or `Music`. The Downloads cleaner is deliberately narrow: it looks only one level deep and recognizes partial-download suffixes.
- Files are checked individually against an age threshold. Beav does not decide that an entire directory is old because its parent timestamp is old.
- Directory traversal is anchored with `openat` and `O_NOFOLLOW`. Mount boundaries, symlink substitutions, and file identity changes stop the deletion.
- Running applications, open file descriptors, and lock markers can hold a cleaner back. Inspection errors fail closed for processes owned by the target user.
- Heavy cleaners require `--include-heavy`, or an exact cleaner ID or tag in `--only`. `--yes`, prefixes, and `--all` do not grant that permission.

There is no hidden privilege escalation. A non-root `--system` request is refused with a hint; a root login cannot clean `/root` unless `--allow-root-home` is present. Under `sudo --all`, Beav cross-checks `SUDO_UID`, `SUDO_USER`, and the home directory owner before selecting the user scope.

<details>
<summary><strong>How deletion is checked</strong></summary>

For path-based cleaners, Beav opens each component relative to a trusted directory descriptor. It rejects symlinks, checks the filesystem device, protects directories containing `.git`, applies the per-file age rule, and compares `(inode, device, size, mtime)` again immediately before `unlinkat`.

Native cleanup profiles are closed definitions compiled into Beav. YAML may select a known profile, but it cannot inject a command, argument list, environment, or arbitrary root. Managed Hugging Face and Torch cache units use rename-first deletion and a recovery journal so an interrupted run can be recovered without trusting a stale pathname.

Rootless Docker and Podman cleaners also verify the daemon mode, socket owner, and socket location. System cleaners refuse to use a rootless daemon.

</details>

## Useful commands

```bash
# Preview one family of cleaners
beav clean --dry-run --only langs

# Run one exact cleaner
beav clean --only lang-uv

# Keep browser caches out of this run
beav clean --skip browser

# Use a 30-day global threshold
beav clean --min-age=30d

# Override selected tags
beav clean --min-age=langs=14d,logs=30d

# Permit enabled heavy cleaners
beav clean --include-heavy

# Emit JSON Lines for a script
beav clean --dry-run --output json
```

`--only` and `--skip` accept cleaner IDs, ID prefixes, or tags. Heavy entries are the partial exception: an exact full ID or an exact tag in `--only` authorizes one implicitly, while ID prefixes never do. Built-in minimum floors still apply to sensitive profiles, so a command-line age cannot make interrupted downloads or managed caches newer than their safety floor eligible.

The binary prints the complete flag list:

```bash
beav clean --help
```

## Configuration

Create a starter file, inspect the merged result, or open it in your editor:

```bash
beav config init
beav config show
beav config edit
```

The default path is `~/.config/beav/config.yaml`. Command-line flags take precedence over this file, and the file takes precedence over built-in defaults.

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

Extra cleaner files can be placed in `~/.config/beav/cleaners.d/`. A simple path cleaner looks like this:

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

Custom YAML still goes through registry and path-safety validation. `type: command` is rejected. Closed executor types such as `tool_prune`, `debris`, and `managed_cache` accept built-in profile names only.

## Output and automation

Terminal output uses a spinner when stdout is a TTY and plain text when it is not. Choose JSONL explicitly when another program will read the result:

```bash
beav clean --dry-run --output json | jq -c 'select(.event == "finish" or .event == "summary")'
```

A dry run reports candidates in `files_candidate` and `bytes_candidate`; actual deletion fields remain zero. Some native tools do not report exact byte counts. In that case the summary sets `metrics_incomplete` and names the unmeasured cleaners rather than inventing a number.

Exit codes are stable for scripts:

| Code | Meaning |
|---:|---|
| `0` | Selected cleaners completed without error events |
| `1` | Invalid usage or refused privilege request |
| `2` | Configuration, registry, or path validation failed |
| `3` | At least one cleaner emitted an error event |
| `4` | Interrupted by a signal |

## What Beav leaves alone

Beav does not remove old kernels, container volumes, arbitrary files from Downloads, browser history, recent-file lists, or remote caches. It also avoids application data whose cache semantics are not clear enough to encode safely, including Ollama and ModelScope data today.

Nix cleanup calls native garbage collection without deleting generations. Conda cleanup is limited to index caches, tarballs, and logs. Rust toolchain cleanup keeps the default toolchain, the newest member of each release line (stable, beta, nightly, and each `major.minor`), and linked toolchains. There is no general-purpose shell-command cleaner.

These limits are intentional. If a directory may contain the only copy of someone's work, it needs a purpose-built design rather than another glob.

## Contributing

For a new path cleaner, start with a YAML file and include a conservative age. Native-tool or managed-cache support usually needs a closed profile plus tests because its safety rules belong in Go, not user-supplied arguments.

Run the checks before opening a change:

```bash
make test
make race
make lint
```

The [design notes](docs/superpowers/specs/2026-04-26-beav-design.md) document the original safety model. The [implementation plan](docs/superpowers/plans/2026-04-26-beav-implementation.md) records how the first release was built.

## Credits and license

Beav was inspired by [Mole](https://github.com/tw93/mole). Its disk browser is powered by [gdu](https://github.com/dundee/gdu), and the terminal interface uses the [Charm](https://charm.sh/) libraries.

Released under the [MIT License](LICENSE).
