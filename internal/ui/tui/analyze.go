package tui

import (
	"os"

	gduapp "github.com/dundee/gdu/v5/cmd/gdu/app"
	"github.com/dundee/gdu/v5/pkg/device"
	"github.com/gdamore/tcell/v2"
	"github.com/mattn/go-isatty"
	"github.com/rivo/tview"
)

// RunAnalyze launches the interactive disk analyzer (gdu) for the given path.
// RunAnalyze 为给定路径启动交互式磁盘分析器 (gdu)。
func RunAnalyze(path string) error {
	istty := isatty.IsTerminal(os.Stdout.Fd())
	flags := &gduapp.Flags{
		NoDelete:     true,
		NoSpawnShell: true,
	}
	var screen tcell.Screen
	var termApp *tview.Application
	var err error
	if !flags.ShouldRunInNonInteractiveMode(istty) {
		screen, err = tcell.NewScreen()
		if err != nil {
			return err
		}
		defer screen.Clear()
		defer screen.Fini()
		termApp = tview.NewApplication()
		termApp.SetScreen(screen)
	}
	app := gduapp.App{
		Flags:       flags,
		Args:        []string{path},
		Istty:       istty,
		Writer:      os.Stdout,
		TermApp:     termApp,
		Screen:      screen,
		Getter:      device.Getter,
		PathChecker: os.Stat,
	}
	return app.Run()
}
