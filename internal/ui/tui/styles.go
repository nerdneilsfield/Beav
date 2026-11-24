package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPrimary = lipgloss.Color("#38bdf8")
	colorSafe    = lipgloss.Color("#22c55e")
	colorHeavy   = lipgloss.Color("#f59e0b")
	colorDanger  = lipgloss.Color("#ef4444")
	colorMuted   = lipgloss.Color("#71717a")
	colorSubtle  = lipgloss.Color("#a1a1aa")
	colorBorder  = lipgloss.Color("#3f3f46")
	colorFocus   = lipgloss.Color("#0ea5e9")
	colorSelect  = lipgloss.Color("#1e293b")
	colorBarFill = lipgloss.Color("#0284c7")

	styleTitle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary)

	styleBadgeReadOnly = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#0f172a")).
				Background(colorPrimary).
				Padding(0, 1)

	styleBadgeBypass = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#ffffff")).
				Background(colorDanger).
				Padding(0, 1)

	styleBadgeExec = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0f172a")).
			Background(colorHeavy).
			Padding(0, 1)

	styleBadgeDone = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0f172a")).
			Background(colorSafe).
			Padding(0, 1)

	styleBadgeHeavy = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorHeavy)

	styleBadgeSafe = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorSafe)

	styleDangerText = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorDanger)

	styleWarnText = lipgloss.NewStyle().
			Foreground(colorHeavy)

	styleMuted = lipgloss.NewStyle().
			Foreground(colorMuted)

	styleSubtle = lipgloss.NewStyle().
			Foreground(colorSubtle)

	styleSectionHeader = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorPrimary)

	styleActiveRow = lipgloss.NewStyle().
			Background(colorSelect).
			Bold(true)

	styleTabActive = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#0f172a")).
			Background(colorPrimary).
			Padding(0, 1)

	styleTabInactive = lipgloss.NewStyle().
				Foreground(colorSubtle).
				Background(lipgloss.Color("#27272a")).
				Padding(0, 1)

	stylePill = lipgloss.NewStyle().
			Foreground(colorSubtle).
			Background(lipgloss.Color("#1e293b")).
			Padding(0, 1)

	stylePillWarn = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#fef2f2")).
			Background(lipgloss.Color("#991b1b")).
			Padding(0, 1)

	styleBar = lipgloss.NewStyle().
			Foreground(colorBarFill)

	stylePane = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	styleFocusedPane = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorFocus).
				Padding(0, 1)

	styleModal = lipgloss.NewStyle().
			Border(lipgloss.DoubleBorder()).
			BorderForeground(colorHeavy).
			Padding(1, 2)

	styleHelpModal = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Padding(1, 2)

	styleBtnDefault = lipgloss.NewStyle().
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Foreground(colorSubtle)

	styleBtnFocused = lipgloss.NewStyle().
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorPrimary).
			Foreground(lipgloss.Color("#ffffff")).
			Bold(true)

	styleBtnDangerFocused = lipgloss.NewStyle().
				Padding(0, 2).
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorDanger).
				Foreground(lipgloss.Color("#ffffff")).
				Background(lipgloss.Color("#7f1d1d")).
				Bold(true)
)
