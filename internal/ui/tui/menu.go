package tui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type item struct {
	title  string
	desc   string
	action string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

type menuModel struct {
	list   list.Model
	choice string
}

func (m menuModel) Init() tea.Cmd { return nil }

func (m menuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "enter":
			if selected, ok := m.list.SelectedItem().(item); ok {
				m.choice = selected.action
			}
			return m, tea.Quit
		case "ctrl+c", "q":
			return m, tea.Quit
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m menuModel) View() string {
	return lipgloss.NewStyle().Margin(1, 2).Render(m.list.View())
}

// RunMenu displays the interactive menu and returns the selected action.
// RunMenu 显示交互菜单并返回所选操作。
func RunMenu() (string, error) {
	items := []list.Item{
		item{title: "Clean", desc: "Run age-aware cache cleanup", action: "clean"},
		item{title: "Analyze", desc: "Interactive disk explorer", action: "analyze"},
		item{title: "List cleaners", desc: "Show all known cleaners", action: "list"},
		item{title: "Quit", desc: "", action: ""},
	}
	l := list.New(items, list.NewDefaultDelegate(), 60, 14)
	l.Title = "Beav"
	model, err := tea.NewProgram(menuModel{list: l}).Run()
	if err != nil {
		return "", err
	}
	return model.(menuModel).choice, nil
}
