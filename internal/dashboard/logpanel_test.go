package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard/flex"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// panelNow is the moment the panel's frames are drawn at: Thursday 1
// October, 14:42:07.
var panelNow = time.Date(2026, 10, 1, 14, 42, 7, 0, now.Location())

// framesSessions are today's sessions as the panel's frames list them, seen
// last first, with lines and events today: d28c asking; c61b started within
// the minute; 7f3a, db8a and 41e0 busy; 9c2e idle 9 minutes; then more,
// ended.
func framesSessions(more int) []views.LogSession {
	seen := func(ago time.Duration) views.Activity { return views.Activity{LastSeen: panelNow.Add(-ago)} }
	idle := panelNow.Add(-9 * time.Minute)
	sessions := []views.LogSession{
		{ID: "d28c0001", Dir: "/code/switchboard", Account: "side", Lines: 445, Events: 2, Activity: views.Activity{Doing: views.Asking, LastSeen: panelNow}},
		{ID: "c61b0001", Dir: "/code/mobile", Account: "side", Lines: 5, Events: 1, Activity: seen(5 * time.Second), Started: panelNow.Add(-56 * time.Second)},
		{ID: "7f3a0001", Dir: "/code/api", Account: "side", Lines: 84, Events: 2, Activity: seen(6 * time.Second)},
		{ID: "db8a0001", Dir: "/code/infra", Account: "side", Lines: 126, Events: 2, Activity: seen(20 * time.Second)},
		{ID: "41e00001", Dir: "/code/docs", Account: "personal", Lines: 158, Events: 1, Activity: seen(30 * time.Second)},
		{ID: "9c2e0001", Dir: "/code/website", Account: "personal", Lines: 267, Events: 1, Activity: views.Activity{Since: idle, LastSeen: idle}},
	}
	for i := range more {
		ended := panelNow.Add(-time.Duration(i+2) * time.Hour)
		sessions = append(sessions, views.LogSession{
			ID: fmt.Sprintf("e%03d0001", i), Dir: "/code/old", Account: "work", Lines: 1, Events: 1,
			Activity: views.Activity{Doing: views.Ended, Since: ended, LastSeen: ended},
		})
	}
	return sessions
}

// framesPanel is the panel of the table given as its frames draw it, with
// today's sessions and more of them: work, personal and side; today's 1,275
// lines, 1,240 of them requests, shown, 3 failed, of two models; and today's
// 20 events.
func framesPanel(of LogView, more int) LogPanel {
	return LogPanel{
		Of: of, Filter: views.LogOpening(), Now: panelNow,
		Accounts: []status.AccountNow{{ID: "work"}, {ID: "personal"}, {ID: "side"}},
		Lines: views.LogCount{
			Requests: 1240, Checks: 31, Counts: 4, Failed: 3, Shown: 1240,
			Models: map[string]int{"claude-opus-5-5": 1236, "claude-haiku-4-5": 39},
		},
		Events: views.EventCount{
			All: 20, Shown: 20,
			Kinds: map[string]int{status.EventStarted: 10, status.EventMoved: 4, status.EventCap: 1, status.EventPrimed: 5},
		},
		Sessions: framesSessions(more),
	}
}

// bodyText is the panel's body as the frame f shows it, each row its text,
// its blanks at its end trimmed.
func bodyText(p LogPanel, f Frame) []string {
	var rows []string
	for _, l := range p.card(f).body {
		rows = append(rows, strings.TrimRight(l.plain(), " "))
	}
	return rows
}

// litText is what the tick the cursor's on reads, as the panel lights it on
// the frame f.
func litText(p LogPanel, f Frame) string {
	k := p.card(f)
	row := []rune(k.body[k.lit.first].plain() + strings.Repeat(" ", k.lit.x+k.lit.cells))
	return strings.TrimSpace(string(row[max(k.lit.x-1, 0) : k.lit.x-1+k.lit.cells]))
}

// Space ticks the tick the cursor's on: a column shows or hides; of
// accounts, models and kinds, a choice ticked keeps those ticked, and every
// one is ticked where none is, so a choice ticking every one, or none, is
// none; failed keeps only those that failed; and of sessions, those ticked,
// none at first.
func TestSpaceTicksTheTickTheCursorsOn(t *testing.T) {
	tests := []struct {
		name  string
		of    LogView
		keys  func(LogPanel) LogPanel
		check func(LogPanel) any
		want  any
	}{
		{
			name: "a column shown", of: LogRequests, keys: func(p LogPanel) LogPanel { return p.Moved(11).Ticked() },
			check: func(p LogPanel) any { return p.Columns.Shows(LogWorth) }, want: true,
		},
		{
			name: "a column hidden", of: LogRequests, keys: func(p LogPanel) LogPanel { return p.Ticked() },
			check: func(p LogPanel) any { return p.Columns.Shows(LogSession) }, want: false,
		},
		{
			name: "an account of every one", of: LogRequests, keys: func(p LogPanel) LogPanel { return p.Sided(1).Ticked() },
			check: func(p LogPanel) any { return fmt.Sprint(p.Filter.Accounts) }, want: "[personal side]",
		},
		{
			name: "an account again: every one, as none", of: LogRequests,
			keys:  func(p LogPanel) LogPanel { return p.Sided(1).Ticked().Ticked() },
			check: func(p LogPanel) any { return p.Filter.Accounts == nil }, want: true,
		},
		{
			name: "a model", of: LogRequests, keys: func(p LogPanel) LogPanel { return p.Sided(1).Moved(4).Ticked() },
			check: func(p LogPanel) any { return fmt.Sprint(p.Filter.Models) }, want: "[claude-opus-5-5]",
		},
		{
			name: "checks shown", of: LogRequests, keys: func(p LogPanel) LogPanel { return p.Sided(1).Moved(6).Ticked() },
			check: func(p LogPanel) any { return fmt.Sprint(p.Filter.Kinds) }, want: fmt.Sprint([]views.RowKind{views.RowRequest, views.RowCheck}),
		},
		{
			name: "every kind: none", of: LogRequests, keys: func(p LogPanel) LogPanel { return p.Sided(1).Moved(6).Ticked().Moved(1).Ticked() },
			check: func(p LogPanel) any { return p.Filter.Kinds == nil }, want: true,
		},
		{
			name: "failed alone", of: LogRequests, keys: func(p LogPanel) LogPanel { return p.Sided(1).Moved(8).Ticked() },
			check: func(p LogPanel) any { return p.Filter.Failed }, want: true,
		},
		{
			name: "a session", of: LogRequests, keys: func(p LogPanel) LogPanel { return p.Sided(1).Moved(10).Ticked() },
			check: func(p LogPanel) any { return fmt.Sprint(p.Filter.Sessions) }, want: "[c61b0001]",
		},
		{
			name: "a session again: none", of: LogRequests, keys: func(p LogPanel) LogPanel { return p.Sided(1).Moved(10).Ticked().Ticked() },
			check: func(p LogPanel) any { return len(p.Filter.Sessions) }, want: 0,
		},
		{
			name: "a kind of event", of: LogEvents, keys: func(p LogPanel) LogPanel { return p.Moved(7).Ticked() },
			check: func(p LogPanel) any {
				return fmt.Sprint(len(p.Shows.Kinds), slices.Contains(p.Shows.Kinds, status.EventRoom))
			},
			want: "11 false",
		},
		{
			name: "an account's events", of: LogEvents, keys: func(p LogPanel) LogPanel { return p.Sided(1).Moved(2).Ticked() },
			check: func(p LogPanel) any { return fmt.Sprint(p.Shows.Accounts) }, want: "[work personal]",
		},
		{
			name: "a session's events", of: LogEvents, keys: func(p LogPanel) LogPanel { return p.Sided(1).Moved(3).Ticked() },
			check: func(p LogPanel) any { return fmt.Sprint(p.Shows.Sessions, p.Filter.Sessions) }, want: "[d28c0001] []",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.check(tt.keys(framesPanel(tt.of, 8))); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

// ↑ and ↓ move the cursor through the ticks of its side in their reading
// order, along a row then down, a row too long wrapping under its label,
// no further than the first or the last; ← and → move it between the
// sides, each keeping where it was.
func TestTheCursorMovesThroughTheTicksInReadingOrder(t *testing.T) {
	p := framesPanel(LogRequests, 0)
	for i := range 10 {
		p.Accounts = append(p.Accounts, status.AccountNow{ID: fmt.Sprintf("account-%d", i)})
	}
	f := frameOf(160, 37)
	var read []string
	at := p.Sided(1)
	for range 20 {
		read = append(read, litText(at, f))
		at = at.Moved(1)
	}
	want := []string{
		"■ work", "■ personal", "■ side", "■ account-0", "■ account-1", "■ account-2", "■ account-3", "■ account-4",
		"■ account-5", "■ account-6", "■ account-7", "■ account-8", "■ account-9", "■ opus-5-5", "■ haiku-4-5",
		"■ requests 1,240", "□ checks 31", "□ counts 4", "□ only those that failed 3",
		"□ d28c  switchboard   side       445 today   asking",
	}
	if !slices.Equal(read, want[:len(read)]) {
		t.Errorf("↓ lit\n%q\nwant\n%q", read, want)
	}
	if got := litText(at.Moved(99), f); !strings.HasPrefix(got, "□ 9c2e") {
		t.Errorf("↓ past the last tick lit %q, want the last, 9c2e's", got)
	}
	if got := litText(at.Moved(-99), f); got != "■ work" {
		t.Errorf("↑ past the first tick lit %q, want the first, work", got)
	}
	back := at.Sided(-1).Moved(2).Sided(1)
	if got := litText(back, f); got != litText(at, f) {
		t.Errorf("back on the right side, the cursor lit %q, want where it was, %q", got, litText(at, f))
	}
	if got := litText(back.Sided(-1), f); got != "■ on" {
		t.Errorf("back on the left side, the cursor lit %q, want where it was, on", got)
	}
	if got := litText(p.Sided(-1), f); got != "■ session" {
		t.Errorf("← on the left side lit %q, want it to stay there, on session", got)
	}
}

// A row of ticks too long for ONLY wraps under its label, its ticks from
// where its choices start.
func TestARowTooLongWrapsUnderItsLabel(t *testing.T) {
	p := framesPanel(LogRequests, 0)
	for i := range 10 {
		p.Accounts = append(p.Accounts, status.AccountNow{ID: fmt.Sprintf("account-%d", i)})
	}
	rows := bodyText(p, frameOf(160, 37))
	want := []string{
		"accounts  ■ work   ■ personal   ■ side   ■ account-0   ■ account-1",
		"          ■ account-2   ■ account-3   ■ account-4   ■ account-5   ■ account-6",
		"          ■ account-7   ■ account-8   ■ account-9",
		"models    ■ opus-5-5   ■ haiku-4-5",
	}
	for i, w := range want {
		if got := strings.TrimRight(string([]rune(rows[3+i])[min(onlyAt[LogRequests], len([]rune(rows[3+i]))):]), " "); got != w {
			t.Errorf("ONLY's row %d reads %q, want %q", i+1, got, w)
		}
	}
}

// Where its two sides' leasts don't fit, the panel shows the
// side the cursor's on alone, from its content's start; on a phone, today's
// sessions from ONLY's start, where they'd lack the room further in.
func TestWhereItsSidesLeastsDontFitThePanelShowsOneSideAtATime(t *testing.T) {
	tests := []struct {
		name       string
		width      int
		panel      LogPanel
		has, lacks string
	}{
		{name: "both sides of Requests at 105", width: 105, panel: framesPanel(LogRequests, 0), has: "COLUMNS                                 ONLY"},
		{name: "both sides of Events at 123", width: 123, panel: framesPanel(LogEvents, 0), has: "EVENTS        today                                       ONLY"},
		{name: "Events' left side at 122", width: 122, panel: framesPanel(LogEvents, 0), has: "EVENTS", lacks: "ONLY"},
		{name: "the left side at 104", width: 104, panel: framesPanel(LogRequests, 0), has: "COLUMNS", lacks: "ONLY"},
		{name: "the right side at 104", width: 104, panel: framesPanel(LogRequests, 0).Sided(1), has: "ONLY", lacks: "COLUMNS"},
		{name: "Events' right side on a phone", width: 52, panel: framesPanel(LogEvents, 0).Sided(1), has: "ONLY", lacks: "EVENTS"},
		{name: "today's sessions from ONLY's start on a phone", width: 52, panel: framesPanel(LogRequests, 0).Sided(1), has: "□ d28c  switchboard"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.Join(bodyText(tt.panel, frameOf(tt.width, 40)), "\n")
			if !strings.Contains(body, "\n"+tt.has) || tt.lacks != "" && strings.Contains(body, tt.lacks) {
				t.Errorf("the body reads\n%s\nwant a row starting %q, and no %q", body, tt.has, tt.lacks)
			}
		})
	}
}

// The panel's last line says what it would show: of Requests, how many of
// today's lines in how many columns; of Events, all of today's events, or
// how many of them, or that there are none yet, in text.faint.
// A session's row says it asking in accent.key, and as last known, the
// router not answering, as it was, in asKnown's dimmed ink.
func TestAPanelRowSaysWhatsInFlightAsLastKnownAsItWas(t *testing.T) {
	for _, doing := range []views.Doing{views.Asking, views.Throttled, views.Refused} {
		live := views.Activity{Doing: doing, Status: 429, LastSeen: routedAt}
		stale := live
		stale.Stale = true
		for a, want := range map[views.Activity]span{live: {"asking", doingInk}, stale: {"asking", dimInk}} {
			if got := sessionDoing(views.LogSession{Activity: a, Started: routedAt.Add(-time.Hour)}, routedAt); got != want {
				t.Errorf("doing %d, as last known %t, the row says %+v, want %+v", doing, a.Stale, got, want)
			}
		}
	}
}

func TestThePanelSaysWhatItWouldShow(t *testing.T) {
	some := framesPanel(LogEvents, 0)
	some.Events.Shown = 12
	none := framesPanel(LogEvents, 0)
	none.Events = views.EventCount{}
	one := framesPanel(LogRequests, 0)
	one.Columns = LogKept(theme.LogPrefs{Columns: []string{"session"}})
	tests := []struct {
		name  string
		panel LogPanel
		want  string
		faint bool
	}{
		{name: "of Requests", panel: framesPanel(LogRequests, 0), want: "It shows 1,240 of 1,275 lines, in 11 columns."},
		{name: "in a column", panel: one, want: "It shows 1,240 of 1,275 lines, in 1 column."},
		{name: "every event", panel: framesPanel(LogEvents, 0), want: "It shows all 20 of today's events."},
		{name: "some events", panel: some, want: "It shows 12 of today's 20 events."},
		{name: "no events yet", panel: none, want: "It shows today's events: none yet.", faint: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			said := tt.panel.shows()
			if got := said.plain(); got != tt.want || (said[0].ink == faintInk) != tt.faint {
				t.Errorf("it says %q in %v, want %q, faint %v", got, said[0].ink, tt.want, tt.faint)
			}
		})
	}
}

// Today's sessions show six at a time, those up to the cursor where it's
// past them, and a line under them counts those below.
func TestTodaysSessionsScrollWithTheCursor(t *testing.T) {
	p := framesPanel(LogRequests, 8).Sided(1).Moved(9 + 8)
	rows := bodyText(p, frameOf(160, 37))
	var listed []string
	for _, r := range rows[8:15] {
		listed = append(listed, strings.Fields(string([]rune(r)[min(onlyAt[LogRequests]+todayAt[LogRequests], len([]rune(r))):]))[1])
	}
	want := []string{"db8a", "41e0", "9c2e", "e000", "e001", "e002", "5"}
	if !slices.Equal(listed, want) {
		t.Errorf("the cursor on today's ninth session, the list reads %q, want %q: the six up to it, then 5 more", listed, want)
	}
	if got := litText(p, frameOf(160, 37)); !strings.HasPrefix(got, "□ e002") {
		t.Errorf("the cursor lit %q, want e002's row", got)
	}
}

// A click on a tick ticks it, as space does, the cursor moving to it,
// whichever side it's on; a click elsewhere in the panel does nothing.
func TestAClickOnATickTicksIt(t *testing.T) {
	f := frameOf(160, 37)
	tests := []struct {
		name   string
		x, y   int
		ticked bool
		check  func(LogPanel) bool
	}{
		{name: "on worth's name", x: 52, y: 12, ticked: true, check: func(p LogPanel) bool { return p.Columns.Shows(LogWorth) && p.side == 0 }},
		{name: "a cell before worth's box", x: 46, y: 12, ticked: true, check: func(p LogPanel) bool { return p.Columns.Shows(LogWorth) }},
		{name: "on d28c's doing", x: 120, y: 17, ticked: true, check: func(p LogPanel) bool { return slices.Equal(p.Filter.Sessions, []string{"d28c0001"}) && p.side == 1 }},
		{name: "between ticks", x: 31, y: 12},
		{name: "on the panel's keys", x: 130, y: 27},
		{name: "outside the panel", x: 5, y: 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ticked := framesPanel(LogRequests, 8).Clicked(f, tt.x, tt.y)
			if ticked != tt.ticked || tt.ticked && !tt.check(got) {
				t.Errorf("clicked at %d, %d, it ticked %v, giving %+v; want ticked %v", tt.x, tt.y, ticked, got.Filter, tt.ticked)
			}
		})
	}
	moved, _ := framesPanel(LogRequests, 8).Clicked(f, 52, 12)
	if got := litText(moved, f); got != "■ worth" {
		t.Errorf("after the click, the cursor lit %q, want worth, ticked", got)
	}
}

// Taller than the frame has room for, the panel shows the rows around the
// cursor's, so it's always in view.
func TestAPanelTallerThanItsRoomKeepsTheCursorInView(t *testing.T) {
	f := frameOf(160, 16)
	for _, p := range []LogPanel{framesPanel(LogRequests, 8), framesPanel(LogRequests, 8).Sided(1).Moved(20)} {
		k := p.card(f)
		if len(k.body) != f.bodyRoom(panelSpare, true) || k.lit.n != 1 || k.lit.first >= len(k.body) {
			t.Errorf("the card shows %d rows, lighting %+v, want %d rows, the cursor's among them", len(k.body), k.lit, f.bodyRoom(panelSpare, true))
		}
	}
}

// COLUMNS lays out those shown first, six to a column in its first two,
// then the rest, shown then hidden, in its third, as they were as it
// opened, so a tick never moves as it's ticked.
func TestColumnsLayOutThoseShownFirstAsTheyWereAsItOpened(t *testing.T) {
	p := framesPanel(LogRequests, 0)
	p.Laid = p.Laid.Toggled(LogWorth).Toggled(LogAgent)
	p.Columns = p.Laid.Toggled(LogSession)
	rows := bodyText(p, frameOf(160, 37))
	want := []string{
		"COLUMNS",
		"□ session    ■ read       ■ routing",
		"■ model      ■ written    □ input",
		"■ on         ■ out        □ attempts",
		"■ status     ■ worth      □ size",
	}
	for i, w := range want {
		if got := strings.TrimRight(string([]rune(rows[2+i])[:onlyAt[LogRequests]]), " "); got != w {
			t.Errorf("COLUMNS' row %d reads %q, want %q", i, got, w)
		}
	}
}

// The panel opens on what the page shows, and applying it shows what it
// picks.
func TestThePanelOpensOnWhatThePageShowsAndApplyingItShowsItsPicks(t *testing.T) {
	page := LogPage{Shown: LogEvents, Columns: LogTicks{}.Toggled(LogWorth), Filter: views.LogFilter{Failed: true}, Shows: views.EventFilter{Kinds: []string{status.EventCap}}}
	p := page.Panel()
	if p.Of != LogEvents || p.Laid != page.Columns || p.Columns != page.Columns || !p.Filter.Failed || !slices.Equal(p.Shows.Kinds, page.Shows.Kinds) {
		t.Errorf("the panel opened as %+v, want it on the page's Events and its picks", p)
	}
	p.Columns, p.Filter.Failed, p.Shows.Kinds = p.Columns.Toggled(LogAgent), false, nil
	applied := page.Applied(p)
	if !applied.Columns.Shows(LogAgent) || applied.Filter.Failed || applied.Shows.Kinds != nil || applied.Shown != LogEvents {
		t.Errorf("applied, the page shows %+v, want the panel's picks", applied)
	}
}

// A session or a model the filter ticks that today counts none of, as one
// ticked before midnight, has a tick, ticked, as views lists it, on either
// table's panel: ticking another keeps it, and let go, it stays listed to
// be ticked again.
func TestWhatTheFilterTicksThatTodayCountsNoneOfHasATick(t *testing.T) {
	const gone, old = "0ld50001", "claude-opus-4-1"
	built := func(p LogPanel) LogPanel {
		p.Sessions = views.LogSessions(views.LogSessionSources{
			Lines: map[string]int{"d28c0001": 445}, Events: map[string]int{"d28c0001": 2}, Ticked: p.TickedSessions(), Unlisted: true, Now: p.Now,
		})
		return p
	}
	tickOf := func(p LogPanel, on tickOn) (tick, bool) {
		ticks := p.sideTicks(1)
		i := slices.IndexFunc(ticks, func(tk tick) bool { return tk.on == on })
		if i < 0 {
			return tick{}, false
		}
		return ticks[i], true
	}
	for _, of := range []LogView{LogRequests, LogEvents} {
		page := LogPage{Shown: of, Filter: views.LogFilter{Sessions: []string{gone}, Models: []string{old}}, Shows: views.EventFilter{Sessions: []string{gone}}}
		p := page.Panel()
		frames := framesPanel(of, 0)
		p.Accounts, p.Lines, p.Events, p.Now = frames.Accounts, frames.Lines, frames.Events, frames.Now
		p = built(p)

		session, ok := tickOf(p, tickOn{sessionTick, gone})
		if !ok || !session.ticked {
			t.Fatalf("of %d, the filter ticks %s, but the panel's tick of it is %+v, listed %t", of, gone, session, ok)
		}
		p = built(session.toggle(p))
		if again, ok := tickOf(p, tickOn{sessionTick, gone}); !ok || again.ticked {
			t.Errorf("of %d, %s let go is listed %t, ticked %t; want it listed, unticked", of, gone, ok, again.ticked)
		}
		if of == LogEvents {
			continue
		}
		model, ok := tickOf(p, tickOn{modelTick, old})
		if !ok || !model.ticked {
			t.Fatalf("the filter ticks %s, but the panel's tick of it is %+v, listed %t", old, model, ok)
		}
		haiku, _ := tickOf(p, tickOn{modelTick, "claude-haiku-4-5"})
		if got, want := haiku.toggle(p).Filter.Models, []string{"claude-haiku-4-5", old}; !slices.Equal(got, want) {
			t.Errorf("haiku ticked too, the filter keeps the models %q, want %q", got, want)
		}
	}
}

// labelledPanel is the frames' panel of the table given, personal labelled
// as given.
func labelledPanel(of LogView, label string) LogPanel {
	p := framesPanel(of, 0)
	p.Accounts[1].Label = label
	return p
}

// A session's row's account is a column of names, as labelColumn lays it out
// with the rest of its row in ONLY: as wide as the widest name the panel
// knows, at most 24; where ONLY's short of room, the rows start further
// out first, then the name narrows toward 8.
func TestASessionsRowsAccountIsALabelsColumn(t *testing.T) {
	tests := []struct {
		name  string
		panel LogPanel
		width int
		at    int
		want  string
	}{
		{name: "the frames' names, 8 cells", panel: framesPanel(LogRequests, 0), width: 78, at: 12, want: "9c2e  website       personal  267 today   idle 9m"},
		{name: "a name of 24, whole", panel: labelledPanel(LogRequests, "a-very-long-account-name"), width: 85, at: 12,
			want: "9c2e  website       a-very-long-account-name  267 today   idle 9m"},
		{name: "a name past 24, cut there", panel: labelledPanel(LogRequests, "a-very-long-account-name-and-more"), width: 85, at: 12,
			want: "9c2e  website       a-very-long-account-nam…  267 today   idle 9m"},
		{name: "short of room, the blank cells in go first", panel: labelledPanel(LogRequests, "a-very-long-account-name"), width: 78, at: 5,
			want: "9c2e  website       a-very-long-account-name  267 today   idle 9m"},
		{name: "then the name narrows", panel: labelledPanel(LogRequests, "a-very-long-account-name"), width: 60, at: 0,
			want: "9c2e  website       a-very-lon…  267 today   idle 9m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := tt.panel.sessionColumns(tt.width)
			if got := strings.TrimRight(tt.panel.sessionWords(tt.panel.Sessions[5], at.account).plain(), " "); got != tt.want || at.at != tt.at {
				t.Errorf("the row reads\n%q from %d\nwant\n%q from %d", got, at.at, tt.want, tt.at)
			}
		})
	}
}

// The panel is panelNarrowest wide, 124, with the frames' names, wherever it
// shows both sides, or as the frame has room for.
func TestWithTheFramesNamesThePanelIsItsNarrowest(t *testing.T) {
	for _, of := range []LogView{LogRequests, LogEvents} {
		p := framesPanel(of, 0)
		for width := 40; width <= 220; width++ {
			f := frameOf(width, 60)
			got, _, _, _ := p.laidOut(f)
			if p.bothFit(got-2*cardInset) && got != f.cardWidth(124) {
				t.Errorf("%d at %d columns, the panel is %d wide, want %d", of, width, got, f.cardWidth(124))
			}
		}
	}
}

// Where every name is under 8 cells, the account's column is as wide as
// the widest of them, never wider, however much room the panel has.
func TestASessionsAccountColumnIsNoWiderThanItsNames(t *testing.T) {
	for _, of := range []LogView{LogRequests, LogEvents} {
		p := framesPanel(of, 0)
		for i, label := range []string{"ab", "cdef", "gh"} {
			p.Accounts[i].Label = label
		}
		for _, width := range []int{60, 78, 100} {
			if got := p.sessionColumns(width).account; got != 4 {
				t.Errorf("%d at %d cells, the account's column is %d cells, want 4, its widest name's", of, width, got)
			}
		}
	}
}

// Where a configured name is long enough that today's sessions' account
// reaches its natural, as wide as the name, the panel widens to its parts'
// natural where the frame has room: the name whole, on either side.
func TestALongNameWidensThePanelToItsParts(t *testing.T) {
	const name = "a-very-long-account-name"
	for _, of := range []LogView{LogRequests, LogEvents} {
		for side := range 2 {
			p := labelledPanel(of, name).Sided(side)
			for _, width := range []int{160, 220} {
				body := strings.Join(bodyText(p, frameOf(width, 40)), "\n")
				if !strings.Contains(body, "  "+name+"  ") {
					t.Errorf("%d, side %d, at %d columns, the body reads\n%s\nwant %q whole", of, side, width, body, name)
				}
			}
		}
	}
}

// With a long name, on either side, at every width, no row of the panel is
// wider than its card's content wherever today's sessions' rows fit there
// at their least, and none cuts what a session's doing.
func TestNoPanelRowRunsPastItsCard(t *testing.T) {
	for _, of := range []LogView{LogRequests, LogEvents} {
		for side := range 2 {
			p := labelledPanel(of, "a-very-long-account-name").Sided(side)
			for width := 40; width <= 220; width++ {
				f := frameOf(width, 60)
				content := f.cardWidth(p.natural()) - 2*cardInset
				body := p.body(content, p.bothFit(content)).rows()
				_, fits := flex.Lay(content, p.sessionParts()...)
				if !fits {
					continue
				}
				said := ""
				for y, l := range body {
					said += l.plain() + "\n"
					if l.width() > content {
						t.Errorf("%d, side %d, at %d columns, row %d is %d cells, past the card's %d: %q", of, side, width, y+1, l.width(), content, l.plain())
					}
				}
				if !strings.Contains(said, "d28c") {
					continue
				}
				for _, doing := range []string{"asking", "started 14:41", "idle 9m"} {
					if !strings.Contains(said, doing) {
						t.Errorf("%d, side %d, at %d columns, the panel reads\n%s\nwant %q whole", of, side, width, said, doing)
					}
				}
			}
		}
	}
}
