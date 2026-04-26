package registry

import (
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestValidatePathsRejectsBlacklist(t *testing.T) {
	bad := model.Cleaner{ID: "bad", Scope: model.ScopeUser, Type: model.TypePaths, Paths: []string{"/etc/passwd"}}
	if err := ValidatePaths(bad, "/home/u"); err == nil {
		t.Fatal("expected blacklist error")
	}
}

func TestValidatePathsRejectsOutsideAllowList(t *testing.T) {
	bad := model.Cleaner{ID: "bad", Scope: model.ScopeUser, Type: model.TypePaths, Paths: []string{"/opt/cache/*"}}
	if err := ValidatePaths(bad, "/home/u"); err == nil {
		t.Fatal("expected allow-list error")
	}
}

func TestValidatePathsAcceptsGlobUnderHome(t *testing.T) {
	ok := model.Cleaner{ID: "ok", Scope: model.ScopeUser, Type: model.TypePaths, Paths: []string{"~/.cache/foo/*"}}
	if err := ValidatePaths(ok, "/home/u"); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestValidatePathsUsesResolverFallbackWhenCommandMissing(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	c := model.Cleaner{
		ID:    "npm",
		Scope: model.ScopeUser,
		Type:  model.TypePaths,
		PathResolvers: []model.PathResolverRef{{
			Resolver: "npm_cache",
			Subpaths: []string{"_cacache/*"},
		}},
	}
	if err := ValidatePaths(c, "/home/u"); err != nil {
		t.Fatalf("fallback resolver path should validate: %v", err)
	}
}
