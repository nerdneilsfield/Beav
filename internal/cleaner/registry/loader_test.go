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
