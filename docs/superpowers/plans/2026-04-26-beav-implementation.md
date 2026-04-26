# Beav Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a single static Go binary `beav` that runs `beav clean` (age-aware Linux cache cleanup, user + system scope) and `beav analyze` (gdu-backed TUI), per `docs/superpowers/specs/2026-04-26-beav-design.md`.

**Architecture:** YAML registry (built-in via `//go:embed`, mergeable from `~/.config/beav/cleaners.d/`) drives a small set of executors (`paths`, `command`, `journal_vacuum`, `pkg_cache`, `container_prune`). All deletions go through layered safety gates: load-time path allow/deny, runtime `openat`-based descent with per-file age check, TOCTOU re-stat before unlink. Output is rendered through a `Renderer` interface (plain / spinner / JSONL).

**Tech Stack:** Go 1.24, `cobra`, `yaml.v3`, `bubbletea` + `lipgloss` + `bubbles`, `gdu/v5`, `golang.org/x/sys/unix`, `mattn/go-isatty`, `dustin/go-humanize`, stdlib `log/slog`. No viper, no third-party process libs.

---

## File Structure

```
cmd/beav/main.go                    cobra root + subcommand wiring
internal/cli/
  clean.go                          beav clean
  analyze.go                        beav analyze
  config.go                         beav config show|edit
  completion.go                     beav completion
  version.go                        beav version
  flags.go                          shared flag definitions + parsing helpers
internal/cleaner/
  model/
    cleaner.go                      Cleaner / ExecutorType / AgeFilter / Scope structs
    events.go                       Event types (Start, Deleted, Skipped, ClnSkipped, Error, Finish, Summary, CommandOutput)
  registry/
    loader.go                       embed + ~/.config/beav/cleaners.d/ → []Cleaner
    merge.go                        merge by ID
    validate.go                     enum/required-field checks
  resolver/
    resolver.go                     closed-enum path resolvers + fallbacks
  safety/
    blacklist.go                    hard-deny prefix list (load-time)
    bounds.go                       allow-list boundary check (load-time)
    fs.go                           openat walk, lstat, cross-fs guard, TOCTOU re-stat
    age.go                          recursive age filter + bottom-up empty-dir removal
    procs.go                        /proc scan for running_processes
    whitelist.go                    merged user whitelist prefixes
  executor/
    paths.go                        paths-type executor (uses safety/* + resolver)
    command.go                      shared command runner (timeout, stdout/stderr capture)
    journal.go                      journalctl --vacuum-time
    pkgcache.go                     apt/dnf/pacman/zypper
    container.go                    docker/podman per-target argv + rootless verify
  engine/
    engine.go                       orchestrates registry → safety → executor → renderer
internal/ui/
  renderer.go                       Renderer interface
  cli/
    plain.go                        non-TTY output
    spinner.go                      TTY spinner output (lipgloss)
  json/
    json.go                         JSONL renderer (stdout)
  tui/
    menu.go                         bubbletea main menu (no-args)
    analyze.go                      bubbletea wrapper around gdu lib
internal/config/
  config.go                         ~/.config/beav/config.yaml loader
internal/sysinfo/
  user.go                           SUDO_UID/SUDO_USER triple-check, --user flag resolution
  distro.go                         /etc/os-release parsing + pkg-mgr detection
  tty.go                            isatty wrapper
internal/oplog/
  oplog.go                          rolling log writer (10MB×5)
cleaners/                           //go:embed
  user/{vscode,jetbrains,browsers,desktop,shells}.yaml
  user/langs/{npm,pnpm,yarn,bun,pip,cargo,go,gradle,maven}.yaml
  user/k8s/{minikube,helm,k9s,kubectl}.yaml
  user/containers/{docker-rootless,podman-rootless}.yaml
  system/{apt,dnf,pacman,zypper,journal,varcache,tmp}.yaml
  system/containers/{docker,podman}.yaml
testdata/
  fakehome/                         synthetic dirty $HOME for integration tests
  cleaners/                         malformed/edge YAML for loader tests
go.mod
Makefile
.golangci.yml
.github/workflows/ci.yml
README.md
```

---

## Phase 0: Project Skeleton

### Task 1: Initialize repo, cobra root, version

**Files:**
- Create: `go.mod`, `cmd/beav/main.go`, `internal/cli/version.go`, `Makefile`, `.golangci.yml`

- [ ] **Step 1: Write the failing test for version output**

Create `internal/cli/version_test.go`:
```go
package cli

import (
	"bytes"
	"testing"
)

func TestVersionPrintsVersion(t *testing.T) {
	var out bytes.Buffer
	cmd := NewVersionCmd("0.1.0", "abcdef0", "2026-04-26")
	cmd.SetOut(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	want := "beav 0.1.0 (abcdef0, 2026-04-26)\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/...`
Expected: FAIL — package does not compile (NewVersionCmd undefined).

- [ ] **Step 3: Initialize module and write minimal implementation**

```bash
go mod init github.com/dengqi/beav
go get github.com/spf13/cobra@latest
```

`cmd/beav/main.go`:
```go
package main

import (
	"os"

	"github.com/dengqi/beav/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	root := cli.NewRootCmd(version, commit, date)
	if err := root.Execute(); err != nil {
		os.Exit(1)
	}
}
```

`internal/cli/root.go`:
```go
package cli

import "github.com/spf13/cobra"

func NewRootCmd(version, commit, date string) *cobra.Command {
	root := &cobra.Command{
		Use:   "beav",
		Short: "Linux cache cleaner",
	}
	root.AddCommand(NewVersionCmd(version, commit, date))
	return root
}
```

`internal/cli/version.go`:
```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewVersionCmd(version, commit, date string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print beav version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintf(cmd.OutOrStdout(), "beav %s (%s, %s)\n", version, commit, date)
			return err
		},
	}
}
```

- [ ] **Step 4: Run test, build, smoke**

Run:
```bash
go test ./internal/cli/...
go build -ldflags "-X main.version=0.1.0 -X main.commit=abcdef0 -X main.date=2026-04-26" -o bin/beav ./cmd/beav
./bin/beav version
```

Expected: PASS; binary prints `beav 0.1.0 (abcdef0, 2026-04-26)`.

- [ ] **Step 5: Add Makefile and golangci-lint config, commit**

`Makefile`:
```make
GO ?= go
VERSION ?= dev
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    := $(shell date -u +%Y-%m-%d)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build test lint
build:
	$(GO) build -ldflags "$(LDFLAGS)" -o bin/beav ./cmd/beav
test:
	$(GO) test ./...
lint:
	golangci-lint run ./...
```

`.golangci.yml`:
```yaml
run:
  go: "1.24"
  timeout: 5m
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - revive
    - gocritic
    - gosec
```

```bash
git add go.mod go.sum cmd internal Makefile .golangci.yml
git commit -m "feat: project skeleton with cobra root and version subcommand"
```

---

## Phase 1: Config and Registry

### Task 2: Cleaner model types

**Files:**
- Create: `internal/cleaner/model/cleaner.go`, `internal/cleaner/model/cleaner_test.go`, `internal/cleaner/model/events.go`

- [ ] **Step 1: Write failing tests for type defaults and validation**

`internal/cleaner/model/cleaner_test.go`:
```go
package model

import "testing"

func TestExecutorTypeKnown(t *testing.T) {
	cases := []struct {
		s    string
		want ExecutorType
		ok   bool
	}{
		{"paths", TypePaths, true},
		{"command", TypeCommand, true},
		{"journal_vacuum", TypeJournalVacuum, true},
		{"pkg_cache", TypePkgCache, true},
		{"container_prune", TypeContainerPrune, true},
		{"bogus", "", false},
	}
	for _, c := range cases {
		got, ok := ParseExecutorType(c.s)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseExecutorType(%q) = (%v,%v); want (%v,%v)", c.s, got, ok, c.want, c.ok)
		}
	}
}

func TestScopeKnown(t *testing.T) {
	if _, ok := ParseScope("user"); !ok {
		t.Error("user should parse")
	}
	if _, ok := ParseScope("planet"); ok {
		t.Error("planet should not parse")
	}
}
```

- [ ] **Step 2: Run test, expect FAIL**

Run: `go test ./internal/cleaner/model/...`
Expected: FAIL — undefined identifiers.

- [ ] **Step 3: Implement model**

`internal/cleaner/model/cleaner.go`:
```go
package model

type ExecutorType string

const (
	TypePaths          ExecutorType = "paths"
	TypeCommand        ExecutorType = "command"
	TypeJournalVacuum  ExecutorType = "journal_vacuum"
	TypePkgCache       ExecutorType = "pkg_cache"
	TypeContainerPrune ExecutorType = "container_prune"
)

func ParseExecutorType(s string) (ExecutorType, bool) {
	switch ExecutorType(s) {
	case TypePaths, TypeCommand, TypeJournalVacuum, TypePkgCache, TypeContainerPrune:
		return ExecutorType(s), true
	}
	return "", false
}

type Scope string

const (
	ScopeUser   Scope = "user"
	ScopeSystem Scope = "system"
)

func ParseScope(s string) (Scope, bool) {
	switch Scope(s) {
	case ScopeUser, ScopeSystem:
		return Scope(s), true
	}
	return "", false
}

type TimeField string

const (
	TimeMtime TimeField = "mtime"
	TimeCtime TimeField = "ctime"
)

type PathResolverRef struct {
	Resolver string   `yaml:"resolver"`
	Subpaths []string `yaml:"subpaths"`
}

type PkgCacheCfg struct {
	Manager string `yaml:"manager"` // apt|dnf|pacman|zypper
}

type ContainerPruneCfg struct {
	Runtime string `yaml:"runtime"` // docker|podman
	Target  string `yaml:"target"`  // builder|image|container|network|system
}

type Cleaner struct {
	ID               string             `yaml:"id"`
	Name             string             `yaml:"name"`
	Description      string             `yaml:"description"`
	Scope            Scope              `yaml:"scope"`
	Type             ExecutorType       `yaml:"type"`
	Enabled          *bool              `yaml:"enabled"` // pointer so missing means default-true
	MinAgeDays       *int               `yaml:"min_age_days"`
	// NoAgeFilter declares "this cleaner has no age filter" explicitly.
	// Distinct from "min_age_days field omitted" (which means: use the default).
	// Only such cleaners require --force-no-age to run.
	NoAgeFilter      bool               `yaml:"no_age_filter"`
	TimeField        TimeField          `yaml:"time_field"`
	Paths            []string           `yaml:"paths"`
	PathResolvers    []PathResolverRef  `yaml:"path_resolvers"`
	Exclude          []string           `yaml:"exclude"`
	RunningProcesses []string           `yaml:"running_processes"`
	NeedsRoot        bool               `yaml:"needs_root"`
	Tags             []string           `yaml:"tags"`
	PkgCache         *PkgCacheCfg       `yaml:"pkg_cache"`
	ContainerPrune   *ContainerPruneCfg `yaml:"container_prune"`
}

func (c Cleaner) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

func (c Cleaner) AgeOrDefault(def int) int {
	if c.MinAgeDays == nil {
		return def
	}
	return *c.MinAgeDays
}
```

`internal/cleaner/model/events.go`:
```go
package model

import "time"

type EventType string

const (
	EvStart          EventType = "start"
	EvDeleted        EventType = "deleted"
	EvSkipped        EventType = "skipped"
	EvCleanerSkipped EventType = "cleaner_skipped"
	EvCommandOutput  EventType = "command_output"
	EvError          EventType = "error"
	EvFinish         EventType = "finish"
	EvSummary        EventType = "summary"
)

type Event struct {
	Event       EventType `json:"event"`
	CleanerID   string    `json:"cleaner_id,omitempty"`
	Name        string    `json:"name,omitempty"`
	Scope       Scope     `json:"scope,omitempty"`
	Type        ExecutorType `json:"type,omitempty"`
	DryRun      bool      `json:"dry_run,omitempty"`
	Path        string    `json:"path,omitempty"`
	Size        int64     `json:"size,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Detail      string    `json:"detail,omitempty"`
	Stream      string    `json:"stream,omitempty"`
	Line        string    `json:"line,omitempty"`
	Status      string    `json:"status,omitempty"`
	FilesDeleted int64    `json:"files_deleted,omitempty"`
	BytesFreed   int64    `json:"bytes_freed,omitempty"`
	Errors       int      `json:"errors,omitempty"`
	DurationMs   int64    `json:"duration_ms,omitempty"`
	CleanersRun     int   `json:"cleaners_run,omitempty"`
	CleanersSkipped int   `json:"cleaners_skipped,omitempty"`
	CleanersErrored int   `json:"cleaners_errored,omitempty"`
	TS           time.Time `json:"ts"`
}
```

- [ ] **Step 4: Tests pass, commit**

Run: `go test ./internal/cleaner/model/...`
Expected: PASS.
```bash
git add internal/cleaner/model
git commit -m "feat(model): cleaner schema and event types"
```

---

### Task 3: Registry loader (embed + filesystem) with merge-by-id

**Files:**
- Create: `internal/cleaner/registry/loader.go`, `internal/cleaner/registry/merge.go`, `internal/cleaner/registry/validate.go`, `internal/cleaner/registry/loader_test.go`
- Create: `cleaners/builtin.go` (embed root)

- [ ] **Step 1: Write failing tests for embed-only load and merge override**

`internal/cleaner/registry/loader_test.go`:
```go
package registry

import (
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestLoadBuiltinValid(t *testing.T) {
	fs := fstest.MapFS{
		"a.yaml": &fstest.MapFile{Data: []byte(`
id: editor-vscode
name: VS Code Cache
scope: user
type: paths
min_age_days: 7
paths: ["~/.config/Code/Cache/*"]
`)},
	}
	cs, err := loadFS(fs, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Cleaner.ID != "editor-vscode" {
		t.Fatalf("got %+v", cs)
	}
}

func TestMergeUserOverridesEnabledAndAge(t *testing.T) {
	builtin := []Loaded{{From: "builtin", Cleaner: mustParse(t, `
id: editor-vscode
name: vscode
scope: user
type: paths
min_age_days: 7
paths: ["~/.config/Code/Cache/*"]
`)}}
	user := []Loaded{{From: "user", Cleaner: mustParse(t, `
id: editor-vscode
enabled: false
min_age_days: 3
paths: ["~/.cache/extra/*"]
`)}}
	merged := MergeByID(builtin, user)
	if len(merged) != 1 {
		t.Fatalf("got %d", len(merged))
	}
	c := merged[0]
	if c.IsEnabled() {
		t.Error("user disabled should override")
	}
	if c.AgeOrDefault(99) != 3 {
		t.Error("user age should override")
	}
	if len(c.Paths) != 2 {
		t.Errorf("paths should append; got %v", c.Paths)
	}
}

func TestLoaderAcceptsListShape(t *testing.T) {
	fs := fstest.MapFS{"x.yaml": &fstest.MapFile{Data: []byte(`
- id: a
  scope: user
  type: paths
  paths: [~/.cache/a]
- id: b
  scope: user
  type: paths
  paths: [~/.cache/b]
`)}}
	cs, err := loadFS(fs, ".")
	if err != nil { t.Fatal(err) }
	if len(cs) != 2 { t.Fatalf("got %d", len(cs)) }
}

func TestLoaderTreatsEmptyAsNoOp(t *testing.T) {
	fs := fstest.MapFS{
		"empty.yaml": &fstest.MapFile{Data: []byte("[]\n")},
		"blank.yaml": &fstest.MapFile{Data: []byte("\n\n")},
	}
	cs, err := loadFS(fs, ".")
	if err != nil { t.Fatal(err) }
	if len(cs) != 0 { t.Fatalf("expected 0; got %d", len(cs)) }
}

func TestRejectsImmutableTypeOverride(t *testing.T) {
	builtin := []Loaded{{From: "builtin", Cleaner: mustParse(t, `
id: editor-vscode
scope: user
type: paths
`)}}
	user := []Loaded{{From: "user", Cleaner: mustParse(t, `
id: editor-vscode
type: command
`)}}
	_, err := mergeByIDStrict(builtin, user)
	if err == nil {
		t.Fatal("expected error for type override")
	}
}

// mustParse is a small helper in registry_helpers_test.go using yaml.v3.
var _ = filepath.Join // silence
```

`internal/cleaner/registry/registry_helpers_test.go`:
```go
package registry

import (
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
	"gopkg.in/yaml.v3"
)

func mustParse(t *testing.T, src string) model.Cleaner {
	t.Helper()
	var c model.Cleaner
	if err := yaml.Unmarshal([]byte(src), &c); err != nil {
		t.Fatal(err)
	}
	return c
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/cleaner/registry/...`
Expected: FAIL — undefined `loadFS`, `Loaded`, `MergeByID`, `mergeByIDStrict`.

- [ ] **Step 3: Implement loader, merge, validate**

```bash
go get gopkg.in/yaml.v3
```

**Placeholder YAML to satisfy `//go:embed`** — without at least one matching file, the embed declaration fails at compile time. Create `cleaners/user/_placeholder.yaml`:
```yaml
# Intentionally empty; ensures //go:embed always matches at least one file.
# Real cleaners are added in Tasks 25–26. The loader treats empty/`[]` files as no-ops.
[]
```

`cleaners/builtin.go`:
```go
package cleaners

import "embed"

// Initial embed pattern only matches the placeholder. Tasks 25–26 extend
// this declaration to cover user/, user/langs/, user/k8s/, user/containers/,
// system/, and system/containers/ as those directories gain real YAML files.
//go:embed user/*.yaml
var Builtin embed.FS
```

`internal/cleaner/registry/loader.go`:
```go
package registry

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"gopkg.in/yaml.v3"
)

type Loaded struct {
	From    string // "builtin" or absolute path
	Cleaner model.Cleaner
}

func LoadBuiltin(root fs.FS) ([]Loaded, error) {
	return loadFS(root, ".")
}

func LoadUserDir(dir string) ([]Loaded, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return loadFS(os.DirFS(dir), ".")
}

func loadFS(root fs.FS, base string) ([]Loaded, error) {
	var out []Loaded
	err := fs.WalkDir(root, base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".yaml") {
			return nil
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		// Try list-of-cleaners first; fall back to single Cleaner; empty file → skip.
		if len(strings.TrimSpace(string(data))) == 0 {
			return nil
		}
		var list []model.Cleaner
		if err := yaml.Unmarshal(data, &list); err == nil && len(list) > 0 {
			for _, c := range list {
				out = append(out, Loaded{From: p, Cleaner: c})
			}
			return nil
		}
		var c model.Cleaner
		if err := yaml.Unmarshal(data, &c); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if c.ID == "" {
			// `[]` parses as empty Cleaner; treat as no-op file.
			return nil
		}
		out = append(out, Loaded{From: p, Cleaner: c})
		return nil
	})
	if err != nil {
		return nil, err
	}
	_ = filepath.Separator
	return out, nil
}
```

`internal/cleaner/registry/merge.go`:
```go
package registry

import (
	"fmt"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// MergeByID merges builtin and user cleaners. User entries with the same ID
// override mutable fields and append to paths/exclude/path_resolvers/tags.
// Structural fields (type, scope, pkg_cache, container_prune) cannot be overridden.
func MergeByID(builtin, user []Loaded) []model.Cleaner {
	out, _ := mergeByIDStrict(builtin, user)
	return out
}

func mergeByIDStrict(builtin, user []Loaded) ([]model.Cleaner, error) {
	byID := map[string]model.Cleaner{}
	order := []string{}
	for _, l := range builtin {
		if _, ok := byID[l.Cleaner.ID]; !ok {
			order = append(order, l.Cleaner.ID)
		}
		byID[l.Cleaner.ID] = l.Cleaner
	}
	for _, l := range user {
		base, ok := byID[l.Cleaner.ID]
		if !ok {
			byID[l.Cleaner.ID] = l.Cleaner
			order = append(order, l.Cleaner.ID)
			continue
		}
		if l.Cleaner.Type != "" && l.Cleaner.Type != base.Type {
			return nil, fmt.Errorf("cleaner %q: cannot override type", l.Cleaner.ID)
		}
		if l.Cleaner.Scope != "" && l.Cleaner.Scope != base.Scope {
			return nil, fmt.Errorf("cleaner %q: cannot override scope", l.Cleaner.ID)
		}
		if l.Cleaner.Enabled != nil {
			base.Enabled = l.Cleaner.Enabled
		}
		if l.Cleaner.MinAgeDays != nil {
			base.MinAgeDays = l.Cleaner.MinAgeDays
		}
		if l.Cleaner.TimeField != "" {
			base.TimeField = l.Cleaner.TimeField
		}
		base.Paths = append(base.Paths, l.Cleaner.Paths...)
		base.Exclude = append(base.Exclude, l.Cleaner.Exclude...)
		base.PathResolvers = append(base.PathResolvers, l.Cleaner.PathResolvers...)
		base.Tags = append(base.Tags, l.Cleaner.Tags...)
		byID[l.Cleaner.ID] = base
	}
	out := make([]model.Cleaner, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out, nil
}
```

`internal/cleaner/registry/validate.go`:
```go
package registry

import (
	"fmt"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// Validate performs schema-level checks (enum, required fields).
// Path-safety validation lives in registry.ValidatePaths and is added once
// the safety + resolver packages exist (see Task 11.5).
func Validate(c model.Cleaner) error {
	if c.ID == "" {
		return fmt.Errorf("cleaner missing id")
	}
	if _, ok := model.ParseScope(string(c.Scope)); !ok {
		return fmt.Errorf("cleaner %q: invalid scope %q", c.ID, c.Scope)
	}
	if _, ok := model.ParseExecutorType(string(c.Type)); !ok {
		return fmt.Errorf("cleaner %q: invalid type %q", c.ID, c.Type)
	}
	switch c.Type {
	case model.TypePaths:
		if len(c.Paths) == 0 && len(c.PathResolvers) == 0 {
			return fmt.Errorf("cleaner %q: paths type requires paths or path_resolvers", c.ID)
		}
	case model.TypePkgCache:
		if c.PkgCache == nil || c.PkgCache.Manager == "" {
			return fmt.Errorf("cleaner %q: pkg_cache type requires pkg_cache.manager", c.ID)
		}
	case model.TypeContainerPrune:
		if c.ContainerPrune == nil {
			return fmt.Errorf("cleaner %q: container_prune type requires container_prune block", c.ID)
		}
		if c.MinAgeDays == nil {
			return fmt.Errorf("cleaner %q: container_prune requires min_age_days", c.ID)
		}
	}
	return nil
}

```

`ValidatePaths` is added in Task 11.5 once the safety + resolver packages exist.

- [ ] **Step 4: Tests pass, commit**

Run: `go test ./internal/cleaner/registry/...`
Expected: PASS.
```bash
git add internal/cleaner/registry cleaners/builtin.go go.sum
git commit -m "feat(registry): YAML loader, merge-by-id, schema validation"
```

---

### Task 4: User config loader

**Files:**
- Create: `internal/config/config.go`, `internal/config/config_test.go`

- [ ] **Step 1: Write failing test**

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigOverridesAndWhitelist(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(`
defaults:
  min_age_days: 21
  output: spinner
overrides:
  editor-vscode:
    min_age_days: 3
    enabled: true
whitelist:
  - ~/.cache/keepme
`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "whitelist.txt"), []byte("/tmp/keepme\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Defaults.MinAgeDays != 21 {
		t.Errorf("min_age_days got %d", cfg.Defaults.MinAgeDays)
	}
	ovr := cfg.Overrides["editor-vscode"]
	if ovr.MinAgeDays == nil || *ovr.MinAgeDays != 3 {
		t.Errorf("vscode override age missing: %+v", ovr)
	}
	wl := cfg.MergedWhitelist()
	want := []string{"/tmp/keepme", os.ExpandEnv("$HOME/.cache/keepme")}
	if len(wl) != 2 || wl[0] != want[0] || wl[1] != want[1] {
		t.Errorf("whitelist got %v want %v", wl, want)
	}
}
```

- [ ] **Step 2: Run, FAIL**

Run: `go test ./internal/config/...`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Defaults struct {
	MinAgeDays int    `yaml:"min_age_days"`
	Output     string `yaml:"output"`
}

type Override struct {
	Enabled    *bool `yaml:"enabled"`
	MinAgeDays *int  `yaml:"min_age_days"`
}

type Config struct {
	Dir       string
	Defaults  Defaults             `yaml:"defaults"`
	Overrides map[string]Override  `yaml:"overrides"`
	Whitelist []string             `yaml:"whitelist"`
	whitelistTxt []string
}

func Load(dir string) (*Config, error) {
	cfg := &Config{Dir: dir, Defaults: Defaults{MinAgeDays: 14, Output: "spinner"}}
	if data, err := os.ReadFile(filepath.Join(dir, "config.yaml")); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if data, err := os.ReadFile(filepath.Join(dir, "whitelist.txt")); err == nil {
		s := bufio.NewScanner(strings.NewReader(string(data)))
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			cfg.whitelistTxt = append(cfg.whitelistTxt, expand(line))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for i, p := range cfg.Whitelist {
		cfg.Whitelist[i] = expand(p)
	}
	return cfg, nil
}

func (c *Config) MergedWhitelist() []string {
	out := make([]string, 0, len(c.Whitelist)+len(c.whitelistTxt))
	out = append(out, c.whitelistTxt...)
	out = append(out, c.Whitelist...)
	return out
}

func expand(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return os.ExpandEnv(p)
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/config
git commit -m "feat(config): user config and whitelist loader"
```

---

## Phase 2: Safety Layers

### Task 5: Load-time gates — boundaries and blacklist

**Files:**
- Create: `internal/cleaner/safety/bounds.go`, `internal/cleaner/safety/blacklist.go`, `internal/cleaner/safety/safety_test.go`

- [ ] **Step 1: Failing tests**

```go
package safety

import "testing"

func TestInsideAllowList(t *testing.T) {
	cases := []struct{ p string; ok bool }{
		{"/home/u/.cache/x", true},
		{"/var/cache/apt", true},
		{"/var/log/journal", true},
		{"/tmp/foo", true},
		{"/var/tmp/foo", true},
		{"/etc/passwd", false},
		{"/usr/bin/x", false},
	}
	for _, c := range cases {
		if got := InsideAllowList(c.p, "/home/u"); got != c.ok {
			t.Errorf("InsideAllowList(%q) = %v want %v", c.p, got, c.ok)
		}
	}
}

func TestBlacklisted(t *testing.T) {
	home := "/home/u"
	for _, p := range []string{
		"/", "/etc", "/boot", "/usr", "/usr/lib",
		"/home/other/.cache",
		"/home/u",
		"/home/u/Documents", "/home/u/.ssh", "/home/u/.gnupg",
		"/home/u/.docker/config.json", "/home/u/.kube/config",
		"/var/lib/docker", "/var/lib/containerd", "/var/lib/kubelet",
	} {
		if !Blacklisted(p, home) {
			t.Errorf("expected %q blacklisted", p)
		}
	}
	for _, p := range []string{
		"/home/u/.cache/x",
		"/home/u/.kube/cache/x",
		"/home/u/.kube/http-cache/y",
	} {
		if Blacklisted(p, home) {
			t.Errorf("expected %q not blacklisted", p)
		}
	}
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

`internal/cleaner/safety/bounds.go`:
```go
package safety

import (
	"path/filepath"
	"strings"
)

var systemRoots = []string{"/var/cache", "/var/log", "/tmp", "/var/tmp"}

func InsideAllowList(path, home string) bool {
	clean := filepath.Clean(path)
	if home != "" {
		if hasPrefix(clean, filepath.Clean(home)) {
			return true
		}
	}
	for _, root := range systemRoots {
		if hasPrefix(clean, root) {
			return true
		}
	}
	return false
}

func hasPrefix(p, prefix string) bool {
	if p == prefix {
		return true
	}
	return strings.HasPrefix(p, prefix+string(filepath.Separator))
}
```

`internal/cleaner/safety/blacklist.go`:
```go
package safety

import (
	"path/filepath"
	"strings"
)

var hardDenyExact = []string{"/", "/etc", "/boot", "/usr"}
var hardDenyPrefix = []string{
	"/etc/", "/boot/", "/usr/",
	"/var/lib/docker", "/var/lib/containerd", "/var/lib/kubelet",
}

var homeBlacklistRel = []string{
	"Documents", "Desktop", "Downloads", "Pictures", "Videos", "Music",
	".ssh", ".gnupg", ".password-store",
}

func Blacklisted(path, home string) bool {
	clean := filepath.Clean(path)

	for _, d := range hardDenyExact {
		if clean == d {
			return true
		}
	}
	for _, p := range hardDenyPrefix {
		if clean == strings.TrimSuffix(p, "/") || strings.HasPrefix(clean, p) {
			return true
		}
	}

	// /home/<other>: anything under /home except resolved home
	if strings.HasPrefix(clean, "/home/") && home != "" {
		if !hasPrefix(clean, filepath.Clean(home)) {
			return true
		}
	}

	if home == "" {
		return false
	}
	hc := filepath.Clean(home)
	if clean == hc {
		return true
	}
	for _, rel := range homeBlacklistRel {
		full := filepath.Join(hc, rel)
		if clean == full || strings.HasPrefix(clean, full+string(filepath.Separator)) {
			return true
		}
	}

	if clean == filepath.Join(hc, ".docker", "config.json") {
		return true
	}
	if clean == filepath.Join(hc, ".kube", "config") {
		return true
	}
	// .kube/* allowed only for cache/http-cache
	kube := filepath.Join(hc, ".kube") + string(filepath.Separator)
	if strings.HasPrefix(clean, kube) {
		rest := strings.TrimPrefix(clean, kube)
		first := strings.SplitN(rest, string(filepath.Separator), 2)[0]
		if first != "cache" && first != "http-cache" {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/safety
git commit -m "feat(safety): allow-list bounds and hard blacklist"
```

---

### Task 6: TOCTOU-safe filesystem walker

**Files:**
- Create: `internal/cleaner/safety/fs.go`, `internal/cleaner/safety/fs_test.go`

- [ ] **Step 1: Failing test exercising symlink and cross-fs guards**

```go
package safety

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWalkSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil { t.Fatal(err) }
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil { t.Fatal(err) }

	w, err := OpenWalker(root)
	if err != nil { t.Fatal(err) }
	defer w.Close()

	var got []string
	err = w.Walk(func(e Entry) {
		if e.IsRegular() {
			got = append(got, e.RelPath)
		}
	})
	if err != nil { t.Fatal(err) }
	if len(got) != 1 || got[0] != "real" {
		t.Fatalf("expected [real]; got %v", got)
	}
}

func TestReStatBeforeUnlinkDetectsChange(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "f")
	if err := os.WriteFile(p, []byte("a"), 0o600); err != nil { t.Fatal(err) }

	w, err := OpenWalker(root)
	if err != nil { t.Fatal(err) }
	defer w.Close()

	var got Entry
	_ = w.Walk(func(e Entry) {
		if e.IsRegular() { got = e }
	})
	if err := os.WriteFile(p, []byte("BBBBBBBBBBB"), 0o600); err != nil { t.Fatal(err) }
	if w.UnlinkIfUnchanged(got) == nil {
		t.Fatal("expected ErrChanged after content modification")
	}
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement openat-style walker**

```bash
go get golang.org/x/sys/unix
```

**Design note (fd lifetime):** `Entry` does NOT hold any directory fd. fds are opened only for the duration of a single descent and closed before the walk returns. `UnlinkIfUnchanged` later re-opens parent dirs from the root fd via `openat` step-by-step, applying `O_NOFOLLOW` at every level. This (a) avoids "use after close" of fds saved in plan results, (b) gives correct TOCTOU semantics (a swap during deletion is detected at any path component), and (c) handles arbitrarily deep trees without keeping fd-per-dir alive.

`internal/cleaner/safety/fs.go`:
```go
package safety

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

type Action int

const (
	ActionDelete Action = iota
	ActionSkip
)

var ErrChanged = errors.New("entry changed since stat")
var ErrCrossFS = errors.New("entry on different filesystem")
var ErrNotFound = errors.New("entry not found")

// Entry is a stat snapshot identified by path-relative-to-walker-root.
// It holds no live fds.
type Entry struct {
	RelPath string
	stat    unix.Stat_t
}

func (e Entry) IsRegular() bool { return (e.stat.Mode & unix.S_IFMT) == unix.S_IFREG }
func (e Entry) IsDir() bool     { return (e.stat.Mode & unix.S_IFMT) == unix.S_IFDIR }
func (e Entry) IsSymlink() bool { return (e.stat.Mode & unix.S_IFMT) == unix.S_IFLNK }
func (e Entry) Size() int64     { return e.stat.Size }
func (e Entry) Mtime() int64    { return int64(e.stat.Mtim.Sec) }
func (e Entry) Ctime() int64    { return int64(e.stat.Ctim.Sec) }

type Walker struct {
	rootFD  int
	rootDev uint64
	rootAbs string
}

// OpenWalker opens `root` as a directory. To handle a single regular file
// (e.g. a glob match like ~/.zcompdump-*), use OpenFileEntry instead.
func OpenWalker(root string) (*Walker, error) {
	fd, err := unix.Open(root, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	var st unix.Stat_t
	if err := unix.Fstat(fd, &st); err != nil {
		unix.Close(fd)
		return nil, err
	}
	abs, _ := filepath.Abs(root)
	return &Walker{rootFD: fd, rootDev: st.Dev, rootAbs: abs}, nil
}

func (w *Walker) Close() error { return unix.Close(w.rootFD) }
func (w *Walker) Root() string { return w.rootAbs }

type WalkFunc func(Entry)

// Walk descends through w.rootFD without ever following symlinks and without
// crossing filesystems. Subdirectories matching `.git` are skipped wholesale,
// and the directory containing the `.git` child is itself never emitted to fn.
func (w *Walker) Walk(fn WalkFunc) error {
	_, err := w.walkDir(w.rootFD, "", fn)
	return err
}

// walkDir returns (skippedAsGitTree, error). When skippedAsGitTree is true,
// the caller MUST NOT pass this directory's Entry to fn — its contents have
// not been visited and removing it would discard the git tree.
func (w *Walker) walkDir(parentFD int, rel string, fn WalkFunc) (bool, error) {
	names, err := readdirnames(parentFD)
	if err != nil { return false, err }
	sort.Strings(names)

	// .git guard: any directory containing a .git child is a git work tree.
	for _, n := range names {
		if n == ".git" { return true, nil }
	}

	for _, name := range names {
		var st unix.Stat_t
		if err := unix.Fstatat(parentFD, name, &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			continue
		}
		if st.Dev != w.rootDev { continue }
		entryRel := filepath.Join(rel, name)
		mode := st.Mode & unix.S_IFMT
		switch mode {
		case unix.S_IFLNK:
			continue
		case unix.S_IFREG, unix.S_IFDIR:
		default:
			continue
		}
		e := Entry{RelPath: entryRel, stat: st}
		if e.IsDir() {
			fd, err := unix.Openat(parentFD, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			if err != nil { continue }
			gitTree, _ := w.walkDir(fd, entryRel, fn)
			unix.Close(fd)
			if gitTree {
				// Don't emit this directory — leaving it intact protects the git tree.
				continue
			}
		}
		fn(e)
	}
	return false, nil
}

// reopenLeaf walks RelPath component by component starting from rootFD,
// opening each parent with O_NOFOLLOW. Returns (parentFD, leafName).
// Caller must Close parentFD.
func (w *Walker) reopenLeaf(rel string) (int, string, error) {
	parts := strings.Split(filepath.Clean(rel), string(filepath.Separator))
	if len(parts) == 0 || parts[0] == "" {
		return -1, "", ErrNotFound
	}
	cur, err := unix.Dup(w.rootFD)
	if err != nil { return -1, "", err }
	for i := 0; i < len(parts)-1; i++ {
		next, err := unix.Openat(cur, parts[i], unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		unix.Close(cur)
		if err != nil { return -1, "", err }
		cur = next
	}
	return cur, parts[len(parts)-1], nil
}

// UnlinkIfUnchanged removes a regular-file entry. It re-opens the entry's
// parent from rootFD, re-stats with AT_SYMLINK_NOFOLLOW, and unlinks only if
// (ino,dev,size,mtim) match the snapshot.
//
// Caller MUST use RemoveEmptyDirIfMatch for directories — comparing a dir's
// pre-deletion mtime against its post-child-deletion mtime always fails,
// because removing a child mutates the parent's mtime.
func (w *Walker) UnlinkIfUnchanged(e Entry) error {
	if e.IsDir() {
		return errors.New("use RemoveEmptyDirIfMatch for directories")
	}
	parentFD, leaf, err := w.reopenLeaf(e.RelPath)
	if err != nil { return err }
	defer unix.Close(parentFD)

	var cur unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &cur, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if cur.Dev != w.rootDev { return ErrCrossFS }
	if cur.Ino != e.stat.Ino || cur.Dev != e.stat.Dev || cur.Size != e.stat.Size ||
		cur.Mtim.Sec != e.stat.Mtim.Sec || cur.Mtim.Nsec != e.stat.Mtim.Nsec {
		return ErrChanged
	}
	return unix.Unlinkat(parentFD, leaf, 0)
}

// RemoveEmptyDirIfMatch removes a directory only if (a) its inode/dev still
// match the planned entry, (b) it is still on the same filesystem, (c) it is
// still a directory (not a symlink that replaced it), and (d) it is empty.
// mtime is intentionally NOT compared, since deleting children would have
// changed it.
func (w *Walker) RemoveEmptyDirIfMatch(e Entry) error {
	if !e.IsDir() {
		return errors.New("use UnlinkIfUnchanged for non-directories")
	}
	parentFD, leaf, err := w.reopenLeaf(e.RelPath)
	if err != nil { return err }
	defer unix.Close(parentFD)

	var cur unix.Stat_t
	if err := unix.Fstatat(parentFD, leaf, &cur, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if cur.Dev != w.rootDev { return ErrCrossFS }
	if (cur.Mode & unix.S_IFMT) != unix.S_IFDIR { return ErrChanged }
	if cur.Ino != e.stat.Ino || cur.Dev != e.stat.Dev { return ErrChanged }

	// Empty check: open it and read at most one entry.
	dfd, err := unix.Openat(parentFD, leaf, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil { return err }
	names, _ := readdirnames(dfd)
	unix.Close(dfd)
	if len(names) > 0 { return ErrChanged }

	return unix.Unlinkat(parentFD, leaf, unix.AT_REMOVEDIR)
}

// OpenFileEntry stats a single file path via lstat, returning a Walker
// whose RelPath-style "entry" is the file itself. Used when a glob expands
// directly to a regular file (e.g. ~/.zcompdump-host-5.9).
func OpenFileEntry(path string) (*Walker, Entry, error) {
	abs, _ := filepath.Abs(path)
	parent := filepath.Dir(abs)
	pfd, err := unix.Open(parent, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil { return nil, Entry{}, err }
	var st unix.Stat_t
	if err := unix.Fstatat(pfd, filepath.Base(abs), &st, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		unix.Close(pfd); return nil, Entry{}, err
	}
	if (st.Mode & unix.S_IFMT) != unix.S_IFREG {
		unix.Close(pfd); return nil, Entry{}, errors.New("not a regular file")
	}
	w := &Walker{rootFD: pfd, rootDev: st.Dev, rootAbs: parent}
	return w, Entry{RelPath: filepath.Base(abs), stat: st}, nil
}

func readdirnames(fd int) ([]string, error) {
	dupFD, err := unix.Dup(fd)
	if err != nil { return nil, err }
	d := os.NewFile(uintptr(dupFD), "<dir>")
	defer d.Close()
	return d.Readdirnames(-1)
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/safety go.sum
git commit -m "feat(safety): openat-based walker with TOCTOU re-stat"
```

---

### Task 7: Recursive age filter and bottom-up empty-dir removal

**Files:**
- Create: `internal/cleaner/safety/age.go`, `internal/cleaner/safety/age_test.go`

- [ ] **Step 1: Failing test**

```go
package safety

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAgePlanKeepsRecentSkipsDirsWithLiveChildren(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	new := filepath.Join(root, "newdir")
	mustMkdirAll(t, old)
	mustMkdirAll(t, new)
	mustWriteAged(t, filepath.Join(old, "a"), -30*24*time.Hour)
	mustWriteAged(t, filepath.Join(new, "fresh"), -1*time.Hour)

	w, err := OpenWalker(root)
	if err != nil { t.Fatal(err) }
	defer w.Close()

	plan := AgePlan(w, 7, TimeFieldMtime, time.Now())
	if !plan.WillDelete(filepath.Join(root, "old")) {
		t.Error("old dir with old child should be planned for deletion")
	}
	if plan.WillDelete(filepath.Join(root, "newdir", "fresh")) {
		t.Error("fresh file should be skipped")
	}
	if plan.WillDelete(filepath.Join(root, "newdir")) {
		t.Error("dir with live child should not be deletable")
	}
}
```

`internal/cleaner/safety/age_helpers_test.go`:
```go
package safety

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustMkdirAll(t *testing.T, p string) { t.Helper(); if err := os.MkdirAll(p, 0o755); err != nil { t.Fatal(err) } }
func mustWriteAged(t *testing.T, p string, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil { t.Fatal(err) }
	when := time.Now().Add(age)
	if err := os.Chtimes(p, when, when); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

`internal/cleaner/safety/age.go`:
```go
package safety

import (
	"path/filepath"
	"time"
)

type TimeField int

const (
	TimeFieldMtime TimeField = iota
	TimeFieldCtime
)

type Plan struct {
	root      string
	delete    map[string]Entry
	keepDirs  map[string]bool
	ordered   []Entry
}

func (p *Plan) WillDelete(absPath string) bool {
	_, ok := p.delete[absPath]
	return ok
}

// Ordered returns entries in safe deletion order (children before parents).
func (p *Plan) Ordered() []Entry { return p.ordered }

func AgePlan(w *Walker, minAgeDays int, field TimeField, now time.Time) *Plan {
	threshold := now.Add(-time.Duration(minAgeDays) * 24 * time.Hour).Unix()

	type info struct{ entry Entry; passed bool; absPath string }
	all := []info{}
	rootAbs := w.Root()
	w.Walk(func(e Entry) {
		ts := e.Mtime()
		if field == TimeFieldCtime { ts = e.Ctime() }
		passed := ts <= threshold
		all = append(all, info{e, passed, filepath.Join(rootAbs, e.RelPath)})
	})

	plan := &Plan{root: rootAbs, delete: map[string]Entry{}, keepDirs: map[string]bool{}}

	// First pass: mark "kept" dirs (any dir containing a live child must be kept).
	for _, it := range all {
		if it.entry.IsRegular() && !it.passed {
			dir := filepath.Dir(it.absPath)
			for dir != rootAbs && dir != "/" {
				plan.keepDirs[dir] = true
				dir = filepath.Dir(dir)
			}
		}
	}

	// Second pass: include files that pass age and aren't in a kept dir's protection scope.
	// (A kept dir doesn't itself protect aged children — only the live child does. We delete
	// aged files normally; the kept dir simply won't be removed itself.)
	for _, it := range all {
		if it.entry.IsRegular() && it.passed {
			plan.delete[it.absPath] = it.entry
			plan.ordered = append(plan.ordered, it.entry)
		}
	}
	// Third pass: dirs whose own age passed AND not kept.
	for _, it := range all {
		if it.entry.IsDir() && it.passed && !plan.keepDirs[it.absPath] {
			plan.delete[it.absPath] = it.entry
		}
	}
	// Sort dirs after files, deepest first.
	plan.ordered = orderForDeletion(all, plan.delete)
	return plan
}

func orderForDeletion(all []anyInfo, set map[string]Entry) []Entry {
	// Deepest path first, files before parent dirs.
	type pair struct{ depth int; e Entry; abs string }
	var ps []pair
	for _, it := range all {
		if _, ok := set[it.absPath]; ok {
			ps = append(ps, pair{depth: pathDepth(it.absPath), e: it.entry, abs: it.absPath})
		}
	}
	// stable sort: deeper first; among equal depth, files (regular) before dirs.
	for i := 0; i < len(ps); i++ {
		for j := i + 1; j < len(ps); j++ {
			if ps[j].depth > ps[i].depth || (ps[j].depth == ps[i].depth && ps[j].e.IsRegular() && ps[i].e.IsDir()) {
				ps[i], ps[j] = ps[j], ps[i]
			}
		}
	}
	out := make([]Entry, len(ps))
	for i, p := range ps { out[i] = p.e }
	return out
}

type anyInfo = struct {
	entry   Entry
	passed  bool
	absPath string
}

func pathDepth(p string) int {
	n := 0
	for _, r := range p { if r == filepath.Separator { n++ } }
	return n
}
```

> The `Walker.Root()` accessor and `rootAbs` field are already in place from Task 6.

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/safety
git commit -m "feat(safety): recursive age plan with bottom-up deletion order"
```

---

### Task 8: Process guard via /proc

**Files:**
- Create: `internal/cleaner/safety/procs.go`, `internal/cleaner/safety/procs_test.go`

- [ ] **Step 1: Failing test**

```go
package safety

import "testing"

func TestAnyProcessRunningSelf(t *testing.T) {
	// Test process is itself running. We check by basename of os.Args[0],
	// but to keep test environment-agnostic we just verify "init" or "systemd" matches on Linux.
	for _, name := range []string{"init", "systemd"} {
		if AnyProcessRunning([]string{name}) {
			return
		}
	}
	t.Skip("no canonical pid-1 process found in this environment")
}

func TestAnyProcessRunningEmpty(t *testing.T) {
	if AnyProcessRunning(nil) {
		t.Error("nil names should never match")
	}
}
```

- [ ] **Step 2: FAIL** (function undefined)

- [ ] **Step 3: Implement**

`internal/cleaner/safety/procs.go`:
```go
package safety

import (
	"os"
	"path/filepath"
	"strings"
)

// AnyProcessRunning returns true if any process under /proc has a comm or
// argv[0] basename in `names`.
func AnyProcessRunning(names []string) bool {
	if len(names) == 0 { return false }
	want := map[string]bool{}
	for _, n := range names { want[n] = true }
	dirs, err := os.ReadDir("/proc")
	if err != nil { return false }
	for _, d := range dirs {
		if !d.IsDir() { continue }
		pid := d.Name()
		if pid == "" || pid[0] < '0' || pid[0] > '9' { continue }
		if comm, err := os.ReadFile(filepath.Join("/proc", pid, "comm")); err == nil {
			name := strings.TrimSpace(string(comm))
			if want[name] { return true }
		}
		if cmd, err := os.ReadFile(filepath.Join("/proc", pid, "cmdline")); err == nil {
			parts := strings.Split(string(cmd), "\x00")
			if len(parts) > 0 {
				if want[filepath.Base(parts[0])] { return true }
				// substring match for tunnels like "node ... vscode-server ..."
				joined := strings.Join(parts, " ")
				for n := range want {
					if strings.Contains(joined, n) {
						// only allow substring match for hyphenated tokens (e.g., vscode-server)
						if strings.Contains(n, "-") { return true }
					}
				}
			}
		}
	}
	return false
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/safety
git commit -m "feat(safety): /proc-based running-process guard"
```

---

### Task 9: User whitelist filter

**Files:**
- Create: `internal/cleaner/safety/whitelist.go`, `internal/cleaner/safety/whitelist_test.go`

- [ ] **Step 1: Failing test**

```go
package safety

import "testing"

func TestWhitelistMatches() {}

func TestWhitelist(t *testing.T) {
	w := NewWhitelist([]string{"/home/u/.cache/keep", "/tmp/keep"})
	if !w.Match("/home/u/.cache/keep") { t.Error() }
	if !w.Match("/home/u/.cache/keep/sub") { t.Error() }
	if w.Match("/home/u/.cache/other") { t.Error() }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

```go
package safety

import (
	"path/filepath"
	"strings"
)

type Whitelist struct{ prefixes []string }

func NewWhitelist(prefixes []string) *Whitelist {
	cleaned := make([]string, 0, len(prefixes))
	for _, p := range prefixes {
		if p == "" { continue }
		cleaned = append(cleaned, filepath.Clean(p))
	}
	return &Whitelist{prefixes: cleaned}
}

func (w *Whitelist) Match(path string) bool {
	clean := filepath.Clean(path)
	for _, p := range w.prefixes {
		if clean == p || strings.HasPrefix(clean, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/safety
git commit -m "feat(safety): user whitelist prefix matcher"
```

---

## Phase 3: Path Resolvers

### Task 10: Built-in path resolvers

**Files:**
- Create: `internal/cleaner/resolver/resolver.go`, `internal/cleaner/resolver/resolver_test.go`

- [ ] **Step 1: Failing test**

```go
package resolver

import (
	"os"
	"testing"
)

func TestXDGCacheUsesEnvOrDefault(t *testing.T) {
	old := os.Getenv("XDG_CACHE_HOME")
	defer os.Setenv("XDG_CACHE_HOME", old)

	os.Unsetenv("XDG_CACHE_HOME")
	got := MustResolve("xdg_cache", "/home/u")
	if got != "/home/u/.cache" {
		t.Errorf("got %q", got)
	}

	os.Setenv("XDG_CACHE_HOME", "/var/somewhere")
	got = MustResolve("xdg_cache", "/home/u")
	if got != "/var/somewhere" {
		t.Errorf("got %q", got)
	}
}

func TestUnknownResolver(t *testing.T) {
	if _, err := Resolve("nosuch", "/home/u"); err == nil {
		t.Fatal("want error")
	}
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

```go
package resolver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Resolver func(home string) string

var resolvers = map[string]Resolver{
	"xdg_cache":  xdg("XDG_CACHE_HOME", ".cache"),
	"xdg_state":  xdg("XDG_STATE_HOME", ".local/state"),
	"xdg_data":   xdg("XDG_DATA_HOME", ".local/share"),
	"npm_cache":  cmdResolver([]string{"npm", "config", "get", "cache"}, "$HOME/.npm"),
	"pnpm_store": cmdResolver([]string{"pnpm", "store", "path"}, "$HOME/.local/share/pnpm/store"),
	"yarn_cache": cmdResolver([]string{"yarn", "cache", "dir"}, "$HOME/.cache/yarn"),
	"bun_cache":  envResolver("BUN_INSTALL_CACHE_DIR", "$HOME/.bun/install/cache"),
	"pip_cache":  cmdResolver([]string{"pip", "cache", "dir"}, "$HOME/.cache/pip"),
	"cargo_home": envResolver("CARGO_HOME", "$HOME/.cargo"),
	"gocache":    cmdResolver([]string{"go", "env", "GOCACHE"}, "$HOME/.cache/go-build"),
	"gradle_home": envResolver("GRADLE_USER_HOME", "$HOME/.gradle"),
	"maven_local_repo": cmdResolver([]string{"mvn", "help:evaluate", "-Dexpression=settings.localRepository", "-q", "-DforceStdout"}, "$HOME/.m2/repository"),
}

// Resolve returns an absolute path for the named resolver. It only errors when
// `name` is not in the closed enum. If a resolver's primary source yields an
// empty/non-absolute value, the documented fallback is used; if the fallback
// itself ends up relative (e.g. an env var was set to a relative path), it is
// rooted under `home` so the caller always receives an absolute path.
func Resolve(name, home string) (string, error) {
	r, ok := resolvers[name]
	if !ok { return "", fmt.Errorf("unknown resolver %q", name) }
	out := filepath.Clean(r(home))
	if !filepath.IsAbs(out) { out = filepath.Clean(filepath.Join(home, out)) }
	return out, nil
}

func MustResolve(name, home string) string {
	out, err := Resolve(name, home)
	if err != nil { panic(err) }
	return out
}

func xdg(envVar, fallbackRel string) Resolver {
	return func(home string) string {
		if v := os.Getenv(envVar); v != "" { return v }
		return filepath.Join(home, fallbackRel)
	}
}

func envResolver(envVar, fallback string) Resolver {
	return func(home string) string {
		if v := os.Getenv(envVar); v != "" { return v }
		return os.Expand(fallback, func(k string) string {
			if k == "HOME" { return home }
			return os.Getenv(k)
		})
	}
}

func cmdResolver(argv []string, fallback string) Resolver {
	return func(home string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c := exec.CommandContext(ctx, argv[0], argv[1:]...)
		out, err := c.Output()
		if err == nil {
			s := strings.TrimSpace(string(out))
			if filepath.IsAbs(s) { return s }
		}
		return expandFallback(fallback, home)
	}
}

// expandFallback resolves $HOME / $XDG_* in the documented fallback string and
// guarantees an absolute path. If somehow the result is still relative, it is
// joined under home so the caller always receives an absolute path.
func expandFallback(fallback, home string) string {
	out := os.Expand(fallback, func(k string) string {
		if k == "HOME" { return home }
		return os.Getenv(k)
	})
	if !filepath.IsAbs(out) {
		out = filepath.Join(home, out)
	}
	return out
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/resolver
git commit -m "feat(resolver): closed-enum path resolvers with timeouts and fallbacks"
```

---

## Phase 4: Executors

### Task 11: Paths executor (orchestrates safety + resolver + age)

**Files:**
- Create: `internal/cleaner/executor/paths.go`, `internal/cleaner/executor/paths_test.go`

- [ ] **Step 1: Failing test against testdata fakehome**

```go
package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

func TestPathsExecutorDeletesAgedFiles(t *testing.T) {
	home := t.TempDir()
	cache := filepath.Join(home, ".cache", "demo")
	if err := os.MkdirAll(cache, 0o755); err != nil { t.Fatal(err) }
	old := filepath.Join(cache, "old.bin")
	new := filepath.Join(cache, "new.bin")
	if err := os.WriteFile(old, make([]byte, 1024), 0o600); err != nil { t.Fatal(err) }
	if err := os.WriteFile(new, make([]byte, 1024), 0o600); err != nil { t.Fatal(err) }
	when := time.Now().Add(-30 * 24 * time.Hour)
	_ = os.Chtimes(old, when, when)

	c := model.Cleaner{
		ID: "demo", Name: "demo", Scope: model.ScopeUser, Type: model.TypePaths,
		MinAgeDays: ptrInt(7), Paths: []string{filepath.Join(home, ".cache", "demo")},
	}
	events := captureEvents(t, func(emit func(model.Event)) {
		exec := NewPathsExecutor(home, safety.NewWhitelist(nil))
		_ = exec.Run(context.Background(), c, false, emit)
	})

	if !hasDelete(events, old) { t.Errorf("expected old to be deleted; events: %v", events) }
	if hasDelete(events, new) { t.Errorf("new file should NOT be deleted") }
	if _, err := os.Stat(old); !os.IsNotExist(err) { t.Errorf("old still exists") }
	if _, err := os.Stat(new); err != nil { t.Errorf("new should still exist: %v", err) }
}

func ptrInt(v int) *int { return &v }
```

`internal/cleaner/executor/testhelpers_test.go`:
```go
package executor

import (
	"sync"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func captureEvents(t *testing.T, fn func(emit func(model.Event))) []model.Event {
	t.Helper()
	var mu sync.Mutex
	var evs []model.Event
	fn(func(e model.Event) { mu.Lock(); evs = append(evs, e); mu.Unlock() })
	return evs
}

func hasDelete(evs []model.Event, path string) bool {
	for _, e := range evs {
		if e.Event == model.EvDeleted && e.Path == path { return true }
	}
	return false
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

```go
package executor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/resolver"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"golang.org/x/sys/unix"
)

type PathsExecutor struct {
	Home      string
	Whitelist *safety.Whitelist
}

func NewPathsExecutor(home string, wl *safety.Whitelist) *PathsExecutor {
	return &PathsExecutor{Home: home, Whitelist: wl}
}

func (p *PathsExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: time.Now()})

	if safety.AnyProcessRunning(c.RunningProcesses) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "running_process", TS: time.Now()})
		emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "skipped", TS: time.Now()})
		return nil
	}

	roots, err := p.expandRoots(c)
	if err != nil {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "boundary_violation", Detail: err.Error(), TS: time.Now()})
		emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "skipped", TS: time.Now()})
		return nil
	}

	field := safety.TimeFieldMtime
	if c.TimeField == model.TimeCtime { field = safety.TimeFieldCtime }
	age := c.AgeOrDefault(14)

	excludes := compileGlobs(c.Exclude)
	startedAt := time.Now()
	var bytesFreed int64
	var filesDel int64
	var errors_ int

	process := func(w *safety.Walker, entries []safety.Entry) {
		for _, e := range entries {
			abs := filepath.Join(w.Root(), e.RelPath)
			if p.Whitelist != nil && p.Whitelist.Match(abs) {
				emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "whitelisted", TS: time.Now()})
				continue
			}
			if matchAny(excludes, abs) {
				emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "whitelisted", TS: time.Now()})
				continue
			}
			if dryRun {
				emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "dry_run", Size: e.Size(), TS: time.Now()})
				bytesFreed += e.Size()
				filesDel++
				continue
			}
			var unlinkErr error
			if e.IsDir() {
				unlinkErr = w.RemoveEmptyDirIfMatch(e)
			} else {
				unlinkErr = w.UnlinkIfUnchanged(e)
			}
			if unlinkErr != nil {
				if errors.Is(unlinkErr, safety.ErrChanged) {
					emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: abs, Reason: "toctou_changed", TS: time.Now()})
					continue
				}
				emit(model.Event{Event: model.EvError, CleanerID: c.ID, Path: abs, Reason: "unlink_failed", Detail: unlinkErr.Error(), TS: time.Now()})
				errors_++
				continue
			}
			emit(model.Event{Event: model.EvDeleted, CleanerID: c.ID, Path: abs, Size: e.Size(), TS: time.Now()})
			bytesFreed += e.Size()
			filesDel++
		}
	}

	for _, root := range roots {
		// Distinguish file vs directory roots using lstat. Regular files take
		// the OpenFileEntry path; directories take the OpenWalker + AgePlan path.
		var st unix.Stat_t
		if err := unix.Lstat(root, &st); err != nil { continue }
		mode := st.Mode & unix.S_IFMT
		if mode == unix.S_IFLNK { continue }

		if mode == unix.S_IFREG {
			w, e, err := safety.OpenFileEntry(root)
			if err != nil { continue }
			ts := e.Mtime(); if field == safety.TimeFieldCtime { ts = e.Ctime() }
			threshold := time.Now().Add(-time.Duration(age) * 24 * time.Hour).Unix()
			if ts <= threshold {
				process(w, []safety.Entry{e})
			} else {
				emit(model.Event{Event: model.EvSkipped, CleanerID: c.ID, Path: root, Reason: "age_too_recent", TS: time.Now()})
			}
			w.Close()
			continue
		}
		if mode != unix.S_IFDIR { continue }

		w, err := safety.OpenWalker(root)
		if err != nil { continue }
		plan := safety.AgePlan(w, age, field, time.Now())
		process(w, plan.Ordered())
		w.Close()
	}

	status := "ok"
	if errors_ > 0 { status = "error" }
	emit(model.Event{
		Event: model.EvFinish, CleanerID: c.ID, Status: status,
		FilesDeleted: filesDel, BytesFreed: bytesFreed, Errors: errors_,
		DurationMs: time.Since(startedAt).Milliseconds(), TS: time.Now(),
	})
	return nil
}

// expandRoots resolves c.Paths and c.PathResolvers (with subpaths) to absolute roots
// that pass §6.1 boundary + blacklist checks. A root failing checks is dropped.
func (p *PathsExecutor) expandRoots(c model.Cleaner) ([]string, error) {
	var raws []string
	for _, pat := range c.Paths {
		raws = append(raws, expandHome(pat, p.Home))
	}
	for _, ref := range c.PathResolvers {
		base, err := resolver.Resolve(ref.Resolver, p.Home)
		if err != nil { continue }
		if len(ref.Subpaths) == 0 {
			raws = append(raws, base)
		}
		for _, sp := range ref.Subpaths {
			raws = append(raws, filepath.Join(base, sp))
		}
	}
	out := make([]string, 0, len(raws))
	for _, r := range raws {
		matches, err := filepath.Glob(r)
		if err != nil || len(matches) == 0 {
			matches = []string{strings.TrimSuffix(r, "/*")}
		}
		for _, m := range matches {
			if !safety.InsideAllowList(m, p.Home) { continue }
			if safety.Blacklisted(m, p.Home) { continue }
			out = append(out, m)
		}
	}
	if len(out) == 0 { return nil, errors.New("no valid roots after safety check") }
	return out, nil
}

func expandHome(p, home string) string {
	if strings.HasPrefix(p, "~/") { return filepath.Join(home, p[2:]) }
	return p
}

type globSet []string

func compileGlobs(patterns []string) globSet { return globSet(patterns) }
func matchAny(set globSet, path string) bool {
	for _, g := range set {
		if ok, _ := filepath.Match(g, filepath.Base(path)); ok { return true }
	}
	return false
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/executor
git commit -m "feat(executor): paths executor with age plan and TOCTOU-safe deletion"
```

---

### Task 11.5: Path safety validation (registry.ValidatePaths)

**Files:**
- Create: `internal/cleaner/registry/validate_paths.go`, `internal/cleaner/registry/validate_paths_test.go`

This task adds the §6.1 load-time path-safety gate. It is split out from Task 3 because it imports `safety` (Task 5) and `resolver` (Task 10).

- [ ] **Step 1: Failing tests**

```go
package registry

import (
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestValidatePathsRejectsBlacklist(t *testing.T) {
	bad := model.Cleaner{ID: "bad", Scope: model.ScopeUser, Type: model.TypePaths, Paths: []string{"/etc/passwd"}}
	if err := ValidatePaths(bad, "/home/u"); err == nil { t.Fatal("expected blacklist error") }
}

func TestValidatePathsRejectsOutsideAllowList(t *testing.T) {
	bad := model.Cleaner{ID: "bad", Scope: model.ScopeUser, Type: model.TypePaths, Paths: []string{"/opt/cache/*"}}
	if err := ValidatePaths(bad, "/home/u"); err == nil { t.Fatal("expected allow-list error") }
}

func TestValidatePathsAcceptsGlobUnderHome(t *testing.T) {
	ok := model.Cleaner{ID: "ok", Scope: model.ScopeUser, Type: model.TypePaths, Paths: []string{"~/.cache/foo/*"}}
	if err := ValidatePaths(ok, "/home/u"); err != nil { t.Fatalf("unexpected: %v", err) }
}
```

- [ ] **Step 2: Run, expect FAIL**

Run: `go test ./internal/cleaner/registry/...`
Expected: FAIL — `ValidatePaths` undefined.

- [ ] **Step 3: Implement**

```go
package registry

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/resolver"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

// ValidatePaths is the §6.1 load-time gate: every static path and every
// resolver fallback must be inside the allow-list and not on the blacklist.
// Failing any path here REFUSES the entire cleaner — there is no "partial" mode.
// Pass home="" for system-scope cleaners.
func ValidatePaths(c model.Cleaner, home string) error {
	if c.Type != model.TypePaths { return nil }
	check := func(p string) error {
		expanded := expandHome(p, home)
		base := globPrefix(expanded)
		if !safety.InsideAllowList(base, home) {
			return fmt.Errorf("cleaner %q: path %q outside allow-list", c.ID, p)
		}
		if safety.Blacklisted(base, home) {
			return fmt.Errorf("cleaner %q: path %q in blacklist", c.ID, p)
		}
		return nil
	}
	for _, p := range c.Paths {
		if err := check(p); err != nil { return err }
	}
	for _, ref := range c.PathResolvers {
		base, err := resolver.Resolve(ref.Resolver, home)
		if err != nil { return fmt.Errorf("cleaner %q: %w", c.ID, err) }
		if err := check(base); err != nil { return err }
		for _, sp := range ref.Subpaths {
			if err := check(filepath.Join(base, sp)); err != nil { return err }
		}
	}
	return nil
}

func expandHome(p, home string) string {
	if home != "" && strings.HasPrefix(p, "~/") { return filepath.Join(home, p[2:]) }
	return p
}

func globPrefix(p string) string {
	for i, r := range p {
		if r == '*' || r == '?' || r == '[' { return filepath.Clean(p[:i]) }
	}
	return filepath.Clean(p)
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/registry
git commit -m "feat(registry): ValidatePaths load-time §6.1 gate"
```

---

### Task 12: Command runner + journal_vacuum executor

**Files:**
- Create: `internal/cleaner/executor/command.go`, `internal/cleaner/executor/journal.go`, `internal/cleaner/executor/journal_test.go`

- [ ] **Step 1: Failing test (uses /bin/true and /bin/false; journalctl skipped if missing)**

```go
package executor

import (
	"context"
	"os/exec"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestJournalVacuumSkipsWhenJournalctlMissing(t *testing.T) {
	if _, err := exec.LookPath("journalctl"); err == nil {
		t.Skip("journalctl present; skipping negative test")
	}
	c := model.Cleaner{ID: "j", Name: "journal", Scope: model.ScopeSystem, Type: model.TypeJournalVacuum, MinAgeDays: ptrInt(7)}
	evs := captureEvents(t, func(emit func(model.Event)) {
		_ = NewJournalExecutor().Run(context.Background(), c, false, emit)
	})
	for _, e := range evs {
		if e.Event == model.EvCleanerSkipped && e.Reason == "manager_not_installed" { return }
	}
	t.Fatalf("expected cleaner_skipped/manager_not_installed; got %+v", evs)
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

`internal/cleaner/executor/command.go`:
```go
package executor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

type CommandRun struct {
	Argv     []string
	Timeout  time.Duration
	OnStdout func(string)
	OnStderr func(string)
}

func RunCommand(ctx context.Context, r CommandRun) error {
	if r.Timeout == 0 { r.Timeout = 60 * time.Second }
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, r.Argv[0], r.Argv[1:]...)
	stdout, err := cmd.StdoutPipe(); if err != nil { return err }
	stderr, err := cmd.StderrPipe(); if err != nil { return err }
	if err := cmd.Start(); err != nil { return err }
	go drain(stdout, r.OnStdout)
	go drain(stderr, r.OnStderr)
	err = cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded { return fmt.Errorf("timeout after %s", r.Timeout) }
	return err
}

func drain(r io.Reader, fn func(string)) {
	if fn == nil { _, _ = io.Copy(io.Discard, r); return }
	s := bufio.NewScanner(r)
	for s.Scan() { fn(s.Text()) }
}

func emitFinish(emit func(model.Event), id string, status string, errs int, start time.Time) {
	emit(model.Event{Event: model.EvFinish, CleanerID: id, Status: status, Errors: errs, DurationMs: time.Since(start).Milliseconds(), TS: time.Now()})
}
```

`internal/cleaner/executor/journal.go`:
```go
package executor

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

type JournalExecutor struct{}

func NewJournalExecutor() *JournalExecutor { return &JournalExecutor{} }

func (j *JournalExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := time.Now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	if _, err := exec.LookPath("journalctl"); err != nil {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "manager_not_installed", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}
	age := c.AgeOrDefault(14)
	argv := []string{"journalctl", "--vacuum-time=" + fmt.Sprintf("%dd", age)}
	if dryRun {
		emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: "[dry-run] " + argv[0] + " " + argv[1], TS: time.Now()})
		emitFinish(emit, c.ID, "ok", 0, start)
		return nil
	}
	errs := 0
	err := RunCommand(ctx, CommandRun{
		Argv:    argv,
		Timeout: 5 * time.Minute,
		OnStdout: func(line string) { emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: line, TS: time.Now()}) },
		OnStderr: func(line string) { emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stderr", Line: line, TS: time.Now()}) },
	})
	if err != nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "command_failed", Detail: err.Error(), TS: time.Now()})
		errs++
	}
	status := "ok"; if errs > 0 { status = "error" }
	emitFinish(emit, c.ID, status, errs, start)
	return nil
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/executor
git commit -m "feat(executor): command runner and journal_vacuum executor"
```

---

### Task 13: pkg_cache executor

**Files:**
- Create: `internal/cleaner/executor/pkgcache.go`, `internal/cleaner/executor/pkgcache_test.go`

- [ ] **Step 1: Failing test (skips if manager not installed)**

```go
package executor

import (
	"context"
	"os/exec"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestPkgCacheSkipsWhenManagerMissing(t *testing.T) {
	mgr := "imaginarypkg"
	if _, err := exec.LookPath(mgr); err == nil { t.Skip() }
	c := model.Cleaner{ID: "p", Name: "p", Scope: model.ScopeSystem, Type: model.TypePkgCache, PkgCache: &model.PkgCacheCfg{Manager: mgr}}
	evs := captureEvents(t, func(emit func(model.Event)) { _ = NewPkgCacheExecutor().Run(context.Background(), c, false, emit) })
	for _, e := range evs {
		if e.Event == model.EvCleanerSkipped && e.Reason == "manager_not_installed" { return }
	}
	t.Fatal("expected manager_not_installed skip")
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

```go
package executor

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

type PkgCacheExecutor struct{}

func NewPkgCacheExecutor() *PkgCacheExecutor { return &PkgCacheExecutor{} }

var pkgArgv = map[string][][]string{
	"apt":    {{"apt-get", "clean"}, {"apt-get", "autoclean"}},
	"dnf":    {{"dnf", "clean", "all"}},
	"pacman": {{"pacman", "-Sc", "--noconfirm"}},
	"zypper": {{"zypper", "clean", "-a"}},
}

func (p *PkgCacheExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := time.Now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	if c.PkgCache == nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: "pkg_cache config missing", TS: time.Now()})
		emitFinish(emit, c.ID, "error", 1, start)
		return nil
	}
	commands, ok := pkgArgv[c.PkgCache.Manager]
	if !ok {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: fmt.Sprintf("unknown manager %q", c.PkgCache.Manager), TS: time.Now()})
		emitFinish(emit, c.ID, "error", 1, start); return nil
	}
	if _, err := exec.LookPath(commands[0][0]); err != nil {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "manager_not_installed", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start); return nil
	}
	if dryRun {
		for _, argv := range commands {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: "[dry-run] " + fmt.Sprintf("%v", argv), TS: time.Now()})
		}
		emitFinish(emit, c.ID, "ok", 0, start); return nil
	}
	errs := 0
	for _, argv := range commands {
		err := RunCommand(ctx, CommandRun{
			Argv: argv, Timeout: 5 * time.Minute,
			OnStdout: func(line string) { emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: line, TS: time.Now()}) },
			OnStderr: func(line string) { emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stderr", Line: line, TS: time.Now()}) },
		})
		if err != nil {
			emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "command_failed", Detail: err.Error(), TS: time.Now()})
			errs++
		}
	}
	status := "ok"; if errs > 0 { status = "error" }
	emitFinish(emit, c.ID, status, errs, start)
	return nil
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/executor
git commit -m "feat(executor): pkg_cache executor for apt/dnf/pacman/zypper"
```

---

### Task 14: container_prune executor with rootless verification

**Files:**
- Create: `internal/cleaner/executor/container.go`, `internal/cleaner/executor/container_test.go`

- [ ] **Step 1: Failing test for argv shape and rootless verification gating**

```go
package executor

import (
	"os/exec"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestArgvForTarget(t *testing.T) {
	cases := []struct{ runtime, target string; want []string }{
		{"docker", "builder",   []string{"docker","builder","prune","-f","--filter","until=336h"}},
		{"docker", "image",     []string{"docker","image","prune","-af","--filter","until=336h"}},
		{"docker", "container", []string{"docker","container","prune","-f","--filter","until=336h"}},
		{"docker", "network",   []string{"docker","network","prune","-f","--filter","until=336h"}},
		{"podman", "system",    []string{"podman","system","prune","-f","--filter","until=336h"}},
	}
	for _, c := range cases {
		got, err := containerArgv(c.runtime, c.target, 14)
		if err != nil { t.Fatalf("%+v: %v", c, err) }
		if !equal(got, c.want) { t.Errorf("argv mismatch: got %v want %v", got, c.want) }
	}
}

func TestVolumeTargetRejected(t *testing.T) {
	if _, err := containerArgv("docker", "volume", 14); err == nil {
		t.Fatal("volume target must be rejected")
	}
}

func TestSkipWhenRuntimeMissing(t *testing.T) {
	if _, err := exec.LookPath("docker"); err == nil { t.Skip() }
	c := model.Cleaner{ID:"c", Name:"c", Scope: model.ScopeSystem, Type: model.TypeContainerPrune, MinAgeDays: ptrInt(14), ContainerPrune: &model.ContainerPruneCfg{Runtime:"docker", Target:"builder"}}
	evs := captureEvents(t, func(emit func(model.Event)) { _ = NewContainerExecutor().Run(testContext(t), c, false, emit) })
	for _, e := range evs { if e.Event == model.EvCleanerSkipped && e.Reason == "runtime_unavailable" { return } }
	t.Fatal("expected runtime_unavailable skip")
}

func equal(a, b []string) bool { if len(a)!=len(b){return false}; for i:=range a{if a[i]!=b[i]{return false}}; return true }
```

`testContext` helper:
```go
package executor
import (
	"context"
	"testing"
)
func testContext(t *testing.T) context.Context { ctx, cancel := context.WithCancel(context.Background()); t.Cleanup(cancel); return ctx }
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

```go
package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"golang.org/x/sys/unix"
)

type ContainerExecutor struct{}

func NewContainerExecutor() *ContainerExecutor { return &ContainerExecutor{} }

func containerArgv(runtime, target string, ageDays int) ([]string, error) {
	if target == "volume" { return nil, fmt.Errorf("volume target not in v1") }
	hours := ageDays * 24
	until := "until=" + strconv.Itoa(hours) + "h"
	switch runtime {
	case "docker":
		switch target {
		case "builder", "container", "network":
			return []string{"docker", target, "prune", "-f", "--filter", until}, nil
		case "image":
			return []string{"docker", "image", "prune", "-af", "--filter", until}, nil
		case "system":
			return nil, fmt.Errorf("docker system target not in v1; use individual targets")
		}
	case "podman":
		switch target {
		case "builder", "container", "network":
			return []string{"podman", target, "prune", "-f", "--filter", until}, nil
		case "image":
			return []string{"podman", "image", "prune", "-af", "--filter", until}, nil
		case "system":
			return []string{"podman", "system", "prune", "-f", "--filter", until}, nil
		}
	}
	return nil, fmt.Errorf("unknown runtime/target %q/%q", runtime, target)
}

func (ce *ContainerExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := time.Now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	if c.ContainerPrune == nil || c.MinAgeDays == nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: "container_prune config missing", TS: time.Now()})
		emitFinish(emit, c.ID, "error", 1, start); return nil
	}
	rt := c.ContainerPrune.Runtime
	target := c.ContainerPrune.Target

	if _, err := exec.LookPath(rt); err != nil {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_unavailable", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start); return nil
	}
	if !daemonReachable(ctx, rt) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_unavailable", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start); return nil
	}
	if c.Scope == model.ScopeUser && !verifyRootless(ctx, rt) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_not_rootless", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start); return nil
	}
	if c.Scope == model.ScopeSystem && verifyRootless(ctx, rt) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_not_rootless", Detail: "system-scope cleaner detected rootless daemon", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start); return nil
	}
	if (target == "builder" || target == "image") && runtimeBusy(ctx, rt) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_busy", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start); return nil
	}

	argv, err := containerArgv(rt, target, *c.MinAgeDays)
	if err != nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: err.Error(), TS: time.Now()})
		emitFinish(emit, c.ID, "error", 1, start); return nil
	}
	if dryRun {
		emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: "[dry-run] " + strings.Join(argv, " "), TS: time.Now()})
		emitFinish(emit, c.ID, "ok", 0, start); return nil
	}
	errs := 0
	err = RunCommand(ctx, CommandRun{
		Argv: argv, Timeout: 5 * time.Minute,
		OnStdout: func(line string) { emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: line, TS: time.Now()}) },
		OnStderr: func(line string) { emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stderr", Line: line, TS: time.Now()}) },
	})
	if err != nil { emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "command_failed", Detail: err.Error(), TS: time.Now()}); errs++ }
	status := "ok"; if errs > 0 { status = "error" }
	emitFinish(emit, c.ID, status, errs, start)
	return nil
}

func daemonReachable(ctx context.Context, runtime string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second); defer cancel()
	return exec.CommandContext(ctx, runtime, "info", "--format", "{{.ServerVersion}}").Run() == nil
}

func runtimeBusy(ctx context.Context, runtime string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second); defer cancel()
	out, err := exec.CommandContext(ctx, runtime, "ps", "-q", "--filter", "status=running").Output()
	if err != nil { return false }
	return len(strings.TrimSpace(string(out))) > 0
}

// verifyRootless returns true iff the daemon is rootless AND its socket is
// owned by the calling user AND the socket lives under an expected prefix.
func verifyRootless(ctx context.Context, runtime string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second); defer cancel()
	var fmtArg string
	switch runtime {
	case "docker":
		fmtArg = "{{.SecurityOptions}}"
	case "podman":
		fmtArg = "{{.Host.Security.Rootless}}"
	default:
		return false
	}
	out, err := exec.CommandContext(ctx, runtime, "info", "--format", fmtArg).Output()
	if err != nil { return false }
	s := strings.ToLower(strings.TrimSpace(string(out)))
	if !strings.Contains(s, "rootless") && s != "true" { return false }

	sock := socketPath(ctx, runtime)
	if sock == "" { return false }
	var st unix.Stat_t
	if err := unix.Lstat(sock, &st); err != nil { return false }
	if int(st.Uid) != os.Getuid() { return false }
	uid := strconv.Itoa(os.Getuid())
	for _, prefix := range []string{"/run/user/" + uid + "/", os.Getenv("XDG_RUNTIME_DIR") + "/", os.Getenv("HOME") + "/"} {
		if prefix != "/" && strings.HasPrefix(sock, prefix) { return true }
	}
	return false
}

func socketPath(ctx context.Context, runtime string) string {
	if v := os.Getenv("DOCKER_HOST"); runtime == "docker" && strings.HasPrefix(v, "unix://") {
		return strings.TrimPrefix(v, "unix://")
	}
	switch runtime {
	case "podman":
		out, err := exec.CommandContext(ctx, "podman", "info", "--format", "{{.Host.RemoteSocket.Path}}").Output()
		if err == nil { return strings.TrimSpace(string(out)) }
	case "docker":
		// best-effort: try the default rootless path
		uid := strconv.Itoa(os.Getuid())
		p := filepath.Join("/run/user/"+uid, "docker.sock")
		if _, err := os.Stat(p); err == nil { return p }
	}
	return ""
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/executor
git commit -m "feat(executor): container_prune with rootless verification and per-target argv"
```

---

## Phase 5: Plumbing

### Task 15: Operations log (rolling)

**Files:**
- Create: `internal/oplog/oplog.go`, `internal/oplog/oplog_test.go`

- [ ] **Step 1: Failing test for rotation at 10 MB**

```go
package oplog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotates(t *testing.T) {
	dir := t.TempDir()
	o, err := New(filepath.Join(dir, "operations.log"), 64, 3)
	if err != nil { t.Fatal(err) }
	for i := 0; i < 10; i++ {
		if err := o.Write("delete", "/x", 1234, "demo"); err != nil { t.Fatal(err) }
	}
	o.Close()
	files, _ := os.ReadDir(dir)
	if len(files) > 4 { t.Errorf("too many rotated files: %d", len(files)) }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

```go
package oplog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Logger struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	maxFiles int
	f        *os.File
	written  int64
}

func New(path string, maxBytes int64, maxFiles int) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { return nil, err }
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil { return nil, err }
	st, _ := f.Stat()
	return &Logger{path: path, maxBytes: maxBytes, maxFiles: maxFiles, f: f, written: st.Size()}, nil
}

func (l *Logger) Write(op, path string, size int64, cleaner string) error {
	l.mu.Lock(); defer l.mu.Unlock()
	if l.maxBytes > 0 && l.written >= l.maxBytes { if err := l.rotate(); err != nil { return err } }
	line := fmt.Sprintf("%s\t%s\t%s\t%d\t%s\n", time.Now().UTC().Format(time.RFC3339), op, path, size, cleaner)
	n, err := l.f.WriteString(line)
	l.written += int64(n)
	return err
}

func (l *Logger) Close() error { return l.f.Close() }

func (l *Logger) rotate() error {
	_ = l.f.Close()
	for i := l.maxFiles - 1; i >= 1; i-- {
		old := fmt.Sprintf("%s.%d", l.path, i)
		new := fmt.Sprintf("%s.%d", l.path, i+1)
		if i+1 > l.maxFiles { _ = os.Remove(old); continue }
		_ = os.Rename(old, new)
	}
	_ = os.Rename(l.path, l.path+".1")
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil { return err }
	l.f = f; l.written = 0
	return nil
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/oplog
git commit -m "feat(oplog): rolling operations log writer"
```

---

### Task 16: Sysinfo — SUDO user resolution and distro detect

**Files:**
- Create: `internal/sysinfo/user.go`, `internal/sysinfo/user_test.go`, `internal/sysinfo/distro.go`, `internal/sysinfo/distro_test.go`, `internal/sysinfo/tty.go`

- [ ] **Step 1: Failing test**

```go
package sysinfo

import "testing"

func TestParseOSRelease(t *testing.T) {
	got := parseOSRelease(`NAME="Ubuntu"
ID=ubuntu
ID_LIKE=debian
VERSION_ID="24.04"`)
	if got.ID != "ubuntu" || got.IDLike != "debian" { t.Errorf("%+v", got) }
}

func TestSudoUserResolutionRequiresAllThree(t *testing.T) {
	r := SudoUserResolver{
		LookupByUID:  func(uid uint32) (string, string, error) { return "alice", "/home/alice", nil },
		LookupByName: func(name string) (uint32, string, error) { return 1000, "/home/alice", nil },
		Lstat:        func(p string) (uint32, bool, error) { return 1000, false, nil },
	}
	got, err := r.Resolve(map[string]string{"SUDO_UID": "1000", "SUDO_USER": "alice"})
	if err != nil { t.Fatal(err) }
	if got.UID != 1000 || got.Name != "alice" || got.Home != "/home/alice" { t.Errorf("%+v", got) }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

`internal/sysinfo/distro.go`:
```go
package sysinfo

import (
	"bufio"
	"os"
	"strings"
)

type OSRelease struct {
	ID     string
	IDLike string
}

func DetectOSRelease() OSRelease {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil { return OSRelease{} }
	return parseOSRelease(string(data))
}

func parseOSRelease(s string) OSRelease {
	out := OSRelease{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "ID=") { out.ID = trim(line[3:]) }
		if strings.HasPrefix(line, "ID_LIKE=") { out.IDLike = trim(line[8:]) }
	}
	return out
}

func trim(s string) string { return strings.Trim(s, `"'`) }
```

`internal/sysinfo/user.go`:
```go
package sysinfo

import (
	"errors"
	"os"
	"os/user"
	"strconv"

	"golang.org/x/sys/unix"
)

type ResolvedUser struct {
	UID  uint32
	Name string
	Home string
}

type SudoUserResolver struct {
	LookupByUID  func(uid uint32) (name, home string, err error)
	LookupByName func(name string) (uid uint32, home string, err error)
	Lstat        func(p string) (uid uint32, isSymlink bool, err error)
}

func DefaultSudoUserResolver() SudoUserResolver {
	return SudoUserResolver{
		LookupByUID: func(uid uint32) (string, string, error) {
			u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
			if err != nil { return "", "", err }
			return u.Username, u.HomeDir, nil
		},
		LookupByName: func(name string) (uint32, string, error) {
			u, err := user.Lookup(name)
			if err != nil { return 0, "", err }
			id, err := strconv.ParseUint(u.Uid, 10, 32)
			if err != nil { return 0, "", err }
			return uint32(id), u.HomeDir, nil
		},
		Lstat: func(p string) (uint32, bool, error) {
			var st unix.Stat_t
			if err := unix.Lstat(p, &st); err != nil { return 0, false, err }
			return st.Uid, (st.Mode&unix.S_IFMT) == unix.S_IFLNK, nil
		},
	}
}

func (r SudoUserResolver) Resolve(env map[string]string) (ResolvedUser, error) {
	uidStr := env["SUDO_UID"]
	name := env["SUDO_USER"]
	if uidStr == "" || name == "" {
		return ResolvedUser{}, errors.New("SUDO_UID or SUDO_USER not set")
	}
	uid64, err := strconv.ParseUint(uidStr, 10, 32)
	if err != nil || uid64 == 0 {
		return ResolvedUser{}, errors.New("SUDO_UID invalid or zero")
	}
	uid := uint32(uid64)
	gotName, homeByUID, err := r.LookupByUID(uid)
	if err != nil { return ResolvedUser{}, err }
	uidByName, homeByName, err := r.LookupByName(name)
	if err != nil { return ResolvedUser{}, err }
	if uid != uidByName || homeByUID != homeByName {
		return ResolvedUser{}, errors.New("SUDO_UID and SUDO_USER disagree")
	}
	owner, isLink, err := r.Lstat(homeByUID)
	if err != nil { return ResolvedUser{}, err }
	if isLink || owner != uid {
		return ResolvedUser{}, errors.New("home directory ownership/symlink check failed")
	}
	if gotName != name {
		return ResolvedUser{}, errors.New("name lookup mismatch")
	}
	return ResolvedUser{UID: uid, Name: name, Home: homeByUID}, nil
}

func EnvMap() map[string]string {
	out := map[string]string{}
	for _, k := range []string{"SUDO_UID", "SUDO_USER", "HOME", "USER"} {
		out[k] = os.Getenv(k)
	}
	return out
}
```

`internal/sysinfo/tty.go`:
```go
package sysinfo

import (
	"os"

	"github.com/mattn/go-isatty"
)

func IsTerminal(f *os.File) bool { return isatty.IsTerminal(f.Fd()) }
```
```bash
go get github.com/mattn/go-isatty
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/sysinfo go.sum
git commit -m "feat(sysinfo): SUDO user triple-check, /etc/os-release parser, isatty"
```

---

## Phase 6: UI / Output

### Task 17: Renderer interface + plain renderer

**Files:**
- Create: `internal/ui/renderer.go`, `internal/ui/cli/plain.go`, `internal/ui/cli/plain_test.go`

- [ ] **Step 1: Failing test**

```go
package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestPlainOneLinePerCleaner(t *testing.T) {
	var buf bytes.Buffer
	r := NewPlain(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "demo", Name: "Demo Cache", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "demo", Status: "ok", FilesDeleted: 3, BytesFreed: 1500})
	r.Render(model.Event{Event: model.EvSummary, CleanersRun: 1, BytesFreed: 1500})

	got := buf.String()
	if !strings.Contains(got, "Demo Cache") { t.Errorf("missing name: %q", got) }
	if !strings.Contains(got, "ok") || !strings.Contains(got, "1.5 kB") { t.Errorf("missing status/size: %q", got) }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

`internal/ui/renderer.go`:
```go
package ui

import "github.com/dengqi/beav/internal/cleaner/model"

type Renderer interface {
	Render(model.Event)
	Close() error
}
```

`internal/ui/cli/plain.go`:
```go
package cli

import (
	"fmt"
	"io"
	"sync"

	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dustin/go-humanize"
)

type Plain struct {
	mu sync.Mutex
	w  io.Writer
	current map[string]model.Event
}

func NewPlain(w io.Writer) *Plain { return &Plain{w: w, current: map[string]model.Event{}} }

func (p *Plain) Render(e model.Event) {
	p.mu.Lock(); defer p.mu.Unlock()
	switch e.Event {
	case model.EvStart:
		p.current[e.CleanerID] = e
	case model.EvFinish:
		s := p.current[e.CleanerID]
		fmt.Fprintf(p.w, "%s · %s · %s freed (%d files)\n", s.Name, e.Status, humanize.Bytes(uint64(e.BytesFreed)), e.FilesDeleted)
		delete(p.current, e.CleanerID)
	case model.EvCleanerSkipped:
		s := p.current[e.CleanerID]
		fmt.Fprintf(p.w, "%s · skipped (%s)\n", s.Name, e.Reason)
	case model.EvError:
		fmt.Fprintf(p.w, "%s · error · %s: %s\n", e.CleanerID, e.Reason, e.Detail)
	case model.EvSummary:
		fmt.Fprintf(p.w, "Total: %d cleaners · %s freed\n", e.CleanersRun, humanize.Bytes(uint64(e.BytesFreed)))
	}
}

func (p *Plain) Close() error { return nil }
```

```bash
go get github.com/dustin/go-humanize
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/ui go.sum
git commit -m "feat(ui): Renderer interface and plain non-TTY renderer"
```

---

### Task 18: Spinner renderer (TTY)

**Files:**
- Create: `internal/ui/cli/spinner.go`, `internal/ui/cli/spinner_test.go`

- [ ] **Step 1: Failing test (verifies it produces output without panicking; ANSI colour codes acceptable)**

```go
package cli

import (
	"bytes"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestSpinnerProducesFinishLine(t *testing.T) {
	var buf bytes.Buffer
	r := NewSpinner(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "x", Name: "X", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "x", Status: "ok", BytesFreed: 1024})
	r.Close()
	if buf.Len() == 0 { t.Fatal("no output") }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement (lipgloss styled, no live spinner — frame-based output)**

```bash
go get github.com/charmbracelet/lipgloss
```

`internal/ui/cli/spinner.go`:
```go
package cli

import (
	"fmt"
	"io"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dustin/go-humanize"
)

var (
	styleOK   = lipgloss.NewStyle().Foreground(lipgloss.Color("#22c55e"))
	styleSkip = lipgloss.NewStyle().Foreground(lipgloss.Color("#a1a1aa"))
	styleErr  = lipgloss.NewStyle().Foreground(lipgloss.Color("#ef4444"))
)

type Spinner struct {
	mu      sync.Mutex
	w       io.Writer
	current map[string]model.Event
}

func NewSpinner(w io.Writer) *Spinner { return &Spinner{w: w, current: map[string]model.Event{}} }

func (s *Spinner) Render(e model.Event) {
	s.mu.Lock(); defer s.mu.Unlock()
	switch e.Event {
	case model.EvStart:
		s.current[e.CleanerID] = e
		fmt.Fprintf(s.w, "  ⏳ %s ...\r", e.Name)
	case model.EvFinish:
		st := s.current[e.CleanerID]
		fmt.Fprintf(s.w, "  %s %s · %s freed\n",
			styleOK.Render("✓"), st.Name, humanize.Bytes(uint64(e.BytesFreed)))
		delete(s.current, e.CleanerID)
	case model.EvCleanerSkipped:
		st := s.current[e.CleanerID]
		fmt.Fprintf(s.w, "  %s %s · skipped (%s)\n", styleSkip.Render("○"), st.Name, e.Reason)
	case model.EvError:
		fmt.Fprintf(s.w, "  %s %s · %s\n", styleErr.Render("✗"), e.CleanerID, e.Detail)
	case model.EvSummary:
		fmt.Fprintf(s.w, "\nFreed %s across %d cleaners.\n", humanize.Bytes(uint64(e.BytesFreed)), e.CleanersRun)
	}
}

func (s *Spinner) Close() error { return nil }
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/ui go.sum
git commit -m "feat(ui): TTY spinner renderer with lipgloss styling"
```

---

### Task 19: JSONL renderer

**Files:**
- Create: `internal/ui/json/json.go`, `internal/ui/json/json_test.go`

- [ ] **Step 1: Failing test (verifies one JSON object per line, ts field present)**

```go
package json

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestJSONLOnePerLine(t *testing.T) {
	var buf bytes.Buffer
	r := New(&buf)
	r.Render(model.Event{Event: model.EvStart, CleanerID: "x", TS: time.Now()})
	r.Render(model.Event{Event: model.EvFinish, CleanerID: "x", Status: "ok"})
	r.Close()

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 2 { t.Fatalf("got %d lines: %q", len(lines), buf.String()) }
	for _, line := range lines {
		var e model.Event
		if err := json.Unmarshal([]byte(line), &e); err != nil { t.Errorf("invalid JSON %q: %v", line, err) }
	}
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

```go
package json

import (
	"encoding/json"
	"io"
	"sync"

	"github.com/dengqi/beav/internal/cleaner/model"
)

type Renderer struct {
	mu sync.Mutex
	w  io.Writer
}

func New(w io.Writer) *Renderer { return &Renderer{w: w} }

func (r *Renderer) Render(e model.Event) {
	r.mu.Lock(); defer r.mu.Unlock()
	b, err := json.Marshal(e); if err != nil { return }
	_, _ = r.w.Write(b)
	_, _ = r.w.Write([]byte("\n"))
}

func (r *Renderer) Close() error { return nil }
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/ui/json
git commit -m "feat(ui): JSONL renderer"
```

---

## Phase 7: Engine and CLI

### Task 20: Engine — orchestrator that runs selected cleaners

**Files:**
- Create: `internal/cleaner/engine/engine.go`, `internal/cleaner/engine/engine_test.go`

- [ ] **Step 1: Failing test for selection filter and summary aggregation**

```go
package engine

import (
	"context"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestEngineRunsSelectedAndAggregates(t *testing.T) {
	cleaners := []model.Cleaner{
		{ID: "a", Name: "A", Scope: model.ScopeUser, Type: model.TypePaths},
		{ID: "b", Name: "B", Scope: model.ScopeUser, Type: model.TypePaths, Tags: []string{"langs"}},
	}
	stub := &stubExecutor{}
	e := New(WithExecutor(model.TypePaths, stub))
	res, err := e.Run(context.Background(), cleaners, Options{Only: []string{"langs"}})
	if err != nil { t.Fatal(err) }
	if res.CleanersRun != 1 { t.Errorf("ran %d", res.CleanersRun) }
}

type stubExecutor struct{ ran int }
func (s *stubExecutor) Run(ctx context.Context, c model.Cleaner, dr bool, emit func(model.Event)) error {
	s.ran++
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID})
	emit(model.Event{Event: model.EvFinish, CleanerID: c.ID, Status: "ok"})
	return nil
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement engine**

```go
package engine

import (
	"context"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
)

type Executor interface {
	Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error
}

type Options struct {
	Scope   model.Scope    // empty = all scopes
	DryRun  bool
	Only    []string       // ID or tag match
	Skip    []string
	Emitter func(model.Event)
}

type Result struct {
	CleanersRun     int
	CleanersSkipped int
	CleanersErrored int
	BytesFreed      int64
	FilesDeleted    int64
	Errors          int
}

type Engine struct {
	executors map[model.ExecutorType]Executor
}

type Option func(*Engine)

func WithExecutor(t model.ExecutorType, e Executor) Option { return func(en *Engine) { en.executors[t] = e } }

func New(opts ...Option) *Engine {
	en := &Engine{executors: map[model.ExecutorType]Executor{}}
	for _, o := range opts { o(en) }
	return en
}

func (e *Engine) Run(ctx context.Context, all []model.Cleaner, opt Options) (Result, error) {
	res := Result{}
	for _, c := range all {
		if !c.IsEnabled() { continue }
		if opt.Scope != "" && c.Scope != opt.Scope { continue }
		if !match(c, opt.Only, opt.Skip) { continue }
		ex, ok := e.executors[c.Type]
		if !ok { continue }
		var status string
		var bytesFreed, filesDel int64
		var errs int
		emit := func(ev model.Event) {
			if opt.Emitter != nil { opt.Emitter(ev) }
			if ev.Event == model.EvFinish {
				status = ev.Status
				bytesFreed = ev.BytesFreed
				filesDel = ev.FilesDeleted
				errs = ev.Errors
			}
		}
		_ = ex.Run(ctx, c, opt.DryRun, emit)
		switch status {
		case "ok": res.CleanersRun++
		case "skipped": res.CleanersSkipped++
		case "error": res.CleanersErrored++; res.Errors += errs
		}
		res.BytesFreed += bytesFreed
		res.FilesDeleted += filesDel
	}
	if opt.Emitter != nil {
		opt.Emitter(model.Event{
			Event: model.EvSummary,
			CleanersRun: res.CleanersRun, CleanersSkipped: res.CleanersSkipped, CleanersErrored: res.CleanersErrored,
			BytesFreed: res.BytesFreed, FilesDeleted: res.FilesDeleted, Errors: res.Errors,
			TS: time.Now(),
		})
	}
	return res, nil
}

func match(c model.Cleaner, only, skip []string) bool {
	if len(only) > 0 {
		if !matchesAny(c, only) { return false }
	}
	if matchesAny(c, skip) { return false }
	return true
}

func matchesAny(c model.Cleaner, sel []string) bool {
	for _, s := range sel {
		if s == c.ID { return true }
		for _, t := range c.Tags { if t == s { return true } }
		if strings.HasPrefix(c.ID, s+"-") { return true }
	}
	return false
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cleaner/engine
git commit -m "feat(engine): orchestrator with selection filter and summary aggregation"
```

---

### Task 21: clean command wiring

**Files:**
- Create: `internal/cli/clean.go`, `internal/cli/flags.go`, `internal/cli/clean_test.go`

- [ ] **Step 1: Failing smoke test (--dry-run, --output json, exits 0 with no cleaners)**

```go
package cli

import (
	"bytes"
	"testing"
)

func TestCleanDryRunNoCleanersExitsZero(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd("test", "test", "test")
	cmd.SetArgs([]string{"clean", "--dry-run", "--output", "json", "--config-dir", t.TempDir(), "--builtin-disabled"})
	cmd.SetOut(&out); cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil { t.Fatalf("err: %v out=%q", err, out.String()) }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

`internal/cli/flags.go`:
```go
package cli

type CleanFlags struct {
	System            bool
	All               bool
	DryRun            bool
	Only              []string
	Skip              []string
	MinAge            string
	ForceNoAge        bool
	Output            string
	Yes               bool
	AllowRootHome     bool
	UserOverride      string
	ConfigDir         string
	BuiltinDisabled   bool // hidden flag for tests
}
```

`internal/cli/clean.go`:
```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/executor"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/cleaner/safety"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/oplog"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui"
	uicli "github.com/dengqi/beav/internal/ui/cli"
	uijson "github.com/dengqi/beav/internal/ui/json"
	"github.com/spf13/cobra"
)

func NewCleanCmd() *cobra.Command {
	var f CleanFlags
	c := &cobra.Command{
		Use:   "clean",
		Short: "Clean caches, logs, and other reclaimable disk usage",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClean(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), f)
		},
	}
	c.Flags().BoolVar(&f.System, "system", false, "clean system-scope cleaners (requires root)")
	c.Flags().BoolVar(&f.All, "all", false, "clean both user and system (requires root + valid SUDO_USER)")
	c.Flags().BoolVar(&f.DryRun, "dry-run", false, "do not delete; print plan")
	c.Flags().StringSliceVar(&f.Only, "only", nil, "run only matching cleaners (id, tag, or id-prefix)")
	c.Flags().StringSliceVar(&f.Skip, "skip", nil, "skip matching cleaners")
	c.Flags().StringVar(&f.MinAge, "min-age", "", "override min age (e.g. 14d or tag=3d,tag2=30d)")
	c.Flags().BoolVar(&f.ForceNoAge, "force-no-age", false, "permit cleaners without age filter")
	c.Flags().StringVar(&f.Output, "output", "", "spinner|plain|json (default: auto)")
	c.Flags().BoolVar(&f.Yes, "yes", false, "skip first-run dry-run hint")
	c.Flags().BoolVar(&f.AllowRootHome, "allow-root-home", false, "allow cleaning /root")
	c.Flags().StringVar(&f.UserOverride, "user", "", "override SUDO_USER for --all")
	c.Flags().StringVar(&f.ConfigDir, "config-dir", "", "override config dir (default: ~/.config/beav)")
	c.Flags().BoolVar(&f.BuiltinDisabled, "builtin-disabled", false, "skip embedded cleaners (for tests)")
	_ = c.Flags().MarkHidden("builtin-disabled")
	return c
}

func runClean(ctx context.Context, stdout, stderr io.Writer, f CleanFlags) error {
	cfgDir := f.ConfigDir
	if cfgDir == "" {
		home, _ := os.UserHomeDir()
		cfgDir = filepath.Join(home, ".config", "beav")
	}
	cfg, err := config.Load(cfgDir)
	if err != nil { return CLIError{code: 2, err: fmt.Errorf("config load: %w", err)} }

	var builtinList, userList []registry.Loaded
	if !f.BuiltinDisabled {
		builtinList, err = registry.LoadBuiltin(cleaners.Builtin)
		if err != nil { return CLIError{code: 2, err: fmt.Errorf("builtin load: %w", err)} }
	}
	userList, err = registry.LoadUserDir(filepath.Join(cfgDir, "cleaners.d"))
	if err != nil { return CLIError{code: 2, err: fmt.Errorf("user cleaners: %w", err)} }
	merged := registry.MergeByID(builtinList, userList)

	scope, home, err := determineScope(f)
	if err != nil { return CLIError{code: 1, err: err} }

	// Apply config + CLI precedence (CLI > config > built-in).
	merged, err = applyEffectiveConfig(merged, cfg, f, home)
	if err != nil { return CLIError{code: 2, err: err} }

	// Schema and path safety validation (load-time §6.1).
	for _, c := range merged {
		if err := registry.Validate(c); err != nil { return CLIError{code: 2, err: err} }
		validateHome := home
		if c.Scope == model.ScopeSystem { validateHome = "" }
		if err := registry.ValidatePaths(c, validateHome); err != nil {
			return CLIError{code: 2, err: err}
		}
	}

	// Output selection: CLI flag > config default > auto.
	outMode := f.Output
	if outMode == "" { outMode = cfg.Defaults.Output }
	r := chooseRenderer(outMode, stdout, stderr)
	defer r.Close()

	// Operations log (unless BEAV_NO_OPLOG=1).
	var oplogger *oplog.Logger
	if os.Getenv("BEAV_NO_OPLOG") == "" && !f.DryRun {
		stateDir := filepath.Join(os.Getenv("HOME"), ".local", "state", "beav")
		if l, err := oplog.New(filepath.Join(stateDir, "operations.log"), 10*1024*1024, 5); err == nil {
			oplogger = l
			defer oplogger.Close()
		}
	}

	wl := safety.NewWhitelist(cfg.MergedWhitelist())
	en := engine.New(
		engine.WithExecutor(model.TypePaths, executor.NewPathsExecutor(home, wl)),
		engine.WithExecutor(model.TypeJournalVacuum, executor.NewJournalExecutor()),
		engine.WithExecutor(model.TypePkgCache, executor.NewPkgCacheExecutor()),
		engine.WithExecutor(model.TypeContainerPrune, executor.NewContainerExecutor()),
	)

	emit := func(ev model.Event) {
		if oplogger != nil && ev.Event == model.EvDeleted {
			_ = oplogger.Write("delete", ev.Path, ev.Size, ev.CleanerID)
		}
		r.Render(ev)
	}

	res, err := en.Run(ctx, merged, engine.Options{
		Scope: scope, DryRun: f.DryRun, Only: f.Only, Skip: f.Skip,
		Emitter: emit,
	})
	if err != nil { return CLIError{code: 3, err: err} }
	if res.CleanersErrored > 0 {
		return CLIError{code: 3, err: fmt.Errorf("errors in %d cleaners", res.CleanersErrored)}
	}
	return nil
}

// applyEffectiveConfig merges config defaults/overrides and CLI flags.
//
// Age precedence (highest first):
//   1. CLI per-tag override (--min-age=tag=Nd)
//   2. CLI global (--min-age=Nd)
//   3. config Overrides[id].MinAgeDays
//   4. cleaner's built-in min_age_days
//   5. config Defaults.MinAgeDays
//
// Cleaners with NoAgeFilter=true bypass age entirely and are only enabled
// when --force-no-age is passed.
func applyEffectiveConfig(cs []model.Cleaner, cfg *config.Config, f CleanFlags, home string) ([]model.Cleaner, error) {
	globalAge, perTagAge, err := parseMinAge(f.MinAge)
	if err != nil { return nil, err }

	out := make([]model.Cleaner, 0, len(cs))
	for _, c := range cs {
		// enabled: config override wins over built-in default.
		if ovr, ok := cfg.Overrides[c.ID]; ok && ovr.Enabled != nil {
			b := *ovr.Enabled
			c.Enabled = &b
		}

		if c.NoAgeFilter {
			// Explicit no-age cleaner: only runs with --force-no-age.
			if !f.ForceNoAge {
				b := false; c.Enabled = &b
			}
			out = append(out, c)
			continue
		}

		// Walk the precedence ladder for age.
		age := -1
		for _, tag := range append([]string{c.ID}, c.Tags...) {
			if v, ok := perTagAge[tag]; ok { age = v; break }
		}
		if age == -1 && globalAge >= 0 { age = globalAge }
		if age == -1 {
			if ovr, ok := cfg.Overrides[c.ID]; ok && ovr.MinAgeDays != nil { age = *ovr.MinAgeDays }
		}
		if age == -1 && c.MinAgeDays != nil { age = *c.MinAgeDays }
		if age == -1 { age = cfg.Defaults.MinAgeDays }
		a := age
		c.MinAgeDays = &a
		out = append(out, c)
	}
	return out, nil
}

// parseMinAge accepts "14d", "tag=3d,tag2=30d", or empty.
func parseMinAge(s string) (global int, perTag map[string]int, err error) {
	global = -1
	perTag = map[string]int{}
	if s == "" { return }
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" { continue }
		if eq := strings.IndexByte(part, '='); eq > 0 {
			tag := part[:eq]
			d, e := parseDays(part[eq+1:]); if e != nil { return 0, nil, e }
			perTag[tag] = d
			continue
		}
		d, e := parseDays(part); if e != nil { return 0, nil, e }
		global = d
	}
	return
}

func parseDays(s string) (int, error) {
	s = strings.TrimSuffix(s, "d")
	v, err := strconv.Atoi(s)
	if err != nil { return 0, fmt.Errorf("invalid age %q: %w", s, err) }
	return v, nil
}

func chooseRenderer(out string, stdout, stderr io.Writer) ui.Renderer {
	switch out {
	case "json": return uijson.New(stdout)
	case "plain": return uicli.NewPlain(stdout)
	case "spinner": return uicli.NewSpinner(stdout)
	}
	if f, ok := stdout.(*os.File); ok && sysinfo.IsTerminal(f) {
		return uicli.NewSpinner(stdout)
	}
	return uicli.NewPlain(stdout)
}

func determineScope(f CleanFlags) (model.Scope, string, error) {
	if f.System && f.All { return "", "", errors.New("--system and --all are mutually exclusive") }
	uid := os.Getuid()
	if !f.System && !f.All {
		if uid == 0 && !f.AllowRootHome { return "", "", errors.New("running as root with no --system/--all; pass --allow-root-home to clean /root") }
		home, _ := os.UserHomeDir()
		return model.ScopeUser, home, nil
	}
	if uid != 0 {
		return "", "", errors.New("--system and --all require root; run with sudo")
	}
	if f.System { return model.ScopeSystem, "", nil }
	r := sysinfo.DefaultSudoUserResolver()
	env := sysinfo.EnvMap()
	if f.UserOverride != "" {
		env["SUDO_USER"] = f.UserOverride
		uid, home, err := r.LookupByName(f.UserOverride); if err != nil { return "", "", err }
		env["SUDO_UID"] = strconv.FormatUint(uint64(uid), 10)
		_ = home
	}
	resolved, err := r.Resolve(env)
	if err != nil { return "", "", fmt.Errorf("--all home resolution failed: %w", err) }
	return "", resolved.Home, nil // "" scope = both
}

type CLIError struct{ code int; err error }
func (e CLIError) Error() string { return e.err.Error() }
func (e CLIError) Code() int     { return e.code }
func (e CLIError) Unwrap() error { return e.err }
```

Update `cmd/beav/main.go`:
```go
package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/dengqi/beav/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	root := cli.NewRootCmd(version, commit, date)
	if err := root.Execute(); err != nil {
		var cerr cli.CLIError
		if errors.As(err, &cerr) {
			fmt.Fprintln(os.Stderr, cerr.Error())
			os.Exit(cerr.Code())
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}
```

Update `root.go`:
```go
root.AddCommand(NewVersionCmd(version, commit, date))
root.AddCommand(NewCleanCmd())
```

- [ ] **Step 4: Build and test**

Run:
```bash
go test ./...
go build -o bin/beav ./cmd/beav
./bin/beav clean --dry-run --builtin-disabled --config-dir /tmp/empty
```
Expected: PASS; binary exits 0 with empty summary.

- [ ] **Step 5: Commit**
```bash
git add internal/cli cmd/beav
git commit -m "feat(cli): clean command with scope/dry-run/output/min-age/only/skip wiring"
```

---

### Task 22: config show / completion / version aux commands

**Files:**
- Create: `internal/cli/config.go`, `internal/cli/completion.go`

- [ ] **Step 1: Failing test for config show emitting JSON**

```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfigShowPrintsCleaners(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd("t","t","t")
	cmd.SetArgs([]string{"config", "show", "--config-dir", t.TempDir()})
	cmd.SetOut(&buf); cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil { t.Fatal(err) }
	if !strings.Contains(buf.String(), "cleaners") { t.Errorf("got %q", buf.String()) }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement**

`internal/cli/config.go`:
```go
package cli

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/config"
	"github.com/spf13/cobra"
)

func NewConfigCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Inspect or edit beav config"}
	var dir string
	show := &cobra.Command{
		Use: "show",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dir == "" { home, _ := os.UserHomeDir(); dir = filepath.Join(home, ".config", "beav") }
			cfg, err := config.Load(dir); if err != nil { return err }
			bi, err := registry.LoadBuiltin(cleaners.Builtin); if err != nil { return err }
			user, _ := registry.LoadUserDir(filepath.Join(dir, "cleaners.d"))
			merged := registry.MergeByID(bi, user)
			out := map[string]any{"config": cfg, "cleaners": merged}
			enc := json.NewEncoder(cmd.OutOrStdout()); enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
	show.Flags().StringVar(&dir, "config-dir", "", "")
	c.AddCommand(show)
	c.AddCommand(&cobra.Command{Use: "edit", RunE: func(cmd *cobra.Command, args []string) error {
		editor := os.Getenv("EDITOR"); if editor == "" { editor = "vi" }
		// not testable in unit; covered by manual
		return nil
	}})
	return c
}
```

`internal/cli/completion.go`:
```go
package cli

import (
	"github.com/spf13/cobra"
)

func NewCompletionCmd(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:       "completion [bash|zsh|fish]",
		Short:     "Generate shell completion script",
		ValidArgs: []string{"bash", "zsh", "fish"},
		Args:      cobra.ExactValidArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash": return root.GenBashCompletion(cmd.OutOrStdout())
			case "zsh":  return root.GenZshCompletion(cmd.OutOrStdout())
			case "fish": return root.GenFishCompletion(cmd.OutOrStdout(), true)
			}
			return nil
		},
	}
}
```

Wire into `root.go`:
```go
root.AddCommand(NewConfigCmd())
root.AddCommand(NewCompletionCmd(root))
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cli
git commit -m "feat(cli): config show and shell completion subcommands"
```

---

### Task 23: analyze command (gdu wrapper)

**Files:**
- Create: `internal/cli/analyze.go`, `internal/ui/tui/analyze.go`

- [ ] **Step 1: Failing build (we just verify the binary compiles and exits cleanly with --help)**

`internal/cli/analyze_test.go`:
```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestAnalyzeHelp(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd("t","t","t")
	cmd.SetArgs([]string{"analyze", "--help"})
	cmd.SetOut(&buf); cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil { t.Fatal(err) }
	if !strings.Contains(buf.String(), "Analyze") { t.Errorf("got %q", buf.String()) }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement (delegate to gdu library)**

```bash
go get github.com/dundee/gdu/v5
```

`internal/ui/tui/analyze.go`:
```go
package tui

import (
	"github.com/dundee/gdu/v5/cmd/gdu/app"
)

// RunAnalyze dispatches to gdu's TUI app on the given path.
// gdu's API is stable enough that we wrap it here; we vendor it as a library.
func RunAnalyze(path string) error {
	a := app.App{} // construct a minimal App; in practice we initialize gdu's flags struct
	return a.Run([]string{path})
}
```

> Note: gdu's exact entry point may have changed. Verify the import path in `vendor/`; the goal of this task is "binary boots, runs gdu, exits when user quits." If gdu's library API differs, adjust the wrapper accordingly — the spec only requires "wrap gdu", not a particular API call.

`internal/cli/analyze.go`:
```go
package cli

import (
	"errors"
	"os"

	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dengqi/beav/internal/ui/tui"
	"github.com/spf13/cobra"
)

func NewAnalyzeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "analyze [path]",
		Short: "Analyze disk usage in a TUI",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !sysinfo.IsTerminal(os.Stdout) {
				return errors.New("analyze requires a TTY; pipe-friendly disk analysis: try `du -sh *` or `gdu`")
			}
			path := "."
			if len(args) == 1 { path = args[0] }
			return tui.RunAnalyze(path)
		},
	}
}
```

Wire into `root.go`.

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cli internal/ui/tui go.sum
git commit -m "feat(cli): analyze subcommand wrapping gdu library"
```

---

### Task 24: Bubbletea main menu (no-args entry)

**Files:**
- Create: `internal/ui/tui/menu.go`, `internal/cli/root.go` (modify)

- [ ] **Step 1: Failing test that no-args + non-TTY prints help instead of launching TUI**

```go
package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoArgsNonTTYPrintsHelp(t *testing.T) {
	var buf bytes.Buffer
	cmd := NewRootCmd("t","t","t")
	cmd.SetArgs([]string{})
	cmd.SetOut(&buf); cmd.SetErr(&buf)
	if err := cmd.Execute(); err != nil { t.Fatal(err) }
	if !strings.Contains(buf.String(), "beav") { t.Errorf("got %q", buf.String()) }
}
```

- [ ] **Step 2: FAIL**

- [ ] **Step 3: Implement minimal main menu (TTY only)**

```bash
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/bubbles
```

`internal/ui/tui/menu.go`:
```go
package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type item struct{ title, desc, action string }
func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type menuModel struct{ list list.Model; choice string }

func (m menuModel) Init() tea.Cmd { return nil }
func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "enter" {
			if i, ok := m.list.SelectedItem().(item); ok { m.choice = i.action }
			return m, tea.Quit
		}
		if msg.String() == "ctrl+c" || msg.String() == "q" { return m, tea.Quit }
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}
func (m menuModel) View() string {
	return lipgloss.NewStyle().Margin(1, 2).Render(m.list.View())
}

// RunMenu shows the main menu and returns the chosen action: "clean", "analyze", "list", or "" on quit.
func RunMenu() (string, error) {
	items := []list.Item{
		item{title: "Clean", desc: "Run age-aware cache cleanup", action: "clean"},
		item{title: "Analyze", desc: "Interactive disk explorer", action: "analyze"},
		item{title: "List cleaners", desc: "Show all known cleaners", action: "list"},
		item{title: "Quit", desc: "", action: ""},
	}
	l := list.New(items, list.NewDefaultDelegate(), 60, 14)
	l.Title = "Beav"
	p := tea.NewProgram(menuModel{list: l})
	m, err := p.Run()
	if err != nil { return "", err }
	return m.(menuModel).choice, nil
}
```

In `root.go` add `RunE` to root command that, when args is empty AND stdout is a TTY, dispatches to `tui.RunMenu()`:
```go
root.RunE = func(cmd *cobra.Command, args []string) error {
	if !sysinfo.IsTerminal(os.Stdout) { return cmd.Help() }
	choice, err := tui.RunMenu()
	if err != nil { return err }
	switch choice {
	case "clean":   return runClean(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), CleanFlags{})
	case "analyze": return tui.RunAnalyze(".")
	case "list":    return NewConfigCmd().Commands()[0].RunE(cmd, nil)
	}
	return nil
}
```

- [ ] **Step 4: PASS, commit**
```bash
git add internal/cli internal/ui/tui go.sum
git commit -m "feat(ui): bubbletea main menu for no-args entry"
```

---

## Phase 8: Built-in cleaners (data) and integration

### Task 25: Built-in user cleaners (desktop, IDE, browser, languages)

**Files:**
- Create: `cleaners/user/{desktop,vscode,jetbrains,nvim,browsers,shells}.yaml`
- Create: `cleaners/user/langs/{npm,pnpm,yarn,bun,pip,cargo,go,gradle,maven}.yaml`

- [ ] **Step 1: Write a single integration test that loads the embedded registry and validates every cleaner**

`cleaners/registry_validate_test.go`:
```go
package cleaners_test

import (
	"testing"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/registry"
)

func TestEmbeddedRegistryValidates(t *testing.T) {
	loaded, err := registry.LoadBuiltin(cleaners.Builtin)
	if err != nil { t.Fatal(err) }
	if len(loaded) == 0 { t.Fatal("no embedded cleaners — did you add YAML?") }
	for _, l := range loaded {
		if err := registry.Validate(l.Cleaner); err != nil {
			t.Errorf("%s: %v", l.From, err)
		}
	}
}
```

- [ ] **Step 2: FAIL** (no embedded YAML yet)

- [ ] **Step 3: Extend embed pattern and author the YAML files**

Update `cleaners/builtin.go` so the embed declaration covers the new files this task is about to create:
```go
//go:embed user/*.yaml user/langs/*.yaml
var Builtin embed.FS
```

`cleaners/user/desktop.yaml`:
```yaml
- id: desktop-trash
  name: Desktop Trash
  description: User's freedesktop trash bin
  scope: user
  type: paths
  min_age_days: 30
  paths: ["~/.local/share/Trash/files/**", "~/.local/share/Trash/info/**"]
  tags: [desktop]
- id: desktop-thumbnails
  name: Thumbnail cache
  scope: user
  type: paths
  min_age_days: 30
  paths: ["~/.cache/thumbnails/**"]
  tags: [desktop]
```

`cleaners/user/vscode.yaml`:
```yaml
- id: editor-vscode
  name: VS Code Cache
  description: Editor cache; rebuilt on launch
  scope: user
  type: paths
  min_age_days: 7
  paths:
    - ~/.config/Code/Cache/*
    - ~/.config/Code/CachedData/*
    - ~/.config/Code/logs/*
    - ~/.cache/vscode-cpptools/**
    - ~/.vscode-server/data/CachedExtensionVSIXs/*
  running_processes: [code, code-insiders]
  tags: [editor, ide]
```

`cleaners/user/jetbrains.yaml`:
```yaml
- id: editor-jetbrains
  name: JetBrains IDE caches
  scope: user
  type: paths
  min_age_days: 30
  paths: ["~/.cache/JetBrains/**"]
  running_processes: [idea, goland, pycharm, webstorm, rustrover, clion]
  tags: [editor, ide]
```

`cleaners/user/nvim.yaml`:
```yaml
- id: editor-nvim
  name: Neovim state
  scope: user
  type: paths
  min_age_days: 30
  paths:
    - ~/.local/state/nvim/log/*
    - ~/.local/state/nvim/swap/*
    - ~/.local/state/nvim/undo/*
  tags: [editor]
```

`cleaners/user/browsers.yaml`:
```yaml
- id: browser-firefox
  name: Firefox cache
  scope: user
  type: paths
  min_age_days: 7
  paths:
    - ~/.cache/mozilla/firefox/*/cache2/**
    - ~/.cache/mozilla/firefox/*/startupCache/**
    - ~/.cache/mozilla/firefox/*/OfflineCache/**
  running_processes: [firefox, firefox-bin]
  tags: [browser]
- id: browser-chromium
  name: Chromium-family cache
  scope: user
  type: paths
  min_age_days: 7
  paths:
    - ~/.cache/google-chrome/*/Cache/**
    - ~/.cache/google-chrome/*/Code\ Cache/**
    - ~/.cache/google-chrome/*/GPUCache/**
    - ~/.cache/chromium/*/Cache/**
    - ~/.cache/microsoft-edge/*/Cache/**
  running_processes: [chrome, chromium, msedge]
  tags: [browser]
```

`cleaners/user/shells.yaml`:
```yaml
- id: shell-zsh
  name: zsh compdump and cache
  scope: user
  type: paths
  min_age_days: 30
  paths: ["~/.cache/zsh/*", "~/.zcompdump-*"]
  tags: [shell]
```

`cleaners/user/langs/npm.yaml`:
```yaml
- id: lang-npm
  name: npm cache
  scope: user
  type: paths
  min_age_days: 14
  path_resolvers:
    - resolver: npm_cache
      subpaths: [_cacache, _npx, _logs, _prebuilds]
  tags: [langs, js]
```

`cleaners/user/langs/pnpm.yaml`:
```yaml
- id: lang-pnpm
  name: pnpm store
  scope: user
  type: paths
  min_age_days: 14
  path_resolvers: [{ resolver: pnpm_store }]
  tags: [langs, js]
```

`cleaners/user/langs/yarn.yaml`:
```yaml
- id: lang-yarn
  name: yarn cache
  scope: user
  type: paths
  min_age_days: 14
  path_resolvers: [{ resolver: yarn_cache }]
  tags: [langs, js]
```

`cleaners/user/langs/bun.yaml`:
```yaml
- id: lang-bun
  name: bun install cache
  scope: user
  type: paths
  min_age_days: 14
  path_resolvers: [{ resolver: bun_cache }]
  tags: [langs, js]
```

`cleaners/user/langs/pip.yaml`:
```yaml
- id: lang-pip
  name: pip wheel cache
  scope: user
  type: paths
  min_age_days: 14
  path_resolvers: [{ resolver: pip_cache }]
  tags: [langs, python]
```

`cleaners/user/langs/cargo.yaml`:
```yaml
- id: lang-cargo
  name: cargo registry + git
  scope: user
  type: paths
  min_age_days: 30
  path_resolvers:
    - resolver: cargo_home
      subpaths: [registry/cache, git]
  tags: [langs, rust]
```

`cleaners/user/langs/go.yaml`:
```yaml
- id: lang-go
  name: Go build cache
  scope: user
  type: paths
  min_age_days: 30
  path_resolvers: [{ resolver: gocache }]
  tags: [langs, go]
```

`cleaners/user/langs/gradle.yaml`:
```yaml
- id: lang-gradle
  name: Gradle caches
  scope: user
  type: paths
  min_age_days: 30
  path_resolvers:
    - resolver: gradle_home
      subpaths: [caches]
  tags: [langs, jvm]
```

`cleaners/user/langs/maven.yaml`:
```yaml
- id: lang-maven
  name: Maven download cache (~/.m2/.../.cache)
  scope: user
  type: paths
  min_age_days: 30
  path_resolvers:
    - resolver: maven_local_repo
      subpaths: [".cache"]
  tags: [langs, jvm]
```

- [ ] **Step 4: PASS, commit**

Run: `go test ./...`
Expected: PASS (all YAML validates).
```bash
git add cleaners/user
git commit -m "feat(cleaners): user-scope YAML for desktop, IDE, browser, languages"
```

---

### Task 26: Built-in system + container/k8s cleaners

**Files:**
- Create: `cleaners/system/{apt,dnf,pacman,zypper,journal,varcache,tmp}.yaml`
- Create: `cleaners/system/containers/{docker,podman}.yaml`
- Create: `cleaners/user/k8s/{minikube,helm,k9s,kubectl}.yaml`
- Create: `cleaners/user/containers/{docker-rootless,podman-rootless}.yaml`

- [ ] **Step 1: Reuse registry validate test from Task 25 — it'll fail when new YAML has a typo**

- [ ] **Step 2: Extend embed pattern and author YAML**

Update `cleaners/builtin.go` to cover all directories that hold real YAML at the end of this task:
```go
//go:embed user/*.yaml user/langs/*.yaml user/k8s/*.yaml user/containers/*.yaml
//go:embed system/*.yaml system/containers/*.yaml
var Builtin embed.FS
```

`cleaners/system/apt.yaml`:
```yaml
- id: pkg-apt-archives
  name: apt downloaded packages
  scope: system
  type: pkg_cache
  pkg_cache: { manager: apt }
  needs_root: true
  tags: [pkg]
```

`cleaners/system/dnf.yaml`:
```yaml
- id: pkg-dnf
  name: dnf clean all
  scope: system
  type: pkg_cache
  pkg_cache: { manager: dnf }
  needs_root: true
  tags: [pkg]
```

`cleaners/system/pacman.yaml`:
```yaml
- id: pkg-pacman
  name: pacman package cache
  scope: system
  type: pkg_cache
  pkg_cache: { manager: pacman }
  needs_root: true
  tags: [pkg]
```

`cleaners/system/zypper.yaml`:
```yaml
- id: pkg-zypper
  name: zypper cache
  scope: system
  type: pkg_cache
  pkg_cache: { manager: zypper }
  needs_root: true
  tags: [pkg]
```

`cleaners/system/journal.yaml`:
```yaml
- id: sys-journal
  name: systemd journal
  scope: system
  type: journal_vacuum
  min_age_days: 14
  needs_root: true
  tags: [logs]
```

`cleaners/system/varcache.yaml`:
```yaml
- id: sys-varcache
  name: /var/cache stragglers
  scope: system
  type: paths
  min_age_days: 30
  needs_root: true
  paths: [/var/cache/man/*, /var/cache/fontconfig/*]
  tags: [system]
```

`cleaners/system/tmp.yaml`:
```yaml
- id: sys-tmp
  name: /tmp stragglers
  scope: system
  type: paths
  min_age_days: 10
  needs_root: true
  paths: [/tmp/*, /var/tmp/*]
  tags: [system]
```

`cleaners/system/containers/docker.yaml`:
```yaml
- id: container-docker-builder
  name: Docker builder cache
  scope: system
  type: container_prune
  min_age_days: 14
  needs_root: true
  container_prune: { runtime: docker, target: builder }
  tags: [container, docker]
- id: container-docker-images
  name: Docker unused images
  scope: system
  type: container_prune
  min_age_days: 30
  needs_root: true
  container_prune: { runtime: docker, target: image }
  tags: [container, docker]
- id: container-docker-containers
  name: Docker stopped containers
  scope: system
  type: container_prune
  min_age_days: 7
  needs_root: true
  container_prune: { runtime: docker, target: container }
  tags: [container, docker]
- id: container-docker-networks
  name: Docker unused networks
  scope: system
  type: container_prune
  min_age_days: 30
  needs_root: true
  container_prune: { runtime: docker, target: network }
  tags: [container, docker]
```

`cleaners/system/containers/podman.yaml`:
```yaml
- id: container-podman-system
  name: Podman system prune (no volumes)
  scope: system
  type: container_prune
  min_age_days: 14
  needs_root: true
  container_prune: { runtime: podman, target: system }
  tags: [container, podman]
```

`cleaners/user/containers/docker-rootless.yaml`:
```yaml
- id: container-rootless-docker-builder
  name: Rootless Docker builder cache
  scope: user
  type: container_prune
  min_age_days: 14
  container_prune: { runtime: docker, target: builder }
  tags: [container, docker, rootless]
- id: container-rootless-docker-images
  name: Rootless Docker unused images
  scope: user
  type: container_prune
  min_age_days: 30
  container_prune: { runtime: docker, target: image }
  tags: [container, docker, rootless]
```

`cleaners/user/containers/podman-rootless.yaml`:
```yaml
- id: container-rootless-podman-system
  name: Rootless Podman system prune
  scope: user
  type: container_prune
  min_age_days: 14
  container_prune: { runtime: podman, target: system }
  tags: [container, podman, rootless]
```

`cleaners/user/k8s/minikube.yaml`:
```yaml
- id: k8s-minikube-cache
  name: minikube image cache
  scope: user
  type: paths
  min_age_days: 30
  paths: [~/.minikube/cache/**]
  tags: [k8s]
```

`cleaners/user/k8s/helm.yaml`:
```yaml
- id: k8s-helm-cache
  name: Helm cache
  scope: user
  type: paths
  min_age_days: 30
  paths: [~/.cache/helm/**]
  tags: [k8s]
```

`cleaners/user/k8s/k9s.yaml`:
```yaml
- id: k8s-k9s-cache
  name: k9s cache
  scope: user
  type: paths
  min_age_days: 30
  paths: [~/.cache/k9s/**]
  tags: [k8s]
```

`cleaners/user/k8s/kubectl.yaml`:
```yaml
- id: k8s-kubectl-cache
  name: kubectl cache (never config)
  scope: user
  type: paths
  min_age_days: 30
  paths: [~/.kube/cache/**, ~/.kube/http-cache/**]
  tags: [k8s]
```

> List-shape support is already in place from Task 3.

- [ ] **Step 3: Run all tests**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 4: Commit**
```bash
git add cleaners internal/cleaner/registry
git commit -m "feat(cleaners): system + container + k8s YAML registry"
```

---

### Task 27: Integration test — fakehome end-to-end

**Files:**
- Create: `testdata/fakehome/` (populated at test time)
- Create: `internal/cleaner/engine/integration_test.go`

- [ ] **Step 1: Failing test that builds a synthetic dirty $HOME and runs the engine end-to-end**

```go
package engine_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dengqi/beav/internal/cleaner/engine"
	"github.com/dengqi/beav/internal/cleaner/executor"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

func TestFakeHomeEndToEnd(t *testing.T) {
	home := t.TempDir()
	mk := func(rel string, content string, age time.Duration) string {
		p := filepath.Join(home, rel)
		_ = os.MkdirAll(filepath.Dir(p), 0o755)
		_ = os.WriteFile(p, []byte(content), 0o600)
		when := time.Now().Add(age)
		_ = os.Chtimes(p, when, when)
		return p
	}
	old := mk(".cache/Code/Cache/old", strings.Repeat("x", 4096), -30*24*time.Hour)
	new_ := mk(".cache/Code/Cache/new", "x", -1*time.Hour)

	cs := []model.Cleaner{{
		ID: "editor-vscode", Name: "vscode",
		Scope: model.ScopeUser, Type: model.TypePaths,
		MinAgeDays: ptr(7),
		Paths: []string{filepath.Join(home, ".cache", "Code", "Cache", "*")},
	}}
	for _, c := range cs { _ = registry.Validate(c) }

	en := engine.New(engine.WithExecutor(model.TypePaths, executor.NewPathsExecutor(home, safety.NewWhitelist(nil))))
	res, err := en.Run(context.Background(), cs, engine.Options{Scope: model.ScopeUser, Emitter: func(model.Event){}})
	if err != nil { t.Fatal(err) }
	if res.CleanersRun != 1 { t.Errorf("ran %d", res.CleanersRun) }
	if _, err := os.Stat(old); !os.IsNotExist(err) { t.Errorf("old still exists") }
	if _, err := os.Stat(new_); err != nil { t.Errorf("new should exist: %v", err) }
	_ = io.Discard
}

func ptr(v int) *int { return &v }
```

- [ ] **Step 2: Run, verify PASS**

Run: `go test ./internal/cleaner/engine/...`
Expected: PASS.

- [ ] **Step 3: Commit**
```bash
git add internal/cleaner/engine
git commit -m "test(engine): fakehome end-to-end integration"
```

---

## Phase 9: CI

### Task 28: GitHub Actions cross-distro matrix

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Author CI matrix**

```yaml
name: CI
on:
  push: { branches: [main, master] }
  pull_request:
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with: { go-version: '1.24' }
      - run: go test -race -coverprofile=cover.out ./...
      - run: go tool cover -func=cover.out
      - uses: golangci/golangci-lint-action@v6
        with: { version: latest }

  cross-distro:
    needs: test
    strategy:
      matrix:
        image:
          - ubuntu:24.04
          - fedora:40
          - archlinux:latest
    runs-on: ubuntu-latest
    container:
      image: ${{ matrix.image }}
    steps:
      - uses: actions/checkout@v4
      - name: Install Go
        run: |
          set -e
          if command -v apt-get >/dev/null; then apt-get update && apt-get install -y golang-1.24 git; ln -sf /usr/lib/go-1.24/bin/go /usr/local/bin/go
          elif command -v dnf >/dev/null; then dnf install -y golang git
          elif command -v pacman >/dev/null; then pacman -Sy --noconfirm go git; fi
      - run: go test ./...
      - run: go build ./cmd/beav
      - run: ./beav clean --dry-run --output json
```

- [ ] **Step 2: Commit and verify**
```bash
git add .github
git commit -m "ci: cross-distro test matrix on Ubuntu/Fedora/Arch"
```

(CI will run on push; verification happens on the PR.)

---

## Self-Review Notes

**Spec coverage check:**
- §1 Goal — Tasks 21–24 ship clean+analyze+config+completion+version.
- §2 Non-goals — enforced by §6.1 hard blacklist (Task 5) + omission of volume/kernel cleaners.
- §3 Privilege model + §3.1 SUDO triple-check — Task 16 (resolver) + Task 21 (`determineScope`).
- §4 Architecture — file structure section + per-task file paths.
- §5.1 Schema — Task 2 (model) + Task 3 (loader/merge/validate).
- §5.2 Executor whitelist — fixed argv in Tasks 12–14.
- §5.3 Path resolvers — Task 10.
- §5.4 Container_prune — Task 14.
- §6.1 Load-time gates — Task 5 (bounds + blacklist).
- §6.2 Per-entry gates — Task 6 (openat walk + TOCTOU re-stat) + Task 7 (recursive age) + Task 8 (procs) + Task 9 (whitelist) + Task 11 (orchestration in paths executor).
- §7 CLI — Task 21 (clean), 22 (config/completion/version), 23 (analyze), 24 (menu).
- §8 UI — Tasks 17–19.
- §8.1 JSONL schema — Task 2 (Event struct) + Task 19 (renderer); reasons enforced ad-hoc per executor.
- §8.2 Exit codes — Task 21 (`CLIError` with code).
- §9 Default cleaners — Tasks 25–26.
- §11 Config — Task 4.
- §12 Dependencies — pinned in Tasks 1, 3, 6, 17, 18, 23, 24.
- §13 Testing — unit per task; cross-distro in Task 28; fakehome in Task 27.
- §14 Out of scope — not built (correct).

**Placeholder scan:** none of "TBD", "implement later", "fill in" remain. The two notes that reference behavior the implementer must verify on the spot are tagged with explicit guidance:
- Task 23 mentions verifying gdu's library API at vendor time; the goal is well-defined ("wrap gdu so analyze launches").
- Task 25 and Task 26 each extend `cleaners/builtin.go`'s `//go:embed` declaration to cover the directories whose YAML they author; the placeholder from Task 3 keeps the embed declaration valid in between.

**Type consistency:** `Cleaner`, `Event`, `Renderer`, `Executor` interface, `Walker`, `Whitelist`, `ResolvedUser`, `CLIError` — names and signatures used in later tasks match earlier definitions.

**Round-2 fixes (2026-04-26):**
- Task 3 ships an `_placeholder.yaml` and list-shape loader so `//go:embed` always matches and Task 25/26 list-shape YAML loads cleanly.
- Task 6 walker no longer caches dirfds inside `Entry`; `UnlinkIfUnchanged` re-opens the parent from rootFD via per-component `openat` (fixes UAF when AgePlan returns and dir fds have been closed). Walker also has `OpenFileEntry` for glob matches that resolve to a regular file (fixes `/tmp/*`, `~/.zcompdump-*`).
- Task 6 walker skips any directory containing a `.git` child (.git guard, §6.2 #6).
- Task 10 resolver no longer errors when its primary source fails — fallback always wins; only unknown resolver names error.
- Task 5 `Validate` is split: schema check (`Validate`) is registry-load; `ValidatePaths(c, home)` is invoked at clean-time after home resolution, refusing the entire cleaner on any allow-list / blacklist violation.
- Task 21 wires config defaults/overrides + CLI `--min-age` (global and per-tag), `--force-no-age`, `--output`, and instantiates `oplog` (honoring `BEAV_NO_OPLOG=1`).

**Round-3 fixes (2026-04-26):**
- `//go:embed` pattern in Task 3 starts narrow (`user/*.yaml`); Tasks 25 and 26 each widen it as new directories appear, avoiding "no matching files" build errors.
- `ValidatePaths` was still in Task 3 with imports that hadn't been created yet — moved to its own **Task 11.5** that runs after `safety` (Task 5) and `resolver` (Task 10) are in place. Task 3's `validate.go` is back to schema checks only.
- Task 3 loader test now correctly references `cs[0].Cleaner.ID` (not `cs[0].ID`).
- Walker's `.git` guard now bubbles a `(skipped, error)` pair so the parent loop knows not to emit the git-tree directory itself.
- Walker grew a separate `RemoveEmptyDirIfMatch` for directory removal — comparing pre-deletion mtime against post-child-deletion mtime always failed, masking valid deletions as TOCTOU. The new method matches inode/dev, re-checks fs/mode/empty, then `unlinkat(AT_REMOVEDIR)`. The paths executor dispatches between `UnlinkIfUnchanged` (files) and `RemoveEmptyDirIfMatch` (dirs).
- Resolver `Resolve` now post-processes every result with `filepath.IsAbs` + `filepath.Join(home, …)` fallback, so xdg/env/cmd resolvers all guarantee an absolute return.
- New `Cleaner.NoAgeFilter bool` (YAML `no_age_filter: true`) declares "no age filter" explicitly, cleanly distinct from a missing field. `applyEffectiveConfig` walks a documented 5-step age precedence ladder; cleaners with `NoAgeFilter` are gated by `--force-no-age`.

---

## Plan complete and saved to `docs/superpowers/plans/2026-04-26-beav-implementation.md`.

Two execution options:

1. **Subagent-Driven (recommended)** — I dispatch a fresh subagent per task, review between tasks, fast iteration.
2. **Inline Execution** — Execute tasks in this session using executing-plans, batch execution with checkpoints.

Which approach?
