package executor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

// TestPkgCacheSkipsWhenManagerMissing verifies that the cleaner skips when the package manager is not installed.
// TestPkgCacheSkipsWhenManagerMissing 验证当包管理器未安装时清理器会跳过。
func TestPkgCacheSkipsWhenManagerMissing(t *testing.T) {
	mgr := "imaginarypkg"
	if _, err := exec.LookPath(mgr); err == nil {
		t.Skip("test package manager unexpectedly present")
	}
	c := model.Cleaner{
		ID:       "p",
		Name:     "p",
		Scope:    model.ScopeSystem,
		Type:     model.TypePkgCache,
		PkgCache: &model.PkgCacheCfg{Manager: mgr},
	}
	evs := captureEvents(t, func(emit func(model.Event)) {
		_ = NewPkgCacheExecutor().Run(context.Background(), c, false, emit)
	})
	if !hasCleanerSkip(evs, "manager_not_installed") {
		t.Fatalf("expected manager_not_installed skip; got %+v", evs)
	}
}

func TestPkgCacheDryRunMarksMetricsIncomplete(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "apt-get")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	c := model.Cleaner{ID: "apt", Scope: model.ScopeSystem, Type: model.TypePkgCache, PkgCache: &model.PkgCacheCfg{Manager: "apt"}}
	evs := captureEvents(t, func(emit func(model.Event)) {
		if err := NewPkgCacheExecutor().Run(context.Background(), c, true, emit); err != nil {
			t.Fatal(err)
		}
	})
	for _, ev := range evs {
		if ev.Event == model.EvFinish {
			if !ev.MetricsIncomplete {
				t.Fatalf("finish metrics incomplete = false: %+v", ev)
			}
			return
		}
	}
	t.Fatal("missing finish event")
}

func TestPkgCacheRealRunMarksMetricsIncompleteAndUsesSingleAptClean(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "apt.log")
	bin := filepath.Join(dir, "apt-get")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho \"$*\" >> "+logFile+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	c := model.Cleaner{ID: "pkg-apt-archives", Scope: model.ScopeSystem, Type: model.TypePkgCache, PkgCache: &model.PkgCacheCfg{Manager: "apt"}}
	evs := captureEvents(t, func(emit func(model.Event)) {
		if err := NewPkgCacheExecutor().Run(context.Background(), c, false, emit); err != nil {
			t.Fatal(err)
		}
	})
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(data)) != "clean" {
		t.Fatalf("apt invocations = %q, want single 'clean'", string(data))
	}
	for _, ev := range evs {
		if ev.Event == model.EvFinish {
			if !ev.MetricsIncomplete || ev.Status != "ok" {
				t.Fatalf("real pkg_cache finish must report ok + MetricsIncomplete=true: %+v", ev)
			}
			return
		}
	}
	t.Fatal("missing finish event")
}
