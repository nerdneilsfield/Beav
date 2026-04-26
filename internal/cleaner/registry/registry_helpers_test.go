package registry

import (
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
	"gopkg.in/yaml.v3"
)

func mustParse(t *testing.T, src string) model.Cleaner {
	t.Helper()
	var c model.Cleaner
	if err := yaml.Unmarshal([]byte(src), &c); err != nil {
		t.Fatal(err)
	}
	return c
}
