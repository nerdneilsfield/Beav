package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/spf13/cobra"
)

// NewConfigCmd creates a command for managing beav configuration and whitelists.
// NewConfigCmd 创建一个用于管理 beav 配置与白名单的命令。
func NewConfigCmd(tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	var dir string
	var force bool
	var showJSON bool

	cmd := &cobra.Command{
		Use:   "config",
		Short: tr.Text("Inspect or manage beav configuration and whitelist", "查看或管理 beav 配置文件与白名单"),
		Long: tr.Text(
			"Manage ~/.config/beav/config.yaml, per-cleaner enable/disable overrides, and protected whitelist paths.\n"+
				"To browse all available cleaners and IDs, use `beav list` or `beav info <id>`.",
			"管理 ~/.config/beav/config.yaml 配置、单个清理项的启用/禁用规则及白名单保护路径。\n"+
				"若要浏览全部清理项及其 ID，请使用 `beav list` 或 `beav info <ID>`。",
		),
	}
	cmd.PersistentFlags().StringVar(&dir, "config-dir", "", tr.Text("override config directory", "覆盖默认配置目录"))

	initCmd := &cobra.Command{
		Use:   "init",
		Short: tr.Text("Create a commented starter config.yaml", "创建带注释说明的初始 config.yaml"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigInit(resolveConfigDir(dir), force, cmd.OutOrStdout(), localTr)
		},
	}
	initCmd.Flags().BoolVar(&force, "force", false, tr.Text("overwrite an existing config.yaml", "强制覆盖已存在的 config.yaml"))

	show := &cobra.Command{
		Use:   "show",
		Short: tr.Text("Show config file path, defaults, overrides, and whitelist", "显示配置文件路径、默认值、覆盖规则与白名单"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigShow(cmd, dir, showJSON, localTr)
		},
	}
	show.Flags().BoolVar(&showJSON, "json", false, tr.Text("machine-readable JSON output", "输出机器可读 JSON"))

	edit := &cobra.Command{
		Use:   "edit",
		Short: tr.Text("Open config.yaml in $VISUAL or $EDITOR", "使用 $VISUAL 或 $EDITOR 打开 config.yaml"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			cfgDir := resolveConfigDir(dir)
			if err := os.MkdirAll(cfgDir, 0o755); err != nil {
				return err
			}
			path := filepath.Join(cfgDir, "config.yaml")
			if _, err := os.Stat(path); os.IsNotExist(err) {
				if err := writeDefaultConfig(path, false, localTr); err != nil {
					return err
				}
			}
			editor := strings.TrimSpace(os.Getenv("VISUAL"))
			if editor == "" {
				editor = strings.TrimSpace(os.Getenv("EDITOR"))
			}
			if editor == "" {
				editor = "vi"
			}
			parts := strings.Fields(editor)
			if len(parts) == 0 {
				parts = []string{"vi"}
			}
			args := append(parts[1:], path)
			c := exec.Command(parts[0], args...) //nolint:gosec // VISUAL/EDITOR intentionally selects the user's editor command.
			c.Stdin = os.Stdin
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.ErrOrStderr()
			return c.Run()
		},
	}

	completeCleanerIDs := func(_ *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		_, effective, _, err := loadCatalog(dir, CleanFlags{}, tr)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var ids []string
		for _, c := range effective {
			if strings.HasPrefix(c.ID, toComplete) {
				ids = append(ids, c.ID)
			}
		}
		return ids, cobra.ShellCompDirectiveNoFileComp
	}

	enable := &cobra.Command{
		Use:               "enable <cleaner-id>",
		Short:             tr.Text("Enable a cleaner in every future run", "永久启用指定清理项"),
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeCleanerIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigSetEnabled(resolveConfigDir(dir), args[0], true, cmd.OutOrStdout(), localTr)
		},
	}

	disable := &cobra.Command{
		Use:               "disable <cleaner-id>",
		Short:             tr.Text("Disable a cleaner in every future run", "永久禁用指定清理项"),
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: completeCleanerIDs,
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigSetEnabled(resolveConfigDir(dir), args[0], false, cmd.OutOrStdout(), localTr)
		},
	}

	whitelistCmd := &cobra.Command{
		Use:   "whitelist",
		Short: tr.Text("Inspect or manage protected whitelist paths", "查看或管理白名单保护路径"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runWhitelistList(resolveConfigDir(dir), cmd.OutOrStdout(), localTr)
		},
	}

	wlAdd := &cobra.Command{
		Use:   "add <path>",
		Short: tr.Text("Add a path prefix to config.yaml whitelist", "向 config.yaml 添加白名单保护路径"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runWhitelistAdd(resolveConfigDir(dir), args[0], cmd.OutOrStdout(), localTr)
		},
	}

	wlRemove := &cobra.Command{
		Use:   "remove <path>",
		Short: tr.Text("Remove a path prefix from config.yaml whitelist", "从 config.yaml 移除白名单路径"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runWhitelistRemove(resolveConfigDir(dir), args[0], cmd.OutOrStdout(), localTr)
		},
	}
	whitelistCmd.AddCommand(wlAdd, wlRemove)

	cmd.AddCommand(initCmd, show, edit, enable, disable, whitelistCmd)
	return cmd
}

// runConfigInit creates a starter config.yaml in cfgDir.
// runConfigInit 在指定目录创建带注释的初始配置文件。
func runConfigInit(cfgDir string, force bool, stdout io.Writer, tr *i18n.Translator) error {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(cfgDir, "config.yaml")
	if !force {
		if _, err := os.Stat(path); err == nil {
			return ExitError{
				code: 1,
				err: fmt.Errorf("%s", tr.Textf(
					"config file already exists at %s; use `beav config edit` to modify it or pass `--force` to overwrite",
					"配置文件已存在于 %s；请使用 `beav config edit` 编辑或加 `--force` 覆盖",
					path,
				)),
			}
		}
	}
	if err := writeDefaultConfig(path, force, tr); err != nil {
		return err
	}
	if stdout != nil {
		fmt.Fprintf(stdout, "%s\n", tr.Textf("Created config file at %s", "已创建配置文件：%s", path))
		fmt.Fprintf(stdout, "%s\n", tr.Text("Next: edit with `beav config edit` or browse cleaners with `beav list`.", "下一步：运行 `beav config edit` 编辑配置，或 `beav list` 查看所有清理项。"))
	}
	return nil
}

func writeDefaultConfig(path string, force bool, tr *i18n.Translator) error {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
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
	_, err = f.WriteString(tr.Text(
		"# Beav configuration\n"+
			"# CLI flags override this file; this file overrides built-in cleaner defaults.\n"+
			"defaults:\n"+
			"  min_age_days: 14\n"+
			"  output: auto # auto | spinner | plain | json\n\n"+
			"# Per-cleaner overrides (run `beav list` to inspect all cleaner IDs):\n"+
			"# overrides:\n"+
			"#   editor-vscode:\n"+
			"#     min_age_days: 3\n"+
			"#   browser-chromium:\n"+
			"#     enabled: false\n\n"+
			"# Paths never touched by any cleaner (or manage via `beav config whitelist add <path>`):\n"+
			"# whitelist:\n"+
			"#   - ~/.cache/keep-this\n",
		"# Beav 配置文件\n"+
			"# 优先级：命令行参数 > 本配置文件 > 清理项内置默认值。\n"+
			"defaults:\n"+
			"  min_age_days: 14\n"+
			"  output: auto # auto | spinner | plain | json\n\n"+
			"# 针对单个清理项的覆盖设置（运行 `beav list` 查看全部清理项 ID）：\n"+
			"# overrides:\n"+
			"#   editor-vscode:\n"+
			"#     min_age_days: 3\n"+
			"#   browser-chromium:\n"+
			"#     enabled: false\n\n"+
			"# 白名单保护路径（也可通过 `beav config whitelist add <路径>` 管理）：\n"+
			"# whitelist:\n"+
			"#   - ~/.cache/keep-this\n",
	))
	return err
}

func runConfigShow(cmd *cobra.Command, dir string, showJSON bool, tr *i18n.Translator) error {
	cfgDir := resolveConfigDir(dir)
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	userCleaners, err := registry.LoadUserDir(cfg.CleanersDir())
	if err != nil {
		return ExitError{code: 2, err: err}
	}

	if showJSON {
		out := map[string]any{
			"config_path":   cfg.ConfigPath(),
			"config_exists": cfg.ConfigExists(),
			"cleaners_dir":  cfg.CleanersDir(),
			"defaults":      cfg.Defaults,
			"overrides":     cfg.Overrides,
			"whitelist":     cfg.RawWhitelist(),
			"whitelist_txt": cfg.WhitelistTxt(),
		}
		enc := json.NewEncoder(cmd.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	w := cmd.OutOrStdout()
	statusLabel := tr.Text("not created yet, using defaults", "尚未创建，使用默认配置")
	if cfg.ConfigExists() {
		statusLabel = tr.Text("active", "生效中")
	}
	fmt.Fprintf(w, "%s %s (%s)\n", padLabel(tr.Text("Config file:", "配置文件："), 16), cfg.ConfigPath(), statusLabel)
	fmt.Fprintf(w, "%s %s (%s)\n", padLabel(tr.Text("Custom dir:", "自定义目录："), 16), cfg.CleanersDir(), tr.Plural(int64(len(userCleaners)), "custom cleaner", "custom cleaners", "个自定义清理项"))
	fmt.Fprintf(w, "%s min_age_days=%d, output=%s\n", padLabel(tr.Text("Defaults:", "默认设置："), 16), cfg.Defaults.MinAgeDays, cfg.Defaults.Output)

	fmt.Fprintln(w)
	fmt.Fprintln(w, tr.Text("Overrides:", "清理项覆盖："))
	if len(cfg.Overrides) == 0 {
		fmt.Fprintln(w, "  "+tr.Text("(none)", "（无）"))
	} else {
		ids := make([]string, 0, len(cfg.Overrides))
		for id := range cfg.Overrides {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			ov := cfg.Overrides[id]
			var parts []string
			if ov.Enabled != nil {
				parts = append(parts, fmt.Sprintf("enabled=%v", *ov.Enabled))
			}
			if ov.MinAgeDays != nil {
				parts = append(parts, fmt.Sprintf("min_age_days=%d", *ov.MinAgeDays))
			}
			fmt.Fprintf(w, "  %s: %s\n", id, strings.Join(parts, ", "))
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, tr.Text("Whitelist:", "白名单："))
	printWhitelistEntries(w, cfg, tr)

	fmt.Fprintln(w)
	fmt.Fprintln(w, tr.Text("Run `beav list` to inspect all available cleaners, or `beav config init` to create a starter config.", "运行 `beav list` 查看所有清理项，或运行 `beav config init` 生成初始配置文件。"))
	return nil
}

func runWhitelistList(cfgDir string, w io.Writer, tr *i18n.Translator) error {
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	printWhitelistEntries(w, cfg, tr)
	return nil
}

func printWhitelistEntries(w io.Writer, cfg *config.Config, tr *i18n.Translator) {
	yamlList := cfg.RawWhitelist()
	txtList := cfg.WhitelistTxt()
	if len(yamlList) == 0 && len(txtList) == 0 {
		fmt.Fprintln(w, "  "+tr.Text("(none) - add one with `beav config whitelist add <path>`", "（无）— 可使用 `beav config whitelist add <路径>` 添加"))
		return
	}
	for _, p := range yamlList {
		fmt.Fprintf(w, "  %s (config.yaml)\n", p)
	}
	for _, p := range txtList {
		fmt.Fprintf(w, "  %s (whitelist.txt)\n", p)
	}
}

func runWhitelistAdd(cfgDir, path string, w io.Writer, tr *i18n.Translator) error {
	home, _ := os.UserHomeDir()
	cfg, err := config.LoadWithHome(cfgDir, home)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	added, err := cfg.AddWhitelist(path, home)
	if err != nil {
		return ExitError{code: 1, err: err}
	}
	if !added {
		fmt.Fprintf(w, "%s\n", tr.Textf("%q is already in the whitelist.", "%q 已在白名单中。", path))
		return nil
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s\n", tr.Textf("Added %q to %s", "已将 %q 添加至白名单（%s）", path, cfg.ConfigPath()))
	return nil
}

func runWhitelistRemove(cfgDir, path string, w io.Writer, tr *i18n.Translator) error {
	home, _ := os.UserHomeDir()
	cfg, err := config.LoadWithHome(cfgDir, home)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	removed, inTxt := cfg.RemoveWhitelist(path, home)
	if !removed {
		if inTxt {
			return ExitError{
				code: 1,
				err: fmt.Errorf("%s", tr.Textf(
					"%q is configured in %s; edit that file directly to remove it",
					"%q 配置在 %s 中，请直接编辑该文件将其移除",
					path, filepath.Join(cfgDir, "whitelist.txt"),
				)),
			}
		}
		return ExitError{
			code: 1,
			err:  fmt.Errorf("%s", tr.Textf("%q was not found in the whitelist", "白名单中未找到 %q", path)),
		}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s\n", tr.Textf("Removed %q from %s", "已从白名单移除 %q（%s）", path, cfg.ConfigPath()))
	if inTxt {
		fmt.Fprintf(w, "%s\n", tr.Textf("Note: a matching entry still exists in %s.", "提示：%s 中仍保留有相同条目。", filepath.Join(cfgDir, "whitelist.txt")))
	}
	return nil
}

func boolState(b bool, tr *i18n.Translator) string {
	if b {
		return tr.Text("enabled", "启用")
	}
	return tr.Text("disabled", "禁用")
}

// runConfigSetEnabled persists an enable/disable override for one cleaner.
// runConfigSetEnabled 为单个清理项持久化启用或禁用规则。
func runConfigSetEnabled(cfgDir, id string, enabled bool, stdout io.Writer, tr *i18n.Translator) error {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
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
	known := false
	for _, c := range merged {
		if c.ID == id {
			known = true
			break
		}
	}
	if !known {
		suggestions := suggestCleanerIDs(merged, id, 3)
		if len(suggestions) > 0 {
			return ExitError{
				code: 1,
				err: fmt.Errorf("%s", tr.Textf(
					"unknown cleaner id %q (did you mean: %s?); run `beav list` to see all IDs",
					"未找到清理项 ID %q（是否指：%s？）；运行 `beav list` 查看全部 ID",
					id, strings.Join(suggestions, ", "),
				)),
			}
		}
		return ExitError{
			code: 1,
			err: fmt.Errorf("%s", tr.Textf(
				"unknown cleaner id %q; run `beav list` to list all IDs",
				"未找到清理项 ID %q；运行 `beav list` 查看全部 ID",
				id,
			)),
		}
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
	fmt.Fprintf(stdout, "%s\n", tr.Textf("%s is now %s by default", "%s 已设为默认%s", id, boolState(enabled, tr)))
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
