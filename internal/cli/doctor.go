package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dengqi/beav/cleaners"
	"github.com/dengqi/beav/internal/cleaner/registry"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/sysinfo"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"
	"golang.org/x/sys/unix"
)

type diskUsageInfo struct {
	Path       string  `json:"path"`
	TotalBytes uint64  `json:"total_bytes"`
	UsedBytes  uint64  `json:"used_bytes"`
	AvailBytes uint64  `json:"avail_bytes"`
	UsedPct    float64 `json:"used_pct"`
}

type toolStatusInfo struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
}

type doctorReport struct {
	Distro         string           `json:"distro"`
	Arch           string           `json:"arch"`
	UID            int              `json:"uid"`
	IsRoot         bool             `json:"is_root"`
	Home           string           `json:"home"`
	ConfigDir      string           `json:"config_dir"`
	ConfigExists   bool             `json:"config_exists"`
	ConfigValid    bool             `json:"config_valid"`
	ConfigError    string           `json:"config_error,omitempty"`
	BuiltinCount   int              `json:"builtin_cleaners"`
	CustomCount    int              `json:"custom_cleaners"`
	OperationLog   string           `json:"operation_log,omitempty"`
	OpLogBytes     int64            `json:"operation_log_bytes"`
	BypassAll      bool             `json:"bypass_all"`
	Disks          []diskUsageInfo  `json:"disks"`
	Tools          []toolStatusInfo `json:"tools"`
}

// NewDoctorCmd creates the `beav doctor` command (aliased to `status`).
// NewDoctorCmd 创建 `beav doctor`（别名 `status`）系统与环境诊断命令。
func NewDoctorCmd(tr *i18n.Translator) *cobra.Command {
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	var configDir string
	var showJSON bool

	cmd := &cobra.Command{
		Use:     "doctor",
		Aliases: []string{"status", "check"},
		GroupID: "inspect",
		Short:   tr.Text("Diagnose disk space, config, safety policy, and tool availability", "诊断磁盘空间、配置状态、安全策略与外部工具可用性"),
		Example: "  beav doctor\n  beav status\n  beav doctor --json",
		RunE: func(cmd *cobra.Command, _ []string) error {
			localTr := translatorForCommand(cmd, tr)
			rep := collectDoctorReport(configDir)
			out := cmd.OutOrStdout()
			if showJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(rep)
			}

			fmt.Fprintln(out, localTr.Text("Beav Environment & System Status", "Beav 运行环境与系统诊断"))
			fmt.Fprintln(out, strings.Repeat("─", 44))
			fmt.Fprintf(out, "%s %s (%s)\n", padLabel(localTr.Text("System:", "系统平台："), 16), rep.Distro, rep.Arch)
			priv := localTr.Textf("user (uid=%d)", "普通用户 (uid=%d)", rep.UID)
			if rep.IsRoot {
				priv = localTr.Text("root (uid=0)", "管理员 root (uid=0)")
			}
			fmt.Fprintf(out, "%s %s\n", padLabel(localTr.Text("Privilege:", "当前权限："), 16), priv)
			fmt.Fprintf(out, "%s %s\n", padLabel(localTr.Text("Home:", "主目录："), 16), rep.Home)

			cfgState := localTr.Text("default (no config.yaml)", "使用默认配置（未创建 config.yaml）")
			if rep.ConfigExists {
				cfgState = localTr.Text("active", "已启用")
			}
			if !rep.ConfigValid {
				cfgState = localTr.Textf("invalid: %s", "校验失败：%s", rep.ConfigError)
			}
			fmt.Fprintf(out, "%s %s (%s)\n", padLabel(localTr.Text("Config:", "配置文件："), 16), rep.ConfigDir, cfgState)
			fmt.Fprintf(out, "%s %s\n", padLabel(localTr.Text("Cleaners:", "清理项数量："), 16),
				localTr.Textf("%d built-in, %d custom", "%d 内置，%d 自定义", rep.BuiltinCount, rep.CustomCount))

			safetyState := localTr.Text("standard (configurable)", "标准轻量保护（全部规则可配置）")
			if rep.BypassAll {
				safetyState = localTr.Text("BYPASS ALL ENABLED", "已启用全局绕过（Bypass All）")
			}
			fmt.Fprintf(out, "%s %s\n", padLabel(localTr.Text("Safety:", "安全策略："), 16), safetyState)

			if rep.OperationLog != "" {
				fmt.Fprintf(out, "%s %s (%s)\n", padLabel(localTr.Text("Op log:", "操作日志："), 16), rep.OperationLog, humanize.Bytes(safeDoctorBytes(rep.OpLogBytes)))
			}

			if len(rep.Disks) > 0 {
				fmt.Fprintln(out)
				fmt.Fprintln(out, localTr.Text("Disk Space:", "磁盘空间："))
				rows := make([][]string, 0, len(rep.Disks))
				for _, d := range rep.Disks {
					rows = append(rows, []string{
						d.Path,
						humanize.Bytes(d.TotalBytes),
						humanize.Bytes(d.UsedBytes),
						humanize.Bytes(d.AvailBytes),
						fmt.Sprintf("%.1f%%", d.UsedPct),
					})
				}
				writeAlignedTable(out, []string{
					localTr.Text("PATH", "路径"),
					localTr.Text("TOTAL", "总容量"),
					localTr.Text("USED", "已用"),
					localTr.Text("AVAIL", "可用"),
					localTr.Text("USE%", "使用率"),
				}, rows)
			}

			fmt.Fprintln(out)
			fmt.Fprintln(out, localTr.Text("External Tools:", "外部工具检测："))
			var installed, missing []string
			for _, t := range rep.Tools {
				if t.Installed {
					installed = append(installed, t.Name)
				} else {
					missing = append(missing, t.Name)
				}
			}
			fmt.Fprintf(out, "  %s %s\n", localTr.Text("Available:", "已安装："), strings.Join(installed, ", "))
			if len(missing) > 0 {
				fmt.Fprintf(out, "  %s %s\n", localTr.Text("Not found:", "未检测到："), strings.Join(missing, ", "))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&configDir, "config-dir", "", tr.Text("override config directory", "覆盖默认配置目录"))
	cmd.Flags().BoolVar(&showJSON, "json", false, tr.Text("output diagnostic report as JSON", "输出 JSON 格式诊断报告"))
	return cmd
}

func collectDoctorReport(configDir string) doctorReport {
	osr := sysinfo.DetectOSRelease()
	distro := osr.ID
	if distro == "" {
		distro = runtime.GOOS
	}
	home, _ := os.UserHomeDir()
	cfgDir := resolveConfigDir(configDir)
	rep := doctorReport{
		Distro:    distro,
		Arch:      runtime.GOARCH,
		UID:       os.Getuid(),
		IsRoot:    os.Getuid() == 0,
		Home:      home,
		ConfigDir: cfgDir,
	}

	if builtin, err := registry.LoadBuiltin(cleaners.Builtin); err == nil {
		rep.BuiltinCount = len(builtin)
	}
	if cfg, err := config.LoadWithHome(cfgDir, home); err == nil {
		rep.ConfigExists = cfg.ConfigExists()
		rep.BypassAll = cfg.Safety.BypassAll
		if user, uErr := registry.LoadUserDirWithPolicy(cfg.CleanersDir(), cfg.Safety.ToPolicy()); uErr == nil {
			rep.CustomCount = len(user)
			rep.ConfigValid = true
		} else {
			rep.ConfigError = uErr.Error()
		}
	} else {
		rep.ConfigError = err.Error()
	}

	if logPath := operationLogPath(home, os.Geteuid()); logPath != "" {
		rep.OperationLog = logPath
		if st, err := os.Stat(logPath); err == nil {
			rep.OpLogBytes = st.Size()
		}
	}

	seenFS := map[uint64]bool{}
	for _, p := range []string{home, "/"} {
		if p == "" {
			continue
		}
		if d, fsid, ok := statDisk(p); ok && !seenFS[fsid] {
			seenFS[fsid] = true
			rep.Disks = append(rep.Disks, d)
		}
	}

	probeTools := []string{
		"docker", "podman", "uv", "pip", "npm", "pnpm", "yarn", "bun",
		"cargo", "rustup", "go", "conda", "brew", "flatpak", "journalctl",
		"apt-get", "dnf", "pacman", "zypper", "hf", "mise", "nix-collect-garbage",
	}
	for _, name := range probeTools {
		p, err := exec.LookPath(name)
		rep.Tools = append(rep.Tools, toolStatusInfo{Name: name, Installed: err == nil, Path: p})
	}
	return rep
}

func statDisk(path string) (diskUsageInfo, uint64, bool) {
	var st unix.Stat_t
	if err := unix.Stat(path, &st); err != nil {
		return diskUsageInfo{}, 0, false
	}
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil {
		return diskUsageInfo{}, 0, false
	}
	bsize := uint64(fs.Bsize) //nolint:gosec // Linux statfs block size is positive.
	total := fs.Blocks * bsize
	avail := fs.Bavail * bsize
	free := fs.Bfree * bsize
	var used uint64
	if total >= free {
		used = total - free
	}
	var pct float64
	if total > 0 {
		pct = float64(used) * 100 / float64(total)
	}
	return diskUsageInfo{
		Path:       filepath.Clean(path),
		TotalBytes: total,
		UsedBytes:  used,
		AvailBytes: avail,
		UsedPct:    pct,
	}, uint64(st.Dev), true
}

func safeDoctorBytes(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}
