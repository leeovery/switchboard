package views_test

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
	"github.com/leeovery/switchboard/internal/views/viewstest"
)

// The lines a filter keeps reach back to the earliest day whose summary may
// hold one: of a session alone, its first day; of an account or a model,
// the first day of its lines; of the kinds ticked alone, the first that
// counts one; a day whose summary names none of its sessions alike where it
// doesn't name every one it counts; none where no day may; and of the
// failed alone, as of all lines, as no summary counts a day's failures.
func TestALogFiltersLinesReachBackToTheFirstDayASummaryMayHoldOne(t *testing.T) {
	day := func(date, account, model string, requests, checks int, sessions ...string) ledger.Summary {
		m := ledger.ModelDay{Model: model, Upstream: requests, Checks: checks, Sessions: len(sessions), SessionIDs: sessions}
		return ledger.Summary{Day: date, Accounts: []ledger.AccountDay{{Account: account, Models: []ledger.ModelDay{m}}}}
	}
	unnamed := day("2026-10-02", "side", opus, 1, 0)
	unnamed.Accounts[0].Models[0].Sessions = 1
	summaries := []ledger.Summary{
		day("2026-10-01", "work", haiku, 0, 2),
		unnamed,
		day("2026-10-03", "work", opus, 4, 0, older),
		day("2026-10-05", "side", haiku, 2, 1, older, mover),
		day("2026-10-07", "work", opus, 3, 0, mover),
	}
	tests := []struct {
		name   string
		filter views.LogFilter
		want   string
	}{
		{name: "a session alone", filter: views.LogFilter{Sessions: []string{mover}, Kinds: []views.RowKind{views.RowRequest}}, want: "2026-10-02"},
		{name: "a session every summary names", filter: views.LogFilter{Sessions: []string{mover}, Accounts: []string{"work"}}, want: "2026-10-07"},
		{name: "an account", filter: views.LogFilter{Accounts: []string{"side"}}, want: "2026-10-02"},
		{name: "a model", filter: views.LogFilter{Models: []string{opus}, Kinds: []views.RowKind{views.RowRequest}}, want: "2026-10-02"},
		{name: "the kinds alone", filter: views.LogOpening(), want: "2026-10-02"},
		{name: "a kind only some days count", filter: views.LogFilter{Kinds: []views.RowKind{views.RowCheck}}, want: "2026-10-01"},
		{name: "a session of none of a kind", filter: views.LogFilter{Sessions: []string{older}, Kinds: []views.RowKind{views.RowCount}}},
		{name: "a session no day names", filter: views.LogFilter{Sessions: []string{"0eb00000-0000-4000-8000-000000000003"}, Accounts: []string{"work"}}},
		{name: "the failed alone", filter: views.LogFilter{Kinds: []views.RowKind{views.RowRequest}, Failed: true}, want: "2026-10-02"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filter.KeptFrom(summaries); got != tt.want {
				t.Errorf("the lines %+v keeps reach back to %q, want %q", tt.filter, got, tt.want)
			}
		})
	}
}

// The days' summaries bound how far back the Requests read through every
// filter but one keeping the failed of no session, by the lines it keeps;
// and the Events through a filter keeping some sessions' events, by those
// sessions' lines of every kind; never the Events through every session's,
// as no summary tells the days of an event's kind or account.
func TestTheDaysSummariesBoundATablesReachWhereTheyTellOfWhatItKeeps(t *testing.T) {
	ticked := func(f views.LogFilter, tick func(*views.LogFilter)) views.LogFilter {
		tick(&f)
		return f
	}
	checks := views.LogFilter{Kinds: []views.RowKind{views.RowCheck}}
	ofWork := ticked(views.LogOpening(), func(f *views.LogFilter) { f.Accounts = []string{"work"} })
	failed := ticked(views.LogOpening(), func(f *views.LogFilter) { f.Failed = true })
	failedOfWork := ticked(ofWork, func(f *views.LogFilter) { f.Failed = true })
	failedOfMover := ticked(failed, func(f *views.LogFilter) { f.Sessions = []string{mover} })
	tests := []struct {
		name   string
		reach  func() (views.LogFilter, bool)
		want   views.LogFilter
		bounds bool
	}{
		{name: "the Requests through the kinds alone", reach: views.LogOpening().Reach, want: views.LogOpening(), bounds: true},
		{name: "the Requests through checks alone", reach: checks.Reach, want: checks, bounds: true},
		{name: "the Requests through an account", reach: ofWork.Reach, want: ofWork, bounds: true},
		{name: "the Requests through the failed of a session", reach: failedOfMover.Reach, want: failedOfMover, bounds: true},
		{name: "the Requests through the failed alone", reach: failed.Reach},
		{name: "the Requests through the failed of an account", reach: failedOfWork.Reach},
		{
			name: "the Events through a session's moves", reach: views.EventFilter{Kinds: []string{"moved"}, Sessions: []string{mover}}.Reach,
			want: views.LogFilter{Sessions: []string{mover}}, bounds: true,
		},
		{name: "the Events through every one", reach: views.EventFilter{}.Reach},
		{name: "the Events through a kind alone", reach: views.EventFilter{Kinds: []string{"limit"}}.Reach},
		{name: "the Events through an account", reach: views.EventFilter{Accounts: []string{"work"}}.Reach},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, bounds := tt.reach()
			if bounds != tt.bounds || bounds && !got.Equal(tt.want) {
				t.Errorf("the reach is of %+v, bounding %t; want of %+v, bounding %t", got, bounds, tt.want, tt.bounds)
			}
		})
	}
}

// Two filters are equal where they tick the same, each choice in the same
// order, and unequal where any one choice differs.
func TestFiltersAreEqualWhereEveryChoiceTicksTheSame(t *testing.T) {
	logs := func() views.LogFilter {
		return views.LogFilter{Accounts: []string{"work"}, Models: []string{opus}, Sessions: []string{mover, older}, Kinds: []views.RowKind{views.RowRequest}, Failed: true}
	}
	events := func() views.EventFilter {
		return views.EventFilter{Kinds: []string{"cap"}, Accounts: []string{"work"}, Sessions: []string{mover, older}}
	}
	if !logs().Equal(logs()) || !events().Equal(events()) {
		t.Error("a filter isn't equal to one ticking the same")
	}
	logChanges := map[string]func(*views.LogFilter){
		"accounts":                  func(f *views.LogFilter) { f.Accounts = []string{"side"} },
		"models":                    func(f *views.LogFilter) { f.Models = []string{haiku} },
		"sessions in another order": func(f *views.LogFilter) { f.Sessions = []string{older, mover} },
		"kinds":                     func(f *views.LogFilter) { f.Kinds = nil },
		"failed":                    func(f *views.LogFilter) { f.Failed = false },
	}
	for name, change := range logChanges {
		f := logs()
		if change(&f); logs().Equal(f) {
			t.Errorf("the Requests' filters ticking other %s, %+v and %+v, are equal", name, logs(), f)
		}
	}
	eventChanges := map[string]func(*views.EventFilter){
		"kinds":                     func(f *views.EventFilter) { f.Kinds = []string{"limit"} },
		"accounts":                  func(f *views.EventFilter) { f.Accounts = nil },
		"sessions in another order": func(f *views.EventFilter) { f.Sessions = []string{older, mover} },
	}
	for name, change := range eventChanges {
		f := events()
		if change(&f); events().Equal(f) {
			t.Errorf("the Events' filters ticking other %s, %+v and %+v, are equal", name, events(), f)
		}
	}
}

// filterWorld are the lines the filter's tests pick among, today's: of two
// sessions and none, two models, two accounts and none, every kind, some
// failed; a quota check naming a session, toward which it counts none.
func filterWorld() []ledger.Line {
	kinded := func(kind string, l ledger.Line) ledger.Line {
		l.Kind = kind
		return l
	}
	failed := func(l ledger.Line) ledger.Line {
		l.Status = 429
		return l
	}
	return []ledger.Line{
		withID("a", line(mover, opus, "work", "new", october(7, 9, 0), hourUsage)),
		failed(withID("b", line(mover, haiku, "side", "new", october(7, 9, 5), ""))),
		withID("c", line(older, opus, "side", "new", october(7, 9, 10), hourUsage)),
		kinded(ledger.KindCheck, withID("d", line(older, haiku, "work", "new", october(7, 9, 15), smallUsage))),
		withID("e", line("", opus, "work", "unsessioned", october(7, 9, 20), hourUsage)),
		failed(withID("f", line(mover, opus, "", "no account has room", october(7, 9, 25), ""))),
		kinded(ledger.KindCount, withID("g", line(older, opus, "work", "sticky", october(7, 9, 30), ""))),
	}
}

func TestALogFilterKeepsThoseTickedAndEveryOneWhereNoneIs(t *testing.T) {
	both := []views.RowKind{views.RowCheck, views.RowCount}
	tests := []struct {
		name     string
		filter   views.LogFilter
		ids      []string
		hiddenBy []views.RowKind
	}{
		{name: "nothing ticked: every line", ids: []string{"a", "b", "c", "d", "e", "f", "g"}},
		{name: "as the Log opens: checks and counts hidden", filter: views.LogOpening(), ids: []string{"a", "b", "c", "e", "f"}, hiddenBy: both},
		{name: "an account", filter: views.LogFilter{Accounts: []string{"work"}}, ids: []string{"a", "d", "e", "g"}},
		{name: "two accounts: none on no account", filter: views.LogFilter{Accounts: []string{"work", "side"}}, ids: []string{"a", "b", "c", "d", "e", "g"}},
		{name: "a model", filter: views.LogFilter{Models: []string{haiku}}, ids: []string{"b", "d"}},
		{name: "a session: none of no session", filter: views.LogFilter{Sessions: []string{mover}}, ids: []string{"a", "b", "f"}},
		{name: "two sessions: none of a quota check", filter: views.LogFilter{Sessions: []string{older, mover}}, ids: []string{"a", "b", "c", "f", "g"}},
		{name: "kinds", filter: views.LogFilter{Kinds: both}, ids: []string{"d", "g"}, hiddenBy: []views.RowKind{views.RowRequest}},
		{name: "failed only", filter: views.LogFilter{Failed: true}, ids: []string{"b", "f"}},
		{name: "failed on an account", filter: views.LogFilter{Accounts: []string{"side"}, Failed: true}, ids: []string{"b"}},
		{name: "a session as the Log opens: hidden by more than kinds", filter: views.LogFilter{Kinds: views.LogOpening().Kinds, Sessions: []string{older}},
			ids: []string{"c"}},
	}
	lines := filterWorld()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := views.LogRows(views.LogSources{From: now, Now: now, Read: logRead(now, lines...), Filter: tt.filter})
			var ids []string
			for _, l := range table.Lines {
				ids = append(ids, l.Row.Request)
			}
			if !slices.Equal(ids, tt.ids) {
				t.Errorf("the table shows %q, want %q", ids, tt.ids)
			}
			c := table.Count(views.Today(now).Date)
			if c.Shown != len(tt.ids) || c.Hidden() != len(lines)-len(tt.ids) || !slices.Equal(c.HiddenBy(), tt.hiddenBy) {
				t.Errorf("today counts %d shown and %d hidden, by %v, want %d, %d, by %v",
					c.Shown, c.Hidden(), c.HiddenBy(), len(tt.ids), len(lines)-len(tt.ids), tt.hiddenBy)
			}
		})
	}
}

func TestTheDaysCountsAreTheSameWhateverTheFilter(t *testing.T) {
	lines := filterWorld()
	want := views.LogCount{
		Requests: 5, Checks: 1, Counts: 1, Failed: 2, Shown: 7,
		Models:   map[string]int{opus: 5, haiku: 2},
		Accounts: map[string]int{"work": 4, "side": 2},
		Sessions: map[string]int{mover: 3, older: 2},
	}
	count := func(f views.LogFilter) views.LogCount {
		return views.LogRows(views.LogSources{From: now, Now: now, Read: logRead(now, lines...), Filter: f}).Count(views.Today(now).Date)
	}
	if got := count(views.LogFilter{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("today counts %+v with nothing ticked, want %+v", got, want)
	}
	for _, f := range []views.LogFilter{views.LogOpening(), {Sessions: []string{older}}, {Failed: true}} {
		c := count(f)
		if c.Requests != want.Requests || c.Checks != want.Checks || c.Counts != want.Counts || c.Failed != want.Failed ||
			!maps.Equal(c.Models, want.Models) || !maps.Equal(c.Accounts, want.Accounts) || !maps.Equal(c.Sessions, want.Sessions) {
			t.Errorf("filtered %+v, today counts %+v, want %+v, as with nothing ticked", f, c, want)
		}
	}
}

// TestTheCountsAgreeWithWhatTheFilterShowsAtEachHardMoment holds each of
// each day's counts to the rows the filter that picks them shows, at each
// hard moment: the heading's hidden and the state line's shown make the
// panel's lines, as each kind's do; failed is what the failed filter shows;
// and each session's, model's and account's lines are what ticking it shows,
// together each of the lines naming one, a line naming the session it
// counts toward, as countedSession has it.
func TestTheCountsAgreeWithWhatTheFilterShowsAtEachHardMoment(t *testing.T) {
	for _, hm := range viewstest.LogMoments() {
		t.Run(hm.Name, func(t *testing.T) {
			src := logSourcesOf(hm)
			shows := func(f views.LogFilter) views.LogTable {
				src.Filter = f
				return views.LogRows(src)
			}
			every, filtered := shows(views.LogFilter{}), shows(hm.Filter)
			for _, date := range datesOf(every) {
				c := filtered.Count(date)
				if lines, shown := rowsOn(every, date, nil), rowsOn(filtered, date, nil); c.Lines() != lines || c.Shown != shown || c.Hidden() != lines-shown {
					t.Errorf("%s counts %d lines, %d shown and %d hidden, want the %d it holds, the %d it shows, and the rest", date, c.Lines(), c.Shown, c.Hidden(), lines, shown)
				}
				if failed := rowsOn(shows(views.LogFilter{Failed: true}), date, nil); c.Failed != failed {
					t.Errorf("%s counts %d failed, want the %d the failed filter shows", date, c.Failed, failed)
				}
				for _, by := range []struct {
					name    string
					counted map[string]int
					of      func(views.RequestRow) string
					ticking func(string) views.LogFilter
				}{
					{"session", c.Sessions, countedSession, func(id string) views.LogFilter { return views.LogFilter{Sessions: []string{id}} }},
					{"model", c.Models, func(r views.RequestRow) string { return r.Model }, func(id string) views.LogFilter { return views.LogFilter{Models: []string{id}} }},
					{"account", c.Accounts, func(r views.RequestRow) string { return r.Account }, func(id string) views.LogFilter { return views.LogFilter{Accounts: []string{id}} }},
				} {
					t.Run(date+" by "+by.name, func(t *testing.T) {
						sum := 0
						for id, n := range by.counted {
							if shown := rowsOn(shows(by.ticking(id)), date, nil); n != shown {
								t.Errorf("%d lines of %s counted, want the %d ticking it shows", n, id, shown)
							}
							sum += n
						}
						if named := rowsOn(every, date, func(r views.RequestRow) bool { return by.of(r) != "" }); sum != named {
							t.Errorf("the lines counted come to %d, want the %d naming one", sum, named)
						}
					})
				}
			}
		})
	}
}

// countedSession is the session the request of the row r counts toward, as
// ledger.Line.CountedSession has it: none for a quota check, which Claude
// Code sends under an id it never uses again.
func countedSession(r views.RequestRow) string {
	if r.Kind == views.RowCheck {
		return ""
	}
	return r.Session
}

// datesOf are the dates of the days the table's rows are on, each once.
func datesOf(table views.LogTable) []string {
	var dates []string
	for _, l := range table.Lines {
		if date := views.Today(l.Row.At).Date; l.IsRow() && !slices.Contains(dates, date) {
			dates = append(dates, date)
		}
	}
	return dates
}

// rowsOn counts the table's rows on the day with the given date that keep
// says, every one where it's nil.
func rowsOn(table views.LogTable, date string, keep func(views.RequestRow) bool) int {
	n := 0
	for _, l := range table.Lines {
		if l.IsRow() && views.Today(l.Row.At).Date == date && (keep == nil || keep(l.Row)) {
			n++
		}
	}
	return n
}

// The panel lists the models today's lines asked for the most asked first,
// those asked alike by their ids, then those the filter ticks that none
// asked for, as one ticked before midnight, by their ids, each once; and the
// kinds of line in its own order.
func TestThePanelsModelsAreTheMostAskedFirstAndItsKindsInItsOrder(t *testing.T) {
	c := views.LogCount{Models: map[string]int{"claude-haiku-4-5": 40, "claude-opus-5-5": 1200, "claude-fable-5-1": 40}}
	tests := []struct {
		name   string
		ticked []string
		want   []string
	}{
		{name: "none ticked", want: []string{"claude-opus-5-5", "claude-fable-5-1", "claude-haiku-4-5"}},
		{name: "today's ticked", ticked: []string{"claude-haiku-4-5"}, want: []string{"claude-opus-5-5", "claude-fable-5-1", "claude-haiku-4-5"}},
		{
			name: "others ticked", ticked: []string{"claude-sonnet-4-5", "claude-opus-5-5", "claude-opus-4-1", "claude-sonnet-4-5"},
			want: []string{"claude-opus-5-5", "claude-fable-5-1", "claude-haiku-4-5", "claude-opus-4-1", "claude-sonnet-4-5"},
		},
	}
	for _, tt := range tests {
		if got := c.ModelsListed(tt.ticked); !slices.Equal(got, tt.want) {
			t.Errorf("%s, the models are listed %q, want %q", tt.name, got, tt.want)
		}
	}
	if got, want := views.RowKinds(), []views.RowKind{views.RowRequest, views.RowCheck, views.RowCount}; !slices.Equal(got, want) {
		t.Errorf("the kinds are listed %v, want %v", got, want)
	}
}
