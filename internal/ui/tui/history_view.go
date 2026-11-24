package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dengqi/beav/internal/oplog"
)

type historyLoadedMsg struct {
	entries []oplog.Entry
	err     error
}

type historyClearedMsg struct {
	err error
}

func (m workbenchModel) loadHistoryCmd() tea.Cmd {
	readFn := m.hooks.ReadHistory
	logPath := m.logPath
	return func() tea.Msg {
		if readFn != nil {
			entries, err := readFn()
			return historyLoadedMsg{entries: entries, err: err}
		}
		if logPath == "" {
			return historyLoadedMsg{}
		}
		entries, err := oplog.ReadEntries(logPath, true, 5)
		return historyLoadedMsg{entries: entries, err: err}
	}
}

func (m workbenchModel) handleHistoryKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.histConfirmClear {
		switch key {
		case "y", "Y":
			m.histConfirmClear = false
			clearFn := m.hooks.ClearHistory
			logPath := m.logPath
			return m, func() tea.Msg {
				if clearFn != nil {
					return historyClearedMsg{err: clearFn()}
				}
				if logPath != "" {
					return historyClearedMsg{err: oplog.Clear(logPath, 5)}
				}
				return historyClearedMsg{}
			}
		case "n", "N", "esc", "enter":
			m.histConfirmClear = false
		}
		return m, nil
	}

	if m.histFiltering {
		switch key {
		case "esc":
			m.histFilter.SetValue("")
			m.histFilter.Blur()
			m.histFiltering = false
			m.histOffset = 0
			return m, nil
		case "enter":
			m.histFilter.Blur()
			m.histFiltering = false
			m.histOffset = 0
			return m, nil
		default:
			var cmd tea.Cmd
			m.histFilter, cmd = m.histFilter.Update(msg)
			m.histOffset = 0
			return m, cmd
		}
	}

	switch key {
	case "q":
		return m.quitAll()
	case "/":
		m.histFiltering = true
		return m, m.histFilter.Focus()
	case "esc":
		if m.histFilter.Value() != "" {
			m.histFilter.SetValue("")
			m.histOffset = 0
		}
	case "up", "k":
		if m.histOffset > 0 {
			m.histOffset--
		}
	case "down", "j":
		filtered := m.filteredHistory()
		vp := maxInt(4, m.tabContentHeight()-5)
		if m.histOffset+vp < len(filtered) {
			m.histOffset++
		}
	case "pgup":
		m.histOffset -= maxInt(1, m.tabContentHeight()-5)
		if m.histOffset < 0 {
			m.histOffset = 0
		}
	case "pgdown":
		filtered := m.filteredHistory()
		vp := maxInt(4, m.tabContentHeight()-5)
		m.histOffset += vp
		if m.histOffset > maxInt(0, len(filtered)-vp) {
			m.histOffset = maxInt(0, len(filtered)-vp)
		}
	case "r":
		return m, m.loadHistoryCmd()
	case "X":
		m.histConfirmClear = true
	}
	return m, nil
}

func (m workbenchModel) filteredHistory() []oplog.Entry {
	q := strings.ToLower(strings.TrimSpace(m.histFilter.Value()))
	if q == "" {
		return m.histEntries
	}
	out := make([]oplog.Entry, 0, len(m.histEntries))
	for _, ent := range m.histEntries {
		if strings.Contains(strings.ToLower(ent.CleanerID), q) || strings.Contains(strings.ToLower(ent.Path), q) {
			out = append(out, ent)
		}
	}
	return out
}

func (m workbenchModel) renderHistoryTab() string {
	w := maxInt(40, m.width-4)
	h := maxInt(8, m.tabContentHeight())

	if m.histConfirmClear {
		var mb strings.Builder
		mb.WriteString(styleDangerText.Render(m.tr.Text("Clear Operation History?", "确认清空操作历史？")) + "\n\n")
		mb.WriteString(m.tr.Text("This deletes recorded operation logs from disk.", "此操作将删除磁盘上的操作日志记录。") + "\n\n")
		mb.WriteString(styleMuted.Render(m.tr.Text("Press y to confirm, n / Esc to cancel.", "按 y 确认清空，按 n 或 Esc 取消。")))
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, styleModal.MaxWidth(maxInt(44, w-6)).Render(mb.String()))
	}

	filtered := m.filteredHistory()
	var totalBytes int64
	for _, ent := range filtered {
		totalBytes += ent.Size
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("%s %d   %s %s\n",
		styleSubtle.Render(m.tr.Text("Operations:", "操作记录数：")),
		len(filtered),
		styleSubtle.Render(m.tr.Text("Total freed:", "累计释放：")),
		styleBadgeSafe.Render(humanBytes(totalBytes)),
	))
	if m.histFiltering || m.histFilter.Value() != "" {
		b.WriteString(styleSubtle.Render(m.tr.Text("Filter:", "筛选：")) + " " + m.histFilter.View() + "\n")
	}

	if m.histErr != nil {
		b.WriteString("\n" + styleDangerText.Render(m.histErr.Error()) + "\n")
	} else if len(filtered) == 0 {
		b.WriteString("\n" + styleMuted.Render(m.tr.Text("No cleanup operations recorded.", "暂无清理操作记录。")) + "\n")
	} else {
		b.WriteString(styleSectionHeader.Render(fmt.Sprintf("%-19s  %-22s  %9s  %s",
			m.tr.Text("TIME", "时间"),
			m.tr.Text("CLEANER", "清理项"),
			m.tr.Text("SIZE", "大小"),
			m.tr.Text("PATH", "路径"),
		)) + "\n")
		vp := maxInt(3, h-6)
		end := m.histOffset + vp
		if end > len(filtered) {
			end = len(filtered)
		}
		pathW := maxInt(16, w-58)
		for i := m.histOffset; i < end; i++ {
			ent := filtered[i]
			ts := ent.Timestamp.Local().Format("2006-01-02 15:04:05")
			cid := fitWidth(ent.CleanerID, 22)
			sz := humanBytes(ent.Size)
			p := fitWidth(shortenHome(ent.Path, m.scope.Home), pathW)
			b.WriteString(fmt.Sprintf("%-19s  %-22s  %9s  %s\n", ts, cid, sz, styleMuted.Render(p)))
		}
	}

	pane := styleFocusedPane.Width(w).Height(h).Render(b.String())
	footer := styleMuted.Render(fitWidth(m.tr.Text(
		"↑/↓ Scroll  / Filter  r Refresh  X Clear log  1-4 Tabs  q Quit",
		"↑/↓ 滚动  / 搜索  r 刷新  X 清空日志  1-4 切页  q 退出",
	), w))
	return pane + "\n" + footer
}
