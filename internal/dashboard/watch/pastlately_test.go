package watch

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// pastLatelyRun is when the router that filed pastLatelyFiles' events
// started.
var pastLatelyRun = start.Add(-20 * day)

// pastLatelyFiles are the router's files of events, read on the harness's
// clock: work at its cap on each of the four days before the clock starts,
// at noon, and an hour ago.
func pastLatelyFiles(clock *fakeClock) *countedFiles {
	var lines []events.Line
	for i, d := range []time.Duration{-4*day - time.Hour, -3*day - time.Hour, -2*day - time.Hour, -day - time.Hour, -time.Hour} {
		e := status.Event{ID: i + 1, At: start.Add(d), Kind: status.EventCap, Account: "work"}
		lines = append(lines, events.Line{Event: e, Run: pastLatelyRun})
	}
	files := filesOf(lines...)
	files.clock = clock
	return files
}

// pastLatelyHarness is a model of the router's document, its files of events
// those given, its dial on the day back from today the count of < given.
func pastLatelyHarness(t *testing.T, filed views.Events, back int) *harness {
	t.Helper()
	h := newHarness(t, routerDocument(three()...))
	h.model.cfg.Events = filed
	h.model = steppedBack(t, h.model, back)
	return h
}

// steppedBack is m with its dial on today, stepped back by < the count
// given, as the dial steps it.
func steppedBack(t *testing.T, m Model, back int) Model {
	t.Helper()
	m.period = views.LengthToday.Now()
	for range back {
		stepped, ok := m.period.Back(m.now(), m.cfg.WeekStarts, time.Time{})
		if !ok {
			t.Fatalf("today stepped back %d times doesn't step", back)
		}
		m.period = stepped
	}
	return m
}

// askedLately is m asked for its past LATELY, as askPastLately has it, and
// what its read sends back, nil where it reads nothing.
func askedLately(t *testing.T, m Model) (Model, tea.Msg) {
	t.Helper()
	m, cmd := m.askPastLately()
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

// landed is m with what a past LATELY's read sent back taken in, as
// pastLatelyRead has it, and what the read it asks again sends back, nil
// where it asks none.
func landed(t *testing.T, m Model, msg tea.Msg) (Model, tea.Msg) {
	t.Helper()
	read, ok := msg.(pastLatelyReadMsg)
	if !ok {
		t.Fatalf("a past LATELY's read sent back %T", msg)
	}
	m, cmd := m.pastLatelyRead(read)
	if cmd == nil {
		return m, nil
	}
	return m, cmd()
}

// heldIDs are the ids of the events m holds for its past LATELY, oldest
// first, and whether it holds any for the dial's last day.
func heldIDs(m Model) ([]int, bool) {
	held, _, ok := m.pastLatelyHeld()
	var ids []int
	for _, e := range held {
		ids = append(ids, e.Key.ID)
	}
	return ids, ok
}

// A past period's LATELY is read once for each last day: the events of the
// week to the end of it, none after; asked again for the same day, it reads
// nothing; stepped back a day more, it reads that day's.
func TestPastLATELYIsReadOnceForEachLastDay(t *testing.T) {
	h := pastLatelyHarness(t, nil, 1)
	files := pastLatelyFiles(h.clock)
	h.model.cfg.Events = files
	if _, _, ok := h.model.pastLatelyHeld(); ok {
		t.Fatal("before it's asked, LATELY holds yesterday's events")
	}
	m, msg := askedLately(t, h.model)
	if msg == nil {
		t.Fatal("asked for yesterday, LATELY reads nothing")
	}
	h.model = m
	h.update(msg)
	m = h.model
	if ids, ok := heldIDs(m); !ok || !slices.Equal(ids, []int{1, 2, 3, 4}) {
		t.Errorf("yesterday's LATELY holds %v, held %v; want the events to yesterday's end, 1 to 4", ids, ok)
	}
	read := files.datesRead()
	if m, msg = askedLately(t, m); msg != nil || !slices.Equal(files.datesRead(), read) {
		t.Errorf("asked again for yesterday, LATELY read %v, the files read %v; want nothing read again", msg, files.datesRead())
	}
	m = steppedBack(t, m, 2)
	if _, msg = askedLately(t, m); msg == nil {
		t.Error("stepped back a day more, LATELY reads nothing; want that day's read")
	}
}

// A read the dial has stepped past by the time it lands is dropped, one
// read under way at a time, what's held kept as it was: where it was asked
// for another day meanwhile, that day's is read as it lands, unless what's
// held is of it already.
func TestAPastLATELYsReadTheDialSteppedPastIsDropped(t *testing.T) {
	tests := []struct {
		name string
		// first is the days back held as the read is asked, 0 for none;
		// asked the days back it reads; and steps the days back the dial
		// steps to, each asked for, as it's under way.
		first, asked int
		steps        []int
		// again is the days back read as it lands, 0 for none; held those
		// whose events are held then, their ids those given.
		again, held int
		ids         []int
	}{
		{name: "stepped back a day more", asked: 1, steps: []int{2}, again: 2, held: 2, ids: []int{1, 2, 3}},
		{name: "stepped on to now", asked: 1, steps: []int{0}},
		{name: "stepped back then on again", asked: 1, steps: []int{2, 1}, held: 1, ids: []int{1, 2, 3, 4}},
		{name: "stepped on to the day held", first: 1, asked: 2, steps: []int{1}, held: 1, ids: []int{1, 2, 3, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := pastLatelyHarness(t, nil, 1)
			h.model.cfg.Events = pastLatelyFiles(h.clock)
			m := h.model
			if tt.first != 0 {
				var held tea.Msg
				m, held = askedLately(t, steppedBack(t, m, tt.first))
				m, _ = landed(t, m, held)
			}
			m, read := askedLately(t, steppedBack(t, m, tt.asked))
			for _, back := range tt.steps {
				var msg tea.Msg
				if m, msg = askedLately(t, steppedBack(t, m, back)); msg != nil {
					t.Fatalf("stepped to %d back as a read is under way, LATELY read again at once", back)
				}
			}
			m, again := landed(t, m, read)
			if got := daysBackOf(t, m, again); got != tt.again {
				t.Fatalf("the read landed, LATELY read %d back again; want %d back", got, tt.again)
			}
			if again != nil {
				m, _ = landed(t, m, again)
			}
			ids, ok := heldIDs(m)
			if ok != (tt.held != 0) || !slices.Equal(ids, tt.ids) {
				t.Errorf("LATELY holds %v, held %v; want %v of %d back", ids, ok, tt.ids, tt.held)
			}
		})
	}
}

// daysBackOf is how many days back from today the day a past LATELY's
// read is of lies, as m steps them, 0 where there's no read.
func daysBackOf(t *testing.T, m Model, msg tea.Msg) int {
	t.Helper()
	read, ok := msg.(pastLatelyReadMsg)
	if !ok {
		return 0
	}
	for back := 1; back <= 7; back++ {
		if until, _ := steppedBack(t, m, back).period.Until(m.now(), m.cfg.WeekStarts); until.Equal(read.until) {
			return back
		}
	}
	t.Fatalf("a read to %v is of none of the week's days", read.until)
	return 0
}

// Without the router's files to read, or files that read nothing, a past
// period's LATELY holds no events, and tells none, as LATELY at now tells
// none where the document gives none.
func TestAPastLATELYThatCantBeReadTellsNone(t *testing.T) {
	tests := []struct {
		name  string
		filed views.Events
		reads bool
	}{
		{name: "no files"},
		{name: "files that read nothing", filed: events.Empty{}, reads: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, msg := askedLately(t, pastLatelyHarness(t, tt.filed, 1).model)
			if (msg != nil) != tt.reads {
				t.Fatalf("asked, LATELY read %v; want a read %v", msg, tt.reads)
			}
			if msg != nil {
				m, _ = landed(t, m, msg)
			}
			held, until, ok := m.pastLatelyHeld()
			if !ok || len(held) != 0 {
				t.Fatalf("LATELY holds %d events, held %v; want none, held", len(held), ok)
			}
			mo := m.moment(m.now())
			said := views.AccountLatelyTo(mo, held, "work", until, math.MaxInt, views.SessionDirs{})
			if atNow := views.AccountLately(mo, "work", math.MaxInt, views.SessionDirs{}); len(said) != 0 || len(atNow) != 0 {
				t.Errorf("LATELY tells %q, and at now %q; want none, alike", fullSaid(said), fullSaid(atNow))
			}
		})
	}
}

// fullSaid are the sentences given, each in full, on one line.
func fullSaid(told []views.Sentence) string {
	var said []string
	for _, s := range told {
		said = append(said, s.Full.String())
	}
	return strings.Join(said, "; ")
}
