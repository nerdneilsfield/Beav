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
	Enabled    *bool `yaml:"enabled"`
	MinAgeDays *int  `yaml:"min_age_days"`
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
			cfg.whitelistTxt = append(cfg.whitelistTxt, expand(line, home))
		}
		if err := s.Err(); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	for i, p := range cfg.Whitelist {
		cfg.Whitelist[i] = expand(p, home)
	}
	return cfg, nil
}

func (c *Config) MergedWhitelist() []string {
	out := make([]string, 0, len(c.Whitelist)+len(c.whitelistTxt))
	out = append(out, c.whitelistTxt...)
	out = append(out, c.Whitelist...)
	return out
}

func expand(p, home string) string {
	if strings.HasPrefix(p, "~/") {
		if home != "" {
			return filepath.Join(home, p[2:])
		}
	}
	return os.ExpandEnv(p)
}
