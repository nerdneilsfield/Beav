package executor

import (
	"context"
	"os/exec"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
)

func TestJournalVacuumSkipsWhenJournalctlMissing(t *testing.T) {
	if _, err := exec.LookPath("journalctl"); err == nil {
		t.Skip("journalctl present; skipping negative test")
	}
	c := model.Cleaner{
		ID:         "j",
		Name:       "journal",
		Scope:      model.ScopeSystem,
		Type:       model.TypeJournalVacuum,
		MinAgeDays: ptrInt(7),
	}
	evs := captureEvents(t, func(emit func(model.Event)) {
		_ = NewJournalExecutor().Run(context.Background(), c, false, emit)
	})
	if !hasCleanerSkip(evs, "manager_not_installed") {
		t.Fatalf("expected cleaner_skipped/manager_not_installed; got %+v", evs)
	}
}
