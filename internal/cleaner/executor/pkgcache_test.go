package executor

import (
	"context"
	"os/exec"
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
