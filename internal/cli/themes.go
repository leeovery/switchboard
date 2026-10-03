package cli

import (
	"errors"
	"image/color"
	"io"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/term"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/theme"
)

// errNowhereToKeep is why a theme chosen can't be kept without a state
// directory, which the preferences file is in.
var errNowhereToKeep = errors.New("there's no state directory to keep it in")

// themes are the dashboard's themes, as the theme picker takes them: the
// built-ins and the themes directory's, and the preferences file the user's
// choice is kept in, beside the view shown.
type themes struct {
	library *theme.Library
	// prefs is the preferences file: nil where the state directory can't be
	// found, leaving nowhere to keep a choice.
	prefs *theme.PrefsFile
}

// themes finds the dashboard's themes: in the themes directory, and the
// choice in the preferences file, in the state directory. Without a home to
// find them in, there are the built-ins alone, and the default pair.
func (a *app) themes() themes {
	dir, err := config.ThemesDir(a.Getenv, a.HomeDir)
	if err != nil {
		logger.Debug("no themes directory: the built-in themes alone", "error", err)
	}
	t := themes{library: theme.NewLibrary(dir)}
	state, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		logger.Debug("no state directory: the default themes, and nowhere to keep a choice", "error", err)
		return t
	}
	prefs := theme.NewPrefsFile(state, a.Now)
	t.prefs = &prefs
	return t
}

// kept are the preferences the dashboard kept: the defaults where there's no
// preferences file to say.
func (t themes) kept() theme.Prefs {
	if t.prefs == nil {
		return theme.Prefs{}
	}
	return t.prefs.Read()
}

// List lists every theme there is to pick from.
func (t themes) List() theme.Listing {
	return t.library.List()
}

// Keep keeps the user's choice in the preferences file.
func (t themes) Keep(c theme.Choice) error {
	if t.prefs == nil {
		return errNowhereToKeep
	}
	return t.prefs.Update(func(p *theme.Prefs) { p.Choice = c })
}

// noColour reports whether NO_COLOR is set, to anything at all, as
// no-color.org has it.
func (a *app) noColour() bool {
	return a.Getenv("NO_COLOR") != ""
}

// printLook is how usage prints the dashboard into the scrollback: under
// NO_COLOR, without colour; else, of the themes, in the one chosen for the
// terminal's background, as the terminal says it is when asked, its blends
// worked out against it, and no canvas painted.
func (a *app) printLook(out io.Writer, t themes, chosen theme.Choice) dashboard.Look {
	if a.noColour() {
		return dashboard.NoColour()
	}
	background := a.Background(out)
	return dashboard.Print(t.library.Pair(chosen).For(theme.Dark(background)), background)
}

// TerminalBackground asks the terminal what its background is (OSC 11), as
// Deps.Background does, for the process's own output: nil where out, or
// stdin, isn't a terminal, or the terminal doesn't say.
func TerminalBackground(out io.Writer) color.Color {
	f, ok := out.(term.File)
	if !ok || !term.IsTerminal(f.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		return nil
	}
	background, err := lipgloss.BackgroundColor(os.Stdin, f)
	if err != nil {
		logger.Debug("the terminal didn't say what its background is", "error", err)
		return nil
	}
	return background
}
