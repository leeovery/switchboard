package views_test

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// kept90 is how long the held pages' tests' ledger keeps a day's lines, as
// pageOf keeps them.
const kept90 = 90 * 24 * time.Hour

// heldOf is lines as the ledger's Today gives them.
func heldOf(lines ...ledger.Line) []ledger.Held {
	held := make([]ledger.Held, len(lines))
	for i, l := range lines {
		held[i] = ledger.Held{Line: l}
	}
	return held
}

// A page held at noon, then fed the lines its day gained since, the
// session's and another's on its accounts, one it held already among them,
// is the page built whole of every line at once.
func TestAPageHeldThenFedWhatItsDayGainedIsThePageBuiltWhole(t *testing.T) {
	fiveHour := local(15, 0, 0)
	ours := func(id, account, reason, prompt string, at time.Time, use string) ledger.Line {
		return limited(filedAs(id, serving(prompt, ledger.ClassMain, line(paged, opus, account, reason, at, hourUsage))), "5h", use, fiveHour)
	}
	theirs := func(id string, at time.Time, use string) ledger.Line {
		return limited(filedAs(id, line(other, opus, "work", "sticky", at, hourUsage)), "5h", use, fiveHour)
	}
	early := []ledger.Line{
		ours("p1", "work", "new", p1, local(11, 0, 0), "0.10"),
		theirs("o1", local(11, 5, 0), "0.12"),
		ours("p2", "work", "sticky", p1, local(11, 30, 0), "0.15"),
	}
	gained := []ledger.Line{
		movedFrom("work", ours("p3", "side", "moved: work hit its limit", p2, local(12, 10, 0), "0.05")),
		theirs("o2", local(12, 20, 0), "0.30"),
		ours("p4", "side", "sticky", p2, local(12, 30, 0), "0.08"),
	}
	noon := local(12, 0, 0)
	read := views.ReadSessions(fakeLedger{now: noon, lines: early}, nil, noon, paged)
	held := views.HoldPage(paged, read, ledger.Pricing, kept90, noon).Take("2026-10-07", heldOf(append(gained, early[2])...))
	got, ok := held.Page(views.PageLive{Now: now})
	if want := pageOf(t, slices.Concat(early, gained), nil); !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("the page held, then fed its gain, is\n%+v\nwant it as built whole\n%+v", got, want)
	}
}

// A held page reads the ledger once, as it's held: each look after, and each
// gain it takes, builds of what it holds.
func TestAHeldPageReadsTheLedgerOnceHoweverOftenItsLookedAt(t *testing.T) {
	counts := &readCounts{}
	l := countedLedger{counts: counts, fakeLedger: summarisedLedger([]ledger.Line{filedAs("p1", mainOn(p1, local(12, 0, 0), "tool_use"))})}
	held := views.HoldPage(paged, views.ReadSessions(l, nil, now, paged), ledger.Pricing, kept90, now)
	for i := range 5 {
		held = held.Take("2026-10-07", heldOf(filedAs("p"+string(rune('2'+i)), mainOn(p1, local(12, 1+i, 0), "tool_use"))))
		if page, ok := held.Page(views.PageLive{Now: now.Add(time.Duration(i) * time.Second)}); !ok || page.Totals.Requests != 2+i {
			t.Fatalf("look %d gave %+v, %v; want the page, of its %d requests", i, page.Totals, ok, 2+i)
		}
		held.TurnRequests(1, views.PageLive{Now: now})
	}
	if want := (readCounts{sessionsRead: 1}); *counts != want {
		t.Errorf("the held page read the ledger %+v, want it read once, as it was held", *counts)
	}
}

// A held page built at a day that has turned prices its requests at the new
// day's prices, as a page built afresh then would: looked at then, or rolled
// on to it as it takes in the new day's lines, none here.
func TestAHeldPagePricesAfreshAsTheDayTurns(t *testing.T) {
	lines := []ledger.Line{filedAs("p1", mainOn(p1, local(12, 0, 0), "end_turn"))}
	tomorrow := now.AddDate(0, 0, 1)
	held := views.HoldPage(paged, views.ReadSessions(summarisedLedger(lines), nil, now, paged), ledger.Pricing, kept90, now)
	read := views.ReadSessions(fakeLedger{now: tomorrow, lines: lines, summaries: summarisedLedger(lines).summaries}, nil, tomorrow, paged)
	want, _ := views.SessionPageOf(paged, views.PageSources{Read: read, Prices: ledger.Pricing, Keep: kept90, Now: tomorrow})
	for name, h := range map[string]views.HeldPage{"looked at": held, "rolled on": held.Take(views.Today(tomorrow).Date, nil)} {
		if got, _ := h.Page(views.PageLive{Now: tomorrow}); !reflect.DeepEqual(got, want) {
			t.Errorf("the page held a day, %s, is\n%+v\nwant it as built that day\n%+v", name, got, want)
		}
	}
}

// turnRow is what a turn's request row holds that a frame draws: when it
// came, its model and account, its status, its first byte and its end,
// the tokens it read, wrote and put out, why its answer stopped and the
// tool it called first.
type turnRow struct {
	at                 time.Time
	model, account     string
	status             int
	first, took        time.Duration
	read, written, out int
	stop, tool         string
}

// requested is a request of paged's turn p1 of the model given on the
// account given at at, its first byte and end after it as given, reading,
// writing for an hour and putting out the tokens given, its answer stopping
// as stop says after calling the tools given.
func requested(id, model, account string, at time.Time, first, end time.Duration, read, written, out int, stop string, tools ...string) ledger.Line {
	usage := `{"input_tokens":3,"cache_creation_input_tokens":` + strconv.Itoa(written) + `,"cache_read_input_tokens":` + strconv.Itoa(read) +
		`,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":` + strconv.Itoa(written) + `},"output_tokens":` + strconv.Itoa(out) + `}`
	class := ledger.ClassMain
	if model == haiku {
		class = "auxiliary"
	}
	return filedAs(id, stopped(stop, headersAfter(first, took(end, serving(p1, class, line(paged, model, account, "sticky", at, usage)))), tools...))
}

// The first five requests of d28c's turn 12, opened, are the frame's rows:
// each its line's, the move at work's cap writing the context again on
// side, at Claude Opus 5.5's price for an hour's write of its 182k tokens.
func TestATurnOpenedIsTheFramesRowsOfItsRequests(t *testing.T) {
	sec := func(tenths int) time.Duration { return time.Duration(tenths) * time.Second / 10 }
	moved := requested("t3", opus, "side", local(14, 31, 13), sec(29), sec(146), 0, 182_000, 1200, "tool_use", "Bash")
	moved.From, moved.Reason = "work", "moved: work is at its reserve"
	lines := []ledger.Line{
		requested("t1", opus, "work", local(14, 29, 4), sec(8), sec(112), 176_000, 2100, 1300, "tool_use", "Bash"),
		requested("t2", opus, "work", local(14, 30, 42), sec(9), sec(124), 181_000, 2400, 1100, "tool_use", "Bash"),
		moved,
		requested("t4", haiku, "side", local(14, 31, 44), sec(3), sec(6), 0, 0, 40, "end_turn"),
		requested("t5", opus, "side", local(14, 31, 58), sec(11), sec(98), 182_000, 2600, 900, "tool_use", "Edit"),
	}
	at := local(14, 42, 7)
	l := summarisedLedger(lines)
	l.now = at
	rows := views.HoldPage(paged, views.ReadSessions(l, nil, at, paged), ledger.Pricing, kept90, at).TurnRequests(1, views.PageLive{Unlisted: true, Now: at})
	want := []turnRow{
		{local(14, 29, 4), opus, "work", 200, sec(8), sec(112), 176_000, 2100, 1300, "tool_use", "Bash"},
		{local(14, 30, 42), opus, "work", 200, sec(9), sec(124), 181_000, 2400, 1100, "tool_use", "Bash"},
		{local(14, 31, 13), opus, "side", 200, sec(29), sec(146), 0, 182_000, 1200, "tool_use", "Bash"},
		{local(14, 31, 44), haiku, "side", 200, sec(3), sec(6), 0, 0, 40, "end_turn", ""},
		{local(14, 31, 58), opus, "side", 200, sec(11), sec(98), 182_000, 2600, 900, "tool_use", "Edit"},
	}
	got := make([]turnRow, len(rows))
	for i, r := range rows {
		got[i] = turnRow{r.At.Local(), r.Model, r.Account, r.Status, *r.First, *r.Took, r.Read, r.Written, r.Out, r.Stop, r.Tool}
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("turn 12's rows are\n%+v\nwant the frame's\n%+v", got, want)
	}
	if r := rows[2]; !r.Rewrote || wordsSaid(r.Routing.Words) != "from [work], at its cap" || r.Routing.Cost == nil || *r.Routing.Cost != micro(182_000*8) {
		t.Errorf("the move's row wrote again %t, routed %q, costing %v; want it written again, from work, at its cap, costing 182k written for an hour",
			r.Rewrote, wordsSaid(r.Routing.Words), r.Routing.Cost)
	}
}

// A turn opened shows its requests oldest first, each the Log's row of it
// with no mark: by its line, the one that wrote the context again on the
// account it moved to saying so, and where it moved from and what that
// cost; one sent again after a limit, as tried; a 429 every account
// refused, failed; and one in flight, as the stream tells it. Another
// turn's requests are none of them.
func TestATurnsRequestsAreTheLogsRowsOfThemOldestFirstWithNoMark(t *testing.T) {
	main := func(id, account string, at time.Time) ledger.Line {
		return filedAs(id, took(10*time.Second, serving(p1, ledger.ClassMain, line(paged, opus, account, "sticky", at, hourUsage))))
	}
	moved := main("r3", "side", local(12, 31, 13))
	moved.From, moved.Reason = "work", "moved: work is at its reserve"
	tried := main("r6", "side", local(12, 33, 0))
	tried.Tried = []ledger.Tried{{Account: "work", Why: status.WhyLimit}}
	refused := main("r7", "side", local(12, 34, 0))
	refused.Status = 429
	lines := []ledger.Line{
		main("r1", "work", local(12, 29, 4)),
		main("r2", "work", local(12, 30, 42)),
		moved,
		filedAs("r4", took(time.Second, serving(p1, "auxiliary", line(paged, haiku, "side", "sticky", local(12, 31, 44), smallUsage)))),
		filedAs("r5", serving(p2, ledger.ClassMain, line(paged, opus, "side", "sticky", local(12, 32, 0), hourUsage))),
		tried,
		refused,
	}
	held := views.HoldPage(paged, views.ReadSessions(summarisedLedger(lines), nil, now, paged), ledger.Pricing, kept90, now)
	rows := held.TurnRequests(1, views.PageLive{Now: now, Stream: heard(toldOf("r8", p1, ledger.ClassMain, opus, "side", local(13, 10, 0)))})
	var ids []string
	for _, r := range rows {
		ids = append(ids, r.Request)
		if r.Mark != views.NoMark {
			t.Errorf("%s has mark %q, want none", r.Request, r.Mark)
		}
	}
	if want := []string{"r1", "r2", "r3", "r4", "r6", "r7", "r8"}; !slices.Equal(ids, want) {
		t.Fatalf("turn 1's requests are %v, want %v, oldest first, turn 2's none of them", ids, want)
	}
	if r := rows[2]; !r.Rewrote || wordsSaid(r.Routing.Words) != "from [work], at its cap" || r.Routing.Cost == nil || *r.Routing.Cost != micro(hourWrite) {
		t.Errorf("the move's row wrote again %t, routed %q, costing %v; want it written again, from work, at its cap, costing its write",
			r.Rewrote, wordsSaid(r.Routing.Words), r.Routing.Cost)
	}
	if r := rows[4]; wordsSaid(r.Routing.Words) != "after [work]'s limit" {
		t.Errorf("the row sent again is routed %q, want after work's limit", wordsSaid(r.Routing.Words))
	}
	if r := rows[5]; !r.Failed || r.Status != 429 {
		t.Errorf("the 429 every account refused is failed %t, status %d; want failed, 429", r.Failed, r.Status)
	}
	if r := rows[6]; r.Stage != views.RowAsking {
		t.Errorf("the request in flight is at stage %v, want asking, as the stream tells it", r.Stage)
	}
	if got := held.TurnRequests(3, views.PageLive{Now: now}); got != nil {
		t.Errorf("TurnRequests(3) = %+v, want none, the session having two turns", got)
	}
}
