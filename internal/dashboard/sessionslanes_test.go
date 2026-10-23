package dashboard

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// sceneCall is the call of the session with the given id on the account
// given, run in a directory of the name given, doing what a says, and come
// from the account named, "" where no move brought it.
func sceneCall(account, session, dir, from string, a views.Activity) views.Call {
	return views.Call{Account: account, Session: session, Activity: a, Dir: "~/code/" + dir, From: from}
}

// phoneScene are the calls of the 14:42:07 scene, as sessions-phone-52
// draws them: 41e0 and 9c2e idle on personal; on side, c61b answering, d28c
// asking, 7f3a answering and db8a idle, the last three come from work.
func phoneScene() []views.Call {
	return []views.Call{
		sceneCall("personal", id41E0, "docs", "", idleFor(34*time.Second)),
		sceneCall("personal", id9C2E, "website", "", idleFor(9*time.Minute)),
		sceneCall("side", idC61B, "mobile", "", views.Activity{Doing: views.Answering}),
		sceneCall("side", idD28C, "switchboard", "work", views.Activity{Doing: views.Asking}),
		sceneCall("side", id7F3A, "api", "work", views.Activity{Doing: views.Answering}),
		sceneCall("side", idDB8A, "infra", "work", idleFor(50*time.Second)),
	}
}

// lanesFrame is a frame of Sessions' Switchboard, width by height, in the
// look given, of the calls given, as the patch keeps them at a first look.
func lanesFrame(width, height int, look Look, calls []views.Call) Frame {
	f := frameOf(width, height)
	f.View, f.Look = Sessions, look
	f.SessionsTab = SessionsShown{Patched: views.Patch(views.Patched{}, calls, nil), Calls: calls}
	return f
}

// lanesText are the rows of the frame of doc at routedAt as text alone.
func lanesText(f Frame, doc status.Document) []string {
	return textOf(f.Draw(doc, routedAt))
}

// A session's row says what it's doing where a lane's columns put it, in a
// session's page's words, bold accent.key while it asks or answers, bold
// state.destructive while refused, text.subtle while throttled, and
// text.faint while idle;
// and its id is bold text.primary while it works, refused or throttled
// included, as its request is to be sent again, else text.muted.
func TestALanesSessionSaysWhatItsDoingAndIsBoldWhileItWorks(t *testing.T) {
	tests := []struct {
		name      string
		activity  views.Activity
		said      string
		doing, id ink
	}{
		{name: "asking", activity: views.Activity{Doing: views.Asking}, said: "↑ asking", doing: keyInk, id: titleInk},
		{name: "answering", activity: views.Activity{Doing: views.Answering}, said: "↓ answering", doing: keyInk, id: titleInk},
		{name: "refused", activity: views.Activity{Doing: views.Refused, Status: 429}, said: "✕ 429", doing: exhaustedInk, id: titleInk},
		{name: "throttled", activity: views.Activity{Doing: views.Throttled, Status: 429}, said: "… 429", doing: dimInk, id: titleInk},
		{name: "idle", activity: idleFor(34 * time.Second), said: "idle 34s", doing: faintInk, id: mutedInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := lanesFrame(52, 33, Look{}, []views.Call{sceneCall("side", idC61B, "mobile", "", tt.activity)})
			doc := phoneDoc()
			c := f.At(doc, routedAt).page(doc, routedAt)
			y := slices.IndexFunc(c.rows(Look{}), func(r string) bool { return strings.HasPrefix(r, "  c61b mobile") })
			if y < 0 {
				t.Fatalf("no row says c61b; drew\n%s", strings.Join(lanesText(f, doc), "\n"))
			}
			k := laneColumnsOf(f.Width, 0)
			if got := strings.TrimRight(c.rows(Look{})[y][k.doing:], " "); got != tt.said {
				t.Errorf("c61b's row says %q at column %d, want %q", got, k.doing, tt.said)
			}
			if got := c.at(k.doing, y).ink; got.token != tt.doing.token || got.bold != tt.doing.bold {
				t.Errorf("what c61b's doing is in %+v, want %+v", got, tt.doing)
			}
			if got := c.at(k.id, y).ink; got.token != tt.id.token || got.bold != tt.id.bold {
				t.Errorf("c61b's id is in %+v, want %+v", got, tt.id)
			}
		})
	}
}

// An account's name row ends at the last column with ● pinned, where the
// global pin names it, or ✕ out of the pool, in text.faint, where it leaves
// it out, its name faint; its short state words, cut by whole clauses, end
// two cells before them. Its windows' bars are faded as the Overview fades
// them, but its state words keep their colours.
func TestALanesNameRowSaysWhetherThePinNamesItOrLeavesItOut(t *testing.T) {
	doc := usageWorld(extraOff, usagePinned("personal", "side"))
	f := lanesFrame(52, 33, Look{}, phoneScene())
	rows := lanesText(f, doc)
	work := endsWith("1 work          ● at its cap", "✕ out of the pool", f.Width)
	for _, want := range []string{
		work,
		endsWith("2 personal ◆ ▲  ● open · next", "● pinned", f.Width),
		endsWith("3 side          ● open", "● pinned", f.Width),
	} {
		if !slices.Contains(rows, want) {
			t.Errorf("no row reads %q; drew\n%s", want, strings.Join(rows, "\n"))
		}
	}
	c := f.At(doc, routedAt).page(doc, routedAt)
	y := slices.Index(rows, work)
	for _, at := range []struct {
		name string
		x    int
		want theme.Token
	}{
		{name: "work's name", x: 2, want: theme.TextFaint},
		{name: "its state words", x: 16, want: theme.AccentAttention},
		{name: "✕ out of the pool", x: 35, want: theme.TextFaint},
	} {
		if got := c.at(at.x, y).ink.token; got != at.want {
			t.Errorf("%s, at column %d, is in %s, want %s", at.name, at.x, got, at.want)
		}
	}
	if got := c.at(laneWindowsAt+laneFirstBar, y+1).ink.fade; got != staleFade {
		t.Errorf("work's 5-hour bar is faded %v, want %v, as the Overview fades an account out of the pool", got, staleFade)
	}
}

// Where a lane's mark would leave its state words short of their first
// clause, the mark gives way, the words saying first how the account
// stands: at 46 columns, work's ● at its cap · 17:10 shows whole, without
// ✕ out of the pool, while personal's ● open leaves its ● pinned room.
func TestALanesPinMarkGivesWayToItsFirstWords(t *testing.T) {
	doc := usageWorld(extraOff, usagePinned("personal", "side"))
	f := lanesFrame(46, 33, Look{}, phoneScene())
	rows := lanesText(f, doc)
	for _, want := range []string{"1 work          ● at its cap · 17:10", endsWith("2 personal ◆ ▲  ● open · next", "● pinned", f.Width)} {
		if !slices.Contains(rows, want) {
			t.Errorf("no row reads %q; drew\n%s", want, strings.Join(rows, "\n"))
		}
	}
}

// endsWith is a row width cells wide, left at its start and right ending
// at its last column.
func endsWith(left, right string, width int) string {
	return left + strings.Repeat(" ", width-len([]rune(left))-len([]rune(right))) + right
}

// Where rows are too few to stack them, a lane's windows share one row, in
// the order of care: Week at column 1, its bar from 8, a cell and its
// percentage; three cells on, 5-hour, its bar 7 cells after it, a cell and
// its percentage; each bar ⌊(width − 28) ÷ 2⌋ cells, ending at the last
// column where the width leaves no cell over.
func TestALanesWindowsShareARowEachBarHalfWhatTheWidthLeaves(t *testing.T) {
	for _, tt := range []struct{ width, bar int }{{52, 12}, {61, 16}, {80, 26}, {99, 35}} {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			f := lanesFrame(tt.width, 40, Look{}, phoneScene())
			doc := phoneDoc()
			rows := f.At(doc, routedAt).page(doc, routedAt).rows(Look{})
			y := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "2 personal") }) + 1
			row := []rune(rows[y])
			second := laneWindowsAt + laneFirstBar + tt.bar + 1 + percentCells + groupGap
			for _, at := range []struct {
				x    int
				want string
			}{
				{x: laneWindowsAt, want: "Week"},
				{x: laneWindowsAt + laneFirstBar + tt.bar + 1, want: " 66%"},
				{x: second, want: "5-hour"},
				{x: second + laneNextBar + tt.bar + 1, want: " 39%"},
			} {
				if got := string(row[at.x : at.x+len([]rune(at.want))]); got != at.want {
					t.Errorf("column %d reads %q, want %q; the row reads %q", at.x, got, at.want, rows[y])
				}
			}
			if end := second + laneNextBar + tt.bar + 1 + percentCells; strings.TrimRight(rows[y], " ") != string(row[:end]) {
				t.Errorf("the row ends past column %d: %q", end, rows[y])
			}
		})
	}
}

// The lanes draw no pool at any width they're drawn at, whether they fit
// the view or scroll, with three accounts or one: no rule, no Σ pool and no
// room left. It isn't an account and has no sessions, and the board never
// shows it.
func TestTheLanesDrawNoPool(t *testing.T) {
	one := usageWorld(extraOff, func(d *status.Document) { d.Accounts, d.Best = d.Accounts[:1], "work" })
	for _, doc := range []status.Document{phoneDoc(), one} {
		for width := 52; width < 100; width++ {
			for _, height := range []int{61, 36, 20} {
				f := lanesFrame(width, height, Look{}, phoneScene())
				rows := lanesText(f, doc)
				if slices.ContainsFunc(rows, func(r string) bool {
					return strings.Contains(r, "Σ pool") || strings.Contains(r, " of 3 left") || strings.HasPrefix(r, "─")
				}) {
					t.Errorf("at %d×%d, with %d accounts, the lanes draw the pool:\n%s", width, height, len(doc.Accounts), strings.Join(rows, "\n"))
				}
			}
		}
	}
}

// With one account, its lane is drawn as with more: its windows stacked
// where rows allow, a blank row before its call, and the key over the key
// line; its name column at its least, a label of labelLeast.
func TestWithOneAccountTheLaneStandsAlone(t *testing.T) {
	doc := usageWorld(extraOff, func(d *status.Document) { d.Accounts, d.Best = d.Accounts[:1], "work" })
	f := lanesFrame(52, 33, Look{}, []views.Call{sceneCall("work", idC61B, "mobile", "", idleFor(time.Minute))})
	rows := lanesText(f, doc)
	top := f.top()
	// Each row starts as given, the windows' rows' bars and words held by the
	// test of a lane's stacked windows.
	want := []string{"1 work ▲      ● at its cap · 17:10", " Week ", "", " 5-hour ", "", " Fable wk ", "", fmt.Sprintf("%-33sidle 1m", "  c61b mobile")}
	for i, w := range want {
		if got := rows[top+i]; !strings.HasPrefix(got, w) || w == "" && got != "" {
			t.Errorf("row %d reads %q, want it to start %q; drew\n%s", top+i+1, got, w, strings.Join(rows, "\n"))
		}
	}
	if got := rows[f.Height-3]; !strings.HasPrefix(got, "┃ even pace") {
		t.Errorf("the row two over the key line reads %q, want the key's first", got)
	}
}

// With no session running, as while no router lists any, the lanes say so
// as the Switchboard does, in text.faint, where the first session would be:
// under the first account's windows, after the blank row before its calls,
// its lane two rows longer, the others standing empty after it.
func TestWithNoSessionRunningTheLanesSaySo(t *testing.T) {
	doc := phoneDoc()
	f := lanesFrame(52, 33, Look{}, nil)
	c := f.At(doc, routedAt).page(doc, routedAt)
	rows := lanesText(f, doc)
	top := f.top()
	if got := rows[top+3]; got != "  "+noSessionsRunning {
		t.Errorf("the row under work's windows reads %q, want %q", got, "  "+noSessionsRunning)
	}
	if got := c.at(2, top+3).ink; got != faintInk {
		t.Errorf("it's in %+v, want text.faint", got)
	}
	for _, want := range []struct {
		y    int
		says string
	}{{top + 2, ""}, {top + 4, ""}, {top + 5, ""}, {top + 6, "2 personal ◆"}, {top + 8, ""}, {top + 9, ""}, {top + 10, "3 side ▲"}} {
		if got := rows[want.y]; !strings.HasPrefix(got, want.says) || want.says == "" && got != "" {
			t.Errorf("row %d reads %q, want it to start %q", want.y+1, got, want.says)
		}
	}
	if n := strings.Count(strings.Join(rows, "\n"), noSessionsRunning); n != 1 {
		t.Errorf("the lanes say %q %d times, want once", noSessionsRunning, n)
	}
}

// From 100 columns, where the bent cords outnumber the bends' room, the
// Switchboard draws lanes under the wide heading: here, too many rows for
// the windows stacked, side by side, the bars half what the width leaves,
// and the wide key, its marks four cells apart.
func TestWhereBentCordsOutnumberTheRoomTheSwitchboardDrawsLanes(t *testing.T) {
	var calls []views.Call
	for i := range 28 {
		calls = append(calls, sceneCall("side", fmt.Sprintf("s%03d-session", i), "api", "", idleFor(time.Minute)))
	}
	doc := phoneDoc()
	f := lanesFrame(160, 60, Look{}, calls)
	if !f.switchboardLaid(doc, routedAt).lanes {
		t.Fatalf("28 bent cords at 160 columns are drawn as cords, want lanes")
	}
	rows := lanesText(f, doc)
	top := f.top()
	for _, want := range []struct {
		y    int
		says string
	}{
		{y: top, says: "1 work          ● at its cap · 17:10"},
		{y: top + 11, says: fmt.Sprintf("%-33sidle 1m", "  s000 api")},
	} {
		if got := rows[want.y]; got != want.says {
			t.Errorf("row %d reads %q, want %q", want.y+1, got, want.says)
		}
	}
	bar := (160 - 28) / 2
	if got := []rune(rows[top+1])[laneWindowsAt+laneFirstBar+bar+1:][:4]; string(got) != " 34%" {
		t.Errorf("work's week percentage reads %q, want a bar of %d before it", string(got), bar)
	}
	if !slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, "◆ primary    ▲ new sessions go here") }) {
		t.Errorf("the key isn't the wide one:\n%s", strings.Join(rows, "\n"))
	}
	if !strings.Contains(rows[f.headingY()], "[ ]") {
		t.Errorf("the heading row reads %q, want the wide one", rows[f.headingY()])
	}
}

// Where the lanes don't fit the view with the key under them, the key gives
// way and they scroll an account at a time, its line and its sessions
// together: ▲ and how many accounts are above on the view's first row, while
// any are, and ▼ and how many below, right-aligned on the row over the key
// line, no further than shows the last account; a page up going back as far
// as shows the lane above the first shown, and no further.
func TestLanesThatDontFitScrollAnAccountAtATime(t *testing.T) {
	doc := phoneDoc()
	tests := []struct {
		offset    int
		scrolling Scrolling
		first     string
		over      string
	}{
		{offset: 0, scrolling: Scrolling{Most: 2, Page: 2}, first: "1 work          ● at its cap · 17:10", over: "               ▼ 1 more account"},
		{offset: 1, scrolling: Scrolling{Most: 2, Page: 1}, first: "▲ 1 more account", over: "               ▼ 1 more account"},
		{offset: 2, scrolling: Scrolling{Most: 2, Page: 1, Back: 2}, first: "▲ 2 more accounts"},
		{offset: 5, scrolling: Scrolling{Most: 2, Page: 1, Back: 2}, first: "▲ 2 more accounts"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.offset), func(t *testing.T) {
			f := lanesFrame(52, 20, Look{}, phoneScene())
			f.Scroll = tt.offset
			if got := f.Scrolling(doc, routedAt); got != tt.scrolling {
				t.Errorf("Scrolling() = %+v, want %+v", got, tt.scrolling)
			}
			rows := lanesText(f, doc)
			if got := rows[f.top()]; got != tt.first {
				t.Errorf("the view's first row reads %q, want %q", got, tt.first)
			}
			if got := strings.TrimSpace(rows[f.Height-underView]); got != strings.TrimSpace(tt.over) {
				t.Errorf("the row over the key line reads %q, want %q", got, strings.TrimSpace(tt.over))
			}
			if tt.over != "" && len([]rune(rows[f.Height-underView])) != f.Width {
				t.Errorf("the row over the key line reads %q, want it right-aligned", rows[f.Height-underView])
			}
			if slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, "Σ pool") || strings.Contains(r, "even pace") }) {
				t.Errorf("the key shows while the lanes scroll:\n%s", strings.Join(rows, "\n"))
			}
		})
	}
	if got := lanesFrame(52, 33, Look{}, phoneScene()).Scrolling(doc, routedAt); got != (Scrolling{}) {
		t.Errorf("the lanes that fit scroll %+v, want nowhere", got)
	}
}

// Where a lane doesn't fit the view, under ▲ where any are above it, the
// lanes scroll a row at a time instead, the key giving way, so no session
// is cut out of reach: a lane's name row shows with its windows' row, or the
// page ends over it; and right-aligned on the row over the key line, how many
// rows are out of view, above and below, as USAGE says it.
func TestLanesScrollARowAtATimeWhereALaneDoesntFit(t *testing.T) {
	doc := phoneDoc()
	tests := []struct {
		offset    int
		scrolling Scrolling
		first     string
		over      string
	}{
		{offset: 0, scrolling: Scrolling{Most: 8, Page: 6}, first: "1 work          ● at its cap · 17:10", over: "▼ 8 more below"},
		{offset: 3, scrolling: Scrolling{Most: 8, Page: 5}, first: "2 personal ◆    ● open", over: "▲ 3 more above · ▼ 6 more below"},
		{offset: 8, scrolling: Scrolling{Most: 8, Page: 6}, first: "3 side ▲        ● open · next", over: "▲ 8 more above"},
	}
	seen := make(map[string]bool)
	for _, tt := range tests {
		f := lanesFrame(52, 15, Look{}, phoneScene())
		f.Scroll = tt.offset
		if got := f.Scrolling(doc, routedAt); got != tt.scrolling || !f.ScrollsRows(doc, routedAt) {
			t.Errorf("scrolled %d, Scrolling() = %+v, a row at a time %v; want %+v, a row at a time", tt.offset, got, f.ScrollsRows(doc, routedAt), tt.scrolling)
		}
		rows := lanesText(f, doc)
		if got := rows[f.top()]; got != tt.first {
			t.Errorf("scrolled %d, the view's first row reads %q, want %q", tt.offset, got, tt.first)
		}
		if got := rows[f.Height-underView]; strings.TrimSpace(got) != tt.over || len([]rune(got)) != f.Width {
			t.Errorf("scrolled %d, the row over the key line reads %q, want %q right-aligned", tt.offset, got, tt.over)
		}
		for _, call := range phoneScene() {
			seen[call.Session] = seen[call.Session] || slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, status.ShownID(call.Session)) })
		}
	}
	for _, call := range phoneScene() {
		if !seen[call.Session] {
			t.Errorf("%s's row never shows, scrolled as far as the lanes go", status.ShownID(call.Session))
		}
	}
	if f := lanesFrame(52, 20, Look{}, phoneScene()); f.ScrollsRows(doc, routedAt) {
		t.Errorf("at 20 rows, where every lane fits, the lanes scroll a row at a time, want an account at a time")
	}
}

// Each session's row is a zone a click chooses its call by, across the row,
// its session and its account named; the row chosen is on bg.selection
// across the row.
func TestALanesSessionsAreClickedAndChosen(t *testing.T) {
	doc := phoneDoc()
	f := lanesFrame(52, 33, Look{}, phoneScene())
	f.SessionsTab.Chosen = Plug{Session: id7F3A, Account: "side"}
	var got []Target
	for _, t := range f.Targets(doc, routedAt) {
		if t.On == OnSession {
			got = append(got, t)
		}
	}
	top := f.top()
	want := []Target{
		{Y: top + 7, Width: 52, On: OnSession, Session: id41E0, Account: "personal"},
		{Y: top + 8, Width: 52, On: OnSession, Session: id9C2E, Account: "personal"},
		{Y: top + 14, Width: 52, On: OnSession, Session: idC61B, Account: "side"},
		{Y: top + 15, Width: 52, On: OnSession, Session: idD28C, Account: "side"},
		{Y: top + 16, Width: 52, On: OnSession, Session: id7F3A, Account: "side"},
		{Y: top + 17, Width: 52, On: OnSession, Session: idDB8A, Account: "side"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the sessions' targets are\n%+v\nwant\n%+v", got, want)
	}
	c := f.At(doc, routedAt).page(doc, routedAt)
	for y := top + 14; y <= top+17; y++ {
		for _, x := range []int{0, 2, 51} {
			if lit := c.at(x, y).ink.on.token == theme.BgSelection; lit != (y == top+16) {
				t.Errorf("row %d, column %d, on bg.selection: %t; want only 7f3a's row", y+1, x, lit)
			}
		}
	}
}

// The bars the lanes fill are each account's 5-hour and week the view shows,
// as they're used, never the windows a lane doesn't show, nor the pool's,
// nor an account's without a token, whose bars are drawn empty.
func TestTheLanesFillTheBarsTheyShow(t *testing.T) {
	tests := []struct {
		name   string
		doc    status.Document
		height int
		scroll int
		want   map[Ref]float64
	}{
		{name: "whole", doc: phoneDoc(), height: 33, want: map[Ref]float64{
			{Account: "work", Window: "5h"}: 0.95, {Account: "work", Window: "7d"}: 0.34,
			{Account: "personal", Window: "5h"}: 0.39, {Account: "personal", Window: "7d"}: 0.66,
			{Account: "side", Window: "5h"}: 0.13, {Account: "side", Window: "7d"}: 0.78,
		}},
		{name: "scrolled to side", doc: phoneDoc(), height: 20, scroll: 2, want: map[Ref]float64{
			{Account: "side", Window: "5h"}: 0.13, {Account: "side", Window: "7d"}: 0.78,
		}},
		{name: "side without a token", doc: usageWorld(extraOff, func(d *status.Document) { d.Accounts[2].TokenSet = false }), height: 33, want: map[Ref]float64{
			{Account: "work", Window: "5h"}: 0.95, {Account: "work", Window: "7d"}: 0.34,
			{Account: "personal", Window: "5h"}: 0.39, {Account: "personal", Window: "7d"}: 0.66,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := lanesFrame(52, tt.height, Look{}, phoneScene())
			f.Scroll = tt.scroll
			got := f.Filled(tt.doc, routedAt)
			if !maps.EqualFunc(got, tt.want, func(a, b float64) bool { return a-b < 1e-9 && b-a < 1e-9 }) {
				t.Errorf("Filled() = %v, want %v", got, tt.want)
			}
		})
	}
}

// laneRow is the row of rows a lane's name row is on, starting with its
// account's number and label: -1 where none is.
func laneRow(rows []string, number int, label string) int {
	return slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, fmt.Sprintf("%d %s", number, label)) })
}

// Where rows allow, a lane stacks its windows as the board's lines do, each
// row the board's own, cell for cell, where its bars reach their natural:
// Week, a blank row, 5-hour, a blank row, Fable wk; its bar lineBar cells
// and its words in full, the line running to the last column, from 73
// columns. Narrower, the line narrows in the board's turns: the bars to
// leastWideBar, then the words to usage's short form.
func TestALanesWindowsStackAsTheBoardsLines(t *testing.T) {
	doc := phoneDoc()
	board := lanesFrame(160, 60, Look{}, phoneScene())
	laid := board.switchboardLaid(doc, routedAt)
	if laid.lanes || laid.step != 1 || laid.columns.bar != lineBar || laid.columns.shortWords {
		t.Fatalf("the board at 160×60 is laid %+v, want cords at step 1, its bars %d, its words in full", laid, lineBar)
	}
	boardRows := lanesText(board, doc)
	f := lanesFrame(73, 70, Look{}, phoneScene())
	rows := lanesText(f, doc)
	for i, label := range []string{"work", "personal", "side"} {
		y := laneRow(rows, i+1, label)
		if y < 0 {
			t.Fatalf("no lane is %s's; drew\n%s", label, strings.Join(rows, "\n"))
		}
		top := board.top() + laid.lines[i].top
		for j := range 5 {
			want := strings.TrimRight(string([]rune(fmt.Sprintf("%-160s", boardRows[top+1+j]))[laid.columns.lines:]), " ")
			if got := strings.TrimRight(rows[y+1+j], " "); got != want {
				t.Errorf("%s's lane's row %d reads\n%q\nwant the board's\n%q", label, j+1, got, want)
			}
		}
	}
	for _, tt := range []struct {
		width, bar int
		short      bool
	}{{99, 32, false}, {73, 32, false}, {72, 31, false}, {57, 16, false}, {56, 16, true}, {52, 16, true}} {
		lf := lanesFrame(tt.width, 70, Look{}, phoneScene())
		k := lf.laneLines(lf.Moment(doc, routedAt)).laid.columns
		if k.bar != tt.bar || k.shortWords != tt.short {
			t.Errorf("at %d columns, a lane's bars are %d, their words short %t; want %d, %t", tt.width, k.bar, k.shortWords, tt.bar, tt.short)
		}
	}
}

// A lane has a blank row between its line and its calls, and the accounts
// are two blank rows apart, where rows allow.
func TestTheLanesGapsWhereRowsAllow(t *testing.T) {
	doc := phoneDoc()
	f := lanesFrame(73, 70, Look{}, phoneScene())
	rows := lanesText(f, doc)
	work, personal, side := laneRow(rows, 1, "work"), laneRow(rows, 2, "personal"), laneRow(rows, 3, "side")
	for _, tt := range []struct {
		name      string
		from, to  int
		blank     int
		nextStart string
	}{
		{name: "work to personal", from: work + 5, to: personal, blank: 2, nextStart: "2 personal"},
		{name: "personal's line to its calls", from: personal + 5, to: personal + 7, blank: 1, nextStart: "  41e0"},
		{name: "personal's calls to side", from: personal + 8, to: side, blank: 2, nextStart: "3 side"},
		{name: "side's line to its calls", from: side + 5, to: side + 7, blank: 1, nextStart: "  c61b"},
	} {
		if rows[tt.from] == "" {
			t.Errorf("%s: row %d is blank, want the last of what's before the gap", tt.name, tt.from+1)
		}
		for y := tt.from + 1; y < tt.from+1+tt.blank; y++ {
			if rows[y] != "" {
				t.Errorf("%s: row %d reads %q, want it blank", tt.name, y+1, rows[y])
			}
		}
		if y := tt.from + 1 + tt.blank; y != tt.to || !strings.HasPrefix(rows[y], tt.nextStart) {
			t.Errorf("%s: row %d reads %q, want it to start %q", tt.name, y+1, rows[y], tt.nextStart)
		}
	}
}

// laneStepOf is the place among laneSteps of the step the lanes of f take,
// of doc at routedAt, and whether they fit the view with the key under them.
func laneStepOf(f Frame, doc status.Document) (int, bool) {
	b := f.lanesOf(doc, routedAt, f.switchboardLaid(doc, routedAt))
	return slices.Index(laneSteps, b.step), b.fits()
}

// As rows run short, the lanes take their steps in order: their windows
// stacked, accounts 2 apart and a blank row before their calls; then side by
// side; then the gaps back to their least; then they scroll, the key giving
// way. Each step holds while its lanes, a blank row and the key fit the
// view and the row over the key line: of the scene's 30 rows stacked, from
// 41 rows of terminal at 73 columns; its 18 side by side, from 29; and its
// 14 with the least gaps, from 25.
func TestTheLanesStepsAsRowsRunShort(t *testing.T) {
	doc := phoneDoc()
	for height := 12; height <= 70; height++ {
		want, fits := 2, false
		switch {
		case height >= 41:
			want, fits = 0, true
		case height >= 29:
			want, fits = 1, true
		case height >= 25:
			want, fits = 2, true
		}
		f := lanesFrame(73, height, Look{}, phoneScene())
		if got, gotFits := laneStepOf(f, doc); got != want || gotFits != fits {
			t.Errorf("at 73×%d, the lanes take step %d, fitting %t; want %d, %t", height, got, gotFits, want, fits)
		}
		rows := lanesText(f, doc)
		if y := laneRow(rows, 1, "work"); y >= 0 && strings.Contains(rows[y+1], "5-hour") != (want > 0) {
			t.Errorf("at 73×%d, work's windows read %q, want them side by side: %t", height, rows[y+1], want > 0)
		}
	}
}

// Each lane's label shows whole wherever it fits, its column the widest
// configured heading and two cells, the state after it alike on every lane;
// the labels are cut, to their least of 8, only where the width leaves no
// room.
func TestALanesLabelIsWholeWhereItFits(t *testing.T) {
	doc := usageWorld(extraOff, func(d *status.Document) {
		d.Accounts[0].Label, d.Accounts[1].Label, d.Accounts[2].Label = "work-team-plan", "research", "side-hustle-consulting"
	})
	headings := []string{"1 work-team-plan", "2 research ◆", "3 side-hustle-consulting ▲"}
	for _, width := range []int{52, 73, 99} {
		rows := lanesText(lanesFrame(width, 61, Look{}, phoneScene()), doc)
		for _, h := range headings {
			y := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, h) })
			if y < 0 {
				t.Errorf("at %d columns, no row starts %q; drew\n%s", width, h, strings.Join(rows, "\n"))
				continue
			}
			if got := []rune(rows[y]); len(got) < 29 || string(got[:28]) != fmt.Sprintf("%-28s", h) || got[28] == ' ' {
				t.Errorf("at %d columns, %q's row reads %q, want its state from column 28", width, h, rows[y])
			}
		}
	}
	rows := lanesText(lanesFrame(40, 61, Look{}, phoneScene()), doc)
	if !slices.ContainsFunc(rows, func(r string) bool { return strings.HasPrefix(r, "3 side-hustle-consu… ▲") }) {
		t.Errorf("at 40 columns, side-hustle-consulting isn't cut to leave its state room:\n%s", strings.Join(rows, "\n"))
	}
}

// The key sits directly over the key line, as the board's does, its first
// row two over it, the rows left over lying between the lanes and it; where
// the lanes scroll, it gives way.
func TestTheLanesKeyIsOverTheKeyLine(t *testing.T) {
	doc := phoneDoc()
	for height := 20; height <= 70; height += 5 {
		f := lanesFrame(73, height, Look{}, phoneScene())
		rows := lanesText(f, doc)
		_, fits := laneStepOf(f, doc)
		pace, marks := strings.HasPrefix(rows[height-3], "┃ even pace"), rows[height-2] == "◆ primary    ▲ new sessions go here"
		if pace != fits || marks != fits {
			t.Errorf("at 73×%d, fitting %t, the key over the key line: %t, %t; drew\n%s", height, fits, pace, marks, strings.Join(rows, "\n"))
		}
		if y := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "even pace") }); fits && y != height-3 {
			t.Errorf("at 73×%d, the key starts on row %d, want %d", height, y+1, height-2)
		}
	}
}

// As calls come and go, the lanes' form changes only at a step's edge: in a
// view of 45 rows at 73 columns, their windows stacked up to 11 calls on
// side, side by side up to 23, the gaps at their least up to 26, and
// scrolling past; as many calls give the same form whether they came or
// went.
func TestTheLanesFormChangesOnlyAtAStepsEdgeAsCallsComeAndGo(t *testing.T) {
	doc := phoneDoc()
	formOf := func(n int) (int, bool) {
		var calls []views.Call
		for i := range n {
			calls = append(calls, sceneCall("side", fmt.Sprintf("s%03d-session", i), "api", "", idleFor(time.Minute)))
		}
		return laneStepOf(lanesFrame(73, 45, Look{}, calls), doc)
	}
	var counts []int
	for n := range 31 {
		counts = append(counts, n)
	}
	for n := 30; n >= 0; n-- {
		counts = append(counts, n)
	}
	for _, n := range counts {
		want, fits := 2, false
		switch {
		case n <= 11:
			want, fits = 0, true
		case n <= 23:
			want, fits = 1, true
		case n <= 26:
			want, fits = 2, true
		}
		if got, gotFits := formOf(n); got != want || gotFits != fits {
			t.Errorf("with %d calls, the lanes take step %d, fitting %t; want %d, %t", n, got, gotFits, want, fits)
		}
	}
}

// unevenLanes are a document of six accounts, each its windows used apart,
// so no two lanes' rows read alike, and calls on them, as many on each as
// sessions has it, each idle a minute longer than the last.
func unevenLanes(sessions ...int) (status.Document, []views.Call) {
	accounts := make([]status.Account, len(sessions))
	var calls []views.Call
	for i, n := range sessions {
		id := "a" + strconv.Itoa(i+1)
		used := float64(i+1) / 10
		accounts[i] = routedAccount(id, routedWindow("5h", used, routedClock(1, 17, 0)), routedWindow("7d", used/2, routedClock(4, 21, 0)))
		for s := range n {
			session := fmt.Sprintf("%d%d00b6c2-8d3f-4a71-95e4-2c7b0f9a1d68", i+1, s)
			calls = append(calls, sceneCall(id, session, "dir"+strconv.Itoa(len(calls)), "", idleFor(time.Duration(len(calls)+1)*time.Minute)))
		}
	}
	doc := status.Document{GeneratedAt: routedAt, Source: status.SourceRouter, Best: "a1", Router: status.Health{Healthy: true}, Accounts: accounts}
	return doc.WorkedOut(claudeLike), calls
}

// lanesInView are the rows of the frame's view of doc, scrolled as it has
// them, each trimmed at its right.
func lanesInView(f Frame, doc status.Document) []string {
	rows := lanesText(f, doc)
	return rows[f.top() : f.top()+f.viewRows()]
}

// Paged a page at a time, from the top down and from the foot up, the lanes
// show every row they draw whole, whether they scroll an account at a time
// or a row at a time: a page down starts at the first lane or row the page
// before didn't show, and a page up ends at the one above the first it
// showed, so neither way passes over one.
func TestPagingTheLanesDownAndUpShowsEveryRow(t *testing.T) {
	doc, calls := unevenLanes(0, 3, 1, 0, 4, 2)
	paged := map[bool]bool{}
	for _, width := range []int{52, 80} {
		for height := 10; height <= 40; height++ {
			f := lanesFrame(width, height, Look{}, calls)
			b := f.lanesOf(doc, routedAt, f.switchboardLaid(doc, routedAt))
			most := f.Scrolling(doc, routedAt).Most
			if most == 0 {
				continue
			}
			paged[f.ScrollsRows(doc, routedAt)] = true
			whole := textOf(b.laned().c.rows(Look{}))
			for _, way := range []struct {
				name          string
				offset, pages int
			}{{"down", 0, 1}, {"up", most, -1}} {
				reached := make(map[string]bool)
				f.Scroll = way.offset
				for range len(whole) + 1 {
					for _, r := range lanesInView(f, doc) {
						reached[r] = true
					}
					next := f.Scrolling(doc, routedAt).Paged(f.Scroll, way.pages)
					if next == f.Scroll {
						break
					}
					f.Scroll = next
				}
				for _, r := range whole {
					if r != "" && !reached[r] {
						t.Errorf("at %d×%d, paged %s, the lanes never show %q", width, height, way.name, r)
					}
				}
			}
		}
	}
	if !paged[false] || !paged[true] {
		t.Errorf("the lanes paged an account at a time %v and a row at a time %v, want both", paged[false], paged[true])
	}
}
