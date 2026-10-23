package dashboard

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionNow is when the frames of a session's page are drawn: Thursday 1
// October 2026, 14:42:07, in a zone an hour east of UTC.
var sessionNow = time.Date(2026, 10, 1, 14, 42, 7, 0, time.FixedZone("BST", 60*60))

// at is the time on the day the frames are drawn, at the clock given, or
// the day before, where yesterday says.
func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 1, hour, minute, 0, 0, sessionNow.Location())
}

func yesterdayAt(hour, minute int) time.Time {
	return at(hour, minute).AddDate(0, 0, -1)
}

// worthOf is a worth of the cents given.
func worthOf(cents int) views.Worth {
	amount := ledger.Picodollars(cents) * 10_000_000_000
	return views.Worth{Amount: &amount}
}

// pageWindow is a window of the frames' accounts, used so far, resetting
// three hours after the frames are drawn.
func pageWindow(key, label string, used float64) quota.Window {
	return quota.Window{Key: key, Label: label, Utilization: used, ResetsAt: sessionNow.Add(3 * time.Hour).UTC()}
}

// pageAccount is an account of the frames, labelled as its id, its windows
// read a minute before they're drawn: the 5-hour, the week and Fable's week
// used as given.
func pageAccount(id string, session, week, fable float64) status.Account {
	return status.Account{
		ID: id, Label: id, TokenSet: true, FetchedAt: sessionNow.Add(-time.Minute).UTC(),
		Windows: []quota.Window{pageWindow("5h", "Session", session), pageWindow("7d", "Week", week), pageWindow("7d_oi", "Fable week", fable)},
	}
}

// pageDoc is the router's document as the frames draw it: work, personal,
// the primary, and side, where new sessions go.
func pageDoc() status.Document {
	personal := pageAccount("personal", 0.39, 0.66, 0)
	personal.Primary = true
	return status.Document{
		GeneratedAt: sessionNow.UTC(), Source: status.SourceRouter, Best: "side", Router: status.Health{Healthy: true}, Sessions: 6,
		Accounts: []status.Account{pageAccount("work", 0.95, 0.34, 0.12), personal, pageAccount("side", 0.13, 0.78, 0.04)},
	}.WorkedOut(claudeLike)
}

// turnRow is a turn as a frame's row has it: its number, its start, how
// many minutes it took, none while it's going, its requests, its tools, as
// pairs of a tool and its times, the tokens it read, wrote and put out, its
// worth in cents, and the accounts it went to.
type turnRow struct {
	n                  int
	started            time.Time
	took               int
	requests           int
	tools              []any
	read, written, out int
	cents              int
	on                 []string
}

// turn is the turn the row gives, at the frames' now.
func (r turnRow) turn() views.Turn {
	t := views.Turn{Turn: r.n, Started: r.started, Requests: r.requests, Read: r.read, Written: r.written, Out: r.out, Worth: worthOf(r.cents), Accounts: r.on}
	if r.took > 0 {
		t.Ended = r.started.Add(time.Duration(r.took-1) * time.Minute)
	}
	for i := 0; i < len(r.tools); i += 2 {
		t.Tools = append(t.Tools, views.ToolCalls{Tool: r.tools[i].(string), Times: r.tools[i+1].(int)})
	}
	return t
}

// turnsOf are the turns of the rows, newest first.
func turnsOf(rows ...turnRow) []views.Turn {
	turns := make([]views.Turn, len(rows))
	for i, r := range rows {
		turns[i] = r.turn()
	}
	return turns
}

// moved is d28c's page as session-moved-160 draws it: asking, on side
// since its move off work at its cap at 14:31.
func moved() *SessionShown {
	page := views.SessionPage{
		Session: "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13", Dir: "/home/dev/Code/switchboard", Account: "side", Model: opus,
		Started: at(12, 2), Activity: views.Activity{Doing: views.Asking, Since: sessionNow.Add(-2 * time.Second)},
		Goes: []views.SessionModel{{Model: opus, Account: "side"}, {Model: haiku, Account: "side"}},
		Moves: []views.SessionMove{{
			At: at(14, 31), Model: opus, From: "work", To: "side", Reason: status.ReasonMovedOff + "work " + status.WhyReserve,
			Written: 182_000, Cost: worthOf(182).Amount, Rewrote: true,
		}},
		Accounts: []views.SessionAccount{
			{Account: "work", From: at(12, 2), To: at(14, 31), Requests: 404, Worth: worthOf(4106), Points: map[string]int{"5h": 41, "7d": 6}},
			{Account: "side", From: at(14, 31), To: at(14, 42), Requests: 41, Worth: worthOf(714), Points: map[string]int{"5h": 9, "7d": 1}},
		},
		Turns: turnsOf(
			turnRow{13, at(14, 40), 0, 6, []any{"Read", 3, "Bash", 2}, 958_000, 18_000, 5_400, 79, []string{"side"}},
			turnRow{12, at(14, 29), 9, 37, []any{"Bash", 14, "Edit", 9, "Read", 8}, 5_700_000, 293_000, 33_000, 661, []string{"work", "side"}},
			turnRow{11, at(14, 15), 15, 58, []any{"Bash", 20, "Edit", 13, "Read", 13}, 8_400_000, 174_000, 52_000, 726, []string{"work"}},
			turnRow{10, at(14, 0), 12, 52, []any{"Edit", 16, "Bash", 14, "Read", 12}, 6_900_000, 156_000, 47_000, 619, []string{"work"}},
			turnRow{9, at(13, 50), 5, 17, []any{"Bash", 7, "Read", 5}, 2_100_000, 51_000, 15_000, 194, []string{"work"}},
			turnRow{8, at(13, 37), 8, 31, []any{"Read", 11, "Edit", 8, "Bash", 6}, 3_500_000, 93_000, 28_000, 340, []string{"work"}},
			turnRow{7, at(13, 12), 19, 83, []any{"Bash", 27, "Edit", 21, "Read", 17}, 8_300_000, 249_000, 75_000, 849, []string{"work"}},
			turnRow{6, at(13, 6), 2, 5, []any{"Read", 2}, 444_000, 15_000, 4_500, 48, []string{"work"}},
			turnRow{5, at(12, 54), 6, 22, []any{"Read", 8, "Edit", 6, "Bash", 3}, 1_800_000, 66_000, 20_000, 205, []string{"work"}},
			turnRow{4, at(12, 35), 14, 61, []any{"Bash", 19, "Edit", 15, "Read", 13}, 4_300_000, 183_000, 55_000, 534, []string{"work"}},
			turnRow{3, at(12, 25), 8, 30, []any{"Read", 9}, 2_000_000, 80_000, 24_000, 300, []string{"work"}},
			turnRow{2, at(12, 12), 10, 40, []any{"Edit", 12}, 3_000_000, 100_000, 30_000, 400, []string{"work"}},
			turnRow{1, at(12, 2), 9, 33, []any{"Bash", 10}, 2_500_000, 90_000, 27_000, 360, []string{"work"}},
		),
		Totals: views.SessionTotals{Turns: 13, Requests: 445, FromCache: 0.97, Worth: worthOf(4820)},
	}
	return &SessionShown{Page: page, Turn: 13, Home: "/home/dev"}
}

// quiet is 9c2e's page as session-quiet-160 draws it: idle 9 minutes on
// personal, where it started, its ten turns all in view.
func quiet() *SessionShown {
	page := views.SessionPage{
		Session: "9c2e41b7-0d6a-4c3e-9f18-2b7a5e0c8d94", Dir: "/home/dev/Code/website", Account: "personal", Model: opus,
		Started: at(11, 55), Activity: views.Activity{Doing: views.Idle, Since: sessionNow.Add(-9 * time.Minute)},
		Goes: []views.SessionModel{{Model: opus, Account: "personal", Reason: status.ReasonSticky}},
		Accounts: []views.SessionAccount{
			{Account: "personal", From: at(11, 55), To: at(14, 33), Requests: 267, Worth: worthOf(2715), Points: map[string]int{"5h": 23, "7d": 4}},
		},
		Turns: turnsOf(
			turnRow{10, at(14, 29), 4, 16, []any{"Edit", 5, "Bash", 4, "Read", 3}, 2_500_000, 48_000, 14_000, 210, []string{"personal"}},
			turnRow{9, at(14, 20), 3, 9, []any{"Bash", 3, "Edit", 2, "Read", 1}, 1_300_000, 27_000, 8_100, 115, []string{"personal"}},
			turnRow{8, at(13, 56), 14, 70, []any{"Edit", 23, "Bash", 17, "Read", 14}, 9_500_000, 210_000, 63_000, 844, []string{"personal"}},
			turnRow{7, at(13, 41), 5, 20, []any{"Bash", 6, "Edit", 5, "Read", 4}, 2_400_000, 60_000, 18_000, 226, []string{"personal"}},
			turnRow{6, at(13, 22), 9, 27, []any{"Edit", 9, "Bash", 6, "Read", 5}, 2_900_000, 81_000, 24_000, 288, []string{"personal"}},
			turnRow{5, at(13, 14), 2, 6, []any{"Bash", 2, "Edit", 1, "Read", 1}, 600_000, 18_000, 5_400, 62, []string{"personal"}},
			turnRow{4, at(12, 56), 2, 6, []any{"Edit", 2, "Bash", 1, "Read", 1}, 519_000, 18_000, 5_400, 57, []string{"personal"}},
			turnRow{3, at(12, 32), 10, 30, []any{"Bash", 10, "Edit", 7, "Read", 6}, 2_100_000, 90_000, 27_000, 265, []string{"personal"}},
			turnRow{2, at(12, 16), 8, 32, []any{"Edit", 10, "Bash", 8, "Read", 6}, 1_900_000, 96_000, 29_000, 262, []string{"personal"}},
			turnRow{1, at(11, 55), 17, 51, []any{"Bash", 17, "Edit", 12, "Read", 10}, 2_400_000, 153_000, 46_000, 386, []string{"personal"}},
		),
		Totals: views.SessionTotals{Turns: 10, Requests: 267, FromCache: 0.97, Worth: worthOf(2715)},
	}
	return &SessionShown{Page: page, Turn: 10, Home: "/home/dev"}
}

// ended is b9c4's page as session-ended-160 draws it: over since 18:47
// yesterday, having moved off work at its cap at 16:02.
func ended() *SessionShown {
	page := views.SessionPage{
		Session: "b9c40d2e-5f71-4a8b-b3c6-91e7d0f4a258", Dir: "/home/dev/Code/switchboard", Account: "side", Model: opus,
		Started: yesterdayAt(13, 20), Activity: views.Activity{Doing: views.Ended, Since: yesterdayAt(18, 47)}, KeptUntil: "2026-12-29",
		Goes: []views.SessionModel{{Model: opus, Account: "side"}, {Model: haiku, Account: "side"}},
		Moves: []views.SessionMove{{
			At: yesterdayAt(16, 2), Model: opus, From: "work", To: "side", Reason: status.ReasonMovedOff + "work " + status.WhyReserve,
			Written: 188_000, Cost: worthOf(188).Amount, Rewrote: true,
		}},
		Accounts: []views.SessionAccount{
			{Account: "work", From: yesterdayAt(13, 20), To: yesterdayAt(16, 2), Requests: 330, Worth: worthOf(3251), Points: map[string]int{"5h": 52, "7d": 8}},
			{Account: "side", From: yesterdayAt(16, 2), To: yesterdayAt(18, 47), Requests: 265, Worth: worthOf(4049), Points: map[string]int{"5h": 38, "7d": 6}},
		},
		Turns: turnsOf(
			turnRow{16, yesterdayAt(18, 36), 11, 44, []any{"Edit", 14, "Bash", 11, "Read", 8}, 8_400_000, 132_000, 40_000, 649, []string{"side"}},
			turnRow{15, yesterdayAt(17, 55), 15, 45, []any{"Bash", 15, "Edit", 11, "Read", 9}, 8_600_000, 135_000, 40_000, 664, []string{"side"}},
			turnRow{14, yesterdayAt(17, 31), 14, 42, []any{"Edit", 14, "Bash", 10, "Read", 8}, 8_000_000, 126_000, 38_000, 620, []string{"side"}},
			turnRow{13, yesterdayAt(17, 16), 3, 15, []any{"Bash", 5, "Edit", 3, "Read", 3}, 2_900_000, 45_000, 14_000, 221, []string{"side"}},
			turnRow{12, yesterdayAt(17, 9), 3, 12, []any{"Edit", 4, "Bash", 3, "Read", 2}, 2_300_000, 36_000, 11_000, 177, []string{"side"}},
			turnRow{11, yesterdayAt(16, 41), 12, 36, []any{"Bash", 12, "Edit", 9, "Read", 7}, 6_800_000, 108_000, 32_000, 531, []string{"side"}},
			turnRow{10, yesterdayAt(16, 22), 11, 55, []any{"Edit", 18, "Bash", 13, "Read", 11}, 9_900_000, 165_000, 50_000, 786, []string{"side"}},
			turnRow{9, yesterdayAt(15, 58), 8, 32, []any{"Bash", 10, "Edit", 8, "Read", 6}, 5_200_000, 284_000, 29_000, 614, []string{"work", "side"}},
			turnRow{8, yesterdayAt(15, 42), 2, 8, []any{"Edit", 2, "Bash", 2, "Read", 1}, 1_200_000, 24_000, 7_200, 101, []string{"work"}},
			turnRow{7, yesterdayAt(15, 24), 4, 20, []any{"Bash", 6, "Edit", 5, "Read", 4}, 2_700_000, 60_000, 18_000, 240, []string{"work"}},
			turnRow{6, yesterdayAt(15, 0), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"work"}},
			turnRow{5, yesterdayAt(14, 40), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"work"}},
			turnRow{4, yesterdayAt(14, 20), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"work"}},
			turnRow{3, yesterdayAt(14, 0), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"work"}},
			turnRow{2, yesterdayAt(13, 40), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"work"}},
			turnRow{1, yesterdayAt(13, 20), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"work"}},
		),
		Totals: views.SessionTotals{Turns: 16, Requests: 595, FromCache: 0.97, Worth: worthOf(7300)},
	}
	return &SessionShown{Page: page, Turn: 16, Home: "/home/dev"}
}

// resumed is f41a's page as session-resumed-160 draws it: answering on
// personal, where it was rescored after a night's idle on side.
func resumed() *SessionShown {
	page := views.SessionPage{
		Session: "f41a7c3d-2e8b-4d05-a6f9-0c3b1e8d7a52", Dir: "/home/dev/Code/api", Account: "personal", Model: opus,
		Started: yesterdayAt(16, 40), Resumed: at(9, 12), Activity: views.Activity{Doing: views.Answering, Since: sessionNow.Add(-5 * time.Second)},
		Goes:  []views.SessionModel{{Model: opus, Account: "personal"}},
		Moves: []views.SessionMove{{At: at(9, 12), Model: opus, From: "side", To: "personal", Reason: status.Rescored(at(9, 12).Sub(yesterdayAt(18, 5)))}},
		Accounts: []views.SessionAccount{
			{Account: "side", From: yesterdayAt(16, 40), To: yesterdayAt(18, 5), Requests: 154, Worth: worthOf(1408), Points: map[string]int{"5h": 18, "7d": 3}},
			{Account: "personal", From: at(9, 12), To: at(14, 42), Requests: 767, Worth: worthOf(11410), Points: map[string]int{"5h": 31, "7d": 9}},
		},
		Turns: turnsOf(
			turnRow{21, at(14, 25), 0, 44, []any{"Bash", 14, "Edit", 11, "Read", 8}, 8_400_000, 132_000, 40_000, 649, []string{"personal"}},
			turnRow{20, at(13, 58), 17, 85, []any{"Edit", 28, "Bash", 21, "Read", 17}, 16_100_000, 255_000, 76_000, 1254, []string{"personal"}},
			turnRow{19, at(13, 37), 9, 27, []any{"Bash", 9, "Edit", 6, "Read", 5}, 5_100_000, 81_000, 24_000, 398, []string{"personal"}},
			turnRow{18, at(13, 17), 16, 48, []any{"Edit", 16, "Bash", 12, "Read", 9}, 9_100_000, 144_000, 43_000, 708, []string{"personal"}},
			turnRow{17, at(13, 1), 6, 18, []any{"Bash", 6, "Edit", 4, "Read", 3}, 3_400_000, 54_000, 16_000, 266, []string{"personal"}},
			turnRow{16, at(12, 44), 9, 27, []any{"Edit", 9, "Bash", 6, "Read", 5}, 5_100_000, 81_000, 24_000, 398, []string{"personal"}},
			turnRow{15, at(12, 18), 14, 42, []any{"Bash", 14, "Edit", 10, "Read", 8}, 8_000_000, 126_000, 38_000, 620, []string{"personal"}},
			turnRow{14, at(11, 55), 15, 75, []any{"Edit", 25, "Bash", 18, "Read", 15}, 14_200_000, 225_000, 68_000, 1106, []string{"personal"}},
			turnRow{13, at(11, 34), 13, 39, []any{"Bash", 13, "Edit", 9, "Read", 7}, 7_400_000, 117_000, 35_000, 575, []string{"personal"}},
			turnRow{12, at(11, 24), 4, 20, []any{"Edit", 6, "Bash", 5, "Read", 4}, 3_800_000, 60_000, 18_000, 295, []string{"personal"}},
			turnRow{11, at(11, 0), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"personal"}},
			turnRow{10, at(10, 40), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"personal"}},
			turnRow{9, at(10, 20), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"personal"}},
			turnRow{8, at(10, 0), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"personal"}},
			turnRow{7, at(9, 40), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"personal"}},
			turnRow{6, at(9, 12), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"personal"}},
			turnRow{5, yesterdayAt(17, 50), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"side"}},
			turnRow{4, yesterdayAt(17, 30), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"side"}},
			turnRow{3, yesterdayAt(17, 10), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"side"}},
			turnRow{2, yesterdayAt(16, 55), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"side"}},
			turnRow{1, yesterdayAt(16, 40), 9, 30, []any{"Read", 9}, 2_000_000, 50_000, 15_000, 200, []string{"side"}},
		),
		Totals: views.SessionTotals{Turns: 21, Requests: 921, FromCache: 0.97, Worth: worthOf(12819)},
	}
	return &SessionShown{Page: page, Turn: 21, Home: "/home/dev"}
}

// The row under a session's turns counts the turns out of view above them
// wherever both counts fit, its keys given way first, at every width from
// 30 columns to 220: "▲ 1 more turn   ·   ▼ 7 more turns" at 52.
func TestASessionsTurnsCountThoseAboveWhereverBothCountsFit(t *testing.T) {
	for width := 30; width <= 220; width++ {
		shown := moved()
		shown.Turn, shown.Scroll = 12, 1
		f := pageFrame(t, shown, width, 32)
		rows, above, below := f.sessionDrawn(pageDoc(), sessionNow).turnsShown()
		if above != 1 || len(rows) == 0 {
			t.Fatalf("at %d columns, %d turns are out of view above, want 1", width, above)
		}
		both := turnsOut.keyless().said(above, below)
		got := pageText(f)[f.pageLayout().moreTurns]
		if both.width() <= width && !strings.HasPrefix(got, both.plain()) {
			t.Errorf("at %d columns, the row under the turns reads %q, want it to start %q", width, got, both.plain())
		}
	}
}

// pageFrame is a frame of the session's page shown, in a terminal of the
// size given, in nord, over the Log: its turn open, where it has one, showing
// as many requests at a time as RequestsFit has it, as the watch opens it.
func pageFrame(t *testing.T, shown *SessionShown, width, height int) Frame {
	t.Helper()
	f := frameOf(width, height)
	f.View, f.Look, f.Session = Log, Screen(builtin(t, "nord")), shown
	if shown != nil && shown.Open != 0 && shown.RequestsRows == 0 {
		shown.RequestsRows = f.RequestsFit()
	}
	return f
}

// pageText is the page f draws, of the frames' document at their now, as
// text, each row without the blanks at its end.
func pageText(f Frame) []string {
	return textOf(f.Draw(pageDoc(), sessionNow))
}

// pageCanvas is the page f draws, of the frames' document at their now, on
// its canvas, for its inks.
func pageCanvas(f Frame) *canvas {
	doc := pageDoc()
	return f.At(doc, sessionNow).page(doc, sessionNow)
}

// ranOn is the session given as it ran on n accounts the config doesn't
// have, so each is named by its id, unnumbered: a few requests on each, one
// after the other.
func ranOn(shown *SessionShown, n int) *SessionShown {
	shown.Page.Accounts = nil
	for i := range n {
		id := "account-" + string(rune('a'+i))
		shown.Page.Accounts = append(shown.Page.Accounts, views.SessionAccount{
			Account: id, From: at(12, i), To: at(12, i), Requests: 3, Worth: worthOf(10), Points: map[string]int{"5h": 1},
		})
	}
	return shown
}

// A session's page heads itself in the wide chrome's form from 100 columns,
// its right parts ending at the last column, and a phone's under them, as
// the chrome does; its turns take one form at every width, their columns
// leaving as it narrows; and below its least rows, it's the chrome alone.
// Its accounts take a row each while three turns fit beside them, then, once
// the story has given way, give way to a row saying how many more; and with
// the turns at their least, that row, the story's heading and the now rows
// give way in turn.
func TestASessionsPageAtEachSize(t *testing.T) {
	tests := []struct {
		name          string
		shown         *SessionShown
		width, height int
		want          map[int]string
		// ends are rows that end as given, at endAt, or the last column where
		// it's none.
		ends  map[int]string
		endAt int
		// blank are rows drawn blank, from the first given to the second.
		blank [2]int
	}{
		{
			name: "200 wide: the rows' right parts at the last column, tools at its most, the spare trailing", shown: moved(), width: 200, height: 37,
			want: map[int]string{
				4:  "SESSION   d28c   switchboard  ~/Code/switchboard " + rule(141) + "  esc back",
				6:  "↑ asking   on side ▲ since 14:31, moved from work: work reached its cap" + strings.Repeat(" ", 107) + "started 12:02 · 2h 40m",
				22: "TURNS    " + rule(126) + "  13 turns · 445 requests · 97% from cache · $48.20 at API prices",
				24: "  #  started  took   requests  tools ",
				25: " 13  14:40    3m…           6  Read ×3, Bash ×2 ",
			},
			ends:  map[int]string{25: "958k      18k    5.4k     $0.79  side             ↑ asking"},
			endAt: 158,
		},
		{
			name: "122 wide: every column, tools at its natural", shown: moved(), width: 122, height: 37,
			want: map[int]string{
				6:  "↑ asking   on side ▲ since 14:31, moved from work: work reached its cap                             started 12:02 · 2h 40m",
				24: "  #  started  took   requests  tools                            read  written     out     worth  on",
			},
			ends: map[int]string{25: "$0.79  side             ↑ asking"},
		},
		{
			name: "108 wide: the wide chrome's heading, tools narrower", shown: moved(), width: 108, height: 37,
			want: map[int]string{
				4:  "SESSION   d28c   switchboard  ~/Code/switchboard ─",
				24: "  #  started  took   requests  tools              read  written     out     worth  on",
				25: " 13  14:40    3m…           6  Read ×3            958k",
			},
			ends: map[int]string{25: "$0.79  side             ↑ asking"},
		},
		{
			name: "100 wide: the wide chrome's heading, tools gone", shown: moved(), width: 100, height: 30,
			want: map[int]string{
				4: "SESSION   d28c   switchboard  ~/Code/switchboard ─", 6: "↑ asking   on side ▲ since 14:31, moved from work: work reached its cap       started 12:02 · 2h 40m", 21: "TURNS    ",
				23: "  #  started  took   requests     read  written     out     worth  on",
			},
		},
		{
			name: "99 wide: a phone's heading, under its own chrome, its now rows wide as they fit", shown: moved(), width: 99, height: 30,
			want: map[int]string{
				5: "SESSION  d28c  switchboard", 7: "↑ asking   on side ▲ since 14:31, moved from work: work reached its cap      started 12:02 · 2h 40m", 8: "opus-5-5 → side   haiku-4-5 → side",
				21: "TURNS    ", 23: "  #  started  took   requests     read  written     out     worth  on",
			},
		},
		{
			name: "96 wide: the now rows wide where their leasts fit, started as how it has run alone", shown: moved(), width: 96, height: 30,
			want: map[int]string{
				7: "↑ asking   on side ▲ since 14:31, moved from work: work reached its cap                   2h 40m",
				8: "opus-5-5 → side   haiku-4-5 → side",
			},
		},
		{
			name: "95 wide: the now rows short, as their leasts don't fit", shown: moved(), width: 95, height: 30,
			want: map[int]string{7: "↑ asking  on side ▲ since 14:31", 8: "moved from work: its cap"},
		},
		{
			name: "52 wide: a phone's, its turns their figures, worth and on", shown: moved(), width: 52, height: 32,
			want: map[int]string{
				5: "SESSION  d28c  switchboard                  esc back", 7: "↑ asking  on side ▲ since 14:31",
				23: "  #  started  took     req     worth  on", 24: " 13  14:40    3m…        6     $0.79  side",
				30: "▼ 7 more turns   ·   j/k to scroll",
			},
		},
		{
			name: "ten accounts at 37 rows: every one, the story's columns four rows tall", shown: ranOn(moved(), 10), width: 160, height: 37,
			want: map[int]string{18: "account-a       12:00 – 12:00", 27: "account-j       12:09 – now", 29: "TURNS    "},
		},
		{
			name: "ten accounts at 24 rows, the story folded: three and seven more", shown: ranOn(moved(), 10), width: 160, height: 24,
			want: map[int]string{11: "account-a", 13: "account-c", 14: "+7 more accounts", 16: "TURNS    "},
		},
		{
			name: "two accounts at 20 rows, the story folded: the row of how many alone", shown: moved(), width: 160, height: 20,
			want: map[int]string{11: "+2 more accounts", 13: "TURNS    "},
		},
		{
			name: "160×13: the accounts' row given way", shown: moved(), width: 160, height: 13,
			want: map[int]string{4: "SESSION   d28c", 5: "↑ asking", 7: "STORY   no room at 13 rows", 8: "TURNS    ", 10: " 13  14:40", 11: "▼ 12 more turns"},
		},
		{
			name: "160×12: the story's heading given way", shown: moved(), width: 160, height: 12,
			want: map[int]string{4: "SESSION   d28c", 5: "↑ asking", 7: "TURNS    ", 9: " 13  14:40", 10: "▼ 12 more turns"},
		},
		{
			name: "160×10: the now rows given way, its least", shown: moved(), width: 160, height: 10,
			want: map[int]string{4: "SESSION   d28c", 5: "TURNS    ", 7: " 13  14:40", 8: "▼ 12 more turns"},
		},
		{
			name: "52×11: a phone's least, its now rows given way", shown: moved(), width: 52, height: 11,
			want: map[int]string{5: "SESSION  d28c", 6: "TURNS    ", 8: " 13  14:40", 9: "▼ 12 more turns"},
		},
		{name: "160×9: the chrome alone", shown: moved(), width: 160, height: 9, blank: [2]int{3, 7}},
		{name: "52×10: a phone's chrome alone", shown: moved(), width: 52, height: 10, blank: [2]int{4, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := pageText(pageFrame(t, tt.shown, tt.width, tt.height))
			for y, want := range tt.want {
				if !strings.HasPrefix(rows[y], want) || len(want) == 0 {
					t.Errorf("row %d reads\n%q\nwant it to start\n%q", y+1, rows[y], want)
				}
			}
			for y, want := range tt.ends {
				if end := cmp.Or(tt.endAt, tt.width); !strings.HasSuffix(rows[y], want) || ansi.StringWidth(rows[y]) != end {
					t.Errorf("row %d reads\n%q\nwant it to end at column %d\n%q", y+1, rows[y], end, want)
				}
			}
			for y := tt.blank[0]; y > 0 && y <= tt.blank[1]; y++ {
				if rows[y] != "" {
					t.Errorf("row %d reads %q, want it blank", y+1, rows[y])
				}
			}
			for y, row := range rows {
				if strings.HasPrefix(row, "SESSION") && tt.blank[0] > 0 {
					t.Errorf("row %d reads %q, want no heading under the chrome alone", y+1, row)
				}
			}
		})
	}
}

// Where the now rows are wide, row 6 says where it runs and why whole, its
// right part whole; row 7's models leave whole from the end three cells
// before the windows; and an account's points take their short words where
// the wide don't fit whole, and leave whole windows from the end where they
// wouldn't fit at their most, 999+ points each, two cells before its worth's
// column, as wide as its widest.
func TestANarrowWidePageCutsItsRowsBeforeTheirRightParts(t *testing.T) {
	shown := resumed()
	shown.Page.Goes = append(shown.Page.Goes, views.SessionModel{Model: haiku, Account: "side"})
	shown.Page.Accounts[1].Points["7d_oi"] = 2
	rows := pageText(pageFrame(t, shown, 157, 37))
	if want := "↓ answering   on personal ◆ since 09:12, rescored after 15h 7m idle: personal had the most room   "; !strings.HasPrefix(rows[6], want) ||
		!strings.HasSuffix(rows[6], "started yesterday 16:40 · resumed 09:12") {
		t.Errorf("row 7 reads\n%q\nwant where it runs whole, three cells or more before when it started", rows[6])
	}
	windows := "personal now  week 66%   5-hour 39%   fable week 0%"
	models := "opus-5-5 → personal   haiku-4-5 → side"
	if !strings.HasPrefix(rows[7], models+"   ") || !strings.HasSuffix(rows[7], windows) {
		t.Errorf("row 8 reads\n%q\nwant its models whole, all fitting, %q", rows[7], models)
	}
	for _, more := range [][2]string{{"claude-sonnet-5-5", "personal"}, {"claude-fable-1", "work"}, {"claude-opus-4-1", "side"}, {"claude-sonnet-4-5", "work"}} {
		shown.Page.Goes = append(shown.Page.Goes, views.SessionModel{Model: more[0], Account: more[1]})
	}
	rows = pageText(pageFrame(t, shown, 157, 37))
	if want := models + "   sonnet-5-5 → personal   fable-1 → work   opus-4-1 → side"; !strings.HasPrefix(rows[7], want+"   ") ||
		strings.Contains(rows[7], "sonnet-4-5") || !strings.HasSuffix(rows[7], windows) {
		t.Errorf("row 8 reads\n%q\nwant the model that doesn't fit left out whole, after\n%q", rows[7], want)
	}
	rows = pageText(pageFrame(t, shown, 130, 37))
	if want := "≈ 9 of week · 31 of 5-hour · 2 of Fable week" + strings.Repeat(" ", 14) + "$114.10"; !strings.HasSuffix(rows[20], want) {
		t.Errorf("row 21 reads\n%q\nwant its points in their short words, whole where the wide don't fit, ending\n%q", rows[20], want)
	}
	if want := "≈ 9 points of its week · 31 of its 5-hour · 2 of its Fable week"; !strings.Contains(pageText(pageFrame(t, shown, 160, 37))[20], want) {
		t.Errorf("at 160 columns, row 21 doesn't say %q", want)
	}
	f := pageFrame(t, shown, 58, 37)
	d, narrow := pageDrawn(f, pageDoc())
	if row, want := strings.TrimRight(string(narrow[d.l.accountsAt+1]), " "), "≈ 9 of week · 31 of 5-hour"+strings.Repeat(" ", 9)+"$114.10"; !strings.HasSuffix(row, want) {
		t.Errorf("at 58 columns, its row reads\n%q\nwant Fable week's points left out whole, ending\n%q", row, want)
	}
}

// What the session's doing leads row 6, bold, in its own ink; and while a
// request of it waits for its answer, asking, or throttled or refused, to go
// out again, the newest turn's row ends in ↑ asking, but
// never while its answer streams, or it idles. What's in flight as last
// known, the router not answering, is said as it was, in text.subtle, not
// bold: leading row 6, and ending the newest turn's row where it waited.
func TestTheNewestTurnAsksWhileTheSessionWaits(t *testing.T) {
	tests := []struct {
		name     string
		doing    views.Activity
		said     string
		ink      ink
		asking   bool
		askingIn ink
	}{
		{name: "asking", doing: views.Activity{Doing: views.Asking}, said: "↑ asking", ink: keyInk, asking: true, askingIn: keyInk},
		{name: "throttled", doing: views.Activity{Doing: views.Throttled, Status: 429}, said: "… 429", ink: labelInk, asking: true, askingIn: keyInk},
		{name: "refused", doing: views.Activity{Doing: views.Refused, Status: 403}, said: "✕ 403", ink: exhaustedInk, asking: true, askingIn: keyInk},
		{name: "answering", doing: views.Activity{Doing: views.Answering}, said: "↓ answering", ink: keyInk},
		{name: "asking as last known", doing: views.Activity{Doing: views.Asking, Stale: true}, said: "↑ asking", ink: dimInk, asking: true, askingIn: dimInk},
		{name: "throttled as last known", doing: views.Activity{Doing: views.Throttled, Status: 429, Stale: true}, said: "… 429", ink: dimInk, asking: true, askingIn: dimInk},
		{name: "refused as last known", doing: views.Activity{Doing: views.Refused, Status: 403, Stale: true}, said: "✕ 403", ink: dimInk, asking: true, askingIn: dimInk},
		{name: "answering as last known", doing: views.Activity{Doing: views.Answering, Stale: true}, said: "↓ answering", ink: dimInk},
		{name: "idle", doing: views.Activity{Since: sessionNow.Add(-9 * time.Minute)}, said: "idle 9m", ink: idleInk},
		{name: "ended", doing: views.Activity{Doing: views.Ended, Since: at(14, 0)}, said: "ended 14:00", ink: strongInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := moved()
			shown.Page.Activity = tt.doing
			f := pageFrame(t, shown, 160, 37)
			c := pageCanvas(f)
			if got := rowOf(c, 6); !strings.HasPrefix(got, tt.said+"   ") || c.cells[6][0].ink != tt.ink {
				t.Errorf("row 7 reads %q in %+v, want it to lead with %q in %+v", got, c.cells[6][0].ink, tt.said, tt.ink)
			}
			d, _ := pageDrawn(f, pageDoc())
			col, _ := d.turnColumnOf(turnAsking)
			askingAt := col.x
			asking := strings.TrimSpace(string([]rune(rowOf(c, 25) + strings.Repeat(" ", c.width))[askingAt:]))
			if tt.asking != (asking == "↑ asking") || tt.asking && c.cells[25][askingAt].ink != tt.askingIn {
				t.Errorf("the newest turn's row ends %q in %+v, want ↑ asking in %+v: %t", asking, c.cells[25][askingAt].ink, tt.askingIn, tt.asking)
			}
			for y := 26; y < 35; y++ {
				if strings.Contains(rowOf(c, y), "asking") {
					t.Errorf("row %d reads %q, want ↑ asking on the newest turn's row alone", y+1, rowOf(c, y))
				}
			}
		})
	}
}

// The turn chosen is lit on bg.selection across its columns, to the end of
// on, short of ↑ asking's, its words bold, at every width. No other turn is
// lit.
func TestTheTurnChosenIsLit(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		chosen        int
		lit           int
	}{
		{name: "wide, the newest", width: 160, height: 37, chosen: 25, lit: 148},
		{name: "wide, one under it", width: 160, height: 37, chosen: 27, lit: 148},
		{name: "a phone", width: 52, height: 32, chosen: 24, lit: 52},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := moved()
			if tt.chosen == 27 {
				shown.Turn = 11
			}
			c := pageCanvas(pageFrame(t, shown, tt.width, tt.height))
			for y := 22; y < tt.height-2; y++ {
				for x := range tt.width {
					lit := y == tt.chosen && x < tt.lit
					if got := c.cells[y][x].ink.on == chosenTurn; got != lit {
						t.Fatalf("row %d, column %d is lit %t, want %t", y+1, x+1, got, lit)
					}
				}
			}
			if !c.cells[tt.chosen][5].ink.bold {
				t.Error("the turn chosen's start isn't bold")
			}
		})
	}
}

// The turns scroll within their rows, as far as leaves them full, the row
// under them saying how many are out of view each way, and how to scroll to
// them.
func TestATurnsScrollWithinTheirRows(t *testing.T) {
	tests := []struct {
		scroll int
		first  string
		more   string
	}{
		{scroll: 0, first: " 13  14:40", more: "▼ 3 more turns   ·   j/k to scroll"},
		{scroll: 2, first: " 11  14:15", more: "▲ 2 more turns   ·   ▼ 1 more turn   ·   j/k to scroll"},
		{scroll: 9, first: " 10  14:00", more: "▲ 3 more turns   ·   j/k to scroll"},
		{scroll: -1, first: " 13  14:40", more: "▼ 3 more turns   ·   j/k to scroll"},
	}
	for _, tt := range tests {
		shown := moved()
		shown.Scroll = tt.scroll
		rows := pageText(pageFrame(t, shown, 160, 37))
		if !strings.HasPrefix(rows[25], tt.first) || rows[35] != tt.more {
			t.Errorf("scrolled %d, the turns start %q and the row under them reads %q; want %q and %q", tt.scroll, rows[25], rows[35], tt.first, tt.more)
		}
	}
}

// A turn's on names the accounts it went to in its cells, each at a
// label's most: each in turn where they fit, the arrows narrowing to →
// before any name is cut, then two moves or more as their first and last,
// … between; then cut, the arrow kept narrow: a move's second name, its
// first whole, down to moveToLeast cells, else both as movedCells shares
// the room; and where the first would keep under two cells, it alone.
func TestATurnsAccountsFitItsOnColumn(t *testing.T) {
	tests := []struct {
		accounts []string
		cells    int
		want     string
	}{
		{accounts: []string{"work"}, cells: 12, want: "work"},
		{accounts: []string{"work", "side"}, cells: 12, want: "work → side"},
		{accounts: []string{"personal", "side"}, cells: 12, want: "person…→side"},
		{accounts: []string{"personal", "side"}, cells: 11, want: "perso…→side"},
		{accounts: []string{"personal", "side"}, cells: 15, want: "personal → side"},
		{accounts: []string{"personal", "side"}, cells: 13, want: "personal→side"},
		{accounts: []string{"work", "side"}, cells: 10, want: "work→side"},
		{accounts: []string{"work", "side"}, cells: 9, want: "work→side"},
		{accounts: []string{"work", "side", "work"}, cells: 12, want: "work→…→work"},
		{accounts: []string{"work", "side", "work"}, cells: 8, want: "w…→…→wo…"},
		{accounts: []string{"a", "b", "c", "d"}, cells: 12, want: "a→b→c→d"},
		{accounts: []string{"a", "b", "c", "d"}, cells: 6, want: "a→…→d"},
		{accounts: []string{"work", "personal", "side"}, cells: 18, want: "work→personal→side"},
		{accounts: []string{"work", "personal", "side"}, cells: 12, want: "work→…→side"},
		{accounts: []string{"work", "personal", "side"}, cells: 5, want: "wo…→…"},
		{accounts: []string{"work", "side"}, cells: 4, want: "w…→…"},
		{accounts: []string{"abcdefghijklmnopqrstuvwxyz0123"}, cells: 30, want: "abcdefghijklmnopqrstuvw…"},
		{accounts: []string{"abcdefghijklmnopqrstuvwxyz0123", "side"}, cells: 30, want: "abcdefghijklmnopqrstuvw…→side"},
		{accounts: []string{"workstation-one"}, cells: 12, want: "workstation…"},
		{accounts: []string{"work", "side"}, cells: labelLeast, want: "work→si…"},
		{accounts: []string{"work", "side"}, cells: 7, want: "wo…→si…"},
		{accounts: []string{"work", "side"}, cells: 6, want: "w…→si…"},
		{accounts: []string{"work", "side"}, cells: 5, want: "w…→s…"},
	}
	for _, tt := range tests {
		d := sessionDrawn{m: pageDoc().At(sessionNow, claudeLike)}
		if got := d.turnOn(tt.accounts, tt.cells); got.plain() != tt.want || slices.ContainsFunc(got, func(s span) bool { return s.ink != secondaryInk }) {
			t.Errorf("in %d cells, %q are named %q in %+v, want %q in text.secondary", tt.cells, tt.accounts, got.plain(), got, tt.want)
		}
	}
}

// A turn's on takes its natural from the accounts configured, never those
// the session ran on: the widest name, a label's, at most labelMost and
// labelLeast at the least; and past it, where there are two or more, a
// move's arrow and the second widest name.
func TestATurnsOnIsAsWideAsTheAccountsConfiguredSay(t *testing.T) {
	long := strings.Repeat("x", labelMost+6)
	tests := []struct {
		name        string
		labels      []string
		named, move int
	}{
		{name: "the frames' three", labels: []string{"work", "personal", "side"}, named: 8, move: 7},
		{name: "one", labels: []string{"work"}, named: labelLeast},
		{name: "one of a label's most", labels: []string{long}, named: labelMost},
		{name: "two long", labels: []string{long, "side-hustle-consulting"}, named: labelMost, move: 3 + 22},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := pageDoc()
			doc.Accounts = doc.Accounts[:len(tt.labels)]
			for i, label := range tt.labels {
				doc.Accounts[i].Label = label
			}
			if name, move := turnOnCells(doc.At(sessionNow, claudeLike)); name != tt.named || move != tt.move {
				t.Errorf("on's natural is %d and a move %d past it, want %d and %d", name, move, tt.named, tt.move)
			}
		})
	}
}

// A turn's row keeps its figures whole at every width from 40 columns to
// 220, at their bounds: its number in as many cells as the newest's, so turn
// 1005 reads apart from turn 5; its length to "999m…", going, and "999m"
// once over; and its requests to "9,999".
func TestATurnsRowKeepsItsFiguresWholeAtTheirBounds(t *testing.T) {
	shown := moved()
	shown.Page.Turns = turnsOf(
		turnRow{1005, at(14, 42).Add(-998 * time.Minute), 0, 9999, nil, 0, 0, 0, 661, []string{"side"}},
		turnRow{1004, at(14, 29).Add(-24 * time.Hour), 999, 9999, nil, 0, 0, 0, 661, []string{"side"}},
		turnRow{5, at(14, 29), 9, 37, nil, 0, 0, 0, 661, []string{"side"}},
	)
	shown.Turn = 1005
	for width := 40; width <= 220; width++ {
		d, rows := pageDrawn(pageFrame(t, shown, width, 37), pageDoc())
		for i, turn := range shown.Page.Turns {
			row := rows[d.l.turnsAt+i]
			took := turn.TookSaid(sessionNow)
			figures := map[turnPart]string{
				turnNumber: strconv.Itoa(turn.Turn), turnTook: took, turnRequests: prose.Grouped(turn.Requests), turnWorth: turn.Cents(),
			}
			for p, want := range figures {
				if got := turnText(row, d, p); got != want {
					t.Errorf("at %d columns, turn %d's column %d reads %q, want %q whole", width, turn.Turn, p, got, want)
				}
			}
		}
	}
}

// A session's page names an account by its label wherever it names it, its
// story among its parts, where the config gives one other than its id.
func TestASessionsPageNamesItsAccountsByTheirLabels(t *testing.T) {
	doc := pageDoc()
	doc.Accounts[0].Label = "office"
	shown := moved()
	shown.Story = movedStory()
	text := strings.Join(textOf(pageFrame(t, shown, 160, 37).Draw(doc, sessionNow)), "\n")
	for _, want := range []string{"moved from office: office reached its cap", "office reached its cap → side · 182k", "─ office ─", "1 office "} {
		if !strings.Contains(text, want) {
			t.Errorf("the page doesn't say %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "work") {
		t.Errorf("the page names work by its id:\n%s", text)
	}
}

// A session's page's heading names it by its directory: one with none by
// its id alone; a long one cut from its start with …, its name kept whole.
// A phone names it by its id and its directory's name. Each holds here with
// its rule, whatever its length, as one ─.
func TestASessionsHeadingNamesItByItsDirectory(t *testing.T) {
	long := "/home/dev/Code/clients/acme-corporation/platform/services/reconciliation/workers/nightly/billing"
	tests := []struct {
		name, dir string
		width     int
		want      string
	}{
		{name: "none", width: 160, want: "SESSION   d28c ─  esc back"},
		{name: "none, on a phone", width: 52, want: "SESSION  d28c                               esc back"},
		{name: "a long one, whole where it fits", dir: long, width: 160,
			want: "SESSION   d28c   billing  ~/Code/clients/acme-corporation/platform/services/reconciliation/workers/nightly/billing ─  esc back"},
		{name: "a long one, cut from its start", dir: long, width: 122,
			want: "SESSION   d28c   billing  …e/clients/acme-corporation/platform/services/reconciliation/workers/nightly/billing ─  esc back"},
		{name: "a long one, on a phone", dir: long, width: 52, want: "SESSION  d28c  billing                      esc back"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := moved()
			shown.Page.Dir = tt.dir
			f := pageFrame(t, shown, tt.width, 37)
			got := pageText(f)[f.layoutOf(len(shown.Page.Accounts)).heading]
			for strings.Contains(got, "──") {
				got = strings.ReplaceAll(got, "──", "─")
			}
			if got != tt.want {
				t.Errorf("the heading reads\n%q\nwant\n%q", got, tt.want)
			}
		})
	}
}

// A session's heading draws its id and its directory's name bold
// text.primary, and the directory, as wide pages show it, in text.muted, the
// tone of a heading's quieter words: SESSION   d28c   switchboard
// ~/Code/switchboard.
func TestASessionsHeadingsInks(t *testing.T) {
	f := pageFrame(t, moved(), 160, 37)
	c, y := pageCanvas(f), f.layoutOf(len(moved().Page.Accounts)).heading
	tests := []struct {
		name   string
		x, end int
		want   ink
	}{
		{name: "SESSION", x: 0, end: 6, want: dimInk},
		{name: "the id", x: 10, end: 13, want: titleInk},
		{name: "the directory's name", x: 17, end: 27, want: titleInk},
		{name: "the directory", x: 30, end: 47, want: mutedInk},
	}
	for _, tt := range tests {
		for x := tt.x; x <= tt.end; x++ {
			if got := c.at(x, y).ink; got != tt.want {
				t.Errorf("%s, at column %d, is in %+v, want %+v", tt.name, x, got, tt.want)
			}
		}
	}
	if mutedInk != (ink{token: theme.TextMuted}) {
		t.Errorf("the directory's ink is %+v, want text.muted", mutedInk)
	}
}

// Where a session's requests name no prompt, TURNS says why it has no
// turns, in text.faint, where its rows would be (C14); with no request at
// all, it says nothing.
func TestTURNSSaysWhyASessionHasNoTurns(t *testing.T) {
	for _, width := range []int{160, 52} {
		shown := quiet()
		shown.Page.Turns, shown.Page.Totals.Turns = nil, 0
		l := pageFrame(t, shown, width, 37).layoutOf(1)
		c := pageCanvas(pageFrame(t, shown, width, 37))
		if got := rowOf(c, l.heads); got != noTurns[:min(len(noTurns), width)] && !strings.HasPrefix(noTurns, strings.TrimSuffix(got, "…")) || c.cells[l.heads][0].ink != faintInk {
			t.Errorf("at %d columns, TURNS' rows read %q in %+v, want %q in text.faint", width, got, c.cells[l.heads][0].ink, noTurns)
		}
		shown.Page.Totals.Requests = 0
		if got := rowOf(pageCanvas(pageFrame(t, shown, width, 37)), l.heads); got != "" {
			t.Errorf("at %d columns, with no request, TURNS' rows read %q, want nothing", width, got)
		}
	}
}

// Row 7 draws nothing at its right where the document lacks the account the
// session runs on, as one the config has since dropped: its models alone,
// with the whole row to them.
func TestRow7DrawsNoWindowsOfAnAccountTheDocumentLacks(t *testing.T) {
	shown := moved()
	shown.Page.Account = "gone"
	shown.Page.Goes = []views.SessionModel{{Model: opus, Account: "gone"}, {Model: haiku, Account: "gone"}}
	f := pageFrame(t, shown, 160, 37)
	if got := pageText(f)[f.layoutOf(len(shown.Page.Accounts)).now+1]; got != "opus-5-5 → gone   haiku-4-5 → gone" {
		t.Errorf("row 7 reads %q, want its models alone", got)
	}
}

// A phone's two rows of now say a pin's move as its views do, short, the
// second cut with … to leave how it has run whole at its right: its own pin
// moving it, and its pin yielding, which the hard moments don't hold.
func TestAPhonesNowRowsSayAPinsMoveShort(t *testing.T) {
	tests := []struct {
		name, reason, from, to string
		row7, row8             string
	}{
		{name: "pinned here", reason: status.ReasonPinned, from: "work", to: "side",
			row7: "↑ asking  on side ▲ since 14:31", row8: "pinned here                                   2h 40m"},
		{name: "its pin yielding", reason: status.ReasonPinYields + "side has no room", from: "side", to: "work",
			row7: "↑ asking  on work since 14:31", row8: "its pin to side yields: side has no room      2h 40m"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := moved()
			shown.Page.Account = tt.to
			shown.Page.Moves = []views.SessionMove{{At: at(14, 31), Model: opus, From: tt.from, To: tt.to, Reason: tt.reason}}
			f := pageFrame(t, shown, 52, 32)
			rows, now := pageText(f), f.layoutOf(len(shown.Page.Accounts)).now
			if rows[now] != tt.row7 || rows[now+1] != tt.row8 {
				t.Errorf("a phone's rows of now read\n%q\n%q\nwant\n%q\n%q", rows[now], rows[now+1], tt.row7, tt.row8)
			}
		})
	}
}

// Once it's over, a phone's rows say it ended, then where it ran, short,
// and how long it ran.
func TestAnEndedSessionOnAPhone(t *testing.T) {
	rows := pageText(pageFrame(t, ended(), 52, 32))
	if rows[7] != "ended yesterday 18:47" {
		t.Errorf("row 8 reads %q, want it ended", rows[7])
	}
	if !strings.HasPrefix(rows[8], "ran on work, then side from") || !strings.HasSuffix(rows[8], "ran 5h 27m") {
		t.Errorf("row 9 reads %q, want where it ran, short, and how long it ran", rows[8])
	}
}

// A click on a turn's row shown chooses it: a target each, over the row,
// as the turns are scrolled; none of the tab's heading row under the page,
// and none of the page under the chrome alone.
func TestASessionsPageTargetsItsTurnsShown(t *testing.T) {
	shown := moved()
	shown.Scroll = 2
	f := pageFrame(t, shown, 160, 37)
	f.View = History
	var turns []Target
	for _, target := range f.Targets(pageDoc(), sessionNow) {
		switch target.On {
		case OnTurn:
			turns = append(turns, target)
		case OnSubTab, OnStep:
			t.Errorf("the page targets %+v, the heading row of the tab under it", target)
		}
	}
	if len(turns) != 10 {
		t.Fatalf("the page targets %d turns, want the 10 shown", len(turns))
	}
	for i, target := range turns {
		if want := (Target{X: 0, Y: 25 + i, Width: 160, On: OnTurn, Turn: 11 - i}); target != want {
			t.Errorf("target %d is %+v, want %+v", i, target, want)
		}
	}
	f.Height = 9
	if slices.ContainsFunc(f.Targets(pageDoc(), sessionNow), func(t Target) bool { return t.On == OnTurn }) {
		t.Error("under the chrome alone, the page targets its turns, want none")
	}
}

// A session's page open over the Log draws as over a Log not read yet: the
// state line the Log draws under its table once it's read is none of the
// page's, whose row it would be says how many more turns there are.
func TestASessionsPageOverTheLogDrawsNoneOfTheLogsStateLine(t *testing.T) {
	tests := []struct {
		width, height int
	}{
		{width: 160, height: 37},
		{width: 52, height: 32},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d×%d", tt.width, tt.height), func(t *testing.T) {
			unread := pageFrame(t, moved(), tt.width, tt.height)
			read := unread
			read.Log = LogPage{Read: true}
			want, got := pageText(unread), pageText(read)
			for y := range want {
				if got[y] != want[y] {
					t.Errorf("over the Log read, row %d reads\n%q\nwant, as over the Log unread,\n%q", y+1, got[y], want[y])
				}
			}
		})
	}
}

// A session's page open over the Log takes a click as the page does: on its
// turns, never on the lines of the Log's table behind it.
func TestASessionsPageOverTheLogTargetsNoneOfTheLogsLines(t *testing.T) {
	f := pageFrame(t, moved(), 160, 37)
	f.Log = cardPage(40)
	var turns int
	for _, target := range f.Targets(pageDoc(), sessionNow) {
		switch target.On {
		case OnRow:
			t.Errorf("the Log's line %d is a target along row %d, under the page", target.Row, target.Y+1)
		case OnTurn:
			turns++
		}
	}
	if turns == 0 {
		t.Error("the page's turns are no targets, want a target each")
	}
}

// Nothing of the view under a session's page moves, nor is a bar eased on
// it, nor a number the request stream changes; row 7's windows are the
// account's own numbers, and its accounts' points the session's, which
// brighten as a look changes them, and move the frame while they settle.
func TestWhatMovesOnASessionsPage(t *testing.T) {
	doc := threeRouted()
	f := frameOf(160, 40)
	f.Eased = f.Filled(doc, now)
	if got := f.Motion(doc, now); got != (Motion{Smooth: true}) {
		t.Fatalf("the Overview's Motion is %+v, want its bars easing", got)
	}
	f.Session = moved()
	if got := f.Motion(doc, now); got != (Motion{}) {
		t.Errorf("over the Overview, the page's Motion is %+v, want nothing of the Overview's settling", got)
	}
	if filled := f.Filled(doc, now); filled != nil {
		t.Errorf("the page fills bars %v, want none", filled)
	}
	if f.StreamNumbered() {
		t.Error("over the Overview, the page draws a number the stream changes, want none")
	}
	page := pageFrame(t, moved(), 160, 37)
	side5h := Number{Kind: NumberUsed, Account: "side", Of: "5h"}
	numbers := page.Numbers(pageDoc(), sessionNow)
	if numbers[side5h] != "13%" {
		t.Errorf("the page's numbers are %v, want side's 5-hour, 13%%, among them", numbers)
	}
	for _, n := range []Number{{Kind: NumberPoints, Account: "work", Of: "5h"}, {Kind: NumberPoints, Account: "side", Of: "7d"}} {
		if numbers[n] == "" {
			t.Errorf("the page's numbers are %v, want the points %+v the session took among them, as a look reads them", numbers, n)
		}
	}
	page.Brightened = map[Number]float64{side5h: 0.5}
	if got := page.Motion(pageDoc(), sessionNow); got != (Motion{Smooth: true}) {
		t.Errorf("with row 7's 5-hour brightening, the page's Motion is %+v, want it settling", got)
	}
}
