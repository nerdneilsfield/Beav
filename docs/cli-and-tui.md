# CLI and TUI Reference

Complete command reference, keyboard controls, environment variables, and machine-readable output behavior for `beav`.

## Root invocation and global flags

Running `beav` in an interactive terminal (TTY) with no subcommand launches the 4-tab TUI workbench on the `[1] Clean` tab. When stdout is not a TTY, `beav` prints command help instead.

### Global flags (available on all commands)

| Flag | Default | Description |
| --- | --- | --- |
| `--lang <auto\|en\|zh>` | `auto` | Override UI language (`BEAV_LANG`, `LC_ALL`, `LC_MESSAGES`, `LANG` fallback) |
| `-q`, `--quiet` | `false` | Suppress non-essential output (prints summary/errors only) |
| `--no-color` | `false` | Disable ANSI colors in terminal output |

### Root flags

| Flag | Description |
| --- | --- |
| `-V`, `--version` | Print `beav <version> (<commit>, <date>)` and exit |
| `--system` | Launch TUI for system-scope cleaners (`sudo` required) |
| `--all` | Launch TUI for both user and system scopes (`sudo` required) |
| Policy & scope flags | Same policy, scope, and bypass flags accepted by `tui`, `scan`, and `clean` |

## Subcommands

### Core & Cleanup

| Command | Aliases | Purpose |
| --- | --- | --- |
| `beav tui` | — | Launch the 4-tab TUI workbench (`--tab clean\|analyze\|history\|config`) |
| `beav scan` | — | Strictly read-only preview of matching cleaners and candidate sizes |
| `beav clean` | — | Execute cleanup (asks confirmation in TTY unless `-y`/`--yes` is passed) |
| `beav purge <path...>` | — | Clean arbitrary paths or globs directly without a YAML cleaner (`--dry-run`, `-y`/`--yes`, `-v`/`--verbose`, `--output`, `--force-no-age`) |
| `beav analyze [path]` | — | Launch bundled `gdu` disk usage explorer (read-only by default; pass `--allow-delete` or `--unsafe` to allow deletion) |

#### Flags for `scan` and `clean`

| Flag | Applies to | Description |
| --- | --- | --- |
| `--system` | `scan`, `clean`, `tui` | Operate on system-scope cleaners |
| `--all` | `scan`, `clean`, `tui` | Operate on both user and system scopes |
| `--user <name>` | `scan`, `clean`, `tui` | Target user when running with `--all` |
| `--only <selector>` | `scan`, `clean` | Run only matching IDs, ID prefixes, or tags (repeatable) |
| `--skip <selector>` | `scan`, `clean` | Skip matching IDs, ID prefixes, or tags (repeatable) |
| `--include-heavy` | `scan`, `clean`, `tui` | Authorize cleaners with `impact: heavy` |
| `--min-age <rule>` | `scan`, `clean`, `purge`, `tui` | Override retention age (`30d` or `langs=14d,logs=30d`) |
| `--force-no-age` | `scan`, `clean`, `purge`, `tui` | Delete matching targets regardless of modification time |
| `--output <mode>` | `scan`, `clean`, `purge` | Output format: `auto`, `spinner`, `plain`, or `json` |
| `-v`, `--verbose` | `scan`, `clean`, `purge` | Print candidate/deleted file samples and full skip details |
| `--dry-run` | `clean`, `purge` | Preview without deleting |
| `-y`, `--yes` | `clean`, `purge` | Skip confirmation prompt in TTY |
| `--config-dir <dir>` | `scan`, `clean`, `purge`, `tui`, `list`, `info` | Override config directory |

For policy bypass flags (`--unsafe`/`--force`, `--allow-path`, `--bypass-blacklist`, `--bypass-allow-list`, `--bypass-age-floor`, `--ignore-running`, `--follow-symlinks`, `--allow-cross-fs`, `--allow-root-home`), see [Safety and Configuration](safety-and-config.md).

### Discovery & Diagnostics

| Command | Aliases | Flags | Purpose |
| --- | --- | --- | --- |
| `beav list` | `ls` | `--ids`, `--heavy`, `--scope`, `--config-dir` | List registered cleaners, impacts, effective age, and paths |
| `beav info <cleaner-id>` | — | `--config-dir` | Show details, paths, impact, and safety notes for one cleaner |
| `beav tags` | — | `--scope`, `--json`, `--config-dir` | List all available tags and cleaner counts |
| `beav doctor` | `status`, `check` | `--json`, `--config-dir` | Inspect OS/privileges, disk usage, config validity, policy state, and external tools |
| `beav history` | `log`, `logs` | `-n`/`--limit`, `--cleaner`, `--all`, `--json`, `--clear`, `-y`/`--yes` | Inspect or clear `operations.log` deletion history (`--all` includes rotated logs) |

### Configuration & Utilities

| Command | Aliases | Purpose |
| --- | --- | --- |
| `beav config init` | — | Create starter `~/.config/beav/config.yaml` (`--force` to overwrite) |
| `beav config show` | — | Print resolved configuration (`--json` supported) |
| `beav config edit` | — | Open `config.yaml` in `$VISUAL` or `$EDITOR` (falls back to `vi`) |
| `beav config path` | — | Print resolved config dir, `config.yaml`, `cleaners.d`, `whitelist.txt`, and log path |
| `beav config validate` | — | Validate `config.yaml`, whitelist, and custom cleaners in `cleaners.d` |
| `beav config reset` | — | Reset `config.yaml` to starter defaults (`-y`/`--yes` skips confirmation) |
| `beav config enable <id>` | — | Persist `enabled: true` override for `<id>` |
| `beav config disable <id>` | — | Persist `enabled: false` override for `<id>` |
| `beav config whitelist [add\|remove <path>]` | — | List, add, or remove protected whitelist prefixes |
| `beav config safety ...` | — | View or edit safety policy switches and allow/deny lists |
| `beav version` | `ver` | Show version, commit, build date, Go version, OS/arch/distro, config dir, cleaner count (`-s`/`--short`, `--json`) |
| `beav help [command\|topic]` | — | Show grouped help or a topic (`safety`, `exit-codes`, `env`, `examples`) |
| `beav completion [bash\|zsh\|fish]` | — | Generate shell completion scripts |

## TUI Workbench

Switch between tabs with `1`-`4`, `[`, or `]`. Press `q` or `Ctrl+C` to quit from any view, and `?` to open the shortcut help overlay.

### `[1] Clean`

- `↑` / `↓`, `j` / `k`, `PgUp` / `PgDn`, `g` / `G`: Navigate cleaner list
- `Space`: Toggle selection on current cleaner
- `a`: Select or deselect all non-heavy candidates
- `A`: Select all candidates including heavy cleaners
- `o`: Cycle sort order (`size desc` -> `name` -> `files desc` -> `impact`)
- `t`: Cycle tag filter
- `m`: Cycle minimum-age preset (`default`, `0d`, `3d`, `7d`, `14d`, `30d`) and rescan
- `b`: Toggle `bypass_all` live and rescan
- `s`: Show or hide skipped/zero-byte cleaners
- `/`: Search cleaners by ID, name, or tag
- `Tab` / `J` / `K`: Focus or scroll the right-side detail pane
- `r`: Trigger a fresh read-only scan
- `Enter` / `c`: Open confirmation dialog to run selected cleanups

### `[2] Analyze`

- Built-in directory size browser sorted by size, file count, or name (`o`)
- `Enter`: Descend into directory; `Backspace` / `h`: Go to parent directory
- `r`: Refresh current directory scan
- `d`: Delete selected file or directory (prompts for confirmation)
- `g`: Suspend TUI and launch full `gdu` explorer on the current path

### `[3] History`

- `/`: Filter entries by cleaner ID, scope, or path
- `r`: Reload entries from `operations.log`
- `X`: Clear operation logs (with confirmation)

### `[4] Safety & Config`

- `↑` / `↓` or `j` / `k`: Move across boolean policy flags, allow-list entries, and deny-list entries
- `Space`: Toggle selected boolean safety flag
- `+`: Add a path to allow-list or deny-list
- `x` / `Delete`: Remove selected custom path entry
- `w`: Save edited policy to `config.yaml`
- `R`: Apply current policy and rescan cleaners

## Environment variables

| Variable | Effect |
| --- | --- |
| `BEAV_LANG` | UI language override (`auto`, `en`, `zh`) |
| `BEAV_CONFIG_DIR` | Override `~/.config/beav` configuration directory |
| `BEAV_UNSAFE` | Set to `1` or `true` to enable full bypass mode by default |
| `BEAV_NO_OPLOG` | Any non-empty value disables writing `operations.log` |
| `NO_COLOR` | Any non-empty value disables ANSI colors |
| `VISUAL`, `EDITOR` | Editor launched by `beav config edit` |

## Machine-readable output and exit codes

When `--output json` is passed to `scan`, `clean`, or `purge`, Beav streams JSON Lines (JSONL) events (`start`, `result`, `skip`, `error`, `summary`). In `scan` or `--dry-run`, matched counts and sizes appear in `files_candidate` and `bytes_candidate` while deleted counters stay `0`. When an external tool does not report deleted byte counts accurately, `summary` sets `metrics_incomplete: true` and lists unmeasured cleaners.

| Exit code | Meaning |
| ---: | --- |
| `0` | Selected cleaners completed with zero error events |
| `1` | Invalid flags/arguments, unknown selector, TTY missing for TUI/analyze, or unprivileged `--system` request |
| `2` | Invalid configuration, registry YAML, or path validation failure |
| `3` | One or more cleaners reported an execution error |
| `4` | Interrupted by `SIGINT` or `SIGTERM` |
