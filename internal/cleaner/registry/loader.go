package registry

import (
	"bytes"
	"fmt"
	"io"
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
	return loadFSWithReservedIDs(os.DirFS(dir), ".", true)
}

// loadFS walks the filesystem and parses YAML files into Loaded cleaner definitions.
// loadFS 遍历文件系统并将 YAML 文件解析为 Loaded 清理器定义。
func loadFS(root fs.FS, base string) ([]Loaded, error) {
	return loadFSWithReservedIDs(root, base, false)
}

func loadFSWithReservedIDs(root fs.FS, base string, rejectReservedIDs bool) ([]Loaded, error) {
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
		if rejectReservedIDs {
			if err := rejectReservedCargoDocument(data, p); err != nil {
				return err
			}
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

// rejectReservedCargoDocument inspects original YAML nodes before strict
// model decoding. Only a direct scalar id: lang-cargo is authoritative;
// merged or aliased IDs remain subject to normal strict decoding errors.
func rejectReservedCargoDocument(data []byte, path string) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	found := false
	for {
		var document yaml.Node
		err := decoder.Decode(&document)
		if err == io.EOF {
			if found {
				return cargoMigrationError(path)
			}
			return nil
		}
		if err != nil {
			// Let strict decode report malformed YAML; do not misclassify it as
			// a Cargo migration when the document cannot be trusted.
			return nil
		}
		if explicitCargoID(documentNode(document)) {
			found = true
		}
	}
}

func documentNode(document yaml.Node) *yaml.Node {
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil
	}
	return document.Content[0]
}

func explicitCargoID(node *yaml.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case yaml.SequenceNode:
		for _, item := range node.Content {
			if item.Kind == yaml.MappingNode && mappingHasExplicitCargoID(item) {
				return true
			}
		}
	case yaml.MappingNode:
		return mappingHasExplicitCargoID(node)
	}
	return false
}

func mappingHasExplicitCargoID(node *yaml.Node) bool {
	for i := 0; i+1 < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		if key.Kind == yaml.ScalarNode && key.Value == "id" && value.Kind == yaml.ScalarNode && value.Value == "lang-cargo" {
			return true
		}
	}
	return false
}

func cargoMigrationError(path string) error {
	return fmt.Errorf("%s: cleaner %q is reserved; migration required: use Cargo native GC instead of a Beav Cargo cleaner", path, "lang-cargo")
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
