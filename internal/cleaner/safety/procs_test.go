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

// TestAnyProcessRunningCheckedFailsClosedOnMissingMetadata verifies a proc
// race that removes both process name files is not treated as no process.
func TestAnyProcessRunningCheckedFailsClosedOnMissingMetadata(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "123"), 0o755); err != nil {
		t.Fatal(err)
	}
	running, err := AnyProcessRunningChecked([]string{"triton"}, root)
	if err == nil {
		t.Fatal("missing process metadata should fail closed")
	}
	if running {
		t.Fatal("inspection error should not claim confirmed process")
	}
}

// TestVerifyProcVisibilityRejectsHidepidTwo verifies that a proc mount hiding
// other users cannot be treated as a complete fd enumeration.
func TestVerifyProcVisibilityRejectsHidepidTwo(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "mounts"), []byte("proc /proc proc rw,nosuid,hidepid=2 0 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyProcVisibility(root, 1000); err == nil {
		t.Fatal("hidepid=2 proc fixture should be rejected")
	}
}

// TestVerifyProcVisibilityAllowsCompleteFixture verifies the explicit fixture
// seam accepts a proc mount without hidepid restrictions.
func TestVerifyProcVisibilityAllowsCompleteFixture(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "mounts"), []byte("proc /proc proc rw,nosuid 0 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyProcVisibility(root, 1000); err != nil {
		t.Fatalf("complete proc fixture rejected: %v", err)
	}
}

// TestOpenDescriptorMatchesFailsClosedOnDanglingFD verifies a descriptor
// entry that disappears or points nowhere is treated as inspection failure.
func TestOpenDescriptorMatchesFailsClosedOnDanglingFD(t *testing.T) {
	root := t.TempDir()
	procRoot := t.TempDir()
	fdRoot := filepath.Join(procRoot, "123", "fd")
	if err := os.MkdirAll(fdRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(t.TempDir(), "gone"), filepath.Join(fdRoot, "7")); err != nil {
		t.Fatal(err)
	}
	busy, err := OpenDescriptorMatches(root, 1, 1, procRoot)
	if err == nil {
		t.Fatal("dangling proc fd should fail closed")
	}
	if busy {
		t.Fatal("inspection error should not claim confirmed descriptor")
	}
}
