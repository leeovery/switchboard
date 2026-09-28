package watch

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// alertFrom is how much of a window is used when reaching it is announced.
const alertFrom = 0.9

// reading is an account as it was last read, and when.
type reading struct {
	account status.Account
	at      time.Time
}

// readings are each account as it was last read, by id. A new document is
// compared with them rather than with the document before it, so an account
// that couldn't be read for a while is compared with how it last stood.
type readings map[string]reading

// alerts are the notifications doc, read at now, calls for: each account that
// had no room under the windows every model shares and now has some, and each
// window that has reached 90% from below. An account is only compared with an
// earlier reading, so the first document calls for none.
func (r readings) alerts(doc status.Document, now time.Time, policy score.Policy) []string {
	var messages []string
	for _, a := range doc.Accounts {
		last, ok := r[a.ID]
		if !ok || !wasRead(a) {
			continue
		}
		if roomAgain(last, reading{account: a, at: now}, policy) {
			messages = append(messages, a.Title()+" has room again")
		}
		for _, w := range crossed(last.account.Windows, a.Windows) {
			messages = append(messages, fmt.Sprintf("%s: %s at %s", a.Title(), w.Label, status.Percent(w.Utilization)))
		}
	}
	return messages
}

// with returns the readings updated with each account doc read at now.
func (r readings) with(doc status.Document, now time.Time) readings {
	updated := make(readings, len(r))
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
// windows at its last reading, and has at this one. Each reading is judged at
// the time it was taken: judged now, a window exhausted then that has since
// reset would count as having had room, and the account's return would pass
// unannounced.
func roomAgain(last, this reading, policy score.Policy) bool {
	return !score.Available(last.account.Windows, policy.IsShared, last.at) &&
		score.Available(this.account.Windows, policy.IsShared, this.at)
}

// crossed lists the windows in after that have reached alertFrom and were
// below it in before.
func crossed(before, after []quota.Window) []quota.Window {
	var up []quota.Window
	for _, w := range after {
		i := slices.IndexFunc(before, func(b quota.Window) bool { return b.Key == w.Key })
		if i >= 0 && before[i].Utilization < alertFrom && w.Utilization >= alertFrom {
			up = append(up, w)
		}
	}
	return up
}
