package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/dengqi/beav/internal/cleaner/model"
	"github.com/dengqi/beav/internal/i18n"
)

func sampleScans() []CleanerScan {
	return []CleanerScan{
		{
			Cleaner:        model.Cleaner{ID: "lang-pip", Name: "pip wheel cache", Scope: model.ScopeUser, Type: model.TypePaths, Impact: model.ImpactSafe},
			Status:         "ok",
			FilesCandidate: 10,
			BytesCandidate: 1024 * 1024,
			Samples:        []FileSample{{Path: "/home/u/.cache/pip/a.whl", Size: 512 * 1024}},
		},
		{
			Cleaner:        model.Cleaner{ID: "lang-rustup", Name: "Rust toolchains", Scope: model.ScopeUser, Type: model.TypeRustupPrune, Impact: model.ImpactHeavy},
			Status:         "ok",
			FilesCandidate: 4,
			BytesCandidate: 500 * 1024 * 1024,
		},
		{
			Cleaner:    model.Cleaner{ID: "browser-firefox", Name: "Firefox cache", Scope: model.ScopeUser, Type: model.TypePaths, Impact: model.ImpactSafe},
			Status:     "skipped",
			SkipReason: string(model.SkipReasonRunningProcess),
		},
	}
}

func completeScan(m workbenchModel, items []CleanerScan) workbenchModel {
	for i := range items {
		it := items[i]
		next, _ := m.Update(scanMsg(ScanProgress{
			Total:     len(items),
			Completed: i + 1,
			Item:      &it,
		}))
		m = next.(workbenchModel)
	}
	next, _ := m.Update(scanMsg(ScanProgress{Total: len(items), Completed: len(items), Done: true}))
	return next.(workbenchModel)
}

func TestDefaultsSelectSafeOnlyAndAllNeverSelectsHeavy(t *testing.T) {
	m := newWorkbenchModel(context.Background(), Options{
		Translator: i18n.New(i18n.LangEN),
		Scope:      ScopeInfo{Scope: model.ScopeUser, Home: "/home/u"},
	})
	m = completeScan(m, sampleScans())

	if m.phase != phaseReview {
		t.Fatalf("phase = %v, want phaseReview", m.phase)
	}
	if !m.selected["lang-pip"] {
		t.Fatal("safe actionable item must be selected by default")
	}
	if m.selected["lang-rustup"] {
		t.Fatal("heavy item must NOT be selected by default")
	}

	// Pressing 'a' toggles safe items off, then back on; heavy remains unchecked.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = next.(workbenchModel)
	if m.selected["lang-pip"] || m.selected["lang-rustup"] {
		t.Fatalf("expected all safe deselected and heavy still false: %+v", m.selected)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = next.(workbenchModel)
	if !m.selected["lang-pip"] || m.selected["lang-rustup"] {
		t.Fatalf("expected safe reselected and heavy STILL false: %+v", m.selected)
	}
}

func TestFilterModeTypingQDoesNotQuitAndEnterAppliesFilter(t *testing.T) {
	m := newWorkbenchModel(context.Background(), Options{
		Translator: i18n.New(i18n.LangEN),
		Scope:      ScopeInfo{Scope: model.ScopeUser, Home: "/home/u"},
	})
	m = completeScan(m, sampleScans())

	// Open filter with '/'.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = next.(workbenchModel)
	if !m.filtering {
		t.Fatal("expected filtering mode active")
	}

	// Typing 'q' while filtering must filter text, NOT return tea.Quit.
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	m = next.(workbenchModel)
	if cmd != nil && cmd() == tea.Quit() {
		t.Fatal("typing 'q' during filter mode must not quit")
	}
	if m.filterInput.Value() != "q" {
		t.Fatalf("filter value = %q, want %q", m.filterInput.Value(), "q")
	}

	// Pressing Enter exits filter mode without jumping to confirm.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(workbenchModel)
	if m.filtering || m.phase != phaseReview {
		t.Fatalf("Enter in filter mode should stay in phaseReview with filtering=false; got phase=%v filtering=%v", m.phase, m.filtering)
	}
}

func TestConfirmDefaultsToCancelAndPassesExactSelectedIDs(t *testing.T) {
	var executedIDs []string
	m := newWorkbenchModel(context.Background(), Options{
		Translator: i18n.New(i18n.LangEN),
		Scope:      ScopeInfo{Scope: model.ScopeUser, Home: "/home/u"},
		Hooks: Hooks{
			StartExec: func(_ context.Context, ids []string) (<-chan ExecProgress, context.CancelFunc) {
				executedIDs = append([]string(nil), ids...)
				ch := make(chan ExecProgress, 1)
				ch <- ExecProgress{Done: true, TotalFreed: 1024, TotalFiles: 10}
				close(ch)
				return ch, func() {}
			},
		},
	})
	m = completeScan(m, sampleScans())

	// Enter opens confirm modal focused on Cancel.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(workbenchModel)
	if m.phase != phaseConfirm || m.confirmDeleteFocused {
		t.Fatalf("expected confirm modal with Cancel focused; phase=%v deleteFocused=%v", m.phase, m.confirmDeleteFocused)
	}

	// Immediate Enter cancels back to review.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(workbenchModel)
	if m.phase != phaseReview {
		t.Fatalf("Enter on Cancel must return to phaseReview, got %v", m.phase)
	}

	// Open confirm again, switch focus to Delete with Tab, and press Enter.
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(workbenchModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(workbenchModel)
	if !m.confirmDeleteFocused {
		t.Fatal("Tab should focus Delete button")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(workbenchModel)
	if m.phase != phaseExecuting {
		t.Fatalf("expected phaseExecuting, got %v", m.phase)
	}
	if cmd == nil {
		t.Fatal("expected start exec command")
	}
	// Run the start command returned inside the batch.
	started := execStartedMsg{}
	if batch, ok := cmd().(tea.BatchMsg); ok {
		for _, c := range batch {
			if c == nil {
				continue
			}
			if sm, ok := c().(execStartedMsg); ok {
				started = sm
			}
		}
	}
	if len(executedIDs) != 1 || executedIDs[0] != "lang-pip" {
		t.Fatalf("executedIDs = %v, want [lang-pip]", executedIDs)
	}
	if started.ch == nil {
		t.Fatal("expected exec channel from StartExec")
	}
}

func TestResponsiveRenderInNarrowAndWideModesBilingual(t *testing.T) {
	for _, lang := range []i18n.Lang{i18n.LangEN, i18n.LangZH} {
		for _, size := range [][2]int{{80, 24}, {130, 40}} {
			m := newWorkbenchModel(context.Background(), Options{
				Translator: i18n.New(lang),
				Scope:      ScopeInfo{Scope: model.ScopeUser, Home: "/home/u"},
			})
			next, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m = next.(workbenchModel)
			m = completeScan(m, sampleScans())

			view := m.View()
			if !strings.Contains(view, "lang-pip") || !strings.Contains(view, "Rust") {
				t.Fatalf("render missing items for %s %dx%d: %q", lang, size[0], size[1], view)
			}
		}
	}
}
