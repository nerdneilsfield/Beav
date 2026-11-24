package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type configRowKind int

const (
	cfgRowToggle configRowKind = iota
	cfgRowMinAge
	cfgRowListHeader
	cfgRowListItem
)

type configListTarget int

const (
	cfgListAllow configListTarget = iota
	cfgListBlacklist
	cfgListWhitelist
)

type configRow struct {
	kind       configRowKind
	key        string
	label      string
	desc       string
	listTarget configListTarget
	listIndex  int
	listValue  string
}

func (m workbenchModel) configRows() []configRow {
	rows := []configRow{
		{kind: cfgRowToggle, key: "bypass_all", label: m.tr.Text("Bypass all restrictions", "全局绕过所有限制 (bypass_all)"), desc: m.tr.Text("Disables allow-list, blacklist (except /), age floors, process guards, and unlocks heavy + commands", "禁用允许列表、黑名单（仍保护 /）、年龄下限与进程检查，并启用重度与命令项")},
		{kind: cfgRowToggle, key: "disable_allow_list", label: m.tr.Text("Disable allow-list check", "禁用允许路径边界检查 (disable_allow_list)"), desc: m.tr.Text("Allows cleaning paths outside default home and system roots", "允许清理默认主目录与系统根目录之外的路径")},
		{kind: cfgRowToggle, key: "replace_allow_list", label: m.tr.Text("Replace default allow-list", "替换默认允许列表 (replace_allow_list)"), desc: m.tr.Text("Use only safety.allow_list instead of built-in defaults", "仅使用自定义的 allow_list 代替内置默认列表")},
		{kind: cfgRowToggle, key: "disable_blacklist", label: m.tr.Text("Disable blacklist protection", "禁用黑名单保护 (disable_blacklist)"), desc: m.tr.Text("Allows paths in default blacklist (literal / remains protected)", "允许访问默认黑名单路径（根目录 / 始终受保护）")},
		{kind: cfgRowToggle, key: "replace_blacklist", label: m.tr.Text("Replace default blacklist", "替换默认黑名单 (replace_blacklist)"), desc: m.tr.Text("Use only safety.blacklist instead of built-in defaults", "仅使用自定义的 blacklist 代替内置默认黑名单")},
		{kind: cfgRowToggle, key: "ignore_running_processes", label: m.tr.Text("Ignore running processes", "忽略运行中进程检查 (ignore_running_processes)"), desc: m.tr.Text("Skip running app, lock marker, and open-file checks", "跳过应用运行中、锁文件与打开文件描述符检查")},
		{kind: cfgRowToggle, key: "bypass_age_floor", label: m.tr.Text("Bypass minimum age floor", "绕过最小保留天数下限 (bypass_age_floor)"), desc: m.tr.Text("Allow 0-day cleanup on managed caches, debris, and rustup", "允许对托管缓存、残留文件与 rustup 按 0 天清理")},
		{kind: cfgRowToggle, key: "follow_symlinks", label: m.tr.Text("Follow symbolic links", "允许符号链接 (follow_symlinks)"), desc: m.tr.Text("Allow symlinked roots, entries, and tool binaries", "允许遍历符号链接目录及符号链接外部工具")},
		{kind: cfgRowToggle, key: "allow_cross_fs", label: m.tr.Text("Allow cross-filesystem", "允许跨文件系统 (allow_cross_fs)"), desc: m.tr.Text("Allow traversing across different mount points/filesystems", "允许在扫描与清理时跨越挂载点与文件系统边界")},
		{kind: cfgRowToggle, key: "allow_root_home", label: m.tr.Text("Allow root home (/root)", "允许清理 /root 目录 (allow_root_home)"), desc: m.tr.Text("Allow cleaning /root when running as root", "以 root 身份运行时允许清理 /root")},
		{kind: cfgRowToggle, key: "allow_command_cleaners", label: m.tr.Text("Allow custom command cleaners", "允许自定义命令清理项 (allow_command_cleaners)"), desc: m.tr.Text("Allow user YAML cleaners with type: command", "允许执行用户自定义 YAML 中的 type: command 清理器")},
		{kind: cfgRowMinAge, key: "min_age_days", label: m.tr.Text("Default min age (days)", "默认最小保留天数 (min_age_days)"), desc: m.tr.Text("Use ←/→ or -/+ to adjust default age threshold", "使用 ←/→ 或 -/+ 调整默认保留天数")},
	}

	rows = append(rows, configRow{kind: cfgRowListHeader, listTarget: cfgListAllow, label: m.tr.Text("Custom Allow-List (press + to add)", "自定义允许路径 allow_list（按 + 添加）")})
	for i, p := range m.policy.AllowList {
		rows = append(rows, configRow{kind: cfgRowListItem, listTarget: cfgListAllow, listIndex: i, listValue: p})
	}
	rows = append(rows, configRow{kind: cfgRowListHeader, listTarget: cfgListBlacklist, label: m.tr.Text("Custom Blacklist (press + to add)", "自定义黑名单 blacklist（按 + 添加）")})
	for i, p := range m.policy.Blacklist {
		rows = append(rows, configRow{kind: cfgRowListItem, listTarget: cfgListBlacklist, listIndex: i, listValue: p})
	}
	if m.cfg != nil {
		rows = append(rows, configRow{kind: cfgRowListHeader, listTarget: cfgListWhitelist, label: m.tr.Text("Protected Whitelist (press + to add)", "保护白名单 whitelist（按 + 添加）")})
		for i, p := range m.cfg.RawWhitelist() {
			rows = append(rows, configRow{kind: cfgRowListItem, listTarget: cfgListWhitelist, listIndex: i, listValue: p})
		}
	}
	return rows
}

func (m workbenchModel) getPolicyToggle(key string) bool {
	switch key {
	case "bypass_all":
		return m.policy.BypassAll
	case "disable_allow_list":
		return m.policy.DisableAllowList
	case "replace_allow_list":
		return m.policy.ReplaceAllowList
	case "disable_blacklist":
		return m.policy.DisableBlacklist
	case "replace_blacklist":
		return m.policy.ReplaceBlacklist
	case "ignore_running_processes":
		return m.policy.IgnoreRunningProcesses
	case "bypass_age_floor":
		return m.policy.BypassAgeFloor
	case "follow_symlinks":
		return m.policy.FollowSymlinks
	case "allow_cross_fs":
		return m.policy.AllowCrossFS
	case "allow_root_home":
		return m.policy.AllowRootHome
	case "allow_command_cleaners":
		return m.policy.AllowCommandCleaners
	}
	return false
}

func (m *workbenchModel) setPolicyToggle(key string, val bool) {
	switch key {
	case "bypass_all":
		m.policy.BypassAll = val
	case "disable_allow_list":
		m.policy.DisableAllowList = val
	case "replace_allow_list":
		m.policy.ReplaceAllowList = val
	case "disable_blacklist":
		m.policy.DisableBlacklist = val
	case "replace_blacklist":
		m.policy.ReplaceBlacklist = val
	case "ignore_running_processes":
		m.policy.IgnoreRunningProcesses = val
	case "bypass_age_floor":
		m.policy.BypassAgeFloor = val
	case "follow_symlinks":
		m.policy.FollowSymlinks = val
	case "allow_cross_fs":
		m.policy.AllowCrossFS = val
	case "allow_root_home":
		m.policy.AllowRootHome = val
	case "allow_command_cleaners":
		m.policy.AllowCommandCleaners = val
	}
	m.syncPolicyToConfig()
}

func (m *workbenchModel) syncPolicyToConfig() {
	if m.cfg == nil {
		return
	}
	m.cfg.Safety.BypassAll = m.policy.BypassAll
	m.cfg.Safety.AllowList = append([]string(nil), m.policy.AllowList...)
	m.cfg.Safety.ReplaceAllowList = m.policy.ReplaceAllowList
	m.cfg.Safety.DisableAllowList = m.policy.DisableAllowList
	m.cfg.Safety.Blacklist = append([]string(nil), m.policy.Blacklist...)
	m.cfg.Safety.ReplaceBlacklist = m.policy.ReplaceBlacklist
	m.cfg.Safety.DisableBlacklist = m.policy.DisableBlacklist
	m.cfg.Safety.IgnoreRunningProcesses = m.policy.IgnoreRunningProcesses
	m.cfg.Safety.BypassAgeFloor = m.policy.BypassAgeFloor
	m.cfg.Safety.FollowSymlinks = m.policy.FollowSymlinks
	m.cfg.Safety.AllowCrossFS = m.policy.AllowCrossFS
	m.cfg.Safety.AllowRootHome = m.policy.AllowRootHome
	m.cfg.Safety.AllowCommandCleaners = m.policy.AllowCommandCleaners
}

func (m workbenchModel) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.cfgAdding {
		switch key {
		case "esc":
			m.cfgAdding = false
			m.cfgAddInput.SetValue("")
			m.cfgAddInput.Blur()
			return m, nil
		case "enter":
			val := strings.TrimSpace(m.cfgAddInput.Value())
			m.cfgAdding = false
			m.cfgAddInput.SetValue("")
			m.cfgAddInput.Blur()
			if val != "" {
				switch m.cfgAddTarget {
				case cfgListAllow:
					m.policy.AllowList = append(m.policy.AllowList, val)
					m.syncPolicyToConfig()
				case cfgListBlacklist:
					m.policy.Blacklist = append(m.policy.Blacklist, val)
					m.syncPolicyToConfig()
				case cfgListWhitelist:
					if m.cfg != nil {
						_, _ = m.cfg.AddWhitelist(val, m.scope.Home)
					}
				}
				m.cfgStatus = m.tr.Text("Updated in memory (press w to save, R to apply & rescan).", "已在内存中更新（按 w 保存，按 R 应用并重新扫描）。")
			}
			return m, nil
		default:
			var cmd tea.Cmd
			m.cfgAddInput, cmd = m.cfgAddInput.Update(msg)
			return m, cmd
		}
	}

	rows := m.configRows()
	if m.cfgCursor >= len(rows) {
		m.cfgCursor = maxInt(0, len(rows)-1)
	}
	cur := rows[m.cfgCursor]

	switch key {
	case "q":
		return m.quitAll()
	case "up", "k":
		if m.cfgCursor > 0 {
			m.cfgCursor--
			m.ensureConfigCursorVisible(len(rows))
		}
	case "down", "j":
		if m.cfgCursor < len(rows)-1 {
			m.cfgCursor++
			m.ensureConfigCursorVisible(len(rows))
		}
	case " ", "enter":
		switch cur.kind {
		case cfgRowToggle:
			m.setPolicyToggle(cur.key, !m.getPolicyToggle(cur.key))
			m.cfgStatus = m.tr.Text("Updated (press w to save, R to rescan).", "已更新（按 w 保存到 config.yaml，按 R 重新扫描）。")
		case cfgRowListHeader:
			m.cfgAdding = true
			m.cfgAddTarget = cur.listTarget
			return m, m.cfgAddInput.Focus()
		}
	case "+", "=":
		if cur.kind == cfgRowMinAge && m.cfg != nil {
			m.cfg.Defaults.MinAgeDays++
		} else {
			target := cfgListAllow
			if cur.kind == cfgRowListHeader || cur.kind == cfgRowListItem {
				target = cur.listTarget
			}
			m.cfgAdding = true
			m.cfgAddTarget = target
			return m, m.cfgAddInput.Focus()
		}
	case "-", "_":
		if cur.kind == cfgRowMinAge && m.cfg != nil && m.cfg.Defaults.MinAgeDays > 0 {
			m.cfg.Defaults.MinAgeDays--
		}
	case "right", "l":
		if cur.kind == cfgRowMinAge && m.cfg != nil {
			m.cfg.Defaults.MinAgeDays++
		}
	case "left", "h":
		if cur.kind == cfgRowMinAge && m.cfg != nil && m.cfg.Defaults.MinAgeDays > 0 {
			m.cfg.Defaults.MinAgeDays--
		}
	case "x", "delete", "backspace":
		if cur.kind == cfgRowListItem {
			switch cur.listTarget {
			case cfgListAllow:
				m.policy.AllowList = append(m.policy.AllowList[:cur.listIndex], m.policy.AllowList[cur.listIndex+1:]...)
				m.syncPolicyToConfig()
			case cfgListBlacklist:
				m.policy.Blacklist = append(m.policy.Blacklist[:cur.listIndex], m.policy.Blacklist[cur.listIndex+1:]...)
				m.syncPolicyToConfig()
			case cfgListWhitelist:
				if m.cfg != nil {
					_, _ = m.cfg.RemoveWhitelist(cur.listValue, m.scope.Home)
				}
			}
			m.cfgStatus = m.tr.Text("Entry removed (press w to save).", "已移除条目（按 w 保存）。")
		}
	case "w":
		if m.cfg == nil {
			m.cfgStatus = m.tr.Text("No config object loaded.", "未加载配置对象。")
			return m, nil
		}
		m.syncPolicyToConfig()
		var saveErr error
		if m.hooks.SaveConfig != nil {
			saveErr = m.hooks.SaveConfig(m.cfg)
		} else {
			saveErr = m.cfg.Save()
		}
		if saveErr != nil {
			m.cfgStatus = m.tr.Textf("Save failed: %v", "保存失败：%v", saveErr)
		} else {
			m.cfgStatus = m.tr.Textf("Saved to %s", "已保存到 %s", m.cfg.ConfigPath())
		}
	case "R":
		m.activeTab = tabClean
		nextM, cmd := m.startScan()
		return nextM, tea.Batch(nextM.spin.Tick, cmd)
	}
	return m, nil
}

func (m *workbenchModel) ensureConfigCursorVisible(total int) {
	vp := maxInt(5, m.tabContentHeight()-5)
	if m.cfgCursor < m.cfgOffset {
		m.cfgOffset = m.cfgCursor
	}
	if m.cfgCursor >= m.cfgOffset+vp {
		m.cfgOffset = m.cfgCursor - vp + 1
	}
	if m.cfgOffset > maxInt(0, total-vp) {
		m.cfgOffset = maxInt(0, total-vp)
	}
	if m.cfgOffset < 0 {
		m.cfgOffset = 0
	}
}

func (m workbenchModel) renderConfigTab() string {
	w := maxInt(40, m.width-4)
	h := maxInt(8, m.tabContentHeight())
	rows := m.configRows()

	var b strings.Builder
	cfgPath := m.configDir
	if m.cfg != nil {
		cfgPath = m.cfg.ConfigPath()
	}
	b.WriteString(styleSubtle.Render(m.tr.Text("Config: ", "配置文件：")+fitWidth(cfgPath, maxInt(16, w-16))) + "\n")
	switch {
	case m.cfgAdding:
		b.WriteString(styleTitle.Render(m.tr.Text("Add path prefix: ", "添加路径前缀：")) + m.cfgAddInput.View() + "\n")
	case m.cfgStatus != "":
		b.WriteString(styleWarnText.Render(fitWidth(m.cfgStatus, w-4)) + "\n")
	default:
		b.WriteString(styleMuted.Render(m.tr.Text("Toggle switches with Space; press w to save to config.yaml.", "按空格切换开关；按 w 写入 config.yaml；按 R 应用并重扫。")) + "\n")
	}
	b.WriteString("\n")

	vp := maxInt(4, h-6)
	end := m.cfgOffset + vp
	if end > len(rows) {
		end = len(rows)
	}
	for i := m.cfgOffset; i < end; i++ {
		r := rows[i]
		focused := i == m.cfgCursor
		pointer := "  "
		if focused {
			pointer = "> "
		}
		var line string
		switch r.kind {
		case cfgRowToggle:
			on := m.getPolicyToggle(r.key)
			box := styleSubtle.Render("[OFF]")
			if on {
				box = styleBadgeSafe.Render("[ON ]")
			}
			line = fmt.Sprintf("%s%s %s", pointer, box, fitWidth(r.label, maxInt(16, w-14)))
		case cfgRowMinAge:
			days := 14
			if m.cfg != nil {
				days = m.cfg.Defaults.MinAgeDays
			}
			line = fmt.Sprintf("%s%s %s", pointer, styleBadgeReadOnly.Render(fmt.Sprintf("%dd", days)), r.label)
		case cfgRowListHeader:
			line = pointer + styleSectionHeader.Render(r.label)
		case cfgRowListItem:
			line = fmt.Sprintf("%s  - %s", pointer, r.listValue)
		}
		if focused {
			line = styleActiveRow.Width(w - 4).Render(line)
		}
		b.WriteString(line + "\n")
	}

	if m.cfgCursor >= 0 && m.cfgCursor < len(rows) && rows[m.cfgCursor].desc != "" {
		b.WriteString("\n" + styleSubtle.Render(fitWidth(rows[m.cfgCursor].desc, w-4)))
	}

	pane := styleFocusedPane.Width(w).Height(h).Render(b.String())
	footer := styleMuted.Render(fitWidth(m.tr.Text(
		"↑/↓ Move  Space Toggle  +/- Adjust  + Add path  x Remove path  w Save  R Apply & Rescan  1-4 Tabs  q Quit",
		"↑/↓ 移动  空格 切换  +/- 调整天数  + 添加路径  x 删除路径  w 保存  R 应用并重扫  1-4 切页  q 退出",
	), w))
	return pane + "\n" + footer
}
