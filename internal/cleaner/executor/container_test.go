package executor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestArgvForTarget(t *testing.T) {
	cases := []struct {
		runtime string
		target  string
		want    []string
	}{
		{"docker", "builder", []string{"docker", "builder", "prune", "-f", "--filter", "until=336h"}},
		{"docker", "image", []string{"docker", "image", "prune", "-af", "--filter", "until=336h"}},
		{"docker", "container", []string{"docker", "container", "prune", "-f", "--filter", "until=336h"}},
		{"docker", "network", []string{"docker", "network", "prune", "-f", "--filter", "until=336h"}},
		{"podman", "system", []string{"podman", "system", "prune", "-f", "--filter", "until=336h"}},
	}
	for _, tc := range cases {
		got, err := containerArgv(tc.runtime, tc.target, 14)
		if err != nil {
			t.Fatalf("%+v: %v", tc, err)
		}
		if !equalStrings(got, tc.want) {
			t.Errorf("argv mismatch: got %v want %v", got, tc.want)
		}
	}
}

func TestVolumeTargetRejected(t *testing.T) {
	if _, err := containerArgv("docker", "volume", 14); err == nil {
		t.Fatal("volume target must be rejected")
	}
}

func TestSkipWhenRuntimeMissing(t *testing.T) {
	if _, err := exec.LookPath("docker"); err == nil {
		t.Skip("docker present; skipping negative test")
	}
	c := model.Cleaner{
		ID:             "c",
		Name:           "c",
		Scope:          model.ScopeSystem,
		Type:           model.TypeContainerPrune,
		MinAgeDays:     ptrInt(14),
		ContainerPrune: &model.ContainerPruneCfg{Runtime: "docker", Target: "builder"},
	}
	evs := captureEvents(t, func(emit func(model.Event)) {
		_ = NewContainerExecutor().Run(testContext(t), c, false, emit)
	})
	if !hasCleanerSkip(evs, "runtime_unavailable") {
		t.Fatalf("expected runtime_unavailable skip; got %+v", evs)
	}
}

func TestSystemScopeSkipsRootlessDaemonEvenWithoutVerifiedSocket(t *testing.T) {
	dir := t.TempDir()
	docker := filepath.Join(dir, "docker")
	if err := os.WriteFile(docker, []byte("#!/bin/sh\nif [ \"$1\" = info ]; then echo rootless; exit 0; fi\nexit 0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(docker, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_HOST", "unix:///not/in/an/allowed/rootless/location.sock")

	c := model.Cleaner{
		ID:             "c",
		Name:           "c",
		Scope:          model.ScopeSystem,
		Type:           model.TypeContainerPrune,
		MinAgeDays:     ptrInt(14),
		ContainerPrune: &model.ContainerPruneCfg{Runtime: "docker", Target: "builder"},
	}
	evs := captureEvents(t, func(emit func(model.Event)) {
		_ = NewContainerExecutor().Run(testContext(t), c, false, emit)
	})
	if !hasCleanerSkip(evs, "runtime_not_rootless") {
		t.Fatalf("expected rootless system skip; got %+v", evs)
	}
}

func TestRootlessSocketVerificationUsesTargetUserAndHome(t *testing.T) {
	home := t.TempDir()
	sock := filepath.Join(home, "docker.sock")
	if err := os.WriteFile(sock, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DOCKER_HOST", "unix://"+sock)
	if !verifyRootlessSocket(testContext(t), "docker", os.Getuid(), home) {
		t.Fatal("expected target-owned socket under target home to verify")
	}
}

func TestDockerContextSocketDiscovery(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	sock := filepath.Join(home, "context.sock")
	if err := os.WriteFile(sock, []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	docker := filepath.Join(dir, "docker")
	script := "#!/bin/sh\nif [ \"$1\" = context ]; then echo unix://" + sock + "; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(docker, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(docker, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DOCKER_HOST", "")
	got := socketPath(testContext(t), "docker", 424242)
	if got != sock {
		t.Fatalf("socketPath = %q, want %q", got, sock)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
