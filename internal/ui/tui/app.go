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
	"github.com/dengqi/beav/internal/cleaner/safety"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/oplog"
	"golang.org/x/sys/unix"
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

// Hooks connects the TUI to the CLI's read-only scanner, executor, config, and history.
// Hooks 将交互界面与底层的只读扫描、执行、配置及历史记录逻辑衔接。
type Hooks struct {
	StartScan            func(ctx context.Context) (<-chan ScanProgress, context.CancelFunc)
	StartExec            func(ctx context.Context, selectedIDs []string) (<-chan ExecProgress, context.CancelFunc)
	StartScanWithOptions func(ctx context.Context, policy safety.Policy, minAgeOverride string) (<-chan ScanProgress, context.CancelFunc)
	StartExecWithOptions func(ctx context.Context, policy safety.Policy, minAgeOverride string, selectedIDs []string) (<-chan ExecProgress, context.CancelFunc)
	SaveConfig           func(cfg *config.Config) error
	ReadHistory          func() ([]oplog.Entry, error)
	ClearHistory         func() error
}

// Options configures a TUI workbench session.
// Options 配置交互式工作台会话。
type Options struct {
	Translator *i18n.Translator
	Scope      ScopeInfo
	Version    string
	ConfigDir  string
	Config     *config.Config
	Policy     safety.Policy
	LogPath    string
	InitialTab int
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

type activeTab int

const (
	tabClean activeTab = iota
	tabAnalyze
	tabHistory
	tabConfig
)

type cleanSortMode int

const (
	cleanSortSize cleanSortMode = iota
	cleanSortName
	cleanSortFiles
	cleanSortImpact
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

type fsStat struct {
	total uint64
	used  uint64
	avail uint64
	valid bool
}

type workbenchModel struct {
	ctx       context.Context
	tr        *i18n.Translator
	scope     ScopeInfo
	version   string
	configDir string
	cfg       *config.Config
	policy    safety.Policy
	logPath   string
	hooks     Hooks

	activeTab activeTab
	phase     phase
	width     int
	height    int
	diskStat  fsStat

	spin                 spinner.Model
	filterInput          textinput.Model
	filtering            bool
	showHelp             bool
	showSkipped          bool
	narrowDetailExpanded bool
	detailFocused        bool
	detailScroll         int
	confirmDeleteFocused bool
	statusMsg            string
	err                  error

	sortMode       cleanSortMode
	tagFilterIdx   int
	tagOptions     []string
	agePresetIdx   int
	minAgeOverride string

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

	// Analyze tab state.
	diskPath          string
	diskLoading       bool
	diskErr           error
	diskStatus        string
	diskEntries       []diskEntry
	diskTotal         int64
	diskCursor        int
	diskOffset        int
	diskSort          diskSortMode
	diskConfirmDelete string

	// History tab state.
	histEntries      []oplog.Entry
	histErr          error
	histOffset       int
	histFilter       textinput.Model
	histFiltering    bool
	histConfirmClear bool

	// Config tab state.
	cfgCursor    int
	cfgOffset    int
	cfgStatus    string
	cfgAdding    bool
	cfgAddTarget configListTarget
	cfgAddInput  textinput.Model
}

var agePresets = []string{"", "0d", "3d", "7d", "14d", "30d"}

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

	histTi := textinput.New()
	histTi.Prompt = "/ "
	histTi.Placeholder = tr.Text("filter history by cleaner or path...", "按清理项或路径搜索历史...")
	histTi.CharLimit = 64
	histTi.Width = 36

	cfgTi := textinput.New()
	cfgTi.Prompt = "+ "
	cfgTi.Placeholder = tr.Text("e.g. ~/.cache/custom or /opt/cache", "例如 ~/.cache/custom 或 /opt/cache")
	cfgTi.CharLimit = 128
	cfgTi.Width = 42

	tab := tabClean
	if opts.InitialTab >= int(tabClean) && opts.InitialTab <= int(tabConfig) {
		tab = activeTab(opts.InitialTab)
	}

	return workbenchModel{
		ctx:         ctx,
		tr:          tr,
		scope:       opts.Scope,
		version:     opts.Version,
		configDir:   opts.ConfigDir,
		cfg:         opts.Config,
		policy:      opts.Policy,
		logPath:     opts.LogPath,
		hooks:       opts.Hooks,
		activeTab:   tab,
		phase:       phaseScanning,
		width:       100,
		height:      30,
		diskStat:    readFSStat(opts.Scope.Home),
		spin:        sp,
		filterInput: ti,
		histFilter:  histTi,
		cfgAddInput: cfgTi,
		tagOptions:  []string{""},
		scanByID:    map[string]CleanerScan{},
		selected:    map[string]bool{},
		execItems:   map[string]ExecProgress{},
		diskPath:    opts.Scope.Home,
	}
}

func readFSStat(path string) fsStat {
	if path == "" {
		path = "/"
	}
	var fs unix.Statfs_t
	if err := unix.Statfs(path, &fs); err != nil || fs.Blocks == 0 {
		return fsStat{}
	}
	bsize := uint64(fs.Bsize) //nolint:gosec // Linux statfs block size is positive.
	total := fs.Blocks * bsize
	free := fs.Bfree * bsize
	avail := fs.Bavail * bsize
	var used uint64
	if total >= free {
		used = total - free
	}
	return fsStat{total: total, used: used, avail: avail, valid: true}
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
	_, scanCmd := m.startScan()
	cmds := []tea.Cmd{m.spin.Tick, scanCmd}
	if m.activeTab == tabAnalyze {
		var diskCmd tea.Cmd
		_, diskCmd = m.startDiskScan(m.diskPath)
		cmds = append(cmds, diskCmd)
	}
	if m.activeTab == tabHistory {
		cmds = append(cmds, m.loadHistoryCmd())
	}
	return tea.Batch(cmds...)
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
	m.detailScroll = 0
	m.scanTotal = 0
	m.scanCompleted = 0
	m.currentID = ""
	m.currentName = ""
	m.scanCandidateBytes = 0
	m.scanCandidateFiles = 0
	m.scanNativeCount = 0

	hooks := m.hooks
	ctx := m.ctx
	policy := m.policy
	ageOverride := m.minAgeOverride
	return m, func() tea.Msg {
		if hooks.StartScanWithOptions != nil {
			ch, cancel := hooks.StartScanWithOptions(ctx, policy, ageOverride)
			return scanStartedMsg{ch: ch, cancel: cancel}
		}
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
		if m.phase == phaseScanning || m.phase == phaseExecuting || m.diskLoading {
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
			m.diskStat = readFSStat(m.scope.Home)
		}
		return m, nil

	case diskScanDoneMsg:
		m.diskLoading = false
		m.diskPath = msg.path
		m.diskErr = msg.err
		m.diskEntries = msg.entries
		m.diskTotal = msg.total
		m.sortDiskEntries()
		m.diskCursor = 0
		m.diskOffset = 0
		return m, nil

	case diskDeleteDoneMsg:
		if msg.err != nil {
			m.diskStatus = msg.err.Error()
			return m, nil
		}
		m.diskStatus = m.tr.Textf("Deleted: %s", "已删除：%s", msg.path)
		m.diskStat = readFSStat(m.scope.Home)
		return m.startDiskScan(m.diskPath)

	case historyLoadedMsg:
		m.histEntries = msg.entries
		m.histErr = msg.err
		m.histOffset = 0
		return m, nil

	case historyClearedMsg:
		if msg.err != nil {
			m.histErr = msg.err
			return m, nil
		}
		m.histEntries = nil
		m.histOffset = 0
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
	tagSet := map[string]bool{}
	for _, s := range m.scans {
		if isActionableScan(s) && s.Cleaner.EffectiveImpact() != model.ImpactHeavy {
			m.selected[s.Cleaner.ID] = true
		} else {
			m.selected[s.Cleaner.ID] = false
		}
		for _, t := range s.Cleaner.Tags {
			tagSet[t] = true
		}
	}
	tags := make([]string, 0, len(tagSet)+1)
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	m.tagOptions = append([]string{""}, tags...)
	if m.tagFilterIdx >= len(m.tagOptions) {
		m.tagFilterIdx = 0
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
		m.diskStat = readFSStat(m.scope.Home)
		return m, nil
	}
	if m.execCh != nil {
		return m, waitExecMsg(m.execCh)
	}
	return m, nil
}

func (m workbenchModel) isTypingInput() bool {
	return m.filtering || m.histFiltering || m.cfgAdding ||
		m.diskConfirmDelete != "" || m.histConfirmClear ||
		m.phase == phaseConfirm || m.phase == phaseExecuting || m.showHelp
}

func (m workbenchModel) switchTab(target activeTab) (workbenchModel, tea.Cmd) {
	if target == m.activeTab {
		return m, nil
	}
	m.activeTab = target
	switch target {
	case tabAnalyze:
		if !m.diskLoading && len(m.diskEntries) == 0 && m.diskErr == nil {
			return m.startDiskScan(m.diskPath)
		}
	case tabHistory:
		return m, m.loadHistoryCmd()
	}
	return m, nil
}

func (m workbenchModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m.quitAll()
	}

	if !m.isTypingInput() {
		switch key {
		case "1":
			return m.switchTab(tabClean)
		case "2":
			return m.switchTab(tabAnalyze)
		case "3":
			return m.switchTab(tabHistory)
		case "4":
			return m.switchTab(tabConfig)
		case "[":
			prev := (int(m.activeTab) + 3) % 4
			return m.switchTab(activeTab(prev))
		case "]":
			next := (int(m.activeTab) + 1) % 4
			return m.switchTab(activeTab(next))
		}
	}

	if m.activeTab == tabAnalyze {
		return m.handleAnalyzeKey(msg)
	}
	if m.activeTab == tabHistory {
		return m.handleHistoryKey(msg)
	}
	if m.activeTab == tabConfig {
		return m.handleConfigKey(msg)
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
		if m.detailFocused {
			if m.detailScroll > 0 {
				m.detailScroll--
			}
		} else {
			m.moveCursor(-1)
		}
	case "down", "j":
		if m.detailFocused {
			m.detailScroll++
		} else {
			m.moveCursor(1)
		}
	case "J", "ctrl+d":
		m.detailScroll += 3
	case "K", "ctrl+u":
		m.detailScroll -= 3
		if m.detailScroll < 0 {
			m.detailScroll = 0
		}
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
	case "A":
		m.toggleAllIncludingHeavy()
	case "s":
		m.showSkipped = !m.showSkipped
		m.rebuildRows()
	case "o":
		m.sortMode = (m.sortMode + 1) % 4
		m.rebuildRows()
	case "t":
		if len(m.tagOptions) > 1 {
			m.tagFilterIdx = (m.tagFilterIdx + 1) % len(m.tagOptions)
			m.rebuildRows()
		}
	case "m":
		m.agePresetIdx = (m.agePresetIdx + 1) % len(agePresets)
		m.minAgeOverride = agePresets[m.agePresetIdx]
		if m.minAgeOverride == "0d" {
			m.policy.BypassAgeFloor = true
		}
		nextM, cmd := m.startScan()
		return nextM, tea.Batch(nextM.spin.Tick, cmd)
	case "b":
		m.policy.BypassAll = !m.policy.BypassAll
		m.syncPolicyToConfig()
		nextM, cmd := m.startScan()
		return nextM, tea.Batch(nextM.spin.Tick, cmd)
	case "tab":
		if m.width < 96 {
			m.narrowDetailExpanded = !m.narrowDetailExpanded
		} else {
			m.detailFocused = !m.detailFocused
		}
	case "d":
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
	policy := m.policy
	ageOverride := m.minAgeOverride
	idsCopy := append([]string(nil), selectedIDs...)
	cmd := func() tea.Msg {
		if hooks.StartExecWithOptions != nil {
			ch, cancel := hooks.StartExecWithOptions(ctx, policy, ageOverride, idsCopy)
			return execStartedMsg{ch: ch, cancel: cancel}
		}
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
	if row.kind != rowItem {
		return
	}
	if row.section == sectionSkipped {
		if !m.policy.BypassAll {
			m.statusMsg = m.tr.Text("Skipped item: press b to enable Bypass All or switch to Tab 4 to configure rules.", "该项已被跳过：可按 b 启用全局绕过，或按 4 前往安全配置页放宽限制。")
			return
		}
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

func (m *workbenchModel) toggleAllIncludingHeavy() {
	var ids []string
	allSelected := true
	for _, s := range m.scans {
		if !isActionableScan(s) {
			continue
		}
		ids = append(ids, s.Cleaner.ID)
		if !m.selected[s.Cleaner.ID] {
			allSelected = false
		}
	}
	target := !allSelected
	for _, id := range ids {
		m.selected[id] = target
	}
	m.statusMsg = ""
}

func (m workbenchModel) activeTagFilter() string {
	if m.tagFilterIdx <= 0 || m.tagFilterIdx >= len(m.tagOptions) {
		return ""
	}
	return m.tagOptions[m.tagFilterIdx]
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
	activeTag := m.activeTagFilter()
	var safeItems, heavyItems, skippedItems []CleanerScan
	for _, s := range m.scans {
		if filter != "" && !m.matchesFilter(s.Cleaner, filter) {
			continue
		}
		if activeTag != "" && !cleanerHasTag(s.Cleaner, activeTag) {
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
			a, b := list[i], list[j]
			switch m.sortMode {
			case cleanSortName:
				return strings.ToLower(m.tr.CleanerName(a.Cleaner)) < strings.ToLower(m.tr.CleanerName(b.Cleaner))
			case cleanSortFiles:
				if a.FilesCandidate != b.FilesCandidate {
					return a.FilesCandidate > b.FilesCandidate
				}
				return a.Cleaner.ID < b.Cleaner.ID
			case cleanSortImpact:
				if a.Cleaner.EffectiveImpact() != b.Cleaner.EffectiveImpact() {
					return a.Cleaner.EffectiveImpact() == model.ImpactHeavy
				}
				return a.BytesCandidate > b.BytesCandidate
			default:
				if a.BytesCandidate != b.BytesCandidate {
					return a.BytesCandidate > b.BytesCandidate
				}
				if a.FilesCandidate != b.FilesCandidate {
					return a.FilesCandidate > b.FilesCandidate
				}
				return a.Cleaner.ID < b.Cleaner.ID
			}
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
	m.detailScroll = 0
	m.ensureCursorVisible()
}

func cleanerHasTag(c model.Cleaner, want string) bool {
	for _, t := range c.Tags {
		if strings.EqualFold(t, want) {
			return true
		}
	}
	return false
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
	if lastValid != m.cursor {
		m.detailScroll = 0
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
	m.detailScroll = 0
	m.ensureCursorVisible()
}

func (m workbenchModel) listPageSize() int {
	h := m.listViewportHeight()
	if h < 4 {
		return 4
	}
	return h - 1
}

func (m workbenchModel) tabContentHeight() int {
	avail := m.height - 7
	if avail < 8 {
		avail = 8
	}
	return avail
}

func (m workbenchModel) listViewportHeight() int {
	avail := m.height - 11
	if m.width < 96 {
		if m.narrowDetailExpanded {
			avail = m.height/2 - 7
		} else {
			avail = m.height - 19
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
		if m.selected[s.Cleaner.ID] && (isActionableScan(s) || m.policy.BypassAll) {
			n++
		}
	}
	return n
}

func (m workbenchModel) orderedSelectedIDs() []string {
	var safeIDs, heavyIDs []string
	for _, s := range m.scans {
		if !m.selected[s.Cleaner.ID] || (!isActionableScan(s) && !m.policy.BypassAll) {
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
		if !m.selected[s.Cleaner.ID] || (!isActionableScan(s) && !m.policy.BypassAll) {
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
