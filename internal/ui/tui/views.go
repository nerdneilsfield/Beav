package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dustin/go-humanize"
)

func (m workbenchModel) View() string {
	switch m.activeTab {
	case tabAnalyze:
		return m.renderFramedShell(m.renderHeaderBadge(), m.renderAnalyzeTab())
	case tabHistory:
		return m.renderFramedShell(m.renderHeaderBadge(), m.renderHistoryTab())
	case tabConfig:
		return m.renderFramedShell(m.renderHeaderBadge(), m.renderConfigTab())
	}

	switch m.phase {
	case phaseScanning:
		return m.renderFramedShell(m.renderHeaderBadge(), m.renderScanningView())
	case phaseReview:
		if m.showHelp {
			return m.renderHelpView()
		}
		return m.renderFramedShell(m.renderHeaderBadge(), m.renderReviewView())
	case phaseConfirm:
		return m.renderConfirmView()
	case phaseExecuting:
		return m.renderFramedShell(styleBadgeExec.Render(m.tr.Text("CLEANING", "正在清理")), m.renderExecutingView())
	case phaseDone:
		return m.renderFramedShell(styleBadgeDone.Render(m.tr.Text("DONE", "清理完成")), m.renderDoneView())
	default:
		return ""
	}
}

func (m workbenchModel) renderHeaderBadge() string {
	if m.policy.BypassAll {
		return styleBadgeBypass.Render(m.tr.Text("BYPASS ALL", "全局绕过"))
	}
	if m.phase == phaseScanning {
		return styleBadgeReadOnly.Render(m.tr.Text("READ-ONLY SCAN", "只读扫描"))
	}
	return styleBadgeReadOnly.Render(m.tr.Text("WORKBENCH", "清理工作台"))
}

func (m workbenchModel) renderFramedShell(badge, content string) string {
	header := m.renderHeader(badge)
	tabs := m.renderTabBar()
	return lipgloss.NewStyle().Padding(0, 1).Render(header + "\n" + tabs + "\n" + content)
}

func (m workbenchModel) renderHeader(badge string) string {
	scopeDesc := m.tr.ScopeLabel(m.scope.Scope)
	if m.scope.Home != "" && m.scope.Scope != model.ScopeSystem {
		scopeDesc += " (" + m.scope.Home + ")"
	}
	if m.scope.IsRoot {
		scopeDesc += " [root]"
	}
	ver := ""
	if m.version != "" {
		ver = " " + styleMuted.Render(m.version)
	}
	left := styleTitle.Render("Beav") + ver + "  " + badge + "  " + styleSubtle.Render(scopeDesc)

	right := ""
	if m.diskStat.valid && m.diskStat.total > 0 {
		pct := float64(m.diskStat.used) * 100 / float64(m.diskStat.total)
		right = styleSubtle.Render(m.tr.Textf("Disk: %s free (%.0f%% used)", "磁盘可用：%s（已用 %.0f%%）", humanize.Bytes(m.diskStat.avail), pct))
	}
	if right == "" {
		return left
	}
	gap := m.width - 4 - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 2 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

func (m workbenchModel) renderTabBar() string {
	labels := []struct {
		tab   activeTab
		label string
	}{
		{tabClean, m.tr.Text("1 Clean", "1 清理")},
		{tabAnalyze, m.tr.Text("2 Analyze", "2 磁盘分析")},
		{tabHistory, m.tr.Text("3 History", "3 历史")},
		{tabConfig, m.tr.Text("4 Safety & Config", "4 安全与配置")},
	}
	parts := make([]string, 0, len(labels))
	for _, it := range labels {
		if m.activeTab == it.tab {
			parts = append(parts, styleTabActive.Render(it.label))
		} else {
			parts = append(parts, styleTabInactive.Render(it.label))
		}
	}
	return strings.Join(parts, " ")
}

func (m workbenchModel) renderScanningView() string {
	pct := 0
	if m.scanTotal > 0 {
		pct = m.scanCompleted * 100 / m.scanTotal
	}
	barWidth := m.width - 20
	if barWidth > 50 {
		barWidth = 50
	}
	if barWidth < 16 {
		barWidth = 16
	}
	filled := 0
	if m.scanTotal > 0 {
		filled = m.scanCompleted * barWidth / m.scanTotal
	}
	if filled > barWidth {
		filled = barWidth
	}
	bar := styleBar.Render(strings.Repeat("█", filled)) + styleMuted.Render(strings.Repeat("░", barWidth-filled))

	cur := m.currentName
	if m.currentID != "" {
		if scan, ok := m.scanByID[m.currentID]; ok {
			cur = m.tr.CleanerName(scan.Cleaner)
		} else {
			cur = m.tr.CleanerName(model.Cleaner{ID: m.currentID, Name: m.currentName})
		}
	}
	if cur == "" {
		cur = m.tr.Text("preparing cleaners...", "正在准备扫描...")
	}

	var b strings.Builder
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s %s\n\n", m.spin.View(), m.tr.Text("Scanning caches without modifying any files...", "正在执行只读扫描，不会修改或删除任何文件..."))
	fmt.Fprintf(&b, "[%s] %d/%d (%d%%)\n", bar, m.scanCompleted, m.scanTotal, pct)
	b.WriteString(styleSubtle.Render(fitWidth(m.tr.Textf("Current: %s", "当前项目：%s", cur), maxInt(20, m.width-6))) + "\n\n")
	b.WriteString(m.tr.Textf("%s %s (%s)",
		"%s%s（%s）",
		m.tr.Text("Estimated reclaimable:", "已发现可释放空间："),
		styleBadgeSafe.Render(humanBytes(m.scanCandidateBytes)),
		m.tr.Plural(m.scanCandidateFiles, "file", "files", "个文件"),
	))
	if m.scanNativeCount > 0 {
		b.WriteString(m.tr.Textf(" + %d native", " + %d 个原生命令", m.scanNativeCount))
	}
	b.WriteString("\n\n")
	b.WriteString(styleMuted.Render(m.tr.Text("Press q or Esc to cancel scan, or 2-4 to switch tabs.", "按 q 或 Esc 取消扫描，按 2-4 可切换其他功能页。")))

	return lipgloss.NewStyle().Padding(1, 1).Render(b.String())
}

func (m workbenchModel) renderReviewToolbar() string {
	count, selBytes, selFiles, _, selNative := m.selectionTotals()
	summary := m.tr.Textf("%d selected · %s (%s)",
		"已选 %d 项 · %s（%s）",
		count,
		styleBadgeSafe.Render(humanBytes(selBytes)),
		m.tr.Plural(selFiles, "file", "files", "个文件"),
	)
	if len(selNative) > 0 {
		summary += m.tr.Textf(" + %d native", " + %d 个原生命令", len(selNative))
	}

	sortLabel := map[cleanSortMode]string{
		cleanSortSize:   m.tr.Text("size↓", "大小↓"),
		cleanSortName:   m.tr.Text("name", "名称"),
		cleanSortFiles:  m.tr.Text("files↓", "文件数↓"),
		cleanSortImpact: m.tr.Text("impact", "影响"),
	}[m.sortMode]

	tagLabel := m.activeTagFilter()
	if tagLabel == "" {
		tagLabel = m.tr.Text("all", "全部")
	}
	ageLabel := m.minAgeOverride
	if ageLabel == "" {
		ageLabel = m.tr.Text("default", "默认")
	}

	bypassPill := stylePill.Render(m.tr.Text("b bypass:OFF", "b 绕过:关"))
	if m.policy.BypassAll {
		bypassPill = stylePillWarn.Render(m.tr.Text("b BYPASS:ON", "b 全局绕过:开"))
	}

	pills := strings.Join([]string{
		stylePill.Render(m.tr.Text("o sort:", "o 排序:") + sortLabel),
		stylePill.Render(m.tr.Text("t tag:", "t 标签:") + tagLabel),
		stylePill.Render(m.tr.Text("m age:", "m 留存:") + ageLabel),
		bypassPill,
	}, " ")

	return styleSubtle.Render(summary) + "   " + pills
}

func (m workbenchModel) renderReviewView() string {
	toolbar := m.renderReviewToolbar()
	notice := m.reviewNoticeLine()

	var body string
	if m.width >= 96 {
		leftW := m.width * 55 / 100
		if leftW < 42 {
			leftW = 42
		}
		rightW := m.width - leftW - 3
		if rightW < 30 {
			rightW = 30
		}
		paneH := m.listViewportHeight() + 3
		leftStyle, rightStyle := styleFocusedPane, stylePane
		if m.detailFocused {
			leftStyle, rightStyle = stylePane, styleFocusedPane
		}
		leftPane := leftStyle.Width(leftW).Height(paneH).Render(m.renderListPane(leftW - 4))
		rightPane := rightStyle.Width(rightW).Height(paneH).Render(m.renderDetailPane(rightW-4, paneH-2))
		body = lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane)
	} else {
		fullW := maxInt(36, m.width-2)
		listH := m.listViewportHeight() + 2
		detailH := 8
		if m.narrowDetailExpanded {
			detailH = maxInt(10, m.height/2-4)
		}
		topPane := styleFocusedPane.Width(fullW).Height(listH).Render(m.renderListPane(fullW - 4))
		botPane := stylePane.Width(fullW).Height(detailH).Render(m.renderDetailPane(fullW-4, detailH-2))
		body = lipgloss.JoinVertical(lipgloss.Left, topPane, botPane)
	}

	footer := m.renderReviewFooter()
	parts := []string{toolbar}
	if notice != "" {
		parts = append(parts, notice)
	}
	if m.filtering || m.filterInput.Value() != "" {
		filterLabel := m.tr.Text("Filter:", "筛选：")
		parts = append(parts, styleSubtle.Render(filterLabel)+" "+m.filterInput.View())
	}
	if m.statusMsg != "" {
		parts = append(parts, styleWarnText.Render(m.statusMsg))
	}
	parts = append(parts, body, footer)
	return strings.Join(parts, "\n")
}

func (m workbenchModel) reviewNoticeLine() string {
	var running []string
	var errCount int
	for _, s := range m.scans {
		if s.Status == "error" {
			errCount++
		}
		if s.SkipReason == string(model.SkipReasonRunningProcess) || s.SkipReason == "cache_busy" || s.SkipReason == string(model.SkipReasonRuntimeBusy) {
			running = append(running, m.tr.CleanerName(s.Cleaner))
		}
	}
	var notes []string
	if len(running) > 0 {
		shown := running
		suffix := ""
		if len(shown) > 3 {
			suffix = m.tr.Textf(" (+%d)", "（另有 %d 项）", len(shown)-3)
			shown = shown[:3]
		}
		notes = append(notes, m.tr.Textf("Running apps skipped: %s%s", "应用运行中已跳过：%s%s", strings.Join(shown, m.tr.Text(", ", "、")), suffix))
	}
	if errCount > 0 {
		notes = append(notes, m.tr.Textf("%d scan errors (press s to inspect)", "%d 项扫描出错（按 s 查看）", errCount))
	}
	if len(notes) == 0 {
		return ""
	}
	return styleWarnText.Render(fitWidth("! "+strings.Join(notes, "  |  "), maxInt(20, m.width-4)))
}

func (m workbenchModel) maxActionableBytes() int64 {
	var maxB int64
	for _, s := range m.scans {
		if isActionableScan(s) && s.BytesCandidate > maxB {
			maxB = s.BytesCandidate
		}
	}
	return maxB
}

func (m workbenchModel) renderListPane(innerWidth int) string {
	if len(m.rows) == 0 {
		if m.filterInput.Value() != "" || m.activeTagFilter() != "" {
			return styleMuted.Render(m.tr.Text("No cleaners match the current filter/tag. Press Esc or t to clear.", "没有匹配当前筛选条件的清理项，按 Esc 或 t 重置。"))
		}
		return styleMuted.Render(m.tr.Text("No cleaners found for this scope.", "当前作用域下没有可用的清理项。"))
	}

	vp := m.listViewportHeight()
	end := m.listOffset + vp
	if end > len(m.rows) {
		end = len(m.rows)
	}
	maxBytes := m.maxActionableBytes()
	var lines []string
	for idx := m.listOffset; idx < end; idx++ {
		r := m.rows[idx]
		focused := idx == m.cursor
		switch r.kind {
		case rowHeader:
			lines = append(lines, styleSectionHeader.Render(fitWidth(fmt.Sprintf("%s (%d)", r.label, r.count), innerWidth)))
		case rowToggleSkipped:
			arrow := "▸"
			if m.showSkipped || m.filterInput.Value() != "" {
				arrow = "▾"
			}
			label := fmt.Sprintf("%s %s (%d)", arrow, m.tr.Text("Skipped & nothing to do", "已跳过 / 无需清理"), r.count)
			if focused {
				lines = append(lines, styleActiveRow.Render(fitWidth("> "+label, innerWidth)))
			} else {
				lines = append(lines, styleMuted.Render(fitWidth("  "+label, innerWidth)))
			}
		case rowItem:
			s := m.scanByID[r.id]
			lines = append(lines, m.renderItemRow(s, r.section, focused, innerWidth, maxBytes))
		}
	}
	return strings.Join(lines, "\n")
}

func (m workbenchModel) renderItemRow(s CleanerScan, section rowSection, focused bool, innerWidth int, maxBytes int64) string {
	pointer := "  "
	if focused {
		pointer = "> "
	}

	var check string
	switch {
	case m.selected[s.Cleaner.ID]:
		check = styleBadgeSafe.Render("[x]")
	case section == sectionSkipped && s.Status == "error":
		check = styleDangerText.Render("[!]")
	case section == sectionSkipped:
		check = styleMuted.Render("[-]")
	default:
		check = styleSubtle.Render("[ ]")
	}

	right := ""
	switch {
	case section == sectionSkipped && s.Status == "error":
		right = m.tr.Text("error", "出错")
	case section == sectionSkipped && s.Status == "skipped":
		right = m.tr.SkipPhrase(s.SkipReason)
	case section == sectionSkipped:
		right = m.tr.Text("0 B", "无需清理")
	case s.BytesCandidate > 0:
		right = humanBytes(s.BytesCandidate)
	case s.FilesCandidate > 0:
		right = m.tr.Plural(s.FilesCandidate, "file", "files", "个文件")
	default:
		right = m.tr.Text("native", "原生命令")
	}

	bar := ""
	if section != sectionSkipped && maxBytes > 0 && innerWidth >= 46 {
		bar = " " + renderRatioBar(s.BytesCandidate, maxBytes, 5)
	}

	badge := ""
	if s.Cleaner.EffectiveImpact() == model.ImpactHeavy && section != sectionSkipped {
		badge = " " + styleBadgeHeavy.Render(m.tr.Text("[heavy]", "[重度]"))
	} else if s.Cleaner.Type == model.TypeCommand {
		badge = " " + styleWarnText.Render("[cmd]")
	}

	name := m.tr.CleanerName(s.Cleaner)
	reservedRight := lipgloss.Width(right) + lipgloss.Width(badge) + lipgloss.Width(bar) + 2
	nameWidth := innerWidth - 6 - reservedRight
	if nameWidth < 10 {
		nameWidth = 10
	}
	trimmedName := fitWidth(name, nameWidth)
	leftPart := fmt.Sprintf("%s%s %s%s", pointer, check, trimmedName, badge)
	rightPart := right + bar
	gap := innerWidth - lipgloss.Width(leftPart) - lipgloss.Width(rightPart)
	if gap < 1 {
		gap = 1
	}
	line := leftPart + strings.Repeat(" ", gap) + styleSubtle.Render(rightPart)
	if focused {
		return styleActiveRow.Width(innerWidth).Render(line)
	}
	return line
}

func (m workbenchModel) renderDetailPane(innerWidth, maxLines int) string {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return styleMuted.Render(m.tr.Text("Select an item to inspect details.", "选择列表项查看详细信息。"))
	}
	row := m.rows[m.cursor]
	if row.kind == rowToggleSkipped {
		return styleSubtle.Render(m.tr.Text(
			"Press Space or s to expand or collapse skipped and zero-result cleaners.",
			"按空格键或 s 展开/折叠已跳过和无需清理的项目。",
		))
	}
	s := m.scanByID[row.id]
	c := s.Cleaner

	var lines []string
	lines = append(lines, styleTitle.Render(fitWidth(m.tr.CleanerName(c), innerWidth)))
	lines = append(lines, styleMuted.Render(fitWidth(fmt.Sprintf("ID: %s  |  %s%s", c.ID, m.tr.Text("Scope: ", "作用域："), m.tr.ScopeLabel(c.Scope)), innerWidth)))
	if len(c.Tags) > 0 {
		lines = append(lines, styleMuted.Render(fitWidth(m.tr.Text("Tags: ", "标签：")+strings.Join(c.Tags, ", "), innerWidth)))
	}

	if desc := m.tr.CleanerDesc(c); desc != "" {
		lines = append(lines, "")
		lines = append(lines, wrapText(desc, innerWidth)...)
	}

	lines = append(lines, "")
	ageText := "-"
	if c.MinAgeDays != nil {
		ageText = m.tr.Days(*c.MinAgeDays)
	}
	lines = append(lines, fitWidth(m.tr.Textf(
		"Impact: %s   Min age: %s   Method: %s",
		"影响：%s   保留期：%s   方式：%s",
		m.tr.ImpactLabel(c.EffectiveImpact()),
		ageText,
		m.tr.ExecutorTypeLabel(c.Type),
	), innerWidth))

	if len(c.Paths) > 0 {
		lines = append(lines, styleMuted.Render(fitWidth(m.tr.Text("Targets: ", "目标路径：")+strings.Join(c.Paths, ", "), innerWidth)))
	}

	if c.EffectiveImpact() == model.ImpactHeavy {
		if note := m.tr.HeavyNote(c); note != "" {
			lines = append(lines, "")
			for _, wl := range wrapText("! "+note, innerWidth) {
				lines = append(lines, styleWarnText.Render(wl))
			}
		}
	}

	switch s.Status {
	case "error":
		lines = append(lines, "")
		lines = append(lines, styleDangerText.Render(m.tr.Text("Scan error:", "扫描出错：")))
		lines = append(lines, wrapText(s.ErrorDetail, innerWidth)...)
	case "skipped":
		lines = append(lines, "")
		lines = append(lines, styleWarnText.Render(fitWidth(m.tr.Text("Skip reason: ", "跳过原因：")+m.tr.SkipPhrase(s.SkipReason), innerWidth)))
		if remedy := m.tr.SkipRemedy(s.SkipReason); remedy != "" {
			lines = append(lines, wrapText(m.tr.Text("Tip: ", "建议：")+remedy, innerWidth)...)
		}
		if s.SkipDetail != "" {
			lines = append(lines, wrapText(s.SkipDetail, innerWidth)...)
		}
	}

	switch {
	case len(s.Samples) > 0:
		lines = append(lines, "")
		lines = append(lines, styleSubtle.Render(m.tr.Textf("Sample candidates (%s, %s):", "待清理文件预览（共 %s，%s）：",
			humanBytes(s.BytesCandidate), m.tr.Plural(s.FilesCandidate, "file", "files", "个文件"))))
		for _, smp := range s.Samples {
			short := shortenHome(smp.Path, m.scope.Home)
			entry := fmt.Sprintf("  %s (%s)", short, humanBytes(smp.Size))
			lines = append(lines, styleMuted.Render(fitWidth(entry, innerWidth)))
		}
	case len(s.CommandPreview) > 0:
		lines = append(lines, "")
		lines = append(lines, styleSubtle.Render(m.tr.Text("Native command preview:", "命令输出预览：")))
		for _, cl := range s.CommandPreview {
			lines = append(lines, styleMuted.Render(fitWidth("  "+cl, innerWidth)))
		}
	case s.MetricsIncomplete:
		lines = append(lines, "")
		lines = append(lines, styleSubtle.Render(m.tr.Text(
			"Native command cleaner: exact size cannot be estimated ahead of execution.",
			"原生命令清理项：执行前无法准确估算可释放空间。",
		)))
	}

	if maxLines > 0 && len(lines) > maxLines {
		start := m.detailScroll
		maxStart := len(lines) - maxLines + 1
		if start > maxStart {
			start = maxStart
		}
		if start < 0 {
			start = 0
		}
		end := start + maxLines - 1
		if end > len(lines) {
			end = len(lines)
		}
		visible := append([]string(nil), lines[start:end]...)
		visible = append(visible, styleMuted.Render(m.tr.Textf(
			"[scroll %d-%d/%d · J/K or Tab]",
			"[滚动 %d-%d/%d · J/K 或 Tab]",
			start+1, end, len(lines),
		)))
		return strings.Join(visible, "\n")
	}
	return strings.Join(lines, "\n")
}

func (m workbenchModel) renderReviewFooter() string {
	var hints string
	if m.filtering {
		hints = m.tr.Text("Type to filter  |  Enter apply  |  Esc clear & cancel", "输入关键词搜索  |  Enter 应用  |  Esc 清除并退出")
	} else {
		hints = m.tr.Text(
			"Space Toggle  a Safe  A All+Heavy  s Skipped  o Sort  t Tag  m Age  b Bypass  / Search  Enter Clean  ? Help  q Quit",
			"空格 勾选  a 常规  A 全选含重度  s 跳过项  o 排序  t 标签  m 留存天数  b 绕过  / 搜索  Enter 清理  ? 帮助  q 退出",
		)
	}
	out := styleSubtle.Render(fitWidth(hints, maxInt(20, m.width-4)))
	if !m.scope.IsRoot && m.scope.Scope == model.ScopeUser {
		sysHint := m.tr.Text(
			"Tip: 1-4 switch tabs (Clean / Analyze / History / Safety & Config) | system caches: `sudo beav --system`",
			"提示：1-4 切换工作台标签（清理 / 磁盘分析 / 历史 / 安全与配置）| 系统缓存需 `sudo beav --system`",
		)
		out += "\n" + styleMuted.Render(fitWidth(sysHint, maxInt(20, m.width-4)))
	}
	return out
}

func (m workbenchModel) renderConfirmView() string {
	count, selBytes, selFiles, heavy, native := m.selectionTotals()

	var b strings.Builder
	b.WriteString(styleTitle.Render(m.tr.Text("Confirm Cleanup", "确认清理")) + "\n\n")
	b.WriteString(m.tr.Textf("Selected cleaners: %d\n", "已选清理项：%d 项\n", count))
	b.WriteString(m.tr.Textf("Estimated reclaimable: %s (%s)\n", "预计可释放：%s（%s）\n",
		humanBytes(selBytes), m.tr.Plural(selFiles, "file", "files", "个文件")))

	if m.policy.BypassAll {
		b.WriteString("\n" + styleDangerText.Render(m.tr.Text("WARNING: Bypass All mode is active!", "警告：当前处于全局绕过（Bypass All）模式！")) + "\n")
	}
	if len(heavy) > 0 {
		b.WriteString("\n" + styleWarnText.Render(m.tr.Text("Heavy cleaners manually selected:", "已手动勾选的重度清理项：")) + "\n")
		for _, s := range heavy {
			b.WriteString(styleWarnText.Render(fmt.Sprintf("  - %s (%s)\n", m.tr.CleanerName(s.Cleaner), s.Cleaner.ID)))
		}
	}
	if len(native) > 0 {
		b.WriteString("\n" + styleSubtle.Render(m.tr.Text("Native commands (exact byte count unmeasured until run):", "以下原生命令在执行前无法估算释放空间：")) + "\n")
		for _, s := range native {
			b.WriteString(styleSubtle.Render(fmt.Sprintf("  - %s (%s)\n", m.tr.CleanerName(s.Cleaner), s.Cleaner.ID)))
		}
	}

	b.WriteString("\n" + styleMuted.Render(m.tr.Text("Deleted cache files cannot be undone.", "注意：删除操作不可撤销，请确认后再继续。")) + "\n\n")

	cancelLabel := m.tr.Text("Cancel", "取消")
	deleteLabel := m.tr.Text("Delete selected", "确认删除")
	var btnCancel, btnDelete string
	if m.confirmDeleteFocused {
		btnCancel = styleBtnDefault.Render(cancelLabel)
		btnDelete = styleBtnDangerFocused.Render(deleteLabel)
	} else {
		btnCancel = styleBtnFocused.Render(cancelLabel)
		btnDelete = styleBtnDefault.Render(deleteLabel)
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Center, btnCancel, "   ", btnDelete) + "\n\n")
	b.WriteString(styleMuted.Render(m.tr.Text("←/→ or Tab switch  |  Enter confirm button  |  y delete now  |  Esc back", "←/→ 或 Tab 切换按钮  |  Enter 确认  |  y 立即删除  |  Esc 返回")))

	modal := styleModal.MaxWidth(maxInt(44, m.width-4)).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}

func (m workbenchModel) renderExecutingView() string {
	var b strings.Builder
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s %s (%d/%d)\n\n",
		m.spin.View(),
		m.tr.Text("Running selected cleaners...", "正在清理已选项目..."),
		m.execCompleted, m.execTotal,
	)

	maxRows := maxInt(6, m.height-12)
	shown := 0
	for _, id := range m.execOrder {
		if shown >= maxRows {
			b.WriteString(styleMuted.Render(m.tr.Textf("  ... and %d more", "  ... 另有 %d 项", len(m.execOrder)-shown)) + "\n")
			break
		}
		name := id
		if scan, ok := m.scanByID[id]; ok {
			name = m.tr.CleanerName(scan.Cleaner)
		}
		res, done := m.execItems[id]
		switch {
		case done:
			switch {
			case res.Status == "error":
				fmt.Fprintf(&b, "  %s %s (%s) - %s\n", styleDangerText.Render("✗"), name, id, fitWidth(res.ErrorDetail, 45))
			case res.MetricsIncomplete && res.BytesFreed == 0:
				fmt.Fprintf(&b, "  %s %s (%s) - %s\n", styleBadgeSafe.Render("✓"), name, id, m.tr.Text("native completed", "命令已执行"))
			default:
				fmt.Fprintf(&b, "  %s %s (%s) - %s\n", styleBadgeSafe.Render("✓"), name, id, humanBytes(res.BytesFreed))
			}
		case m.currentID == id:
			fmt.Fprintf(&b, "  %s %s (%s)\n", m.spin.View(), name, id)
		default:
			b.WriteString(styleMuted.Render(fmt.Sprintf("  · %s (%s)\n", name, id)))
		}
		shown++
	}

	return lipgloss.NewStyle().Padding(0, 1).Render(b.String())
}

func (m workbenchModel) renderDoneView() string {
	var b strings.Builder
	b.WriteString("\n")
	if m.err != nil {
		b.WriteString(styleDangerText.Render(m.tr.Textf("Completed with error: %v\n\n", "清理过程出错：%v\n\n", m.err)))
	}
	b.WriteString(m.tr.Textf("%s %s (%s)\n",
		"%s%s（%s）\n",
		m.tr.Text("Total space freed:", "共释放空间："),
		styleBadgeSafe.Render(humanBytes(m.execSummary.TotalFreed)),
		m.tr.Plural(m.execSummary.TotalFiles, "file", "files", "个文件"),
	))
	if len(m.execSummary.Unmeasured) > 0 {
		b.WriteString(styleSubtle.Render(m.tr.Textf(
			"Native commands completed (exact byte size unmeasured): %s\n",
			"以下原生命令已执行（未统计大小）：%s\n",
			strings.Join(m.execSummary.Unmeasured, ", "),
		)))
	}
	if m.execSummary.Errored > 0 {
		b.WriteString(styleDangerText.Render(m.tr.Textf("Cleaners with errors: %d\n", "执行出错：%d 项\n", m.execSummary.Errored)))
	}
	if m.execSummary.LogPath != "" {
		b.WriteString(styleMuted.Render(m.tr.Textf("Operation log: %s\n", "操作日志：%s\n", m.execSummary.LogPath)))
	}

	b.WriteString("\n" + styleSubtle.Render(m.tr.Text("Press r to rescan, 3 for history, or q / Enter to exit.", "按 r 重新扫描，按 3 查看历史记录，按 q 或 Enter 退出。")))
	return lipgloss.NewStyle().Padding(0, 1).Render(b.String())
}

func (m workbenchModel) renderHelpView() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.tr.Text("Beav TUI Workbench Help", "Beav 交互工作台操作说明")) + "\n\n")
	b.WriteString(m.tr.Text(
		"Tabs & Navigation:\n"+
			"  1 / 2 / 3 / 4        Switch tabs (Clean / Analyze / History / Safety & Config)\n"+
			"  [ / ]                Previous / next tab\n"+
			"  ↑/↓, j/k, PgUp/PgDn  Move cursor\n"+
			"  Tab / J / K          Focus / scroll right detail pane\n"+
			"  Space                Toggle current cleaner (or expand skipped group)\n"+
			"  a / A                Toggle safe cleaners / Toggle ALL including heavy\n"+
			"  o / t / m            Cycle sort order / tag filter / min-age preset\n"+
			"  b                    Toggle Bypass All mode and rescan\n"+
			"  s                    Show / hide skipped and empty cleaners\n"+
			"  /                    Search by name, ID, or tag (Esc clears)\n"+
			"  r                    Re-run read-only scan\n"+
			"  Enter / c            Proceed to confirmation dialog\n"+
			"  q / Ctrl+C           Quit without deleting\n",
		"标签页与操作快捷键：\n"+
			"  1 / 2 / 3 / 4        切换视图（清理 / 磁盘分析 / 历史 / 安全与配置）\n"+
			"  [ / ]                上一个 / 下一个标签页\n"+
			"  ↑/↓、j/k、PgUp/PgDn  移动光标\n"+
			"  Tab / J / K          切换焦点或滚动右侧详情面板\n"+
			"  空格 / Space         勾选或取消当前清理项（在折叠行上展开跳过项）\n"+
			"  a / A                全选常规项 / 全选全部项（包含重度项）\n"+
			"  o / t / m            切换排序方式 / 标签筛选 / 留存天数预设\n"+
			"  b                    一键切换全局绕过（Bypass All）并重扫\n"+
			"  s                    显示/隐藏已跳过与无需清理的项目\n"+
			"  /                    按名称、ID 或标签搜索（Esc 清除筛选）\n"+
			"  r                    重新执行只读扫描\n"+
			"  Enter / c            进入最终删除确认界面\n"+
			"  q / Ctrl+C           退出且不删除任何文件\n",
	))
	b.WriteString("\n" + styleMuted.Render(m.tr.Text("Press ?, Esc, or Enter to close help.", "按 ?、Esc 或 Enter 关闭帮助。")))
	modal := styleHelpModal.MaxWidth(maxInt(54, m.width-6)).Render(b.String())
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
}

func humanBytes(n int64) string {
	if n < 0 {
		n = 0
	}
	return humanize.Bytes(uint64(n))
}

func shortenHome(path, home string) string {
	if home == "" {
		return path
	}
	cleanHome := filepath.Clean(home)
	if path == cleanHome {
		return "~"
	}
	if strings.HasPrefix(path, cleanHome+string(filepath.Separator)) {
		return "~" + path[len(cleanHome):]
	}
	return path
}

func fitWidth(s string, maxCells int) string {
	if maxCells <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxCells {
		return s
	}
	if maxCells == 1 {
		return "…"
	}
	var out strings.Builder
	cur := 0
	for _, r := range s {
		rw := lipgloss.Width(string(r))
		if cur+rw > maxCells-1 {
			break
		}
		out.WriteRune(r)
		cur += rw
	}
	out.WriteRune('…')
	return out.String()
}

func wrapText(s string, width int) []string {
	if width <= 4 {
		return []string{fitWidth(s, maxInt(1, width))}
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		var line strings.Builder
		for _, r := range para {
			if lipgloss.Width(line.String()+string(r)) > width {
				out = append(out, line.String())
				line.Reset()
			}
			line.WriteRune(r)
		}
		if line.Len() > 0 {
			out = append(out, line.String())
		}
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
