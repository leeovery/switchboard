package router

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

func TestDecide(t *testing.T) {
	var (
		// soon's week resets tomorrow, and later's in five days: soon's quota
		// needs using first.
		soon  = weekAt(0.5, 24*time.Hour)
		later = weekAt(0.5, 5*24*time.Hour)
		// even scores 0.01 an hour; a little ahead of it by 10%, and well
		// ahead by 25%.
		even        = weekAt(0.5, 50*time.Hour)
		littleAhead = weekAt(0.45, 50*time.Hour)
		wellAhead   = weekAt(0.375, 50*time.Hour)
		spent       = []quota.Window{
			{Key: "5h", Utilization: 1, ResetsAt: start.Add(2 * time.Hour), Status: quota.StatusRejected},
			{Key: "7d", Utilization: 0.5, ResetsAt: start.Add(72 * time.Hour)},
		}
		// unread is an account nothing has been read of.
		unread []quota.Window
	)
	pinSide := status.Pin{Account: "side", Since: start.Add(-time.Hour)}
	moveToSide := status.Pin{Account: "side", Since: start.Add(-time.Hour), Move: true}
	hitLimit := func(id string) []Attempt { return []Attempt{{Account: id, Why: "hit its limit"}} }
	tests := []struct {
		name string
		// unsessioned leaves the request without a session.
		unsessioned bool
		// pin is the request's own.
		pin string
		// current is the session's assignment, nil for a new session.
		current *assignment
		global  status.Pin
		// tried are the accounts the request has gone out on already.
		tried []Attempt
		// work's and side's windows. Personal can't be sent on.
		work, side []quota.Window
		want       decision
	}{
		{
			name:        "a request without a session goes to the account whose quota most needs using",
			unsessioned: true,
			work:        later, side: soon,
			want: decision{account: "side", reason: "unsessioned", afresh: true},
		},
		{
			name: "a new session goes to the account whose quota most needs using",
			work: later, side: soon,
			want: decision{account: "side", reason: "new", afresh: true},
		},
		{
			name: "a new session goes to an account a little ahead, preferring none",
			work: even, side: littleAhead,
			want: decision{account: "side", reason: "new", afresh: true},
		},
		{
			name: "a new session keeps to the client's account when none has room",
			work: spent, side: spent,
			want: decision{account: "work", reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name: "a new session keeps to the client's account when nothing's been read",
			work: unread, side: unread,
			want: decision{account: "work", reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name:    "a session stays while its cache is warm, however far ahead another account is",
			current: on("work", 59*time.Minute),
			work:    later, side: soon,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "a session idle an hour exactly stays",
			current: on("work", time.Hour),
			work:    later, side: soon,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "a session idle past the hour is rescored",
			current: on("work", time.Hour+time.Second),
			work:    later, side: soon,
			want: decision{account: "side", reason: "rescored after 1h idle", afresh: true},
		},
		{
			name:    "a rescored session keeps its account against one a little ahead",
			current: on("work", 2*time.Hour+30*time.Minute),
			work:    even, side: littleAhead,
			want: decision{account: "work", reason: "rescored after 2h 30m idle", afresh: true},
		},
		{
			name:    "a rescored session leaves its account for one well ahead",
			current: on("work", 3*24*time.Hour),
			work:    even, side: wellAhead,
			want: decision{account: "side", reason: "rescored after 3d idle", afresh: true},
		},
		{
			name:    "a session whose account has no room moves",
			current: on("work", 5*time.Minute),
			work:    spent, side: later,
			want: decision{account: "side", reason: "moved: work has no room", afresh: true},
		},
		{
			name:    "a session whose account has no room stays when none has",
			current: on("side", 5*time.Minute),
			work:    spent, side: spent,
			want: decision{account: "side", reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name:    "a session on an account nothing's been read of stays",
			current: on("work", 5*time.Minute),
			work:    unread, side: soon,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "a session on an account nothing can go out on moves",
			current: on("personal", 5*time.Minute),
			work:    later, side: soon,
			want: decision{account: "side", reason: "moved: personal has no room", afresh: true},
		},
		{
			name:    "a session on an account nothing can go out on goes to the client's when none has room",
			current: on("personal", 5*time.Minute),
			work:    spent, side: spent,
			want: decision{account: "work", reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name: "a session's pin sends it to its account",
			pin:  "side",
			work: soon, side: later,
			want: decision{account: "side", reason: "pinned"},
		},
		{
			name: "a session's pin holds on an account nothing's been read of",
			pin:  "side",
			work: soon, side: unread,
			want: decision{account: "side", reason: "pinned"},
		},
		{
			name:    "a session's pin moves a warm session it's new to",
			pin:     "side",
			current: on("work", 5*time.Minute),
			work:    soon, side: later,
			want: decision{account: "side", reason: "pinned"},
		},
		{
			name:   "a session's pin beats the global pin",
			pin:    "side",
			global: status.Pin{Account: "work", Since: start.Add(-time.Hour)},
			work:   soon, side: later,
			want: decision{account: "side", reason: "pinned"},
		},
		{
			name: "a session's pin yields when its account has no room",
			pin:  "side",
			work: later, side: spent,
			want: decision{account: "work", reason: "pin yields: side has no room", afresh: true},
		},
		{
			name:    "a session's pin yields to the session's account while its cache is warm",
			pin:     "side",
			current: on("work", 5*time.Minute),
			work:    later, side: spent,
			want: decision{account: "work", reason: "pin yields: side has no room", sticky: true},
		},
		{
			name: "a session's pin yields to none when no account has room",
			pin:  "side",
			work: spent, side: spent,
			want: decision{account: "work", reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name:    "a session that yielded its pin isn't brought back while its cache is warm",
			pin:     "side",
			current: pinned(on("work", 5*time.Minute), "side"),
			work:    later, side: soon,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "a session that yielded its pin goes back once its cache is cold",
			pin:     "side",
			current: pinned(on("work", 2*time.Hour), "side"),
			work:    soon, side: later,
			want: decision{account: "side", reason: "pinned"},
		},
		{
			name:    "a session that yielded its pin goes back when its account has no room",
			pin:     "side",
			current: pinned(on("work", 5*time.Minute), "side"),
			work:    spent, side: later,
			want: decision{account: "side", reason: "pinned"},
		},
		{
			name:   "the global pin sends a new session to its account",
			global: pinSide,
			work:   soon, side: later,
			want: decision{account: "side", reason: "pinned (global)", afresh: true},
		},
		{
			name:   "the global pin yields when its account has no room",
			global: pinSide,
			work:   later, side: spent,
			want: decision{account: "work", reason: "new", afresh: true},
		},
		{
			name:    "the global pin leaves a warm session where it is",
			global:  pinSide,
			current: assignedAt(on("work", 5*time.Minute), start.Add(-2*time.Hour)),
			work:    soon, side: later,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "the global pin takes a session idle past the hour",
			global:  pinSide,
			current: on("work", 2*time.Hour),
			work:    soon, side: later,
			want: decision{account: "side", reason: "pinned (global)", afresh: true},
		},
		{
			name:    "the global pin takes a session whose account has no room",
			global:  pinSide,
			current: on("work", 5*time.Minute),
			work:    spent, side: later,
			want: decision{account: "side", reason: "pinned (global)", afresh: true},
		},
		{
			name:    "a global pin that moves sessions moves one assigned before it",
			global:  moveToSide,
			current: assignedAt(on("work", 5*time.Minute), start.Add(-2*time.Hour)),
			work:    soon, side: later,
			want: decision{account: "side", reason: "moved by pin"},
		},
		{
			name:    "a global pin that moves sessions moves one idle past the hour",
			global:  moveToSide,
			current: assignedAt(on("work", 90*time.Minute), start.Add(-2*time.Hour)),
			work:    soon, side: later,
			want: decision{account: "side", reason: "moved by pin"},
		},
		{
			name:    "a global pin that moves sessions moves each once",
			global:  moveToSide,
			current: assignedAt(on("work", 5*time.Minute), start.Add(-30*time.Minute)),
			work:    soon, side: later,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "a global pin that moves sessions waits while its account has no room",
			global:  moveToSide,
			current: assignedAt(on("work", 5*time.Minute), start.Add(-2*time.Hour)),
			work:    soon, side: spent,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "a global pin that moves sessions leaves one on its account",
			global:  moveToSide,
			current: assignedAt(on("side", 5*time.Minute), start.Add(-2*time.Hour)),
			work:    soon, side: later,
			want: decision{account: "side", reason: "sticky", sticky: true},
		},
		{
			name:    "a global pin that moves sessions leaves one its own pin holds",
			pin:     "work",
			global:  moveToSide,
			current: pinned(assignedAt(on("work", 5*time.Minute), start.Add(-2*time.Hour)), "work"),
			work:    soon, side: later,
			want: decision{account: "work", reason: "pinned"},
		},
		{
			name:    "a session whose account hit its limit on the request moves, however warm its cache",
			current: on("work", 0),
			tried:   hitLimit("work"),
			work:    soon, side: later,
			want: decision{account: "side", reason: "moved: work hit its limit", afresh: true},
		},
		{
			name:    "a session whose account refused the request moves",
			current: on("work", 0),
			tried:   []Attempt{{Account: "work", Why: "was refused"}},
			work:    soon, side: later,
			want: decision{account: "side", reason: "moved: work was refused", afresh: true},
		},
		{
			name:  "a new session tried on an account goes to another",
			tried: hitLimit("side"),
			work:  later, side: soon,
			want: decision{account: "work", reason: "new", afresh: true},
		},
		{
			name:    "a session's pin yields once its account hit its limit on the request",
			pin:     "side",
			current: pinned(on("side", 0), "side"),
			tried:   hitLimit("side"),
			work:    later, side: soon,
			want: decision{account: "work", reason: "pin yields: side hit its limit", afresh: true},
		},
		{
			name:    "a session tried on every account with room finds none, and stays",
			current: on("work", 0),
			tried:   hitLimit("work"),
			work:    soon, side: spent,
			want: decision{account: "work", reason: "no account has room", afresh: true, noRoom: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{Session: "0b5c6f2e", Model: opus, Pin: tt.pin, Client: "work", Tried: tt.tried}
			if tt.unsessioned {
				req.Session = ""
			}
			s := situation{req: req, now: start, pin: tt.global, accounts: known(tt.work, tt.side).without(req.tried())}
			if tt.current != nil {
				s.current, s.assigned = *tt.current, true
			}
			if got := decide(s); got != tt.want {
				t.Errorf("decide() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWithNoRoomARequestFallsBackToAnAccountThatHasntRefusedIt(t *testing.T) {
	spent := []quota.Window{
		{Key: "5h", Utilization: 1, ResetsAt: start.Add(2 * time.Hour), Status: quota.StatusRejected},
		{Key: "7d", Utilization: 0.5, ResetsAt: start.Add(72 * time.Hour)},
	}
	tests := []struct {
		name string
		// current is the session's account, or "" for a new session.
		current string
		// refused are the accounts that refused the request lately.
		refused []string
		want    string
	}{
		{name: "the session's account", current: "side", want: "side"},
		{name: "the client's, for a new session", want: "work"},
		{name: "the client's, when the session's refused the request", current: "side", refused: []string{"side"}, want: "work"},
		{name: "another, when the client's refused it too", current: "side", refused: []string{"side", "work"}, want: "spare"},
		{name: "another, for a new session whose client's refused it", refused: []string{"work"}, want: "side"},
		{name: "the client's, when every one refused it", current: "side", refused: []string{"work", "side", "spare"}, want: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := situation{
				req: Request{Session: "0b5c6f2e", Model: opus, Client: "work"},
				now: start,
				accounts: view{
					policy:     testPolicy,
					now:        start,
					candidates: []score.Candidate{{ID: "work", Windows: spent}, {ID: "side", Windows: spent}, {ID: "spare", Windows: spent}},
					applies:    testPolicy.IsShared,
					barred:     tt.refused,
					refused:    tt.refused,
				},
			}
			if tt.current != "" {
				s.current, s.assigned = *on(tt.current, 5*time.Minute), true
			}
			want := decision{account: tt.want, reason: reasonNoRoom, afresh: true, noRoom: true}
			if got := decide(s); got != want {
				t.Errorf("decide() = %+v, want %+v", got, want)
			}
		})
	}
}

// known is what a choice knows of work and side, whose windows are as given,
// at start: personal, without a token, can't be sent on.
func known(work, side []quota.Window) view {
	return view{
		policy:     testPolicy,
		now:        start,
		candidates: []score.Candidate{{ID: "work", Windows: work}, {ID: "side", Windows: side}},
		applies:    testPolicy.IsShared,
	}
}

// weekAt is the windows of an account with room in its session, whose week is
// used as given and resets after left.
func weekAt(used float64, left time.Duration) []quota.Window {
	return []quota.Window{
		{Key: "5h", Utilization: 0.1, ResetsAt: start.Add(3 * time.Hour)},
		{Key: "7d", Utilization: used, ResetsAt: start.Add(left)},
	}
}

// on is a session's assignment to account, made the day before start, and
// last used idle before start.
func on(account string, idle time.Duration) *assignment {
	return &assignment{Account: account, Reason: "new", AssignedAt: start.Add(-24 * time.Hour), LastSeen: start.Add(-idle)}
}

// pinned is a with the session's own pin naming account.
func pinned(a *assignment, account string) *assignment {
	a.Pin = account
	return a
}

// assignedAt is a, made at t.
func assignedAt(a *assignment, t time.Time) *assignment {
	a.AssignedAt = t
	return a
}
