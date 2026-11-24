package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dengqi/beav/internal/cleaner/safety"
)

type diskEntry struct {
	Name    string
	Path    string
	Size    int64
	IsDir   bool
	ModTime time.Time
	Err     string
}

type diskScanDoneMsg struct {
	path    string
	entries []diskEntry
	total   int64
	err     error
}

type diskDeleteDoneMsg struct {
	path string
	err  error
}

type diskSortMode int

const (
	diskSortSize diskSortMode = iota
	diskSortName
	diskSortMtime
)

func (m workbenchModel) startDiskScan(path string) (workbenchModel, tea.Cmd) {
	if path == "" {
		path = m.scope.Home
	}
	if path == "" {
		path = "/"
	}
	path = filepath.Clean(path)
	m.diskPath = path
	m.diskLoading = true
	m.diskErr = nil
	m.diskEntries = nil
	m.diskTotal = 0
	m.diskCursor = 0
	m.diskOffset = 0
	return m, func() tea.Msg {
		entries, total, err := scanDirectorySizes(path)
		return diskScanDoneMsg{path: path, entries: entries, total: total, err: err}
	}
}

func scanDirectorySizes(dir string) ([]diskEntry, int64, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}
	out := make([]diskEntry, 0, len(dirEntries))
	var total int64
	for _, de := range dirEntries {
		full := filepath.Join(dir, de.Name())
		info, lErr := os.Lstat(full)
		if lErr != nil {
			out = append(out, diskEntry{Name: de.Name(), Path: full, IsDir: de.IsDir(), Err: lErr.Error()})
			continue
		}
		var sz int64
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			sz = info.Size()
		case info.IsDir():
			sz = calcDirSize(full)
		default:
			sz = info.Size()
		}
		total += sz
		out = append(out, diskEntry{
			Name:    de.Name(),
			Path:    full,
			Size:    sz,
			IsDir:   info.IsDir(),
			ModTime: info.ModTime(),
		})
	}
	return out, total, nil
}

func calcDirSize(root string) int64 {
	var total int64
	_ = filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || d.IsDir() {
			return nil
		}
		if info, infoErr := d.Info(); infoErr == nil {
			total += info.Size()
		}
		return nil
	})
	return total
}

func (m *workbenchModel) sortDiskEntries() {
	sort.Slice(m.diskEntries, func(i, j int) bool {
		a, b := m.diskEntries[i], m.diskEntries[j]
		switch m.diskSort {
		case diskSortName:
			return strings.ToLower(a.Name) < strings.ToLower(b.Name)
		case diskSortMtime:
			if !a.ModTime.Equal(b.ModTime) {
				return a.ModTime.After(b.ModTime)
			}
			return a.Size > b.Size
		default:
			if a.Size != b.Size {
				return a.Size > b.Size
			}
			return a.Name < b.Name
		}
	})
}

func (m workbenchModel) handleAnalyzeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.diskConfirmDelete != "" {
		switch key {
		case "y", "Y":
			target := m.diskConfirmDelete
			m.diskConfirmDelete = ""
			policy := m.policy
			home := m.scope.Home
			return m, func() tea.Msg {
				if target == "/" {
					return diskDeleteDoneMsg{path: target, err: fmt.Errorf("refusing to delete '/'")}
				}
				if safety.BlacklistedWithPolicy(target, home, policy) {
					return diskDeleteDoneMsg{path: target, err: fmt.Errorf("path is protected by blacklist (enable bypass in tab 4)")}
				}
				return diskDeleteDoneMsg{path: target, err: os.RemoveAll(target)}
			}
		case "n", "N", "esc", "enter":
			m.diskConfirmDelete = ""
		}
		return m, nil
	}

	switch key {
	case "q":
		return m.quitAll()
	case "up", "k":
		if m.diskCursor > 0 {
			m.diskCursor--
			m.ensureDiskCursorVisible()
		}
	case "down", "j":
		if m.diskCursor < len(m.diskEntries)-1 {
			m.diskCursor++
			m.ensureDiskCursorVisible()
		}
	case "pgup":
		m.diskCursor -= maxInt(1, m.tabContentHeight()-2)
		if m.diskCursor < 0 {
			m.diskCursor = 0
		}
		m.ensureDiskCursorVisible()
	case "pgdown":
		m.diskCursor += maxInt(1, m.tabContentHeight()-2)
		if m.diskCursor >= len(m.diskEntries) {
			m.diskCursor = maxInt(0, len(m.diskEntries)-1)
		}
		m.ensureDiskCursorVisible()
	case "enter", "l", "right":
		if m.diskCursor >= 0 && m.diskCursor < len(m.diskEntries) && m.diskEntries[m.diskCursor].IsDir {
			return m.startDiskScan(m.diskEntries[m.diskCursor].Path)
		}
	case "backspace", "h", "left":
		parent := filepath.Dir(m.diskPath)
		if parent != m.diskPath {
			return m.startDiskScan(parent)
		}
	case "r":
		return m.startDiskScan(m.diskPath)
	case "o":
		m.diskSort = (m.diskSort + 1) % 3
		m.sortDiskEntries()
	case "d", "delete":
		if m.diskCursor >= 0 && m.diskCursor < len(m.diskEntries) {
			m.diskConfirmDelete = m.diskEntries[m.diskCursor].Path
		}
	case "g":
		path := m.diskPath
		allowDel := m.policy.BypassAll
		return m, tea.ExecProcess(newGDUCmdProxy(path, allowDel), func(err error) tea.Msg {
			if err != nil {
				return diskScanDoneMsg{path: path, err: err}
			}
			entries, total, scanErr := scanDirectorySizes(path)
			return diskScanDoneMsg{path: path, entries: entries, total: total, err: scanErr}
		})
	}
	return m, nil
}

func (m *workbenchModel) ensureDiskCursorVisible() {
	vp := maxInt(5, m.tabContentHeight()-4)
	if m.diskCursor < m.diskOffset {
		m.diskOffset = m.diskCursor
	}
	if m.diskCursor >= m.diskOffset+vp {
		m.diskOffset = m.diskCursor - vp + 1
	}
	if m.diskOffset < 0 {
		m.diskOffset = 0
	}
}

func (m workbenchModel) renderAnalyzeTab() string {
	w := maxInt(40, m.width-4)
	h := maxInt(8, m.tabContentHeight())

	if m.diskConfirmDelete != "" {
		var mb strings.Builder
		mb.WriteString(styleDangerText.Render(m.tr.Text("Confirm Delete Path", "确认删除路径")) + "\n\n")
		mb.WriteString(m.diskConfirmDelete + "\n\n")
		mb.WriteString(styleMuted.Render(m.tr.Text("Press y to delete permanently, or n / Esc to cancel.", "按 y 永久删除，按 n 或 Esc 取消。")))
		return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, styleModal.MaxWidth(maxInt(44, w-6)).Render(mb.String()))
	}

	sortName := map[diskSortMode]string{
		diskSortSize:  m.tr.Text("size↓", "按大小↓"),
		diskSortName:  m.tr.Text("name", "按名称"),
		diskSortMtime: m.tr.Text("newest", "按修改时间"),
	}[m.diskSort]

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s   %s %s   %s\n",
		styleSubtle.Render(m.tr.Text("Path:", "当前路径：")),
		styleTitle.Render(fitWidth(m.diskPath, maxInt(16, w-40))),
		styleSubtle.Render(m.tr.Text("Total:", "总大小：")),
		styleBadgeSafe.Render(humanBytes(m.diskTotal)),
		stylePill.Render(m.tr.Text("sort: ", "排序：")+sortName),
	)

	if m.diskStatus != "" {
		b.WriteString(styleWarnText.Render(m.diskStatus) + "\n")
	}

	switch {
	case m.diskLoading:
		fmt.Fprintf(&b, "\n%s %s\n", m.spin.View(), m.tr.Text("Calculating directory sizes...", "正在扫描并统计目录大小..."))
	case m.diskErr != nil:
		b.WriteString("\n" + styleDangerText.Render(m.diskErr.Error()) + "\n")
	case len(m.diskEntries) == 0:
		b.WriteString("\n" + styleMuted.Render(m.tr.Text("Empty directory.", "目录为空。")) + "\n")
	default:
		vp := maxInt(4, h-5)
		end := m.diskOffset + vp
		if end > len(m.diskEntries) {
			end = len(m.diskEntries)
		}
		maxSz := m.diskEntries[0].Size
		for _, ent := range m.diskEntries {
			if ent.Size > maxSz {
				maxSz = ent.Size
			}
		}
		for i := m.diskOffset; i < end; i++ {
			ent := m.diskEntries[i]
			pointer := "  "
			if i == m.diskCursor {
				pointer = "> "
			}
			kind := " "
			if ent.IsDir {
				kind = "/"
			}
			pct := 0.0
			if m.diskTotal > 0 {
				pct = float64(ent.Size) * 100 / float64(m.diskTotal)
			}
			bar := renderRatioBar(ent.Size, maxSz, 10)
			right := fmt.Sprintf("%s  %9s  %5.1f%%", bar, humanBytes(ent.Size), pct)
			nameW := maxInt(12, w-lipgloss.Width(right)-8)
			namePart := fitWidth(ent.Name+kind, nameW)
			gap := maxInt(1, w-4-2-lipgloss.Width(namePart)-lipgloss.Width(right))
			line := pointer + namePart + strings.Repeat(" ", gap) + styleSubtle.Render(right)
			if i == m.diskCursor {
				line = styleActiveRow.Width(w - 4).Render(line)
			}
			b.WriteString(line + "\n")
		}
	}

	pane := styleFocusedPane.Width(w).Height(h).Render(b.String())
	footer := styleMuted.Render(fitWidth(m.tr.Text(
		"↑/↓ Move  Enter Open dir  Backspace Up  o Sort  d Delete  r Rescan  g Launch gdu  1-4 Tabs  q Quit",
		"↑/↓ 移动  Enter 进入目录  Backspace 返回上级  o 排序  d 删除  r 重扫  g 启动 gdu  1-4 切页  q 退出",
	), w))
	return pane + "\n" + footer
}

func renderRatioBar(val, maxVal int64, width int) string {
	if width <= 0 {
		return ""
	}
	if maxVal <= 0 || val <= 0 {
		return styleMuted.Render(strings.Repeat("░", width))
	}
	filled := int(val * int64(width) / maxVal)
	if filled == 0 && val > 0 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	return styleBar.Render(strings.Repeat("█", filled)) + styleMuted.Render(strings.Repeat("░", width-filled))
}

// Keep context import referenced cleanly for future cancellation hooks.
var _ = context.Background
