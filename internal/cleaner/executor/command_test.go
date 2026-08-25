package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestRunCommandUsesAbsoluteExecutableAndExplicitEnvironment(t *testing.T) {
	binDir := t.TempDir()
	home := filepath.Join(t.TempDir(), "target-home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(binDir, "probe")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf 'arg=%s\\n' \"$1\"\nprintf 'home=%s\\n' \"$HOME\"\nprintf 'secret=%s\\n' \"$BEAV_SECRET\"\nprintf 'path=%s\\n' \"$PATH\"\nprintf 'cwd=%s\\n' \"$(pwd)\"\nif read line; then printf 'stdin=%s\\n' \"$line\"; else printf 'stdin=eof\\n'; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	var lines []string
	if err := RunCommand(context.Background(), CommandRun{
		Executable: script,
		Args:       []string{"ok"},
		Env:        []string{"HOME=" + home, "PATH=/safe/bin"},
		Cwd:        home,
		Timeout:    time.Second,
		OnStdout: func(line string) {
			lines = append(lines, line)
		},
	}); err != nil {
		t.Fatal(err)
	}
	out := strings.Join(lines, "\n")
	for _, want := range []string{
		"arg=ok",
		"home=" + home,
		"secret=",
		"path=/safe/bin",
		"cwd=" + home,
		"stdin=eof",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("output %q missing %q", out, want)
		}
	}
}

func TestRunCommandKillsCompleteProcessGroupOnTimeout(t *testing.T) {
	home := t.TempDir()
	script := filepath.Join(t.TempDir(), "hang")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30 &\necho $! > \"$HOME/child.pid\"\nwait\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := RunCommand(context.Background(), CommandRun{
		Executable: script,
		Env:        []string{"HOME=" + home, "PATH=/bin:/usr/bin"},
		Cwd:        home,
		Timeout:    50 * time.Millisecond,
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
	data, readErr := os.ReadFile(filepath.Join(home, "child.pid"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var pid int
	if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr != nil {
		t.Fatal(scanErr)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if err := unix.Kill(pid, 0); errors.Is(err, unix.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child process %d survived process-group kill", pid)
}

func TestRunCommandDrainsDescendantPipesAfterCancellation(t *testing.T) {
	home := t.TempDir()
	script := filepath.Join(t.TempDir(), "orphan-pipes")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 30 &\necho $! > \"$HOME/child.pid\"\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		result <- RunCommand(context.Background(), CommandRun{
			Executable: script,
			Env:        []string{"HOME=" + home, "PATH=/bin:/usr/bin"},
			Cwd:        home,
			Timeout:    50 * time.Millisecond,
		})
	}()
	var pid int
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		data, err := os.ReadFile(filepath.Join(home, "child.pid"))
		if err == nil {
			if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr != nil {
				t.Fatal(scanErr)
			}
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pid > 0 {
		defer func() { _ = unix.Kill(pid, unix.SIGKILL) }()
	}
	select {
	case err := <-result:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("error = %v, want deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunCommand hung while descendant retained stdout/stderr")
	}
	if pid > 0 {
		for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
			if err := unix.Kill(pid, 0); errors.Is(err, unix.ESRCH) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("descendant process %d survived cancellation", pid)
	}
}

func TestKillProcessGroupSurfacesGroupFailureAfterLeaderCleanup(t *testing.T) {
	original := killSyscall
	t.Cleanup(func() { killSyscall = original })
	var calls []int
	killSyscall = func(pid int, _ syscall.Signal) error {
		calls = append(calls, pid)
		if pid < 0 {
			return unix.EPERM
		}
		return nil
	}
	err := killProcessGroup(12345)
	if !errors.Is(err, unix.EPERM) {
		t.Fatalf("error = %v, want process-group EPERM", err)
	}
	if len(calls) != 2 || calls[0] != -12345 || calls[1] != 12345 {
		t.Fatalf("kill calls = %v, want group then leader cleanup", calls)
	}
}

func TestRunCommandDisablesPrivilegeGainBeforeExec(t *testing.T) {
	home := t.TempDir()
	script := filepath.Join(t.TempDir(), "status")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nwhile IFS=: read -r key value; do case \"$key\" in NoNewPrivs|CapAmb) printf '%s=%s\\n' \"$key\" \"$value\" >> \"$HOME/status\";; esac; done < /proc/self/status\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := RunCommand(context.Background(), CommandRun{
		Executable: script,
		Env:        []string{"HOME=" + home, "PATH=/bin:/usr/bin"},
		Cwd:        home,
		Timeout:    time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, "status"))
	if err != nil {
		t.Fatal(err)
	}
	status := string(data)
	if !strings.Contains(status, "NoNewPrivs=\t1") {
		t.Fatalf("status = %q, want NoNewPrivs=1", status)
	}
	if !strings.Contains(status, "CapAmb=\t0000000000000000") {
		t.Fatalf("status = %q, want empty ambient capability set", status)
	}
}

func TestRunCommandDoesNotLeakHelperPayloadFD(t *testing.T) {
	home := t.TempDir()
	script := filepath.Join(t.TempDir(), "fd-check")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nif [ -e /proc/self/fd/3 ]; then echo open > \"$HOME/fd3\"; else echo closed > \"$HOME/fd3\"; fi\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := RunCommand(context.Background(), CommandRun{
		Executable: script,
		Env:        []string{"HOME=" + home, "PATH=/bin:/usr/bin"},
		Cwd:        home,
		Timeout:    time.Second,
	}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, "fd3"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "closed" {
		t.Fatalf("fd3 state = %q, want closed", data)
	}
}
