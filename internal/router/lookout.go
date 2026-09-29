package router

import (
	"time"

	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/quota"
)

// windowOf names one account's window.
type windowOf struct {
	account, key string
}

// lookout compares the accounts, each time it's shown them, with how they
// stood before: an account whose quota ran out and has come back, and a
// window that has passed the warning since last shown, call for
// notifications. It knows nothing of an account's quota until the account has
// been read or a limit has barred it, so the first look at one calls for none.
type lookout struct {
	room bool
	// warning is the share of a window's limit whose passing is told of, or 0
	// for none.
	warning float64
	// short holds, by id, the accounts whose quota ran out and that haven't
	// had room since.
	short map[string]bool
	// levels holds each window's utilization as last shown.
	levels map[windowOf]float64
	// warned holds when the window each warning was last due in resets.
	warned map[windowOf]time.Time
}

func newLookout(room bool, warning float64) *lookout {
	return &lookout{
		room:    room,
		warning: warning,
		short:   make(map[string]bool),
		levels:  make(map[windowOf]float64),
		warned:  make(map[windowOf]time.Time),
	}
}

// look returns the notices the accounts, as they stand, call for, and
// remembers how they stand.
func (l *lookout) look(accounts standings) []notify.Notice {
	var due []notify.Notice
	for _, s := range accounts {
		if l.roomAgain(s) {
			due = append(due, notify.RoomAgain(s.Account))
		}
		for _, w := range l.passed(s) {
			due = append(due, notify.Warning(s.Account, w))
		}
	}
	return due
}

// roomAgain reports whether the account's quota ran out since it last had
// room, and it has room now, when room again is told of: its quota is back,
// and its token isn't refused. A refusal isn't the quota running out, so one
// lifting says nothing of its own, which a revoked token would otherwise say
// each time the router tried it again.
func (l *lookout) roomAgain(s standing) bool {
	switch {
	case !s.known:
		return false
	case !s.quota:
		l.short[s.ID] = true
		return false
	case s.refused || !l.short[s.ID]:
		return false
	}
	delete(l.short, s.ID)
	return l.room
}

// passed returns the account's windows that have passed the warning since
// they were last shown, each once a reset.
func (l *lookout) passed(s standing) []quota.Window {
	var passed []quota.Window
	for _, w := range s.Windows {
		id := windowOf{account: s.ID, key: w.Key}
		before, seen := l.levels[id]
		l.levels[id] = w.Utilization
		if !seen || !notify.Passed(before, w.Utilization, l.warning) || l.warnedIn(id, w) {
			continue
		}
		l.warned[id] = w.ResetsAt
		passed = append(passed, w)
	}
	return passed
}

// warnedIn reports whether the window id names was warned of before its reset
// as w reads it.
func (l *lookout) warnedIn(id windowOf, w quota.Window) bool {
	reset, ok := l.warned[id]
	return ok && reset.Equal(w.ResetsAt)
}
