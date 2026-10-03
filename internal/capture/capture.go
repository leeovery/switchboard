// Package capture is the visual capture harness's: the named, deterministic
// fixtures of the dashboard that cmd/capturetool draws, and the fakes it
// draws them through. A fixture is a moment of the dashboard as the frames
// signed off for milestone 5 draw it: what the router gives at that moment,
// on a terminal of a size. It's drawn by the dashboard's own watch model,
// built through watch.New, with every seam faked, so a capture never dials
// the router, probes, touches the network, reads or writes the real config,
// state, prefs or tokens, or runs another process.
//
// Only cmd/capturetool imports it: switchboard itself never does.
package capture

import (
	"fmt"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/theme"
)

// Fixture is a named moment of the dashboard: what the router gives at that
// moment, the terminal it's drawn on, the theme it's drawn in, and the keys
// pressed once its document is read.
type Fixture struct {
	// Name is what the capture tool's --fixture calls it.
	Name string
	// Size is the terminal of the frame it mirrors.
	Size watch.Size
	// now is the moment drawn.
	now    time.Time
	router source
	keys   []tea.KeyPressMsg
	// theme is what it's drawn in: the frames' nord unless InTheme says
	// otherwise.
	theme theme.Theme
	// colourless draws it without colour, as NO_COLOR asks.
	colourless bool
}

// InTheme is the fixture drawn in t, as the one theme chosen.
func (f Fixture) InTheme(t theme.Theme) Fixture {
	f.theme = t
	return f
}

// WithoutColour is the fixture drawn without colour, as NO_COLOR asks.
func (f Fixture) WithoutColour() Fixture {
	f.colourless = true
	return f
}

// Names lists every fixture's name, sorted.
func Names() []string {
	var names []string
	for _, f := range fixtures(moment(time.Local)) {
		names = append(names, f.Name)
	}
	slices.Sort(names)
	return names
}

// ByName returns the fixture with the given name, drawn in the local time
// zone. An empty or unknown name is an error that lists the fixtures.
func ByName(name string) (Fixture, error) {
	return named(name, time.Local)
}

// named returns the fixture with the given name, drawn in the time zone
// given.
func named(name string, loc *time.Location) (Fixture, error) {
	if name == "" {
		return Fixture{}, fmt.Errorf("name a fixture (available: %s)", strings.Join(Names(), ", "))
	}
	all := fixtures(moment(loc))
	i := slices.IndexFunc(all, func(f Fixture) bool { return f.Name == name })
	if i < 0 {
		return Fixture{}, fmt.Errorf("unknown fixture %q (available: %s)", name, strings.Join(Names(), ", "))
	}
	return all[i], nil
}

// moment is when every fixture is drawn, as the frames are: Thursday 1
// October 2026, 14:42:07, in the time zone given.
func moment(loc *time.Location) time.Time {
	return time.Date(2026, time.October, 1, 14, 42, 7, 0, loc)
}

// fixtures are every fixture, at now, in the frames' theme: one for each set
// of accounts the frames draw, at the size of the frame it mirrors, or for
// five accounts, which the final page draws only in Sessions and Runway, at
// Sessions'. A frame of another view, or reached by a key, is a fixture with
// its own size and keys over one of these sets, as the theme picker, open,
// its cursor moved up a theme from the frames' to show the one before; and
// the help, which the frames don't draw.
func fixtures(now time.Time) []Fixture {
	nord, _ := theme.Builtin(theme.DefaultDark)
	w := tea.KeyPressMsg{Code: 'w', Text: "w"}
	all := []Fixture{
		{Name: "accounts-1", Size: wide(27), now: now, router: oneAccount(now)},
		{Name: "accounts-3", Size: wide(34), now: now, router: threeAccounts(now)},
		{Name: "accounts-3-5h", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{w}},
		{Name: "accounts-3-keys", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{{Code: '?', Text: "?"}}},
		{Name: "accounts-3-themes", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{{Code: 't', Text: "t"}, {Code: tea.KeyUp}}},
		{Name: "accounts-3-week", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{w, w}},
		{Name: "accounts-4", Size: wide(40), now: now, router: fourAccounts(now)},
		{Name: "accounts-5", Size: wide(40), now: now, router: fiveAccounts(now)},
		{Name: "accounts-6", Size: wide(40), now: now, router: sixAccounts(now)},
		{Name: "accounts-8", Size: wide(40), now: now, router: eightAccounts(now)},
		{Name: "accounts-8-scrolling", Size: wide(28), now: now, router: eightAccounts(now)},
		{Name: "accounts-phone", Size: watch.Size{Width: 52, Height: 36}, now: now, router: threeAccounts(now)},
	}
	for i := range all {
		all[i].theme = nord
	}
	return all
}

// wide is a terminal as wide as the frames are, 160 columns, and as tall as
// given.
func wide(height int) watch.Size {
	return watch.Size{Width: 160, Height: height}
}
