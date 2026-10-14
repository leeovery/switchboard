package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// stateNow is when the states are said: Thursday 1 October 2026, 14:42:07,
// an hour east of UTC.
var stateNow = time.Date(2026, 10, 1, 14, 42, 7, 0, time.FixedZone("UTC+1", 60*60))

// at is a time of day on stateNow's day, or on another day of its month.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, stateNow.Location())
}

// readNow is an account with a usable token, its windows read just now: the
// session, the week and Fable's week.
func readNow(session, week, fable float64) status.Account {
	return status.Account{
		ID: "work", Label: "work", TokenSet: true, FetchedAt: stateNow.Add(-4 * time.Second).UTC(),
		Windows: []quota.Window{
			{Key: "5h", Label: "Session", Utilization: session, ResetsAt: at(1, 17, 10).UTC()},
			{Key: "7d", Label: "Week", Utilization: week, ResetsAt: at(5, 21, 0).UTC()},
			{Key: "7d_oi", Label: "Fable week", Utilization: fable, ResetsAt: at(5, 21, 0).UTC()},
		},
	}
}

// with is the account a as change leaves it.
func with(a status.Account, change func(*status.Account)) status.Account {
	change(&a)
	return a
}

// pressing has the account's session run out at 16:05 at its last half
// hour's rate, before it resets at 17:10: under pressure.
func pressing(a *status.Account) {
	rate := (1 - a.Windows[0].Utilization) / at(1, 16, 5).Sub(stateNow).Hours()
	since := stateNow.Add(-score.Recent).UTC()
	a.Rates = []status.Rate{{Window: "5h", Rate: rate, Since: since}}
	a.Pressure = status.Pressure{Window: "5h", Rate: rate, Recent: true, Since: since, RunsOut: at(1, 16, 5).UTC(), Under: true}
}

func TestWhatHoldsAnAccountBackAtItsLimit(t *testing.T) {
	tests := []struct {
		name    string
		account status.Account
		want    status.Hold
		wantOK  bool
	}{
		{
			name: "the limit the router saw, its shared window first",
			account: with(readNow(0.98, 0.3, 0.97), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"7d_oi", "5h"}, Until: at(1, 15, 54).UTC()}
			}),
			want:   status.Hold{Windows: []string{"5h", "7d_oi"}, Until: at(1, 15, 54).UTC(), Every: true},
			wantOK: true,
		},
		{
			name: "a model's own limit, and its session read spent: back as its session resets",
			account: with(readNow(1, 0.3, 0.5), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: at(5, 21, 0).UTC()}
			}),
			want:   status.Hold{Windows: []string{"5h", "7d_oi"}, Until: at(1, 17, 10).UTC(), Every: true},
			wantOK: true,
		},
		{
			name: "a limit lifting before a window read spent resets: back as that resets",
			account: with(readNow(1, 0.3, 0), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"5h"}, Until: at(1, 15, 54).UTC()}
			}),
			want:   status.Hold{Windows: []string{"5h"}, Until: at(1, 17, 10).UTC(), Every: true},
			wantOK: true,
		},
		{
			name: "a limit naming no window, and the week read spent",
			account: with(readNow(0.5, 1.02, 0), func(a *status.Account) {
				a.Limit = status.Limit{Until: at(1, 15, 54).UTC()}
			}),
			want:   status.Hold{Until: at(5, 21, 0).UTC(), Every: true},
			wantOK: true,
		},
		{
			name:    "windows read spent, without the router: back as the last resets",
			account: readNow(1, 1.02, 0),
			want:    status.Hold{Windows: []string{"5h", "7d"}, Until: at(5, 21, 0).UTC(), Every: true},
			wantOK:  true,
		},
		{
			name:    "a window read spent whose reset isn't known",
			account: with(readNow(1.02, 0.3, 0), func(a *status.Account) { a.Windows[0].ResetsAt = time.Time{} }),
			want:    status.Hold{Windows: []string{"5h"}, Every: true},
			wantOK:  true,
		},
		{
			name:    "a model's own window alone",
			account: with(readNow(0.2, 0.3, 1), func(a *status.Account) { a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: at(5, 21, 0).UTC()} }),
			want:    status.Hold{Windows: []string{"7d_oi"}, Until: at(5, 21, 0).UTC()},
			wantOK:  true,
		},
		{
			name:    "a window read spent, without the router",
			account: readNow(0.2, 1.02, 0),
			want:    status.Hold{Windows: []string{"7d"}, Until: at(5, 21, 0).UTC(), Every: true},
			wantOK:  true,
		},
		{
			name: "a limit since lifted",
			account: with(readNow(0.2, 0.3, 0), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"5h"}, Until: stateNow.Add(-time.Minute)}
			}),
		},
		{name: "room in every window", account: readNow(0.2, 0.3, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.account.Held(stateNow, policy)
			if ok != tt.wantOK || !got.Until.Equal(tt.want.Until) || got.Every != tt.want.Every || !slices.Equal(got.Windows, tt.want.Windows) {
				t.Errorf("Held() = %+v, %v, want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestAHoldLiftsOffEachWindowAsItsOwnHoldDoes(t *testing.T) {
	fable := func(a *status.Account) { a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: at(5, 21, 0).UTC()} }
	unnamed := func(a *status.Account) { a.Limit = status.Limit{Until: at(1, 15, 54).UTC()} }
	tests := []struct {
		name    string
		account status.Account
		want    map[string]time.Time
	}{
		{
			name:    "a model's own limit, its session read spent",
			account: with(readNow(1, 0.3, 0.5), fable),
			want:    map[string]time.Time{"5h": at(1, 17, 10).UTC(), "7d": {}, "7d_oi": at(5, 21, 0).UTC()},
		},
		{
			name:    "a limit naming no window, the week read spent",
			account: with(readNow(0.5, 1.02, 0), unnamed),
			want:    map[string]time.Time{"5h": at(1, 15, 54).UTC(), "7d": at(5, 21, 0).UTC()},
		},
	}
	for _, tt := range tests {
		held, ok := tt.account.Held(stateNow, policy)
		if !ok {
			t.Fatalf("%s: Held() reports nothing", tt.name)
		}
		for key, want := range tt.want {
			if got := held.Lifts(key); !got.Equal(want) {
				t.Errorf("%s: Lifts(%q) = %v, want %v", tt.name, key, got, want)
			}
		}
	}
}

func TestAHoldHoldsTheWindowsItNamesOrEveryWindowNamingNone(t *testing.T) {
	tests := []struct {
		name string
		hold status.Hold
		want map[string]bool
	}{
		{name: "naming some", hold: status.Hold{Windows: []string{"5h", "7d_oi"}, Every: true}, want: map[string]bool{"5h": true, "7d": false, "7d_oi": true}},
		{name: "naming none, as a limit only the overall verdict said", hold: status.Hold{Every: true}, want: map[string]bool{"5h": true, "7d": true, "7d_oi": true}},
	}
	for _, tt := range tests {
		for key, want := range tt.want {
			if got := tt.hold.Holds(key); got != want {
				t.Errorf("%s: Holds(%q) = %v, want %v", tt.name, key, got, want)
			}
		}
	}
}
