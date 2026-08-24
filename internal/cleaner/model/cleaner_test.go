package model

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// TestExecutorTypeKnown tests that ParseExecutorType correctly parses known executor types.
// TestExecutorTypeKnown 测试 ParseExecutorType 是否正确解析已知的执行器类型。
func TestExecutorTypeKnown(t *testing.T) {
	cases := []struct {
		s    string
		want ExecutorType
		ok   bool
	}{
		{"paths", TypePaths, true},
		{"command", "", false},
		{"journal_vacuum", TypeJournalVacuum, true},
		{"pkg_cache", TypePkgCache, true},
		{"container_prune", TypeContainerPrune, true},
		{"tool_prune", TypeToolPrune, true},
		{"debris", TypeDebris, true},
		{"managed_cache", TypeManagedCache, true},
		{"bogus", "", false},
	}
	for _, c := range cases {
		got, ok := ParseExecutorType(c.s)
		if ok != c.ok || got != c.want {
			t.Errorf("ParseExecutorType(%q) = (%v,%v); want (%v,%v)", c.s, got, ok, c.want, c.ok)
		}
	}
}

func TestCleanerDefaultsImpactToSafeAndEnforcesProfileFloor(t *testing.T) {
	c := Cleaner{Type: TypeToolPrune, ToolPrune: &ToolPruneCfg{Profile: "sys-nix-gc"}}
	if got := c.EffectiveImpact(); got != ImpactHeavy {
		t.Fatalf("EffectiveImpact() = %q, want %q", got, ImpactHeavy)
	}

	c = Cleaner{Type: TypePaths}
	if got := c.EffectiveImpact(); got != ImpactSafe {
		t.Fatalf("default EffectiveImpact() = %q, want %q", got, ImpactSafe)
	}
}

func TestCleanerParsesClosedProfileBlocks(t *testing.T) {
	c := Cleaner{
		Type:         TypeToolPrune,
		Impact:       ImpactHeavy,
		ToolPrune:    &ToolPruneCfg{Profile: "sys-nix-gc"},
		Debris:       nil,
		ManagedCache: nil,
	}
	if c.ToolPrune.Profile != "sys-nix-gc" {
		t.Fatalf("tool profile = %q", c.ToolPrune.Profile)
	}
}

func TestYAMLUnmarshalDefaultsOmittedImpactToSafe(t *testing.T) {
	var c Cleaner
	if err := yaml.Unmarshal([]byte("id: old\nscope: user\ntype: paths\npaths: [~/.cache/old/*]\n"), &c); err != nil {
		t.Fatal(err)
	}
	if c.Impact != ImpactSafe {
		t.Fatalf("yaml impact = %q, want %q", c.Impact, ImpactSafe)
	}
}

func TestYAMLUnmarshalRaisesImpactToProfileFloor(t *testing.T) {
	var c Cleaner
	if err := yaml.Unmarshal([]byte("id: nix\nscope: user\ntype: tool_prune\nimpact: safe\ntool_prune:\n  profile: sys-nix-gc\n"), &c); err != nil {
		t.Fatal(err)
	}
	if c.Impact != ImpactHeavy {
		t.Fatalf("profile floor impact = %q, want %q", c.Impact, ImpactHeavy)
	}
}

func TestYAMLUnmarshalPreservesInvalidImpactForValidation(t *testing.T) {
	var c Cleaner
	if err := yaml.Unmarshal([]byte("id: nix\nscope: user\ntype: tool_prune\nimpact: dangerous\ntool_prune:\n  profile: sys-nix-gc\n"), &c); err != nil {
		t.Fatal(err)
	}
	if c.Impact != Impact("dangerous") {
		t.Fatalf("invalid impact was normalized to %q", c.Impact)
	}
}

// TestScopeKnown tests that ParseScope correctly parses known scopes.
// TestScopeKnown 测试 ParseScope 是否正确解析已知的作用范围。
func TestScopeKnown(t *testing.T) {
	if _, ok := ParseScope("user"); !ok {
		t.Error("user should parse")
	}
	if _, ok := ParseScope("planet"); ok {
		t.Error("planet should not parse")
	}
}
