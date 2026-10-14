package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// SessionsView is one of Sessions' views, its sub-tabs: the Switchboard, the
// default, and the List.
type SessionsView int

const (
	// SessionsSwitchboard draws each session running as a call, its cord
	// plugged into its account's line.
	SessionsSwitchboard SessionsView = iota
	// SessionsList lists today's sessions, and those the router lists.
	SessionsList
)

// sessionsViews are Sessions' sub-tabs' names, in the heading row's order.
var sessionsViews = []string{"Switchboard", "List"}

// SessionsShown is Sessions as the watch gives it: the sub-tab shown, which
// the preferences keep; who sits where on the Switchboard, kept from look to
// look as views.Patch keeps it, and the calls the look found, as
// views.CallsOf has them; the List, as the List held builds it; and the row
// chosen, which holds for the run alone. The watch builds each once a look:
// what draws them reads what it built.
type SessionsShown struct {
	Shown   SessionsView
	Patched views.Patched
	Calls   []views.Call
	List    views.SessionList
	// Chosen is the row chosen: on the Switchboard, a session's call on an
	// account, as each of a session's rows is chosen on its own; on the
	// List, a session, its Account "". It's the zero Plug while none is.
	Chosen Plug
}

// SessionsKept is Sessions as the preferences kept it: the sub-tab, by its
// name. What names none, as a later build's, shows as Sessions opens: the
// Switchboard.
func SessionsKept(kept theme.SessionsPrefs) SessionsShown {
	return SessionsShown{Shown: SessionsView(placeAmong(kept.View, lowerNames(sessionsViews)))}
}

// Kept is what the preferences keep of Sessions, as SessionsKept reads it:
// the sub-tab shown, never what's chosen.
func (s SessionsShown) Kept() theme.SessionsPrefs {
	return theme.SessionsPrefs{View: lowerNames(sessionsViews)[s.Shown]}
}

// heading is Sessions' heading row: its name and its sub-tabs, the one shown
// picked out, set apart on a phone as the Log's are.
func (s SessionsShown) heading() heading {
	return heading{name: strings.ToUpper(Sessions.Title()), subTabs: sessionsViews, shown: int(s.Shown)}
}

// Showing is Sessions with the sub-tab at place i shown, as a click on it
// shows it, its choice kept where its session shows there, as chosenThere
// has it, reporting false where it's shown already, or is no sub-tab.
func (s SessionsShown) Showing(i int, accounts []string) (SessionsShown, bool) {
	if i < 0 || i >= len(sessionsViews) || SessionsView(i) == s.Shown {
		return s, false
	}
	s.Shown = SessionsView(i)
	s.Chosen = s.chosenThere(accounts)
	return s, true
}

// Turned is Sessions with the sub-tab by places on from the one shown
// shown, round, as ] and [ move, a place each, as Showing has it.
func (s SessionsShown) Turned(by int, accounts []string) (SessionsShown, bool) {
	return s.Showing(round(int(s.Shown)+by, len(sessionsViews)), accounts)
}

// chosenThere is the row of the sub-tab shown that keeps the choice, its
// session's: on the List, its session's row; on the Switchboard, its
// session's first call, on the lines of the accounts given; none where its
// session shows on neither.
func (s SessionsShown) chosenThere(accounts []string) Plug {
	stops := s.stops(accounts)
	if i := slices.IndexFunc(stops, func(p Plug) bool { return p.Session == s.Chosen.Session }); i >= 0 {
		return stops[i]
	}
	return Plug{}
}

// stops are the rows ↑ and ↓ choose among, top to bottom, on the sub-tab
// shown: on the Switchboard, each call, the lines of the accounts given in
// their order, each line's calls in the order it keeps them; on the List,
// its sessions.
func (s SessionsShown) stops(accounts []string) []Plug {
	var stops []Plug
	if s.Shown == SessionsList {
		for _, l := range s.List.Sessions {
			stops = append(stops, Plug{Session: l.Session})
		}
		return stops
	}
	for _, a := range accounts {
		for _, c := range s.Patched.Line(a).Calls {
			stops = append(stops, Plug{Session: c.Session, Account: a})
		}
	}
	return stops
}

// Choosing is Sessions with the row by rows on from the one chosen chosen,
// as ↑ and ↓ choose, among those of the sub-tab shown, on the lines of the
// accounts given, as stops has them, no further than the first and the
// last: from none, ↓ chooses the first and ↑ the last. It reports false
// where the choice stays, as at either end, or with no row to choose.
func (s SessionsShown) Choosing(by int, accounts []string) (SessionsShown, bool) {
	stops := s.stops(accounts)
	if len(stops) == 0 {
		return s, false
	}
	i := slices.Index(stops, s.Chosen)
	switch {
	case i < 0 && by < 0:
		i = len(stops) - 1
	case i < 0:
		i = 0
	default:
		i = min(max(i+by, 0), len(stops)-1)
	}
	was := s.Chosen
	s.Chosen = stops[i]
	return s, s.Chosen != was
}

// ChosenOn is Sessions with the row given chosen, as a click on it chooses
// it.
func (s SessionsShown) ChosenOn(row Plug) SessionsShown {
	s.Chosen = row
	return s
}

// Unchosen is Sessions with no row chosen, as esc ends the choice, reporting
// false where none was.
func (s SessionsShown) Unchosen() (SessionsShown, bool) {
	was := s.Chosen
	s.Chosen = Plug{}
	return s, was != Plug{}
}

// Opens reports whether enter opens a session's page: while a row is
// chosen, its session's.
func (s SessionsShown) Opens() bool {
	return s.Chosen != Plug{}
}

// sessionsForm is Sessions' view shown as its form draws it: what it draws
// from row top; how far it scrolls, scrolled to each offset; what of it
// scrolls, from column x along row y, width cells wide and rows tall; the
// zones a click acts on; and how much of each bar it draws it fills, as
// Filled has them. Each is nothing where it's nil: it draws nothing, never
// scrolls, its view scrolls whole, nothing takes a click, and it fills no
// bar.
type sessionsForm struct {
	draw      func(c *canvas, top int)
	scrolling func(offset int) Scrolling
	scrolls   func() (x, y, width, rows int)
	targets   func() []Target
	filled    func() map[Ref]float64
}

// sessionsForm is the form Sessions' view shown is drawn in, of doc at now:
// the List's, as listForm has it; else the Switchboard laid out as
// switchboardLaid lays it, in lanes, as lanesForm draws it, where it says
// so, as on a phone, and with cords, as switchboardForm draws it, where it
// doesn't.
func (f Frame) sessionsForm(doc status.Document, now time.Time) sessionsForm {
	if f.SessionsTab.Shown == SessionsList {
		return f.listForm(doc, now)
	}
	laid := f.switchboardLaid(doc, now)
	if laid.lanes {
		return f.lanesForm(doc, now, laid)
	}
	return f.switchboardForm(doc, now, laid)
}

// switchboardLaid is the Switchboard of doc at now laid out in the frame's
// view, as sessionsLaid lays it: on a line for each of the moment's accounts,
// in their order, each drawing a row for each window usage shows of it.
func (f Frame) switchboardLaid(doc status.Document, now time.Time) boardLayout {
	return sessionsLaid(f.Width, f.viewRows(), f.SessionsTab.Patched, boardLinesOf(f.Moment(doc, now)))
}

// boardLinesOf are the Switchboard's lines of the moment m: its accounts',
// in their order, each drawing a row for each window UsageWindows has of m.
func boardLinesOf(m status.Moment) boardLines {
	lines := boardLines{windows: len(UsageWindows(m))}
	for _, a := range m.Accounts {
		lines.accounts = append(lines.accounts, a.ID)
	}
	return lines
}

// sessions draws Sessions' view shown of doc at now from row top, as its
// form draws it.
func (f Frame) sessions(c *canvas, doc status.Document, now time.Time, top int) {
	if form := f.sessionsForm(doc, now); form.draw != nil {
		form.draw(c, top)
	}
}

// sessionsScrolling is how far Sessions' view shown of doc at now scrolls,
// scrolled to each offset given, as its form says: nowhere where it doesn't
// say.
func (f Frame) sessionsScrolling(doc status.Document, now time.Time) func(offset int) Scrolling {
	if form := f.sessionsForm(doc, now); form.scrolling != nil {
		return form.scrolling
	}
	return func(int) Scrolling { return Scrolling{} }
}

// sessionsScrolls is what of Sessions' view shown of doc at now scrolls, as
// its form says: else the view's rows, across the frame.
func (f Frame) sessionsScrolls(doc status.Document, now time.Time) (x, y, width, rows int) {
	if form := f.sessionsForm(doc, now); form.scrolls != nil {
		return form.scrolls()
	}
	return 0, f.top(), f.Width, f.viewRows()
}

// sessionsTargets are the zones of Sessions' view shown of doc at now a
// click acts on, as its form lays them out.
func (f Frame) sessionsTargets(doc status.Document, now time.Time) []Target {
	if form := f.sessionsForm(doc, now); form.targets != nil {
		return form.targets()
	}
	return nil
}

// sessionsFilled are how much of each bar Sessions' view shown of doc at now
// draws it fills, as Filled has them, as its form says.
func (f Frame) sessionsFilled(doc status.Document, now time.Time) map[Ref]float64 {
	if form := f.sessionsForm(doc, now); form.filled != nil {
		return form.filled()
	}
	return nil
}
