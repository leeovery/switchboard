package views

import (
	"fmt"
	"iter"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// readsEveryDay reads a session's lines through l of every day, whatever spans
// it's asked for: as a turn was read before it read its own days alone.
type readsEveryDay struct {
	l LedgerSession
}

func (e readsEveryDay) Session(id string, _ ...ledger.Span) iter.Seq[ledger.Held] {
	return e.l.Session(id)
}

// datesGiven reads a session's lines through l, noting in dates the date of
// the day each line it gives arrived on.
type datesGiven struct {
	l     LedgerSession
	dates map[string]bool
}

func (d datesGiven) Session(id string, within ...ledger.Span) iter.Seq[ledger.Held] {
	return func(yield func(ledger.Held) bool) {
		for held := range d.l.Session(id, within...) {
			d.dates[dayfile.DateOf(held.At)] = true
			if !yield(held) {
				return
			}
		}
	}
}

// Every turn of a generated session's page, opened at each look of a run of
// feeds, and again three hours, a week and a month after its last request,
// as requests of the session's own come then, shows the rows it shows read
// of every day: a turn across midnight, a subagent's request of it on a
// later day than it began, a turn still pending, a turn whose last request
// ended days after it began, and a turn days back holding a subagent's
// request filed today, since the feed, among them. A turn done reads no
// line of a day further than one from its own, from arrivesWithin before it
// began to when its last row ended, nor from the horizon's on: a turn a
// week or a month back passes over the days between.
func TestEveryTurnOpenedShowsTheRowsReadOfEveryDay(t *testing.T) {
	prices := changingPrices()
	met := make(map[string]bool)
	for seed := range uint64(60) {
		lines, late := pagedRun(seed)
		r := rand.New(rand.NewPCG(seed, 12))
		first := slices.IndexFunc(lines, func(l ledger.Line) bool { return l.Session == "s" })
		at := lines[first].At
		p := &paging{t: t, seed: seed, lines: lines, late: late, prices: prices, met: make(map[string]bool), fed: at, untold: untoldOf(lines, seed)}
		p.page = ReadPage(filedLedger{lines: lines, now: at}, "s", p.streamAt(at), prices, 0, at)
		p.store = HoldSessions(prices).With(p.page.Found())
		p.page = p.page.Joined(p.store)
		for i := first; i < len(lines); i += 1 + r.IntN(6) {
			now := later(instantOf(at), instantOf(ended(&lines[i]))).time().Add(time.Duration(r.IntN(5000)) * time.Millisecond)
			p.look(now, false)
			p.everyTurn(now, met)
			at = now
		}
		last := ended(&lines[len(lines)-1])
		for i, after := range []time.Duration{3 * time.Hour, 7 * 24 * time.Hour, 30 * 24 * time.Hour} {
			p.lookLater(i, last.Add(after), met)
		}
	}
	for _, moment := range []string{
		"a turn across midnight", "a subagent's request of a turn on a later day than it began", "a turn pending",
		"a turn passing over a day of its session's lines", "a turn done holding a subagent's request filed since the feed, on the horizon's day",
		"a turn done whose last request ended days after it began",
	} {
		if !met[moment] {
			t.Errorf("no generated run meets %s", moment)
		}
	}
}

// A turn reads from arrivesWithin before it began to when its last request
// ended, or to now while one is pending, and from the horizon to now: every
// day where the horizon is before every moment.
func TestATurnReadsItsOwnSpanAndTheHorizons(t *testing.T) {
	now := time.Date(2026, 10, 19, 15, 0, 0, 0, time.Local)
	started, ended := now.AddDate(0, 0, -7), now.AddDate(0, 0, -6)
	horizon := instantOf(now.Add(-time.Hour))
	tests := []struct {
		name    string
		horizon instant
		pending bool
		want    []ledger.Span
	}{
		{"done", horizon, false, []ledger.Span{{From: started.Add(-arrivesWithin), To: ended}, {From: horizon.time(), To: now}}},
		{"pending", horizon, true, []ledger.Span{{From: started.Add(-arrivesWithin), To: now}, {From: horizon.time(), To: now}}},
		{"its horizon before every moment", unsealed, false, nil},
	}
	for _, tt := range tests {
		w := worked{rec: &heldSession{horizon: tt.horizon}, live: PageLive{Now: now}}
		turn := &turnTally{started: started, last: turnEnd{end: ended, set: true}, pending: tt.pending}
		if got := w.turnSpans(turn); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: a turn reads within %v, want %v", tt.name, got, tt.want)
		}
	}
}

// lookLater looks at the session at now, the i'th time after its run: a
// subagent's first request, naming its first prompt, came three days
// before, or a minute after the last feed where that's later, and a request
// of its own ten minutes before, both fed; and five minutes before, another
// subagent's first, untold by the stream, naming its first prompt, filed
// since the feed. Then every turn is opened, as everyTurn has it.
func (p *paging) lookLater(i int, now time.Time, met map[string]bool) {
	p.t.Helper()
	own := ledger.Line{
		Request: fmt.Sprintf("later%d", i), At: now.Add(-10 * time.Minute).Truncate(time.Millisecond).UTC(), Kind: ledger.KindMessage, Session: "s",
		Model: foldOpus, Account: "work", Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 3000, Class: ledger.ClassMain,
		Prompt: fmt.Sprintf("plater%d", i),
	}
	agent := func(name string, at time.Time) ledger.Line {
		l := own
		l.Request, l.At, l.Class, l.AgentID, l.Prompt = fmt.Sprintf("%s%d", name, i), at.Truncate(time.Millisecond).UTC(), "subagent", fmt.Sprintf("a-%s%d", name, i), "p1"
		return l
	}
	fed, untold := agent("fed", later(instantOf(p.fed.Add(time.Minute)), instantOf(now.Add(-72*time.Hour))).time()), agent("untold", now.Add(-5*time.Minute))
	p.untold[untold.Request] = true
	p.lines = append(p.lines, fed, own, untold)
	p.look(now.Add(-7*time.Minute), false)
	p.everyTurn(now, met)
}

// everyTurn holds each turn of the page, opened at now, read of its own days
// and the horizon's, to the turn read of every day, row for row; and holds
// the days it read to its own and the horizon's, noting in met the hard
// moments it meets.
func (p *paging) everyTurn(now time.Time, met map[string]bool) {
	p.t.Helper()
	live := PageLive{Stream: p.streamAt(now), Now: now}
	l := filedLedger{lines: p.lines, now: now}
	w := p.page.worked(live)
	for n, turn := range w.tally.turns {
		read := datesGiven{l: l, dates: make(map[string]bool)}
		got, want := p.turnRows(n+1, read, live), p.turnRows(n+1, readsEveryDay{l}, live)
		if !reflect.DeepEqual(rowsOf(got), rowsOf(want)) {
			p.t.Fatalf("session %d's turn %d, opened at %v, has the rows\n%+v\nwant\n%+v, as it reads every day", p.seed, n+1, now, rowsOf(got), rowsOf(want))
		}
		if turn.pending {
			met["a turn pending"] = true
			continue
		}
		p.heldToItsDays(n+1, turn, want, read.dates, w, l, met)
	}
}

// heldToItsDays fails where the turn numbered n, done, with the rows given,
// read a line of a day further than one from its own, from arrivesWithin
// before it began to when its last row ended, and from the horizon's on, as
// dayfile.Dates has them; and notes in met the hard moments it meets.
func (p *paging) heldToItsDays(n int, turn *turnTally, rows []LogLine, read map[string]bool, w worked, l filedLedger, met map[string]bool) {
	p.t.Helper()
	end := turn.started
	for _, r := range rows {
		if r.record == nil {
			continue
		}
		end = later(instantOf(end), r.record.end).time()
		if r.record.agentID != "" && dayfile.DateOf(r.record.arrived()) > dayfile.DateOf(turn.started) {
			met["a subagent's request of a turn on a later day than it began"] = true
		}
		if !w.rec.linedAt(r.record.request, r.record.arrived()) && !slices.Contains(dayfile.Dates(turn.started, turn.last.end), dayfile.DateOf(r.record.arrived())) {
			met["a turn done holding a subagent's request filed since the feed, on the horizon's day"] = true
		}
	}
	if dayfile.DateOf(end) > dayfile.DateOf(turn.started) {
		met["a turn across midnight"] = true
	}
	if !slices.Contains(dayfile.Dates(turn.started, turn.started), dayfile.DateOf(turn.last.end)) {
		met["a turn done whose last request ended days after it began"] = true
	}
	if w.rec.horizon == unsealed {
		return
	}
	own := slices.Concat(dayfile.Dates(turn.started.Add(-arrivesWithin), end), dayfile.Dates(w.rec.horizon.time(), w.live.Now))
	for date := range read {
		if !slices.Contains(own, date) {
			p.t.Fatalf("session %d's turn %d, opened at %v, read a line of %s, want its own days and the horizon's alone, %v", p.seed, n, w.live.Now, date, own)
		}
	}
	for _, line := range l.filed() {
		if date := dayfile.DateOf(line.At); line.Session == "s" && !slices.Contains(own, date) {
			met["a turn passing over a day of its session's lines"] = true
		}
	}
}

// turnRows are the rows of the page's turn numbered n, opened, read through
// l, as live builds the page.
func (p *paging) turnRows(n int, l LedgerSession, live PageLive) []LogLine {
	page := p.page.Opening(n, live)
	page = page.Turned(page.ReadTurn(l, live))
	rows, loading := page.worked(live).turnRequests(n)
	if loading {
		p.t.Fatalf("session %d's turn %d, read at %v, reads its requests still, want them read", p.seed, n, live.Now)
	}
	return rows
}

// rowsOf are the rows the lines given show.
func rowsOf(lines []LogLine) []RequestRow {
	rows := make([]RequestRow, len(lines))
	for i, l := range lines {
		rows[i] = l.Row
	}
	return rows
}
