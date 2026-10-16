package capture

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

// trail is an account's use of one of its windows as it runs now: when it
// started, and its use as read since, in order.
type trail struct {
	account, window string
	start           time.Time
	readings        []router.HistoryPoint
}

// sessionLength is how long the session runs.
const sessionLength = 5 * time.Hour

// history is the account's use of the window from its start until now, a
// point each step, as GET /history gives it: each the use last read at or
// before its time, but for points before the first reading.
func (t trail) history(step time.Duration, now time.Time) router.AccountHistory {
	h := router.AccountHistory{ID: t.account, Start: t.start}
	for at := t.start; !at.After(now); at = at.Add(step) {
		i, found := slices.BinarySearchFunc(t.readings, at, func(r router.HistoryPoint, at time.Time) int { return r.At.Compare(at) })
		if !found {
			i--
		}
		if i >= 0 {
			h.Points = append(h.Points, router.HistoryPoint{At: at, Utilization: t.readings[i].Utilization})
		}
	}
	return h
}
