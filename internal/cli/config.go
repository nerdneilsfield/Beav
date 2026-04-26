// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/config"
	"github.com/spf13/cobra"
)

// NewConfigCmd creates a command for inspecting or editing beav config.
// NewConfigCmd 创建一个用于检查或编辑 beav 配置的命令。
func NewConfigCmd() *cobra.Command {
	var dir string
	var force bool
	cmd := &cobra.Command{Use: "config", Short: "Inspect or edit beav config"}
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter config.yaml",
		RunE: func(_ *cobra.Command, _ []string) error {
			return runConfigInit(resolveConfigDir(dir), force)
		},
	}
	initCmd.Flags().StringVar(&dir, "config-dir", "", "override config directory")
	initCmd.Flags().BoolVar(&force, "force", false, "overwrite an existing config.yaml")

	show := &cobra.Command{
		Use:   "show",
		Short: "Show effective config and cleaner list",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigShow(cmd, dir)
		},
	}
	show.Flags().StringVar(&dir, "config-dir", "", "override config directory")

	edit := &cobra.Command{
		Use:   "edit",
		Short: "Open config.yaml in $EDITOR",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfgDir := resolveConfigDir(dir)
			if err := os.MkdirAll(cfgDir, 0o755); err != nil {
				return err
			}
			path := filepath.Join(cfgDir, "config.yaml")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := writeDefaultConfig(path, false); err != nil {
					return err
				}
			}
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vi"
			}
			c := exec.Command(editor, path)
			c.Stdin = os.Stdin
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.ErrOrStderr()
			return c.Run()
		},
	}
	edit.Flags().StringVar(&dir, "config-dir", "", "override config directory")

	cmd.AddCommand(initCmd, show, edit)
	return cmd
}

// runConfigInit creates a default config file in the specified directory.
// runConfigInit 在指定目录中创建默认配置文件。
func runConfigInit(cfgDir string, force bool) error {
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	return writeDefaultConfig(filepath.Join(cfgDir, "config.yaml"), force)
}

// writeDefaultConfig writes a default config.yaml to the given path.
// writeDefaultConfig 将默认 config.yaml 写入给定路径。
func writeDefaultConfig(path string, force bool) error {
	flag := os.O_WRONLY | os.O_CREATE
	if force {
		flag |= os.O_TRUNC
	} else {
		flag |= os.O_EXCL
	}
	f, err := os.OpenFile(path, flag, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.WriteString("defaults:\n  min_age_days: 14\n  output: auto\n")
	return err
}

// runConfigShow displays the effective config and cleaner list as JSON.
// runConfigShow 以 JSON 格式显示有效配置和清理器列表。
func runConfigShow(cmd *cobra.Command, dir string) error {
	cfgDir := resolveConfigDir(dir)
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return err
	}
	builtin, err := registry.LoadBuiltin(cleaners.Builtin)
	if err != nil {
		return err
	}
	user, err := registry.LoadUserDir(filepath.Join(cfgDir, "cleaners.d"))
	if err != nil {
		return err
	}
	out := map[string]any{
		"config":   cfg,
		"cleaners": nil,
	}
	merged, err := registry.MergeByID(builtin, user)
	if err != nil {
		return err
	}
	out["cleaners"] = merged
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// resolveConfigDir returns the config directory, using the override if provided.
// resolveConfigDir 返回配置目录，如果提供了覆盖则使用覆盖值。
func resolveConfigDir(dir string) string {
	if dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "beav")
}
