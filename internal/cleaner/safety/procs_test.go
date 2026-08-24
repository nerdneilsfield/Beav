package safety

import (
	"os"
	"path/filepath"
	"testing"
)

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

// TestAnyProcessRunningCheckedReadsSyntheticProc verifies process guards can
// inspect a controlled proc fixture without consulting shell state.
func TestAnyProcessRunningCheckedReadsSyntheticProc(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "123"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "123", "comm"), []byte("triton\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	running, err := AnyProcessRunningChecked([]string{"triton"}, root)
	if err != nil {
		t.Fatal(err)
	}
	if !running {
		t.Fatal("synthetic triton process should match")
	}
}

// TestAnyProcessRunningCheckedFailsClosedOnProcReadError verifies proc access
// errors are surfaced to callers that must not delete while uncertain.
func TestAnyProcessRunningCheckedFailsClosedOnProcReadError(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(root, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	running, err := AnyProcessRunningChecked([]string{"triton"}, root)
	if err == nil {
		t.Fatal("expected proc read error")
	}
	if running {
		t.Fatal("error should not claim confirmed process")
	}
}
