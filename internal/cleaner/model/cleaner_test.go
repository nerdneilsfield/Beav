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
