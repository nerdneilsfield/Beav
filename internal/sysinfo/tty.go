package sysinfo

import (
	"os"

	"github.com/mattn/go-isatty"
)

func IsTerminal(f *os.File) bool {
	return isatty.IsTerminal(f.Fd())
}
