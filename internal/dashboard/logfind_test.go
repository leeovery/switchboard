package dashboard

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/views"
)

// The ids of the sessions the find's tests find.
const (
	apiID   = "7f3a0000-0000-4000-8000-000000000001"
	docsID  = "7f3b0000-0000-4000-8000-000000000002"
	rapidID = "c61b0000-0000-4000-8000-000000000003"
)

// foundThree is what a find of "api" finds at noon on the Log's day: api,
// running today; api-docs, ended on Tuesday 29 September; and rapid, of the
// same day.
func foundThree() views.Found {
	today, tuesday := views.Today(logDayNoon), views.Today(logDayNoon.AddDate(0, 0, -2))
	return views.Found{Sessions: []views.FoundSession{
		{ID: apiID, Dir: "~/Code/api", Day: today},
		{ID: docsID, Dir: "~/Code/api-docs", Day: tuesday, Ended: true},
		{ID: rapidID, Dir: "~/Code/rapid", Day: tuesday, Ended: true},
	}}
}

// findingThree is the Log's page with its find open, "api" typed, finding
// foundThree, the one given picked.
func findingThree(picked string) LogPage {
	p := threeAsked()
	p.Finding = LogFinding{Open: true, Typed: "api", Found: foundThree(), Picked: picked}
	return p
}

// The find's line says what's typed, the session picked as the table names
// it and the day it's found on, and at its right how many it finds and the
// keys that pick one; narrowing, the keys go first, then the day, then the
// count, then the session picked, what's typed cut last.
func TestTheFindsLineSaysWhatsTypedThePickAndHowManyItFinds(t *testing.T) {
	none := threeAsked()
	none.Finding = LogFinding{Open: true, Typed: "zzz"}
	tests := []struct {
		name        string
		page        LogPage
		width       int
		left, right string
	}{
		{name: "today's picked, the first found", page: findingThree(""), width: 128, left: "/ api▏   7f3a api  today", right: "3 sessions · ↑↓ enter"},
		{name: "another day's picked", page: findingThree(docsID), width: 128, left: "/ api▏   7f3b api-docs  Tue 29 Sep", right: "3 sessions · ↑↓ enter"},
		{name: "none found", page: none, width: 128, left: "/ zzz▏", right: "no sessions"},
		{name: "whole, just", page: findingThree(""), width: 48, left: "/ api▏   7f3a api  today", right: "3 sessions · ↑↓ enter"},
		{name: "the keys gone", page: findingThree(""), width: 47, left: "/ api▏   7f3a api  today", right: "3 sessions"},
		{name: "the day gone", page: findingThree(""), width: 36, left: "/ api▏   7f3a api", right: "3 sessions"},
		{name: "the count gone", page: findingThree(""), width: 29, left: "/ api▏   7f3a api"},
		{name: "what's typed alone", page: findingThree(""), width: 19, left: "/ api▏"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, ok := tt.page.findLine(tt.width)
			if !ok {
				t.Fatal("the find is open, but its line isn't drawn")
			}
			want := tt.left + strings.Repeat(" ", tt.width-utf8.RuneCountInString(tt.left+tt.right)) + tt.right
			if got := l.plain(); strings.TrimRight(got, " ") != strings.TrimRight(want, " ") {
				t.Errorf("the find's line reads\n%q\nwant\n%q", got, want)
			}
			if w := l.width(); w > tt.width {
				t.Errorf("the find's line is %d cells, past the %d it has", w, tt.width)
			}
		})
	}
}

// What the find's line says is inked as the page inks it: / and the keys
// that pick bold accent.key, what's typed text.primary, the session picked
// as the table inks a session.
func TestTheFindsLineIsInkedAsThePageInksIt(t *testing.T) {
	l, _ := findingThree("").findLine(128)
	for _, want := range []span{{"/", keyInk}, {" api", typedInk}, {"▏", cursorInk}, {"7f3a", secondaryInk}, {" api", mutedInk}, {"↑↓", keyInk}, {"enter", keyInk}} {
		if !slices.Contains(l, want) {
			t.Errorf("the find's line draws no %q in %v: %v", want.text, want.ink, l.inked())
		}
	}
}

// While the find is open, its line takes the state line's place, on a
// terminal too short for the view as on any, where the row's under the
// status line; closed, the state line is drawn, where the view is.
func TestTheFindsLineTakesTheStateLinesPlace(t *testing.T) {
	for _, tt := range []struct {
		name   string
		page   LogPage
		height int
		want   string
	}{
		{name: "open", page: findingThree(""), height: 24, want: "/ api▏   7f3a api  today"},
		{name: "closed", page: threeAsked(), height: 24, want: "● following"},
		{name: "open, cramped", page: findingThree(""), height: 10, want: "/ api▏   7f3a api  today"},
		{name: "closed, cramped", page: threeAsked(), height: 10},
	} {
		t.Run(tt.name, func(t *testing.T) {
			text := textOf(logFrameOf(tt.page, 160, tt.height).Draw(routerDoc("side", 0), logDayNoon))
			got := text[tt.height-2]
			if tt.want == "" && strings.TrimSpace(got) != "" || !strings.HasPrefix(got, tt.want) {
				t.Errorf("the state line's row reads %q, want it to start %q", got, tt.want)
			}
		})
	}
	text := textOf(logFrameOf(findingThree(""), 160, 4).Draw(routerDoc("side", 0), logDayNoon))
	if slices.ContainsFunc(text, func(row string) bool { return strings.Contains(row, "/ api▏") }) {
		t.Errorf("no row under the status line, the find's line is drawn over the page:\n%s", strings.Join(text, "\n"))
	}
}

// The find's line has a row, the state line's, only under the status line:
// none on a terminal 4 rows tall, or 6 on a phone whose tabs take two rows,
// its title and tabs taking two rows more.
func TestTheFindsLineHasARowOnlyUnderTheStatusLine(t *testing.T) {
	for _, tt := range []struct {
		name          string
		width, height int
		finds         bool
	}{
		{name: "wide, 4 rows", width: 160, height: 4},
		{name: "wide, 5 rows", width: 160, height: 5, finds: true},
		{name: "a phone, 6 rows", width: 52, height: 6},
		{name: "a phone, 7 rows", width: 52, height: 7, finds: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := logFrameOf(findingThree(""), tt.width, tt.height).LogFinds(); got != tt.finds {
				t.Errorf("the find's line has a row: %t, want %t", got, tt.finds)
			}
		})
	}
}

// ↑ and ↓ step the pick through those found, no further than the first or
// the last; one picked that's no longer found picks the first.
func TestTheFindsPickStepsThroughThoseFound(t *testing.T) {
	tests := []struct {
		name   string
		picked string
		by     []int
		want   string
	}{
		{name: "none picked is the first", want: apiID},
		{name: "down", by: []int{1}, want: docsID},
		{name: "down past the last", by: []int{1, 1, 1, 1}, want: rapidID},
		{name: "up past the first", picked: docsID, by: []int{-1, -1}, want: apiID},
		{name: "one no longer found", picked: "99990000-0000-4000-8000-000000000009", want: apiID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := findingThree(tt.picked).Finding
			for _, by := range tt.by {
				f = f.Stepped(by)
			}
			if got, ok := f.Pick(); !ok || got.ID != tt.want {
				t.Errorf("the find picks %q, want %q", got.ID, tt.want)
			}
		})
	}
	if _, ok := (LogFinding{Open: true}).Stepped(1).Pick(); ok {
		t.Error("finding none, the find picks one")
	}
}

// enter on a session found filters both tables to its lines alone, as the
// heading names it, each following again, and closes the find: the kinds
// each shows kept.
func TestTheFindsPickFiltersBothTablesToItsSessionAlone(t *testing.T) {
	p := findingThree("").Chose(-1, 10).Turned(1).Chose(-1, 10)
	p.Shows.Kinds = []string{"cap"}
	got := p.FoundOnly(apiID)
	if !slices.Equal(got.Filter.Sessions, []string{apiID}) || !slices.Equal(got.Shows.Sessions, []string{apiID}) {
		t.Errorf("the tables keep the sessions %v and %v, want %s alone in both", got.Filter.Sessions, got.Shows.Sessions, apiID)
	}
	if !slices.Equal(got.Filter.Kinds, p.Filter.Kinds) || !slices.Equal(got.Shows.Kinds, p.Shows.Kinds) {
		t.Errorf("the kinds shown are %v and %v, want %v and %v kept", got.Filter.Kinds, got.Shows.Kinds, p.Filter.Kinds, p.Shows.Kinds)
	}
	for v := range got.Stands {
		if got.Stands[v].paused {
			t.Errorf("table %d is paused, want both following again", v)
		}
	}
	if got.Finding.Open {
		t.Error("the find is open still, want it closed")
	}
	if said := got.filterNames(func(id string) string { return id }).said().plain(); said != "only 7f3a" {
		t.Errorf("the heading says %q of the filter, want only 7f3a", said)
	}
}

// A filter is on where it keeps some lines alone, as the heading's "only"
// names them, whatever kinds each table shows; letting it go keeps those.
func TestAFilterIsOnWhereItKeepsSomeLinesAlone(t *testing.T) {
	opening := views.LogOpening()
	tests := []struct {
		name   string
		filter views.LogFilter
		shows  views.EventFilter
		on     bool
	}{
		{name: "as the Log opens", filter: opening},
		{name: "checks shown too", filter: views.LogFilter{Kinds: []views.RowKind{views.RowRequest, views.RowCheck}}},
		{name: "some events' kinds", filter: opening, shows: views.EventFilter{Kinds: []string{"cap"}}},
		{name: "a session's requests", filter: views.LogFilter{Kinds: opening.Kinds, Sessions: []string{apiID}}, on: true},
		{name: "an account's requests", filter: views.LogFilter{Accounts: []string{"side"}}, on: true},
		{name: "a model's requests", filter: views.LogFilter{Models: []string{"claude-opus-5-5"}}, on: true},
		{name: "those that failed", filter: views.LogFilter{Failed: true}, on: true},
		{name: "a session's events", filter: opening, shows: views.EventFilter{Sessions: []string{apiID}}, on: true},
		{name: "an account's events", filter: opening, shows: views.EventFilter{Kinds: []string{"cap"}, Accounts: []string{"side"}}, on: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := LogPage{Filter: tt.filter, Shows: tt.shows}
			if got := p.Filtered(); got != tt.on {
				t.Errorf("Filtered is %t, want %t", got, tt.on)
			}
			let := p.Unfiltered()
			if let.Filtered() {
				t.Errorf("let go, the filter keeps %+v and %+v, want none alone", let.Filter, let.Shows)
			}
			if !slices.Equal(let.Filter.Kinds, tt.filter.Kinds) || !slices.Equal(let.Shows.Kinds, tt.shows.Kinds) {
				t.Errorf("let go, the kinds shown are %v and %v, want %v and %v kept", let.Filter.Kinds, let.Shows.Kinds, tt.filter.Kinds, tt.shows.Kinds)
			}
		})
	}
}
