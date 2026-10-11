// Package capture is the visual capture harness's: the named, deterministic
// fixtures of the dashboard that cmd/capturetool draws, the scenarios it
// plays for the README's demos, and the fakes it draws them through. A
// fixture is a moment of the dashboard as a signed-off frame draws it, on a
// terminal of the frame's size: milestone 7's, over the specs' worlds, each
// its accounts as the router stands with them and its records, a year of
// days, today's lines and the router's events, served through fake readers;
// or milestone 5's, kept until the views they draw go. A scenario is a
// stretch of the router's life, played in real time. Both are drawn by the
// dashboard's own watch model, built through watch.New, with every seam
// faked; a printout of usage, through a path of its own, from the fake
// router read as usage reads one, through watch.Once. So a capture never
// dials the router, probes, touches the network, reads or writes the real
// config, state, prefs or tokens, or runs another process.
//
// Only cmd/capturetool imports it: switchboard itself never does.
package capture

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"sync"
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
	// Printout is set for a printout of usage, drawn once, as it prints into
	// the scrollback, rather than by the watch, full screen.
	Printout bool
	// now is the moment drawn.
	now    time.Time
	router source
	keys   []tea.KeyPressMsg
	// theme is what it's drawn in: the frames' nord unless InTheme says
	// otherwise.
	theme theme.Theme
	// colourless draws it without colour, as NO_COLOR asks.
	colourless bool
	// records are what its world's readers serve the watch: nil serves
	// none.
	records *records
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
	return local().names()
}

// ByName returns the fixture with the given name, drawn in the local time
// zone, as fixtures says. An empty or unknown name is an error that lists the
// fixtures.
func ByName(name string) (Fixture, error) {
	return local().named(name)
}

// local is every fixture drawn in the local time zone, built once.
var local = sync.OnceValue(func() fixtureSet { return fixturesIn(time.Local) })

// fixtureSet is every fixture drawn in a time zone, each built once, by name.
type fixtureSet map[string]Fixture

// fixturesIn is every fixture drawn in the time zone given, as fixtures says.
func fixturesIn(loc *time.Location) fixtureSet {
	return setOf(fixtures(loc))
}

// setOf is the fixtures given, by name. It panics on a name given twice, as
// the second would hide the first.
func setOf(all []Fixture) fixtureSet {
	set := make(fixtureSet, len(all))
	for _, f := range all {
		if _, ok := set[f.Name]; ok {
			panic(fmt.Sprintf("capture: two fixtures named %q", f.Name))
		}
		set[f.Name] = f
	}
	return set
}

// names lists the set's names, sorted.
func (s fixtureSet) names() []string {
	return slices.Sorted(maps.Keys(s))
}

// named returns the set's fixture with the given name, its keys its own, so
// a caller changing them changes no other's. An empty or unknown name is an
// error that lists the fixtures.
func (s fixtureSet) named(name string) (Fixture, error) {
	if name == "" {
		return Fixture{}, fmt.Errorf("name a fixture (available: %s)", strings.Join(s.names(), ", "))
	}
	f, ok := s[name]
	if !ok {
		return Fixture{}, fmt.Errorf("unknown fixture %q (available: %s)", name, strings.Join(s.names(), ", "))
	}
	f.keys = slices.Clone(f.keys)
	return f, nil
}

// moment is when every fixture over no records is drawn, as the frames are:
// Thursday 1 October 2026, 14:42:07, in the time zone given, fixed as its
// clocks are then. Such a fixture is built and drawn in that, a zone whose
// clocks never change: where the given zone's change within the fixtures'
// week, as Sydney's do on the Sunday, their resets would move by an hour.
func moment(loc *time.Location) time.Time {
	name, offset := time.Date(2026, time.October, 1, 14, 42, 7, 0, loc).Zone()
	return time.Date(2026, time.October, 1, 14, 42, 7, 0, time.FixedZone(name, offset))
}

// localMoment is when every fixture over records is drawn: the frames'
// moment in the machine's own zone, time.Local itself, whose clocks change as
// they do there. The ledger, its readers and the views split days in it, so a
// world whose days they read is built in it too, placed wall clock by wall
// clock, its days falling where they put them through a year that crosses
// summer time where the machine's zone does.
func localMoment() time.Time {
	return time.Date(2026, time.October, 1, 14, 42, 7, 0, time.Local)
}

// fixtures are every fixture, in the frames' theme: those over no records at
// the frames' moment in the zone given, as moment fixes it, and those over
// records at it in the machine's, as localMoment fixes it, whatever the zone
// given. There's one for each set of accounts the frames draw, at the size of
// the frame it mirrors, or for five accounts, which the final page draws only
// in Sessions and Runway, at Sessions'. A frame of another view, or reached
// by a key, is a fixture with its own size and keys over one of these sets:
// as every card flipped with s; work's card flipped with space, and d28c, its
// second session as the router lists them, picked out with ↓ twice; the
// theme picker, open, its cursor moved up a theme from the frames' to show
// the one before; the help, which the frames don't draw; and Runway, with
// tab, over the day, and with w, over the week.
func fixtures(loc *time.Location) []Fixture {
	now := moment(loc)
	nord, _ := theme.Builtin(theme.DefaultDark)
	w, g, tab := tea.KeyPressMsg{Code: 'w', Text: "w"}, tea.KeyPressMsg{Code: 'g', Text: "g"}, tea.KeyPressMsg{Code: tea.KeyTab}
	space, down := tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}, tea.KeyPressMsg{Code: tea.KeyDown}
	storyboard := storyboardAt(now)
	all := []Fixture{
		{Name: "accounts-1", Size: wide(27), now: now, router: oneAccount(now)},
		{Name: "accounts-3", Size: wide(34), now: now, router: threeAccounts(now)},
		{Name: "accounts-3-5h", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{w}},
		{Name: "accounts-3-hourglass", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{g, g}},
		{Name: "accounts-3-keys", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{{Code: '?', Text: "?"}}},
		{Name: "accounts-3-rate", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{g}},
		{Name: "accounts-3-themes", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{{Code: 't', Text: "t"}, {Code: tea.KeyUp}}},
		{Name: "accounts-3-week", Size: wide(34), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{w, w}},
		{Name: "accounts-4", Size: wide(40), now: now, router: fourAccounts(now)},
		{Name: "accounts-4-hourglass", Size: wide(40), now: now, router: fourAccounts(now), keys: []tea.KeyPressMsg{g, g}},
		{Name: "accounts-4-rate", Size: wide(40), now: now, router: fourAccounts(now), keys: []tea.KeyPressMsg{g}},
		{Name: "accounts-5", Size: wide(40), now: now, router: fiveAccounts(now)},
		{Name: "accounts-6", Size: wide(40), now: now, router: sixAccounts(now)},
		{Name: "accounts-8", Size: wide(40), now: now, router: eightAccounts(now)},
		{Name: "accounts-8-hourglass", Size: wide(40), now: now, router: eightAccounts(now), keys: []tea.KeyPressMsg{g, g}},
		{Name: "accounts-8-rate", Size: wide(40), now: now, router: eightAccounts(now), keys: []tea.KeyPressMsg{g}},
		{Name: "accounts-8-scrolling", Size: wide(28), now: now, router: eightAccounts(now)},
		{Name: "accounts-flipped-all", Size: wide(34), now: now, router: threeAccounts(now).telling(backs(now)...), keys: []tea.KeyPressMsg{{Code: 's', Text: "s"}}},
		{Name: "accounts-flipped-selected", Size: wide(34), now: now, router: threeAccounts(now).telling(backs(now)...), keys: []tea.KeyPressMsg{space, down, down}},
		{Name: "accounts-phone", Size: watch.Size{Width: 52, Height: 36}, now: now, router: threeAccounts(now)},
		{Name: "runway-day-3", Size: wide(26), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{tab, tab}},
		{Name: "runway-day-5", Size: wide(32), now: now, router: fiveAccounts(now), keys: []tea.KeyPressMsg{tab, tab}},
		{Name: "runway-week-3", Size: wide(26), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{tab, tab, w}},
		{Name: "sessions-1", Size: wide(27), now: now, router: oneAccount(now), keys: []tea.KeyPressMsg{tab}},
		{Name: "sessions-3", Size: wide(28), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{tab}},
		{Name: "sessions-3-keys", Size: wide(28), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{tab, {Code: '?', Text: "?"}}},
		{Name: "sessions-5", Size: wide(40), now: now, router: fiveAccounts(now), keys: []tea.KeyPressMsg{tab}},
		{Name: "sessions-storyboard-1-request-out", Size: wide(28), now: storyboard, router: requestOut(storyboard), keys: []tea.KeyPressMsg{tab}},
		{Name: "sessions-storyboard-2-streaming-back", Size: wide(28), now: storyboard, router: streamingBack(storyboard), keys: []tea.KeyPressMsg{tab}},
		{Name: "sessions-storyboard-3-refused", Size: wide(28), now: storyboard, router: refused(storyboard), keys: []tea.KeyPressMsg{tab}},
		{Name: "sessions-storyboard-4-repatched", Size: wide(28), now: storyboard, router: repatched(storyboard), keys: []tea.KeyPressMsg{tab}},
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
