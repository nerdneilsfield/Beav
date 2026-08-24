package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

func (e *ToolPruneExecutor) probe(ctx context.Context, binary string, profile toolProfile, identity CommandIdentity, commandIdentity *CommandIdentity, env []string, dryRun bool) error {
	args := profile.ProbeArgs
	required := profile.Required
	versionWords := profile.VersionWords
	if dryRun {
		if len(profile.DryRunProbeArgs) > 0 {
			args = profile.DryRunProbeArgs
		}
		required = profile.DryRunRequired
		versionWords = profile.DryRunVersionWords
	}
	if len(args) == 0 {
		args = []string{"--version"}
	}
	var (
		linesMu sync.Mutex
		lines   []string
	)
	err := RunCommand(ctx, CommandRun{
		Executable: binary,
		Args:       args,
		Env:        env,
		Cwd:        identity.Home,
		Identity:   commandIdentity,
		Timeout:    e.probeTimeout(),
		OnStdout: func(line string) {
			linesMu.Lock()
			lines = append(lines, line)
			linesMu.Unlock()
		},
		OnStderr: func(line string) {
			linesMu.Lock()
			lines = append(lines, line)
			linesMu.Unlock()
		},
	})
	if err != nil {
		return fmt.Errorf("capability probe failed: %w", err)
	}
	joined := strings.TrimSpace(strings.Join(lines, "\n"))
	if joined == "" {
		return errors.New("capability probe produced empty output")
	}
	if len(versionWords) > 0 {
		lowerJoined := strings.ToLower(joined)
		matched := false
		for _, word := range versionWords {
			if strings.Contains(lowerJoined, strings.ToLower(word)) {
				matched = true
				break
			}
		}
		if !matched {
			return fmt.Errorf("%w: %s", errVersionMismatch, joined)
		}
	}
	if len(required) > 0 {
		for _, flag := range required {
			if !strings.Contains(joined, flag) {
				return fmt.Errorf("required capability %q unavailable", flag)
			}
		}
	}
	return nil
}

func (e *ToolPruneExecutor) probeTimeout() time.Duration {
	if e.Timeout > 0 && e.Timeout < 10*time.Second {
		return e.Timeout
	}
	return 10 * time.Second
}
