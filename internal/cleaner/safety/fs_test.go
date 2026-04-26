package safety

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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

func TestOpenAnchoredRejectsIntermediateSymlink(t *testing.T) {
	root := t.TempDir()
	real := filepath.Join(root, "real")
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenAnchoredDirFD(root, filepath.Join(link, "sub")); !errors.Is(err, ErrSymlink) {
		t.Fatalf("expected ErrSymlink; got %v", err)
	}
}
