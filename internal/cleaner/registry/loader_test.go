package registry

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// TestLoadBuiltin verifies that loadFS correctly parses a single YAML cleaner definition.
// TestLoadBuiltin 验证 loadFS 正确解析单个 YAML 清理器定义。
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
	merged, err := MergeByID(builtin, user)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Fatalf("got %d", len(cs))
	}
}

func TestLoaderTreatsEmptyAsNoOp(t *testing.T) {
	fs := fstest.MapFS{
		"empty.yaml": &fstest.MapFile{Data: []byte("[]\n")},
		"blank.yaml": &fstest.MapFile{Data: []byte("\n\n")},
	}
	cs, err := loadFS(fs, ".")
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 0 {
		t.Fatalf("expected 0; got %d", len(cs))
	}
}

func TestLoaderDefaultsLegacyCleanerImpactToSafe(t *testing.T) {
	fs := fstest.MapFS{
		"legacy.yaml": &fstest.MapFile{Data: []byte(`
id: editor-vscode
scope: user
type: paths
paths: [~/.cache/editor/*]
`)},
	}
	cs, err := loadFS(fs, ".")
	if err != nil {
		t.Fatal(err)
	}
	if got := cs[0].Cleaner.Impact; got != model.ImpactSafe {
		t.Fatalf("legacy impact = %q, want %q", got, model.ImpactSafe)
	}
}

func TestLoaderRejectsCommandWithFileAndCleanerID(t *testing.T) {
	fs := fstest.MapFS{
		"unsafe.yaml": &fstest.MapFile{Data: []byte(`
id: user-command
scope: user
type: command
`)},
	}
	_, err := loadFS(fs, ".")
	if err == nil {
		t.Fatal("expected command executor to be rejected")
	}
	for _, want := range []string{"unsafe.yaml", "user-command", "invalid type", "command"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func TestLoaderValidatesUnselectedCleanerProfile(t *testing.T) {
	fs := fstest.MapFS{
		"mixed.yaml": &fstest.MapFile{Data: []byte(`
- id: selected
  scope: user
  type: paths
  paths: [~/.cache/selected/*]
- id: unselected
  scope: user
  type: tool_prune
  tool_prune:
    profile: user-supplied-command
`)},
	}
	_, err := loadFS(fs, ".")
	if err == nil {
		t.Fatal("expected unknown profile to be rejected before selection")
	}
	for _, want := range []string{"mixed.yaml", "unselected", "unknown tool_prune profile"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func TestLoaderRejectsArbitraryProfileFields(t *testing.T) {
	fs := fstest.MapFS{
		"unsafe.yaml": &fstest.MapFile{Data: []byte(`
id: uv
scope: user
type: tool_prune
tool_prune:
  profile: lang-uv
  command: rm -rf /
`)},
	}
	_, err := loadFS(fs, ".")
	if err == nil || !strings.Contains(err.Error(), "field command not found") {
		t.Fatalf("err = %v, want unknown profile field rejection", err)
	}
}

func TestLoadUserDirRejectsReservedCargoWithMigrationError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cargo.yaml"), []byte(`
id: lang-cargo
scope: user
type: paths
paths: [~/.cargo/registry/cache]
`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadUserDir(dir)
	if err == nil {
		t.Fatal("expected reserved Cargo cleaner migration error")
	}
	for _, want := range []string{"lang-cargo", "Cargo native GC", "migration"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

func TestValidateAcceptsClosedProfileExecutors(t *testing.T) {
	cases := []model.Cleaner{
		{ID: "tool", Scope: model.ScopeUser, Type: model.TypeToolPrune, ToolPrune: &model.ToolPruneCfg{Profile: "lang-uv"}},
		{ID: "debris", Scope: model.ScopeUser, Type: model.TypeDebris, Debris: &model.DebrisCfg{Profile: "desktop-download-partials"}},
		{ID: "managed", Scope: model.ScopeUser, Type: model.TypeManagedCache, ManagedCache: &model.ManagedCacheCfg{Profile: "ai-huggingface-datasets-arrow"}},
	}
	for _, c := range cases {
		if err := Validate(c); err != nil {
			t.Errorf("Validate(%q): %v", c.ID, err)
		}
	}
}

func TestValidateRejectsNoAgeFilterForManagedCache(t *testing.T) {
	c := model.Cleaner{
		ID:          "managed",
		Scope:       model.ScopeUser,
		Type:        model.TypeManagedCache,
		NoAgeFilter: true,
		ManagedCache: &model.ManagedCacheCfg{
			Profile: "ai-torch-hub",
		},
	}
	if err := Validate(c); err == nil || !strings.Contains(err.Error(), "no_age_filter") {
		t.Fatalf("err = %v, want managed cache no_age_filter rejection", err)
	}
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
	_, err := MergeByID(builtin, user)
	if err == nil {
		t.Fatal("expected error for type override")
	}
}

func TestMergeRejectsInvalidNewCleaner(t *testing.T) {
	_, err := MergeByID(nil, []Loaded{{From: "user.yaml", Cleaner: model.Cleaner{ID: "bad", Scope: model.ScopeUser, Type: model.TypeCommand}}})
	if err == nil || !strings.Contains(err.Error(), `cleaner "bad"`) {
		t.Fatalf("err = %v, want invalid new cleaner", err)
	}
}

func TestValidateRejectsUnsupportedContainerTarget(t *testing.T) {
	c := mustParse(t, `
id: docker-volume
scope: system
type: container_prune
min_age_days: 14
container_prune:
  runtime: docker
  target: volume
`)
	if err := Validate(c); err == nil {
		t.Fatal("expected unsupported container_prune target to be rejected")
	}
}

var _ = filepath.Join
