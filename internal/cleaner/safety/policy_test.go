package safety

import "testing"

func TestPolicyConfigurableAllowAndBlacklist(t *testing.T) {
	home := "/home/u"

	// Custom allow-list entry allows path outside default roots.
	p := Policy{AllowList: []string{"/opt/mycache"}}
	if !InsideAllowListWithPolicy("/opt/mycache/data", home, p) {
		t.Fatal("expected custom allow_list entry to be permitted")
	}
	if !InsideAllowListWithPolicy("/home/u/.cache/x", home, p) {
		t.Fatal("default home root should remain allowed when ReplaceAllowList is false")
	}

	// ReplaceAllowList restricts to custom entries only.
	pReplace := Policy{AllowList: []string{"/opt/only"}, ReplaceAllowList: true}
	if InsideAllowListWithPolicy("/home/u/.cache/x", home, pReplace) {
		t.Fatal("ReplaceAllowList should exclude default roots not in AllowList")
	}
	if !InsideAllowListWithPolicy("/opt/only/sub", home, pReplace) {
		t.Fatal("ReplaceAllowList should permit entries in AllowList")
	}

	// DisableBlacklist allows normally blacklisted paths except literal "/".
	pBypass := Policy{DisableBlacklist: true}
	if BlacklistedWithPolicy("/home/u/Documents/tmp", home, pBypass) {
		t.Fatal("DisableBlacklist should allow user-configured blacklist bypass")
	}
	if !BlacklistedWithPolicy("/", home, Policy{BypassAll: true}) {
		t.Fatal("literal '/' must always remain protected")
	}
}
