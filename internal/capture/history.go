package capture

import (
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
)

// trail is an account's window as the router holds it: the window as the
// status document reads it, and its readings, in the order taken.
type trail struct {
	account string
	window  quota.Window
	read    []readings.Reading
}

// sessionLength is how long the session runs.
const sessionLength = 5 * time.Hour

// history is the account's use of the window over its length back from now,
// a point each step, as GET /history gives it, as router.HistoryOf has it.
func (t trail) history(step time.Duration, now time.Time) router.AccountHistory {
	length, _ := quota.Length(t.window.Key)
	return router.HistoryOf(t.account, t.window, t.read, step, now.Add(-length), now)
}
