package cli

import (
	"bytes"
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
	uicli "github.com/dengqi/beav/internal/ui/cli"
)

func TestCleanDryRunNoCleanersExitsZero(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCmd("test", "test", "test")
	cmd.SetArgs([]string{"clean", "--dry-run", "--output", "json", "--config-dir", t.TempDir(), "--builtin-disabled", "--allow-root-home"})
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("err: %v out=%q", err, out.String())
	}
}

func TestChooseRendererAutoUsesPlainForNonTTY(t *testing.T) {
	var out bytes.Buffer
	if _, ok := chooseRenderer("", "auto", &out).(*uicli.Plain); !ok {
		t.Fatal("auto output on non-TTY should use plain renderer")
	}
	if _, ok := chooseRenderer("", "spinner", &out).(*uicli.Spinner); !ok {
		t.Fatal("explicit config spinner should use spinner renderer")
	}
}

func TestValidateCleanersForRunSkipsWrongScopeBeforePathValidation(t *testing.T) {
	cleaners := []model.Cleaner{
		{
			ID:    "browser-firefox",
			Scope: model.ScopeUser,
			Type:  model.TypePaths,
			Paths: []string{"~/.cache/mozilla/firefox/*/cache2/**"},
		},
		{
			ID:    "system-cache",
			Scope: model.ScopeSystem,
			Type:  model.TypePaths,
			Paths: []string{"/tmp/beav-system-cache-*"},
		},
	}

	if err := validateCleanersForRun(cleaners, model.ScopeSystem, "", CleanFlags{}); err != nil {
		t.Fatal(err)
	}
}
