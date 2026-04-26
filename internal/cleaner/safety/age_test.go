package safety

import (
	"path/filepath"
	"testing"
	"time"
)

func TestAgePlanKeepsRecentSkipsDirsWithLiveChildren(t *testing.T) {
	root := t.TempDir()
	old := filepath.Join(root, "old")
	newDir := filepath.Join(root, "newdir")
	mustMkdirAll(t, old)
	mustMkdirAll(t, newDir)
	mustWriteAged(t, filepath.Join(old, "a"), -30*24*time.Hour)
	mustAgePath(t, old, -30*24*time.Hour)
	mustWriteAged(t, filepath.Join(newDir, "fresh"), -1*time.Hour)

	w, err := OpenWalker(root)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	plan := AgePlan(w, 7, TimeFieldMtime, time.Now(), false)
	if !plan.WillDelete(filepath.Join(root, "old")) {
		t.Error("old dir with old child should be planned for deletion")
	}
	if plan.WillDelete(filepath.Join(root, "newdir", "fresh")) {
		t.Error("fresh file should be skipped")
	}
	if plan.WillDelete(filepath.Join(root, "newdir")) {
		t.Error("dir with live child should not be deletable")
	}
}
