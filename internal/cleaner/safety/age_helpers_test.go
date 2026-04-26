package safety

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustMkdirAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWriteAged(t *testing.T, p string, age time.Duration) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(age)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
}

func mustAgePath(t *testing.T, p string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(age)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
}
