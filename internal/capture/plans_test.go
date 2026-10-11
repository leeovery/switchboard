package capture

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// keptFor is how long the frames' ledger keeps a day's lines: [ledger] keep
// at 90 days.
const keptFor = 90 * 24 * time.Hour

// shownTokens is a count of tokens as the frames show it: 5.7M, 958k, 5.4k,
// 900 (session2.tok).
func shownTokens(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 10_000:
		return fmt.Sprintf("%.0fk", float64(n)/1e3)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1e3)
	}
	return fmt.Sprint(n)
}

// shownWorth is a worth as the frames show it, to the cent.
func shownWorth(w views.Worth) string {
	if w.Amount == nil {
		return "unpriced"
	}
	return w.Amount.Cents() + strings.Repeat("+", min(len(w.Unpriced), 1))
}

// turnRow is a turn as a session's page draws its row: its number, when it
// started, how long it took, its requests, the three tools its answers called
// most, its tokens read, written and out, its worth, and where it ran.
type turnRow struct {
	turn                                 int
	started, took                        string
	requests                             int
	tools, read, written, out, worth, on string
}

// rowOf is the turn's row as a session's page draws it, at now: its length
// in minutes from the minute it started to the one it ended in, or, while it
// goes, the one now is in, … after it.
func rowOf(t views.Turn, now time.Time) turnRow {
	started := t.Started.In(now.Location())
	took := fmt.Sprintf("%dm", minutesFrom(started, t.Ended.In(now.Location())))
	if t.Ended.IsZero() {
		took = fmt.Sprintf("%dm…", minutesFrom(started, now))
	}
	var tools []string
	for _, c := range t.Tools[:min(len(t.Tools), 3)] {
		tools = append(tools, fmt.Sprintf("%s ×%d", c.Tool, c.Times))
	}
	return turnRow{
		turn: t.Turn, started: started.Format("15:04"), took: took, requests: t.Requests, tools: strings.Join(tools, ", "),
		read: shownTokens(t.Read), written: shownTokens(t.Written), out: shownTokens(t.Out), worth: shownWorth(t.Worth),
		on: strings.Join(t.Accounts, " → "),
	}
}

// minutesFrom counts the minutes from the one from is in to the one to is
// in, both counted.
func minutesFrom(from, to time.Time) int {
	return int(to.Truncate(time.Minute).Sub(from.Truncate(time.Minute))/time.Minute) + 1
}

// pageOf is the page of the session with the given id, of the records' lines
// read at now, the router listing routed, priced at the frames' prices.
func pageOf(t *testing.T, r *records, id string, routed *status.Session, now time.Time) views.SessionPage {
	t.Helper()
	l, _, _ := r.readers(func() time.Time { return now })
	var running []status.Session
	if routed != nil {
		running = []status.Session{*routed}
	}
	page, ok := views.SessionPageOf(id, views.PageSources{Routed: routed, Read: views.ReadSessions(l, running, now, id), Prices: drawnPrices,
		Keep: keptFor, Now: now})
	if !ok {
		t.Fatalf("SessionPageOf(%s) found no session", id)
	}
	return page
}

// routedAs is the session with the given id among those given.
func routedAs(t *testing.T, sessions []status.Session, id string) *status.Session {
	t.Helper()
	i := slices.IndexFunc(sessions, func(s status.Session) bool { return s.ID == id })
	if i < 0 {
		t.Fatalf("the router lists no session %s", id)
	}
	return &sessions[i]
}

// accountRow is an account a session ran on, as its page draws its row:
// where, its requests and their worth.
type accountRow struct {
	account  string
	requests int
	worth    string
}

// accountRows are the page's accounts as it draws their rows.
func accountRows(page views.SessionPage) []accountRow {
	var rows []accountRow
	for _, a := range page.Accounts {
		rows = append(rows, accountRow{account: a.Account, requests: a.Requests, worth: shownWorth(a.Worth)})
	}
	return rows
}

// checkTurns holds the page's turns, newest first, to the rows given, those
// past them unchecked.
func checkTurns(t *testing.T, page views.SessionPage, now time.Time, want []turnRow) {
	t.Helper()
	if len(page.Turns) < len(want) {
		t.Fatalf("the page has %d turns, want at least %d", len(page.Turns), len(want))
	}
	for i, w := range want {
		if got := rowOf(page.Turns[i], now); got != w {
			t.Errorf("turn %d is drawn\n%+v, want\n%+v", w.turn, got, w)
		}
	}
}

// checkMove holds the page's one move to the one given: when, from and to,
// what it wrote again, and its cost, none where its cache had gone.
func checkMove(t *testing.T, page views.SessionPage, now time.Time, at, from, to string, written int, cost string) {
	t.Helper()
	if len(page.Moves) != 1 {
		t.Fatalf("the page has moves %+v, want one", page.Moves)
	}
	m := page.Moves[0]
	gotCost := "none"
	if m.Cost != nil {
		gotCost = m.Cost.Cents()
	}
	if got := m.At.In(now.Location()).Format("15:04"); got != at || m.From != from || m.To != to || shownTokens(m.Written) != shownTokens(written) || gotCost != cost {
		t.Errorf("the move = %s %s → %s, %s written, costing %s, want %s %s → %s, %s, costing %s",
			got, m.From, m.To, shownTokens(m.Written), gotCost, at, from, to, shownTokens(written), cost)
	}
}

func TestD28CsPageIsItsFramesFromTodaysLines(t *testing.T) {
	now := moment(time.Local)
	r := &records{lines: heldAsWritten(d28cPlan(now).lines())}
	page := pageOf(t, r, idD28C, routedAs(t, routedSessions(now), idD28C), now)

	if page.Totals.Turns != 13 || page.Totals.Requests != 445 || shownWorth(page.Totals.Worth) != "$48.20" {
		t.Errorf("TURNS sums %d turns, %d requests, %s, want 13 turns, 445 requests, $48.20", page.Totals.Turns, page.Totals.Requests,
			shownWorth(page.Totals.Worth))
	}
	if started := page.Started.In(now.Location()).Format(time.TimeOnly); started != "12:02:10" || !page.Running || page.State != status.Asking {
		t.Errorf("the page has d28c started at %s, running %t, %s, want started at 12:02:10, running, asking", started, page.Running, page.State)
	}
	// The frames draw work's worth $41.06 and side's $7.14, as the generator
	// shares turn 12's worth out by its requests; its first two, on work, are
	// worth more than that share, so the views give $41.09 and $7.12. Work's
	// stretch is drawn to 14:31, the move, where its last request there came
	// at 14:30.
	if got := accountRows(page); len(got) != 2 || got[0].account != "work" || got[0].requests != 404 || got[1].account != "side" || got[1].requests != 41 {
		t.Errorf("the page's accounts = %+v, want work's 404 requests, then side's 41", got)
	}
	checkMove(t, page, now, "14:31", "work", "side", 182_000, "$1.82")
	checkTurns(t, page, now, []turnRow{
		{13, "14:40", "3m…", 6, "Read ×3, Bash ×2", "958k", "18k", "5.4k", "$0.79", "side"},
		{12, "14:29", "9m", 37, "Bash ×14, Edit ×9, Read ×8", "5.7M", "293k", "33k", "$6.61", "work → side"},
		{11, "14:15", "15m", 58, "Bash ×20, Edit ×13, Read ×13", "8.4M", "174k", "52k", "$7.26", "work"},
		{10, "14:00", "12m", 52, "Edit ×16, Bash ×14, Read ×12", "6.9M", "156k", "47k", "$6.19", "work"},
		{9, "13:50", "5m", 17, "Bash ×7, Read ×5", "2.1M", "51k", "15k", "$1.94", "work"},
		{8, "13:37", "8m", 31, "Read ×11, Edit ×8, Bash ×6", "3.5M", "93k", "28k", "$3.40", "work"},
		{7, "13:12", "19m", 83, "Bash ×27, Edit ×21, Read ×17", "8.3M", "249k", "75k", "$8.49", "work"},
		{6, "13:06", "2m", 5, "Read ×2", "444k", "15k", "4.5k", "$0.48", "work"},
		{5, "12:54", "6m", 22, "Read ×8, Edit ×6, Bash ×3", "1.8M", "66k", "20k", "$2.05", "work"},
		{4, "12:35", "14m", 61, "Bash ×19, Edit ×15, Read ×13", "4.3M", "183k", "55k", "$5.34", "work"},
		{3, "12:28", "3m", 9, "Read ×4, Bash ×2", "546k", "27k", "8.1k", "$0.75", "work"},
		{2, "12:11", "11m", 46, "Read ×14, Edit ×12, Bash ×10", "2.3M", "138k", "41k", "$3.59", "work"},
		{1, "12:02", "4m", 18, "Read ×9, Grep ×5, Glob ×2", "747k", "54k", "16k", "$1.32", "work"},
	})
}

func TestD28CsTurnTwelveOpensOnTheRequestsItsFrameDraws(t *testing.T) {
	now := moment(time.Local)
	l, _, _ := (&records{lines: heldAsWritten(d28cPlan(now).lines())}).readers(func() time.Time { return now })
	var lines []ledger.Line
	for h := range l.Session(idD28C) {
		lines = append(lines, h.Line)
	}
	slices.Reverse(lines)
	turn12 := slices.DeleteFunc(lines, func(l ledger.Line) bool { return l.Prompt != lines[0].Prompt[:9]+"prompt-12" })
	type row struct {
		at, model, on, read, written, out, stopped, from string
	}
	var got []row
	for _, l := range turn12[:5] {
		tokens, _ := l.Tokens()
		got = append(got, row{at: l.At.In(now.Location()).Format(time.TimeOnly), model: ledger.ShortModelID(l.Model), on: l.Account,
			read: shownTokens(tokens.CacheRead), written: shownTokens(tokens.CacheWrite), out: shownTokens(tokens.Output),
			stopped: strings.Join(append([]string{l.Answer.Stop}, l.Answer.Tools...), " · "), from: l.From})
	}
	want := []row{
		{"14:29:04", "opus-5-5", "work", "176k", "2.1k", "1.3k", "tool_use · Bash", ""},
		{"14:30:42", "opus-5-5", "work", "181k", "2.4k", "1.1k", "tool_use · Bash", ""},
		{"14:31:13", "opus-5-5", "side", "0", "182k", "1.2k", "tool_use · Bash", "work"},
		{"14:31:44", "haiku-4-5", "side", "0", "0", "40", "end_turn", ""},
		{"14:31:58", "opus-5-5", "side", "182k", "2.6k", "900", "tool_use · Edit", ""},
	}
	if !slices.Equal(got, want) || len(turn12) != 37 {
		t.Errorf("turn 12 opens on %d requests, the first\n%+v, want 37, the first\n%+v", len(turn12), got, want)
	}
	move := turn12[2]
	worth, _ := drawnPrices.Request(&move, move.At.Format(time.DateOnly))
	if move.Request != "3f2a91c4" || worth.Cost.Cents() != "$1.85" {
		t.Errorf("the move's request is %s, worth %s, want 3f2a91c4, worth $1.85, as its details in the Log draw it", move.Request, worth.Cost.Cents())
	}
}

func TestNineC2EsPageIsItsFrameFromTodaysLines(t *testing.T) {
	now := moment(time.Local)
	r := &records{lines: heldAsWritten(quietPlan(now).lines())}
	page := pageOf(t, r, id9C2E, routedAs(t, routedSessions(now), id9C2E), now)

	if page.Totals.Turns != 10 || page.Totals.Requests != 267 || shownWorth(page.Totals.Worth) != "$27.15" {
		t.Errorf("TURNS sums %d turns, %d requests, %s, want 10 turns, 267 requests, $27.15", page.Totals.Turns, page.Totals.Requests,
			shownWorth(page.Totals.Worth))
	}
	if got := accountRows(page); !slices.Equal(got, []accountRow{{"personal", 267, "$27.15"}}) {
		t.Errorf("the page's accounts = %+v, want personal's 267 requests, $27.15", got)
	}
	if len(page.Moves) != 0 {
		t.Errorf("the page has moves %+v, want none", page.Moves)
	}
	checkTurns(t, page, now, []turnRow{
		{10, "14:29", "4m", 16, "Edit ×5, Bash ×4, Read ×3", "2.5M", "48k", "14k", "$2.10", "personal"},
		{9, "14:20", "3m", 9, "Bash ×3, Edit ×2, Read ×1", "1.3M", "27k", "8.1k", "$1.15", "personal"},
		{8, "13:56", "14m", 70, "Edit ×23, Bash ×17, Read ×14", "9.5M", "210k", "63k", "$8.44", "personal"},
		{7, "13:41", "5m", 20, "Bash ×6, Edit ×5, Read ×4", "2.4M", "60k", "18k", "$2.26", "personal"},
		{6, "13:22", "9m", 27, "Edit ×9, Bash ×6, Read ×5", "2.9M", "81k", "24k", "$2.88", "personal"},
		{5, "13:14", "2m", 6, "Bash ×2, Edit ×1, Read ×1", "600k", "18k", "5.4k", "$0.62", "personal"},
		{4, "12:56", "2m", 6, "Edit ×2, Bash ×1, Read ×1", "519k", "18k", "5.4k", "$0.57", "personal"},
		{3, "12:32", "10m", 30, "Bash ×10, Edit ×7, Read ×6", "2.1M", "90k", "27k", "$2.65", "personal"},
		{2, "12:16", "8m", 32, "Edit ×10, Bash ×8, Read ×6", "1.9M", "96k", "29k", "$2.62", "personal"},
		{1, "11:55", "17m", 51, "Bash ×17, Edit ×12, Read ×10", "2.4M", "153k", "46k", "$3.86", "personal"},
	})
}

func TestB9C4sPageIsItsFrameFromYesterdaysLines(t *testing.T) {
	now := moment(time.Local)
	r := &records{lines: heldAsWritten(endedPlan(now).lines())}
	page := pageOf(t, r, idB9C4, nil, now)

	if page.Totals.Turns != 16 || page.Totals.Requests != 595 || shownWorth(page.Totals.Worth) != "$73.00" {
		t.Errorf("TURNS sums %d turns, %d requests, %s, want 16 turns, 595 requests, $73.00", page.Totals.Turns, page.Totals.Requests,
			shownWorth(page.Totals.Worth))
	}
	// Its stretches are drawn to 16:02 and 18:47, the move and the end of the
	// minute it ended in, where its last requests on work and on side came at
	// 16:01 and 18:46, which the page's stretches and its end follow.
	if got := accountRows(page); !slices.Equal(got, []accountRow{{"work", 330, "$32.51"}, {"side", 265, "$40.49"}}) {
		t.Errorf("the page's accounts = %+v, want work's 330 requests, $32.51, then side's 265, $40.49", got)
	}
	if page.Running || page.KeptUntil != "2026-12-29" {
		t.Errorf("the page has b9c4 running %t, kept until %q, want ended, kept until 2026-12-29, as [ledger] keep at 90 days keeps it", page.Running,
			page.KeptUntil)
	}
	checkMove(t, page, now, "16:02", "work", "side", 188_000, "$1.88")
	// Turn 8's tools are drawn "Edit ×2, Bash ×2, Read ×1", as the generator
	// lists its sample's; the views list those called as often by their names.
	checkTurns(t, page, now, []turnRow{
		{16, "18:36", "11m", 44, "Edit ×14, Bash ×11, Read ×8", "8.4M", "132k", "40k", "$6.49", "side"},
		{15, "17:55", "15m", 45, "Bash ×15, Edit ×11, Read ×9", "8.6M", "135k", "40k", "$6.64", "side"},
		{14, "17:31", "14m", 42, "Edit ×14, Bash ×10, Read ×8", "8.0M", "126k", "38k", "$6.20", "side"},
		{13, "17:16", "3m", 15, "Bash ×5, Edit ×3, Read ×3", "2.9M", "45k", "14k", "$2.21", "side"},
		{12, "17:09", "3m", 12, "Edit ×4, Bash ×3, Read ×2", "2.3M", "36k", "11k", "$1.77", "side"},
		{11, "16:41", "12m", 36, "Bash ×12, Edit ×9, Read ×7", "6.8M", "108k", "32k", "$5.31", "side"},
		{10, "16:22", "11m", 55, "Edit ×18, Bash ×13, Read ×11", "9.9M", "165k", "50k", "$7.86", "side"},
		{9, "15:58", "8m", 32, "Bash ×10, Edit ×8, Read ×6", "5.2M", "284k", "29k", "$6.14", "work → side"},
	})
	if got := rowOf(page.Turns[9], now); got.requests != 20 || got.worth != "$2.40" {
		t.Errorf("turn 7 is drawn %+v, want 20 requests, $2.40", got)
	}
}

func TestF41AsPageIsItsFrameFromItsLinesOfYesterdayAndToday(t *testing.T) {
	now := moment(time.Local)
	r := &records{lines: heldAsWritten(resumedPlan(now).lines())}
	routed := resumedRouted(now)
	page := pageOf(t, r, idF41A, &routed, now)

	if page.Totals.Turns != 21 || page.Totals.Requests != 921 || shownWorth(page.Totals.Worth) != "$128.19" {
		t.Errorf("TURNS sums %d turns, %d requests, %s, want 21 turns, 921 requests, $128.19", page.Totals.Turns, page.Totals.Requests,
			shownWorth(page.Totals.Worth))
	}
	// Side's stretch is drawn to 18:05, where its idle begins, where its last
	// request there came at 18:04.
	if got := accountRows(page); !slices.Equal(got, []accountRow{{"side", 154, "$14.08"}, {"personal", 767, "$114.10"}}) {
		t.Errorf("the page's accounts = %+v, want side's 154 requests, $14.08, then personal's 767, $114.10", got)
	}
	if resumed := page.Resumed.In(now.Location()).Format("15:04"); resumed != "09:12" || page.State != status.Answering {
		t.Errorf("the page has f41a resumed at %s, %s, want resumed at 09:12, answering", resumed, page.State)
	}
	checkMove(t, page, now, "09:12", "side", "personal", 96_000, "none")
	checkTurns(t, page, now, []turnRow{
		{21, "14:25", "18m…", 44, "Bash ×14, Edit ×11, Read ×8", "8.4M", "132k", "40k", "$6.49", "personal"},
		{20, "13:58", "17m", 85, "Edit ×28, Bash ×21, Read ×17", "16.1M", "255k", "76k", "$12.54", "personal"},
		{19, "13:37", "9m", 27, "Bash ×9, Edit ×6, Read ×5", "5.1M", "81k", "24k", "$3.98", "personal"},
		{18, "13:17", "16m", 48, "Edit ×16, Bash ×12, Read ×9", "9.1M", "144k", "43k", "$7.08", "personal"},
		{17, "13:01", "6m", 18, "Bash ×6, Edit ×4, Read ×3", "3.4M", "54k", "16k", "$2.66", "personal"},
		{16, "12:44", "9m", 27, "Edit ×9, Bash ×6, Read ×5", "5.1M", "81k", "24k", "$3.98", "personal"},
		{15, "12:18", "14m", 42, "Bash ×14, Edit ×10, Read ×8", "8.0M", "126k", "38k", "$6.20", "personal"},
		{14, "11:55", "15m", 75, "Edit ×25, Bash ×18, Read ×15", "14.2M", "225k", "68k", "$11.06", "personal"},
		{13, "11:34", "13m", 39, "Bash ×13, Edit ×9, Read ×7", "7.4M", "117k", "35k", "$5.75", "personal"},
		{12, "11:24", "4m", 20, "Edit ×6, Bash ×5, Read ×4", "3.8M", "60k", "18k", "$2.95", "personal"},
	})
}
