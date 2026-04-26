package safety

import "testing"

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
