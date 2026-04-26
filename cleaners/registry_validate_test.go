package cleaners_test

import (
	"testing"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/registry"
)

func TestEmbeddedRegistryValidates(t *testing.T) {
	loaded, err := registry.LoadBuiltin(cleaners.Builtin)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) == 0 {
		t.Fatal("no embedded cleaners")
	}
	for _, l := range loaded {
		if err := registry.Validate(l.Cleaner); err != nil {
			t.Errorf("%s: %v", l.From, err)
		}
	}
}
