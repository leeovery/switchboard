package cli

import (
	"errors"
	"image/color"
	"io"

	"github.com/charmbracelet/colorprofile"

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

// Chosen is the user's choice as the preferences file keeps it now.
func (t themes) Chosen() theme.Choice {
	return t.kept().Choice
}

// Keep changes the user's choice in the preferences file, as it stands now,
// as change says, and returns the choice it keeps.
func (t themes) Keep(change func(theme.Choice) theme.Choice) (theme.Choice, error) {
	if t.prefs == nil {
		return theme.Choice{}, errNowhereToKeep
	}
	var kept theme.Choice
	err := t.prefs.Update(func(p *theme.Prefs) {
		p.Choice = change(p.Choice)
		kept = p.Choice
	})
	return kept, err
}

// noColour reports whether NO_COLOR is set, to anything at all, as
// no-color.org has it.
func (a *app) noColour() bool {
	return a.Getenv("NO_COLOR") != ""
}

// printLook is how usage prints the dashboard into the scrollback, on out,
// which shows the colours its profile says: under NO_COLOR, without colour;
// else, of the themes, in the one chosen for the terminal's background, as
// the terminal says it is when asked, where out shows colour, its blends
// worked out against it, and no canvas painted. As it paints none, one theme
// chosen is printed only on a background as dark, or as light, as its own:
// on another, the default pair's half for that background is.
func (a *app) printLook(out io.Writer, shows colorprofile.Profile, t themes, chosen theme.Choice) dashboard.Look {
	if a.noColour() {
		return dashboard.NoColour()
	}
	var background color.Color
	if shows > colorprofile.ASCII {
		background = a.Background(out)
	}
	dark := theme.Dark(background)
	in := t.library.Pair(chosen).For(dark)
	if chosen.IsOne() && !in.Suits(dark) {
		logger.Debug("the theme chosen doesn't suit the terminal's background, so the default stands in", "theme", in.Slug, "dark", dark)
		in = t.library.Pair(theme.Choice{}).For(dark)
	}
	return dashboard.Print(in, background)
}
