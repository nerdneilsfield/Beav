package safety

import "testing"

// TestWhitelistMatches verifies that the whitelist correctly matches paths by prefix.
// TestWhitelistMatches 验证白名单是否正确按前缀匹配路径。
func TestWhitelistMatches(t *testing.T) {
	w := NewWhitelist([]string{"/home/u/.cache/keep", "/tmp/keep"})
	if !w.Match("/home/u/.cache/keep") {
		t.Error()
	}
	if !w.Match("/home/u/.cache/keep/sub") {
		t.Error()
	}
	if w.Match("/home/u/.cache/other") {
		t.Error()
	}
}
