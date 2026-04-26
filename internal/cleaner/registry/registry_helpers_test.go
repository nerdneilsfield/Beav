package registry

import (
	"testing"

	"github.com/dengqi/beav/internal/cleaner/model"
	"gopkg.in/yaml.v3"
)

// mustParse is a test helper that parses a YAML string into a model.Cleaner, failing the test on error.
// mustParse 是一个测试辅助函数，将 YAML 字符串解析为 model.Cleaner，出错时使测试失败。
func mustParse(t *testing.T, src string) model.Cleaner {
	t.Helper()
	var c model.Cleaner
	if err := yaml.Unmarshal([]byte(src), &c); err != nil {
		t.Fatal(err)
	}
	return c
}
