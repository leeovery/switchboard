package theme

import (
	"embed"
	"fmt"
	"image/color"
	"maps"
	"slices"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// The default pair: what a choice that names no theme draws in.
const (
	DefaultLight = "tokyo-night-day"
	DefaultDark  = "nord"
)

// Terminal is the slug of the theme drawn in the terminal's own colours.
const Terminal = "terminal"

// fileExt ends a theme file's name.
const fileExt = ".theme"

// The built-ins are .theme files read as a user's are, so a built-in is a
// file added here; but the terminal's, whose colours #RRGGBB can't name.
//
//go:embed builtin/*.theme
var builtinFiles embed.FS

// builtins are the built-in themes, by slug.
var builtins = loadBuiltins()

// loadBuiltins reads the built-in themes. One that doesn't load is a broken
// build, which every test of the package reports.
func loadBuiltins() map[string]Theme {
	themes := map[string]Theme{Terminal: terminal()}
	files, err := builtinFiles.ReadDir("builtin")
	if err != nil {
		panic(fmt.Sprintf("read the built-in themes: %v", err))
	}
	for _, f := range files {
		data, err := builtinFiles.ReadFile("builtin/" + f.Name())
		if err != nil {
			panic(fmt.Sprintf("read the built-in theme %s: %v", f.Name(), err))
		}
		slug := strings.TrimSuffix(f.Name(), fileExt)
		t, err := Parse(slug, data)
		if err != nil {
			panic(fmt.Sprintf("the built-in theme %s doesn't load: %v", slug, err))
		}
		themes[slug] = t
	}
	return themes
}

// Builtin returns the built-in theme slug names, and whether there's one.
func Builtin(slug string) (Theme, bool) {
	t, ok := builtins[slug]
	return t, ok
}

// Builtins lists the built-in themes, in order of their slugs.
func Builtins() []Theme {
	var themes []Theme
	for _, slug := range slices.Sorted(maps.Keys(builtins)) {
		themes = append(themes, builtins[slug])
	}
	return themes
}

// terminal is the theme for a terminal whose background is transparent or an
// image: its text in the terminal's own foreground and its colours the
// terminal's own sixteen, on the terminal's own background, so it paints no
// canvas, and, its colours being whatever the terminal makes them, blends
// nothing. A bar's ramp steps from colour to colour rather than through them.
func terminal() Theme {
	t := Theme{Slug: Terminal}
	for tok, c := range map[Token]color.Color{
		TextMuted:        ansi.BrightBlack,
		TextSubtle:       ansi.BrightBlack,
		TextFaint:        ansi.BrightBlack,
		AccentPrimary:    ansi.Magenta,
		AccentKey:        ansi.Blue,
		AccentMode:       ansi.Cyan,
		AccentAttention:  ansi.Yellow,
		StatePositive:    ansi.Green,
		StateDestructive: ansi.Red,
		BgSelection:      ansi.BrightBlack,
		Border:           ansi.BrightBlack,
		TextOnAttention:  ansi.Yellow,
		VizRamp3:         ansi.BrightRed,
	} {
		t.colours[tok] = c
	}
	t.derive()
	return t
}
