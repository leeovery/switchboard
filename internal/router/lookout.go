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
	short   ranOut
	// levels holds each window's utilization as last shown.
	levels map[windowOf]float64
	// warned holds when the window each warning was last due in resets.
	warned map[windowOf]time.Time
}

func newLookout(room bool, warning float64) *lookout {
	return &lookout{
		room:    room,
		warning: warning,
		short:   make(ranOut),
		levels:  make(map[windowOf]float64),
		warned:  make(map[windowOf]time.Time),
	}
}

// look returns the notices the accounts, as they stand, call for, and
// remembers how they stand.
func (l *lookout) look(accounts standings) []notify.Notice {
	var due []notify.Notice
	for _, s := range accounts {
		if _, again := l.short.roomAgain(s); again && l.room {
			due = append(due, notify.RoomAgain(s.Account))
		}
		for _, w := range l.passed(s) {
			due = append(due, notify.Warning(s.Account, w))
		}
	}
	return due
}

// ranOut holds, by id, the accounts whose quota ran out and that haven't had
// room since, each with the keys of the windows that have held it back
// meanwhile, each once.
type ranOut map[string][]string

// roomAgain reports whether the account's quota ran out since it last had
// room, and it has room now: its quota is back, and its token isn't refused;
// and returns the windows that held it back meanwhile. A refusal isn't the
// quota running out, so one lifting says nothing of its own, which a revoked
// token would otherwise say each time the router tried it again.
func (r ranOut) roomAgain(s standing) ([]string, bool) {
	held, ran := r[s.ID]
	switch {
	case !s.known:
		return nil, false
	case !s.quota:
		r[s.ID] = withEach(held, s.held...)
		return nil, false
	case s.refused || !ran:
		return nil, false
	}
	delete(r, s.ID)
	return held, true
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
