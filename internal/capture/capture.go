// Package capture is the visual capture harness's: the named, deterministic
// fixtures of the dashboard that cmd/capturetool draws, the scenarios it
// plays for the README's demos, and the fakes it draws them through. A
// fixture is a moment of the dashboard as a signed-off frame draws it, on a
// terminal of the frame's size, over the specs' worlds, each its accounts as
// the router stands with them and its records, a year of days, today's lines
// and the router's events, served through fake readers. A scenario is a
// stretch of the router's life, played in real time from one of those
// worlds. Both are drawn by the
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

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/status"
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
	// before are keys pressed a while before the moment, as a frame of a
	// table paused while lines came has them: none where it has none.
	before earlier
	// view is the tab it opens on, as the preferences keep one: the
	// Overview where it's unset; and log the Log's columns they keep shown.
	view dashboard.View
	log  dashboard.LogTicks
	// by is how a printout groups the windows, and windows are the keys of
	// those it prints, in the order of care: none prints every one.
	by      dashboard.By
	windows []string
	// theme is what it's drawn in: the frames' nord unless InTheme says
	// otherwise.
	theme theme.Theme
	// colourless draws it without colour, as NO_COLOR asks.
	colourless bool
	// scale is the requests a minute that fill a trace's height, where its
	// frame pins them otherwise than the frames' usual, as a session's story's
	// frames do: zero for the usual.
	scale float64
	// loading has its last key's reads still under way, what they're for
	// saying it's loading, as underWay has them.
	loading bool
	// records are what its world's readers serve the watch, and accounts
	// its world's, as configured, whose plans History prices against: a
	// printout has neither, and a watch drawn of it reads none.
	records  *records
	accounts config.Accounts
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
// zone, as localMoment says. An empty or unknown name is an error that lists
// the fixtures.
func ByName(name string) (Fixture, error) {
	return local().named(name)
}

// local is every fixture, drawn in the local time zone over the worlds at
// localMoment, built once.
var local = sync.OnceValue(func() fixtureSet { return setOf(overWorlds(), worldsNow) })

// fixtureSet is every fixture drawn in a time zone, by name, each built the
// first time it's asked for: usage's printouts and the pages are drawn over
// the worlds, whose year takes a while, so no set builds them before one is
// asked for.
type fixtureSet map[string]func() Fixture

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

// setOf is the fixtures given over the worlds, by name, each drawn over what
// worlds gives the first time it's asked for, and only then. It panics on a
// name given twice, as the second would hide the first.
func setOf(over []overWorld, worlds func() accountWorlds) fixtureSet {
	set := make(fixtureSet, len(over))
	for _, o := range over {
		if _, ok := set[o.called()]; ok {
			panic(fmt.Sprintf("capture: two fixtures named %q", o.called()))
		}
		set[o.called()] = sync.OnceValue(func() Fixture { return o.fixture(worlds()) })
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
	f.keys, f.windows, f.before.keys = slices.Clone(f.keys), slices.Clone(f.windows), slices.Clone(f.before.keys)
	return f, nil
}

// earlier are keys pressed a while before a fixture's moment: how long
// before, and the keys.
type earlier struct {
	by   time.Duration
	keys []tea.KeyPressMsg
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
// its size, with its keys pressed once its document is read, and those
// before it pressed then; gone, with the router no longer answering from
// that moment, as a key that reads then finds it, or from goneFor before it,
// as the fixture's settling finds it; the tab its preferences keep, which
// it opens on, the Overview where it's unset, and the Log's columns they
// keep shown; and the requests a minute that fill a trace's height, where
// its frame pins them otherwise than the frames' usual.
type page struct {
	name    string
	size    watch.Size
	world   func(accountWorlds) accountWorld
	keys    []tea.KeyPressMsg
	before  earlier
	gone    bool
	goneFor time.Duration
	view    dashboard.View
	log     dashboard.LogTicks
	scale   float64
	loading bool
	looked  *looked
}

// looked is a page's moment as a later look at the router has it: after
// the worlds' moment by after, the router having listed, as its keys before
// were pressed, the sessions listed gives then, its stream telling nothing,
// and its document as at the moment, but that no limit had yet moved a
// session; and standing, from the moment on, as the page's world has it.
type looked struct {
	after  time.Duration
	listed func(now time.Time) []status.Session
}

// called is the page's name.
func (p page) called() string {
	return p.name
}

// fixture is the page drawn over its world of worlds, at its moment, as a
// later look has it, where it's one.
func (p page) fixture(worlds accountWorlds) Fixture {
	w, now := p.world(worlds), worlds.now
	if p.looked != nil {
		now = now.Add(p.looked.after)
	}
	router := routerOf(w, now)
	if p.looked != nil {
		was := router
		was.sessions, was.stream = p.looked.listed(now.Add(-p.before.by)), nil
		was.doc.Events = slices.DeleteFunc(slices.Clone(was.doc.Events), func(e status.Event) bool { return e.Kind == status.EventLimit })
		router = was.then(now, router)
	}
	if p.gone {
		router = router.goneAt(now.Add(-p.goneFor))
	}
	return Fixture{
		Name: p.name, Size: p.size, now: now, router: router, keys: slices.Clone(p.keys), before: p.before,
		theme: framesTheme(), records: w.records(), accounts: w.accounts, view: p.view, log: p.log, scale: p.scale, loading: p.loading,
	}
}

// pages are the pages the watch draws over the worlds. The Overview's frames
// and the routing card's over it, each named after its frame, at its size,
// over its world, with the keys that reach it: on auto, pinned and in the bad
// hour; by account; each model's own window shown, with w; side's Fable week
// nearing its limit, shown with them hidden; USAGE alone; SESSIONS shown
// too, the rest folded; six accounts and ten; at 216 columns, with SESSIONS,
// and everything shown; on a phone; and the routing card opened
// on auto, with two picked, with one picked and m ticked, and over routing's
// world of a limit, with side picked. History's frames, each reached from
// the Overview with tab twice, then the keys to its view: the Year, and
// filtered to Opus 5.5; Weeks, its table and its columns, and of ten
// accounts; Days, and of ten; Tokens by account, and by model; and the Year
// on a phone; and where no frame draws it, to judge by eye, the Year at
// 160×24, scrolled down three rows with j. Then the page as stage 6 builds
// it, over the auto world, where no frame draws it, to judge by eye: the
// routing card over the Overview, two accounts picked and m ticked, at
// 160×40, and on a phone, one picked and m ticked; and as it opens at
// 160×16, shorter than it, its keys kept; the router gone, R having found it
// so, the status line saying since when and p pin faint; and the Overview's
// help on a short terminal, scrolled down three rows. Then the Log's frames,
// each reached from the Overview with shift-tab: Events, with ]; Requests
// as it opens; the router gone, over the world of the Log not live;
// Requests on a phone; an event's
// details, the cap's, ↑ five times on Events and enter; a request's, d28c's
// move, ↑ to 14:31:09 and ↓ to it as the Log stood at 14:32:41, the table
// paused there, then enter at the moment, what came since read; the f
// panel, over Events, with ] and f, and over Requests, with f, then → and ↓
// twelve times to d28c's row; and Requests filtered to d28c's lines alone,
// found with / d28c and enter, worth shown as the preferences keep it.
// Where no frame draws them, to judge by eye: the Log's find open on
// Requests, / sw typed, and HELP · LOG over them. Then a session's page's
// frames, each reached through the Log, its story's scale pinned at 8: d28c's
// page, and on a phone; 9c2e's; b9c4's, ended yesterday; f41a's, over its
// world, come back this morning; d28c's with ↓ to turn 12 and enter; and
// the routing card over 9c2e's page, with p and 3. Where no frame draws it,
// to judge by eye: d28c's page loading, its read under way. Then Accounts'
// frames, each reached from the Overview with tab: Compare as it opens, over
// the auto world, ten accounts' and the bad hour's, and with >, the last 30
// days; Detail with ], all accounts chosen, ↓ to work, and over the bad
// hour's world, ↓ twice to personal; where no frame draws them, to judge by
// eye, side chosen as Detail opens on Accounts, its list holding the focus,
// and → and ↓ into its SESSIONS, d28c selected; side's SESSIONS, d28c
// selected, opening on Accounts, so the stream first opens there, telling
// each request in flight as it went; Detail alone, over work alone's
// world; and on a phone, Compare, and Detail of work. Then Sessions' frames, each reached from the
// Overview with tab three times: the Switchboard and, with ], the List, over
// the scene's world, wide and on a phone; and, each as a later look has it,
// the Switchboard kept from a look before its moment that saw its sessions
// in the frame's order, then left for Runway and come back to with
// shift-tab, so its stream opens afresh: of ten accounts, of work alone, and
// pinned to personal and side, looked at a minute before; and at personal's
// limit, refused at 16:05:41, and moved at 16:05:42, looked at at 14:42:07.
// Where no frame draws it, to judge by eye: the Switchboard at 120 × 30;
// with ↓ four times, d28c chosen, its call lit to its plug; at 160 × 14, its
// lines scrolling, ↓ six times scrolling to db8a; and the router gone, R
// having found it so, what was in flight said as last known.
// Then Runway's frames, each reached from the Overview with tab four times:
// the week, which it opens on, wide and on a phone; the 5-hour view, with >;
// Wednesday, > then ← twice, today and the day before; all over the auto
// world; and the week of ten accounts. Among each view's, at the owner's
// sizes, each named for its size, the frames a fluid layout is checked by:
// the Overview on a phone, an iPad and a phone on its side; Accounts'
// Compare on a phone and Detail of work on its side; History's Year and
// Days on its side; Runway's week on its side and on a desktop; Sessions'
// Switchboard on an iPad and its List at 90 × 40; the Log's Requests at 90 ×
// 40; and d28c's page on an iPad. And over the world of long names, so a
// cut name shows, the Switchboard, the List and the Log's Requests on an
// iPad.
var pages = []page{
	{name: "overview-default-160", size: wide(37), world: accountWorlds.auto},
	{name: "overview-pinned-160", size: wide(37), world: accountWorlds.pinned},
	{name: "overview-bad-hour-160", size: wide(37), world: accountWorlds.badHour},
	{name: "overview-by-account-160", size: wide(42), world: accountWorlds.auto, keys: keysOf("b")},
	{name: "overview-models-160", size: wide(37), world: accountWorlds.auto, keys: keysOf("w")},
	{name: "overview-model-nearing-160", size: wide(37), world: accountWorlds.fableNearing},
	{name: "overview-usage-alone-160", size: wide(37), world: accountWorlds.auto, keys: keysOf("r", "c", "l")},
	{name: "overview-everything-160x39", size: wide(39), world: accountWorlds.auto, keys: keysOf("s")},
	{name: "overview-six-accounts-160", size: wide(41), world: manyAccounts(6)},
	{name: "overview-six-running-173x53", size: watch.Size{Width: 173, Height: 53}, world: accountWorlds.sixRunning, keys: keysOf("s")},
	{name: "overview-ten-accounts-160", size: wide(53), world: manyAccounts(10)},
	{name: "overview-default-216", size: broad(70), world: accountWorlds.auto},
	{name: "overview-sessions-216", size: broad(70), world: accountWorlds.auto, keys: keysOf("s")},
	{name: "overview-everything-216x106", size: broad(106), world: accountWorlds.auto, keys: keysOf("s")},
	{name: "overview-phone-52", size: watch.Size{Width: 52, Height: 66}, world: accountWorlds.auto},
	{name: "overview-73x61", size: watch.Size{Width: 73, Height: 61}, world: accountWorlds.auto},
	{name: "overview-108x44", size: watch.Size{Width: 108, Height: 44}, world: accountWorlds.auto},
	{name: "overview-138x15", size: watch.Size{Width: 138, Height: 15}, world: accountWorlds.auto},
	{name: "routing-auto-160", size: wide(37), world: accountWorlds.auto, keys: keysOf("p")},
	{name: "routing-two-160", size: wide(37), world: accountWorlds.auto, keys: keysOf("p", "2", "3")},
	{name: "routing-move-160", size: wide(37), world: accountWorlds.auto, keys: keysOf("p", "2", "m")},
	{name: "routing-limit-160", size: wide(37), world: accountWorlds.limit, keys: keysOf("p", "3")},
	{name: "history-year-160", size: wide(37), world: accountWorlds.auto, keys: toHistory()},
	{name: "history-year-opus-160", size: wide(37), world: accountWorlds.auto, keys: toHistory("v")},
	{name: "history-year-plan-unknown-160", size: wide(37), world: accountWorlds.sidesPlanUnknown, keys: toHistory()},
	{name: "history-weeks-160", size: wide(37), world: accountWorlds.auto, keys: toHistory("]")},
	{name: "history-weeks-columns-160", size: wide(37), world: accountWorlds.auto, keys: toHistory("]", "v")},
	{name: "history-weeks-ten-160", size: wide(37), world: manyAccounts(10), keys: toHistory("]")},
	{name: "history-days-160", size: wide(37), world: accountWorlds.auto, keys: toHistory("]", "]")},
	{name: "history-days-ten-160", size: wide(37), world: manyAccounts(10), keys: toHistory("]", "]")},
	{name: "history-tokens-160", size: wide(37), world: accountWorlds.auto, keys: toHistory("]", "]", "]")},
	{name: "history-tokens-models-160", size: wide(37), world: accountWorlds.auto, keys: toHistory("]", "]", "]", "v")},
	{name: "history-tokens-plan-unknown-160", size: wide(37), world: accountWorlds.sidesPlanUnknown, keys: toHistory("]", "]", "]")},
	{name: "history-phone-year-52", size: watch.Size{Width: 52, Height: 35}, world: accountWorlds.auto, keys: toHistory()},
	{name: "history-year-138x30", size: watch.Size{Width: 138, Height: 30}, world: accountWorlds.auto, keys: toHistory()},
	{name: "history-days-138x30", size: watch.Size{Width: 138, Height: 30}, world: accountWorlds.auto, keys: toHistory("]", "]")},
	{name: "page-year-160x24", size: wide(24), world: accountWorlds.auto, keys: toHistory("j", "j", "j")},
	{name: "page-routing", size: watch.Size{Width: 160, Height: 40}, world: accountWorlds.auto, keys: keysOf("p", "2", "3", "m")},
	{name: "page-routing-phone", size: watch.Size{Width: 52, Height: 36}, world: accountWorlds.auto, keys: keysOf("p", "2", "m")},
	{name: "page-routing-160x16", size: wide(16), world: accountWorlds.auto, keys: keysOf("p")},
	{name: "page-not-answering", size: wide(37), world: accountWorlds.auto, keys: keysOf("R"), gone: true},
	{name: "page-help-scrolled", size: wide(16), world: accountWorlds.auto, keys: keysOf("?", "j", "j", "j")},
	{name: "log-events-160", size: wide(37), world: accountWorlds.auto, keys: toLog("]")},
	{name: "log-requests-160", size: wide(37), world: accountWorlds.auto, keys: toLog()},
	{name: "log-not-live-160", size: wide(37), world: accountWorlds.notLive, keys: toLog(), gone: true, goneFor: notLiveGone},
	{name: "log-requests-phone-52", size: watch.Size{Width: 52, Height: 33}, world: accountWorlds.auto, keys: toLog()},
	{name: "log-requests-90x40", size: watch.Size{Width: 90, Height: 40}, world: accountWorlds.auto, keys: toLog()},
	{name: "log-requests-long-names-108x44", size: watch.Size{Width: 108, Height: 44}, world: accountWorlds.longNames, keys: toLog()},
	{name: "log-event-details-160", size: wide(37), world: accountWorlds.auto, keys: slices.Concat(toLog("]"), repeated(upKey, 5), []tea.KeyPressMsg{enterKey})},
	{
		name: "log-request-details-160", size: wide(37), world: accountWorlds.auto, keys: []tea.KeyPressMsg{enterKey},
		before: earlier{by: 9*time.Minute + 26*time.Second, keys: slices.Concat(toLog(), repeated(upKey, 30), []tea.KeyPressMsg{downKey})},
	},
	{name: "log-events-filter-160", size: wide(37), world: accountWorlds.auto, keys: toLog("]", "f")},
	{
		name: "log-requests-filter-160", size: wide(37), world: accountWorlds.auto,
		keys: slices.Concat(toLog("f"), []tea.KeyPressMsg{rightKey}, repeated(downKey, 12)),
	},
	{
		name: "log-requests-filtered-160", size: wide(37), world: accountWorlds.auto, keys: append(toLog("/", "d", "2", "8", "c"), enterKey),
		log: dashboard.LogTicks{}.Toggled(dashboard.LogWorth),
	},
	{name: "page-log-find", size: wide(37), world: accountWorlds.auto, keys: toLog("/", "s", "w")},
	{name: "page-log-help", size: wide(37), world: accountWorlds.auto, keys: toLog("?")},
	{name: "session-moved-160", size: wide(37), world: accountWorlds.auto, keys: toSession("d28c"), scale: storyScale},
	{name: "session-quiet-160", size: wide(37), world: accountWorlds.auto, keys: toSession("9c2e"), scale: storyScale},
	{name: "session-ended-160", size: wide(37), world: accountWorlds.auto, keys: toSession("b9c4"), scale: storyScale},
	{name: "session-resumed-160", size: wide(37), world: accountWorlds.resumed, keys: toSession("f41a"), scale: storyScale},
	{name: "session-turn-opened-160", size: wide(37), world: accountWorlds.auto, keys: toSession("d28c", downKey, enterKey), scale: storyScale},
	{name: "session-phone-52", size: watch.Size{Width: 52, Height: 32}, world: accountWorlds.auto, keys: toSession("d28c"), scale: storyScale},
	{name: "session-108x44", size: watch.Size{Width: 108, Height: 44}, world: accountWorlds.auto, keys: toSession("d28c"), scale: storyScale},
	{name: "session-138x15", size: watch.Size{Width: 138, Height: 15}, world: accountWorlds.auto, keys: toSession("d28c"), scale: storyScale},
	{name: "routing-session-160", size: wide(37), world: accountWorlds.auto, keys: toSession("9c2e", keysOf("p", "3")...), scale: storyScale},
	{name: "page-session-loading", size: wide(37), world: accountWorlds.auto, keys: toSession("d28c"), scale: storyScale, loading: true},
	{name: "accounts-compare-160", size: wide(37), world: accountWorlds.auto, keys: toAccounts()},
	{name: "accounts-compare-month-160", size: wide(37), world: accountWorlds.auto, keys: toAccounts(">")},
	{name: "accounts-compare-plan-unknown-160", size: wide(37), world: accountWorlds.sidesPlanUnknown, keys: toAccounts()},
	{name: "accounts-compare-ten-160", size: wide(37), world: manyAccounts(10), keys: toAccounts()},
	{name: "accounts-compare-bad-hour-160", size: wide(37), world: accountWorlds.badHour, keys: toAccounts()},
	{name: "accounts-detail-all-160", size: wide(37), world: accountWorlds.auto, keys: toAccounts("]")},
	{name: "accounts-detail-work-160", size: wide(37), world: accountWorlds.auto, keys: toDetail(downKey)},
	{name: "accounts-detail-limit-160", size: wide(37), world: accountWorlds.badHour, keys: toDetail(downKey, downKey)},
	{
		name: "accounts-detail-sessions-160", size: wide(37), world: accountWorlds.auto, view: dashboard.Accounts,
		keys: slices.Concat(keysOf("]"), repeated(downKey, 3), []tea.KeyPressMsg{rightKey, downKey}),
	},
	{
		name: "page-detail-list-focus", size: wide(37), world: accountWorlds.auto, view: dashboard.Accounts,
		keys: slices.Concat(keysOf("]"), repeated(downKey, 3)),
	},
	{
		name: "page-detail-sessions-focus", size: wide(37), world: accountWorlds.auto, view: dashboard.Accounts,
		keys: slices.Concat(keysOf("]"), repeated(downKey, 3), []tea.KeyPressMsg{rightKey, downKey}),
	},
	{name: "accounts-detail-one-160", size: wide(37), world: accountWorlds.oneLately, keys: toAccounts()},
	{name: "accounts-phone-compare-52", size: watch.Size{Width: 52, Height: 48}, world: accountWorlds.auto, keys: toAccounts()},
	{name: "accounts-phone-detail-52", size: watch.Size{Width: 52, Height: 41}, world: accountWorlds.auto, keys: toDetail(downKey)},
	{name: "accounts-phone-compare-plan-unknown-52", size: watch.Size{Width: 52, Height: 48}, world: accountWorlds.sidesPlanUnknown, keys: toAccounts()},
	{name: "accounts-phone-detail-plan-unknown-52", size: watch.Size{Width: 52, Height: 41}, world: accountWorlds.sidesPlanUnknown, keys: toDetail()},
	{name: "accounts-compare-73x61", size: watch.Size{Width: 73, Height: 61}, world: accountWorlds.auto, keys: toAccounts()},
	{name: "accounts-detail-138x30", size: watch.Size{Width: 138, Height: 30}, world: accountWorlds.auto, keys: toDetail(downKey)},
	{name: "sessions-switchboard-160", size: wide(37), world: accountWorlds.scene, keys: toSessions()},
	{name: "sessions-list-160", size: wide(37), world: accountWorlds.scene, keys: toSessions("]")},
	{name: "sessions-phone-52", size: phone(33), world: accountWorlds.scene, keys: toSessions()},
	{name: "sessions-list-phone-52", size: phone(33), world: accountWorlds.scene, keys: toSessions("]")},
	{name: "sessions-108x44", size: watch.Size{Width: 108, Height: 44}, world: accountWorlds.scene, keys: toSessions()},
	{name: "sessions-list-90x40", size: watch.Size{Width: 90, Height: 40}, world: accountWorlds.scene, keys: toSessions("]")},
	{name: "sessions-switchboard-long-names-108x44", size: watch.Size{Width: 108, Height: 44}, world: accountWorlds.longNames, keys: toSessions()},
	{name: "sessions-list-long-names-108x44", size: watch.Size{Width: 108, Height: 44}, world: accountWorlds.longNames, keys: toSessions("]")},
	{
		name: "sessions-ten-accounts-160", size: wide(37), world: accountWorlds.ten, before: lookedAway(time.Minute), keys: cameBack,
		looked: &looked{listed: inTheFramesOrder(tenSessions)},
	},
	{
		name: "sessions-one-account-160", size: wide(37), world: accountWorlds.aloneOnWork, before: lookedAway(time.Minute), keys: cameBack,
		looked: &looked{listed: inTheFramesOrder(onWork)},
	},
	{
		name: "sessions-pinned-160", size: wide(37), world: accountWorlds.pinnedAtTheLimit, before: lookedAway(time.Minute), keys: cameBack,
		looked: &looked{listed: inTheFramesOrder(sceneSessions)},
	},
	{
		name: "sessions-refused-160", size: wide(37), world: accountWorlds.refusedAt, before: lookedAway(refusedAfter), keys: cameBack,
		looked: &looked{after: refusedAfter, listed: inTheFramesOrder(sceneSessions)},
	},
	{
		name: "sessions-moved-160", size: wide(37), world: accountWorlds.movedAt, before: lookedAway(movedAfter), keys: cameBack,
		looked: &looked{after: movedAfter, listed: inTheFramesOrder(sceneSessions)},
	},
	{name: "page-sessions-120x30", size: watch.Size{Width: 120, Height: 30}, world: accountWorlds.scene, keys: toSessions()},
	{name: "page-sessions-chosen", size: wide(37), world: accountWorlds.scene, keys: append(toSessions(), repeated(downKey, 4)...)},
	{name: "page-sessions-short", size: wide(14), world: accountWorlds.scene, keys: append(toSessions(), repeated(downKey, 6)...)},
	{name: "page-sessions-not-answering", size: wide(37), world: accountWorlds.scene, keys: toSessions("R"), gone: true},
	{name: "runway-5-hour-160", size: wide(37), world: accountWorlds.auto, keys: toRunway(">")},
	{name: "runway-week-160", size: wide(37), world: accountWorlds.auto, keys: toRunway()},
	{name: "runway-stepped-back-160", size: wide(37), world: accountWorlds.auto, keys: append(toRunway(">"), leftKey, leftKey)},
	{name: "runway-ten-accounts-160", size: wide(37), world: manyAccounts(10), keys: toRunway()},
	{name: "runway-phone-52", size: phone(33), world: accountWorlds.auto, keys: toRunway()},
	{name: "runway-collapsed-160", size: wide(37), world: accountWorlds.auto, keys: toRunway("a")},
	{name: "runway-collapsed-108x43", size: watch.Size{Width: 108, Height: 43}, world: accountWorlds.auto, keys: toRunway("a")},
	{name: "runway-phone-collapsed-52", size: phone(33), world: accountWorlds.auto, keys: toRunway("a")},
	{name: "runway-138x15", size: watch.Size{Width: 138, Height: 15}, world: accountWorlds.auto, keys: toRunway()},
	{name: "runway-173x53", size: watch.Size{Width: 173, Height: 53}, world: accountWorlds.auto, keys: toRunway()},
	{name: "graphs-160x37", size: wide(37), world: accountWorlds.auto, keys: toGraphs()},
	{name: "graphs-108x44", size: watch.Size{Width: 108, Height: 44}, world: accountWorlds.auto, keys: toGraphs()},
	{name: "graphs-73x61", size: watch.Size{Width: 73, Height: 61}, world: accountWorlds.auto, keys: toGraphs()},
	{name: "graphs-phone-52", size: phone(63), world: accountWorlds.auto, keys: toGraphs()},
	{name: "graphs-four-73x61", size: watch.Size{Width: 73, Height: 61}, world: manyAccounts(4), keys: toGraphs()},
	{name: "graphs-sections-173x53", size: watch.Size{Width: 173, Height: 53}, world: accountWorlds.auto, keys: toGraphs("f", "o", "g", "h")},
	{name: "graphs-sections-180x53", size: watch.Size{Width: 180, Height: 53}, world: accountWorlds.auto, keys: toGraphs("f", "o", "g", "h")},
	{name: "graphs-glance-108x44", size: watch.Size{Width: 108, Height: 44}, world: accountWorlds.auto, keys: toGraphs("f", "g")},
	{name: "graphs-glance-73x61", size: watch.Size{Width: 73, Height: 61}, world: accountWorlds.auto, keys: toGraphs("f", "g")},
	{name: "graphs-pool-73x61", size: watch.Size{Width: 73, Height: 61}, world: accountWorlds.auto, keys: toGraphs("f", "o")},
}

// lookedAway are the keys a look at Sessions' Switchboard by before a page's
// moment pressed: tab three times to reach it, then once more, on to Runway,
// its request stream closing.
func lookedAway(by time.Duration) earlier {
	return earlier{by: by, keys: append(toSessions(), tabKey)}
}

// cameBack are the keys pressed at the moment of a page looked away from, as
// lookedAway has it: shift-tab, back to Sessions, its stream opening again.
var cameBack = []tea.KeyPressMsg{{Code: tea.KeyTab, Mod: tea.ModShift}}

// keysOf are the keys pressed as the text given, a key a character.
func keysOf(text ...string) []tea.KeyPressMsg {
	var keys []tea.KeyPressMsg
	for _, t := range text {
		keys = append(keys, tea.KeyPressMsg{Code: []rune(t)[0], Text: t})
	}
	return keys
}

// toHistory are the keys that reach History from the Overview, tab twice,
// then the text given, a key a character.
func toHistory(text ...string) []tea.KeyPressMsg {
	return append([]tea.KeyPressMsg{{Code: tea.KeyTab}, {Code: tea.KeyTab}}, keysOf(text...)...)
}

// toAccounts are the keys that reach Accounts from the Overview, tab, then
// the text given, a key a character.
func toAccounts(text ...string) []tea.KeyPressMsg {
	return append([]tea.KeyPressMsg{{Code: tea.KeyTab}}, keysOf(text...)...)
}

// toDetail are the keys that reach Accounts' Detail from the Overview, tab
// and ], then the keys given.
func toDetail(then ...tea.KeyPressMsg) []tea.KeyPressMsg {
	return append(toAccounts("]"), then...)
}

// toSessions are the keys that reach Sessions from the Overview, tab three
// times, then the text given, a key a character.
func toSessions(text ...string) []tea.KeyPressMsg {
	return append(repeated(tabKey, 3), keysOf(text...)...)
}

// toRunway are the keys that reach Runway from the Overview, tab four times,
// then the text given, a key a character.
func toRunway(text ...string) []tea.KeyPressMsg {
	return append(repeated(tabKey, 4), keysOf(text...)...)
}

// toGraphs are the keys that reach Graphs from the Overview, shift-tab twice,
// back round the tabs past the Log, then the text given, a key a character.
func toGraphs(text ...string) []tea.KeyPressMsg {
	return append(repeated(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}, 2), keysOf(text...)...)
}

// toLog are the keys that reach the Log from the Overview, shift-tab, back
// round the tabs, then the text given, a key a character.
func toLog(text ...string) []tea.KeyPressMsg {
	return append([]tea.KeyPressMsg{{Code: tea.KeyTab, Mod: tea.ModShift}}, keysOf(text...)...)
}

// toSession are the keys that reach a session's page from the Overview,
// through the Log, as the Overview's SESSIONS has no room for it at the
// frames' size, nor ever shows an ended session: the Log's find, / and the
// session's short id, enter to filter the Log to its lines, ↑ to its
// newest, and s to open its page; then the keys given.
func toSession(id string, then ...tea.KeyPressMsg) []tea.KeyPressMsg {
	keys := append(toLog("/"), keysOf(strings.Split(id, "")...)...)
	return slices.Concat(keys, []tea.KeyPressMsg{enterKey, upKey}, keysOf("s"), then)
}

// The keys tab, ↑, ↓, ←, → and enter, as pressed.
var (
	tabKey   = tea.KeyPressMsg{Code: tea.KeyTab}
	upKey    = tea.KeyPressMsg{Code: tea.KeyUp}
	downKey  = tea.KeyPressMsg{Code: tea.KeyDown}
	leftKey  = tea.KeyPressMsg{Code: tea.KeyLeft}
	rightKey = tea.KeyPressMsg{Code: tea.KeyRight}
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
)

// repeated is the key given pressed n times.
func repeated(key tea.KeyPressMsg, n int) []tea.KeyPressMsg {
	return slices.Repeat([]tea.KeyPressMsg{key}, n)
}

// wide is a terminal as wide as the frames are, 160 columns, and as tall as
// given.
func wide(height int) watch.Size {
	return watch.Size{Width: 160, Height: height}
}

// phone is a terminal as wide as the phone's frames are, 52 columns, and as
// tall as given.
func phone(height int) watch.Size {
	return watch.Size{Width: 52, Height: height}
}

// broad is a terminal as wide as the Overview's widest frames, 216 columns,
// and as tall as given.
func broad(height int) watch.Size {
	return watch.Size{Width: 216, Height: height}
}

// manyAccounts picks the world of n accounts of the worlds.
func manyAccounts(n int) func(accountWorlds) accountWorld {
	return func(a accountWorlds) accountWorld { return a.many(n) }
}
