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
// router's status document, read as usage reads it. So a capture never
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

	"github.com/leeovery/switchboard/internal/dashboard"
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
	// by is how a printout groups the windows, and windows are the keys of
	// those it prints, in Claude's order: none prints every one.
	by      dashboard.By
	windows []string
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

// fixtureSet is every fixture drawn in a time zone, by name, each built as
// it's asked for: usage's printouts and the pages are drawn over the worlds,
// whose year takes a while, so no set builds them before one is asked for.
type fixtureSet map[string]func() Fixture

// fixturesIn is every fixture drawn in the time zone given, as fixtures,
// printouts and pages say.
func fixturesIn(loc *time.Location) fixtureSet {
	return setOf(fixtures(loc), overWorlds(), worldsNow)
}

// overWorld is a fixture drawn over the accounts' worlds, built as it's asked
// for: its name, and the fixture over the worlds given.
type overWorld interface {
	called() string
	fixture(worlds accountWorlds) Fixture
}

// overWorlds are the fixtures drawn over the accounts' worlds: usage's
// printouts, and the pages the watch draws.
func overWorlds() []overWorld {
	var over []overWorld
	for _, p := range printouts {
		over = append(over, p)
	}
	for _, p := range pages {
		over = append(over, p)
	}
	return over
}

// setOf is the watch's fixtures given and those given over the worlds, by
// name, one over the worlds drawn over what worlds gives once it's asked
// for. It panics on a name given twice, as the second would hide the first.
func setOf(watched []Fixture, over []overWorld, worlds func() accountWorlds) fixtureSet {
	set := make(fixtureSet, len(watched)+len(over))
	add := func(name string, build func() Fixture) {
		if _, ok := set[name]; ok {
			panic(fmt.Sprintf("capture: two fixtures named %q", name))
		}
		set[name] = build
	}
	for _, f := range watched {
		add(f.Name, func() Fixture { return f })
	}
	for _, o := range over {
		add(o.called(), func() Fixture { return o.fixture(worlds()) })
	}
	return set
}

// names lists the set's names, sorted.
func (s fixtureSet) names() []string {
	return slices.Sorted(maps.Keys(s))
}

// named returns the set's fixture with the given name, its keys and windows
// its own, so a caller changing them changes no other's. An empty or unknown
// name is an error that lists the fixtures.
func (s fixtureSet) named(name string) (Fixture, error) {
	if name == "" {
		return Fixture{}, fmt.Errorf("name a fixture (available: %s)", strings.Join(s.names(), ", "))
	}
	build, ok := s[name]
	if !ok {
		return Fixture{}, fmt.Errorf("unknown fixture %q (available: %s)", name, strings.Join(s.names(), ", "))
	}
	f := build()
	f.keys, f.windows = slices.Clone(f.keys), slices.Clone(f.windows)
	return f, nil
}

// moment is when every fixture over milestone 5's samples is drawn, as the
// frames are: Thursday 1 October 2026, 14:42:07, in the time zone given,
// fixed as its clocks are then. Such a fixture is built and drawn in that, a
// zone whose clocks never change: where the given zone's change within the
// fixtures' week, as Sydney's do on the Sunday, their resets would move by an
// hour.
func moment(loc *time.Location) time.Time {
	name, offset := time.Date(2026, time.October, 1, 14, 42, 7, 0, loc).Zone()
	return time.Date(2026, time.October, 1, 14, 42, 7, 0, time.FixedZone(name, offset))
}

// localMoment is when every fixture over a world is drawn, its records or its
// document alone: the frames' moment in the machine's own zone, time.Local
// itself, whose clocks change as they do there. The ledger, its readers and
// the views split days in it, so a world whose days they read is built in it
// too, placed wall clock by wall clock, its days falling where they put them
// through a year that crosses summer time where the machine's zone does.
func localMoment() time.Time {
	return time.Date(2026, time.October, 1, 14, 42, 7, 0, time.Local)
}

// worldsNow are the accounts' worlds at localMoment, built once, as every
// zone's fixtures are built over them, and their year takes a while.
var worldsNow = sync.OnceValue(func() accountWorlds { return newAccountWorlds(localMoment()) })

// fixtures are the watch's fixtures, in the frames' theme, over milestone 5's
// samples at the frames' moment in the zone given, as moment fixes it: one
// for each set of accounts its frames draw, at the size of the frame it
// mirrors, or for five accounts, which its final page draws only in Sessions
// and Runway, at Sessions'. Each opens on the Overview, which shows milestone
// 5's cards until stage 7 builds it. A frame of another view, or reached by
// a key, is a fixture with its own size and keys over one of these sets: as
// every card flipped with s; the theme picker, open, its cursor moved up a
// theme from the frames' to show the one before; the help, which the frames
// don't draw, and the help over Sessions on a short terminal, scrolled down
// three rows; Sessions, three tabs on; and Runway, four tabs on, over the
// day, and with w, over the week.
func fixtures(loc *time.Location) []Fixture {
	now := moment(loc)
	w, g, j, tab := tea.KeyPressMsg{Code: 'w', Text: "w"}, tea.KeyPressMsg{Code: 'g', Text: "g"}, tea.KeyPressMsg{Code: 'j', Text: "j"}, tea.KeyPressMsg{Code: tea.KeyTab}
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
		{Name: "accounts-phone", Size: watch.Size{Width: 52, Height: 36}, now: now, router: threeAccounts(now)},
		{Name: "page-help-scrolled", Size: wide(16), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{tab, tab, tab, {Code: '?', Text: "?"}, j, j, j}},
		{Name: "runway-day-3", Size: wide(26), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{tab, tab, tab, tab}},
		{Name: "runway-day-5", Size: wide(32), now: now, router: fiveAccounts(now), keys: []tea.KeyPressMsg{tab, tab, tab, tab}},
		{Name: "runway-week-3", Size: wide(26), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{tab, tab, tab, tab, w}},
		{Name: "sessions-1", Size: wide(27), now: now, router: oneAccount(now), keys: []tea.KeyPressMsg{tab, tab, tab}},
		{Name: "sessions-3", Size: wide(28), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{tab, tab, tab}},
		{Name: "sessions-3-keys", Size: wide(28), now: now, router: threeAccounts(now), keys: []tea.KeyPressMsg{tab, tab, tab, {Code: '?', Text: "?"}}},
		{Name: "sessions-5", Size: wide(40), now: now, router: fiveAccounts(now), keys: []tea.KeyPressMsg{tab, tab, tab}},
		{Name: "sessions-storyboard-1-request-out", Size: wide(28), now: storyboard, router: requestOut(storyboard), keys: []tea.KeyPressMsg{tab, tab, tab}},
		{Name: "sessions-storyboard-2-streaming-back", Size: wide(28), now: storyboard, router: streamingBack(storyboard), keys: []tea.KeyPressMsg{tab, tab, tab}},
		{Name: "sessions-storyboard-3-refused", Size: wide(28), now: storyboard, router: refused(storyboard), keys: []tea.KeyPressMsg{tab, tab, tab}},
		{Name: "sessions-storyboard-4-repatched", Size: wide(28), now: storyboard, router: repatched(storyboard), keys: []tea.KeyPressMsg{tab, tab, tab}},
	}
	for i := range all {
		all[i].theme = framesTheme()
	}
	return all
}

// framesTheme is the theme the frames are drawn in: nord.
func framesTheme() theme.Theme {
	nord, _ := theme.Builtin(theme.DefaultDark)
	return nord
}

// printout is one of usage's printouts, named after the frame it draws, at
// that frame's size: of the world that world picks of the accounts', laid
// out as by and windows say.
type printout struct {
	name    string
	size    watch.Size
	world   func(accountWorlds) accountWorld
	by      dashboard.By
	windows []string
}

// printouts are usage's printouts: the auto world's every window, by window,
// at each width a layout changes at; its 5-hour window alone, as --5h prints
// it; by account, as --by account prints it; and work alone, with six
// sessions on it, as its frame counts them.
var printouts = []printout{
	{name: "usage-160", size: watch.Size{Width: 160, Height: 13}, world: accountWorlds.auto},
	{name: "usage-120", size: watch.Size{Width: 120, Height: 36}, world: accountWorlds.auto},
	{name: "usage-80", size: watch.Size{Width: 80, Height: 38}, world: accountWorlds.auto},
	{name: "usage-52", size: watch.Size{Width: 52, Height: 38}, world: accountWorlds.auto},
	{name: "usage-5h-160", size: watch.Size{Width: 160, Height: 13}, world: accountWorlds.auto, windows: []string{sessionKey}},
	{name: "usage-5h-52", size: watch.Size{Width: 52, Height: 16}, world: accountWorlds.auto, windows: []string{sessionKey}},
	{name: "usage-by-account-160", size: watch.Size{Width: 160, Height: 31}, world: accountWorlds.auto, by: dashboard.ByAccount},
	{name: "usage-by-account-52", size: watch.Size{Width: 52, Height: 38}, world: accountWorlds.auto, by: dashboard.ByAccount},
	{name: "usage-one-account-160", size: watch.Size{Width: 160, Height: 7}, world: accountWorlds.alone},
}

// called is the printout's name.
func (p printout) called() string {
	return p.name
}

// fixture is the printout of its world of worlds, read at their moment, in
// the frames' theme.
func (p printout) fixture(worlds accountWorlds) Fixture {
	return Fixture{
		Name: p.name, Size: p.size, Printout: true, now: worlds.now, router: routerOf(p.world(worlds), worlds.now),
		by: p.by, windows: p.windows, theme: framesTheme(),
	}
}

// page is the page the watch draws over one of the accounts' worlds, at their
// moment, in the frames' theme, with its world's records: on a terminal of
// its size, with its keys pressed once its document is read; and, gone, with
// the router no longer answering from that moment, as a key that reads then
// finds it.
type page struct {
	name  string
	size  watch.Size
	world func(accountWorlds) accountWorld
	keys  []tea.KeyPressMsg
	gone  bool
}

// called is the page's name.
func (p page) called() string {
	return p.name
}

// fixture is the page drawn over its world of worlds.
func (p page) fixture(worlds accountWorlds) Fixture {
	w := p.world(worlds)
	router := routerOf(w, worlds.now)
	if p.gone {
		router = router.goneAt(worlds.now)
	}
	return Fixture{
		Name: p.name, Size: p.size, now: worlds.now, router: router, keys: slices.Clone(p.keys),
		theme: framesTheme(), records: worlds.recordsOf(w),
	}
}

// pages are the page as stage 6 builds it, over the auto world, the frames'
// tabs and cards not yet drawn as their frames are, to judge by eye: the
// routing card over the Overview, two accounts picked and m ticked, at
// 160×40, and on a phone, one picked and m ticked; the Log's empty tab, its
// title row lit there; and the router gone, R having found it so, the status
// line saying since when and p pin faint.
var pages = []page{
	{name: "page-routing", size: watch.Size{Width: 160, Height: 40}, world: accountWorlds.auto, keys: keysOf("p", "2", "3", "m")},
	{name: "page-routing-phone", size: watch.Size{Width: 52, Height: 36}, world: accountWorlds.auto, keys: keysOf("p", "2", "m")},
	{name: "page-log", size: wide(37), world: accountWorlds.auto, keys: []tea.KeyPressMsg{{Code: tea.KeyTab, Mod: tea.ModShift}}},
	{name: "page-not-answering", size: wide(37), world: accountWorlds.auto, keys: keysOf("R"), gone: true},
}

// keysOf are the keys pressed as the text given, a key a character.
func keysOf(text ...string) []tea.KeyPressMsg {
	var keys []tea.KeyPressMsg
	for _, t := range text {
		keys = append(keys, tea.KeyPressMsg{Code: []rune(t)[0], Text: t})
	}
	return keys
}

// wide is a terminal as wide as the frames are, 160 columns, and as tall as
// given.
func wide(height int) watch.Size {
	return watch.Size{Width: 160, Height: height}
}
