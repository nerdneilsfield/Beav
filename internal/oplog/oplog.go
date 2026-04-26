package oplog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Logger struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	maxFiles int
	f        *os.File
	written  int64
}

func New(path string, maxBytes int64, maxFiles int) (*Logger, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return &Logger{path: path, maxBytes: maxBytes, maxFiles: maxFiles, f: f, written: st.Size()}, nil
}

func (l *Logger) Write(op, path string, size int64, cleaner string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.maxBytes > 0 && l.written >= l.maxBytes {
		if err := l.rotate(); err != nil {
			return err
		}
	}
	line := fmt.Sprintf("%s\t%s\t%s\t%d\t%s\n", time.Now().UTC().Format(time.RFC3339), op, path, size, cleaner)
	n, err := l.f.WriteString(line)
	l.written += int64(n)
	return err
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

func (l *Logger) rotate() error {
	if l.f != nil {
		_ = l.f.Close()
	}
	if l.maxFiles < 1 {
		_ = os.Remove(l.path)
	} else {
		for i := l.maxFiles - 1; i >= 1; i-- {
			old := fmt.Sprintf("%s.%d", l.path, i)
			newer := fmt.Sprintf("%s.%d", l.path, i+1)
			_ = os.Rename(old, newer)
		}
		_ = os.Rename(l.path, l.path+".1")
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	l.f = f
	l.written = 0
	return nil
}
