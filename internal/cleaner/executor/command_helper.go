package executor

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// commandHelperEnv is intentionally private to this package. RunCommand
// starts the current executable in this mode so the child can establish its
// privilege boundary before execve; no external command can select this mode
// through a profile or YAML value.
const commandHelperEnv = "BEAV_INTERNAL_COMMAND_HELPER_9F1E2C"

type commandHelperPayload struct {
	Executable string           `json:"executable"`
	Argv       []string         `json:"argv"`
	Env        []string         `json:"env"`
	Cwd        string           `json:"cwd"`
	Identity   *CommandIdentity `json:"identity,omitempty"`
}

// init handles only payloads produced by RunCommand. It runs before main in
// both the CLI binary and Go test binaries, which keeps no_new_privs and
// ambient-capability setup in the child immediately before execve.
func init() {
	if os.Getenv(commandHelperEnv) != "1" {
		return
	}
	if err := runCommandHelper(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(127)
	}
	os.Exit(127)
}

func runCommandHelper() error {
	payloadFile := os.NewFile(uintptr(3), "command-helper-payload")
	if payloadFile == nil {
		return errors.New("command helper payload fd unavailable")
	}
	defer payloadFile.Close()
	payload, err := io.ReadAll(payloadFile)
	if err != nil {
		return fmt.Errorf("read command helper payload: %w", err)
	}
	var command commandHelperPayload
	if err := json.Unmarshal(payload, &command); err != nil {
		return fmt.Errorf("decode command helper payload: %w", err)
	}
	if command.Executable == "" || len(command.Argv) == 0 || command.Argv[0] != command.Executable {
		return errors.New("invalid command helper executable")
	}
	if command.Identity != nil {
		if err := applyCommandIdentity(*command.Identity); err != nil {
			return err
		}
	}
	if err := clearAmbientCapabilities(); err != nil {
		return err
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return fmt.Errorf("set no_new_privs: %w", err)
	}
	if command.Cwd != "" {
		if err := unix.Chdir(command.Cwd); err != nil {
			return fmt.Errorf("change child directory: %w", err)
		}
	}
	if command.Env == nil {
		command.Env = os.Environ()
	}
	if err := unix.Exec(command.Executable, command.Argv, command.Env); err != nil {
		return fmt.Errorf("exec %q: %w", command.Executable, err)
	}
	return errors.New("exec returned unexpectedly")
}

func applyCommandIdentity(identity CommandIdentity) error {
	groups := make([]int, len(identity.Groups))
	for i, gid := range identity.Groups {
		if uint64(gid) > uint64(^uint(0)>>1) {
			return fmt.Errorf("supplementary GID %d overflows int", gid)
		}
		groups[i] = int(gid)
	}
	if err := unix.Setgroups(groups); err != nil {
		return fmt.Errorf("set supplementary groups: %w", err)
	}
	if err := unix.Setgid(int(identity.GID)); err != nil {
		return fmt.Errorf("set primary GID %d: %w", identity.GID, err)
	}
	if err := unix.Setuid(int(identity.UID)); err != nil {
		return fmt.Errorf("set UID %d: %w", identity.UID, err)
	}
	return nil
}

func clearAmbientCapabilities() error {
	if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); err != nil {
		return fmt.Errorf("clear ambient capabilities: %w", err)
	}
	return nil
}
