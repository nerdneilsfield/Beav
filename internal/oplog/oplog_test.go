package oplog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotates(t *testing.T) {
	dir := t.TempDir()
	o, err := New(filepath.Join(dir, "operations.log"), 64, 3)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := o.Write("delete", "/x", 1234, "demo"); err != nil {
			t.Fatal(err)
		}
	}
	if err := o.Close(); err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) > 4 {
		t.Errorf("too many rotated files: %d", len(files))
	}
}
