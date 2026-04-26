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

func NewConfigCmd() *cobra.Command {
	var dir string
	var force bool
	cmd := &cobra.Command{Use: "config", Short: "Inspect or edit beav config"}
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create a starter config.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
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

func runConfigInit(cfgDir string, force bool) error {
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	return writeDefaultConfig(filepath.Join(cfgDir, "config.yaml"), force)
}

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

func resolveConfigDir(dir string) string {
	if dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "beav")
}
