package watch

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayTab is what the watch holds of Runway: what it shows, its timeline,
// lanes and COMING UP as the last look built them; the lanes themselves,
// which hold what they judged of the past till what that's judged from
// changes; the readings history's tail the past is judged by, as read
// last; whether a look since asks what the tail gained; and whether a read
// of it is under way, one at a time.
type runwayTab struct {
	shown   dashboard.RunwayShown
	lanes   views.Lanes
	tail    readingsTail
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

// readingsSpan is the span of the readings history a tail reads: from start
// up to end, each window's last from from up to start carried in.
type readingsSpan struct {
	from, start, end time.Time
}

// is reports whether s and o are the one span, instant for instant.
func (s readingsSpan) is(o readingsSpan) bool {
	return s.from.Equal(o.from) && s.start.Equal(o.start) && s.end.Equal(o.end)
}

// runwayReadMsg is what a read of the readings history's tail over the span
// read found.
type runwayReadMsg struct {
	span readingsSpan
	held []readings.Reading
}

// carriedFor is how far before a span a tail carries each window's last
// reading in from: a week, as a summary carries in the week before its day.
const carriedFor = 7

// spanOf is the span of the readings history the lanes over the timeline tl
// at now are judged by: the live views', and today's and this week's where
// they fall within it, from the start of the day before now's, for eight
// days, so a tail is read once a day, as the day turns, then only what the
// history gains; else the timeline's own, of a day or a week gone.
func spanOf(tl views.Timeline, now time.Time) readingsSpan {
	start := dayfile.DayStart(now, -1)
	if tl.Start.Before(start) {
		start = tl.Start
		return readingsSpan{from: start.AddDate(0, 0, -carriedFor), start: start, end: tl.End}
	}
	return readingsSpan{from: start.AddDate(0, 0, -carriedFor), start: start, end: dayfile.DayStart(now, 7)}
}

// runwayTimeline is the timeline Runway's lanes are drawn over at now, as
// Runway shown has it, on the terminal drawn at.
func (m Model) runwayTimeline(now time.Time) views.Timeline {
	return m.runwayTab.shown.TimelineAt(now, m.cfg.WeekStarts, m.size.Width)
}

// runwayLooked is m with Runway built afresh at the look msg takes, while it
// shows with no session's page over it: its timeline; its lanes, as
// views.Lanes builds them from the last look's, of the readings the tail
// read; and COMING UP, as views.RunwayComing has it. A frame takes nothing
// in, so builds nothing.
func (m Model) runwayLooked(msg tea.Msg) Model {
	if _, frame := msg.(frameMsg); frame || m.view != dashboard.Runway || m.session != nil {
		return m
	}
	now := m.now()
	mo, t := m.moment(now), m.runwayTab
	tl := m.runwayTimeline(now)
	t.lanes = t.lanes.Look(views.LanesFrom{Moment: mo, Readings: t.tail.held, Timeline: tl, Window: t.shown.Window})
	lanes := t.lanes
	t.shown.Timeline, t.shown.Lanes, t.shown.Coming = tl, &lanes, views.RunwayComing(mo, t.shown.Window)
	m.runwayTab = t
	return m
}

// readRunway has the watch read the readings history's tail for Runway's
// lanes while it shows, as a command, once no read of it is under way: at
// a look since the last read, as due says, what the tail gained; and over a
// span its timeline wants afresh, as spanOf has it, as a step back or the
// day turning moves it, a tail of that span made and read at once. Without
// a reader, nothing.
func (m Model) readRunway() (Model, tea.Cmd) {
	t := m.runwayTab
	if m.view != dashboard.Runway || m.session != nil || m.cfg.Readings == nil || t.reading {
		return m, nil
	}
	now := m.now()
	if span := spanOf(m.runwayTimeline(now), now); !span.is(t.tail.span) || t.tail.read == nil {
		t.tail = readingsTail{span: span, read: m.cfg.Readings.Tail(span.from, span.start, span.end)}
	} else if !t.due {
		return m, nil
	}
	tail := t.tail
	t.due, t.reading = false, true
	m.runwayTab = t
	return m, func() tea.Msg {
		return runwayReadMsg{span: tail.span, held: tail.read()}
	}
}

// runwayRead takes in what a read of the readings history's tail found, held
// where it's of the span of the tail held.
func (m Model) runwayRead(msg runwayReadMsg) Model {
	m.runwayTab.reading = false
	if msg.span.is(m.runwayTab.tail.span) {
		m.runwayTab.tail.held = msg.held
	}
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
// as Windowed has it; ← and → step back and on, as Stepped has it. A key
// that changes what's shown shows it as showRunway has it.
func (m Model) runwayKey(key string) (Model, bool) {
	if m.view != dashboard.Runway {
		return m, false
	}
	s := m.runwayTab.shown
	var next dashboard.RunwayShown
	var changed bool
	if by, ok := runwayWindowKeys[key]; ok {
		next, changed = s.Windowed(by)
	} else if by, ok := runwayStepKeys[key]; ok {
		next, changed = s.Stepped(by, m.now(), m.cfg.WeekStarts)
	} else {
		return m, false
	}
	if changed {
		m = m.showRunway(next)
	}
	return m, true
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
