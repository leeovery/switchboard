package watch

import (
	"errors"
	"io/fs"
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayTab is what the watch holds of Runway: what it shows, its timeline,
// lanes and COMING UP as the last look built them; the lanes themselves,
// which hold what they judged of the past till what that's judged from
// changes, and the moment of the message they were built at, as each is
// built once; the readings history's live tail, read at each look for what
// it gained, and the readings of each span stepped back to before it, read
// once and held for the run; the readings the watch read of the documents it
// read itself; the start of the first day the history holds, as the live
// tail last asked it, zero where it holds none; whether a look since asks
// what the live tail gained; and whether a read of the history is under way,
// one at a time.
type runwayTab struct {
	shown   dashboard.RunwayShown
	lanes   views.Lanes
	look    func(views.Lanes, views.LanesFrom) views.Lanes
	builtAt *status.HeldMoment
	live    readingsTail
	back    []heldSpan
	seen    seenReadings
	kept    time.Time
	due     bool
	reading bool
}

// readingsTail is a read of the readings history over a span, as
// views.Readings' Tail reads it, and what it read last: none before its
// first read lands.
type readingsTail struct {
	span readingsSpan
	read func() []readings.Reading
	held []readings.Reading
}

// heldSpan is what a read of the readings history over a span read.
type heldSpan struct {
	span readingsSpan
	held []readings.Reading
}

// readingsSpan is the span of the readings history a tail reads: from start
// up to end, each window's last from from up to start carried in.
type readingsSpan struct {
	from, start, end time.Time
}

// is reports whether s and o are the one span, instant for instant.
func (s readingsSpan) is(o readingsSpan) bool {
	return s.from.Equal(o.from) && s.start.Equal(o.start) && s.end.Equal(o.end)
}

// runwayReadMsg is what a read of the readings history over the span read
// found; where it asked, the start of the first day the history holds, zero
// where it holds none, or failed to say; and where the ledger began, where
// it asked that.
type runwayReadMsg struct {
	span      readingsSpan
	held      []readings.Reading
	keptAsked bool
	kept      time.Time
	beginning
}

// carriedFor is how far before a span a tail carries each window's last
// reading in from, in days: a week, as a summary carries in the week before
// its day.
const carriedFor = 7

// liveSpan is the span of the readings history the live tail reads at now,
// weeks starting on the day given: from the start of the day before now's,
// of this week, or of the week's live view, whichever is earliest, so the
// live views, today and this week are each read of it, to a week on from
// now's day, each window's last from a week before carried in. The week's
// live view starts before the day before now's in the hour after the clocks
// go forward. It moves as the day turns, and the tail is read afresh then,
// but only what the history gains at each look between.
func liveSpan(now time.Time, starts time.Weekday) readingsSpan {
	start := slices.MinFunc([]time.Time{dayfile.DayStart(now, -1), views.WeekStart(now, starts), views.LiveTimeline(views.RunwayWeek, now, 0).Start}, time.Time.Compare)
	return readingsSpan{from: dayfile.DayStart(start, -carriedFor), start: start, end: dayfile.DayStart(now, 7)}
}

// spanOf is the span of the readings history the lanes over the timeline tl
// are judged by, the live tail's span being live: live, where tl starts in
// it, as the live views, today and this week do; else tl's own, of a day or
// a week gone, each window's last from a week before it carried in.
func spanOf(tl views.Timeline, live readingsSpan) readingsSpan {
	if !tl.Start.Before(live.start) {
		return live
	}
	return readingsSpan{from: dayfile.DayStart(tl.Start, -carriedFor), start: tl.Start, end: tl.End}
}

// holds reports whether the tab holds the readings of the span s, as read
// once of a span stepped back to before the live one, and the readings it
// holds of it.
func (t runwayTab) holds(s readingsSpan) ([]readings.Reading, bool) {
	i := slices.IndexFunc(t.back, func(h heldSpan) bool { return h.span.is(s) })
	if i < 0 {
		return nil, false
	}
	return t.back[i].held, true
}

// readingsOf are the readings the lanes over the timeline of the span s are
// judged by, the live span being live: the live tail's, or those read of the
// span stepped back to, none till they're read; and the readings the watch
// read of the documents it read itself, as views.RunwayReadings joins them.
func (t runwayTab) readingsOf(s, live readingsSpan) []readings.Reading {
	filed := t.live.held
	if !s.is(live) {
		filed, _ = t.holds(s)
	}
	return views.RunwayReadings(filed, t.seen.readings())
}

// looking is how the tab builds Runway's lanes at a look: its look, where
// it's given, as a test counts the builds through it; else as
// views.Lanes.Look builds them.
func (t runwayTab) looking() func(views.Lanes, views.LanesFrom) views.Lanes {
	if t.look == nil {
		return views.Lanes.Look
	}
	return t.look
}

// seenReadings are the readings the watch read of the documents it read
// itself, which the readings history holds none of: each account's windows,
// as each look read them, in the order the looks joined them; and when each
// account was last read so, by its id.
type seenReadings struct {
	looks []seenLook
	at    map[string]time.Time
}

// seenLook is an account's windows as a look read them, at at.
type seenLook struct {
	at   time.Time
	read []readings.Reading
}

// saw is s, the readings of the document look is a moment of joined, where
// the watch read it itself, as it does while the router doesn't answer, or
// it probes: of each account read since it was last joined, its windows as
// read, as ReadingsOf has them. The router's own it files in the history.
func (s seenReadings) saw(look status.Moment) seenReadings {
	if look.Routed {
		return s
	}
	at := maps.Clone(s.at)
	if at == nil {
		at = make(map[string]time.Time)
	}
	looks := slices.Clip(s.looks)
	for _, a := range look.Accounts {
		if a.ReadAt.After(at[a.ID]) {
			looks = append(looks, seenLook{at: a.ReadAt, read: look.ReadingsOf(a.ID)})
			at[a.ID] = a.ReadAt
		}
	}
	return seenReadings{looks: looks, at: at}
}

// since is s with those read before t let go.
func (s seenReadings) since(t time.Time) seenReadings {
	s.looks = slices.DeleteFunc(slices.Clone(s.looks), func(l seenLook) bool { return l.at.Before(t) })
	return s
}

// readings are every reading seen, in the order the looks joined them.
func (s seenReadings) readings() []readings.Reading {
	var read []readings.Reading
	for _, l := range s.looks {
		read = append(read, l.read...)
	}
	return read
}

// runwayTimeline is the timeline Runway's lanes are drawn over at now, as
// Runway shown has it, on the terminal drawn at.
func (m Model) runwayTimeline(now time.Time) views.Timeline {
	return m.runwayTab.shown.TimelineAt(now, m.cfg.WeekStarts, m.size.Width)
}

// runwayLooked is m with Runway built at the look msg takes, as runwayBuilt
// builds it, where nothing built it at its moment already, as a read landing
// a document does for the numbers its look compares. A frame takes nothing
// in, so builds nothing.
func (m Model) runwayLooked(msg tea.Msg) Model {
	if _, frame := msg.(frameMsg); frame {
		return m
	}
	return m.runwayBuilt()
}

// runwaySaw is m once the document look is a moment of lands: its accounts'
// readings joined, as saw joins them; and Runway built at it, as runwayBuilt
// builds it, so the numbers the look compares are drawn of the lanes of the
// document each is read of.
func (m Model) runwaySaw(look status.Moment) Model {
	m.runwayTab.seen = m.runwayTab.seen.saw(look)
	return m.runwayBuilt()
}

// runwayBuilt is m with Runway built afresh at the moment of the message
// taken in, while it shows with no session's page over it, once a moment:
// its timeline; its lanes, as views.Lanes builds them from the last look's,
// of the readings and the ledger's days they're judged by, as readingsOf and
// runwayDays have them; and COMING UP, as views.RunwayComing has it.
func (m Model) runwayBuilt() Model {
	t := m.runwayTab
	if m.view != dashboard.Runway || m.session != nil || t.builtAt != nil && t.builtAt == m.taking.moment {
		return m
	}
	now := m.now()
	mo, tl, live := m.moment(now), m.runwayTimeline(now), liveSpan(now, m.cfg.WeekStarts)
	t.lanes = t.looking()(t.lanes, views.LanesFrom{
		Moment: mo, Readings: t.readingsOf(spanOf(tl, live), live), Days: m.runwayDays(tl, now), Kept: t.kept, Timeline: tl, Window: t.shown.Window,
	})
	lanes := t.lanes
	t.shown.Timeline, t.shown.Lanes, t.shown.Coming = tl, &lanes, views.RunwayComing(mo, t.shown.Window)
	t.builtAt = m.taking.moment
	m.runwayTab = t
	return m
}

// runwayWants are the days of the ledger the lanes over the timeline tl are
// judged by, from the first to the last, as civilOf gives them: of a day or
// a week stepped back to, today and this week among them, whose use under
// each name is its highest, and whose resets its summaries give; and of a
// timeline whose readings reach back before the first day the readings
// history holds, a day before which is drawn from its summary. Each takes
// the week before it, as its readings carry each window's last in. It
// reports false where the lanes want none.
func (m Model) runwayWants(tl views.Timeline) (first, last time.Time, ok bool) {
	from := dayfile.DayStart(tl.Start, -carriedFor)
	_, _, named := tl.Named()
	if kept := m.runwayTab.kept; !named && (kept.IsZero() || !from.Before(kept)) {
		return time.Time{}, time.Time{}, false
	}
	return civilOf(from), civilOf(tl.End.Add(-time.Nanosecond)), true
}

// civilOf is the day t falls on in its own time zone, as dayfile.Civil gives
// a day, as a timeline names its days.
func civilOf(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// runwayYear is the year of the ledger's days History reads that reach back
// to the day given, as dayfile.Civil gives one, at now, weeks starting on the
// day given: 0, the last 12 months, where they do, else the calendar year
// it falls in.
func runwayYear(first, now time.Time, starts time.Weekday) int {
	if first.Before(civilOf(views.YearFrom(views.YearAsk{}, now, starts))) {
		return first.Year()
	}
	return 0
}

// runwayYearWanted is the year of the ledger's days Runway's lanes want, as
// runwayYear has it, reporting false where they want none, as runwayWants
// says, or Runway doesn't show: History's reads read it, and Runway shares
// what they hold, as Accounts does.
func (m Model) runwayYearWanted() (int, bool) {
	if m.view != dashboard.Runway || m.session != nil {
		return 0, false
	}
	now := m.now()
	first, _, ok := m.runwayWants(m.runwayTimeline(now))
	return runwayYear(first, now, m.cfg.WeekStarts), ok
}

// runwayDays are the summaries of the days the lanes over the timeline tl at
// now want, as runwayWants has them, as History's reads hold them: none
// where they want none, or till those reads hold them.
func (m Model) runwayDays(tl views.Timeline, now time.Time) []ledger.Summary {
	first, last, ok := m.runwayWants(tl)
	if !ok {
		return nil
	}
	past, held := m.past.reaching(runwayYear(first, now, m.cfg.WeekStarts))
	if !held {
		return nil
	}
	return past.Summaries(first, last)
}

// readRunway has the watch read the readings history for Runway's lanes
// while it shows, as a command, once no read of it is under way, as
// readingsRead reads it: a span stepped back to before the live one, of a
// day or a week gone, once, its readings held for the run; else the live
// tail, made afresh as its span moves, as the day turns, and read at once,
// then at each look since, as due says, for what it gained. Without a
// reader, nothing.
func (m Model) readRunway() (Model, tea.Cmd) {
	t := m.runwayTab
	if m.view != dashboard.Runway || m.session != nil || m.cfg.Readings == nil || t.reading {
		return m, nil
	}
	now := m.now()
	live := liveSpan(now, m.cfg.WeekStarts)
	read := historyRead{span: spanOf(m.runwayTimeline(now), live)}
	switch _, held := t.holds(read.span); {
	case !read.span.is(live) && held:
		return m, nil
	case !read.span.is(live):
		read.tail = m.cfg.Readings.Tail(read.span.from, read.span.start, read.span.end)
	case !live.is(t.live.span) || t.live.read == nil:
		t.live = readingsTail{span: live, read: m.cfg.Readings.Tail(live.from, live.start, live.end), held: t.live.held}
		t.seen = t.seen.since(live.from)
		read.tail, read.kept = t.live.read, m.cfg.Readings
	case t.due:
		read.tail, t.due = t.live.read, false
	default:
		return m, nil
	}
	read.began = m.past.beganAsked(m.cfg.Ledger)
	t.reading = true
	m.runwayTab = t
	return m, read.cmd(now)
}

// historyRead is a read of the readings history for Runway: of the span
// given, by its tail; where kept is given, the first day it holds, asked of
// it; and where began is given, where the ledger began, asked of it.
type historyRead struct {
	span  readingsSpan
	tail  func() []readings.Reading
	kept  views.Readings
	began views.LedgerStart
}

// cmd is the read, at now, as a command.
func (r historyRead) cmd(now time.Time) tea.Cmd {
	return func() tea.Msg {
		msg := runwayReadMsg{span: r.span, held: r.tail()}
		if r.kept != nil {
			msg.keptAsked, msg.kept = true, historyKept(r.kept)
		}
		if r.began != nil {
			msg.beginning = ledgerBegan(r.began, now)
		}
		return msg
	}
}

// historyKept is the start of the first day the readings history given
// holds: zero where it holds none, or fails to say, which is logged.
func historyKept(history views.Readings) time.Time {
	first, err := history.FirstDay()
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("couldn't read where the readings history begins", "error", err)
	}
	return first
}

// runwayRead takes in what a read of the readings history found: the live
// tail's, held in place of what it read last; or a span's stepped back to,
// held for the run; the first day the history holds, where the read asked;
// and where the ledger began, where it asked that, as History learns it.
func (m Model) runwayRead(msg runwayReadMsg) Model {
	t := m.runwayTab
	t.reading = false
	if msg.span.is(t.live.span) {
		t.live.held = msg.held
	} else {
		t.back = append(slices.Clip(t.back), heldSpan{span: msg.span, held: msg.held})
	}
	if msg.keptAsked {
		t.kept = msg.kept
	}
	m.runwayTab = t
	m.past = m.past.learned(msg.beginning)
	return m
}

// The ways Runway's keys move: < and > between the windows, ← and → back
// through the days or weeks and on.
var (
	runwayWindowKeys = map[string]int{"<": -1, ">": 1}
	runwayStepKeys   = map[string]int{"left": -1, "right": 1}
)

// runwayKey acts on a key of Runway's, reporting false for a key it doesn't
// take, or on another tab: < and > show the live view of the other window,
// as Windowed has it; ← and → step back and on, as Stepped has it, ← no
// further than runwayReaches says. A key that changes what's shown shows it
// as showRunway has it.
func (m Model) runwayKey(key string) (Model, bool) {
	if m.view != dashboard.Runway {
		return m, false
	}
	s, now := m.runwayTab.shown, m.now()
	var next dashboard.RunwayShown
	var changed bool
	if by, ok := runwayWindowKeys[key]; ok {
		next, changed = s.Windowed(by)
	} else if by, ok := runwayStepKeys[key]; ok {
		next, changed = s.Stepped(by, now, m.cfg.WeekStarts)
		changed = changed && m.runwayReaches(next, now)
	} else {
		return m, false
	}
	if changed {
		m = m.showRunway(next)
	}
	return m, true
}

// runwayReaches reports whether ← reaches Runway shown as r at now: live,
// today and this week always, as the readings history's live tail draws
// them; a day or a week gone where the ledger holds a summary of a day of
// it, back to the first it holds, as it tells where it began, none while
// that isn't known.
func (m Model) runwayReaches(r dashboard.RunwayShown, now time.Time) bool {
	tl := r.TimelineAt(now, m.cfg.WeekStarts, 0)
	_, last, named := tl.Named()
	began := m.past.began
	return !named || !tl.Now.IsZero() || !began.IsZero() && !last.Before(civilOf(began))
}

// showRunway is m showing Runway as r has it, keeping its window where that
// changed, for the next watch to open with: never a step back.
func (m Model) showRunway(r dashboard.RunwayShown) Model {
	was := m.runwayTab.shown
	m.runwayTab.shown = r
	if kept := r.Kept(); kept != was.Kept() {
		m.keepPrefs("Runway's window", func(prefs *theme.Prefs) { prefs.Runway = kept }, "window", kept.Window)
	}
	return m
}
