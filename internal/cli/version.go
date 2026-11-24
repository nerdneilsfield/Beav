package cli

import (
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/spf13/cobra"
)

type versionInfo struct {
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	Date            string `json:"date"`
	GoVersion       string `json:"go_version"`
	OS              string `json:"os"`
	Arch            string `json:"arch"`
	Distro          string `json:"distro,omitempty"`
	ConfigDir       string `json:"config_dir"`
	CleanersBuiltin int    `json:"cleaners_builtin"`
}

func collectVersionInfo(version, commit, date string) versionInfo {
	osr := sysinfo.DetectOSRelease()
	distro := osr.ID
	if distro == "" {
		distro = osr.IDLike
	}
	builtinCount := 0
	if loaded, err := registry.LoadBuiltin(cleaners.Builtin); err == nil {
		builtinCount = len(loaded)
	}
	return versionInfo{
		Version:         version,
		Commit:          commit,
		Date:            date,
		GoVersion:       runtime.Version(),
		OS:              runtime.GOOS,
		Arch:            runtime.GOARCH,
		Distro:          distro,
		ConfigDir:       resolveConfigDir(""),
		CleanersBuiltin: builtinCount,
	}
}

// NewVersionCmd creates a command that prints detailed beav version and build information.
// NewVersionCmd 创建一个打印详尽 beav 版本与构建信息的命令。
func NewVersionCmd(version, commit, date string, tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	var short bool
	var showJSON bool

	cmd := &cobra.Command{
		Use:     "version",
		Aliases: []string{"ver"},
		GroupID: "util",
		Short:   tr.Text("Print beav version, build, and platform details", "显示 beav 版本、构建与运行平台详情"),
		Example: "  beav version\n  beav version --short\n  beav version --json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			out := cmd.OutOrStdout()
			if short {
				_, err := fmt.Fprintln(out, version)
				return err
			}
			info := collectVersionInfo(version, commit, date)
			if showJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(info)
			}
			platform := fmt.Sprintf("%s/%s", info.OS, info.Arch)
			if info.Distro != "" {
				platform += " (" + info.Distro + ")"
			}
			fmt.Fprintf(out, "beav %s\n", info.Version)
			fmt.Fprintf(out, "  %s %s\n", padLabel(localTr.Text("Commit:", "提交："), 12), info.Commit)
			fmt.Fprintf(out, "  %s %s\n", padLabel(localTr.Text("Built:", "构建时间："), 12), info.Date)
			fmt.Fprintf(out, "  %s %s\n", padLabel(localTr.Text("Go:", "Go 版本："), 12), info.GoVersion)
			fmt.Fprintf(out, "  %s %s\n", padLabel(localTr.Text("Platform:", "运行平台："), 12), platform)
			fmt.Fprintf(out, "  %s %s\n", padLabel(localTr.Text("Config dir:", "配置目录："), 12), info.ConfigDir)
			fmt.Fprintf(out, "  %s %s\n", padLabel(localTr.Text("Cleaners:", "内置清理项："), 12),
				localTr.Plural(int64(info.CleanersBuiltin), "built-in cleaner", "built-in cleaners", "项"))
			return nil
		},
	}
	cmd.Flags().BoolVarP(&short, "short", "s", false, tr.Text("print only the version string", "仅输出版本号"))
	cmd.Flags().BoolVar(&showJSON, "json", false, tr.Text("output version details as JSON", "以 JSON 格式输出版本信息"))
	return cmd
}
