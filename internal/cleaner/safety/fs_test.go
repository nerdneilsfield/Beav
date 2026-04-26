package safety

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
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
