package safety

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

// TestWalkSkipsSymlinks verifies that the walker does not follow symbolic links.
// TestWalkSkipsSymlinks 验证 walker 不会跟随符号链接。
func TestWalkSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real")
	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	w, err := OpenWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	var got []string
	err = w.Walk(func(e Entry) {
		if e.IsRegular() {
			got = append(got, e.RelPath)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "real" {
		t.Fatalf("expected [real]; got %v", got)
	}
}

// TestReStatBeforeUnlinkDetectsChange verifies that modifications are detected before unlinking.
// TestReStatBeforeUnlinkDetectsChange 验证在删除之前能够检测到文件修改。
func TestReStatBeforeUnlinkDetectsChange(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "f")
	if err := os.WriteFile(p, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}

	w, err := OpenWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	var got Entry
	_ = w.Walk(func(e Entry) {
		if e.IsRegular() {
			got = e
		}
	})
	if err := os.WriteFile(p, []byte("BBBBBBBBBBB"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w.UnlinkIfUnchanged(got) == nil {
		t.Fatal("expected ErrChanged after content modification")
	}
}

// TestReStatBeforeUnlinkDetectsCtimeChange verifies that metadata changes are
// detected even when size and mtime remain unchanged.
func TestReStatBeforeUnlinkDetectsCtimeChange(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "f")
	if err := os.WriteFile(p, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var captured Entry
	if err := w.Walk(func(e Entry) {
		if e.IsRegular() {
			captured = e
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o640); err != nil {
		t.Fatal(err)
	}
	if err := w.UnlinkIfUnchanged(captured); !errors.Is(err, ErrChanged) {
		t.Fatalf("want ErrChanged after ctime change, got %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("file should remain after ctime race: %v", err)
	}
}

// TestReStatBeforeUnlinkRejectsHardlinks verifies that a candidate with more
// than one link is never removed.
func TestReStatBeforeUnlinkRejectsHardlinks(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "f")
	link := filepath.Join(root, "f.link")
	if err := os.WriteFile(p, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var captured Entry
	if err := w.Walk(func(e Entry) {
		if e.RelPath == "f" {
			captured = e
		}
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(p, link); err != nil {
		t.Fatal(err)
	}
	if err := w.UnlinkIfUnchanged(captured); !errors.Is(err, ErrChanged) {
		t.Fatalf("want ErrChanged after nlink change, got %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("hardlinked file should remain: %v", err)
	}
}

// TestOpenDescriptorMatchesUsesDeviceAndInode verifies that an open descriptor
// is detected even when its pathname is outside the inspected root.
func TestOpenDescriptorMatchesUsesDeviceAndInode(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "f")
	if err := os.WriteFile(p, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var st unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &st); err != nil {
		t.Fatal(err)
	}
	procRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(procRoot, "123", "fd"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(p, filepath.Join(procRoot, "123", "fd", "7")); err != nil {
		t.Fatal(err)
	}
	busy, err := OpenDescriptorMatches(root, uint64(st.Dev), uint64(st.Ino), procRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !busy {
		t.Fatal("open descriptor should be detected")
	}
}

// TestOpenDescriptorMatchesFailsClosedOnProcReadError verifies that an
// unreadable proc tree returns an error for conservative callers.
func TestOpenDescriptorMatchesFailsClosedOnProcReadError(t *testing.T) {
	root := t.TempDir()
	procRoot := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(procRoot, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	busy, err := OpenDescriptorMatches(root, 1, 1, procRoot)
	if err == nil {
		t.Fatal("expected proc read error")
	}
	if busy {
		t.Fatal("error result should not claim a confirmed descriptor")
	}
}

// TestOpenAnchoredRejectsIntermediateSymlink verifies that intermediate symlinks are rejected.
// TestOpenAnchoredRejectsIntermediateSymlink 验证中间路径中的符号链接会被拒绝。
func TestOpenAnchoredRejectsIntermediateSymlink(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.MkdirAll(filepath.Join(realDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAnchoredDirFD(root, filepath.Join(link, "sub")); !errors.Is(err, ErrSymlink) {
		t.Fatalf("expected ErrSymlink; got %v", err)
	}
}

// TestOpenAnchoredRejectsIntermediateRegularFileAsNotDir verifies that a regular file in the path is rejected.
// TestOpenAnchoredRejectsIntermediateRegularFileAsNotDir 验证路径中的普通文件会被拒绝。
func TestOpenAnchoredRejectsIntermediateRegularFileAsNotDir(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAnchoredDirFD(root, filepath.Join(file, "sub")); !errors.Is(err, ErrNotDir) {
		t.Fatalf("expected ErrNotDir; got %v", err)
	}
}

// TestWalkSkipsDirectoryContainingGitWithoutSwallowingParentSiblings verifies .git directory skipping behavior.
// TestWalkSkipsDirectoryContainingGitWithoutSwalkingParentSiblings 验证包含 .git 的目录跳过行为。
func TestWalkSkipsDirectoryContainingGitWithoutSwallowingParentSiblings(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "cache.bin"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sibling.bin"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	w, err := OpenWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	var got []string
	if err := w.Walk(func(e Entry) {
		got = append(got, e.RelPath)
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "sibling.bin" {
		t.Fatalf("entries = %v, want only parent sibling", got)
	}
}
