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

// filterWorld are the lines the filter's tests pick among, today's: of two
// sessions and none, two models, two accounts and none, every kind, some
// failed.
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
		{name: "two sessions", filter: views.LogFilter{Sessions: []string{older, mover}}, ids: []string{"a", "b", "c", "d", "f", "g"}},
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
		Sessions: map[string]int{mover: 3, older: 3},
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
// together each of the lines naming one.
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
					{"session", c.Sessions, func(r views.RequestRow) string { return r.Session }, func(id string) views.LogFilter { return views.LogFilter{Sessions: []string{id}} }},
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

// datesOf are the dates of the days the table's rows are on, each once.
func datesOf(table views.LogTable) []string {
	var dates []string
	for _, l := range table.Lines {
		if date := views.Today(l.Row.At).Date; l.Rule == nil && !slices.Contains(dates, date) {
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
		if l.Rule == nil && views.Today(l.Row.At).Date == date && (keep == nil || keep(l.Row)) {
			n++
		}
	}
	return n
}
