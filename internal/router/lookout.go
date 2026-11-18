package router

import (
	"cmp"
	"time"

	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// windowOf names one account's window.
type windowOf struct {
	account, key string
}

// lookout compares the accounts, each time it's shown them, with how they
// stood before: an account whose quota ran out and has come back, and a
// window that has passed the warning since last shown, call for
// notifications. It knows nothing of an account's quota until the account has
// been read or a limit has barred it, so the first look at one calls for
// none; but a hold of its read refused is known as it starts, as roomAgain
// says.
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
			due = append(due, notify.RoomAgain(s.ID, s.led(s.Title())))
		}
		for _, w := range l.passed(s) {
			due = append(due, notify.Warning(s.ID, s.led(s.Title()), status.NameOfWindow(w.Key, w.Label), w.Utilization))
		}
	}
	return due
}

// ranOut holds, by id, the accounts whose quota ran out, or that were held by
// their read refused, and that haven't had room since, each as shortOf has
// it.
type ranOut map[string]shortOf

// shortOf is what has held an account back since it last had room: the keys
// of the windows that have, each once; and the cause of the latest hold of
// its read refused, as StaleCause's Said words it, "" where none has.
type shortOf struct {
	windows []string
	cause   string
}

// roomAgain reports whether the account's quota ran out, or its read refused
// held it, since it last had room, and it has room now: its quota is back,
// its read isn't refused, and its token isn't refused; and returns what held
// it back meanwhile. A hold is known of an account whatever's known of its
// quota, as its first read can be the one refused. The upstream's refusal
// isn't the quota running out, so one lifting says nothing of its own, which
// a revoked token would otherwise say each time the router tried it again:
// a hold lifts only as a read comes back clean, so its lift is news once.
func (r ranOut) roomAgain(s standing) (shortOf, bool) {
	held, ran := r[s.ID]
	cause, _ := status.RefusedRead(s.RefusedRead).Said()
	switch {
	case cause == "" && !s.known:
		return shortOf{}, false
	case cause != "" || !s.quota:
		r[s.ID] = held.with(s, cause)
		return shortOf{}, false
	case s.refused || !ran:
		return shortOf{}, false
	}
	delete(r, s.ID)
	return held, true
}

// liftedUntold takes in the holds lifted as their accounts' tokens changed
// while the router was down, why each was refused by its account's id, as
// liftsUntold gives them: each held its account back since it last had
// room, so room again tells its lift.
func (r ranOut) liftedUntold(lifts map[string]quota.ReadRefusal) {
	for id, refused := range lifts {
		cause, _ := status.RefusedRead(refused).Said()
		held := r[id]
		held.cause = cmp.Or(held.cause, cause)
		r[id] = held
	}
}

// with is what has held the account back, with what holds it back as it
// stands: the windows that leave it no room, and the cause of its read
// refused, where there's one.
func (h shortOf) with(s standing, cause string) shortOf {
	h.windows = withEach(h.windows, s.held...)
	h.cause = cmp.Or(cause, h.cause)
	return h
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
