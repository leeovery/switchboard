package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// latelyDrawn is LATELY of doc at now drawn in body, on a canvas of the
// frame's size.
func latelyDrawn(f Frame, doc status.Document, now time.Time, body bounds) *canvas {
	c := newCanvas(f.Width, f.Height)
	f.latelySection(doc, now).draw(c, body)
	return c
}

// eventsOf is threeRouted's document with the events given, newest first,
// numbered from the oldest.
func eventsOf(events ...status.Event) status.Document {
	doc := threeRouted()
	for i := range events {
		events[i].ID = len(events) - i
	}
	doc.Events = events
	return doc
}

func TestLATELYsMarksAreOneAKindInTheirColours(t *testing.T) {
	tests := []struct {
		kind  string
		well  bool
		glyph string
		token theme.Token
	}{
		{kind: status.EventStarted, glyph: "▲", token: theme.AccentMode},
		{kind: status.EventMoved, glyph: "▸", token: theme.AccentKey},
		{kind: status.EventPressure, glyph: "◔", token: theme.AccentAttention},
		{kind: status.EventCap, glyph: "╎", token: theme.AccentAttention},
		{kind: status.EventLimit, glyph: "■", token: theme.StateDestructive},
		{kind: status.EventRefused, glyph: "■", token: theme.StateDestructive},
		{kind: status.EventPrimed, glyph: "◇", token: theme.TextSubtle},
		{kind: status.EventRoom, glyph: "✓", token: theme.StatePositive},
		{kind: status.EventPin, glyph: "●", token: theme.AccentKey},
		{kind: status.EventAuto, glyph: "↩", token: theme.AccentKey},
		{kind: status.EventRestart, glyph: "!", token: theme.AccentAttention},
		{kind: status.EventHealth, glyph: "⊕", token: theme.StateDestructive},
		{kind: status.EventHealth, well: true, glyph: "⊕", token: theme.StatePositive},
	}
	for _, tt := range tests {
		got := eventMark(views.Sentence{Kind: tt.kind, Well: tt.well})
		if got.text != tt.glyph || got.ink != (ink{token: tt.token}) {
			t.Errorf("%s (well %v) is marked %q in %+v, want %q in %v", tt.kind, tt.well, got.text, got.ink, tt.glyph, tt.token)
		}
	}
}

func TestLATELYsLinesAreInTheirInks(t *testing.T) {
	doc := eventsOf(
		status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: status.ReasonNew},
		status.Event{At: now.Add(-10 * time.Minute).UTC(), Kind: status.EventCap, Account: "work", Count: 3, To: "side"},
	)
	for i, label := range []string{"Work", "Personal", "Side"} {
		doc.Accounts[i].Label = label
	}
	lines := frameOf(160, 40).latelyLines(doc, views.Lately(doc, now, 2), now)
	want := []line{
		{{"13:11  ", secondaryInk}, {"▲", accentInk}, spaces(1), {"c61b", titleInk}, {" started on ", mutedInk}, {"Side", strongInk}},
		{{"13:02  ", dimInk}, {"╎", warningInk}, spaces(1), {"Work", titleInk}, {" reached its cap: ", mutedInk}, {"3 sessions", strongInk}, {" moved to ", mutedInk}, {"Side", mutedInk}},
	}
	for i, w := range want {
		if !slices.Equal(lines[i], w) {
			t.Errorf("line %d is %+v, want %+v: the newest's time in text.secondary, the rest's in text.subtle; the subject bold text.primary, the object bold text.secondary, the words text.muted, accounts by their names", i+1, lines[i], w)
		}
	}
}

func TestLATELYsTimesAreDatedInAColumn(t *testing.T) {
	doc := eventsOf(
		status.Event{At: now.Add(-time.Hour).UTC(), Kind: status.EventRoom, Account: "work"},
		status.Event{At: now.Add(-20 * time.Hour).UTC(), Kind: status.EventRoom, Account: "side"},
	)
	c := latelyDrawn(frameOf(160, 10), doc, now, bounds{width: 78, rows: 3})
	for i, want := range []string{"12:12      ✓ work open again", "Sun 17:12  ✓ side open again"} {
		if got := strings.TrimRight(c.row(i, Look{}), " "); got != want {
			t.Errorf("row %d = %q, want %q: yesterday's with its weekday, today's padded to it", i+1, got, want)
		}
	}
}

func TestLATELYsWordsAreShortOnAPhone(t *testing.T) {
	doc := eventsOf(status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventCap, Account: "work", Count: 3, To: "side"})
	tests := []struct {
		width int
		want  string
	}{
		{width: 160, want: "13:11  ╎ work reached its cap: 3 sessions moved to side"},
		{width: 52, want: "13:11  ╎ work at its cap: 3 moved to side"},
	}
	for _, tt := range tests {
		c := latelyDrawn(frameOf(tt.width, 10), doc, now, bounds{width: tt.width, rows: 3})
		if got := strings.TrimRight(c.row(0, Look{}), " "); got != tt.want {
			t.Errorf("at %d columns, LATELY's line is %q, want %q", tt.width, got, tt.want)
		}
	}
}

func TestLATELYDrawsAsManyAsItsBodyHoldsFromItsLeftCutAtItsRight(t *testing.T) {
	doc := eventsOf(
		status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: "pin yields: personal has no room"},
		status.Event{At: now.Add(-2 * time.Minute).UTC(), Kind: status.EventRoom, Account: "work"},
		status.Event{At: now.Add(-3 * time.Minute).UTC(), Kind: status.EventRoom, Account: "side"},
	)
	c := latelyDrawn(frameOf(160, 10), doc, now, bounds{x: 82, y: 1, width: 40, rows: 2})
	from := spaces(82).text
	want := []string{"", from + "13:11  ▲ c61b started on side: its pin…", from + "13:10  ✓ work open again", ""}
	for i, w := range want {
		if got := strings.TrimRight(c.row(i, Look{}), " "); got != w {
			t.Errorf("row %d = %q, want %q", i+1, got, w)
		}
	}
}

func TestLATELYPicksOutAFreshRowAcrossItsBodyFadingBack(t *testing.T) {
	doc := eventsOf(
		status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventRoom, Account: "work"},
		status.Event{At: now.Add(-2 * time.Minute).UTC(), Kind: status.EventRoom, Account: "side"},
	)
	f := frameOf(160, 10)
	f.Fresh = map[int]float64{2: 0.25}
	body := bounds{x: 82, y: 1, width: 78, rows: 3}
	c := latelyDrawn(f, doc, now, body)

	want := hue{token: theme.BgAttention, fade: 0.25}
	for x := range f.Width {
		picked := body.x <= x && x < body.x+body.width
		if got := c.cells[1][x].ink.on; (got == want) != picked {
			t.Errorf("the fresh row's cell %d is on %+v, want %v on bg.attention", x, got, picked)
		}
		if got := c.cells[2][x].ink.on; got != (hue{}) {
			t.Errorf("the row seen before's cell %d is on %+v, want the canvas", x, got)
		}
	}
}

func TestLATELYSaysWhyItHasNone(t *testing.T) {
	tests := []struct {
		name     string
		outdated bool
		want     string
	}{
		{name: "a router that has told of none", want: "nothing lately"},
		{name: "a router from before it told of events", outdated: true, want: "restart the router for recent events"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := frameOf(160, 10)
			f.Outdated = tt.outdated
			c := latelyDrawn(f, threeRouted(), now, bounds{x: 82, y: 1, width: 78, rows: 3})
			if got := strings.TrimSpace(c.row(1, Look{})); got != tt.want {
				t.Errorf("LATELY says %q, want %q", got, tt.want)
			}
			if got := f.latelySection(threeRouted(), now).folded().plain(); got != tt.want {
				t.Errorf("folded, LATELY says %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLATELYFoldsToItsFirstLine(t *testing.T) {
	doc := eventsOf(
		status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventStarted, Account: "side", Session: "c61b2f8d", Reason: status.ReasonNew},
		status.Event{At: now.Add(-25 * time.Hour).UTC(), Kind: status.EventRoom, Account: "work"},
	)
	folded := frameOf(160, 40).latelySection(doc, now).folded()
	want := line{{"13:11  ", secondaryInk}, {"▲", accentInk}, spaces(1), {"c61b", titleInk}, {" started on ", mutedInk}, {"side", strongInk}, {"  …", dimInk}}
	if !slices.Equal(folded, want) {
		t.Errorf("folded, LATELY says %+v, want %+v: its first line as drawn, then …", folded, want)
	}
}

func TestLATELYTakesItsLinesAndTwoRows(t *testing.T) {
	rooms := func(n int) status.Document {
		var events []status.Event
		for i := range n {
			events = append(events, status.Event{At: now.Add(-time.Duration(i+1) * time.Minute).UTC(), Kind: status.EventRoom, Account: "work"})
		}
		return eventsOf(events...)
	}
	for _, tt := range []struct{ events, least, most int }{{0, 5, 5}, {2, 5, 5}, {7, 5, 9}, {20, 5, 14}} {
		s := frameOf(160, 40).latelySection(rooms(tt.events), now)
		if s.least != tt.least || s.most != tt.most {
			t.Errorf("with %d events, LATELY takes %d rows to %d, want %d to %d", tt.events, s.least, s.most, tt.least, tt.most)
		}
	}
}

// overviewAt is when the Overview's frames count down from: 14:42, where
// their clocks read 14:42:07.
var overviewAt = time.Date(2026, 10, 1, 14, 42, 0, 0, time.UTC)

// framedAccount is an account of the Overview's frames, labelled as its id,
// its 5-hour window and its week resetting as given.
func framedAccount(id string, session, week time.Time) status.Account {
	return status.Account{ID: id, Label: id, TokenSet: true, FetchedAt: overviewAt, Windows: []quota.Window{
		{Key: "5h", Label: "Session", ResetsAt: session}, {Key: "7d", Label: "Week", ResetsAt: week},
	}}
}

// framedDoc is the document the Overview's frames draw COMING UP and
// LATELY of: work, personal and side, coming up as given, and the events
// given.
func framedDoc(coming []status.Upcoming, events []status.Event) status.Document {
	return status.Document{
		GeneratedAt: overviewAt, Source: status.SourceRouter, Best: "side", Router: status.Health{Healthy: true},
		Accounts: []status.Account{
			framedAccount("work", routedClock(1, 17, 10), routedClock(4, 21, 0)),
			framedAccount("personal", routedClock(1, 16, 30), routedClock(4, 2, 0)),
			framedAccount("side", routedClock(1, 18, 50), routedClock(5, 10, 0)),
		},
		ComingUp: coming, Events: events,
	}
}

// framedComing is what's coming up in the frames drawn on auto, soonest
// first.
var framedComing = []status.Upcoming{
	{At: routedClock(1, 16, 30), Account: "personal", Kind: status.UpcomingReset, Window: "5h"},
	{At: routedClock(1, 17, 10), Account: "work", Kind: status.UpcomingBack, Cap: true},
	{At: routedClock(1, 18, 50), Account: "side", Kind: status.UpcomingReset, Window: "5h"},
	{At: routedClock(3, 6, 0), Account: "personal", Kind: status.UpcomingRunsOut, Window: "7d"},
	{At: routedClock(4, 2, 0), Account: "personal", Kind: status.UpcomingReset, Window: "7d"},
	{At: routedClock(4, 21, 0), Account: "work", Kind: status.UpcomingReset, Window: "7d"},
	{At: routedClock(5, 10, 0), Account: "side", Kind: status.UpcomingReset, Window: "7d"},
}

// framedEvents are the events given, newest first, numbered from the
// oldest, their moves counted in the line of the event that forced them,
// which is the event before them.
func framedEvents(events ...status.Event) []status.Event {
	line := 0
	for i := len(events) - 1; i >= 0; i-- {
		events[i].ID = len(events) - i
		switch {
		case events[i].Kind == status.EventMoved && events[i].Reason == reasonCapped:
			events[i].ForcedBy = line
		case events[i].Count > 0:
			line = events[i].ID
		}
	}
	return events
}

// reasonCapped is the router's reason for a move off work at its cap.
const reasonCapped = "moved: work is at its reserve"

// startedAt is a session starting on an account at the time given on 1
// October, for the reason given.
func startedAt(hour, minute int, session, account, reason string) status.Event {
	return status.Event{At: routedClock(1, hour, minute), Kind: status.EventStarted, Session: session, Account: account, Reason: reason}
}

// movedOff is a session moved off work at its cap at the time given on 1
// October.
func movedOff(hour, minute int, session string) status.Event {
	return status.Event{At: routedClock(1, hour, minute), Kind: status.EventMoved, Session: session, From: "work", To: "side", Reason: reasonCapped}
}

// primeAt is a prime starting an account's 5-hour window at the time given
// on 1 October.
func primeAt(hour, minute int, account string) status.Event {
	return status.Event{At: routedClock(1, hour, minute), Kind: status.EventPrimed, Account: account, Windows: []string{"5h"}}
}

// workCapped is work reaching its cap at 14:31, its three sessions moved to
// side.
func workCapped() []status.Event {
	return []status.Event{
		movedOff(14, 31, "db8a71e3"), movedOff(14, 31, "7f3a5c90"), movedOff(14, 31, "d28c0b1e"),
		{At: routedClock(1, 14, 31), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Count: 3, To: "side"},
	}
}

// onAuto are the events of the frames drawn on auto, newest first, as their
// twelve lines of LATELY tell them.
func onAuto() []status.Event {
	return framedEvents(slices.Concat(
		[]status.Event{startedAt(14, 41, "c61b2f8d", "side", status.ReasonNew)},
		workCapped(),
		[]status.Event{
			startedAt(14, 12, "7f3a5c90", "work", status.ReasonNew),
			primeAt(13, 50, "side"),
			{At: routedClock(1, 13, 22), Kind: status.EventMoved, Session: "db8a71e3", From: "personal", To: "work", Reason: status.PassingOver(status.Rescored("21h 15m"), "personal")},
			startedAt(12, 47, "41e0b6c2", "personal", status.ReasonNew),
			primeAt(12, 10, "work"),
			startedAt(11, 55, "9c2e47b1", "personal", status.ReasonNew),
			primeAt(11, 30, "personal"),
			startedAt(10, 2, "5b19f0a4", "personal", status.ReasonNew),
			startedAt(9, 40, "3d7a2c6e", "work", status.ReasonNew),
			startedAt(9, 12, "e0c4a917", "work", status.ReasonNew),
		},
	)...)
}

func TestCOMINGUPAndLATELYAreDrawnAsTheirFrames(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	badHour := []status.Upcoming{
		{At: routedClock(1, 15, 54), Account: "personal", Kind: status.UpcomingBack},
		framedComing[2], framedComing[3],
	}
	refused := status.Event{At: routedClock(1, 14, 20), Kind: status.EventRefused, Account: "work", Status: 401, Count: 2, To: "side"}
	limited := status.Event{At: routedClock(1, 14, 12), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 2, To: "side"}
	pinned := status.Event{At: routedClock(1, 14, 2), Kind: status.EventPin, Account: "personal", Accounts: []string{"personal"}, By: "cli"}
	type held struct {
		body bounds
		of   func(Frame, status.Document) section
	}
	coming := func(f Frame, doc status.Document) section { return f.comingSection(doc, overviewAt) }
	lately := func(f Frame, doc status.Document) section { return f.latelySection(doc, overviewAt) }
	tests := []struct {
		stem          string
		width, height int
		doc           status.Document
		held          []held
		// whole are the cells the frame draws otherwise than its world.
		whole []spot
	}{
		{stem: "overview-default-160", width: 160, height: 37, doc: framedDoc(framedComing, onAuto()),
			held: []held{{body: bounds{y: 32, width: 72, rows: 3}, of: coming}, {body: bounds{x: 82, y: 32, width: 78, rows: 3}, of: lately}}},
		{stem: "overview-pinned-160", width: 160, height: 37,
			doc:  framedDoc(framedComing, framedEvents(slices.Concat([]status.Event{startedAt(14, 41, "c61b2f8d", "personal", status.ReasonGlobalPin)}, workCapped(), []status.Event{pinned})...)),
			held: []held{{body: bounds{y: 32, width: 72, rows: 3}, of: coming}, {body: bounds{x: 82, y: 32, width: 78, rows: 3}, of: lately}}},
		// The frame's c61b started on side with a why no reason of the
		// router's reads as, ": personal is at its limit": under the pin of
		// personal and side, the router's reason is the pin's.
		{stem: "overview-bad-hour-160", width: 160, height: 37,
			doc:   framedDoc(badHour, framedEvents(startedAt(14, 41, "c61b2f8d", "side", status.ReasonGlobalPin), refused, limited)),
			held:  []held{{body: bounds{y: 32, width: 72, rows: 3}, of: coming}, {body: bounds{x: 82, y: 32, width: 78, rows: 3}, of: lately}},
			whole: []spot{{32, 111, 159}}},
		{stem: "overview-default-216", width: 216, height: 70, doc: framedDoc(framedComing, onAuto()),
			held: []held{{body: bounds{y: 54, width: 97, rows: 7}, of: coming}, {body: bounds{x: 110, y: 54, width: 106, rows: 12}, of: lately}}},
		{stem: "overview-phone-52", width: 52, height: 66, doc: framedDoc(framedComing, onAuto()),
			held: []held{{body: bounds{y: 55, width: 52, rows: 3}, of: coming}, {body: bounds{y: 61, width: 52, rows: 3}, of: lately}}},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: tt.height, Look: nord, View: Overview, Policy: claudeLike}
			c := newCanvas(f.Width, f.Height)
			for _, h := range tt.held {
				h.of(f, tt.doc).draw(c, h.body)
			}
			drawn := screenOf(t, c.rows(nord))
			for _, h := range tt.held {
				in := region{top: h.body.y, bottom: h.body.y + h.body.rows - 1, left: h.body.x, right: h.body.x + h.body.width - 1}
				heldTo(t, tt.stem, reference(t, tt.stem), drawn, in, shadeOf(nord.Canvas()), tt.whole, nil)
			}
		})
	}
}

func TestCOMINGUPAndLATELYFoldAsTheirFrames(t *testing.T) {
	nord := Screen(builtin(t, "nord"))
	f := Frame{Width: 160, Height: 39, Look: nord, View: Overview, Policy: claudeLike}
	doc := framedDoc(framedComing, onAuto())
	c := newCanvas(f.Width, f.Height)
	c.line(12, 36, f.comingSection(doc, overviewAt).folded())
	c.line(91, 36, f.latelySection(doc, overviewAt).folded())

	// The frame draws the folded LATELY's object, side, in text.muted, not
	// bold, where its rows, and the design, draw it bold text.secondary.
	whole := []spot{{36, 116, 119}}
	drawn, frame := screenOf(t, c.rows(nord)), reference(t, "overview-everything-160x39")
	heldTo(t, "overview-everything-160x39", frame, drawn, region{top: 36, bottom: 36, left: 12, right: 46}, shadeOf(nord.Canvas()), nil, nil)
	heldTo(t, "overview-everything-160x39", frame, drawn, region{top: 36, bottom: 36, left: 91, right: 123}, shadeOf(nord.Canvas()), whole, nil)
}
