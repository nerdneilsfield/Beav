// Package oplog provides a rotating log for recording file operations during cache cleanup.
// Package oplog 提供一个循环日志，用于记录缓存清理期间的文件操作。
package oplog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger writes operation logs to a file with automatic rotation when size limits are reached.
// Logger 将操作日志写入文件，在达到大小限制时自动轮转。
type Logger struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	maxFiles int
	f        *os.File
	written  int64
}

// New creates a new Logger that writes to the given path with rotation limits.
// New 创建一个新的 Logger，写入给定路径并带有轮转限制。
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

// rotate renames the current log file and creates a new one, keeping up to maxFiles backups.
// rotate 重命名当前日志文件并创建新文件，保留最多 maxFiles 个备份。
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
