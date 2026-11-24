package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/cleaner/safety"
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
	var resetYes bool

	cmd := &cobra.Command{
		Use:     "config",
		GroupID: "util",
		Short:   tr.Text("Inspect or manage beav configuration, safety policy, and whitelist", "查看或管理 beav 配置文件、安全策略与白名单"),
		Long: tr.Text(
			"Manage ~/.config/beav/config.yaml, safety policy overrides, per-cleaner enable/disable rules, and protected whitelist paths.\n"+
				"To browse all available cleaners and IDs, use `beav list` or `beav info <id>`.",
			"管理 ~/.config/beav/config.yaml 配置、安全边界策略、单个清理项的启用/禁用规则及白名单保护路径。\n"+
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
			args := make([]string, 0, len(parts))
			args = append(args, parts[1:]...)
			args = append(args, path)
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

	pathCmd := &cobra.Command{
		Use:   "path",
		Short: tr.Text("Print resolved config, cleaners.d, whitelist, and log paths", "显示配置目录、自定义规则、白名单与日志文件路径"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigPath(resolveConfigDir(dir), cmd.OutOrStdout(), localTr)
		},
	}

	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: tr.Text("Validate config.yaml, whitelist, and custom cleaners", "校验 config.yaml、白名单及自定义清理项定义"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigValidate(resolveConfigDir(dir), cmd.OutOrStdout(), localTr)
		},
	}

	resetCmd := &cobra.Command{
		Use:   "reset",
		Short: tr.Text("Reset config.yaml back to starter defaults", "将 config.yaml 重置为默认初始配置"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigReset(resolveConfigDir(dir), resetYes, os.Stdin, cmd.OutOrStdout(), localTr)
		},
	}
	resetCmd.Flags().BoolVarP(&resetYes, "yes", "y", false, tr.Text("overwrite without confirmation", "跳过确认直接重置"))

	safetyCmd := newConfigSafetyCmd(&dir, tr)

	cmd.AddCommand(initCmd, show, edit, pathCmd, validateCmd, resetCmd, enable, disable, whitelistCmd, safetyCmd)
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
			"#   - ~/.cache/keep-this\n\n"+
			"# Configurable safety policy (all restrictions can be customized or bypassed):\n"+
			"# safety:\n"+
			"#   bypass_all: false\n"+
			"#   allow_list: []\n"+
			"#   replace_allow_list: false\n"+
			"#   disable_allow_list: false\n"+
			"#   blacklist: []\n"+
			"#   replace_blacklist: false\n"+
			"#   disable_blacklist: false\n"+
			"#   ignore_running_processes: false\n"+
			"#   bypass_age_floor: false\n"+
			"#   follow_symlinks: false\n"+
			"#   allow_cross_fs: false\n"+
			"#   allow_root_home: false\n"+
			"#   allow_command_cleaners: false\n",
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
			"#   - ~/.cache/keep-this\n\n"+
			"# 可配置安全策略（所有限制均可自定义或绕过）：\n"+
			"# safety:\n"+
			"#   bypass_all: false\n"+
			"#   allow_list: []\n"+
			"#   replace_allow_list: false\n"+
			"#   disable_allow_list: false\n"+
			"#   blacklist: []\n"+
			"#   replace_blacklist: false\n"+
			"#   disable_blacklist: false\n"+
			"#   ignore_running_processes: false\n"+
			"#   bypass_age_floor: false\n"+
			"#   follow_symlinks: false\n"+
			"#   allow_cross_fs: false\n"+
			"#   allow_root_home: false\n"+
			"#   allow_command_cleaners: false\n",
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

// resolveConfigDir returns the config directory, using the override or BEAV_CONFIG_DIR if provided.
// resolveConfigDir 返回配置目录，优先使用显式参数或环境变量 BEAV_CONFIG_DIR。
func resolveConfigDir(dir string) string {
	if dir != "" {
		return dir
	}
	if envDir := strings.TrimSpace(os.Getenv("BEAV_CONFIG_DIR")); envDir != "" {
		return envDir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "beav")
}

func runConfigPath(cfgDir string, out io.Writer, tr *i18n.Translator) error {
	home, _ := os.UserHomeDir()
	opLog := operationLogPath(home, os.Geteuid())
	fmt.Fprintf(out, "%s %s\n", padLabel(tr.Text("Config dir:", "配置目录："), 16), cfgDir)
	fmt.Fprintf(out, "%s %s\n", padLabel(tr.Text("Config file:", "配置文件："), 16), filepath.Join(cfgDir, "config.yaml"))
	fmt.Fprintf(out, "%s %s\n", padLabel(tr.Text("Cleaners dir:", "自定义目录："), 16), filepath.Join(cfgDir, "cleaners.d"))
	fmt.Fprintf(out, "%s %s\n", padLabel(tr.Text("Whitelist file:", "白名单文件："), 16), filepath.Join(cfgDir, "whitelist.txt"))
	if opLog != "" {
		fmt.Fprintf(out, "%s %s\n", padLabel(tr.Text("Operation log:", "操作日志："), 16), opLog)
	}
	return nil
}

func runConfigValidate(cfgDir string, out io.Writer, tr *i18n.Translator) error {
	home, _ := os.UserHomeDir()
	cfg, err := config.LoadWithHome(cfgDir, home)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	policy := cfg.Safety.ToPolicy()
	builtin, err := registry.LoadBuiltin(cleaners.Builtin)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	user, err := registry.LoadUserDirWithPolicy(cfg.CleanersDir(), policy)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	merged, err := registry.MergeByIDWithPolicy(builtin, user, policy)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	for _, c := range merged {
		if c.Scope == model.ScopeUser {
			if err := registry.ValidatePathsWithPolicy(c, home, policy); err != nil {
				return ExitError{code: 2, err: err}
			}
		} else {
			if err := registry.ValidatePathsWithPolicy(c, "", policy); err != nil {
				return ExitError{code: 2, err: err}
			}
		}
	}
	fmt.Fprintf(out, "%s\n", tr.Textf(
		"Configuration is valid (%s, %s).",
		"配置校验通过（%s，%s）。",
		tr.Plural(int64(len(user)), "custom cleaner", "custom cleaners", "个自定义清理项"),
		tr.Plural(int64(len(merged)), "total cleaner", "total cleaners", "个总清理项"),
	))
	return nil
}

func runConfigReset(cfgDir string, yes bool, stdin io.Reader, out io.Writer, tr *i18n.Translator) error {
	path := filepath.Join(cfgDir, "config.yaml")
	if !yes {
		if _, err := os.Stat(path); err == nil {
			fmt.Fprintf(out, "%s", tr.Textf("Overwrite %s with default configuration? [y/N] ", "确认将 %s 重置为默认配置？[y/N] ", path))
			line, readErr := bufio.NewReader(stdin).ReadString('\n')
			if readErr != nil && line == "" {
				return nil
			}
			ans := strings.ToLower(strings.TrimSpace(line))
			if ans != "y" && ans != "yes" {
				fmt.Fprintln(out, tr.Text("Canceled.", "已取消。"))
				return nil
			}
		}
	}
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return err
	}
	if err := writeDefaultConfig(path, true, tr); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n", tr.Textf("Reset config file at %s", "已重置配置文件：%s", path))
	return nil
}

func newConfigSafetyCmd(dir *string, tr *i18n.Translator) *cobra.Command {
	safetyCmd := &cobra.Command{
		Use:   "safety",
		Short: tr.Text("Inspect or configure safety boundaries and bypass rules", "查看或配置安全边界与绕过规则"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigSafetyShow(resolveConfigDir(*dir), cmd.OutOrStdout(), localTr)
		},
	}

	showCmd := &cobra.Command{
		Use:   "show",
		Short: tr.Text("Show current safety policy and allow/blacklist entries", "显示当前安全策略与允许/黑名单规则"),
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigSafetyShow(resolveConfigDir(*dir), cmd.OutOrStdout(), localTr)
		},
	}

	setCmd := &cobra.Command{
		Use:   "set <key> <true|false>",
		Short: tr.Text("Set a boolean safety policy key in config.yaml", "在 config.yaml 中设置安全策略布尔开关"),
		Long: tr.Text(
			"Valid keys: bypass_all, replace_allow_list, disable_allow_list, replace_blacklist,\n"+
				"disable_blacklist, ignore_running_processes, bypass_age_floor, follow_symlinks,\n"+
				"allow_cross_fs, allow_root_home, allow_command_cleaners",
			"可用键名：bypass_all、replace_allow_list、disable_allow_list、replace_blacklist、\n"+
				"disable_blacklist、ignore_running_processes、bypass_age_floor、follow_symlinks、\n"+
				"allow_cross_fs、allow_root_home、allow_command_cleaners",
		),
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			val, err := strconv.ParseBool(args[1])
			if err != nil {
				return ExitError{code: 1, err: fmt.Errorf("%s", localTr.Textf("invalid boolean %q (use true or false)", "无效的布尔值 %q（请传入 true 或 false）", args[1]))}
			}
			return runConfigSafetySet(resolveConfigDir(*dir), args[0], val, cmd.OutOrStdout(), localTr)
		},
	}

	allowAdd := &cobra.Command{
		Use:   "allow-add <path>",
		Short: tr.Text("Add a path prefix to safety.allow_list", "向 safety.allow_list 添加允许清理路径前缀"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigSafetyListEdit(resolveConfigDir(*dir), true, true, args[0], cmd.OutOrStdout(), localTr)
		},
	}
	allowRemove := &cobra.Command{
		Use:   "allow-remove <path>",
		Short: tr.Text("Remove a path prefix from safety.allow_list", "从 safety.allow_list 移除允许路径前缀"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigSafetyListEdit(resolveConfigDir(*dir), true, false, args[0], cmd.OutOrStdout(), localTr)
		},
	}
	denyAdd := &cobra.Command{
		Use:   "deny-add <path>",
		Short: tr.Text("Add a path prefix to safety.blacklist", "向 safety.blacklist 添加保护黑名单前缀"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigSafetyListEdit(resolveConfigDir(*dir), false, true, args[0], cmd.OutOrStdout(), localTr)
		},
	}
	denyRemove := &cobra.Command{
		Use:   "deny-remove <path>",
		Short: tr.Text("Remove a path prefix from safety.blacklist", "从 safety.blacklist 移除黑名单前缀"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			localTr := translatorForCommand(cmd, tr)
			return runConfigSafetyListEdit(resolveConfigDir(*dir), false, false, args[0], cmd.OutOrStdout(), localTr)
		},
	}

	safetyCmd.AddCommand(showCmd, setCmd, allowAdd, allowRemove, denyAdd, denyRemove)
	return safetyCmd
}

func runConfigSafetyShow(cfgDir string, out io.Writer, tr *i18n.Translator) error {
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	s := cfg.Safety
	rows := [][]string{
		{"bypass_all", strconv.FormatBool(s.BypassAll), tr.Text("bypass all optional restrictions", "绕过全部可配置安全限制")},
		{"disable_allow_list", strconv.FormatBool(s.DisableAllowList), tr.Text("disable allow-list boundary check", "关闭允许路径边界检查")},
		{"replace_allow_list", strconv.FormatBool(s.ReplaceAllowList), tr.Text("use only custom allow_list", "仅使用自定义 allow_list")},
		{"disable_blacklist", strconv.FormatBool(s.DisableBlacklist), tr.Text("disable blacklist protection (except /)", "关闭黑名单保护（仍保护 /）")},
		{"replace_blacklist", strconv.FormatBool(s.ReplaceBlacklist), tr.Text("use only custom blacklist", "仅使用自定义 blacklist")},
		{"ignore_running_processes", strconv.FormatBool(s.IgnoreRunningProcesses), tr.Text("ignore running app and busy checks", "忽略运行中进程与缓存占用检查")},
		{"bypass_age_floor", strconv.FormatBool(s.BypassAgeFloor), tr.Text("allow 0d age on managed/debris/rustup", "允许 0 天清理托管/杂项/rustup 缓存")},
		{"follow_symlinks", strconv.FormatBool(s.FollowSymlinks), tr.Text("follow symbolic links and link binaries", "允许遍历符号链接与链接二进制")},
		{"allow_cross_fs", strconv.FormatBool(s.AllowCrossFS), tr.Text("allow crossing filesystem boundaries", "允许跨文件系统或挂载点边界")},
		{"allow_root_home", strconv.FormatBool(s.AllowRootHome), tr.Text("allow cleaning /root when running as root", "允许以 root 身份清理 /root")},
		{"allow_command_cleaners", strconv.FormatBool(s.AllowCommandCleaners), tr.Text("allow type: command custom cleaners", "允许执行自定义 type: command 清理器")},
	}
	writeAlignedTable(out, []string{tr.Text("SETTING", "配置项"), tr.Text("VALUE", "当前值"), tr.Text("DESCRIPTION", "说明")}, rows)

	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %v\n", tr.Text("Custom allow_list:", "自定义允许路径："), s.AllowList)
	fmt.Fprintf(out, "%s %v\n", tr.Text("Default system allow_list:", "默认系统允许路径："), safety.DefaultAllowRoots())
	fmt.Fprintf(out, "%s %v\n", tr.Text("Custom blacklist:", "自定义黑名单："), s.Blacklist)
	return nil
}

func runConfigSafetySet(cfgDir, key string, val bool, out io.Writer, tr *i18n.Translator) error {
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	if !SetSafetyFlag(&cfg.Safety, key, val) {
		return ExitError{code: 1, err: fmt.Errorf("%s", tr.Textf("unknown safety setting %q", "未知的安全配置项 %q", key))}
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n", tr.Textf("safety.%s = %v", "safety.%s 已设为 %v", key, val))
	return nil
}

// SetSafetyFlag updates a named boolean field in config.SafetyConfig.
// SetSafetyFlag 按名称更新 config.SafetyConfig 中的布尔配置项。
func SetSafetyFlag(s *config.SafetyConfig, key string, val bool) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "bypass_all":
		s.BypassAll = val
	case "replace_allow_list":
		s.ReplaceAllowList = val
	case "disable_allow_list":
		s.DisableAllowList = val
	case "replace_blacklist":
		s.ReplaceBlacklist = val
	case "disable_blacklist":
		s.DisableBlacklist = val
	case "ignore_running_processes":
		s.IgnoreRunningProcesses = val
	case "bypass_age_floor":
		s.BypassAgeFloor = val
	case "follow_symlinks":
		s.FollowSymlinks = val
	case "allow_cross_fs":
		s.AllowCrossFS = val
	case "allow_root_home":
		s.AllowRootHome = val
	case "allow_command_cleaners":
		s.AllowCommandCleaners = val
	default:
		return false
	}
	return true
}

func runConfigSafetyListEdit(cfgDir string, isAllow, add bool, rawPath string, out io.Writer, tr *i18n.Translator) error {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return ExitError{code: 1, err: fmt.Errorf("%s", tr.Text("path cannot be empty", "路径不能为空"))}
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return ExitError{code: 2, err: err}
	}
	list := &cfg.Safety.Blacklist
	name := "blacklist"
	if isAllow {
		list = &cfg.Safety.AllowList
		name = "allow_list"
	}
	if add {
		for _, item := range *list {
			if item == p {
				fmt.Fprintf(out, "%s\n", tr.Textf("%q is already in safety.%s", "%q 已存在于 safety.%s 中", p, name))
				return nil
			}
		}
		*list = append(*list, p)
		if err := cfg.Save(); err != nil {
			return err
		}
		fmt.Fprintf(out, "%s\n", tr.Textf("Added %q to safety.%s", "已将 %q 添加到 safety.%s", p, name))
		return nil
	}
	next := make([]string, 0, len(*list))
	removed := false
	for _, item := range *list {
		if item == p {
			removed = true
			continue
		}
		next = append(next, item)
	}
	if !removed {
		return ExitError{code: 1, err: fmt.Errorf("%s", tr.Textf("%q not found in safety.%s", "safety.%s 中未找到 %q", name, p))}
	}
	*list = next
	if err := cfg.Save(); err != nil {
		return err
	}
	fmt.Fprintf(out, "%s\n", tr.Textf("Removed %q from safety.%s", "已从 safety.%s 移除 %q", name, p))
	return nil
}
