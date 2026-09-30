package status_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prime"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

func TestPriming(t *testing.T) {
	tests := []struct {
		name string
		day  config.Day
		want status.Prime
	}{
		{
			name: "a day",
			day:  config.Day{Start: 8 * time.Hour, End: 23 * time.Hour},
			want: status.Prime{Day: "08:00-23:00", Window: "5h", Slots: []status.Slot{{Account: "work", At: "03:50"}, {Account: "personal", At: "05:30"}, {Account: "side", At: "07:10"}}},
		},
		{
			name: "a day past midnight, whose slots fall across it",
			day:  config.Day{Start: 23*time.Hour + 30*time.Minute, End: 7 * time.Hour},
			want: status.Prime{Day: "23:30-07:00", Window: "5h", Slots: []status.Slot{{Account: "work", At: "19:20"}, {Account: "personal", At: "21:00"}, {Account: "side", At: "22:40"}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule, ok := prime.New(tt.day, []string{"work", "personal", "side"}, policy)
			if !ok {
				t.Fatal("prime.New() = false, want a schedule")
			}
			if got := status.Priming(schedule); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Priming() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCollectGivesTheScheduleOverTheAccountsWithTokens(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	collector := status.Collector{
		Prober: &fakeProber{},
		Policy: policy,
		Token:  tokenstest.Files{"work": "test-token-work", "side": "test-token-side"}.Read,
		Now:    func() time.Time { return now },
	}
	if doc := collector.Collect(t.Context(), accounts); !reflect.DeepEqual(doc.Prime, status.Prime{}) {
		t.Errorf("without a day, Collect().Prime = %+v, want none", doc.Prime)
	}

	collector.Prime = config.Prime{Day: config.Day{Start: 8 * time.Hour, End: 23 * time.Hour}}
	want := status.Prime{Day: "08:00-23:00", Window: "5h", Slots: []status.Slot{{Account: "work", At: "04:10"}, {Account: "side", At: "06:40"}}}
	if doc := collector.Collect(t.Context(), accounts); !reflect.DeepEqual(doc.Prime, want) {
		t.Errorf("Collect().Prime = %+v, want %+v: personal has no token, and when each is next primed is the router's to say", doc.Prime, want)
	}
}

func TestComing(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	session := func(resetsIn time.Duration) quota.Window {
		return quota.Window{Key: "5h", Label: "Session", Utilization: 0.2, ResetsAt: now.Add(resetsIn)}
	}
	week := quota.Window{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: now.Add(time.Hour)}
	priming := status.Prime{Day: "08:00-23:00", Window: "5h", Slots: []status.Slot{{Account: "work", At: "04:15"}, {Account: "side", At: "06:45"}}}
	primed := priming
	primed.Slots = []status.Slot{{Account: "work", At: "04:15", Next: now.Add(3 * time.Hour)}, {Account: "side", At: "06:45", Next: now.Add(2 * time.Hour)}}
	work := status.Account{ID: "work", Label: "Work", TokenSet: true, Windows: []quota.Window{session(3 * time.Hour), week}}
	side := status.Account{ID: "side", Label: "Side", TokenSet: true, Windows: []quota.Window{session(2 * time.Hour)}}
	lapsed := status.Account{ID: "side", Label: "Side", TokenSet: true, Windows: []quota.Window{{Key: "5h", Label: "Session"}}, Lapsed: []string{"5h"}}
	tests := []struct {
		name     string
		prime    status.Prime
		accounts []status.Account
		// want are what comes next, as what, account and when it comes.
		want []string
	}{
		{
			name:     "the next session to reset, of the router's document, and its next prime",
			prime:    primed,
			accounts: []status.Account{work, side},
			want:     []string{"next reset side 2h0m0s", "next prime side 2h0m0s"},
		},
		{
			name:     "the next session to reset, not a week resetting sooner, of a probed document",
			prime:    priming,
			accounts: []status.Account{work},
			want:     []string{"next reset work 3h0m0s"},
		},
		{
			name:     "the next prime alone, with every session lapsed",
			prime:    primed,
			accounts: []status.Account{lapsed},
			want:     []string{"next prime side 2h0m0s"},
		},
		{name: "nothing, with priming off", accounts: []status.Account{work, side}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{Source: status.SourceRouter, Prime: tt.prime, Accounts: tt.accounts}
			var got []string
			for _, c := range doc.Coming(now) {
				got = append(got, c.What+" "+c.Account.ID+" "+c.At.Sub(now).String())
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("Coming() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNotStarted(t *testing.T) {
	now := time.Date(2026, 9, 28, 23, 30, 0, 0, time.FixedZone("UTC+1", 60*60))
	next := time.Date(2026, 9, 29, 3, 15, 0, 0, time.UTC)
	tests := []struct {
		name  string
		prime status.Prime
		want  string
	}{
		{name: "with when the router next primes the account", prime: status.Prime{Slots: []status.Slot{{Account: "work", At: "04:15", Next: next}}}, want: "not started · next prime Tue 04:15"},
		{name: "without, from a probed document", prime: status.Prime{Slots: []status.Slot{{Account: "work", At: "04:15"}}}, want: "not started"},
		{name: "without priming", want: "not started"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{Prime: tt.prime}
			if got := doc.NotStarted("work", now); got != tt.want {
				t.Errorf("NotStarted() = %q, want %q", got, tt.want)
			}
		})
	}
}
