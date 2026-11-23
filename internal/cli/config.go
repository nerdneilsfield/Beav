// Package cli provides the command-line interface for the beav tool.
// Package cli 提供 beav 工具的命令行界面。
package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

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
	var showJSON bool
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
		Short: "List cleaners with their IDs and state",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runConfigShow(cmd, dir, showJSON)
		},
	}
	show.Flags().StringVar(&dir, "config-dir", "", "override config directory")
	show.Flags().BoolVar(&showJSON, "json", false, "machine-readable JSON output")

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
			c := exec.Command(editor, path) //nolint:gosec // EDITOR deliberately selects the user's executable; no shell is involved.
			c.Stdin = os.Stdin
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.ErrOrStderr()
			return c.Run()
		},
	}
	edit.Flags().StringVar(&dir, "config-dir", "", "override config directory")

	enable := &cobra.Command{
		Use:   "enable <cleaner-id>",
		Short: "Enable a cleaner in every future run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSetEnabled(resolveConfigDir(dir), args[0], true, cmd.OutOrStdout())
		},
	}
	enable.Flags().StringVar(&dir, "config-dir", "", "override config directory")

	disable := &cobra.Command{
		Use:   "disable <cleaner-id>",
		Short: "Disable a cleaner in every future run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigSetEnabled(resolveConfigDir(dir), args[0], false, cmd.OutOrStdout())
		},
	}
	disable.Flags().StringVar(&dir, "config-dir", "", "override config directory")

	cmd.AddCommand(initCmd, show, edit, enable, disable)
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

// runConfigShow lists cleaners in a human table, or the full merged config as
// JSON when --json is set.
// runConfigShow 以人类可读表格列出清理器；指定 --json 时输出完整合并配置 JSON。
func runConfigShow(cmd *cobra.Command, dir string, showJSON bool) error {
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
	merged, err := registry.MergeByID(builtin, user)
	if err != nil {
		return err
	}
	if showJSON {
		out := map[string]any{"config": cfg, "cleaners": merged}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tNAME\tSCOPE\tTYPE\tAGE\tTAGS\tSTATE")
	for _, c := range merged {
		age := "-"
		if c.MinAgeDays != nil {
			age = strconv.Itoa(*c.MinAgeDays) + "d"
		}
		state := "enabled"
		if ov, ok := cfg.Overrides[c.ID]; ok && ov.Enabled != nil {
			state = boolState(*ov.Enabled)
		} else if c.Enabled != nil {
			state = boolState(*c.Enabled)
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", c.ID, c.Name, c.Scope, c.Type, age, strings.Join(c.Tags, ","), state)
	}
	return w.Flush()
}

func boolState(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

// runConfigSetEnabled persists an enable/disable override for one cleaner.
// runConfigSetEnabled 为单个清理器持久化启用或禁用覆盖。
func runConfigSetEnabled(cfgDir, id string, enabled bool, stdout io.Writer) error {
	builtin, err := registry.LoadBuiltin(cleaners.Builtin)
	if err != nil {
		return err
	}
	user, err := registry.LoadUserDir(filepath.Join(cfgDir, "cleaners.d"))
	if err != nil {
		return err
	}
	merged, err := registry.MergeByID(builtin, user)
	if err != nil {
		return err
	}
	known := false
	for _, c := range merged {
		if c.ID == id {
			known = true
			break
		}
	}
	if !known {
		return fmt.Errorf("unknown cleaner id %q; run `beav config show` to list all IDs", id)
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return err
	}
	ov := cfg.Overrides[id]
	ov.Enabled = &enabled
	cfg.Overrides[id] = ov
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "%s is now %s by default\n", id, boolState(enabled))
	return nil
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
