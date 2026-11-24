package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/config"
	"github.com/dengqi/beav/internal/i18n"
	"github.com/dengqi/beav/internal/oplog"
)

func TestWorkbenchTabsAndBypassControls(t *testing.T) {
	cfg := &config.Config{
		Defaults: config.Defaults{MinAgeDays: 14, Output: "auto"},
	}
	m := newWorkbenchModel(context.Background(), Options{
		Translator: i18n.New(i18n.LangEN),
		Scope:      ScopeInfo{Scope: model.ScopeUser, Home: "/home/u"},
		Version:    "0.2.2",
		Config:     cfg,
		Hooks: Hooks{
			ReadHistory: func() ([]oplog.Entry, error) {
				return []oplog.Entry{{
					Timestamp: time.Now(),
					Operation: "delete",
					Path:      "/home/u/.cache/pip/wheel.whl",
					Size:      4096,
					CleanerID: "lang-pip",
				}}, nil
			},
		},
	})
	m = completeScan(m, sampleScans())

	// Switch to Tab 3 (History) and verify history entry renders.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'3'}})
	m = next.(workbenchModel)
	if m.activeTab != tabHistory {
		t.Fatalf("activeTab = %v, want tabHistory", m.activeTab)
	}
	if cmd != nil {
		next, _ = m.Update(cmd())
		m = next.(workbenchModel)
	}
	if !strings.Contains(m.View(), "lang-pip") {
		t.Fatalf("history tab missing lang-pip entry:\n%s", m.View())
	}

	// Switch to Tab 4 (Safety & Config) and toggle bypass_all.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'4'}})
	m = next.(workbenchModel)
	if m.activeTab != tabConfig {
		t.Fatalf("activeTab = %v, want tabConfig", m.activeTab)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = next.(workbenchModel)
	if !m.policy.BypassAll || !cfg.Safety.BypassAll {
		t.Fatal("Space on first config row should toggle bypass_all in policy and config")
	}

	// Return to Tab 1 (Clean) and verify 'A' selects heavy items too.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	m = next.(workbenchModel)
	if m.activeTab != tabClean {
		t.Fatalf("activeTab = %v, want tabClean", m.activeTab)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'A'}})
	m = next.(workbenchModel)
	if !m.selected["lang-pip"] || !m.selected["lang-rustup"] {
		t.Fatalf("Shift-A should select both safe and heavy actionable items: %+v", m.selected)
	}
}
