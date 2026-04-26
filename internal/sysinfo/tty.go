package sysinfo

import (
	"os"

	"github.com/mattn/go-isatty"
)

// IsTerminal checks if the given file descriptor is connected to a terminal.
// IsTerminal 检查给定的文件描述符是否连接到终端。
func IsTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd())
}
