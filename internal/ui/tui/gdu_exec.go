package tui

import (
	"os"
	"os/exec"
)

// newGDUCmdProxy runs the current beav binary in `analyze` mode so gdu uses the
// existing terminal without fighting Bubble Tea's alt-screen.
func newGDUCmdProxy(path string, allowDelete bool) *exec.Cmd {
	exe, err := os.Executable()
	if err != nil || exe == "" {
		exe = "beav"
	}
	args := []string{"analyze", path}
	if allowDelete {
		args = append(args, "--allow-delete")
	}
	return exec.Command(exe, args...) // #nosec G204 -- invokes self binary with explicit analyze subcommand.
}
