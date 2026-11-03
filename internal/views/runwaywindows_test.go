package views_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// ultraAccount is the account with the given id read at stackAt as a
// verified Ultra account reads: its week alone, a third used, no 5-hour
// window.
func ultraAccount(id string) status.Account {
	return weekAccount(id, stackAt, 0.3, stackAt.Add(3*quota.Day))
}

// fiveHourAccount is the account with the given id read at stackAt with its
// week and a 5-hour window, a fifth used, resetting in two hours.
func fiveHourAccount(id string) status.Account {
	return weekAccount(id, stackAt, 0.3, stackAt.Add(3*quota.Day), func(a *status.Account) {
		a.Windows = append(a.Windows, quota.Window{Key: claude.FiveHourWindow, Label: "5-hour", Utilization: 0.2, ResetsAt: stackAt.Add(2 * time.Hour)})
	})
}

// unreadAccount is the account with the given id, its token set, nothing
// read of it yet.
func unreadAccount(id string) status.Account {
	return status.Account{ID: id, Label: id, TokenSet: true}
}

// windowsMoment is the moment at stackAt of the router's document of the
// accounts given, the global pin naming those pinned, where it names any.
func windowsMoment(pinned []string, accounts ...status.Account) status.Moment {
	doc := status.Document{GeneratedAt: stackAt, Source: status.SourceRouter, Router: status.Health{Healthy: true}, Accounts: accounts}
	if len(pinned) > 0 {
		doc.Pin = status.Pin{Accounts: pinned, Since: stackAt.Add(-time.Hour)}
	}
	return doc.WorkedOut(claude.Policy()).At(stackAt, claude.Policy())
}

// Runway offers the week always, and the 5-hour only while some account has
// one or may have one, as one not read yet may: never where every account
// lacks it, as every verified Ultra account does. Each view is named for
// the key of its window at the moment, none for a window no account has.
func TestRunwayOffersTheFiveHourOnlyWhereSomeAccountHasOrMayHaveOne(t *testing.T) {
	tests := []struct {
		name     string
		accounts []status.Account
		want     []views.RunwayWindow
		fiveHour string
	}{
		{
			name:     "every account verified Ultra",
			accounts: []status.Account{ultraAccount("work"), ultraAccount("personal"), ultraAccount("side")},
			want:     []views.RunwayWindow{views.RunwayWeek},
		},
		{
			name:     "side alone keeping its 5-hour",
			accounts: []status.Account{ultraAccount("work"), ultraAccount("personal"), fiveHourAccount("side")},
			want:     []views.RunwayWindow{views.RunwayWeek, views.RunwayFiveHour}, fiveHour: claude.FiveHourWindow,
		},
		{
			name:     "an account not read yet among Ultra ones, which may have one",
			accounts: []status.Account{ultraAccount("work"), ultraAccount("personal"), unreadAccount("side")},
			want:     []views.RunwayWindow{views.RunwayWeek, views.RunwayFiveHour}, fiveHour: claude.FiveHourWindow,
		},
		{
			name:     "none read yet",
			accounts: []status.Account{unreadAccount("work"), unreadAccount("personal")},
			want:     []views.RunwayWindow{views.RunwayWeek, views.RunwayFiveHour}, fiveHour: claude.FiveHourWindow,
		},
		{
			name:     "every account with a 5-hour",
			accounts: []status.Account{fiveHourAccount("work"), fiveHourAccount("personal")},
			want:     []views.RunwayWindow{views.RunwayWeek, views.RunwayFiveHour}, fiveHour: claude.FiveHourWindow,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := windowsMoment(nil, tt.accounts...)
			if got := views.RunwayWindowsOf(m); !slices.Equal(got, tt.want) {
				t.Errorf("Runway offers %q, want %q", got, tt.want)
			}
			if got := views.RunwayFiveHour.KeyIn(m); got != tt.fiveHour {
				t.Errorf("the 5-hour view is of %q, want %q", got, tt.fiveHour)
			}
			if got := views.RunwayWeek.KeyIn(m); got != claude.WeekWindow {
				t.Errorf("the week view is of %q, want %q", got, claude.WeekWindow)
			}
		})
	}
}

// The 5-hour view has a lane for each account that has or may have a 5-hour
// window, none for one that lacks it, as a verified Ultra account does; the
// week view a lane for every account. The pool's lane is of the window's Of,
// the pool's accounts that have it, where the pool keeps the window, as a
// pin of one does; else, before any reports the window, of its accounts with
// lanes, where the pool doesn't lack the window, as status's PoolLacks has
// it, as a pin of one not read yet doesn't. It goes where there's no pool,
// as with one account, or where the pool lacks the window, as with a pin
// naming one account that has it and one that doesn't; never for one lane
// alone, as a pin of the one account left with the window keeps it.
func TestTheFiveHourViewListsOnlyTheAccountsThatHaveOne(t *testing.T) {
	tests := []struct {
		name     string
		pinned   []string
		accounts []status.Account
		window   views.RunwayWindow
		lanes    []string
		of       int
	}{
		{
			name:     "one account",
			accounts: []status.Account{fiveHourAccount("work")},
			window:   views.RunwayFiveHour, lanes: []string{"work"},
		},
		{
			name:     "two accounts, an Ultra one read and one not read yet",
			accounts: []status.Account{ultraAccount("work"), unreadAccount("personal")},
			window:   views.RunwayFiveHour, lanes: []string{"personal"},
		},
		{
			name:     "an Ultra account among three",
			accounts: []status.Account{ultraAccount("work"), fiveHourAccount("personal"), fiveHourAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"personal", "side"}, of: 2,
		},
		{
			name:     "one account left with a 5-hour",
			accounts: []status.Account{ultraAccount("work"), ultraAccount("personal"), fiveHourAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"side"},
		},
		{
			name:     "an account not read yet, which may have one",
			accounts: []status.Account{ultraAccount("work"), unreadAccount("personal"), fiveHourAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"personal", "side"}, of: 2,
		},
		{
			name:     "none read yet",
			accounts: []status.Account{unreadAccount("work"), unreadAccount("personal"), unreadAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"work", "personal", "side"}, of: 3,
		},
		{
			name:     "starting up, two not read yet beside an Ultra one",
			accounts: []status.Account{ultraAccount("work"), unreadAccount("personal"), unreadAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"personal", "side"}, of: 2,
		},
		{
			name:     "a pin naming one account, a pool of one",
			pinned:   []string{"side"},
			accounts: []status.Account{fiveHourAccount("work"), fiveHourAccount("personal"), fiveHourAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"work", "personal", "side"}, of: 1,
		},
		{
			name:     "a pin naming the one account left with a 5-hour",
			pinned:   []string{"side"},
			accounts: []status.Account{ultraAccount("work"), ultraAccount("personal"), fiveHourAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"side"}, of: 1,
		},
		{
			name:     "a pin naming one account not read yet, the 5-hour",
			pinned:   []string{"side"},
			accounts: []status.Account{fiveHourAccount("work"), fiveHourAccount("personal"), unreadAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"work", "personal", "side"}, of: 1,
		},
		{
			name:     "a pin naming one account not read yet, the week",
			pinned:   []string{"side"},
			accounts: []status.Account{fiveHourAccount("work"), fiveHourAccount("personal"), unreadAccount("side")},
			window:   views.RunwayWeek, lanes: []string{"work", "personal", "side"}, of: 1,
		},
		{
			name:     "a pin naming an account with one and an Ultra one",
			pinned:   []string{"work", "personal"},
			accounts: []status.Account{fiveHourAccount("work"), ultraAccount("personal"), fiveHourAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"work", "side"},
		},
		{
			name:     "every account with one",
			accounts: []status.Account{fiveHourAccount("work"), fiveHourAccount("personal"), fiveHourAccount("side")},
			window:   views.RunwayFiveHour, lanes: []string{"work", "personal", "side"}, of: 3,
		},
		{
			name:     "the week, Ultra accounts among them",
			accounts: []status.Account{ultraAccount("work"), ultraAccount("personal"), fiveHourAccount("side")},
			window:   views.RunwayWeek, lanes: []string{"work", "personal", "side"}, of: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := windowsMoment(tt.pinned, tt.accounts...)
			l := views.Lanes{}.Look(views.LanesFrom{Moment: m, Timeline: views.LiveTimeline(tt.window, stackAt, 136), Window: tt.window})
			var ids []string
			for _, lane := range l.Accounts {
				ids = append(ids, lane.ID)
			}
			if !slices.Equal(ids, tt.lanes) {
				t.Errorf("the lanes are %q's, want %q's", ids, tt.lanes)
			}
			if l.Pool.Of != tt.of {
				t.Errorf("the pool's lane is of %d, want %d", l.Pool.Of, tt.of)
			}
		})
	}
}
