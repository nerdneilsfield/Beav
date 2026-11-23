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
	Dir             string
	Defaults        Defaults            `yaml:"defaults"`
	Overrides       map[string]Override `yaml:"overrides"`
	Whitelist       []string            `yaml:"whitelist"`
	rawWhitelist    []string
	hasRawWhitelist bool
	configExists    bool
	whitelistTxt    []string
	whitelistTxtRaw []string
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
		cfg.configExists = true
		cfg.rawWhitelist = append([]string(nil), cfg.Whitelist...)
		cfg.hasRawWhitelist = true
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
			cfg.whitelistTxtRaw = append(cfg.whitelistTxtRaw, line)
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

// ConfigPath returns the full path to config.yaml inside the config directory.
// ConfigPath 返回配置目录中 config.yaml 的完整路径。
func (c *Config) ConfigPath() string {
	return filepath.Join(c.Dir, "config.yaml")
}

// CleanersDir returns the directory holding user custom cleaner definitions.
// CleanersDir 返回存放用户自定义清理器定义的目录路径。
func (c *Config) CleanersDir() string {
	return filepath.Join(c.Dir, "cleaners.d")
}

// ConfigExists reports whether config.yaml existed when LoadWithHome ran.
// ConfigExists 返回加载配置时 config.yaml 是否已存在。
func (c *Config) ConfigExists() bool {
	return c.configExists
}

// RawWhitelist returns a copy of the unexpanded whitelist entries stored in config.yaml.
// RawWhitelist 返回保存在 config.yaml 中未展开的原始白名单条目副本。
func (c *Config) RawWhitelist() []string {
	if c.hasRawWhitelist {
		return append([]string(nil), c.rawWhitelist...)
	}
	return append([]string(nil), c.Whitelist...)
}

// WhitelistTxt returns a copy of the raw entries read from whitelist.txt.
// WhitelistTxt 返回从 whitelist.txt 读取的原始条目副本。
func (c *Config) WhitelistTxt() []string {
	return append([]string(nil), c.whitelistTxtRaw...)
}

// AddWhitelist validates and adds a path pattern to the config.yaml whitelist while preserving ~/ prefixes.
// Returning added=false with nil error means the entry was already present.
// AddWhitelist 校验并将路径模式添加到 config.yaml 白名单中（保留 ~/ 前缀）；
// 若条目已存在则返回 added=false 且无错误。
func (c *Config) AddWhitelist(raw, home string) (bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return false, errors.New("whitelist path cannot be empty")
	}
	expanded, err := expand(trimmed, home)
	if err != nil {
		return false, err
	}
	c.ensureRawWhitelistInitialized()
	cleaned := filepath.Clean(expanded)
	for i, existingRaw := range c.rawWhitelist {
		if existingRaw == trimmed || (i < len(c.Whitelist) && filepath.Clean(c.Whitelist[i]) == cleaned) {
			return false, nil
		}
	}
	c.rawWhitelist = append(c.rawWhitelist, trimmed)
	c.Whitelist = append(c.Whitelist, expanded)
	return true, nil
}

// RemoveWhitelist removes a matching entry from the config.yaml whitelist and reports
// whether it was removed and whether a matching entry also lives in whitelist.txt.
// RemoveWhitelist 从 config.yaml 白名单移除匹配条目，并返回是否移除及该条目是否同时存在于 whitelist.txt 中。
func (c *Config) RemoveWhitelist(entry, home string) (removed bool, inWhitelistTxt bool) {
	trimmed := strings.TrimSpace(entry)
	if trimmed == "" {
		return false, false
	}
	expanded, expErr := expand(trimmed, home)
	cleanedEntry := filepath.Clean(trimmed)
	cleanedExpanded := ""
	if expErr == nil {
		cleanedExpanded = filepath.Clean(expanded)
	}

	c.ensureRawWhitelistInitialized()
	nextRaw := make([]string, 0, len(c.rawWhitelist))
	nextExpanded := make([]string, 0, len(c.Whitelist))
	for i, raw := range c.rawWhitelist {
		var exp string
		if i < len(c.Whitelist) {
			exp = c.Whitelist[i]
		}
		match := raw == trimmed || filepath.Clean(raw) == cleanedEntry
		if !match && cleanedExpanded != "" && exp != "" {
			match = filepath.Clean(exp) == cleanedExpanded
		}
		if match {
			removed = true
			continue
		}
		nextRaw = append(nextRaw, raw)
		if i < len(c.Whitelist) {
			nextExpanded = append(nextExpanded, exp)
		}
	}
	if removed {
		c.rawWhitelist = nextRaw
		c.Whitelist = nextExpanded
	}

	for i, raw := range c.whitelistTxtRaw {
		if raw == trimmed || filepath.Clean(raw) == cleanedEntry {
			inWhitelistTxt = true
			break
		}
		if cleanedExpanded != "" && i < len(c.whitelistTxt) && filepath.Clean(c.whitelistTxt[i]) == cleanedExpanded {
			inWhitelistTxt = true
			break
		}
	}
	return removed, inWhitelistTxt
}

func (c *Config) ensureRawWhitelistInitialized() {
	if c.hasRawWhitelist {
		return
	}
	c.rawWhitelist = append([]string(nil), c.Whitelist...)
	c.hasRawWhitelist = true
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
	}{c.Defaults, c.Overrides, c.RawWhitelist()})
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
