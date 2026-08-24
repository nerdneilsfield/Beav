package registry

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"strings"

	"github.com/dengqi/beav/internal/cleaner/model"
	"gopkg.in/yaml.v3"
)

type Loaded struct {
	From    string
	Cleaner model.Cleaner
}

func LoadBuiltin(root fs.FS) ([]Loaded, error) {
	return loadFS(root, ".")
}

func LoadUserDir(dir string) ([]Loaded, error) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return loadFS(os.DirFS(dir), ".")
}

// loadFS walks the filesystem and parses YAML files into Loaded cleaner definitions.
// loadFS 遍历文件系统并将 YAML 文件解析为 Loaded 清理器定义。
func loadFS(root fs.FS, base string) ([]Loaded, error) {
	var out []Loaded
	err := fs.WalkDir(root, base, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, ".yaml") {
			return nil
		}
		data, err := fs.ReadFile(root, p)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(string(data))) == 0 {
			return nil
		}

		var list []model.Cleaner
		if err := decodeYAML(data, &list); err == nil {
			for _, c := range list {
				if c.ID != "" {
					if err := normalizeAndValidate(&c); err != nil {
						return fmt.Errorf("%s: %w", p, err)
					}
					out = append(out, Loaded{From: p, Cleaner: c})
				}
			}
			return nil
		}

		var c model.Cleaner
		if err := decodeYAML(data, &c); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if c.ID == "" {
			return nil
		}
		if err := normalizeAndValidate(&c); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, Loaded{From: p, Cleaner: c})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func decodeYAML(data []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	return decoder.Decode(out)
}

// normalizeAndValidate applies compatibility defaults and validates structural
// fields before a cleaner can enter the registry. Mutable user override files
// may contain only an ID plus override fields; those are validated after merge.
func normalizeAndValidate(c *model.Cleaner) error {
	if c.Impact == "" {
		c.Impact = model.ImpactSafe
	}
	if c.Type == "" {
		if c.Impact != model.ImpactSafe && c.Impact != model.ImpactHeavy {
			return fmt.Errorf("cleaner %q: invalid impact %q", c.ID, c.Impact)
		}
		if c.ToolPrune != nil && !model.KnownProfile(model.TypeToolPrune, c.ToolPrune.Profile) {
			return fmt.Errorf("cleaner %q: unknown tool_prune profile %q", c.ID, c.ToolPrune.Profile)
		}
		if c.Debris != nil && !model.KnownProfile(model.TypeDebris, c.Debris.Profile) {
			return fmt.Errorf("cleaner %q: unknown debris profile %q", c.ID, c.Debris.Profile)
		}
		if c.ManagedCache != nil && !model.KnownProfile(model.TypeManagedCache, c.ManagedCache.Profile) {
			return fmt.Errorf("cleaner %q: unknown managed_cache profile %q", c.ID, c.ManagedCache.Profile)
		}
		return nil
	}
	return Validate(*c)
}
