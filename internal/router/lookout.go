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
// stood when last shown: an account without room that has some again, and a
// window that has passed the warning since, call for notifications. It knows
// nothing of an account until the account has been read or barred, so the
// first look at one calls for none.
type lookout struct {
	room bool
	// warning is the share of a window's limit whose passing is told of, or 0
	// for none.
	warning float64
	// rooms holds, by id, whether each account had room when last known.
	rooms map[string]bool
	// levels holds each window's utilization as last shown.
	levels map[windowOf]float64
	// warned holds when the window each warning was last due in resets.
	warned map[windowOf]time.Time
}

func newLookout(room bool, warning float64) *lookout {
	return &lookout{
		room:    room,
		warning: warning,
		rooms:   make(map[string]bool),
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

// roomAgain reports whether the account had no room when last known and has
// some now, when room again is told of.
func (l *lookout) roomAgain(s standing) bool {
	if !s.known {
		return false
	}
	had, knew := l.rooms[s.ID]
	l.rooms[s.ID] = s.room
	return l.room && knew && !had && s.room
}

// passed returns the account's windows that have passed the warning since
// they were last shown, each once a reset.
func (l *lookout) passed(s standing) []quota.Window {
	var passed []quota.Window
	for _, w := range s.Windows {
		id := windowOf{account: s.ID, key: w.Key}
		before, seen := l.levels[id]
		l.levels[id] = w.Utilization
		if !seen || !l.crosses(before, w.Utilization) || l.warnedIn(id, w) {
			continue
		}
		l.warned[id] = w.ResetsAt
		passed = append(passed, w)
	}
	return passed
}

// crosses reports whether a window used as far as before, and now after, has
// passed the warning.
func (l *lookout) crosses(before, after float64) bool {
	return l.warning > 0 && before < l.warning && after >= l.warning
}

// warnedIn reports whether the window id names was warned of before its reset
// as w reads it.
func (l *lookout) warnedIn(id windowOf, w quota.Window) bool {
	reset, ok := l.warned[id]
	return ok && reset.Equal(w.ResetsAt)
}
