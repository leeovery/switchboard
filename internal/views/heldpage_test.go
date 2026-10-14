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

// A held page awaits a line on the day it's filed on, where the stream told
// its request ended, its line yet to come, before today: a line is filed
// under the day its request arrived on, as Told.FiledOn has it, which
// today's tail never reads, the day before for one sent as today began. It
// awaits none once the line is taken, nor for a request in flight, one
// lost, one sent and arrived today, or one the stream told no sending of.
func TestAHeldPageAwaitsTheLineOfARequestThatEndedAcrossMidnight(t *testing.T) {
	night := local(23, 59, 50)
	morning := night.Add(10 * time.Minute)
	lined := filedAs("r2", took(22*time.Second, serving(p1, ledger.ClassMain, line(paged, opus, "side", "sticky", night, hourUsage))))
	sent := toldOf("r2", p1, ledger.ClassMain, opus, "side", night.Add(100*time.Millisecond))
	tests := []struct {
		name   string
		told   views.Told
		taken  bool
		awaits []string
	}{
		{name: "ended after midnight, its line to come", told: endedAt(night.Add(22*time.Second), sent), awaits: []string{"2026-10-07"}},
		{name: "ended, its line taken", told: endedAt(night.Add(22*time.Second), sent), taken: true},
		{name: "in flight", told: sent},
		{name: "lost", told: lostAt(night.Add(time.Minute), sent)},
		{name: "sent today", told: endedAt(morning.Add(time.Second), toldOf("r3", p1, ledger.ClassMain, opus, "side", morning))},
		{name: "sent as today began, arrived the day before", told: endedAt(night.Add(40*time.Second), toldOf("r4", p1, ledger.ClassMain, opus, "side", night.Add(20*time.Second))),
			awaits: []string{"2026-10-07"}},
		{name: "ended, never told sent", told: endedAt(morning.Add(-time.Second), toldOf("r9", p1, ledger.ClassMain, opus, "side", time.Time{}))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			first := []ledger.Line{filedAs("r1", mainOn(p1, local(23, 0, 0), "tool_use"))}
			held := views.HoldPage(paged, views.ReadSessions(summarisedLedger(first), nil, night, paged), ledger.Pricing, kept90, night)
			if tt.taken {
				held = held.Take("2026-10-07", heldOf(lined))
			}
			if got := held.Awaited(views.PageLive{Stream: heard(tt.told), Now: morning}); !slices.Equal(got, tt.awaits) {
				t.Errorf("the page awaits lines on %v, want %v", got, tt.awaits)
			}
		})
	}
}

// Of the days the ledger's tail followed no further, a held page reads
// again whole only the day it's held at and those after it, as it holds
// every line filed before it already.
func TestAHeldPageReadsAgainTheDaysMissedFromTheDayItsHeldAt(t *testing.T) {
	night := local(23, 59, 50)
	first := []ledger.Line{filedAs("r1", mainOn(p1, local(23, 0, 0), "tool_use"))}
	held := views.HoldPage(paged, views.ReadSessions(summarisedLedger(first), nil, night, paged), ledger.Pricing, kept90, night)
	missed := []string{"2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08"}
	tests := []struct {
		name   string
		held   views.HeldPage
		unread []string
	}{
		{name: "held the day before last", held: held, unread: []string{"2026-10-07", "2026-10-08"}},
		{name: "rolled on to the day before", held: held.Take("2026-10-08", nil), unread: []string{"2026-10-08"}},
		{name: "rolled on past them", held: held.Take("2026-10-09", nil)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.held.Unread(missed); !slices.Equal(got, tt.unread) {
				t.Errorf("the page reads again whole the days %v of %v, want %v", got, missed, tt.unread)
			}
		})
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
	rows := turnRowsOf(views.HoldPage(paged, views.ReadSessions(l, nil, at, paged), ledger.Pricing, kept90, at).TurnRequests(1, views.PageLive{Unlisted: true, Now: at}))
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

// turnRowsOf are the rows of a turn's Log lines given.
func turnRowsOf(lines []views.LogLine) []views.RequestRow {
	rows := make([]views.RequestRow, len(lines))
	for i, l := range lines {
		rows[i] = l.Row
	}
	return rows
}

// A turn opened shows its requests oldest first, each the Log's line of it
// with no mark, its details the Log's: by its line, the one that wrote the
// context again on the account it moved to saying so, and where it moved
// from and what that cost; one sent again after a limit, as tried; a 429
// every account refused, failed; and one in flight, as the stream tells it.
// Another turn's requests are none of them.
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
	l := summarisedLedger(lines)
	held := views.HoldPage(paged, views.ReadSessions(l, nil, now, paged), ledger.Pricing, kept90, now)
	stream := heard(toldOf("r8", p1, ledger.ClassMain, opus, "side", local(13, 10, 0)))
	turn := held.TurnRequests(1, views.PageLive{Now: now, Stream: stream})
	table := views.LogRows(views.LogSources{
		From: now, Read: views.ReadLogHeld(l, heldOf(lines...), ledger.Pricing, now), Stream: stream, Now: now,
	})
	logged := logRowsByID(table)
	var ids []string
	for _, line := range turn {
		r := line.Row
		ids = append(ids, r.Request)
		if r.Mark != views.NoMark {
			t.Errorf("%s has mark %q, want none", r.Request, r.Mark)
		}
		want := logged[r.Request]
		want.Mark = views.NoMark
		if !reflect.DeepEqual(r, want) {
			t.Errorf("%s's row is\n%+v\nwant the Log's, but for its mark,\n%+v", r.Request, r, want)
		}
		got, _ := line.Details()
		details := logDetailsOf(t, table, r.Request)
		details.Row.Mark = views.NoMark
		if !reflect.DeepEqual(got, details) {
			t.Errorf("%s's details are\n%+v\nwant the Log's, but for its mark,\n%+v", r.Request, got, details)
		}
	}
	rows := turnRowsOf(turn)
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

// logDetailsOf are the details of the request with the given id, as the
// Log's table holds its line, failing the test where it holds none.
func logDetailsOf(t *testing.T, table views.LogTable, id string) views.RequestDetails {
	t.Helper()
	for _, l := range table.Lines {
		if l.IsRow() && l.Row.Request == id {
			d, _ := l.Details()
			return d
		}
	}
	t.Fatalf("the Log holds no line of %s", id)
	return views.RequestDetails{}
}

// logRowsByID are the rows the Log's table holds, by their requests' ids.
func logRowsByID(table views.LogTable) map[string]views.RequestRow {
	rows := make(map[string]views.RequestRow)
	for _, l := range table.Lines {
		if l.IsRow() {
			rows[l.Row.Request] = l.Row
		}
	}
	return rows
}

// While no router lists the sessions, a page keeps the session's own pin as
// the router's last listing had it, as it stands at the router all the
// same; but not where auto would send it, which the router can't say then,
// nor anything else of that listing: it's no more running than its lines
// say. Where the router answers without listing it, it has no pin.
func TestAPageKeepsItsOwnPinFromTheLastListingWhileNoRouterListsIt(t *testing.T) {
	lines := []ledger.Line{filedAs("p1", mainOn(p1, local(12, 50, 0), "end_turn"))}
	held := views.HoldPage(paged, views.ReadSessions(summarisedLedger(lines), nil, now, paged), ledger.Pricing, kept90, now)
	last := runningOn("side")
	last.Pin, last.Assignments[0].Auto = "side", "work"
	tests := []struct {
		name string
		live views.PageLive
		pin  string
	}{
		{name: "no router listing the sessions", live: views.PageLive{Unlisted: true, LastRouted: last, Now: now}, pin: "side"},
		{name: "no router listing the sessions, nor one ever having listed it", live: views.PageLive{Unlisted: true, Now: now}},
		{name: "the router answering, listing it no more", live: views.PageLive{LastRouted: last, Now: now}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := held.Page(tt.live)
			want, _ := held.Page(views.PageLive{Unlisted: tt.live.Unlisted, Now: now})
			want.Pin = tt.pin
			if !ok || !reflect.DeepEqual(got, want) {
				t.Errorf("the page is\n%+v\nwant it built of its lines alone, its own pin %q\n%+v", got, tt.pin, want)
			}
		})
	}
}

// A session kept where it started says so on its page, sticky, or kept for
// its thinking, whether the router answers or not: the router keeps the
// reason the assignment came by, new, while it keeps a session where it is,
// and its lines say each request kept it. One with no request since its
// first says it started there alone, as does one the router lists, new, on
// another account than a request kept it on.
func TestASessionKeptWhereItStartedSaysSoWhetherTheRouterAnswersOrNot(t *testing.T) {
	asked := func(id, account, reason string, at time.Time) ledger.Line {
		return filedAs(id, stopped("end_turn", took(time.Minute, serving(p1, ledger.ClassMain, line(paged, opus, account, reason, at, hourUsage)))))
	}
	first := asked("p1", "side", status.ReasonNew, local(12, 20, 0))
	listing := func(account, reason string) *status.Session {
		return &status.Session{ID: paged, Assignments: []status.Assignment{{Model: opus, Account: account, Reason: reason, AssignedAt: local(12, 20, 0), LastSeen: local(12, 40, 0)}}}
	}
	pressed := status.PassingOver(status.ReasonNew, "personal")
	tests := []struct {
		name             string
		lines            []ledger.Line
		routed           *status.Session
		listed, unlisted string
		short            string
	}{
		{name: "kept while its cache was warm", lines: []ledger.Line{first, asked("p2", "side", status.ReasonSticky, local(12, 40, 0))},
			routed: listing("side", status.ReasonNew), listed: "on [side] since it started: sticky", unlisted: "on [side] since it started: sticky", short: "sticky"},
		{name: "kept for its thinking once its cache went cold", lines: []ledger.Line{first, asked("p2", "side", status.ReasonBound, local(12, 40, 0))},
			routed:   listing("side", status.ReasonNew),
			listed:   "on [side] since it started: kept for its thinking",
			unlisted: "on [side] since it started: kept for its thinking", short: "kept for its thinking"},
		{name: "started passing an account over for pressure, then kept",
			lines:  []ledger.Line{asked("p1", "side", pressed, local(12, 20, 0)), asked("p2", "side", status.ReasonSticky, local(12, 40, 0))},
			routed: listing("side", pressed), listed: "on [side] since it started: sticky", unlisted: "on [side] since it started: sticky", short: "sticky"},
		{name: "rescored where it ran, then kept, as it was rescored",
			lines: []ledger.Line{asked("p1", "side", status.ReasonNew, local(10, 0, 0)), asked("p2", "side", "rescored after 2h idle", local(12, 20, 0)),
				asked("p3", "side", status.ReasonSticky, local(12, 40, 0))},
			routed:   listing("side", "rescored after 2h idle"),
			listed:   "on [side] since 12:20, rescored after 2h idle: [side] had the most room",
			unlisted: "on [side]: rescored after 2h idle: [side] had the most room"},
		{name: "its first request alone", lines: []ledger.Line{first},
			routed: listing("side", status.ReasonNew), listed: "on [side] since it started", unlisted: "on [side] since it started"},
		{name: "listed new on another account than a request kept it on",
			lines:  []ledger.Line{asked("p1", "work", status.ReasonNew, local(12, 20, 0)), asked("p2", "work", status.ReasonSticky, local(12, 40, 0))},
			routed: listing("side", status.ReasonNew), listed: "on [side] since it started", unlisted: "on [work] since it started: sticky"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			held := views.HoldPage(paged, views.ReadSessions(summarisedLedger(tt.lines), []status.Session{*tt.routed}, now, paged), ledger.Pricing, kept90, now)
			for _, c := range []struct {
				live views.PageLive
				want string
			}{
				{live: views.PageLive{Routed: tt.routed, Now: now}, want: tt.listed},
				{live: views.PageLive{Unlisted: true, LastRouted: tt.routed, Now: now}, want: tt.unlisted},
			} {
				page, ok := held.Page(c.live)
				if !ok {
					t.Fatalf("no page, the router answering %v", !c.live.Unlisted)
				}
				where := page.Where(now)
				if got := wordsSaid(where.Words()); got != c.want {
					t.Errorf("row 6, the router answering %v, says %q, want %q", !c.live.Unlisted, got, c.want)
				}
				if got := wordsSaid(where.Short); tt.listed == tt.unlisted && got != tt.short {
					t.Errorf("a phone's row 8, the router answering %v, says %q, want %q", !c.live.Unlisted, got, tt.short)
				}
			}
		})
	}
}
