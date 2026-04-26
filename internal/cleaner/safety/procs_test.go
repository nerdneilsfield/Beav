package safety

import "testing"

// TestAnyProcessRunningSelf verifies that the function can detect running system processes.
// TestAnyProcessRunningSelf 验证函数能够检测到正在运行的系统进程。
func TestAnyProcessRunningSelf(t *testing.T) {
	for _, name := range []string{"init", "systemd"} {
		if AnyProcessRunning([]string{name}) {
			return
		}
	}
	t.Skip("no canonical pid-1 process found in this environment")
}

// TestAnyProcessRunningEmpty verifies that empty input returns false.
// TestAnyProcessRunningEmpty 验证空输入返回 false。
func TestAnyProcessRunningEmpty(t *testing.T) {
	if AnyProcessRunning(nil) {
		t.Error("nil names should never match")
	}
}
