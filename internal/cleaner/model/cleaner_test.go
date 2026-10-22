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
		{"rustup_prune", TypeRustupPrune, true},
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

func TestYAMLProfileAliasStillValidatesClosedFields(t *testing.T) {
	target := cleanerYAMLMapping(
		"id", "uv",
		"scope", "user",
		"type", "tool_prune",
		"tool_prune", &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "profile"}, {Kind: yaml.ScalarNode, Value: "lang-uv"},
			{Kind: yaml.ScalarNode, Value: "command"}, {Kind: yaml.ScalarNode, Value: "rm -rf /"},
		}},
	)
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: target}
	var c Cleaner
	if err := c.UnmarshalYAML(alias); err == nil {
		t.Fatal("expected aliased unknown profile field to be rejected")
	}
}

func TestYAMLProfileAliasAcceptsKnownFields(t *testing.T) {
	target := cleanerYAMLMapping(
		"id", "uv",
		"scope", "user",
		"type", "tool_prune",
		"tool_prune", &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "profile"}, {Kind: yaml.ScalarNode, Value: "lang-uv"},
		}},
	)
	alias := &yaml.Node{Kind: yaml.AliasNode, Alias: target}
	var c Cleaner
	if err := c.UnmarshalYAML(alias); err != nil {
		t.Fatalf("known alias rejected: %v", err)
	}
	if c.ToolPrune == nil || c.ToolPrune.Profile != "lang-uv" {
		t.Fatalf("decoded profile = %+v", c.ToolPrune)
	}
}

func TestYAMLNestedAliasKeyCannotInjectProfileField(t *testing.T) {
	src := "" +
		"id: uv\n" +
		"scope: user\n" +
		"type: tool_prune\n" +
		"tool_prune:\n" +
		"  profile: &profile lang-uv\n" +
		"  *profile: injected\n"
	var c Cleaner
	if err := yaml.Unmarshal([]byte(src), &c); err == nil {
		t.Fatal("expected aliased nested key to be rejected")
	}
}

func cleanerYAMLMapping(fields ...any) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	for i := 0; i < len(fields); i += 2 {
		key := &yaml.Node{Kind: yaml.ScalarNode, Value: fields[i].(string)}
		value, ok := fields[i+1].(*yaml.Node)
		if !ok {
			value = &yaml.Node{Kind: yaml.ScalarNode, Value: fields[i+1].(string)}
		}
		node.Content = append(node.Content, key, value)
	}
	return node
}

func TestYAMLProfileAliasCyclesFailClosed(t *testing.T) {
	key := &yaml.Node{Kind: yaml.ScalarNode, Value: "tool_prune"}
	alias := &yaml.Node{Kind: yaml.AliasNode}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{key, alias}}
	alias.Alias = root
	var c Cleaner
	if err := c.UnmarshalYAML(root); err == nil {
		t.Fatal("expected cyclic alias to fail closed")
	}

	malformed := &yaml.Node{Kind: yaml.AliasNode}
	if err := c.UnmarshalYAML(malformed); err == nil {
		t.Fatal("expected malformed alias to fail closed")
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
