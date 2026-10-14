package views_test

import (
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// tableSaid says each of a table's lines: a row as its mark, its request's
// id and its stage, ↑ standing for in flight; and a rule as the dates of the
// days it falls between and the requests of the day that ended.
func tableSaid(table views.LogTable) []string {
	said := make([]string, len(table.Lines))
	for i, l := range table.Lines {
		if r := l.Rule; r != nil {
			said[i] = "── " + r.Ended.Date + " " + strings.Repeat("·", r.Totals.Requests) + " → " + r.Began.Date
			continue
		}
		said[i] = strings.TrimSpace(l.Row.Mark.String() + " " + l.Row.Request + " " + stageNames[l.Row.Stage])
	}
	return said
}

// stageNames name how far a row has come, for the tests to say it.
var stageNames = map[views.RowStage]string{
	views.RowLined: "lined", views.RowAsking: "asking", views.RowAnswering: "answering", views.RowEnded: "ended", views.RowLost: "lost",
}

// streamOf is a live stream that told of the requests given.
func streamOf(told ...views.Told) views.Stream {
	s := views.Stream{Live: true, Requests: make(map[string]views.Told, len(told))}
	for _, t := range told {
		s.Requests[t.Request] = t
	}
	return s
}

// sentAt is a request of mover's on side the stream told of, sent at sent.
func sentAt(id string, sent time.Time) views.Told {
	return views.Told{Request: id, Session: mover, Model: opus, Account: "side", Sent: sent, Heard: sent}
}

// ledgerHolding is the ledger holding lines, at now, as a Follower reads
// it: each session's oldest first, and each day's summary naming the
// sessions of its lines.
func ledgerHolding(now time.Time, lines ...ledger.Line) fakeLedger {
	held := slices.SortedStableFunc(slices.Values(lines), func(a, b ledger.Line) int { return a.At.Compare(b.At) })
	named := make(map[string][]string)
	for _, l := range held {
		if date := l.At.Local().Format(time.DateOnly); l.Session != "" && !slices.Contains(named[date], l.Session) {
			named[date] = append(named[date], l.Session)
		}
	}
	f := fakeLedger{lines: held, now: now}
	for _, date := range slices.Sorted(maps.Keys(named)) {
		f.summaries = append(f.summaries, naming(date, named[date]...))
	}
	return f
}

// logRead is the Log's read of lines, in the order given, the ledger at now
// holding them alone.
func logRead(now time.Time, lines ...ledger.Line) views.LogRead {
	return views.ReadLogHeld(ledgerHolding(now, lines...), heldOf(lines...), ledger.Pricing, now)
}

// heldOf are lines as the ledger holds them.
func heldOf(lines ...ledger.Line) []ledger.Held {
	held := make([]ledger.Held, len(lines))
	for i, l := range lines {
		held[i] = ledger.Held{Line: l}
	}
	return held
}

// arrived returns l, its request arriving at at.
func arrived(at time.Time, l ledger.Line) ledger.Line {
	l.At = at
	return l
}

func TestEachRequestIsOneRowByItsIdFromItsLineWhereItHasCome(t *testing.T) {
	sent := local(13, 0, 0)
	ended := sentAt("r2", sent.Add(time.Second))
	ended.Ended, ended.Status = sent.Add(5*time.Second), 200
	got := views.LogRows(views.LogSources{
		From: now, Now: now,
		Read: logRead(now,
			withID("r1", took(5*time.Second, line(mover, opus, "work", "new", sent.Add(-time.Second), hourUsage))),
			withID("r3", line(mover, opus, "work", "sticky", sent.Add(10*time.Second), hourUsage)),
		),
		Stream: streamOf(sentAt("r1", sent), ended, sentAt("r4", sent.Add(20*time.Second))),
	})
	if said, want := tableSaid(got), []string{"▲ r1 lined", "r2 ended", "r3 lined", "↑ r4 asking"}; !slices.Equal(said, want) {
		t.Errorf("the table says %q, want %q: each request once, from its line where it has come", said, want)
	}
}

func TestALineAndARequestTheStreamToldOfAreOneRowOnlyWhereTheyreTheSameRequest(t *testing.T) {
	// The router's ids are unique only while it runs: r1's line is of a run
	// before the one now, and r2's of a run after the one that lost it.
	earlier := withID("r1", took(time.Minute, line(mover, opus, "work", "new", local(11, 0, 0), hourUsage)))
	later := withID("r2", took(time.Minute, line(mover, opus, "work", "sticky", local(12, 0, 0), hourUsage)))
	lost := sentAt("r2", local(11, 30, 0))
	lost.Lost = true
	got := views.LogRows(views.LogSources{
		From: now, Now: now, Read: logRead(now, earlier, later), Stream: streamOf(sentAt("r1", local(13, 0, 0)), lost),
	})
	if said, want := tableSaid(got), []string{"▲ r1 lined", "■ r2 lost", "r2 lined", "↑ r1 asking"}; !slices.Equal(said, want) {
		t.Errorf("the table says %q, want %q: each line where it arrived, and each request told of alone, its id another run's", said, want)
	}
}

func TestARowSitsWhereItWasSentSoItNeverMovesAsItsLineFillsItIn(t *testing.T) {
	sent := local(13, 0, 0)
	// r1 arrived a moment before r2, but went out after it: r2, which the
	// router answered itself, never went out.
	r1 := withID("r1", took(5*time.Second, arrived(sent.Add(-100*time.Millisecond), line(mover, opus, "work", "new", time.Time{}, hourUsage))))
	r2 := withID("r2", arrived(sent.Add(-50*time.Millisecond), line(mover, opus, "", "no account has room", time.Time{}, "")))
	r2.Status, r2.Attempts = 429, 0
	inFlight := views.LogSources{From: now, Now: now, Read: logRead(now, r2), Stream: streamOf(sentAt("r1", sent))}
	lined := inFlight
	lined.Read = logRead(now, r1, r2)
	for name, src := range map[string]views.LogSources{"in flight": inFlight, "its line come": lined} {
		got := views.LogRows(src)
		if rows := tableSaid(got); len(rows) != 2 || !strings.HasSuffix(rows[0], "r2 lined") || !strings.Contains(rows[1], "r1") {
			t.Errorf("%s, the table says %q, want r2 first, then r1, where it was sent", name, rows)
		}
		if r := got.Lines[1].Row; name == "its line come" && !r.At.Equal(r1.At) {
			t.Errorf("its line come, r1 shows at %v, want its arrival, %v", r.At, r1.At)
		}
	}
}

func TestChecksAndCountsAreHiddenUntilShownAndCountedFromTheRows(t *testing.T) {
	kinded := func(id, kind string, l ledger.Line) ledger.Line {
		l.Kind = kind
		return withID(id, l)
	}
	lines := []ledger.Line{
		withID("r1", line(mover, opus, "work", "new", local(10, 0, 0), hourUsage)),
		kinded("c1", ledger.KindCheck, line(mover, haiku, "work", "new", local(10, 1, 0), smallUsage)),
		kinded("c2", ledger.KindCheck, line(mover, haiku, "work", "new", local(10, 2, 0), smallUsage)),
		kinded("n1", ledger.KindCount, line(mover, opus, "work", "sticky", local(10, 3, 0), "")),
	}
	checking := sentAt("c3", local(10, 4, 0))
	checking.Check = true
	tests := []struct {
		name     string
		kinds    []views.RowKind
		rows     []string
		shown    int
		hiddenBy []views.RowKind
	}{
		{name: "both hidden, as the Log opens", kinds: views.LogOpening().Kinds, rows: []string{"▲ r1 lined"}, shown: 1,
			hiddenBy: []views.RowKind{views.RowCheck, views.RowCount}},
		{name: "checks shown", kinds: []views.RowKind{views.RowRequest, views.RowCheck}, rows: []string{"▲ r1 lined", "· c1 lined", "· c2 lined", "· c3 asking"},
			shown: 4, hiddenBy: []views.RowKind{views.RowCount}},
		{name: "both shown", rows: []string{"▲ r1 lined", "· c1 lined", "· c2 lined", "· n1 lined", "· c3 asking"}, shown: 5},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := views.LogRows(views.LogSources{
				From: now, Now: now, Read: logRead(now, lines...), Stream: streamOf(checking), Filter: views.LogFilter{Kinds: tt.kinds},
			})
			if said := tableSaid(got); !slices.Equal(said, tt.rows) {
				t.Errorf("the table says %q, want %q", said, tt.rows)
			}
			c := got.Count(views.Today(now).Date)
			if c.Requests != 1 || c.Checks != 3 || c.Counts != 1 || c.Shown != tt.shown || c.Shown != len(got.Lines) || c.Hidden() != 5-tt.shown {
				t.Errorf("today counts %+v, want 1 request, 3 checks and 1 count, %d of them shown, the rows the table holds", c, tt.shown)
			}
			if by := c.HiddenBy(); !slices.Equal(by, tt.hiddenBy) {
				t.Errorf("today's hidden are hidden by %v, want %v", by, tt.hiddenBy)
			}
		})
	}
}

func TestADaysRuleFallsBeforeTheNextDaysFirstRowShown(t *testing.T) {
	on := func(id string, day, hour int) ledger.Line {
		return withID(id, line(mover, opus, "work", "sticky", october(day, hour, 0), hourUsage))
	}
	check := on("c1", 7, 0)
	check.Kind = ledger.KindCheck
	totals := []views.DayTotals{{Date: "2026-10-04", Requests: 1}, {Date: "2026-10-05", Requests: 2}}
	tests := []struct {
		name  string
		from  time.Time
		lines []ledger.Line
		want  []string
	}{
		{name: "none above the first day's first row", from: october(5, 0, 0), lines: []ledger.Line{on("r1", 5, 9), on("r2", 5, 10)},
			want: []string{"▲ r1 lined", "r2 lined"}},
		{name: "between two days", from: october(5, 0, 0), lines: []ledger.Line{on("r1", 5, 9), on("r2", 5, 10), on("r3", 6, 9)},
			want: []string{"▲ r1 lined", "r2 lined", "── 2026-10-05 ·· → 2026-10-06", "r3 lined"}},
		{name: "one across the days between with none", from: october(4, 0, 0), lines: []ledger.Line{on("r1", 4, 9), on("r2", 7, 9)},
			want: []string{"▲ r1 lined", "── 2026-10-04 · → 2026-10-07", "r2 lined"}},
		{name: "none for the next day's hidden rows alone", from: october(5, 0, 0), lines: []ledger.Line{on("r1", 5, 9), check},
			want: []string{"▲ r1 lined"}},
		{name: "none above the rows of the span's first day, the day before outside it", from: october(6, 0, 0),
			lines: []ledger.Line{on("r1", 5, 9), on("r2", 6, 9)}, want: []string{"r2 lined"}},
	}
	for _, tt := range tests {
		got := views.LogRows(views.LogSources{From: tt.from, Now: now, Read: logRead(now, tt.lines...), Totals: totals, Filter: views.LogOpening()})
		if said := tableSaid(got); !slices.Equal(said, tt.want) {
			t.Errorf("%s: the table says %q, want %q", tt.name, said, tt.want)
		}
	}
}

func TestADaysRuleSaysTheFiguresOfTheDayThatEndedOrItsDateAlone(t *testing.T) {
	lines := []ledger.Line{withID("r1", line(mover, opus, "work", "new", october(6, 9, 0), hourUsage)), withID("r2", line(mover, opus, "work", "sticky", october(7, 9, 0), hourUsage))}
	figures := views.DayTotals{Date: "2026-10-06", Requests: 1912, Sessions: 17, Limits: 2, Worth: priced(212_000_000)}
	for name, tt := range map[string]struct {
		totals []views.DayTotals
		want   views.DayTotals
	}{
		"given": {totals: []views.DayTotals{figures}, want: figures},
		"not":   {want: views.DayTotals{Date: "2026-10-06"}},
	} {
		got := views.LogRows(views.LogSources{From: october(6, 0, 0), Now: now, Read: logRead(now, lines...), Totals: tt.totals})
		if len(got.Lines) != 3 || got.Lines[1].Rule == nil {
			t.Fatalf("figures %s, the table says %q, want a rule between two rows", name, tableSaid(got))
		}
		rule := got.Lines[1].Rule
		if !reflect.DeepEqual(rule.Totals, tt.want) || rule.Ended != views.Today(october(6, 9, 0)) || rule.Began != views.Today(now) {
			t.Errorf("figures %s, the rule is %+v, want the figures %+v, from 6 October's to today's", name, *rule, tt.want)
		}
	}
}

func TestARequestInFlightAsTheDayEndedSitsAboveTheNextDaysRule(t *testing.T) {
	sent := october(6, 23, 59).Add(50 * time.Second)
	late := withID("r2", arrived(sent.Add(-100*time.Millisecond), line(mover, opus, "work", "sticky", time.Time{}, hourUsage)))
	late.TotalMS = (22 * time.Second).Milliseconds()
	ended := sentAt("r2", sent)
	ended.Ended, ended.Status = sent.Add(22*time.Second), 200
	earlier := withID("r1", line(mover, opus, "work", "new", october(6, 23, 0), hourUsage))
	next := withID("r3", line(mover, opus, "work", "sticky", october(7, 0, 1), hourUsage))
	tests := []struct {
		name   string
		lines  []ledger.Line
		stream views.Stream
		now    time.Time
		want   []string
		counts [2]int
	}{
		{name: "in flight at midnight", lines: []ledger.Line{earlier}, stream: streamOf(sentAt("r2", sent)), now: october(7, 0, 0).Add(5 * time.Second),
			want: []string{"▲ r1 lined", "↑ r2 asking"}, counts: [2]int{2, 0}},
		{name: "its line come, the next day's none yet", lines: []ledger.Line{earlier, late}, stream: streamOf(ended), now: october(7, 0, 0).Add(30 * time.Second),
			want: []string{"▲ r1 lined", "r2 lined"}, counts: [2]int{2, 0}},
		{name: "the next day's first come", lines: []ledger.Line{earlier, late, next}, stream: streamOf(ended), now: october(7, 0, 2),
			want: []string{"▲ r1 lined", "r2 lined", "── 2026-10-06  → 2026-10-07", "r3 lined"}, counts: [2]int{2, 1}},
	}
	for _, tt := range tests {
		got := views.LogRows(views.LogSources{From: october(6, 0, 0), Now: tt.now, Read: logRead(tt.now, tt.lines...), Stream: tt.stream})
		if said := tableSaid(got); !slices.Equal(said, tt.want) {
			t.Errorf("%s: the table says %q, want %q", tt.name, said, tt.want)
		}
		if a, b := got.Count("2026-10-06").Shown, got.Count("2026-10-07").Shown; a != tt.counts[0] || b != tt.counts[1] {
			t.Errorf("%s: 6 October counts %d rows and 7 October %d, want %d and %d", tt.name, a, b, tt.counts[0], tt.counts[1])
		}
	}
}

// The table awaits a line on each day before the one given that it holds a
// row on, shown or hidden, of a request whose end the stream told, its line
// yet to come, each day once: none of one in flight, lost or lined, nor of
// the day given. A request first sent just after midnight awaits it on the
// day before, which it may have arrived on, its line filed under it.
func TestTheTableAwaitsTheLineOfARequestTheStreamToldEndedOnAnEarlierDay(t *testing.T) {
	sent := october(6, 23, 59).Add(50 * time.Second)
	ended := func(id string, at time.Time) views.Told {
		told := sentAt(id, at)
		told.Ended, told.Status = at.Add(22*time.Second), 200
		return told
	}
	check := ended("r1", sent)
	check.Check = true
	lost := sentAt("r1", sent)
	lost.Lost = true
	lined := withID("r1", took(22*time.Second, arrived(sent.Add(-100*time.Millisecond), line(mover, opus, "side", "sticky", time.Time{}, hourUsage))))
	tests := []struct {
		name   string
		lines  []ledger.Line
		stream views.Stream
		filter views.LogFilter
		want   []string
	}{
		{name: "ended the day before, shown", stream: streamOf(ended("r1", sent)), want: []string{"2026-10-06"}},
		{name: "a quota check ended the day before, hidden", stream: streamOf(check), filter: views.LogOpening(), want: []string{"2026-10-06"}},
		{name: "two ended the day before: the day once", stream: streamOf(ended("r1", sent), ended("r2", sent.Add(time.Second))), want: []string{"2026-10-06"}},
		{name: "ended today", stream: streamOf(ended("r1", october(7, 13, 0)))},
		{name: "sent as today began, arrived the day before", stream: streamOf(ended("r1", october(7, 0, 0).Add(time.Second))), want: []string{"2026-10-06"}},
		{name: "in flight the day before", stream: streamOf(sentAt("r1", sent))},
		{name: "lost the day before", stream: streamOf(lost)},
		{name: "ended the day before, its line come", lines: []ledger.Line{lined}, stream: streamOf(ended("r1", sent))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := views.LogRows(views.LogSources{From: october(6, 0, 0), Now: now, Read: logRead(now, tt.lines...), Stream: tt.stream, Filter: tt.filter})
			if got := table.Awaited(views.Today(now)); !slices.Equal(got, tt.want) {
				t.Errorf("the table awaits lines on %q, want %q", got, tt.want)
			}
		})
	}
}

func TestARowIsOnTheDayItsRequestArrivedOnAsTheLedgerFilesIt(t *testing.T) {
	// r2 arrived as 6 October ended, and went out as 7 October began.
	midnight := october(7, 0, 0)
	straddling := withID("r2", took(1050*time.Millisecond, arrived(midnight.Add(-50*time.Millisecond), line(mover, opus, "work", "sticky", time.Time{}, hourUsage))))
	ended := sentAt("r2", midnight.Add(50*time.Millisecond))
	ended.Ended, ended.Status = midnight.Add(time.Second), 200
	lines := []ledger.Line{withID("r1", line(mover, opus, "work", "new", october(6, 23, 0), hourUsage)), withID("r3", line(mover, opus, "work", "sticky", midnight, hourUsage))}
	src := views.LogSources{From: october(6, 0, 0), Now: october(7, 0, 5), Stream: streamOf(ended), Read: logRead(october(7, 0, 5), lines...)}
	before := views.LogRows(src)
	src.Read = logRead(src.Now, append(lines, straddling)...)
	after := views.LogRows(src)
	if said, want := tableSaid(before), []string{"▲ r1 lined", "── 2026-10-06  → 2026-10-07", "r3 lined", "r2 ended"}; !slices.Equal(said, want) {
		t.Errorf("before its line came, the table says %q, want %q: on the day it was sent, the one day the stream tells", said, want)
	}
	if said, want := tableSaid(after), []string{"▲ r1 lined", "r2 lined", "── 2026-10-06  → 2026-10-07", "r3 lined"}; !slices.Equal(said, want) {
		t.Errorf("its line come, the table says %q, want %q: on the day it arrived on, as the ledger files it", said, want)
	}
	if a, b := after.Count("2026-10-06").Shown, after.Count("2026-10-07").Shown; a != 2 || b != 1 {
		t.Errorf("its line come, 6 October counts %d rows and 7 October %d, want 2 and 1, as the days' figures count them", a, b)
	}
}

func TestARowsStoryIsItsSessionsWhateverDayItsLinesAreOf(t *testing.T) {
	main := func(id, account, reason string, at time.Time) ledger.Line {
		return withID(id, of(ledger.ClassMain, line(mover, opus, account, reason, at, hourUsage)))
	}
	moved := func(id string, at time.Time) ledger.Line {
		return movedFrom("work", main(id, "side", "moved: work is at its reserve", at))
	}
	tests := []struct {
		name        string
		from        time.Time
		held, shown []ledger.Line
		want        []string
	}{
		{name: "it began the day before, and moved this morning, its cache cold since last night", from: now,
			held:  []ledger.Line{main("r0", "work", "new", october(6, 22, 0)), moved("r1", october(7, 9, 0))},
			shown: []ledger.Line{moved("r1", october(7, 9, 0))}, want: []string{"▸ r1 from [work], at its cap"}},
		{name: "it moved as the day began, its cache warm from the night", from: now,
			held:  []ledger.Line{main("r0", "work", "new", october(6, 23, 50)), moved("r1", october(7, 0, 10))},
			shown: []ledger.Line{moved("r1", october(7, 0, 10))}, want: []string{"▸ r1 from [work], at its cap  " + cost(hourWrite).Cents()}},
		{name: "it began on the table's first day, with no request today", from: october(6, 0, 0),
			held:  []ledger.Line{main("r0", "work", "new", october(6, 22, 0)), main("r1", "work", "sticky", october(6, 23, 0))},
			shown: []ledger.Line{main("r0", "work", "new", october(6, 22, 0)), main("r1", "work", "sticky", october(6, 23, 0))},
			want:  []string{"▲ r0 first new session", "r1"}},
	}
	for _, tt := range tests {
		got := views.LogRows(views.LogSources{From: tt.from, Now: now, Read: views.ReadLogHeld(ledgerHolding(now, tt.held...), heldOf(tt.shown...), ledger.Pricing, now)})
		var said []string
		for _, l := range got.Lines {
			r := l.Row
			first := map[bool]string{true: " first"}[r.NewSession]
			said = append(said, strings.TrimSpace(r.Mark.String()+" "+r.Request+first+" "+routingWords(r.Routing)))
		}
		if !slices.Equal(said, tt.want) {
			t.Errorf("%s: the table says %q, want %q", tt.name, said, tt.want)
		}
	}
}

func TestTheTablesRowsAreInTheOrderTheyArrivedWhateverOrderTheirLinesCame(t *testing.T) {
	first := withID("r1", of(ledger.ClassMain, line(mover, opus, "work", "new", local(10, 0, 0), hourUsage)))
	moved := movedFrom("work", withID("r2", of(ledger.ClassMain, line(mover, opus, "side", "moved: work is at its reserve", local(10, 5, 0), hourUsage))))
	// Its line came first, as it ended first.
	got := views.LogRows(views.LogSources{From: now, Now: now, Read: logRead(now, moved, first)})
	if said, want := tableSaid(got), []string{"▲ r1 lined", "▸ r2 lined"}; !slices.Equal(said, want) || got.Lines[1].Row.Routing.Cost == nil {
		t.Errorf("the table says %q, its move's cost %v, want %q, the move's cost known, its story told in the order its requests arrived",
			said, got.Lines[1].Row.Routing.Cost, want)
	}
}
