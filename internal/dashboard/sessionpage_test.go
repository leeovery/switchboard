package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
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

// pageFrame is a frame of the session's page shown, in a terminal of the
// size given, in nord, over the Log.
func pageFrame(t *testing.T, shown *SessionShown, width, height int) Frame {
	t.Helper()
	f := frameOf(width, height)
	f.View, f.Look, f.Session = Log, Screen(builtin(t, "nord")), shown
	return f
}

// tabsOf are the cells of the tabs named on row y of the frame with the stem
// given, as its text has them.
func tabsOf(t *testing.T, stem string, y int, tabs ...string) []spot {
	t.Helper()
	row := []rune(referenceRows(t, stem, "txt")[y])
	var spots []spot
	for _, tab := range tabs {
		i := strings.Index(string(row)+" ", " "+tab+" ")
		if i < 0 {
			t.Fatalf("%s's row %d has no %s tab", stem, y+1, tab)
		}
		first := len([]rune(string(row)[:i]))
		spots = append(spots, spot{row: y, first: first, last: first + len(tab) + 1})
	}
	return spots
}

// A session's page draws the rows of its frames cell for cell, in nord,
// from the title row to the row over the key line, which the watch lists:
// but its STORY's, which the story's drawer holds, and the tabs, as the
// frames light the Overview the page opened over, where the fixtures reach
// it from the Log (call 64). Each frame passes over what the build draws
// otherwise than it, for the reason given.
func TestASessionsPageDrawsItsFramesRows(t *testing.T) {
	tests := []struct {
		stem          string
		shown         *SessionShown
		width, height int
		// tabs is the row of the tabs, and story the story's rows.
		tabs, storyFrom, storyTo int
		passed                   []spot
	}{
		{stem: "session-moved-160", shown: moved(), width: 160, height: 37, storyFrom: 9, storyTo: 17},
		{stem: "session-quiet-160", shown: quiet(), width: 160, height: 37, storyFrom: 9, storyTo: 17},
		{
			stem: "session-ended-160", shown: ended(), width: 160, height: 37, storyFrom: 9, storyTo: 17,
		},
		{
			stem: "session-resumed-160", shown: resumed(), width: 160, height: 37, storyFrom: 9, storyTo: 17,
			passed: []spot{
				// The frame predates the design's word: it says its answer
				// streams as "↓ streaming", where the design says "↓
				// answering".
				{row: 6, first: 0, last: 10},
				// The router writes the world's idle as it is, 15h 7m, where
				// the frame says 15h, so the where-words run 3 cells longer.
				{row: 6, first: 56, last: 94},
			},
		},
		{
			stem: "session-turn-opened-160", shown: turnOpened(), width: 160, height: 37, storyFrom: 9, storyTo: 17,
			// The frame lights no request of the turn open, where the design
			// has its first chosen as it opens (3417); its text is held by
			// TestATurnOpenedLightsItsRequestChosenAndSaysWhatsOutOfView.
			passed: []spot{{row: 28, first: 0, last: 159}},
		},
		{
			stem: "session-phone-52", shown: moved(), width: 52, height: 32, tabs: 1, storyFrom: 10, storyTo: 16,
			// The frame leaves the row under its seven turns blank, where
			// six more are out of view, as the design says them (3461).
			passed: []spot{{row: 30, first: 0, last: 51}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.stem, func(t *testing.T) {
			f := pageFrame(t, tt.shown, tt.width, tt.height)
			drawn, frame := screenOf(t, f.Draw(pageDoc(), sessionNow)), reference(t, tt.stem)
			passed := slices.Concat(tt.passed, tabsOf(t, tt.stem, tt.tabs, "Overview", "Log"))
			canvas := shadeOf(f.Look.Canvas())
			heldTo(t, tt.stem, frame, drawn, region{top: 0, bottom: tt.storyFrom - 1, right: tt.width - 1}, canvas, passed, nil)
			heldTo(t, tt.stem, frame, drawn, region{top: tt.storyTo + 1, bottom: tt.height - 2, right: tt.width - 1}, canvas, passed, nil)
		})
	}
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

// A session's page draws its wide form from 122 columns, its right parts
// ending at the last column and its turns at their own columns however wide
// it is; under 122 a phone's body, under the wide chrome down to 100; and
// below its least rows, the chrome alone. Its accounts take a row each while
// three turns fit beside them, then give way to a row saying how many more.
func TestASessionsPageAtEachSize(t *testing.T) {
	tests := []struct {
		name          string
		shown         *SessionShown
		width, height int
		want          map[int]string
		// blank are rows drawn blank, from the first given to the second.
		blank [2]int
	}{
		{
			name: "200 wide: the rows' right parts at the last column, the turns at their own", shown: moved(), width: 200, height: 37,
			want: map[int]string{
				4:  "SESSION   d28c   switchboard  ~/Code/switchboard " + rule(141) + "  esc back",
				6:  "↑ asking   on side ▲ since 14:31, moved from work: work reached its cap" + strings.Repeat(" ", 107) + "started 12:02 · 2h 40m",
				22: "TURNS    " + rule(126) + "  13 turns · 445 requests · 97% from cache · $48.20 at API prices",
				25: " 13  14:40    3m…           6   Read ×3, Bash ×2                   958k      18k    5.4k    $0.79   side          ↑ asking",
			},
		},
		{
			name: "122 wide: the least wide", shown: moved(), width: 122, height: 37,
			want: map[int]string{
				6:  "↑ asking   on side ▲ since 14:31, moved from work: work reached its cap                             started 12:02 · 2h 40m",
				24: "  #  started  took   requests   tools                              read  written     out    worth   on",
			},
		},
		{
			name: "121 wide: a phone's body under the wide chrome", shown: moved(), width: 121, height: 37,
			want: map[int]string{
				4:  "SESSION  d28c  switchboard" + strings.Repeat(" ", 87) + "esc back",
				6:  "↑ asking  on side ▲ since 14:31",
				7:  "moved from work: its cap" + strings.Repeat(" ", 91) + "2h 40m",
				17: "1 work     ≈ 41 of 5-hour · 6 of week" + strings.Repeat(" ", 78) + "$41.06",
				20: "TURNS    " + rule(104) + "  $48.20",
				22: "13 14:40   3m…   6 req" + strings.Repeat(" ", 83) + "$0.79  side",
			},
		},
		{
			name: "100 wide: a phone's body under the wide chrome", shown: moved(), width: 100, height: 30,
			want: map[int]string{4: "SESSION  d28c  switchboard", 6: "↑ asking  on side ▲ since 14:31", 20: "TURNS    "},
		},
		{
			name: "80 wide: a phone's, under its own chrome", shown: moved(), width: 80, height: 30,
			want: map[int]string{1: " Overview  Accounts", 5: "SESSION  d28c  switchboard", 7: "↑ asking  on side ▲ since 14:31", 21: "TURNS    "},
		},
		{
			name: "52 wide: a phone's, under its own chrome", shown: moved(), width: 52, height: 32,
			want: map[int]string{5: "SESSION  d28c  switchboard                  esc back", 30: "▼ 6 more turns   ·   j/k to scroll"},
		},
		{
			name: "ten accounts at 37 rows: eight and two more", shown: ranOn(moved(), 10), width: 160, height: 37,
			want: map[int]string{26: "account-h     12:07 – 12:07", 27: "+2 more accounts", 29: "TURNS    "},
		},
		{
			name: "ten accounts at 24 rows, the story folded: three and seven more", shown: ranOn(moved(), 10), width: 160, height: 24,
			want: map[int]string{11: "account-a", 13: "account-c", 14: "+7 more accounts", 16: "TURNS    "},
		},
		{
			name: "two accounts at 29 rows: the row of how many alone", shown: moved(), width: 160, height: 29,
			want: map[int]string{19: "+2 more accounts", 21: "TURNS    "},
		},
		{name: "160×20: the chrome alone", shown: moved(), width: 160, height: 20, blank: [2]int{3, 18}},
		{name: "52×18: a phone's chrome alone", shown: moved(), width: 52, height: 18, blank: [2]int{4, 16}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := pageText(pageFrame(t, tt.shown, tt.width, tt.height))
			for y, want := range tt.want {
				if !strings.HasPrefix(rows[y], want) || len(want) == 0 {
					t.Errorf("row %d reads\n%q\nwant it to start\n%q", y+1, rows[y], want)
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

// Wide but narrow, row 6's where-words are cut three cells before its right
// part, which stays whole; row 7's models leave whole from the end three
// cells before the windows; and an account's points leave whole windows from
// the end two cells before its worth (call 59).
func TestANarrowWidePageCutsItsRowsBeforeTheirRightParts(t *testing.T) {
	shown := resumed()
	shown.Page.Goes = append(shown.Page.Goes, views.SessionModel{Model: haiku, Account: "side"})
	shown.Page.Accounts[1].Points["7d_oi"] = 2
	rows := pageText(pageFrame(t, shown, 122, 37))
	right := "started yesterday 16:40 · resumed 09:12"
	left, _, _ := strings.Cut(rows[6], right)
	if !strings.HasPrefix(left, "↓ answering   on personal ◆ since 09:12, rescored after 15h 7m idle: personal") ||
		!strings.HasSuffix(strings.TrimRight(left, " "), "…") || !strings.HasSuffix(left, "   ") || !strings.HasSuffix(rows[6], right) {
		t.Errorf("row 7 reads\n%q\nwant where it runs cut with … three cells or more before %q, whole at the right", rows[6], right)
	}
	models := "opus-5-5 → personal   haiku-4-5 → side"
	if !strings.HasPrefix(rows[7], models+"    ") {
		t.Errorf("row 8 reads\n%q\nwant its models whole, all fitting, %q", rows[7], models)
	}
	shown.Page.Goes = append(shown.Page.Goes, views.SessionModel{Model: "claude-sonnet-5-5", Account: "personal"}, views.SessionModel{Model: "claude-fable-1", Account: "work"})
	rows = pageText(pageFrame(t, shown, 122, 37))
	if want := models + "   sonnet-5-5 → personal"; !strings.HasPrefix(rows[7], want+"         personal now") {
		t.Errorf("row 8 reads\n%q\nwant the model that doesn't fit left out whole, after\n%q", rows[7], want)
	}
	if want := "≈ 31 points of its 5-hour · 9 of its week" + strings.Repeat(" ", 18) + "$114.10"; !strings.HasSuffix(rows[20], want) {
		t.Errorf("row 21 reads\n%q\nwant Fable week's points left out whole, ending\n%q", rows[20], want)
	}
	if want := "≈ 31 points of its 5-hour · 9 of its week · 2 of its Fable week"; !strings.Contains(pageText(pageFrame(t, shown, 160, 37))[20], want) {
		t.Errorf("at 160 columns, row 21 doesn't say %q", want)
	}
}

// What the session's doing leads row 6, bold, in its own ink; and while a
// request of it waits for its answer, asking, or throttled or refused, to go
// out again, the newest turn's row ends in ↑ asking at 114 (call 50), but
// never while its answer streams, or it idles.
func TestTheNewestTurnAsksWhileTheSessionWaits(t *testing.T) {
	tests := []struct {
		name   string
		doing  views.Activity
		said   string
		ink    ink
		asking bool
	}{
		{name: "asking", doing: views.Activity{Doing: views.Asking}, said: "↑ asking", ink: keyInk, asking: true},
		{name: "throttled", doing: views.Activity{Doing: views.Throttled, Status: 429}, said: "… 429", ink: labelInk, asking: true},
		{name: "refused", doing: views.Activity{Doing: views.Refused, Status: 403}, said: "✕ 403", ink: exhaustedInk, asking: true},
		{name: "answering", doing: views.Activity{Doing: views.Answering}, said: "↓ answering", ink: keyInk},
		{name: "idle", doing: views.Activity{Since: sessionNow.Add(-9 * time.Minute)}, said: "idle 9m", ink: idleInk},
		{name: "ended", doing: views.Activity{Doing: views.Ended, Since: at(14, 0)}, said: "ended 14:00", ink: strongInk},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			shown := moved()
			shown.Page.Activity = tt.doing
			c := pageCanvas(pageFrame(t, shown, 160, 37))
			if got := rowOf(c, 6); !strings.HasPrefix(got, tt.said+"   ") || c.cells[6][0].ink != tt.ink {
				t.Errorf("row 7 reads %q in %+v, want it to lead with %q in %+v", got, c.cells[6][0].ink, tt.said, tt.ink)
			}
			asking := strings.TrimSpace(string([]rune(rowOf(c, 25) + strings.Repeat(" ", c.width))[askingAt:]))
			if tt.asking != (asking == "↑ asking") || tt.asking && c.cells[25][askingAt].ink != keyInk {
				t.Errorf("the newest turn's row ends %q in %+v, want ↑ asking in %+v: %t", asking, c.cells[25][askingAt].ink, keyInk, tt.asking)
			}
			for y := 26; y < 35; y++ {
				if strings.Contains(rowOf(c, y), "asking") {
					t.Errorf("row %d reads %q, want ↑ asking on the newest turn's row alone", y+1, rowOf(c, y))
				}
			}
		})
	}
}

// The turn chosen is lit on bg.selection: wide, across its columns, to 111,
// its words bold; on a phone, across the row, its words as they are. No
// other turn is lit.
func TestTheTurnChosenIsLit(t *testing.T) {
	tests := []struct {
		name          string
		width, height int
		chosen        int
		lit           int
		bold          bool
	}{
		{name: "wide, the newest", width: 160, height: 37, chosen: 25, lit: chosenCells, bold: true},
		{name: "wide, one under it", width: 160, height: 37, chosen: 27, lit: chosenCells, bold: true},
		{name: "a phone", width: 52, height: 32, chosen: 23, lit: 52},
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
			if got := c.cells[tt.chosen][5].ink.bold; got != tt.bold {
				t.Errorf("the turn chosen's start is bold %t, want %t", got, tt.bold)
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

// A turn's on names the accounts it went to: wide, in 12 cells, each in
// turn where they fit, else a move's two names cut as moveNames cuts them,
// and two moves or more as their first and last, … between (C16); on a
// phone, in 9, → between, or → and the last where they don't fit.
func TestATurnsAccountsFitItsOnColumn(t *testing.T) {
	tests := []struct {
		accounts    []string
		wide, phone string
	}{
		{accounts: []string{"work"}, wide: "work", phone: "work"},
		{accounts: []string{"work", "side"}, wide: "work → side", phone: "work→side"},
		{accounts: []string{"personal", "side"}, wide: "pers… → side", phone: "→side"},
		{accounts: []string{"work", "side", "work"}, wide: "w… → … → wo…", phone: "→work"},
		{accounts: []string{"a", "b", "c", "d"}, wide: "a → … → d", phone: "a→b→c→d"},
		{accounts: []string{"workstation-one"}, wide: "workstation…", phone: "workstat…"},
	}
	for _, tt := range tests {
		d := sessionDrawn{m: pageDoc().At(sessionNow, claudeLike)}
		if got := d.turnOn(tt.accounts); got.plain() != tt.wide || slices.ContainsFunc(got, func(s span) bool { return s.ink != secondaryInk }) {
			t.Errorf("wide, %q are named %q in %+v, want %q in text.secondary", tt.accounts, got.plain(), got, tt.wide)
		}
		if got := d.phoneOn(tt.accounts); got.plain() != tt.phone {
			t.Errorf("on a phone, %q are named %q, want %q", tt.accounts, got.plain(), tt.phone)
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

// Where a session's requests name no prompt, TURNS says why it has no
// turns, in text.faint, where its rows would be (C14); with no request at
// all, it says nothing.
func TestTURNSSaysWhyASessionHasNoTurns(t *testing.T) {
	for _, width := range []int{160, 52} {
		shown := quiet()
		shown.Page.Turns, shown.Page.Totals.Turns = nil, 0
		l := pageFrame(t, shown, width, 37).sessionLayout(1)
		c := pageCanvas(pageFrame(t, shown, width, 37))
		if got := rowOf(c, l.turnsHeading+2); got != noTurns[:min(len(noTurns), width)] && !strings.HasPrefix(noTurns, strings.TrimSuffix(got, "…")) || c.cells[l.turnsHeading+2][0].ink != faintInk {
			t.Errorf("at %d columns, TURNS' rows read %q in %+v, want %q in text.faint", width, got, c.cells[l.turnsHeading+2][0].ink, noTurns)
		}
		shown.Page.Totals.Requests = 0
		if got := rowOf(pageCanvas(pageFrame(t, shown, width, 37)), l.turnsHeading+2); got != "" {
			t.Errorf("at %d columns, with no request, TURNS' rows read %q, want nothing", width, got)
		}
	}
}

// Once it's over, a phone's rows say it ended, then where it ran, short,
// and how long it ran (call 60).
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
	f.Height = 20
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
// account's own numbers, which brighten as a look changes them, and move the
// frame while they settle.
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
	if got := page.Numbers(pageDoc(), sessionNow); got[side5h] != "13%" {
		t.Errorf("the page's numbers are %v, want side's 5-hour, 13%%, among them", got)
	}
	page.Brightened = map[Number]float64{side5h: 0.5}
	if got := page.Motion(pageDoc(), sessionNow); got != (Motion{Smooth: true}) {
		t.Errorf("with row 7's 5-hour brightening, the page's Motion is %+v, want it settling", got)
	}
}
