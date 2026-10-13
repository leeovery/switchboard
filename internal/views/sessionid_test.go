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

// The ids sessions are named among in the naming's tests: the first three
// start 5b0e, and the third is that whole.
const startsFirst, startsSecond, startsWhole, startsNone = "5b0e7c1a", "5b0e9d33", "5b0e", "18bb2c41"

func TestASessionIsNamedByAsMuchOfItsIdAsIsUnique(t *testing.T) {
	tests := []struct {
		name  string
		given string
		ids   []string
		want  views.SessionNamed
	}{
		{name: "the one its id starts", given: "5b0e7", ids: []string{startsNone, startsFirst, startsSecond}, want: views.SessionNamed{ID: startsFirst}},
		{name: "one whose id it is whole, though it starts others", given: startsWhole, ids: []string{startsFirst, startsWhole, startsSecond}, want: views.SessionNamed{ID: startsWhole}},
		{name: "itself, where it starts none, as a session's whole id", given: "77aa", ids: []string{startsFirst, startsNone}, want: views.SessionNamed{ID: "77aa"}},
		{name: "one coming more than once", given: "5b0e9", ids: []string{startsSecond, startsNone, startsSecond}, want: views.SessionNamed{ID: startsSecond}},
		{name: "none where it starts several, each listed once, as they first came", given: "5b0e", ids: []string{startsSecond, startsNone, startsFirst, startsSecond},
			want: views.SessionNamed{Several: []string{startsSecond, startsFirst}}},
		{name: "each listed cleaned", given: "5b", ids: []string{"5b\x1b[31m", startsFirst}, want: views.SessionNamed{Several: []string{"5b [31m", startsFirst}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := views.NamedAmong(tt.given, tt.ids); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NamedAmong(%q, %q) = %+v, want %+v", tt.given, tt.ids, got, tt.want)
			}
		})
	}
}

func TestASessionTheRouterListsIsNamedAmongThemListedAsStatusListsThem(t *testing.T) {
	now := time.Date(2026, 10, 7, 13, 12, 0, 0, time.UTC)
	sessions := []status.Session{
		{ID: startsFirst, Assignments: []status.Assignment{{Model: "claude-opus-5-5", Account: "work", LastSeen: now.Add(-time.Minute)}}},
		{ID: startsNone, Assignments: []status.Assignment{{Model: "claude-opus-5-5", Account: "side", LastSeen: now}}},
		{ID: startsSecond, Pin: "side", Assignments: []status.Assignment{{Model: "claude-haiku-4-5", Account: "side", LastSeen: now.Add(-time.Hour)}}},
	}
	if got := views.NamedRouted("18", sessions, now); !reflect.DeepEqual(got, views.SessionNamed{ID: startsNone}) {
		t.Errorf("NamedRouted(18) = %+v, want %s alone", got, startsNone)
	}
	want := views.SessionNamed{Several: []string{sessions[0].Line(now), sessions[2].Line(now)}}
	if got := views.NamedRouted("5b0e", sessions, now); !reflect.DeepEqual(got, want) {
		t.Errorf("NamedRouted(5b0e) = %+v, want those it starts listed as status's text lists them, %+v", got, want)
	}
	if got := views.Routed(sessions, startsSecond); got == nil || got.ID != startsSecond {
		t.Errorf("Routed(%s) = %+v, want the router's session of that id", startsSecond, got)
	}
	if got := views.Routed(sessions, startsWhole); got != nil {
		t.Errorf("Routed(%s) = %+v, want none, as the router lists no session of that id whole", startsWhole, got)
	}
}

func TestTheLinesOfASessionNamedAreHeldOneSessionAtMost(t *testing.T) {
	tests := []struct {
		name string
		// sessions are the sessions of the lines read, in order, each line's
		// request its index.
		sessions []string
		// held are the requests whose lines are the session's, once every
		// line is read, and several those it starts, where it starts several.
		held, several []string
	}{
		{name: "the first session's whose id it starts, while it starts no other's", sessions: []string{startsFirst, startsNone, startsFirst}, held: []string{"0", "2"}},
		{name: "none once it starts another's", sessions: []string{startsFirst, startsFirst, startsSecond, startsFirst, startsSecond}, several: []string{startsFirst, startsSecond}},
		{name: "the session it names whole, once one of its lines comes", sessions: []string{startsFirst, startsSecond, startsFirst, startsWhole, startsFirst, startsSecond, startsWhole}, held: []string{"3", "6"}},
		{name: "the session it names whole, from its first line", sessions: []string{startsWhole, startsFirst, startsWhole}, held: []string{"0", "2"}},
		{name: "none of a session it doesn't start", sessions: []string{startsNone}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := make([]ledger.Held, len(tt.sessions))
			for i, session := range tt.sessions {
				lines[i] = ledger.Held{Request: strconv.Itoa(i), Session: session}
			}
			named, lined := views.NamedLines(slices.Values(lines), startsWhole)
			var held []string
			for _, h := range lined {
				held = append(held, h.Request)
			}
			if !slices.Equal(held, tt.held) || !slices.Equal(named.Several, tt.several) {
				t.Errorf("the lines held are of the requests %q, those it starts %q; want %q and %q", held, named.Several, tt.held, tt.several)
			}
		})
	}
}

func TestTheLinesOfAnAccountAreItsAloneAndEveryLineOfNone(t *testing.T) {
	lines := []ledger.Held{
		{Request: "a", Account: "work"},
		{Request: "b", Account: "side"},
		{Request: "c"},
		{Request: "d", Account: "work"},
	}
	for account, want := range map[string][]string{"work": {"a", "d"}, "": {"a", "b", "c", "d"}, "nobody": nil} {
		var got []string
		for h := range views.OnAccount(slices.Values(lines), account) {
			got = append(got, h.Request)
		}
		if !slices.Equal(got, want) {
			t.Errorf("OnAccount(%q) gives the lines of %q, want %q", account, got, want)
		}
	}
}
