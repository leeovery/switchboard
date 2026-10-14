package views_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

func TestADaysAccountsAreThoseThatServedARequestOrWhoseDayWasEventful(t *testing.T) {
	at := func(hour int) time.Time { return time.Date(2026, 10, 6, hour, 0, 0, 0, time.UTC) }
	worth := ledger.Picodollars(80_000_000_000)
	served := ledger.PricedAccount{
		Account: "work", Sessions: 2, MovedOff: 1, Highest: map[string]float64{"7d": 0.41, "5h": 1},
		Limits: []ledger.Limit{{Window: "5h", At: at(14)}},
		Resets: []ledger.Reset{{Window: "5h", At: at(9)}, {Window: "7d", At: at(10)}, {Window: "7d_oi", At: at(11)}},
		Models: []ledger.PricedModel{
			{Model: "claude-opus-5-5", InferenceGeo: "us", Upstream: 2, Unsent: 1, Sessions: 2, Worth: &worth,
				Usage: json.RawMessage(`{"input_tokens":12,"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,"output_tokens":845}`)},
			{Model: "claude-haiku-4-5", Checks: 3, Sessions: 1},
			{Model: "claude-opus-9", Upstream: 1, Sessions: 1, Unpriced: []string{"claude-opus-9"}},
		},
	}
	unanswered := ledger.PricedAccount{Sessions: 1, Models: []ledger.PricedModel{{Unsent: 1, Sessions: 1}}}
	limited := ledger.PricedAccount{Account: "side", Limits: []ledger.Limit{{Window: "5h", At: at(15)}}, Highest: map[string]float64{"5h": 1}}
	weekReset := ledger.PricedAccount{Account: "spare", Resets: []ledger.Reset{{Window: "7d", At: at(10)}}}
	idle := ledger.PricedAccount{Account: "idle", Highest: map[string]float64{"5h": 0.2}, Resets: []ledger.Reset{{Window: "5h", At: at(9)}}}
	days := []views.Day{
		{Day: "2026-10-05", Accounts: []ledger.PricedAccount{idle}},
		{Day: "2026-10-06", Accounts: []ledger.PricedAccount{unanswered, idle, limited, weekReset, served}},
		{Day: "2026-10-07"},
		{Day: "2026-10-08", Accounts: []ledger.PricedAccount{unanswered}},
	}

	want := []views.DayAccounts{
		{Date: "2026-10-06", Accounts: []views.DayAccount{
			{Sessions: 1, Models: []views.DayModel{{Requests: 1, Sessions: 1}}},
			{Account: "side", Limits: []views.WindowAt{{Window: "5h", At: at(15)}}, Highest: []views.WindowUse{{Window: "5h", Use: 1}}},
			{Account: "spare", WeeksReset: []views.WindowAt{{Window: "7d", At: at(10)}}},
			{Account: "work", Sessions: 2, MovedOff: 1,
				Models: []views.DayModel{
					{Model: "claude-opus-5-5", Geo: "us", Requests: 3, Tokens: 12 + 3120 + 182340 + 845, Sessions: 2, Amount: &worth},
					{Model: "claude-opus-9", Requests: 1, Sessions: 1, Unpriced: []string{"claude-opus-9"}},
				},
				Limits:     []views.WindowAt{{Window: "5h", At: at(14)}},
				WeeksReset: []views.WindowAt{{Window: "7d", At: at(10)}, {Window: "7d_oi", At: at(11)}},
				Highest:    []views.WindowUse{{Window: "5h", Use: 1}, {Window: "7d", Use: 0.41}},
			},
		}},
		{Date: "2026-10-08", Accounts: []views.DayAccount{{Sessions: 1, Models: []views.DayModel{{Requests: 1, Sessions: 1}}}}},
	}
	if got := views.DayAccountsOf(days); !reflect.DeepEqual(got, want) {
		t.Errorf("DayAccountsOf =\n%+v\nwant\n%+v", got, want)
	}
}

func TestAnAccountsDayIsNotedWhereItHasAnythingToNote(t *testing.T) {
	tests := []struct {
		name    string
		account views.DayAccount
		want    bool
	}{
		{name: "sessions", account: views.DayAccount{Account: "work", Sessions: 1}, want: true},
		{name: "a session moved on", account: views.DayAccount{Account: "work", MovedOn: 1}, want: true},
		{name: "a session moved off", account: views.DayAccount{Account: "work", MovedOff: 1}, want: true},
		{name: "a limit", account: views.DayAccount{Account: "work", Limits: []views.WindowAt{{Window: "5h"}}}, want: true},
		{name: "a week reset", account: views.DayAccount{Account: "work", WeeksReset: []views.WindowAt{{Window: "7d"}}}, want: true},
		{name: "a window's use", account: views.DayAccount{Account: "work", Highest: []views.WindowUse{{Window: "5h"}}}, want: true},
		{name: "nothing", account: views.DayAccount{Account: "work", Models: []views.DayModel{{Requests: 1}}}},
		{name: "the requests no account answered", account: views.DayAccount{Sessions: 1, Models: []views.DayModel{{Requests: 1}}}},
	}
	for _, tt := range tests {
		if got := tt.account.Noted(); got != tt.want {
			t.Errorf("%s: Noted = %v, want %v", tt.name, got, tt.want)
		}
	}
}
