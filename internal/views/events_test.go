package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// zone is where the events' tests read their times: an hour east of UTC.
var zone = time.FixedZone("UTC+1", 60*60)

// at is the time in zone on 7 October 2026 at hour:minute.
func at(hour, minute int) time.Time {
	return time.Date(2026, 10, 7, hour, minute, 0, 0, zone)
}

// windowNames name the tests' windows, as the CLI names Claude's.
func windowNames(key string) string {
	return map[string]string{"5h": "5-hour", "7d": "week", "7d_oi": "Fable week"}[key]
}

func TestEachKindOfEventIsToldInTheLogsWords(t *testing.T) {
	now := at(16, 0)
	tests := []struct {
		name                string
		event               status.Event
		kind, account, what string
	}{
		{name: "a session started", event: status.Event{Kind: status.EventStarted, Account: "work", Session: "s1", Reason: status.ReasonNew},
			kind: "started", account: "work"},
		{name: "a session moved off an account at its cap", event: status.Event{Kind: status.EventMoved, From: "work", To: "side", Reason: "moved: work is at its reserve"},
			kind: "moved", account: "work → side", what: "work reached its cap"},
		{name: "a session moved off an account at its limit", event: status.Event{Kind: status.EventMoved, From: "personal", To: "side", Reason: "moved: personal hit its limit"},
			kind: "moved", account: "personal → side", what: "personal reached its limit"},
		{name: "a session moved by pin", event: status.Event{Kind: status.EventMoved, From: "work", To: "side", Reason: status.ReasonMovedByPin},
			kind: "moved", account: "work → side", what: "pinned to side"},
		{name: "a session rescored", event: status.Event{Kind: status.EventMoved, From: "side", To: "work", Reason: status.Rescored("15h")},
			kind: "moved", account: "side → work", what: "rescored after 15h idle"},
		{name: "an account under pressure", event: status.Event{Kind: status.EventPressure, Account: "personal", Windows: []string{"5h"}, Until: at(15, 5)},
			kind: "pressure", account: "personal", what: "its 5-hour to run out at 15:05"},
		{name: "an account under pressure, not saying when it runs out", event: status.Event{Kind: status.EventPressure, Account: "personal", Windows: []string{"5h"}},
			kind: "pressure", account: "personal", what: "under pressure"},
		{name: "a cap moving sessions to one account", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.05, Count: 3, To: "side"},
			kind: "cap", account: "work", what: "95% of its 5-hour window: its 3 sessions move to side as their next requests come"},
		{name: "a cap moving a session", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"7d"}, Reserve: 0.2, Count: 1, To: "side"},
			kind: "cap", account: "work", what: "80% of its week window: its session moves to side as its next request comes"},
		{name: "a cap in two windows, moving sessions to several", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"5h", "7d"}, Reserve: 0.1, Count: 2},
			kind: "cap", account: "work", what: "90% of its 5-hour and week windows: its 2 sessions move to other accounts as their next requests come"},
		{name: "a cap moving none", event: status.Event{Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Reserve: 0.1},
			kind: "cap", account: "work", what: "90% of its 5-hour window"},
		{name: "a limit moving a session", event: status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Until: at(23, 58), Count: 1, To: "side"},
			kind: "limit", account: "personal", what: "its 5-hour window, till 23:58: its session moves to side"},
		{name: "a limit in a model's own week, lifting another day", event: status.Event{Kind: status.EventLimit, Account: "personal", Windows: []string{"7d_oi"}, Until: at(23, 58).AddDate(0, 0, 3)},
			kind: "limit", account: "personal", what: "its Fable week window, till Sat 23:58"},
		{name: "a limit naming no window, moving sessions to several", event: status.Event{Kind: status.EventLimit, Account: "personal", Count: 4},
			kind: "limit", account: "personal", what: "its limit: its 4 sessions move to other accounts"},
		{name: "a limit moving one session to several", event: status.Event{Kind: status.EventLimit, Account: "personal", Count: 1},
			kind: "limit", account: "personal", what: "its limit: its session moves to another account"},
		{name: "a token refused", event: status.Event{Kind: status.EventRefused, Account: "work", Status: 401, Until: at(14, 0), Count: 2, To: "side"},
			kind: "refused", account: "work", what: "its token refused (401), till 14:00: its 2 sessions move to side"},
		{name: "a request refused alone", event: status.Event{Kind: status.EventRefused, Account: "work", Status: 403, Family: "opus", Until: at(14, 0)},
			kind: "refused", account: "work", what: "its opus requests refused (403), till 14:00"},
		{name: "a prime", event: status.Event{Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}, Until: at(12, 5)},
			kind: "primed", account: "side", what: "its 5-hour window started, resets at 12:05"},
		{name: "room again, told as open", event: status.Event{Kind: status.EventRoom, Account: "work", Windows: []string{"5h"}},
			kind: "open", account: "work", what: "its 5-hour window reset"},
		{name: "room again, naming no window", event: status.Event{Kind: status.EventRoom, Account: "work"},
			kind: "open", account: "work"},
		{name: "the global pin, from the command line", event: status.Event{Kind: status.EventPin, Account: "work", Accounts: []string{"work", "side"}, By: "cli"},
			kind: "pin", account: "work, side", what: "new sessions go there, set from the command line"},
		{name: "the global pin, moving running sessions", event: status.Event{Kind: status.EventPin, Account: "side", Accounts: []string{"side"}, Move: true, By: "dashboard"},
			kind: "pin", account: "side", what: "new and running sessions go there, set from the dashboard"},
		{name: "the global pin, not saying who set it", event: status.Event{Kind: status.EventPin, Account: "side", Accounts: []string{"side"}},
			kind: "pin", account: "side", what: "new sessions go there"},
		{name: "a session's own pin", event: status.Event{Kind: status.EventPin, Account: "side", Session: "s1", By: "dashboard"},
			kind: "pin", account: "side", what: "this session goes there, set from the dashboard"},
		{name: "the global pin cleared", event: status.Event{Kind: status.EventAuto, Accounts: []string{"personal"}, By: "dashboard"},
			kind: "auto", what: "new sessions back from personal to the router's choice, set from the dashboard"},
		{name: "the global pin of several cleared", event: status.Event{Kind: status.EventAuto, Accounts: []string{"work", "side", "personal"}, By: "cli"},
			kind: "auto", what: "new sessions back from work, side and personal to the router's choice, set from the command line"},
		{name: "a session's own pin cleared", event: status.Event{Kind: status.EventAuto, Account: "side", Session: "s1", By: "cli"},
			kind: "auto", what: "this session back from side to the router's choice, set from the command line"},
		{name: "a restart for the config", event: status.Event{Kind: status.EventRestart, Reason: status.RestartForConfig},
			kind: "restart", what: "due: the config changed"},
		{name: "a restart for an upgrade", event: status.Event{Kind: status.EventRestart, Reason: status.RestartForUpgrade},
			kind: "restart", what: "due: switchboard upgraded"},
		{name: "a restart for the time zone", event: status.Event{Kind: status.EventRestart, Reason: status.RestartForZone},
			kind: "restart", what: "due: the time zone changed"},
		{name: "a restart for a reason the Log has no words for", event: status.Event{Kind: status.EventRestart, Reason: "the moon\x1b[2Jrose"},
			kind: "restart", what: "due: the moon [2Jrose"},
		{name: "the router unhealthy", event: status.Event{Kind: status.EventHealth, Reason: "most requests failing"},
			kind: "health", what: "unhealthy: most requests failing"},
		{name: "the router healthy again", event: status.Event{Kind: status.EventHealth},
			kind: "health", what: "healthy again"},
		{name: "a kind from a later router", event: status.Event{Kind: "eclipse", Account: "work", Reason: "it went dark"},
			kind: "eclipse", account: "work", what: "it went dark"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.event.At = at(13, 0)

			got := views.NewTelling(windowNames).Line(tt.event, now)
			if got.Kind != tt.kind || got.Account.String() != tt.account || got.What.String() != tt.what {
				t.Errorf("an event %+v is told as %q, %q, %q; want %q, %q, %q",
					tt.event, got.Kind, got.Account.String(), got.What.String(), tt.kind, tt.account, tt.what)
			}
		})
	}
}

func TestAnEventsAccountsAreNamedApart(t *testing.T) {
	e := status.Event{At: at(13, 0), Kind: status.EventLimit, Account: "personal", Windows: []string{"5h"}, Count: 2, To: "side"}

	got := views.NewTelling(windowNames).Line(e, at(16, 0))
	if want := (status.Words{{Text: "personal", Kind: status.PartAccount}}); !slices.Equal(got.Account, want) {
		t.Errorf("a limit's account is %+v, want %+v", got.Account, want)
	}
	if !slices.Contains(got.What, status.Part{Text: "side", Kind: status.PartAccount}) {
		t.Errorf("a limit's words are %+v, want side named apart, as an account", got.What)
	}
	move := status.Event{At: at(13, 0), Kind: status.EventMoved, From: "work", To: "side", Reason: status.ReasonMovedByPin}
	want := status.Words{{Text: "work", Kind: status.PartAccount}, {Text: " → "}, {Text: "side", Kind: status.PartAccount}}
	if got := views.NewTelling(windowNames).Line(move, at(16, 0)); !slices.Equal(got.Account, want) {
		t.Errorf("a move's account is %+v, want %+v", got.Account, want)
	}
}

func TestAMovePassingOverAnAccountUnderPressureSaysWhenItRunsOut(t *testing.T) {
	now := at(16, 0)
	move := status.Event{At: at(14, 0), Kind: status.EventMoved, From: "personal", To: "side", Reason: "new, personal under pressure"}
	telling := views.NewTelling(windowNames)

	if got, want := telling.Line(move, now).What.String(), "personal came under pressure"; got != want {
		t.Errorf("before personal's pressure is told, a move passing it over says %q, want %q", got, want)
	}
	telling.Line(status.Event{At: at(13, 0), Kind: status.EventPressure, Account: "personal", Windows: []string{"5h"}, Until: at(15, 10)}, now)
	telling.Line(status.Event{At: at(13, 5), Kind: status.EventPressure, Account: "work", Windows: []string{"5h"}, Until: at(17, 0)}, now)
	if got, want := telling.Line(move, now).What.String(), "personal came under pressure, its 5-hour to run out at 15:10"; got != want {
		t.Errorf("once personal's pressure is told, a move passing it over says %q, want %q", got, want)
	}
}

func TestAnEventsTimesAreShownInNowsTimeZone(t *testing.T) {
	e := status.Event{At: at(13, 0), Kind: status.EventLimit, Account: "work", Windows: []string{"5h"}, Until: at(18, 0)}

	got := views.NewTelling(windowNames).Line(e, time.Date(2026, 10, 7, 16, 0, 0, 0, time.UTC)).What.String()
	if want := "its 5-hour window, till 17:00"; got != want {
		t.Errorf("a limit lifting at 18:00 an hour east of UTC, told in UTC, says %q, want %q", got, want)
	}
}
