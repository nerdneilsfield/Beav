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
	switch m.phase {
	case phaseScanning:
		return m.renderScanningView()
	case phaseReview:
		if m.showHelp {
			return m.renderHelpView()
		}
		return m.renderReviewView()
	case phaseConfirm:
		return m.renderConfirmView()
	case phaseExecuting:
		return m.renderExecutingView()
	case phaseDone:
		return m.renderDoneView()
	default:
		return ""
	}
}

func (m workbenchModel) renderHeader(badge string) string {
	scopeDesc := m.tr.ScopeLabel(m.scope.Scope)
	if m.scope.Home != "" && m.scope.Scope != model.ScopeSystem {
		scopeDesc += " (" + m.scope.Home + ")"
	}
	title := styleTitle.Render("Beav") + "  " + badge + "  " + styleSubtle.Render(scopeDesc)
	return title
}

func (m workbenchModel) renderScanningView() string {
	badge := styleBadgeReadOnly.Render(m.tr.Text("READ-ONLY SCAN", "只读扫描"))
	header := m.renderHeader(badge)

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
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

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
	b.WriteString(header + "\n\n")
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
	b.WriteString(styleMuted.Render(m.tr.Text("Press q or Esc to cancel scan.", "按 q 或 Esc 取消扫描。")))

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m workbenchModel) renderReviewView() string {
	badge := styleBadgeReadOnly.Render(m.tr.Text("REVIEW PLAN", "清理清单"))
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
	header := m.renderHeader(badge) + "\n" + styleSubtle.Render(summary)

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
		leftPane := styleFocusedPane.Width(leftW).Height(paneH).Render(m.renderListPane(leftW - 4))
		rightPane := stylePane.Width(rightW).Height(paneH).Render(m.renderDetailPane(rightW-4, paneH-2))
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
	parts := []string{header}
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
	return lipgloss.NewStyle().Padding(0, 1).Render(strings.Join(parts, "\n"))
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

func (m workbenchModel) renderListPane(innerWidth int) string {
	if len(m.rows) == 0 {
		if m.filterInput.Value() != "" {
			return styleMuted.Render(m.tr.Text("No cleaners match the current filter. Press Esc to clear.", "没有匹配当前筛选条件的清理项，按 Esc 清除筛选。"))
		}
		return styleMuted.Render(m.tr.Text("No cleaners found for this scope.", "当前作用域下没有可用的清理项。"))
	}

	vp := m.listViewportHeight()
	end := m.listOffset + vp
	if end > len(m.rows) {
		end = len(m.rows)
	}
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
			lines = append(lines, m.renderItemRow(s, r.section, focused, innerWidth))
		}
	}
	return strings.Join(lines, "\n")
}

func (m workbenchModel) renderItemRow(s CleanerScan, section rowSection, focused bool, innerWidth int) string {
	pointer := "  "
	if focused {
		pointer = "> "
	}

	var check string
	switch {
	case section == sectionSkipped && s.Status == "error":
		check = styleDangerText.Render("[!]")
	case section == sectionSkipped:
		check = styleMuted.Render("[-]")
	case m.selected[s.Cleaner.ID]:
		check = styleBadgeSafe.Render("[x]")
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

	badge := ""
	if s.Cleaner.EffectiveImpact() == model.ImpactHeavy && section != sectionSkipped {
		badge = " " + styleBadgeHeavy.Render(m.tr.Text("[heavy]", "[重度]"))
	}

	name := m.tr.CleanerName(s.Cleaner)
	// Prefix cell width: pointer(2) + check(3) + space(1) = 6
	reservedRight := lipgloss.Width(right) + lipgloss.Width(badge) + 2
	nameWidth := innerWidth - 6 - reservedRight
	if nameWidth < 12 {
		nameWidth = 12
	}
	trimmedName := fitWidth(name, nameWidth)
	leftPart := fmt.Sprintf("%s%s %s%s", pointer, check, trimmedName, badge)
	gap := innerWidth - lipgloss.Width(leftPart) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := leftPart + strings.Repeat(" ", gap) + styleSubtle.Render(right)
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
			lines = append(lines, styleMuted.Render(fitWidth(s.SkipDetail, innerWidth)))
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
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

func (m workbenchModel) renderReviewFooter() string {
	var hints string
	if m.filtering {
		hints = m.tr.Text("Type to filter  |  Enter apply  |  Esc clear & cancel", "输入关键词搜索  |  Enter 应用  |  Esc 清除并退出")
	} else {
		hints = m.tr.Text(
			"↑/↓ Move  Space Toggle  a All safe  s Show skipped  / Filter  r Rescan  Enter Clean  ? Help  q Quit",
			"↑/↓ 移动  空格 勾选  a 全选常规  s 显示跳过  / 搜索  r 重扫  Enter 清理  ? 帮助  q 退出",
		)
	}
	out := styleSubtle.Render(fitWidth(hints, maxInt(20, m.width-4)))
	if !m.scope.IsRoot && m.scope.Scope == model.ScopeUser {
		sysHint := m.tr.Text(
			"Tip: system caches require `sudo beav --system`",
			"提示：清理系统级缓存（apt、journal、Docker 等）请运行 `sudo beav --system`",
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
	badge := styleBadgeExec.Render(m.tr.Text("CLEANING", "正在清理"))
	header := m.renderHeader(badge)

	var b strings.Builder
	b.WriteString(header + "\n\n")
	fmt.Fprintf(&b, "%s %s (%d/%d)\n\n",
		m.spin.View(),
		m.tr.Text("Running selected cleaners...", "正在清理已选项目..."),
		m.execCompleted, m.execTotal,
	)

	maxRows := maxInt(6, m.height-10)
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

	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m workbenchModel) renderDoneView() string {
	badge := styleBadgeDone.Render(m.tr.Text("DONE", "清理完成"))
	header := m.renderHeader(badge)

	var b strings.Builder
	b.WriteString(header + "\n\n")
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

	b.WriteString("\n" + styleSubtle.Render(m.tr.Text("Press r to rescan, or q / Enter to exit.", "按 r 重新扫描，按 q 或 Enter 退出。")))
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m workbenchModel) renderHelpView() string {
	var b strings.Builder
	b.WriteString(styleTitle.Render(m.tr.Text("Beav TUI Help", "Beav 交互操作说明")) + "\n\n")
	b.WriteString(m.tr.Text(
		"Navigation & Selection:\n"+
			"  ↑/↓, j/k, PgUp/PgDn  Move cursor\n"+
			"  Space                Toggle current cleaner (or expand skipped group)\n"+
			"  a                    Select / deselect all safe cleaners (never selects heavy)\n"+
			"  s                    Show / hide skipped and empty cleaners\n"+
			"  /                    Search by name, ID, or tag (Esc clears)\n"+
			"  r                    Re-run read-only scan\n"+
			"  Enter / c            Proceed to confirmation dialog\n"+
			"  q / Ctrl+C           Quit without deleting\n\n"+
			"Labels:\n"+
			"  [heavy]  Large reusable caches or tool versions; never selected automatically.\n"+
			"  native   Runs the tool's own cleanup command; byte total may not be measurable beforehand.\n",
		"操作快捷键：\n"+
			"  ↑/↓、j/k、PgUp/PgDn  移动光标\n"+
			"  空格 / Space         勾选或取消当前清理项（在折叠行上展开跳过项）\n"+
			"  a                    全选/取消全选常规项（绝不会自动勾选重度项）\n"+
			"  s                    显示/隐藏已跳过与无需清理的项目\n"+
			"  /                    按名称、ID 或标签搜索（Esc 清除筛选）\n"+
			"  r                    重新执行只读扫描\n"+
			"  Enter / c            进入最终删除确认界面\n"+
			"  q / Ctrl+C           退出且不删除任何文件\n\n"+
			"标记说明：\n"+
			"  [重度]    体积较大的可重建数据或旧版工具链，默认不勾选，需手动确认。\n"+
			"  原生命令  调用工具自带清理命令（如 uv、apt、docker），执行前无法估算可释放空间。\n",
	))
	b.WriteString("\n" + styleMuted.Render(m.tr.Text("Press ?, Esc, or Enter to close help.", "按 ?、Esc 或 Enter 关闭帮助。")))
	modal := styleHelpModal.MaxWidth(maxInt(50, m.width-6)).Render(b.String())
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
