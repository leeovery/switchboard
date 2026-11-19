package capture

import (
	"slices"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// addedID is the id of the account the added world adds: spare.
const addedID = "spare"

// added is the auto world with spare added as a fourth account today at
// 09:44, first read then, nothing of it read before, as Runway draws a lane
// new in its view: its 5-hour window started then, 46% used by 14:41, and
// its week, starting then, 12% used by 14:40.
func (a accountWorlds) added() accountWorld {
	return a.kept("added", func() accountWorld {
		t := a.today().three(true)
		since := t.at(9, 44, 0)
		t.session(addedID, since, false, level{t.at(14, 41, 0), .46})
		week := window{account: addedID, key: accountKey, resets: since.Add(7 * quota.Day)}
		t.read.at(week, since, 0)
		t.read.rise(week, since, t.at(14, 40, 0), 0, .12, .01)
		w := a.autoOf(t)
		i := slices.IndexFunc(tenAccounts, func(c config.Account) bool { return c.ID == addedID })
		w.accounts = append(w.accounts, tenAccounts[i])
		w.history = slices.DeleteFunc(t.history(w.accounts), func(r readings.Reading) bool { return r.Account == addedID && r.At.Before(since.UTC()) })
		return w
	})
}
