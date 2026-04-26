package executor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dengqi/beav/internal/cleaner/model"
	"golang.org/x/sys/unix"
)

// ContainerTarget specifies the user context for container operations.
// ContainerTarget 指定容器操作的用户上下文。
type ContainerTarget struct {
	// UID is the user ID that owns the container daemon.
	// UID 是拥有容器守护进程的用户 ID。
	UID int
	// Home is the user's home directory.
	// Home 是用户的主目录。
	Home string
}

// ContainerExecutor implements the Executor interface for container pruning.
// ContainerExecutor 实现了用于容器清理的 Executor 接口。
type ContainerExecutor struct {
	target ContainerTarget
}

// NewContainerExecutor creates a new ContainerExecutor, optionally with a custom target.
// NewContainerExecutor 创建一个新的 ContainerExecutor，可选择自定义目标。
func NewContainerExecutor(target ...ContainerTarget) *ContainerExecutor {
	t := ContainerTarget{UID: os.Getuid(), Home: os.Getenv("HOME")}
	if len(target) > 0 {
		t = target[0]
	}
	return &ContainerExecutor{target: t}
}

func containerArgv(runtime, target string, ageDays int) ([]string, error) {
	if target == "volume" {
		return nil, fmt.Errorf("volume target not in v1")
	}
	until := "until=" + strconv.Itoa(ageDays*24) + "h"
	switch runtime {
	case "docker":
		switch target {
		case "builder", "container", "network":
			return []string{"docker", target, "prune", "-f", "--filter", until}, nil
		case "image":
			return []string{"docker", "image", "prune", "-af", "--filter", until}, nil
		case "system":
			return nil, fmt.Errorf("docker system target not in v1; use individual targets")
		}
	case "podman":
		switch target {
		case "builder", "container", "network":
			return []string{"podman", target, "prune", "-f", "--filter", until}, nil
		case "image":
			return []string{"podman", "image", "prune", "-af", "--filter", until}, nil
		case "system":
			return []string{"podman", "system", "prune", "-f", "--filter", until}, nil
		}
	}
	return nil, fmt.Errorf("unknown runtime/target %q/%q", runtime, target)
}

// Run executes the container pruning operation, emitting events for each action taken.
// Run 执行容器清理操作，为每个操作发出事件。
func (ce *ContainerExecutor) Run(ctx context.Context, c model.Cleaner, dryRun bool, emit func(model.Event)) error {
	start := time.Now()
	emit(model.Event{Event: model.EvStart, CleanerID: c.ID, Name: c.Name, Scope: c.Scope, Type: c.Type, DryRun: dryRun, TS: start})
	if c.ContainerPrune == nil || c.MinAgeDays == nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: "container_prune config missing", TS: time.Now()})
		emitFinish(emit, c.ID, "error", 1, start)
		return nil
	}

	runtime := c.ContainerPrune.Runtime
	target := c.ContainerPrune.Target
	if _, err := exec.LookPath(runtime); err != nil {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_unavailable", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}
	if !daemonReachable(ctx, runtime) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_unavailable", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}
	rootlessDaemon := daemonRootless(ctx, runtime)
	if c.Scope == model.ScopeUser && (!rootlessDaemon || !verifyRootlessSocket(ctx, runtime, ce.target.UID, ce.target.Home)) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_not_rootless", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}
	if c.Scope == model.ScopeSystem && rootlessDaemon {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_not_rootless", Detail: "system-scope cleaner detected rootless daemon", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}
	if (target == "builder" || target == "image") && runtimeBusy(ctx, runtime) {
		emit(model.Event{Event: model.EvCleanerSkipped, CleanerID: c.ID, Reason: "runtime_busy", TS: time.Now()})
		emitFinish(emit, c.ID, "skipped", 0, start)
		return nil
	}

	argv, err := containerArgv(runtime, target, *c.MinAgeDays)
	if err != nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "internal", Detail: err.Error(), TS: time.Now()})
		emitFinish(emit, c.ID, "error", 1, start)
		return nil
	}
	if dryRun {
		emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: "[dry-run] " + strings.Join(argv, " "), TS: time.Now()})
		emitFinish(emit, c.ID, "ok", 0, start)
		return nil
	}

	errs := 0
	err = RunCommand(ctx, CommandRun{
		Argv:    argv,
		Timeout: 5 * time.Minute,
		OnStdout: func(line string) {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stdout", Line: line, TS: time.Now()})
		},
		OnStderr: func(line string) {
			emit(model.Event{Event: model.EvCommandOutput, CleanerID: c.ID, Stream: "stderr", Line: line, TS: time.Now()})
		},
	})
	if err != nil {
		emit(model.Event{Event: model.EvError, CleanerID: c.ID, Reason: "command_failed", Detail: err.Error(), TS: time.Now()})
		errs++
	}
	status := "ok"
	if errs > 0 {
		status = "error"
	}
	emitFinish(emit, c.ID, status, errs, start)
	return nil
}

// daemonReachable checks if the container runtime daemon is responding.
// daemonReachable 检查容器运行时守护进程是否响应。
func daemonReachable(ctx context.Context, runtime string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, runtime, "info", "--format", "{{.ServerVersion}}").Run() == nil
}

// runtimeBusy checks if the container runtime has running containers.
// runtimeBusy 检查容器运行时是否有正在运行的容器。
func runtimeBusy(ctx context.Context, runtime string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, runtime, "ps", "-q", "--filter", "status=running").Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// daemonRootless checks if the container runtime is running in rootless mode.
// daemonRootless 检查容器运行时是否以无根模式运行。
func daemonRootless(ctx context.Context, runtime string) bool {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var format string
	switch runtime {
	case "docker":
		format = "{{.SecurityOptions}}"
	case "podman":
		format = "{{.Host.Security.Rootless}}"
	default:
		return false
	}
	out, err := exec.CommandContext(ctx, runtime, "info", "--format", format).Output()
	if err != nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(string(out)))
	if !strings.Contains(s, "rootless") && s != "true" {
		return false
	}
	return true
}

// verifyRootlessSocket verifies that the rootless socket is owned by the target user.
// verifyRootlessSocket 验证无根套接字是否由目标用户拥有。
func verifyRootlessSocket(ctx context.Context, runtime string, uid int, home string) bool {
	sock := socketPath(ctx, runtime, uid)
	if sock == "" {
		return false
	}
	var st unix.Stat_t
	if err := unix.Lstat(sock, &st); err != nil {
		return false
	}
	if int(st.Uid) != uid {
		return false
	}
	uidStr := strconv.Itoa(uid)
	for _, prefix := range []string{"/run/user/" + uidStr + "/", os.Getenv("XDG_RUNTIME_DIR") + "/", home + "/"} {
		if prefix != "/" && strings.HasPrefix(sock, prefix) {
			return true
		}
	}
	return false
}

// socketPath discovers the container runtime socket path for the given user.
// socketPath 发现给定用户的容器运行时套接字路径。
func socketPath(ctx context.Context, runtime string, uid int) string {
	if v := os.Getenv("DOCKER_HOST"); runtime == "docker" && strings.HasPrefix(v, "unix://") {
		return strings.TrimPrefix(v, "unix://")
	}
	switch runtime {
	case "docker":
		p := filepath.Join("/run/user", strconv.Itoa(uid), "docker.sock")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		out, err := exec.CommandContext(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}").Output()
		if err == nil {
			s := strings.TrimSpace(string(out))
			if strings.HasPrefix(s, "unix://") {
				return strings.TrimPrefix(s, "unix://")
			}
		}
	case "podman":
		out, err := exec.CommandContext(ctx, "podman", "info", "--format", "{{.Host.RemoteSocket.Path}}").Output()
		if err == nil {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}
