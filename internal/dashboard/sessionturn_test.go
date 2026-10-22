package dashboard

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// turnRequest is a request of a turn opened as a frame's row has it: when it
// came, at the frames' clock, to the second; its model and account; its
// first byte and its end, in tenths of a second; the tokens it read, wrote
// and put out; and why its answer stopped, and its tool.
type turnRequest struct {
	hour, minute, second int
	model, account       string
	first, took          int
	read, written, out   int
	stop, tool           string
}

// line is the Log's line of the request, as a turn opened holds it, with no
// mark.
func (r turnRequest) line() views.LogLine {
	tenths := func(n int) *time.Duration {
		d := time.Duration(n) * time.Second / 10
		return &d
	}
	return views.LogLine{Row: views.RequestRow{
		Request: fmt.Sprintf("%02d%02d%02d", r.hour, r.minute, r.second),
		At:      time.Date(2026, 10, 1, r.hour, r.minute, r.second, 0, sessionNow.Location()),
		Session: moved().Page.Session, Model: r.model, Account: r.account, Status: 200,
		First: tenths(r.first), Took: tenths(r.took), Read: r.read, Written: r.written, Out: r.out, Metered: true,
		Stop: r.stop, Tool: r.tool, Mark: views.NoMark,
	}}
}

// turnOpened is d28c's page as session-turn-opened-160 draws it: its turn
// 12 chosen and open, its 37 requests the frame's five, the move at work's
// cap among them, then 32 more on side.
func turnOpened() *SessionShown {
	shown := moved()
	shown.Turn, shown.Open = 12, 12
	rows := []turnRequest{
		{14, 29, 4, opus, "work", 8, 112, 176_000, 2100, 1300, "tool_use", "Bash"},
		{14, 30, 42, opus, "work", 9, 124, 181_000, 2400, 1100, "tool_use", "Bash"},
		{14, 31, 13, opus, "side", 29, 146, 0, 182_000, 1200, "tool_use", "Bash"},
		{14, 31, 44, haiku, "side", 3, 6, 0, 0, 40, "end_turn", ""},
		{14, 31, 58, opus, "side", 11, 98, 182_000, 2600, 900, "tool_use", "Edit"},
	}
	for i := range 32 {
		rows = append(rows, turnRequest{14, 32 + i/6, i % 6 * 10, opus, "side", 10, 90, 183_000, 2000, 1000, "tool_use", "Read"})
	}
	for _, r := range rows {
		shown.Requests = append(shown.Requests, r.line())
	}
	move := &shown.Requests[2].Row
	move.Rewrote = true
	move.Routing = views.RoutingSaid{
		Words: status.RoutingWords(status.ReasonMovedOff+"work "+status.WhyReserve, status.Facts{From: "work", To: "side"}),
		From:  "work", Cost: worthOf(182).Amount,
	}
	return shown
}

// A turn opened lays its rows out under its own, in place, the turns under
// it moving down: its heads; its requests, as many as the layout shows at a
// time, as far as they're scrolled; and its foot, the rows of another turn
// none of them. Without a turn open, a row a turn.
func TestATurnOpenedLaysItsRowsOutUnderItsOwn(t *testing.T) {
	turn := func(i int) turnsRow { return turnsRow{kind: aTurn, index: i} }
	request := func(i int) turnsRow { return turnsRow{kind: openRequest, index: i} }
	heads, foot := turnsRow{kind: openHeads}, turnsRow{kind: openFoot}
	tests := []struct {
		name   string
		open   func(*SessionShown)
		height int
		want   []turnsRow
	}{
		{name: "none open", open: func(s *SessionShown) { s.Open = 0 }, height: 37, want: []turnsRow{turn(0), turn(1), turn(2)}},
		{name: "the second open, five shown of 37", height: 37, want: []turnsRow{
			turn(0), turn(1), heads, request(0), request(1), request(2), request(3), request(4), foot, turn(2),
		}},
		{name: "scrolled through to its last", open: func(s *SessionShown) { s.RequestsScroll = 40 }, height: 37, want: []turnsRow{
			turn(0), turn(1), heads, request(32), request(33), request(34), request(35), request(36), foot, turn(2),
		}},
		{name: "two of its requests, both shown", open: func(s *SessionShown) { s.Requests = s.Requests[:2] }, height: 37, want: []turnsRow{
			turn(0), turn(1), heads, request(0), request(1), foot, turn(2),
		}},
		{name: "at the least height, one shown", height: 29, want: []turnsRow{turn(0), turn(1), heads, request(0), foot, turn(2)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := turnOpened()
			shown.Page.Turns = shown.Page.Turns[:3]
			if tt.open != nil {
				tt.open(shown)
			}
			if got := shown.turnsRows(pageFrame(t, shown, 160, tt.height).layoutOf(1)); !slices.Equal(got, tt.want) {
				t.Errorf("the turns' rows are\n%v\nwant\n%v", got, tt.want)
			}
		})
	}
}

// A turn opened draws session-turn-opened-160's rows of it, as the frame
// test holds them, but its first request's, chosen as it opens, which the
// frame doesn't light, held here by its text; lit across the width as the
// Log's line chosen is, the turn open lit as it was chosen; and its foot
// saying how many of its requests are out of view either way.
func TestATurnOpenedLightsItsRequestChosenAndSaysWhatsOutOfView(t *testing.T) {
	tests := []struct {
		name            string
		request, scroll int
		lit             int
		foot            string
	}{
		{name: "the first", lit: 28, foot: "  └  ▼ 32 more of its requests   ·   j/k through them"},
		{name: "the third", request: 2, lit: 30, foot: "  └  ▼ 32 more of its requests   ·   j/k through them"},
		{name: "scrolled", request: 4, scroll: 3, lit: 29, foot: "  └  ▲ 3 more of its requests   ·   ▼ 29 more of its requests   ·   j/k through them"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := turnOpened()
			shown.Request, shown.RequestsScroll = tt.request, tt.scroll
			f := pageFrame(t, shown, 160, 37)
			c := pageCanvas(f)
			for y := 25; y < 35; y++ {
				if lit := c.cells[y][150].ink.on == litInk; lit != (y == tt.lit) {
					t.Errorf("row %d is lit across the width %t, want %t", y+1, lit, y == tt.lit)
				}
			}
			if !c.cells[26][100].ink.bold || c.cells[26][0].ink.on != chosenTurn {
				t.Error("the turn open isn't lit as it's chosen")
			}
			rows := pageText(f)
			if got := rows[33]; got != tt.foot {
				t.Errorf("its foot reads %q, want %q", got, tt.foot)
			}
			if y, ok := f.turnLitRow(); !ok || y != tt.lit {
				t.Errorf("the request chosen is drawn on row %d, %t; want row %d", y+1, ok, tt.lit+1)
			}
		})
	}
}

// A turn opened draws its requests as the Log's rows, without their session,
// in the page's columns, from 5, at every width: routing whole from 112 at
// 160 and cut to the room left to its least, 24, at 136, under that folded
// into stopped, and taking its share of the room past 160 as the Log's does;
// narrower, its columns leaving as the Log's do. Its heads are the Log's, in
// text.faint.
func TestATurnOpenedsRequestsAreTheLogsRowsInThePagesColumns(t *testing.T) {
	tests := []struct {
		width int
		heads string
		move  string
	}{
		{width: 160,
			heads: "     at        model      on        status  first    took    read  written    out  stopped                      routing",
			move:  "  │  14:31:13  opus-5-5   side      200      2.9s   14.6s       —     182k   1.2k  tool_use · Bash              from work, at its cap  $1.82"},
		{width: 136,
			heads: "     at        model      on        status  first    took    read  written    out  stopped                      routing",
			move:  "  │  14:31:13  opus-5-5   side      200      2.9s   14.6s       —     182k   1.2k  tool_use · Bash              from work, at it…  $1.82"},
		{width: 135,
			heads: "     at        model      on        status  first    took    read  written    out  stopped",
			move:  "  │  14:31:13  opus-5-5   side      200      2.9s   14.6s       —     182k   1.2k  from work, at its cap  $1.82"},
		{width: 200,
			heads: "     at        model      on        status  first    took    read  written    out  stopped                                          routing",
			move:  "  │  14:31:13  opus-5-5   side      200      2.9s   14.6s       —     182k   1.2k  tool_use · Bash                                  from work, at its cap  $1.82"},
		{width: 121,
			heads: "     at        model      on        status  first    took    read  written    out  stopped",
			move:  "  │  14:31:13  opus-5-5   side      200      2.9s   14.6s       —     182k   1.2k  from work, at its cap  $1.82"},
		{width: 52, heads: "     at        on        status    took  stopped", move: "  │  14:31:13  side      200      14.6s  from work"},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.width), func(t *testing.T) {
			shown := turnOpened()
			shown.Request = 0
			f := pageFrame(t, shown, tt.width, 37)
			l := f.layoutOf(2)
			heads, move := l.turnsAt+2, l.turnsAt+5
			rows := pageText(f)
			if rows[heads] != tt.heads || rows[move] != tt.move {
				t.Errorf("at %d, the heads and the move read\n%q\n%q\nwant\n%q\n%q", tt.width, rows[heads], rows[move], tt.heads, tt.move)
			}
			c := pageCanvas(f)
			if k := c.cells[heads][5].ink; k != faintInk {
				t.Errorf("the heads are in %+v, want text.faint", k)
			}
			if k := c.cells[move][2].ink; k != borderInk {
				t.Errorf("the tree is in %+v, want border", k)
			}
		})
	}
}

// A turn open's request in flight is drawn as the Log's row of it is, as it
// stands at the moment it's drawn, its rows built once: its took counting on
// to that moment, and answering, its characters as the stream the frame
// holds then counts them.
func TestATurnOpensRequestInFlightIsDrawnAsItStands(t *testing.T) {
	sent := sessionNow.Add(-20 * time.Second)
	answering := views.Told{
		Request: "0a000001", Session: moved().Page.Session, Model: opus, Account: "side",
		Sent: sent, First: sent.Add(time.Second), Chars: 1000, Heard: sent,
	}
	streamed := func(chars int) views.Stream {
		told := answering
		told.Chars = chars
		return views.Stream{Live: true, Requests: map[string]views.Told{told.Request: told}}
	}
	shown := turnOpened()
	shown.Requests[0] = views.LogRows(views.LogSources{From: sent, Now: sessionNow, Stream: streamed(1000)}).Line(0)
	for _, tt := range []struct {
		at    time.Duration
		chars int
	}{{at: 20 * time.Second, chars: 1000}, {at: 21500 * time.Millisecond, chars: 5100}, {at: 75 * time.Second, chars: 9900}} {
		f := pageFrame(t, shown, 160, 37)
		f.Stream = streamed(tt.chars)
		rows := textOf(f.Draw(pageDoc(), sent.Add(tt.at)))
		row := rows[f.layoutOf(2).turnsAt+3]
		for _, want := range []string{prose.Seconds(tt.at), "answering · " + prose.Characters(tt.chars) + " characters"} {
			if !strings.Contains(row, want) {
				t.Errorf("drawn %s after it was sent, %d characters streamed, the turn open's row in flight doesn't read %q:\n%s", tt.at, tt.chars, want, row)
			}
		}
	}
}

// A click on a request of the turn open shown chooses it; its heads and its
// foot take none.
func TestATurnOpenedsRequestsShownAreTargets(t *testing.T) {
	shown := turnOpened()
	shown.RequestsScroll = 3
	f := pageFrame(t, shown, 160, 37)
	var got []string
	for _, target := range f.Targets(pageDoc(), sessionNow) {
		if target.Y >= 25 && target.Y < 35 {
			got = append(got, fmt.Sprintf("%d %v %d %d", target.Y, target.On, target.Turn, target.Row))
		}
	}
	want := []string{
		fmt.Sprintf("25 %v 13 0", OnTurn), fmt.Sprintf("26 %v 12 0", OnTurn),
		fmt.Sprintf("28 %v 0 3", OnRequest), fmt.Sprintf("29 %v 0 4", OnRequest), fmt.Sprintf("30 %v 0 5", OnRequest),
		fmt.Sprintf("31 %v 0 6", OnRequest), fmt.Sprintf("32 %v 0 7", OnRequest),
		fmt.Sprintf("34 %v 11 0", OnTurn),
	}
	if !slices.Equal(got, want) {
		t.Errorf("the turns' targets are\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// The turns scroll a row at a time over the turn open's rows too, saying how
// many turns, not rows, are out of view; the turn open's requests scroll
// within their own rows, five at a time.
func TestATurnOpenedScrollsWithTheTurns(t *testing.T) {
	shown := turnOpened()
	shown.Scroll = 3
	f := pageFrame(t, shown, 160, 37)
	if got, want := f.Scrolling(pageDoc(), sessionNow), (Scrolling{Most: 10, Page: 10}); got != want {
		t.Errorf("the turns scroll %+v, want %+v: thirteen turns and seven rows of the one open, in ten", got, want)
	}
	if got, want := f.RequestsScrolling(), (Scrolling{Most: 32, Page: 5}); got != want {
		t.Errorf("its requests scroll %+v, want %+v", got, want)
	}
	if got, want := pageText(f)[35], "▲ 2 more turns   ·   ▼ 7 more turns   ·   j/k to scroll"; got != want {
		t.Errorf("under the turns, %q; want %q", got, want)
	}
}

// The Log's details card opens over a session's page by the turn open's
// request chosen: placed over its line, a row between, the line kept bright
// as the page fades, and without s, as its session's page is the one it's
// over (call 63); centred where it fits by its line on neither side, the
// page's turns lifting for no card, where the Log's table would.
func TestTheLogsCardOverASessionsPageIsPlacedByItsRequestChosen(t *testing.T) {
	tests := []struct {
		name            string
		request, scroll int
		width, height   int
		placed          bool
		line            int
	}{
		{name: "over its line", request: 2, width: 160, height: 37, placed: true, line: 30},
		{name: "too short for it", scroll: 1, width: 160, height: 24, line: 19},
		{name: "a tall phone, its card stacked", width: 52, height: 64, line: 28},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := turnOpened()
			shown.Request, shown.Scroll = tt.request, tt.scroll
			f := pageFrame(t, shown, tt.width, tt.height)
			details, _ := shown.Requests[tt.request].Details()
			page, c := f.LogFitted(LogCard{Request: &details})
			if !reflect.DeepEqual(page, f.Log) {
				t.Error("the card over the page lifted the Log under it, want it as it stands")
			}
			f.Card = c
			card := c.card(f.At(pageDoc(), sessionNow))
			if card.of == nil || card.of.row != tt.line || card.of.placed != tt.placed {
				t.Fatalf("the card is of %+v, want of row %d, placed %t", card.of, tt.line+1, tt.placed)
			}
			if tt.placed && card.of.top+len(card.body)+chrome(true) != tt.line-1 {
				t.Errorf("the card ends on row %d, want it over its line, a row between, on row %d", card.of.top+len(card.body)+chrome(true), tt.line-1)
			}
			if keys := keyRow(card.keys).plain(); keys != "↑↓ previous/next request   esc close" {
				t.Errorf("its keys read %q, want no s", keys)
			}
		})
	}
}
