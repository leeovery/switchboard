package watch

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// reading is an account as it was last read, and when.
type reading struct {
	account status.Account
	at      time.Time
}

// lastReads are each account as it was last read, by id. A new document is
// compared with them rather than with the document before it, so an account
// that couldn't be read for a while is compared with how it last stood.
type lastReads map[string]reading

// alerts are the notifications doc, read at now, calls for, of those settings
// asks for: each account that had no room under the windows every model
// shares and now has some, and each window that has passed the warning. An
// account is only compared with an earlier reading, so the first document
// calls for none.
func (r lastReads) alerts(doc status.Document, now time.Time, policy score.Policy, settings config.Notifications) []notify.Notice {
	var alerts []notify.Notice
	for _, a := range doc.Accounts {
		last, ok := r[a.ID]
		if !ok || !wasRead(a) {
			continue
		}
		if settings.Room && roomAgain(last, reading{account: a, at: now}, policy) {
			alerts = append(alerts, notify.RoomAgain(a))
		}
		for _, w := range passed(last.account.Windows, a.Windows, settings.Warning) {
			alerts = append(alerts, notify.Warning(a, w))
		}
	}
	return alerts
}

// with returns the readings updated with each account doc read at now.
func (r lastReads) with(doc status.Document, now time.Time) lastReads {
	updated := make(lastReads, len(r))
	maps.Copy(updated, r)
	for _, a := range doc.Accounts {
		if wasRead(a) {
			updated[a.ID] = reading{account: a, at: now}
		}
	}
	return updated
}

// wasRead reports whether an account's usage was read: a reading always has
// windows.
func wasRead(a status.Account) bool {
	return len(a.Windows) > 0
}

// roomAgain reports whether an account had no room under the policy's shared
// windows at its last reading, its reserve left unused, and has at this one.
// Each reading is judged at the time it was taken: judged now, a window
// exhausted then that has since reset would count as having had room, and the
// account's return would pass unannounced.
func roomAgain(last, this reading, policy score.Policy) bool {
	return !score.Available(last.account.Windows, last.account.Reserve, policy.IsShared, last.at) &&
		score.Available(this.account.Windows, this.account.Reserve, policy.IsShared, this.at)
}

// passed lists the windows in after that have passed warning since before.
func passed(before, after []quota.Window, warning float64) []quota.Window {
	var up []quota.Window
	for _, w := range after {
		i := slices.IndexFunc(before, func(b quota.Window) bool { return b.Key == w.Key })
		if i >= 0 && notify.Passed(before[i].Utilization, w.Utilization, warning) {
			up = append(up, w)
		}
	}
	return up
}
