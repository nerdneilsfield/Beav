package safety

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mustMkdirAll creates a directory and all parent directories, failing the test on error.
// mustMkdirAll 创建目录及所有父目录，如果出错则使测试失败。
func mustMkdirAll(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

// mustWriteAged creates a file with the specified age by adjusting its timestamps.
// mustWriteAged 通过调整时间戳创建具有指定年龄的文件。
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

// mustAgePath adjusts the timestamps of an existing path to simulate aging.
// mustAgePath 调整现有路径的时间戳以模拟老化。
func mustAgePath(t *testing.T, p string, age time.Duration) {
	t.Helper()
	when := time.Now().Add(age)
	if err := os.Chtimes(p, when, when); err != nil {
		t.Fatal(err)
	}
}
