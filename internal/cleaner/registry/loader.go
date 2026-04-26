package registry

import (
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
		if err := yaml.Unmarshal(data, &list); err == nil {
			for _, c := range list {
				if c.ID != "" {
					out = append(out, Loaded{From: p, Cleaner: c})
				}
			}
			return nil
		}

		var c model.Cleaner
		if err := yaml.Unmarshal(data, &c); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
		if c.ID == "" {
			return nil
		}
		out = append(out, Loaded{From: p, Cleaner: c})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
