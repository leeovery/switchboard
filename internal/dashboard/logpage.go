package dashboard

import (
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// LogView is one of the Log's tables, its sub-tabs: Requests, the default,
// and Events.
type LogView int

const (
	// LogRequests is a line a request the router routes.
	LogRequests LogView = iota
	// LogEvents is a line a thing the router decides or notices.
	LogEvents
)

// logViews are the Log's sub-tabs' names, in the heading row's order.
var logViews = []string{"Requests", "Events"}

// LogPage is the Log as the watch gives it: the sub-tab shown; its tables,
// as the watch built them of the ledger's lines, the router's events and
// the request stream, over the days from From, and the figures of those
// days, which their day rules say; today; what the Requests show of their
// columns and of the lines, and the Events of theirs; how each table
// stands; whether the stream is live, what it tells, and to when the ledger
// wrote what the tables hold; the find, while it's open; and the tables'
// lines as drawn, where Lined holds them.
type LogPage struct {
	Shown LogView
	// Read is set once the first read lands: till then, the view is blank.
	Read     bool
	Requests views.LogTable
	Events   views.EventsTable
	Totals   []views.DayTotals
	From     views.LedgerDay
	Today    views.LedgerDay
	// Columns are the Requests' columns ticked, as f ticks them and the
	// preferences keep them; Filter what lines they show, and Shows what
	// events the Events show.
	Columns LogTicks
	Filter  views.LogFilter
	Shows   views.EventFilter
	// Stands are how the Requests and the Events each stand, by LogView.
	Stands [2]LogStand
	// Live is set while the request stream is the watch's, the router
	// answering; Stream is what it tells as the page is drawn, which a
	// request's row in flight is drawn as, as views.RequestRow.Live has it;
	// Written is when the ledger last wrote of what the Requests hold, which
	// the state line says while it isn't.
	Live    bool
	Stream  views.Stream
	Written time.Time
	Finding LogFinding
	lined   *tableLines
}

// LogStand is how one of the Log's tables stands: following, its newest
// line at the foot, as the zero LogStand does; or paused, its view from the
// line top, no further down than foot, the table's last line as it paused,
// the line chosen lit where one is; and, of Requests', the table as it
// paused, which says the lines come since. filledAt is the rows of the
// tallest view the table has filled, or reached the oldest day in, none
// till it does: after, it reads back only as its top is reached, or,
// following, as its view grows past them, where it refills, as refills
// says.
type LogStand struct {
	paused    bool
	top, foot LogKey
	chosen    LogKey
	choosing  bool
	then      views.LogTable
	filledAt  int
}

// LogKey is a line of one of the Log's tables as each built afresh knows it,
// so a line chosen stays chosen and a table paused stays where it was: a line
// of the Requests' as views.LineKey knows it; an event's row by its key; and
// an Events' day rule, by the days it falls between, the oldest day's line
// and the words of a table with nothing today, each by what it is.
type LogKey struct {
	request views.LineKey
	event   views.EventKey
	other   string
}

// is reports whether k and o are the same event's row, or the same of an
// Events' other lines.
func (k LogKey) is(o LogKey) bool {
	return k.event.Run.Equal(o.event.Run) && k.event.ID == o.event.ID && k.other == o.other
}

// The keys of the lines a table draws that aren't its rows: the oldest
// day's line, the words of a table with nothing today, and before the dates
// it falls between, a day rule's.
const (
	oldestKey = "oldest"
	emptyKey  = "empty"
	ruleKey   = "rule "
)

// logLine is a line of one of the Log's tables as it's drawn: a request's
// row; an event's; a day's rule; the oldest day's line; or the words a table
// with nothing today says at its foot. Its key is who it is, and at, of a
// row, when it came.
type logLine struct {
	key    LogKey
	at     time.Time
	row    *views.RequestRow
	event  *views.LogEvent
	rule   *views.DayRule
	oldest *views.OldestDay
	empty  line
}

// choosable reports whether the line is a row, the one kind the Log
// chooses: never a day rule, the oldest day's line, or a table's words.
func (l logLine) choosable() bool {
	return l.row != nil || l.event != nil
}

// lines are the lines of the table shown, the oldest first, as the frame
// draws them: those Lined holds, while they're of the table; else made
// afresh.
func (p LogPage) lines() []logLine {
	if p.Shown == LogEvents {
		if h := p.lined; h != nil && h.events.of(p.Events.Lines, p.eventsEmpty()) {
			return h.events.lines
		}
		return p.eventLines()
	}
	if h := p.lined; h != nil && h.requests.of(p.Requests.Lines, p.requestsEmpty()) {
		return h.requests.lines
	}
	return p.requestLines()
}

// Lined is p with the lines of both its tables, as the frame draws them,
// made once and held, for each read of them to take while its table is the
// one they were made of: the watch lines each page it builds, so a table is
// made into lines once a build, not at each look at it.
func (p LogPage) Lined() LogPage {
	if h := p.lined; h != nil && h.requests.of(p.Requests.Lines, p.requestsEmpty()) && h.events.of(p.Events.Lines, p.eventsEmpty()) {
		return p
	}
	p.lined = &tableLines{
		requests: heldLines[views.LogLine]{made: p.Requests.Lines, empty: p.requestsEmpty(), lines: p.requestLines()},
		events:   heldLines[views.EventsLine]{made: p.Events.Lines, empty: p.eventsEmpty(), lines: p.eventLines()},
	}
	return p
}

// tableLines are the lines of the Log's tables as Lined holds them.
type tableLines struct {
	requests heldLines[views.LogLine]
	events   heldLines[views.EventsLine]
}

// heldLines are a table's lines as drawn, made of its lines as views gave
// them, the table saying it had nothing today where empty is set.
type heldLines[T any] struct {
	made  []T
	empty bool
	lines []logLine
}

// of reports whether h were made of the lines given, the very ones, the
// table saying it had nothing today where empty is set.
func (h heldLines[T]) of(lines []T, empty bool) bool {
	return h.empty == empty && len(h.made) == len(lines) && (len(lines) == 0 || &h.made[0] == &lines[0])
}

// stand is how the table shown stands.
func (p LogPage) stand() LogStand {
	return p.Stands[p.Shown]
}

// standing is p with the table shown standing as s.
func (p LogPage) standing(s LogStand) LogPage {
	p.Stands[p.Shown] = s
	return p
}

// find is the place among lines of the line whose key is k: false where
// they hold none.
func (p LogPage) find(lines []logLine, k LogKey) (int, bool) {
	if p.Shown == LogRequests && k.other == "" {
		return p.Requests.Find(k.request)
	}
	for i, l := range lines {
		if l.key.is(k) {
			return i, true
		}
	}
	return 0, false
}

// window is what of lines is in view in rows rows, as s stands: following,
// the newest rows lines; paused, from its top, but no further down than its
// foot, or the table's words under it, while it says it has nothing today.
// A foot no longer among them, as one a filter hides, falls back to the last
// line; a top, to the view ending at the foot, but rising to the line chosen
// where that's above it.
func (p LogPage) window(lines []logLine, s LogStand, rows int) (first, last int) {
	last = len(lines) - 1
	if !s.paused {
		return max(len(lines)-rows, 0), last
	}
	if foot, ok := p.find(lines, s.foot); ok {
		last = foot
		if last+1 < len(lines) && lines[last+1].empty != nil {
			last++
		}
	}
	top, ok := p.find(lines, s.top)
	if !ok {
		top = max(last-rows+1, 0)
		if at, chosen := p.lit(lines, s); chosen {
			top = min(top, at)
		}
	}
	first = min(top, last)
	return first, min(last, first+rows-1)
}

// lit is the place of the line chosen among lines, as s stands: false
// where none is chosen, or it's no longer among them.
func (p LogPage) lit(lines []logLine, s LogStand) (int, bool) {
	if !s.choosing {
		return 0, false
	}
	return p.find(lines, s.chosen)
}

// placedRows is how many rows a view rows rows tall is placed by, as the keys
// move it: a view the terminal leaves no row for stands as one a row tall
// would, its top the line it would show first, so it shows what's chosen as
// the terminal grows.
func placedRows(rows int) int {
	return max(rows, 1)
}

// Turned is the Log showing the sub-tab by places on from the one shown,
// round, as [ and ] step: each table stands as it stood.
func (p LogPage) Turned(by int) LogPage {
	n := len(logViews)
	p.Shown = LogView(((int(p.Shown)+by)%n + n) % n)
	return p
}

// Followed is the table shown following again, as end has it: its newest
// line at the foot, and nothing chosen.
func (p LogPage) Followed() LogPage {
	return p.standing(LogStand{filledAt: p.stand().filledAt})
}

// Paused reports whether the table shown is paused.
func (p LogPage) Paused() bool {
	return p.stand().paused
}

// Chosen is the row of the table shown that's chosen, where one is: false
// where none is.
func (p LogPage) Chosen() (views.RequestRow, *views.LogEvent, bool) {
	lines := p.lines()
	i, ok := p.lit(lines, p.stand())
	if !ok {
		return views.RequestRow{}, nil, false
	}
	if r := lines[i].row; r != nil {
		return *r, nil, true
	}
	return views.RequestRow{}, lines[i].event, true
}

// ChosenSession is the session the line chosen of the table shown is of,
// whose page s opens from it: a request's, the session it counts toward, as
// views' CountedSession has it, none for a quota check; or an event's. It's
// "" where none is chosen, or it's of none.
func (p LogPage) ChosenSession() string {
	if line, ok := p.ChosenRequest(); ok {
		return line.CountedSession()
	}
	if _, event, ok := p.Chosen(); ok && event != nil {
		return event.Session
	}
	return ""
}

// Chose is the table shown in a view rows rows tall, its choice moved by
// places, ↑ -1 and ↓ 1, as the keys move it: following, ↑ chooses the newest
// row and pauses there, ↓ doing nothing; paused with a row chosen, the row
// by places on from it, but no further than the first held or the paused
// foot, the view scrolling to keep it in; paused with none chosen, either
// chooses the newest row in view, or above it. A day rule, the oldest day's
// line and a table's words are passed over, and a table holding no row never
// pauses. A view of no rows is placed as placedRows has it.
func (p LogPage) Chose(by, rows int) LogPage {
	lines, s, rows := p.lines(), p.stand(), placedRows(rows)
	if !s.paused && (by > 0 || !holdsRow(lines)) {
		return p
	}
	if !s.paused {
		s = p.pausedAt(lines, rows)
	}
	_, last := p.window(lines, s, rows)
	at, chosen := p.lit(lines, s)
	switch {
	case !chosen:
		at = newestRow(lines, 0, last)
	default:
		at = nextRow(lines, at, by, p.footOf(lines, s))
	}
	if at < 0 {
		return p.standing(s)
	}
	return p.standing(p.choosing(lines, s, at, rows))
}

// choosing is s with the line at at among lines chosen, its view, rows rows
// tall, scrolled to keep it in: to the line, where it's above the view, or
// to end with it, where it's below.
func (p LogPage) choosing(lines []logLine, s LogStand, at, rows int) LogStand {
	s.chosen, s.choosing = lines[at].key, true
	switch first, last := p.window(lines, s, rows); {
	case at < first:
		s.top = lines[at].key
	case at > last:
		s.top = lines[max(at-rows+1, 0)].key
	}
	return s
}

// Landed is the Log showing Requests in a view rows rows tall, paused at
// its newest line and the first request at or after at chosen, the view
// scrolled to keep it in, as g lands from an event's details: as it stood
// where no request is. A view of no rows is placed as placedRows has it.
func (p LogPage) Landed(at time.Time, rows int) LogPage {
	p.Shown = LogRequests
	i, ok := p.Requests.At(at)
	if !ok {
		return p
	}
	lines, rows := p.Followed().lines(), placedRows(rows)
	return p.standing(p.choosing(lines, p.Followed().pausedAt(lines, rows), i, rows))
}

// Scrolled is the table shown in a view rows rows tall, scrolled by rows
// down, or up where by is less than nothing, as j, k, PgUp, PgDn and the
// wheel scroll it: following, scrolled up, it pauses there, nothing chosen,
// where it holds a row; paused, its view goes no higher than the first line
// held, nor lower than to end at the paused foot. A view of no rows is
// placed as placedRows has it.
func (p LogPage) Scrolled(by, rows int) LogPage {
	lines, s, rows := p.lines(), p.stand(), placedRows(rows)
	if len(lines) == 0 || !s.paused && (by >= 0 || !holdsRow(lines)) {
		return p
	}
	if !s.paused {
		s = p.pausedAt(lines, rows)
	}
	first, _ := p.window(lines, s, rows)
	foot := p.footOf(lines, s)
	to := min(max(first+by, 0), max(foot-rows+1, 0))
	s.top = lines[to].key
	return p.standing(s)
}

// pausedAt is the table shown, of lines, paused where it follows in a view
// rows rows tall: its view as it is, to the table's last line that came,
// which it goes no further down than, the words saying it has nothing today
// passed over, as they go once today's first comes; and of Requests', the
// table as it is, which the lines come since are counted against.
func (p LogPage) pausedAt(lines []logLine, rows int) LogStand {
	s := p.stand()
	first, last := p.window(lines, s, rows)
	if lines[last].empty != nil && last > 0 {
		last--
	}
	s.paused, s.top, s.foot, s.then = true, lines[first].key, lines[last].key, p.Requests
	return s
}

// footOf is the place of the paused foot among lines, as s stands: the last
// line, where it's no longer among them.
func (p LogPage) footOf(lines []logLine, s LogStand) int {
	if foot, ok := p.find(lines, s.foot); ok {
		return foot
	}
	return len(lines) - 1
}

// holdsRow reports whether lines hold a row, which a table pauses at.
func holdsRow(lines []logLine) bool {
	return newestRow(lines, 0, len(lines)-1) >= 0
}

// newestRow is the place of the newest row of lines from first to last:
// -1 where none is a row.
func newestRow(lines []logLine, first, last int) int {
	for i := last; i >= first; i-- {
		if lines[i].choosable() {
			return i
		}
	}
	return -1
}

// nextRow is the place of the row by places on from the row at among
// lines, no further than the first of them, nor than foot: -1 where there's
// none between.
func nextRow(lines []logLine, at, by, foot int) int {
	for i := at + by; i >= 0 && i <= foot; i += by {
		if lines[i].choosable() {
			return i
		}
	}
	return -1
}

// Filling is the Log, the table shown filled at a view rows rows tall once
// it fills it, or reaches as far back as it reads, from where it reads back
// only as WantsDayBefore has a filled table read back. A view of no rows
// fills nothing, so the table fills the view it's given as the terminal
// grows.
func (p LogPage) Filling(rows int) LogPage {
	s := p.stand()
	if s.filledAt >= rows || !p.Read {
		return p
	}
	if len(p.lines()) >= rows || p.reached() {
		s.filledAt = rows
	}
	return p.standing(s)
}

// WantsDayBefore reports whether the table shown, in a view rows rows tall,
// wants the day before those it spans read: on opening, until it fills its
// view, and following, until it fills one grown past the rows it filled,
// where it refills, as refills says; and paused, its view's top at the
// first line held. Never once it reaches as far back as it reads.
func (p LogPage) WantsDayBefore(rows int) bool {
	if !p.Read || p.reached() {
		return false
	}
	lines, s := p.lines(), p.stand()
	if s.filledAt == 0 || !s.paused && rows > s.filledAt && p.refills(p.Shown) {
		return len(lines) < rows
	}
	first, _ := p.window(lines, s, rows)
	return s.paused && first == 0
}

// reached reports whether the table shown reaches as far back as it reads:
// the oldest day the ledger keeps, as the Requests' oldest day's line, atop
// them, says; or of the Events, the oldest they read back to, as their
// Reached says.
func (p LogPage) reached() bool {
	return p.oldest() != nil || p.Shown == LogEvents && p.Events.Reached()
}

// oldest is the oldest day the ledger keeps, where the tables reach it: nil
// where they don't, or it's not known.
func (p LogPage) oldest() *views.OldestDay {
	if l := p.Requests.Lines; len(l) > 0 {
		return l[0].Oldest
	}
	return nil
}

// heading is the Log's heading row, on a phone where phone says, width cells
// wide: its name and its sub-tabs, the one shown picked out, and what the
// filter keeps, as the heading says it, each account named as name names it;
// at its right, the table's count, as logHeadingRight picks its form, or on
// a phone, as phone has it.
func (p LogPage) heading(phone bool, width int, name func(id string) string) heading {
	h := heading{name: strings.ToUpper(Log.Title()), subTabs: logViews, shown: int(p.Shown)}
	if !p.Read {
		return h
	}
	count := p.headingCount()
	h.state = p.filterNames(name).said()
	if phone {
		h.right = count.phone()
		return h
	}
	h.right = logHeadingRight(count.forms(), h.state, width-h.named(new(laying), 0, false))
	return h
}

// headingCount is what the heading counts of the table shown: of Requests,
// today's lines hidden, and where their kinds alone hide them, which; of
// Events, today's events, those hidden among them.
func (p LogPage) headingCount() logHeadingCount {
	if p.Shown == LogEvents {
		return todayCount(p.Events.Today.Shown, p.Events.Today.All)
	}
	count := p.today()
	return hiddenCount(count.Hidden(), kindsNamed(count.HiddenBy()))
}

// today is what today's lines count, as the Requests' table counts them.
func (p LogPage) today() views.LogCount {
	return p.Requests.Count(p.Today.Date)
}

// rowKindNames are the kinds of the ledger's lines as the heading names
// them, by the kind.
var rowKindNames = map[views.RowKind]string{views.RowRequest: "requests", views.RowCheck: "checks", views.RowCount: "counts"}

// kindsNamed names the kinds given as a list, as "checks and counts".
func kindsNamed(kinds []views.RowKind) string {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = rowKindNames[k]
	}
	return strings.Join(names, " and ")
}

// filterNames are what the filter of the table shown keeps, each named as
// the table names it: of the Requests', the sessions, accounts and models it
// ticks, and whether it keeps those that failed alone; of the Events', the
// sessions and accounts it ticks. The kinds it ticks the heading's count
// says. A session is named by its id and directory, as a row of it does,
// else its id alone; an account as name names it, as the rows name it; a
// model by its id less claude-.
func (p LogPage) filterNames(name func(id string) string) logFilterNames {
	if p.Shown == LogEvents {
		return logFilterNames{sessions: namedEach(p.Shows.Sessions, p.namedSession), accounts: namedEach(p.Shows.Accounts, name)}
	}
	f := p.Filter
	return logFilterNames{
		sessions: namedEach(f.Sessions, p.namedSession), accounts: namedEach(f.Accounts, name), models: namedEach(f.Models, modelNamed), failed: f.Failed,
	}
}

// modelNamed names the model with the given id as the table does, by its id
// less claude-.
func modelNamed(id string) string {
	return ledger.ShortModelID(prose.Clean(id))
}

// namedEach is each of the ids, named as name names it.
func namedEach(ids []string, name func(id string) string) []string {
	var named []string
	for _, id := range ids {
		named = append(named, name(id))
	}
	return named
}

// namedSession names the session with the given id as sessionNamed does,
// by its directory as dirOf finds it.
func (p LogPage) namedSession(id string) string {
	return sessionNamed(id, p.dirOf(id))
}

// dirOf is the directory of the session with the given id as the Requests'
// rows name it, else the Events': "" where none does.
func (p LogPage) dirOf(id string) string {
	for _, l := range p.Requests.Lines {
		if l.IsRow() && l.Row.Session == id && l.Row.Dir != "" {
			return l.Row.Dir
		}
	}
	for _, e := range p.Events.Rows {
		if e.Session == id && e.Dir != "" {
			return e.Dir
		}
	}
	return ""
}

// sessionNamed names a session by its id and its directory, as the Log's
// session column does: "d28c switchboard", the id alone where there's no
// directory.
func sessionNamed(id, dir string) string {
	if name := status.DirName(dir, views.SessionDirCells); name != "" {
		return status.ShownID(id) + " " + name
	}
	return status.ShownID(id)
}

// state is the Log's state line of the table shown: paused, at the line
// chosen, or the newest in view, with the lines come since; not live, to
// when the ledger last wrote; else following; and at its right how many of
// today's lines, or events, the table shows, of a session alone where the
// filter keeps one.
func (p LogPage) state(rows int) logState {
	lines, s := p.lines(), p.stand()
	st := logState{shown: p.today().Shown, noun: "line", of: p.sessionAlone()}
	if p.Shown == LogEvents {
		st = logState{shown: p.Events.Today.Shown, noun: "event"}
	}
	switch {
	case s.paused:
		st.standing, st.at, st.fresh = logPaused, p.pausedTime(lines, s, rows), p.fresh(lines, s)
	case !p.Live:
		st.standing, st.at = logNotLive, p.Written
	}
	return st
}

// pausedTime is when the line the table, of lines, is paused at came: the
// line chosen, else the newest row in view, or above it.
func (p LogPage) pausedTime(lines []logLine, s LogStand, rows int) time.Time {
	if at, ok := p.lit(lines, s); ok {
		return lines[at].at
	}
	_, last := p.window(lines, s, rows)
	if at := newestRow(lines, 0, last); at >= 0 {
		return lines[at].at
	}
	return time.Time{}
}

// fresh is how many lines have come since the table shown, of lines,
// paused: of Requests', its rows of requests the table as it paused held
// none of, as its filter shows them; of Events', those after its paused
// foot.
func (p LogPage) fresh(lines []logLine, s LogStand) int {
	if p.Shown == LogRequests {
		return p.Requests.NewSince(s.then)
	}
	n := 0
	for _, l := range lines[p.footOf(lines, s)+1:] {
		if l.choosable() {
			n++
		}
	}
	return n
}

// sessionAlone is the session the Requests' filter keeps alone, by its id
// as the table names it: "" where it keeps none, or several.
func (p LogPage) sessionAlone() string {
	if len(p.Filter.Sessions) != 1 {
		return ""
	}
	return status.ShownID(p.Filter.Sessions[0])
}

// The Log's page's rows around its table: its heads, and the blank row
// under the table, which keeps it off the state line.
const logAroundTable = 2

// LogRows is how many of the Log's lines its view shows at once: the
// view's rows but its heads and the blank row under it.
func (f Frame) LogRows() int {
	return max(f.viewRows()-logAroundTable, 0)
}
