package capture

import (
	"cmp"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// The removed world's account removed: its id, and the day of the year it was
// removed on, Friday 4 September, the eight weeks' 26th day.
const (
	teamID      = "h7c2"
	teamRemoved = eightWeeks + 25
)

// goneAccount is an account removed that a world's accounts record keeps, by
// the label it showed last, and its days the ledger holds, by day of the
// year.
type goneAccount struct {
	status.RemovedAccount
	days map[int]ledger.AccountDay
}

// removedTeam is the auto world with an account removed on 4 September, as
// Paper draws History's Days with one: labelled team, its week rising on the
// weekdays of the three weeks to then, and two 5-hour limits on Tuesday 25
// August.
func (a accountWorlds) removedTeam() accountWorld {
	return a.kept("removedTeam", func() accountWorld {
		w := a.autoOf(a.today().three(true))
		w.gone = []goneAccount{teamOf(a.now)}
		return w
	})
}

// teamOf is the removed world's account removed, its times in now's time
// zone.
func teamOf(now time.Time) goneAccount {
	limited := eightWeeks + 15
	days := make(map[int]ledger.AccountDay)
	for d := eightWeeks + 7; d <= teamRemoved; d++ {
		if d%7 < 5 {
			days[d] = ledger.AccountDay{Account: teamID, Rise: map[string]float64{claude.WeekWindow: 0.04 + 0.02*float64(d%3)}}
		}
	}
	day := days[limited]
	day.Limits = []ledger.Limit{
		{Window: claude.FiveHourWindow, At: dayAt(now, limited, 11, 20, 0).UTC()},
		{Window: claude.FiveHourWindow, At: dayAt(now, limited, 16, 45, 0).UTC()},
	}
	days[limited] = day
	removed := status.RemovedAccount{ID: teamID, Label: "team", Removed: dayAt(now, teamRemoved, 16, 0, 0).UTC()}
	return goneAccount{RemovedAccount: removed, days: days}
}

// removedOf are the accounts removed of gone, as the world's accounts record
// keeps them.
func removedOf(gone []goneAccount) status.RemovedAccounts {
	var removed status.RemovedAccounts
	for _, g := range gone {
		removed = append(removed, g.RemovedAccount)
	}
	return removed
}

// withGone is days, the ledger's days of the year from its first, with the
// days of the accounts removed of gone in theirs, each day's accounts in
// the order of their ids.
func withGone(days []ledger.Summary, gone []goneAccount) []ledger.Summary {
	for _, g := range gone {
		for d, day := range g.days {
			s := &days[d]
			i, _ := slices.BinarySearchFunc(s.Accounts, day.Account, func(a ledger.AccountDay, id string) int { return cmp.Compare(a.Account, id) })
			s.Accounts = slices.Insert(s.Accounts, i, day)
		}
	}
	return days
}
