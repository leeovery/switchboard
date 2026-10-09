package views

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// Windows are each account's every window's readings over the days asked
// for, as the readings history holds them: what GRAPHS, and a day Runway
// steps back to, draw from.
type Windows struct {
	Windows []WindowReadings `json:"windows"`
}

// WindowReadings are an account's window's readings, in the order they were
// read: one each time the window's use, its reset or its status changed.
type WindowReadings struct {
	Account  string          `json:"account"`
	Window   string          `json:"window"`
	Readings []WindowReading `json:"readings"`
}

// WindowReading is a reading of a window as the readings history holds it,
// but for the account and the window, which its WindowReadings name.
type WindowReading struct {
	At          time.Time       `json:"at"`
	Utilization float64         `json:"utilization"`
	ResetsAt    time.Time       `json:"resets_at,omitzero"`
	Status      quota.Status    `json:"status,omitempty"`
	Source      readings.Source `json:"source"`
}

// NewWindows builds the Windows of the local days from from's to today's,
// now the time now, as history reads them: the configured accounts' first,
// in the config's order, then any other's, by id, each one's windows in
// quota's order.
func NewWindows(history Readings, accounts config.Accounts, from, now time.Time) Windows {
	type window struct{ account, key string }
	held := make(map[window][]WindowReading)
	for r := range history.Between(dayfile.DayStart(from.Local(), 0), dayfile.DayStart(now.Local(), 1)) {
		w := window{account: r.Account, key: r.Key}
		held[w] = append(held[w], WindowReading{At: r.At, Utilization: r.Utilization, ResetsAt: r.ResetsAt, Status: r.Status, Source: r.Source})
	}
	order := byConfig(accounts)
	listed := slices.SortedFunc(maps.Keys(held), func(a, b window) int {
		return cmp.Or(order(a.account, b.account), quota.CompareKeys(a.key, b.key))
	})
	windows := Windows{Windows: make([]WindowReadings, len(listed))}
	for i, w := range listed {
		windows.Windows[i] = WindowReadings{Account: w.account, Window: w.key, Readings: held[w]}
	}
	return windows
}

// byConfig orders accounts' ids as the config orders the accounts, those it
// doesn't configure after, by id.
func byConfig(accounts config.Accounts) func(a, b string) int {
	place := func(id string) int {
		if i := slices.IndexFunc(accounts, func(a config.Account) bool { return a.ID == id }); i >= 0 {
			return i
		}
		return len(accounts)
	}
	return func(a, b string) int {
		return cmp.Or(cmp.Compare(place(a), place(b)), cmp.Compare(a, b))
	}
}
