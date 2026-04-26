package safety

import "testing"

func TestAnyProcessRunningSelf(t *testing.T) {
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
