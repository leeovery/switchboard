package dashboard

import (
	"fmt"
	"maps"
	"slices"
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

// A phone draws the Switchboard in lanes, as sessions-phone-52 draws it, cell
// for cell, in nord, from its heading row to the row over its key line: each
// account's line, its windows on one row, its sessions under it; then the
// pool's rule, Σ pool and the room in each window; then the phone's key.
func TestAPhonesSwitchboardIsDrawnInLanesAsItsFrame(t *testing.T) {
	f := lanesFrame(52, 33, Screen(builtin(t, "nord")), phoneScene())
	drawn := screenOf(t, f.Draw(phoneDoc(), routedAt))
	heldTo(t, "sessions-phone-52", reference(t, "sessions-phone-52"), drawn, region{top: 5, bottom: 31, right: 51}, shadeOf(f.Look.Canvas()), nil, nil)
}

// A session's row says what it's doing at column 20, in a session's page's
// words, bold accent.key while it asks or answers, bold state.destructive
// while refused, text.subtle while throttled, and text.faint while idle;
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
			if got := strings.TrimRight(c.rows(Look{})[y][laneCall.doing:], " "); got != tt.said {
				t.Errorf("c61b's row says %q at column %d, want %q", got, laneCall.doing, tt.said)
			}
			if got := c.at(laneCall.doing, y).ink; got.token != tt.doing.token || got.bold != tt.doing.bold {
				t.Errorf("what c61b's doing is in %+v, want %+v", got, tt.doing)
			}
			if got := c.at(laneCall.id, y).ink; got.token != tt.id.token || got.bold != tt.id.bold {
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

// endsWith is a row width cells wide, left at its start and right ending
// at its last column.
func endsWith(left, right string, width int) string {
	return left + strings.Repeat(" ", width-len([]rune(left))-len([]rune(right))) + right
}

// A lane's windows share one row, as step 3 draws them: 5-hour at column 1,
// its bar from 8, a cell and its percentage; three cells on, Week, its bar 5
// cells after it, a cell and its percentage; each bar ⌊(width − 26) ÷ 2⌋
// cells, ending at the last column where the width leaves no cell over.
func TestALanesWindowsShareARowEachBarHalfWhatTheWidthLeaves(t *testing.T) {
	for _, tt := range []struct{ width, bar int }{{52, 13}, {61, 17}, {80, 27}, {99, 36}} {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			f := lanesFrame(tt.width, 40, Look{}, phoneScene())
			doc := phoneDoc()
			rows := f.At(doc, routedAt).page(doc, routedAt).rows(Look{})
			y := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "2 personal") }) + 1
			row := []rune(rows[y])
			week := laneWindowsAt + laneFirstBar + tt.bar + 1 + percentCells + laneApart
			for _, at := range []struct {
				x    int
				want string
			}{
				{x: laneWindowsAt, want: "5-hour"},
				{x: laneWindowsAt + laneFirstBar + tt.bar + 1, want: " 39%"},
				{x: week, want: "Week"},
				{x: week + laneNextBar + tt.bar + 1, want: " 66%"},
			} {
				if got := string(row[at.x : at.x+len([]rune(at.want))]); got != at.want {
					t.Errorf("column %d reads %q, want %q; the row reads %q", at.x, got, at.want, rows[y])
				}
			}
			if end := week + laneNextBar + tt.bar + 1 + percentCells; strings.TrimRight(rows[y], " ") != string(row[:end]) {
				t.Errorf("the row ends past column %d: %q", end, rows[y])
			}
		})
	}
}

// With one account there's no pool: no rule, no Σ pool and no room, the key
// under a blank row after its lane; its name column fitted to its name, as
// usage fits it.
func TestWithOneAccountTheLanesHaveNoPool(t *testing.T) {
	doc := usageWorld(extraOff, func(d *status.Document) { d.Accounts, d.Best = d.Accounts[:1], "work" })
	f := lanesFrame(52, 33, Look{}, []views.Call{sceneCall("work", idC61B, "mobile", "", idleFor(time.Minute))})
	rows := lanesText(f, doc)
	top := f.top()
	want := []string{"1 work ▲    ● at its cap · 17:10", "", "  c61b mobile       idle 1m", "", "┃ even pace: where you would be now, used evenly"}
	got := slices.Clone(rows[top : top+len(want)])
	got[1] = "" // the windows' row, held by the test of a lane's windows
	if !slices.Equal(got, want) {
		t.Errorf("drew\n%s\nwant\n%s", strings.Join(rows[top:top+len(want)], "\n"), strings.Join(want, "\n"))
	}
	if slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, "Σ pool") || strings.HasPrefix(r, "─") }) {
		t.Errorf("drew the pool, with one account:\n%s", strings.Join(rows, "\n"))
	}
}

// From 100 columns, where the bent cords outnumber the bends' room, the
// Switchboard draws lanes under the wide heading: the bars half what the
// width leaves, and the wide key, its marks four cells apart.
func TestWhereBentCordsOutnumberTheRoomTheSwitchboardDrawsLanes(t *testing.T) {
	var calls []views.Call
	for i := range 25 {
		calls = append(calls, sceneCall("side", fmt.Sprintf("s%03d-session", i), "api", "", idleFor(time.Minute)))
	}
	doc := phoneDoc()
	f := lanesFrame(160, 60, Look{}, calls)
	if !f.switchboardLaid(doc, routedAt).lanes {
		t.Fatalf("25 bent cords at 160 columns are drawn as cords, want lanes")
	}
	rows := lanesText(f, doc)
	top := f.top()
	for _, want := range []struct {
		y    int
		says string
	}{
		{y: top, says: "1 work          ● at its cap · 17:10"},
		{y: top + 8, says: "  s000 api          idle 1m"},
	} {
		if got := rows[want.y]; got != want.says {
			t.Errorf("row %d reads %q, want %q", want.y+1, got, want.says)
		}
	}
	bar := (160 - 26) / 2
	if got := []rune(rows[top+1])[laneWindowsAt+laneFirstBar+bar+1:][:4]; string(got) != " 95%" {
		t.Errorf("work's 5-hour percentage reads %q, want a bar of %d before it", string(got), bar)
	}
	if !slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, "◆ primary    ▲ new sessions go here") }) {
		t.Errorf("the key isn't the wide one:\n%s", strings.Join(rows, "\n"))
	}
	if !strings.Contains(rows[f.headingY()], "[ ]") {
		t.Errorf("the heading row reads %q, want the wide one", rows[f.headingY()])
	}
}

// Where the lanes don't fit the view with the foot under them, the foot gives
// way and they scroll an account at a time, its line and its sessions
// together: ▲ and how many accounts are above on the view's first row, while
// any are, and ▼ and how many below, right-aligned on the row over the key
// line, no further than shows the last account.
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
		{offset: 2, scrolling: Scrolling{Most: 2, Page: 1}, first: "▲ 2 more accounts"},
		{offset: 5, scrolling: Scrolling{Most: 2, Page: 1}, first: "▲ 2 more accounts"},
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
				t.Errorf("the foot shows while the lanes scroll:\n%s", strings.Join(rows, "\n"))
			}
		})
	}
	if got := lanesFrame(52, 33, Look{}, phoneScene()).Scrolling(doc, routedAt); got != (Scrolling{}) {
		t.Errorf("the lanes that fit scroll %+v, want nowhere", got)
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
		{Y: top + 5, Width: 52, On: OnSession, Session: id41E0, Account: "personal"},
		{Y: top + 6, Width: 52, On: OnSession, Session: id9C2E, Account: "personal"},
		{Y: top + 10, Width: 52, On: OnSession, Session: idC61B, Account: "side"},
		{Y: top + 11, Width: 52, On: OnSession, Session: idD28C, Account: "side"},
		{Y: top + 12, Width: 52, On: OnSession, Session: id7F3A, Account: "side"},
		{Y: top + 13, Width: 52, On: OnSession, Session: idDB8A, Account: "side"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("the sessions' targets are\n%+v\nwant\n%+v", got, want)
	}
	c := f.At(doc, routedAt).page(doc, routedAt)
	for y := top + 10; y <= top+13; y++ {
		for _, x := range []int{0, 2, 51} {
			if lit := c.at(x, y).ink.on.token == theme.BgSelection; lit != (y == top+12) {
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
