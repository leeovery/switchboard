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

// Runway's lanes judge the week, the first window lasting longer than a day
// that every model shares, whatever else the accounts have or lack.
func TestRunwaysLanesJudgeTheWeek(t *testing.T) {
	for _, accounts := range [][]status.Account{
		{ultraAccount("work"), ultraAccount("personal")},
		{ultraAccount("work"), fiveHourAccount("personal"), unreadAccount("side")},
		{unreadAccount("work"), unreadAccount("personal")},
	} {
		if got := views.RunwayWeek.KeyIn(windowsMoment(nil, accounts...)); got != claude.WeekWindow {
			t.Errorf("the week is of %q, want %q", got, claude.WeekWindow)
		}
	}
}

// Lanes judging the 5-hour have a lane for each account that has or may have
// a 5-hour window, none for one that lacks it, as a verified Ultra account
// does; judging the week, a lane for every account. The pool's lane is of the
// window's Of, the pool's accounts that have it, where the pool keeps the
// window, as a pin of one does; else, before any reports the window, of its
// accounts with lanes, where the pool doesn't lack the window, as status's
// PoolLacks has it, as a pin of one not read yet doesn't. It goes where
// there's no pool, as with one account, or where the pool lacks the window,
// as with a pin naming one account that has it and one that doesn't; never
// for one lane alone, as a pin of the one account left with the window keeps
// it.
func TestLanesJudgingAWindowListOnlyTheAccountsThatHaveIt(t *testing.T) {
	tests := []struct {
		name     string
		pinned   []string
		accounts []status.Account
		key      string
		lanes    []string
		of       int
	}{
		{
			name:     "one account",
			accounts: []status.Account{fiveHourAccount("work")},
			key:      claude.FiveHourWindow, lanes: []string{"work"},
		},
		{
			name:     "two accounts, an Ultra one read and one not read yet",
			accounts: []status.Account{ultraAccount("work"), unreadAccount("personal")},
			key:      claude.FiveHourWindow, lanes: []string{"personal"},
		},
		{
			name:     "an Ultra account among three",
			accounts: []status.Account{ultraAccount("work"), fiveHourAccount("personal"), fiveHourAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"personal", "side"}, of: 2,
		},
		{
			name:     "one account left with a 5-hour",
			accounts: []status.Account{ultraAccount("work"), ultraAccount("personal"), fiveHourAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"side"},
		},
		{
			name:     "an account not read yet, which may have one",
			accounts: []status.Account{ultraAccount("work"), unreadAccount("personal"), fiveHourAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"personal", "side"}, of: 2,
		},
		{
			name:     "none read yet",
			accounts: []status.Account{unreadAccount("work"), unreadAccount("personal"), unreadAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"work", "personal", "side"}, of: 3,
		},
		{
			name:     "starting up, two not read yet beside an Ultra one",
			accounts: []status.Account{ultraAccount("work"), unreadAccount("personal"), unreadAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"personal", "side"}, of: 2,
		},
		{
			name:     "a pin naming one account, a pool of one",
			pinned:   []string{"side"},
			accounts: []status.Account{fiveHourAccount("work"), fiveHourAccount("personal"), fiveHourAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"work", "personal", "side"}, of: 1,
		},
		{
			name:     "a pin naming the one account left with a 5-hour",
			pinned:   []string{"side"},
			accounts: []status.Account{ultraAccount("work"), ultraAccount("personal"), fiveHourAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"side"}, of: 1,
		},
		{
			name:     "a pin naming one account not read yet, the 5-hour",
			pinned:   []string{"side"},
			accounts: []status.Account{fiveHourAccount("work"), fiveHourAccount("personal"), unreadAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"work", "personal", "side"}, of: 1,
		},
		{
			name:     "a pin naming one account not read yet, the week",
			pinned:   []string{"side"},
			accounts: []status.Account{fiveHourAccount("work"), fiveHourAccount("personal"), unreadAccount("side")},
			lanes:    []string{"work", "personal", "side"}, of: 1,
		},
		{
			name:     "a pin naming an account with one and an Ultra one",
			pinned:   []string{"work", "personal"},
			accounts: []status.Account{fiveHourAccount("work"), ultraAccount("personal"), fiveHourAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"work", "side"},
		},
		{
			name:     "every account with one",
			accounts: []status.Account{fiveHourAccount("work"), fiveHourAccount("personal"), fiveHourAccount("side")},
			key:      claude.FiveHourWindow, lanes: []string{"work", "personal", "side"}, of: 3,
		},
		{
			name:     "the week, Ultra accounts among them",
			accounts: []status.Account{ultraAccount("work"), ultraAccount("personal"), fiveHourAccount("side")},
			lanes:    []string{"work", "personal", "side"}, of: 3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := windowsMoment(tt.pinned, tt.accounts...)
			l := views.Lanes{}.Look(views.LanesFrom{Moment: m, Timeline: views.LiveTimeline(stackAt, 136), Key: tt.key})
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
