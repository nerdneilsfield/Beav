package tui

import (
	"context"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
)

// ScopeInfo describes the target cleaning scope displayed in the TUI header and hints.
// ScopeInfo 描述交互界面顶部与提示栏展示的目标清理作用域信息。
type ScopeInfo struct {
	Scope  model.Scope
	Home   string
	IsRoot bool
}

// FileSample represents a candidate file discovered during read-only scanning.
// FileSample 表示只读扫描期间发现的候选清理文件样本。
type FileSample struct {
	Path string
	Size int64
}

// CleanerScan summarizes the dry-run scan result for a single cleaner.
// CleanerScan 汇总单个清理器的只读扫描结果。
type CleanerScan struct {
	Cleaner           model.Cleaner
	Status            string // "ok" | "skipped" | "error"
	FilesCandidate    int64
	BytesCandidate    int64
	MetricsIncomplete bool
	SkipReason        string
	SkipDetail        string
	ErrorDetail       string
	Samples           []FileSample
	CommandPreview    []string
}

// ScanProgress streams incremental read-only scan progress to the TUI.
// ScanProgress 向交互界面流式传递只读扫描进度。
type ScanProgress struct {
	Total       int
	Completed   int
	CurrentID   string
	CurrentName string
	Item        *CleanerScan
	Done        bool
	Err         error
}

// ExecProgress streams real deletion execution progress to the TUI.
// ExecProgress 向交互界面流式传递真实清理执行进度。
type ExecProgress struct {
	Total             int
	Completed         int
	CurrentID         string
	CurrentName       string
	CleanerID         string
	Status            string // "ok" | "skipped" | "error"
	FilesDeleted      int64
	BytesFreed        int64
	MetricsIncomplete bool
	ErrorDetail       string
	Done              bool
	TotalFreed        int64
	TotalFiles        int64
	Unmeasured        []string
	Errored           int
	LogPath           string
	Err               error
}

// Hooks connects the TUI to the CLI's read-only scanner and cleaner executor.
// Hooks 将交互界面与底层的只读扫描及执行逻辑解耦衔接。
type Hooks struct {
	StartScan func(ctx context.Context) (<-chan ScanProgress, context.CancelFunc)
	StartExec func(ctx context.Context, selectedIDs []string) (<-chan ExecProgress, context.CancelFunc)
}

// Options configures a TUI workbench session.
// Options 配置交互式工作台会话。
type Options struct {
	Translator *i18n.Translator
	Scope      ScopeInfo
	Hooks      Hooks
}

type phase int

const (
	phaseScanning phase = iota
	phaseReview
	phaseConfirm
	phaseExecuting
	phaseDone
)

type rowKind int

const (
	rowHeader rowKind = iota
	rowItem
	rowToggleSkipped
)

type rowSection string

const (
	sectionSafe    rowSection = "safe"
	sectionHeavy   rowSection = "heavy"
	sectionSkipped rowSection = "skipped"
)

type displayRow struct {
	kind    rowKind
	label   string
	count   int
	id      string
	section rowSection
}

type scanMsg ScanProgress
type scanClosedMsg struct{}
type execMsg ExecProgress
type execClosedMsg struct{}

type workbenchModel struct {
	ctx    context.Context
	tr     *i18n.Translator
	scope  ScopeInfo
	hooks  Hooks
	phase  phase
	width  int
	height int

	spin                 spinner.Model
	filterInput          textinput.Model
	filtering            bool
	showHelp             bool
	showSkipped          bool
	narrowDetailExpanded bool
	confirmDeleteFocused bool
	statusMsg            string
	err                  error

	scans      []CleanerScan
	scanByID   map[string]CleanerScan
	selected   map[string]bool
	rows       []displayRow
	cursor     int
	listOffset int

	scanCh             <-chan ScanProgress
	scanCancel         context.CancelFunc
	scanTotal          int
	scanCompleted      int
	currentID          string
	currentName        string
	scanCandidateBytes int64
	scanCandidateFiles int64
	scanNativeCount    int

	execCh        <-chan ExecProgress
	execCancel    context.CancelFunc
	execTotal     int
	execCompleted int
	execOrder     []string
	execItems     map[string]ExecProgress
	execSummary   ExecProgress
}

func newWorkbenchModel(ctx context.Context, opts Options) workbenchModel {
	tr := opts.Translator
	if tr == nil {
		tr = i18n.New(i18n.LangEN)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(colorPrimary)

	ti := textinput.New()
	ti.Prompt = "/ "
	ti.Placeholder = tr.Text("filter by name, id, or tag...", "按名称、ID 或标签搜索...")
	ti.CharLimit = 64
	ti.Width = 36

	return workbenchModel{
		ctx:         ctx,
		tr:          tr,
		scope:       opts.Scope,
		hooks:       opts.Hooks,
		phase:       phaseScanning,
		width:       100,
		height:      30,
		spin:        sp,
		filterInput: ti,
		scanByID:    map[string]CleanerScan{},
		selected:    map[string]bool{},
		execItems:   map[string]ExecProgress{},
	}
}

// Run starts the full-screen Bubble Tea TUI workbench and blocks until the user exits.
// Run 启动全屏 Bubble Tea 交互工作台并阻塞直至用户退出。
func Run(ctx context.Context, opts Options) error {
	m := newWorkbenchModel(ctx, opts)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithContext(ctx))
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if fm, ok := finalModel.(workbenchModel); ok && fm.err != nil {
		return fm.err
	}
	return nil
}

func (m workbenchModel) Init() tea.Cmd {
	nextM, cmd := m.startScan()
	_ = nextM
	// Note: Init cannot return a modified model, so Run/StartScan relies on initial scanCmd
	// created cleanly below via beginScanCmd if Hooks are invoked in Init; however in Bubble Tea,
	// Init creates channels before Update runs, so we wrap channel start inside tea.Cmd!
	return tea.Batch(m.spin.Tick, cmd)
}

type scanStartedMsg struct {
	ch     <-chan ScanProgress
	cancel context.CancelFunc
}

type execStartedMsg struct {
	ch     <-chan ExecProgress
	cancel context.CancelFunc
}

func (m workbenchModel) startScan() (workbenchModel, tea.Cmd) {
	if m.scanCancel != nil {
		m.scanCancel()
		m.scanCancel = nil
	}
	m.phase = phaseScanning
	m.err = nil
	m.statusMsg = ""
	m.scans = nil
	m.scanByID = map[string]CleanerScan{}
	m.selected = map[string]bool{}
	m.rows = nil
	m.cursor = 0
	m.listOffset = 0
	m.scanTotal = 0
	m.scanCompleted = 0
	m.currentID = ""
	m.currentName = ""
	m.scanCandidateBytes = 0
	m.scanCandidateFiles = 0
	m.scanNativeCount = 0

	hooks := m.hooks
	ctx := m.ctx
	return m, func() tea.Msg {
		if hooks.StartScan == nil {
			return scanMsg(ScanProgress{Done: true})
		}
		ch, cancel := hooks.StartScan(ctx)
		return scanStartedMsg{ch: ch, cancel: cancel}
	}
}

func waitScanMsg(ch <-chan ScanProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return scanClosedMsg{}
		}
		return scanMsg(p)
	}
}

func waitExecMsg(ch <-chan ExecProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return execClosedMsg{}
		}
		return execMsg(p)
	}
}

func (m workbenchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		if msg.Height > 0 {
			m.height = msg.Height
		}
		m.ensureCursorVisible()
		return m, nil

	case spinner.TickMsg:
		if m.phase == phaseScanning || m.phase == phaseExecuting {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case scanStartedMsg:
		m.scanCh = msg.ch
		m.scanCancel = msg.cancel
		return m, waitScanMsg(m.scanCh)

	case scanMsg:
		return m.handleScanProgress(ScanProgress(msg))

	case scanClosedMsg:
		if m.phase == phaseScanning {
			return m.finishScan()
		}
		return m, nil

	case execStartedMsg:
		m.execCh = msg.ch
		m.execCancel = msg.cancel
		return m, waitExecMsg(m.execCh)

	case execMsg:
		return m.handleExecProgress(ExecProgress(msg))

	case execClosedMsg:
		if m.phase == phaseExecuting {
			if m.execCancel != nil {
				m.execCancel()
				m.execCancel = nil
			}
			m.phase = phaseDone
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m workbenchModel) handleScanProgress(p ScanProgress) (workbenchModel, tea.Cmd) {
	if p.Total > 0 {
		m.scanTotal = p.Total
	}
	if p.Completed > m.scanCompleted {
		m.scanCompleted = p.Completed
	}
	if p.CurrentID != "" {
		m.currentID = p.CurrentID
		m.currentName = p.CurrentName
	}
	if p.Item != nil {
		item := *p.Item
		m.upsertScan(item)
		if isActionableScan(item) {
			if item.BytesCandidate > 0 || item.FilesCandidate > 0 {
				m.scanCandidateBytes += item.BytesCandidate
				m.scanCandidateFiles += item.FilesCandidate
			} else if item.MetricsIncomplete || item.SkipReason == string(model.SkipReasonDryRunUnsupported) {
				m.scanNativeCount++
			}
		}
	}
	if p.Err != nil {
		m.err = p.Err
	}
	if p.Done {
		return m.finishScan()
	}
	if m.scanCh != nil {
		return m, waitScanMsg(m.scanCh)
	}
	return m, nil
}

func (m *workbenchModel) upsertScan(item CleanerScan) {
	if _, exists := m.scanByID[item.Cleaner.ID]; !exists {
		m.scans = append(m.scans, item)
	} else {
		for i := range m.scans {
			if m.scans[i].Cleaner.ID == item.Cleaner.ID {
				m.scans[i] = item
				break
			}
		}
	}
	m.scanByID[item.Cleaner.ID] = item
}

func (m workbenchModel) finishScan() (workbenchModel, tea.Cmd) {
	if m.scanCancel != nil {
		m.scanCancel()
		m.scanCancel = nil
	}
	m.scanCh = nil
	m.selected = map[string]bool{}
	for _, s := range m.scans {
		if isActionableScan(s) && s.Cleaner.EffectiveImpact() != model.ImpactHeavy {
			m.selected[s.Cleaner.ID] = true
		} else {
			m.selected[s.Cleaner.ID] = false
		}
	}
	m.phase = phaseReview
	m.rebuildRows()
	return m, nil
}

func (m workbenchModel) handleExecProgress(p ExecProgress) (workbenchModel, tea.Cmd) {
	if p.Total > 0 {
		m.execTotal = p.Total
	}
	if p.Completed > m.execCompleted {
		m.execCompleted = p.Completed
	}
	if p.CurrentID != "" {
		m.currentID = p.CurrentID
		m.currentName = p.CurrentName
	}
	if p.CleanerID != "" {
		m.execItems[p.CleanerID] = p
	}
	if p.Err != nil {
		m.err = p.Err
	}
	if p.Done {
		m.execSummary = p
		if m.execCancel != nil {
			m.execCancel()
			m.execCancel = nil
		}
		m.execCh = nil
		m.phase = phaseDone
		return m, nil
	}
	if m.execCh != nil {
		return m, waitExecMsg(m.execCh)
	}
	return m, nil
}

func (m workbenchModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m.quitAll()
	}

	switch m.phase {
	case phaseScanning:
		if key == "q" || key == "esc" {
			return m.quitAll()
		}
		return m, nil

	case phaseReview:
		return m.handleReviewKey(msg)

	case phaseConfirm:
		switch key {
		case "left", "right", "h", "l", "tab":
			m.confirmDeleteFocused = !m.confirmDeleteFocused
		case "enter":
			if m.confirmDeleteFocused {
				return m.beginExecution()
			}
			m.phase = phaseReview
		case "y", "Y":
			return m.beginExecution()
		case "n", "N", "esc":
			m.phase = phaseReview
		case "q":
			return m.quitAll()
		}
		return m, nil

	case phaseExecuting:
		// Wait until execution finishes unless ctrl+c was pressed (handled above).
		return m, nil

	case phaseDone:
		switch key {
		case "r":
			nextM, cmd := m.startScan()
			return nextM, tea.Batch(nextM.spin.Tick, cmd)
		case "q", "enter", "esc":
			return m.quitAll()
		}
		return m, nil
	}
	return m, nil
}

func (m workbenchModel) handleReviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if m.showHelp {
		switch key {
		case "?", "esc", "enter", "q":
			m.showHelp = false
		}
		return m, nil
	}

	if m.filtering {
		switch key {
		case "esc":
			m.filterInput.SetValue("")
			m.filterInput.Blur()
			m.filtering = false
			m.rebuildRows()
			return m, nil
		case "enter":
			m.filterInput.Blur()
			m.filtering = false
			m.rebuildRows()
			return m, nil
		default:
			var cmd tea.Cmd
			m.filterInput, cmd = m.filterInput.Update(msg)
			m.rebuildRows()
			return m, cmd
		}
	}

	switch key {
	case "q":
		return m.quitAll()
	case "esc":
		if m.filterInput.Value() != "" {
			m.filterInput.SetValue("")
			m.rebuildRows()
		}
		return m, nil
	case "/":
		m.filtering = true
		m.statusMsg = ""
		return m, m.filterInput.Focus()
	case "?":
		m.showHelp = true
		return m, nil
	case "up", "k":
		m.moveCursor(-1)
	case "down", "j":
		m.moveCursor(1)
	case "pgup":
		m.moveCursor(-m.listPageSize())
	case "pgdown":
		m.moveCursor(m.listPageSize())
	case "home", "g":
		m.jumpCursor(true)
	case "end", "G":
		m.jumpCursor(false)
	case " ":
		m.toggleCurrentRow()
	case "a":
		m.toggleAllSafe()
	case "s":
		m.showSkipped = !m.showSkipped
		m.rebuildRows()
	case "d", "tab":
		m.narrowDetailExpanded = !m.narrowDetailExpanded
	case "r":
		nextM, cmd := m.startScan()
		return nextM, tea.Batch(nextM.spin.Tick, cmd)
	case "enter", "c":
		if m.selectedCount() == 0 {
			m.statusMsg = m.tr.Text("Select at least one cleaner with Space first.", "请先按空格键勾选至少一个清理项。")
			return m, nil
		}
		m.statusMsg = ""
		m.confirmDeleteFocused = false
		m.phase = phaseConfirm
	}
	return m, nil
}

func (m workbenchModel) quitAll() (workbenchModel, tea.Cmd) {
	if m.scanCancel != nil {
		m.scanCancel()
		m.scanCancel = nil
	}
	if m.execCancel != nil {
		m.execCancel()
		m.execCancel = nil
	}
	return m, tea.Quit
}

func (m workbenchModel) beginExecution() (workbenchModel, tea.Cmd) {
	selectedIDs := m.orderedSelectedIDs()
	if len(selectedIDs) == 0 {
		m.phase = phaseReview
		return m, nil
	}
	m.phase = phaseExecuting
	m.execOrder = selectedIDs
	m.execTotal = len(selectedIDs)
	m.execCompleted = 0
	m.currentID = ""
	m.currentName = ""
	m.execItems = map[string]ExecProgress{}
	m.execSummary = ExecProgress{}

	hooks := m.hooks
	ctx := m.ctx
	idsCopy := append([]string(nil), selectedIDs...)
	cmd := func() tea.Msg {
		if hooks.StartExec == nil {
			return execMsg(ExecProgress{Done: true})
		}
		ch, cancel := hooks.StartExec(ctx, idsCopy)
		return execStartedMsg{ch: ch, cancel: cancel}
	}
	return m, tea.Batch(m.spin.Tick, cmd)
}

func (m *workbenchModel) toggleCurrentRow() {
	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return
	}
	row := m.rows[m.cursor]
	if row.kind == rowToggleSkipped {
		m.showSkipped = !m.showSkipped
		m.rebuildRows()
		return
	}
	if row.kind != rowItem || row.section == sectionSkipped {
		return
	}
	m.selected[row.id] = !m.selected[row.id]
	m.statusMsg = ""
}

func (m *workbenchModel) toggleAllSafe() {
	var safeIDs []string
	allSelected := true
	for _, s := range m.scans {
		if !isActionableScan(s) || s.Cleaner.EffectiveImpact() == model.ImpactHeavy {
			continue
		}
		safeIDs = append(safeIDs, s.Cleaner.ID)
		if !m.selected[s.Cleaner.ID] {
			allSelected = false
		}
	}
	target := !allSelected
	for _, id := range safeIDs {
		m.selected[id] = target
	}
	m.statusMsg = ""
}

func (m *workbenchModel) rebuildRows() {
	var prevID string
	var prevKind rowKind
	hasPrev := m.cursor >= 0 && m.cursor < len(m.rows)
	if hasPrev {
		prevID = m.rows[m.cursor].id
		prevKind = m.rows[m.cursor].kind
	}

	filter := strings.ToLower(strings.TrimSpace(m.filterInput.Value()))
	var safeItems, heavyItems, skippedItems []CleanerScan
	for _, s := range m.scans {
		if filter != "" && !m.matchesFilter(s.Cleaner, filter) {
			continue
		}
		switch {
		case isActionableScan(s) && s.Cleaner.EffectiveImpact() == model.ImpactHeavy:
			heavyItems = append(heavyItems, s)
		case isActionableScan(s):
			safeItems = append(safeItems, s)
		default:
			skippedItems = append(skippedItems, s)
		}
	}

	sortActionable := func(list []CleanerScan) {
		sort.Slice(list, func(i, j int) bool {
			if list[i].BytesCandidate != list[j].BytesCandidate {
				return list[i].BytesCandidate > list[j].BytesCandidate
			}
			if list[i].FilesCandidate != list[j].FilesCandidate {
				return list[i].FilesCandidate > list[j].FilesCandidate
			}
			return list[i].Cleaner.ID < list[j].Cleaner.ID
		})
	}
	sortActionable(safeItems)
	sortActionable(heavyItems)
	sort.Slice(skippedItems, func(i, j int) bool {
		ri, rj := skippedRank(skippedItems[i]), skippedRank(skippedItems[j])
		if ri != rj {
			return ri < rj
		}
		return skippedItems[i].Cleaner.ID < skippedItems[j].Cleaner.ID
	})

	rows := make([]displayRow, 0, len(m.scans)+4)
	if len(safeItems) > 0 {
		rows = append(rows, displayRow{
			kind:  rowHeader,
			label: m.tr.Text("Safe & Reclaimable", "常规清理项"),
			count: len(safeItems),
		})
		for _, s := range safeItems {
			rows = append(rows, displayRow{kind: rowItem, id: s.Cleaner.ID, section: sectionSafe})
		}
	}
	if len(heavyItems) > 0 {
		rows = append(rows, displayRow{
			kind:  rowHeader,
			label: m.tr.Text("Heavy (manual opt-in)", "重度清理项（需手动勾选）"),
			count: len(heavyItems),
		})
		for _, s := range heavyItems {
			rows = append(rows, displayRow{kind: rowItem, id: s.Cleaner.ID, section: sectionHeavy})
		}
	}
	if len(skippedItems) > 0 {
		rows = append(rows, displayRow{
			kind:  rowToggleSkipped,
			count: len(skippedItems),
		})
		if m.showSkipped || filter != "" {
			for _, s := range skippedItems {
				rows = append(rows, displayRow{kind: rowItem, id: s.Cleaner.ID, section: sectionSkipped})
			}
		}
	}

	m.rows = rows
	m.cursor = -1
	if hasPrev {
		for i, r := range m.rows {
			if prevKind == rowToggleSkipped && r.kind == rowToggleSkipped {
				m.cursor = i
				break
			}
			if prevKind == rowItem && r.kind == rowItem && r.id == prevID {
				m.cursor = i
				break
			}
		}
	}
	if m.cursor == -1 {
		for i, r := range m.rows {
			if isFocusableRow(r) {
				m.cursor = i
				break
			}
		}
	}
	m.ensureCursorVisible()
}

func (m workbenchModel) matchesFilter(c model.Cleaner, q string) bool {
	if strings.Contains(strings.ToLower(c.ID), q) ||
		strings.Contains(strings.ToLower(c.Name), q) ||
		strings.Contains(strings.ToLower(m.tr.CleanerName(c)), q) {
		return true
	}
	for _, tag := range c.Tags {
		if strings.Contains(strings.ToLower(tag), q) {
			return true
		}
	}
	return false
}

func isActionableScan(s CleanerScan) bool {
	if s.Status == "ok" && (s.FilesCandidate > 0 || s.MetricsIncomplete) {
		return true
	}
	return s.SkipReason == string(model.SkipReasonDryRunUnsupported)
}

func skippedRank(s CleanerScan) int {
	if s.Status == "error" {
		return 0
	}
	if s.SkipReason == string(model.SkipReasonRunningProcess) || s.SkipReason == "cache_busy" || s.SkipReason == string(model.SkipReasonRuntimeBusy) {
		return 1
	}
	if s.Status == "skipped" && !i18n.IsQuietSkip(s.SkipReason) {
		return 2
	}
	if s.Status == "skipped" {
		return 3
	}
	return 4 // noop
}

func isFocusableRow(r displayRow) bool {
	return r.kind == rowItem || r.kind == rowToggleSkipped
}

func (m *workbenchModel) moveCursor(delta int) {
	if len(m.rows) == 0 || delta == 0 {
		return
	}
	step := 1
	remaining := delta
	if delta < 0 {
		step = -1
		remaining = -delta
	}
	idx := m.cursor
	if idx < 0 || idx >= len(m.rows) {
		m.jumpCursor(true)
		return
	}
	lastValid := idx
	for remaining > 0 {
		idx += step
		if idx < 0 || idx >= len(m.rows) {
			break
		}
		if isFocusableRow(m.rows[idx]) {
			lastValid = idx
			remaining--
		}
	}
	m.cursor = lastValid
	m.ensureCursorVisible()
}

func (m *workbenchModel) jumpCursor(start bool) {
	if len(m.rows) == 0 {
		m.cursor = -1
		return
	}
	if start {
		for i, r := range m.rows {
			if isFocusableRow(r) {
				m.cursor = i
				break
			}
		}
	} else {
		for i := len(m.rows) - 1; i >= 0; i-- {
			if isFocusableRow(m.rows[i]) {
				m.cursor = i
				break
			}
		}
	}
	m.ensureCursorVisible()
}

func (m workbenchModel) listPageSize() int {
	h := m.listViewportHeight()
	if h < 4 {
		return 4
	}
	return h - 1
}

func (m workbenchModel) listViewportHeight() int {
	avail := m.height - 9
	if m.width < 96 {
		if m.narrowDetailExpanded {
			avail = m.height/2 - 6
		} else {
			avail = m.height - 17
		}
	}
	if avail < 6 {
		avail = 6
	}
	return avail
}

func (m *workbenchModel) ensureCursorVisible() {
	if m.cursor < 0 || len(m.rows) == 0 {
		m.listOffset = 0
		return
	}
	vp := m.listViewportHeight()
	if m.cursor < m.listOffset {
		m.listOffset = m.cursor
		if m.listOffset > 0 && m.rows[m.listOffset-1].kind == rowHeader {
			m.listOffset--
		}
	}
	if m.cursor >= m.listOffset+vp {
		m.listOffset = m.cursor - vp + 1
	}
	if m.listOffset < 0 {
		m.listOffset = 0
	}
}

func (m workbenchModel) selectedCount() int {
	n := 0
	for _, s := range m.scans {
		if isActionableScan(s) && m.selected[s.Cleaner.ID] {
			n++
		}
	}
	return n
}

func (m workbenchModel) orderedSelectedIDs() []string {
	var safeIDs, heavyIDs []string
	for _, s := range m.scans {
		if !isActionableScan(s) || !m.selected[s.Cleaner.ID] {
			continue
		}
		if s.Cleaner.EffectiveImpact() == model.ImpactHeavy {
			heavyIDs = append(heavyIDs, s.Cleaner.ID)
		} else {
			safeIDs = append(safeIDs, s.Cleaner.ID)
		}
	}
	return append(safeIDs, heavyIDs...)
}

func (m workbenchModel) selectionTotals() (count int, bytes int64, files int64, heavy []CleanerScan, native []CleanerScan) {
	for _, s := range m.scans {
		if !isActionableScan(s) || !m.selected[s.Cleaner.ID] {
			continue
		}
		count++
		bytes += s.BytesCandidate
		files += s.FilesCandidate
		if s.Cleaner.EffectiveImpact() == model.ImpactHeavy {
			heavy = append(heavy, s)
		}
		if s.MetricsIncomplete || s.SkipReason == string(model.SkipReasonDryRunUnsupported) {
			native = append(native, s)
		}
	}
	return count, bytes, files, heavy, native
}
