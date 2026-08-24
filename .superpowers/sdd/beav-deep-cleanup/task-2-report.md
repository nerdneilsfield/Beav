# Task 2 report

## TDD log

### RED 1 — secure command runner configuration

Command:

```text
rtk go test ./internal/cleaner/executor -run '^TestRunCommandUsesAbsoluteExecutableAndExplicitEnvironment$' -count=1
```

Expected failure: `CommandRun` has no `Executable`, `Args`, `Env`, or `Cwd` fields.

Observed: failed to compile with all four expected unknown-field errors.

### GREEN 1

Command:

```text
rtk go test ./internal/cleaner/executor -run '^TestRunCommandUsesAbsoluteExecutableAndExplicitEnvironment$' -count=1
```

Observed: `1 passed`; runner now uses absolute executable, explicit environment, `/dev/null` stdin, and target cwd.

### RED 2 — closed tool profiles and dry-run policy

Command:

```text
rtk go test ./internal/cleaner/executor -run '^TestToolPrune' -count=1
```

Expected failure: `NewToolPruneExecutor` and `WithToolSearchPath` are undefined.

Observed: failed to compile with those expected undefined-symbol errors.

### RED 3 — keep-list intersection blocks external command

Command:

```text
rtk go test ./internal/cleaner/executor -run '^TestToolPruneBlocksWholeCommandWhenKeepListIntersectsRoot$' -count=1
```

Expected/observed failure: package cannot compile because the profile executor API is not implemented yet.

### GREEN 2/3

Command:

```text
rtk go test ./internal/cleaner/executor -run '^TestToolPrune' -count=1
```

Observed: `3 passed`; Homebrew uses one resolved absolute binary with native dry-run, unsupported dry-run never invokes uv, and keep-list intersection blocks the command before probing/execution.

### Additional focused guards

`rtk go test ./internal/cleaner/executor -run '^TestRunCommandKillsCompleteProcessGroupOnTimeout$' -count=1` passed.

`rtk go test ./internal/cleaner/executor -run '^TestToolPrune' -count=1` passed 5 tests, covering version-mismatch skip and post-probe command-failure propagation.

### Verification

- `rtk go test ./...` — 115 passed.
- `rtk go test -race ./...` — 115 passed.
- `rtk go vet ./...` — passed.
- `rtk git diff --check` — passed.

Post-commit self-review tightened root `SUDO_UID` home matching; `rtk go test ./...` remained green.

### RED 4 — expose exact target identity resolution

Command:

```text
rtk go test ./internal/cleaner/executor -run '^TestResolveToolIdentityReturnsExactSupplementaryGroups$' -count=1
```

Observed: expected compile failure, `ResolveToolIdentity` undefined.

### GREEN 4

Command:

```text
rtk go test ./internal/cleaner/executor -run '^TestResolveToolIdentityReturnsExactSupplementaryGroups$' -count=1
```

Observed: `1 passed`; resolver returns target UID/name/home, primary GID, and supplementary groups with primary GID excluded.
