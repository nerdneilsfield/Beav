package config

import (
	"bufio"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Defaults struct {
	MinAgeDays int    `yaml:"min_age_days"`
	Output     string `yaml:"output"`
}

type Override struct {
	Enabled    *bool `yaml:"enabled,omitempty"`
	MinAgeDays *int  `yaml:"min_age_days,omitempty"`
}

type Config struct {
	Dir          string
	Defaults     Defaults            `yaml:"defaults"`
	Overrides    map[string]Override `yaml:"overrides"`
	Whitelist    []string            `yaml:"whitelist"`
	whitelistTxt []string
}

func Load(dir string) (*Config, error) {
	home, _ := os.UserHomeDir()
	return LoadWithHome(dir, home)
}

func LoadWithHome(dir, home string) (*Config, error) {
	cfg := &Config{
		Dir:       dir,
		Defaults:  Defaults{MinAgeDays: 14, Output: "auto"},
		Overrides: map[string]Override{},
	}
	if data, err := os.ReadFile(filepath.Join(dir, "config.yaml")); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
		if cfg.Overrides == nil {
			cfg.Overrides = map[string]Override{}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if data, err := os.ReadFile(filepath.Join(dir, "whitelist.txt")); err == nil {
		s := bufio.NewScanner(strings.NewReader(string(data)))
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			expanded, err := expand(line, home)
			if err != nil {
				return nil, err
			}
			cfg.whitelistTxt = append(cfg.whitelistTxt, expanded)
		}
		if err := s.Err(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for i, p := range cfg.Whitelist {
		expanded, err := expand(p, home)
		if err != nil {
			return nil, err
		}
		cfg.Whitelist[i] = expanded
	}
	return cfg, nil
}

// MergedWhitelist returns the combined whitelist from both YAML config and whitelist.txt file.
// MergedWhitelist 返回来自 YAML 配置和 whitelist.txt 文件的合并白名单。
func (c *Config) MergedWhitelist() []string {
	out := make([]string, 0, len(c.Whitelist)+len(c.whitelistTxt))
	out = append(out, c.whitelistTxt...)
	out = append(out, c.Whitelist...)
	return out
}

// Save writes defaults, overrides, and the whitelist back to config.yaml.
// Save 将 defaults、overrides 与 whitelist 写回 config.yaml。
func (c *Config) Save() error {
	if err := os.MkdirAll(c.Dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(c.Dir, "config.yaml"), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := yaml.NewEncoder(f)
	defer func() { _ = enc.Close() }()
	enc.SetIndent(2)
	return enc.Encode(struct {
		Defaults  Defaults            `yaml:"defaults"`
		Overrides map[string]Override `yaml:"overrides,omitempty"`
		Whitelist []string            `yaml:"whitelist,omitempty"`
	}{c.Defaults, c.Overrides, c.Whitelist})
}

func expand(p, home string) (string, error) {
	if strings.HasPrefix(p, "~/") {
		if home != "" {
			return filepath.Join(home, p[2:]), nil
		}
		return "", errors.New("~/ whitelist entries require a target home")
	}
	return os.ExpandEnv(p), nil
}
