package router

import (
	"slices"
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
	pinSide := status.Pin{Accounts: []string{"side"}, Since: start.Add(-time.Hour)}
	moveToSide := status.Pin{Accounts: []string{"side"}, Since: start.Add(-time.Hour), Move: true}
	hitLimit := func(id string) []Attempt { return []Attempt{{Account: id, Why: "hit its limit"}} }
	tests := []struct {
		name string
		// unsessioned leaves the request without a session.
		unsessioned bool
		// bound has the request ask for Sonnet, whose thinking is bound to
		// the account that produced it, in place of Opus.
		bound bool
		// pin is the session's own, given while it runs from pinnedAt, else
		// launched with it.
		pin      string
		pinnedAt time.Time
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
			global: status.Pin{Accounts: []string{"work"}, Since: start.Add(-time.Hour)},
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
			name:     "a session given a pin while it runs goes to it on its next request, though it yielded that account's pin before",
			pin:      "side",
			pinnedAt: start.Add(-time.Minute),
			current:  pinned(on("work", 5*time.Minute), "side"),
			work:     later, side: soon,
			want: decision{account: "side", reason: "pinned"},
		},
		{
			name:     "a session that yielded the pin it was given while it ran isn't brought back while its cache is warm",
			pin:      "side",
			pinnedAt: start.Add(-time.Hour),
			current:  pinned(on("work", 5*time.Minute), "side"),
			work:     later, side: soon,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:     "a session given a pin while it runs yields it, as any pin, when its account has no room",
			pin:      "side",
			pinnedAt: start.Add(-time.Minute),
			current:  on("work", 5*time.Minute),
			work:     later, side: spent,
			want: decision{account: "work", reason: "pin yields: side has no room", sticky: true},
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
			name:    "a session tried on every account with room goes out on no other",
			current: on("work", 0),
			tried:   hitLimit("work"),
			work:    soon, side: spent,
			want: decision{reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name:    "a session refused on every account with room goes out on none, the client's included",
			current: on("side", 0),
			tried:   []Attempt{{Account: "side", Why: "was refused"}},
			work:    spent, side: soon,
			want: decision{reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name:    "a session whose thinking is bound stays while its cache is warm",
			bound:   true,
			current: on("work", 59*time.Minute),
			work:    later, side: soon,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "a session whose thinking is bound stays once its cache is cold, as the thinking is bound there",
			bound:   true,
			current: on("work", time.Hour+time.Second),
			work:    later, side: soon,
			want: decision{account: "work", reason: "bound", sticky: true},
		},
		{
			name:    "a session whose thinking is bound stays against an account well ahead, however long idle",
			bound:   true,
			current: on("work", 3*24*time.Hour),
			work:    even, side: wellAhead,
			want: decision{account: "work", reason: "bound", sticky: true},
		},
		{
			name:    "a session whose thinking is bound stays on an account nothing's been read of",
			bound:   true,
			current: on("work", 2*time.Hour),
			work:    unread, side: soon,
			want: decision{account: "work", reason: "bound", sticky: true},
		},
		{
			name:    "a session whose thinking is bound moves, idle, when its account has no room",
			bound:   true,
			current: on("work", 2*time.Hour),
			work:    spent, side: later,
			want: decision{account: "side", reason: "moved: work has no room", afresh: true},
		},
		{
			name:    "a session whose thinking is bound moves when its account hit its limit on the request",
			bound:   true,
			current: on("work", 0),
			tried:   hitLimit("work"),
			work:    soon, side: later,
			want: decision{account: "side", reason: "moved: work hit its limit", afresh: true},
		},
		{
			name:    "a session whose thinking is bound moves when its account refused the request",
			bound:   true,
			current: on("work", 2*time.Hour),
			tried:   []Attempt{{Account: "work", Why: "was refused"}},
			work:    soon, side: later,
			want: decision{account: "side", reason: "moved: work was refused", afresh: true},
		},
		{
			name:    "a session whose thinking is bound stays when no account has room",
			bound:   true,
			current: on("side", 2*time.Hour),
			work:    spent, side: spent,
			want: decision{account: "side", reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name:    "a global pin that moves sessions moves one whose thinking is bound",
			bound:   true,
			global:  moveToSide,
			current: assignedAt(on("work", 2*time.Hour), start.Add(-3*time.Hour)),
			work:    soon, side: later,
			want: decision{account: "side", reason: "moved by pin"},
		},
		{
			name:    "the global pin leaves a session whose thinking is bound where it is, however long idle",
			bound:   true,
			global:  pinSide,
			current: on("work", 2*time.Hour),
			work:    soon, side: later,
			want: decision{account: "work", reason: "bound", sticky: true},
		},
		{
			name:    "a session whose thinking is bound, having yielded its pin, isn't brought back once its cache is cold",
			bound:   true,
			pin:     "side",
			current: pinned(on("work", 2*time.Hour), "side"),
			work:    later, side: soon,
			want: decision{account: "work", reason: "bound", sticky: true},
		},
		{
			name:    "a session whose thinking is bound, having yielded its pin, goes back when its account has no room",
			bound:   true,
			pin:     "side",
			current: pinned(on("work", 2*time.Hour), "side"),
			work:    spent, side: later,
			want: decision{account: "side", reason: "pinned"},
		},
		{
			name:    "a session's pin yields to the session's account, which its thinking is bound to, however long idle",
			bound:   true,
			pin:     "side",
			current: on("work", 2*time.Hour),
			work:    later, side: spent,
			want: decision{account: "work", reason: "pin yields: side has no room", sticky: true},
		},
		{
			name:     "a session whose thinking is bound goes to a pin it's given while it runs, as the user's move",
			bound:    true,
			pin:      "side",
			pinnedAt: start.Add(-time.Minute),
			current:  pinned(on("work", 5*time.Minute), "side"),
			work:     later, side: soon,
			want: decision{account: "side", reason: "pinned"},
		},
		{
			name:  "a new session whose thinking is bound goes to the account whose quota most needs using",
			bound: true,
			work:  later, side: soon,
			want: decision{account: "side", reason: "new", afresh: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{Session: "0b5c6f2e", Model: opus, Pin: tt.pin, Client: "work", Tried: tt.tried}
			if tt.unsessioned {
				req.Session = ""
			}
			if tt.bound {
				req.Model, req.Bound = sonnet, true
			}
			s := situation{req: req, now: start, pinnedAt: tt.pinnedAt, pin: tt.global, accounts: known(tt.work, tt.side).without(req.tried())}
			if tt.current != nil {
				s.current, s.assigned = *tt.current, true
			}
			if got := decide(s); got != tt.want {
				t.Errorf("decide() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecideLeavesAnAccountsReserveToPins(t *testing.T) {
	var (
		// reserved's quota needs using first, but its session has reached
		// its reserve, a tenth of each window.
		reserved = []quota.Window{
			{Key: "5h", Utilization: 0.95, ResetsAt: start.Add(2 * time.Hour)},
			{Key: "7d", Utilization: 0.5, ResetsAt: start.Add(24 * time.Hour)},
		}
		later = weekAt(0.5, 5*24*time.Hour)
		spent = []quota.Window{
			{Key: "5h", Utilization: 1, ResetsAt: start.Add(2 * time.Hour), Status: quota.StatusRejected},
			{Key: "7d", Utilization: 0.5, ResetsAt: start.Add(72 * time.Hour)},
		}
	)
	pinWork := status.Pin{Accounts: []string{"work"}, Since: start.Add(-time.Hour)}
	moveToWork := status.Pin{Accounts: []string{"work"}, Since: start.Add(-time.Hour), Move: true}
	tests := []struct {
		name string
		// bound has the request ask for Sonnet, whose thinking is bound to
		// the account that produced it, in place of Opus.
		bound bool
		// pin is the request's own.
		pin string
		// current is the session's assignment, nil for a new session.
		current *assignment
		global  status.Pin
		// work's windows, work keeping a tenth of each back, and side's.
		work, side []quota.Window
		want       decision
	}{
		{
			name: "a new session passes over an account at its reserve",
			work: reserved, side: later,
			want: decision{account: "side", reason: "new", afresh: true},
		},
		{
			name: "a new session goes to an account short of its reserve",
			work: weekAt(0.5, 24*time.Hour), side: later,
			want: decision{account: "work", reason: "new", afresh: true},
		},
		{
			name:    "a session on an account at its reserve moves, as at a limit",
			current: on("work", 5*time.Minute),
			work:    reserved, side: later,
			want: decision{account: "side", reason: "moved: work is at its reserve", afresh: true, held: HeldByReserve},
		},
		{
			name:    "a session idle past the hour on an account at its reserve is rescored off it",
			current: on("work", 2*time.Hour),
			work:    reserved, side: later,
			want: decision{account: "side", reason: "rescored after 2h idle", afresh: true},
		},
		{
			name: "a session's pin spends its account's reserve",
			pin:  "work",
			work: reserved, side: later,
			want: decision{account: "work", reason: "pinned"},
		},
		{
			name: "a session's pin runs its account to its limit, and no further",
			pin:  "work",
			work: spent, side: later,
			want: decision{account: "side", reason: "pin yields: work has no room", afresh: true},
		},
		{
			name:   "the global pin spends its account's reserve for a new session",
			global: pinWork,
			work:   reserved, side: later,
			want: decision{account: "work", reason: "pinned (global)", afresh: true},
		},
		{
			name:    "the global pin keeps a warm session on its account at its reserve",
			global:  pinWork,
			current: on("work", 5*time.Minute),
			work:    reserved, side: later,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "a global pin that moves sessions carries one onto its account at its reserve",
			global:  moveToWork,
			current: assignedAt(on("side", 5*time.Minute), start.Add(-2*time.Hour)),
			work:    reserved, side: later,
			want: decision{account: "work", reason: "moved by pin"},
		},
		{
			name:    "a pin elsewhere leaves an account's reserve alone",
			global:  status.Pin{Accounts: []string{"side"}, Since: start.Add(-time.Hour)},
			current: on("work", 5*time.Minute),
			work:    reserved, side: later,
			want: decision{account: "side", reason: "pinned (global)", afresh: true, held: HeldByReserve},
		},
		{
			name: "with no candidate, a request passes over an account held back by its reserve alone",
			work: reserved, side: spent,
			want: decision{account: "side", reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name:    "a session whose thinking is bound moves off an account at its reserve, as at a limit",
			bound:   true,
			current: on("work", 5*time.Minute),
			work:    reserved, side: later,
			want: decision{account: "side", reason: "moved: work is at its reserve", afresh: true, held: HeldByReserve},
		},
		{
			name:    "a session whose thinking is bound moves off an account at its reserve for the reserve, not for idling",
			bound:   true,
			current: on("work", 2*time.Hour),
			work:    reserved, side: later,
			want: decision{account: "side", reason: "moved: work is at its reserve", afresh: true, held: HeldByReserve},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{Session: "0b5c6f2e", Model: opus, Pin: tt.pin, Client: "work"}
			if tt.bound {
				req.Model, req.Bound = sonnet, true
			}
			s := situation{req: req, now: start, pin: tt.global, accounts: reserving(known(tt.work, tt.side), "work", 0.1)}
			if tt.current != nil {
				s.current, s.assigned = *tt.current, true
			}
			if got := decide(s); got != tt.want {
				t.Errorf("decide() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDecideWithTheGlobalPinOnSeveralAccounts(t *testing.T) {
	var (
		// soon's week resets tomorrow, sooner's in two days, and later's in
		// five: soon's quota needs using first, then sooner's.
		soon   = weekAt(0.5, 24*time.Hour)
		sooner = weekAt(0.5, 48*time.Hour)
		later  = weekAt(0.5, 5*24*time.Hour)
		// even scores 0.01 an hour; a little ahead of it by 10%, and well
		// ahead by 25%.
		even        = weekAt(0.5, 50*time.Hour)
		littleAhead = weekAt(0.45, 50*time.Hour)
		wellAhead   = weekAt(0.375, 50*time.Hour)
		// early's session resets in an hour, late's in four, their weeks
		// near enough equal, late's a little ahead.
		early = []quota.Window{
			{Key: "5h", Utilization: 0.1, ResetsAt: start.Add(time.Hour)},
			{Key: "7d", Utilization: 0.5, ResetsAt: start.Add(50 * time.Hour)},
		}
		late = []quota.Window{
			{Key: "5h", Utilization: 0.1, ResetsAt: start.Add(4 * time.Hour)},
			{Key: "7d", Utilization: 0.45, ResetsAt: start.Add(50 * time.Hour)},
		}
		// reserved's session has reached a reserve of a tenth, where its
		// account keeps one.
		reserved = []quota.Window{
			{Key: "5h", Utilization: 0.95, ResetsAt: start.Add(2 * time.Hour)},
			{Key: "7d", Utilization: 0.5, ResetsAt: start.Add(24 * time.Hour)},
		}
		spent = []quota.Window{
			{Key: "5h", Utilization: 1, ResetsAt: start.Add(2 * time.Hour), Status: quota.StatusRejected},
			{Key: "7d", Utilization: 0.5, ResetsAt: start.Add(72 * time.Hour)},
		}
		// unread is an account nothing has been read of.
		unread []quota.Window
	)
	// The pin names side and spare, not work, since an hour before start.
	pinned := status.Pin{Accounts: []string{"side", "spare"}, Since: start.Add(-time.Hour)}
	moving := status.Pin{Accounts: []string{"side", "spare"}, Since: start.Add(-time.Hour), Move: true}
	tests := []struct {
		name string
		// unsessioned leaves the request without a session.
		unsessioned bool
		// current is the session's assignment, nil for a new session.
		current *assignment
		global  status.Pin
		// work's, side's and spare's windows.
		work, side, spare []quota.Window
		// reserving is the account keeping a tenth of every window back, if
		// any.
		reserving string
		want      decision
	}{
		{
			name:   "a new session goes to the pinned account whose quota most needs using, passing over the one unpinned",
			global: pinned,
			work:   soon, side: later, spare: sooner,
			want: decision{account: "spare", reason: "pinned (global)", afresh: true},
		},
		{
			name:        "a request without a session goes to the best pinned account",
			unsessioned: true,
			global:      pinned,
			work:        soon, side: sooner, spare: later,
			want: decision{account: "side", reason: "pinned (global)", afresh: true},
		},
		{
			name:   "between pinned accounts near enough equal, the one whose session resets soonest",
			global: pinned,
			work:   soon, side: early, spare: late,
			want: decision{account: "side", reason: "pinned (global)", afresh: true},
		},
		{
			name:      "a pinned account at its reserve takes the request, its reserve spent",
			global:    pinned,
			reserving: "side",
			work:      later, side: reserved, spare: later,
			want: decision{account: "side", reason: "pinned (global)", afresh: true},
		},
		{
			name:   "a pinned account with room takes the request, the other pinned without",
			global: pinned,
			work:   soon, side: spent, spare: later,
			want: decision{account: "spare", reason: "pinned (global)", afresh: true},
		},
		{
			name:   "the pin yields to every account when none pinned has room",
			global: pinned,
			work:   later, side: spent, spare: spent,
			want: decision{account: "work", reason: "new", afresh: true},
		},
		{
			name:      "the pin yields, an unpinned account at its reserve held back by it",
			global:    pinned,
			reserving: "work",
			work:      reserved, side: spent, spare: spent,
			want: decision{account: "side", reason: "no account has room", afresh: true, noRoom: true},
		},
		{
			name:   "a pinned account that can be scored comes before one nothing has been read of",
			global: pinned,
			work:   soon, side: unread, spare: later,
			want: decision{account: "spare", reason: "pinned (global)", afresh: true},
		},
		{
			name:   "the first pinned account takes the request when none pinned can be scored",
			global: pinned,
			work:   soon, side: unread, spare: unread,
			want: decision{account: "side", reason: "pinned (global)", afresh: true},
		},
		{
			name:    "the pin leaves a warm session where it is",
			global:  pinned,
			current: on("work", 5*time.Minute),
			work:    later, side: soon, spare: soon,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "the pin takes a session whose account has no room",
			global:  pinned,
			current: on("work", 5*time.Minute),
			work:    spent, side: later, spare: sooner,
			want: decision{account: "spare", reason: "pinned (global)", afresh: true},
		},
		{
			name:    "a session idle past the hour on a pinned account keeps to it against another pinned a little ahead",
			global:  pinned,
			current: on("side", 2*time.Hour),
			work:    soon, side: even, spare: littleAhead,
			want: decision{account: "side", reason: "pinned (global)", afresh: true},
		},
		{
			name:    "a session idle past the hour on a pinned account leaves it for another pinned well ahead",
			global:  pinned,
			current: on("side", 2*time.Hour),
			work:    soon, side: even, spare: wellAhead,
			want: decision{account: "spare", reason: "pinned (global)", afresh: true},
		},
		{
			name:    "a pin that moves sessions moves one on an account it doesn't name to the best it names",
			global:  moving,
			current: on("work", 5*time.Minute),
			work:    soon, side: later, spare: sooner,
			want: decision{account: "spare", reason: "moved by pin"},
		},
		{
			name:    "a pin that moves sessions leaves one on an account it names",
			global:  moving,
			current: on("side", 5*time.Minute),
			work:    soon, side: later, spare: sooner,
			want: decision{account: "side", reason: "sticky", sticky: true},
		},
		{
			name:    "a pin that moves sessions moves each once",
			global:  moving,
			current: assignedAt(on("work", 5*time.Minute), start.Add(-30*time.Minute)),
			work:    soon, side: later, spare: sooner,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:    "a pin that moves sessions waits while none it names has room",
			global:  moving,
			current: on("work", 5*time.Minute),
			work:    later, side: spent, spare: spent,
			want: decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:      "a pin that moves sessions carries one onto a pinned account at its reserve",
			global:    moving,
			current:   on("work", 5*time.Minute),
			reserving: "side",
			work:      soon, side: reserved, spare: spent,
			want: decision{account: "side", reason: "moved by pin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{Session: "0b5c6f2e", Model: opus, Client: "work"}
			if tt.unsessioned {
				req.Session = ""
			}
			accounts := view{
				policy: testPolicy,
				now:    start,
				candidates: []score.Candidate{
					{ID: "work", Windows: tt.work},
					{ID: "side", Windows: tt.side},
					{ID: "spare", Windows: tt.spare},
				},
				applies: testPolicy.IsShared,
			}
			s := situation{req: req, now: start, pin: tt.global, accounts: reserving(accounts, tt.reserving, 0.1)}
			if tt.current != nil {
				s.current, s.assigned = *tt.current, true
			}
			if got := decide(s); got != tt.want {
				t.Errorf("decide() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestChoosingAfreshBetweenNearEqualsFavoursTheSessionResettingSoonest(t *testing.T) {
	// Spare's week scores a tenth higher than side's, near enough equal, but
	// side's session resets first: in an hour, where spare's resets in four.
	// Work's week is all but spent.
	windows := func(week float64, sessionLeft time.Duration) []quota.Window {
		return []quota.Window{
			{Key: "5h", Utilization: 0.1, ResetsAt: start.Add(sessionLeft)},
			{Key: "7d", Utilization: week, ResetsAt: start.Add(50 * time.Hour)},
		}
	}
	accounts := view{
		policy: testPolicy,
		now:    start,
		candidates: []score.Candidate{
			{ID: "work", Windows: windows(0.95, 2*time.Hour)},
			{ID: "side", Windows: windows(0.5, time.Hour)},
			{ID: "spare", Windows: windows(0.45, 4*time.Hour)},
		},
		applies: testPolicy.IsShared,
	}
	tests := []struct {
		name string
		// unsessioned leaves the request without a session.
		unsessioned bool
		// current is the session's assignment, nil for a new session.
		current *assignment
		// tried are the accounts the request has gone out on already.
		tried []Attempt
		want  decision
	}{
		{
			name:        "a request without a session",
			unsessioned: true,
			want:        decision{account: "side", reason: "unsessioned", afresh: true},
		},
		{
			name: "a new session",
			want: decision{account: "side", reason: "new", afresh: true},
		},
		{
			name:    "a session idle past the hour, rescored off an account well behind",
			current: on("work", 2*time.Hour),
			want:    decision{account: "side", reason: "rescored after 2h idle", afresh: true},
		},
		{
			name:    "a session idle past the hour, kept on its own account against one resetting sooner",
			current: on("spare", 2*time.Hour),
			want:    decision{account: "spare", reason: "rescored after 2h idle", afresh: true},
		},
		{
			name:    "a session whose account hit its limit on the request",
			current: on("work", 0),
			tried:   []Attempt{{Account: "work", Why: whyLimit}},
			want:    decision{account: "side", reason: "moved: work hit its limit", afresh: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{Session: "0b5c6f2e", Model: opus, Client: "work", Tried: tt.tried}
			if tt.unsessioned {
				req.Session = ""
			}
			s := situation{req: req, now: start, accounts: accounts.without(req.tried())}
			if tt.current != nil {
				s.current, s.assigned = *tt.current, true
			}
			if got := decide(s); got != tt.want {
				t.Errorf("decide() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWithNoRoomButInTheReservesARequestGoesNowhere(t *testing.T) {
	session := func(used float64, resets time.Duration) quota.Window {
		return quota.Window{Key: "5h", Utilization: used, ResetsAt: start.Add(resets)}
	}
	week := quota.Window{Key: "7d", Utilization: 0.5, ResetsAt: start.Add(72 * time.Hour)}
	spent := []quota.Window{{Key: "5h", Utilization: 1, ResetsAt: start.Add(time.Hour), Status: quota.StatusRejected}, week}
	tests := []struct {
		name string
		// work's and side's windows, each keeping a tenth of every window
		// back, and spare's, which keeps none.
		work, side, spare []quota.Window
		// refused are the accounts that refused the request lately.
		refused []string
		want    decision
	}{
		{
			name:    "back at the soonest of the reserves' resets",
			work:    []quota.Window{session(0.95, 3*time.Hour), week},
			side:    []quota.Window{session(0.91, 2*time.Hour), week},
			spare:   spent,
			refused: []string{"spare"},
			want:    decision{reason: reasonNoRoom, afresh: true, noRoom: true, reserved: true, back: start.Add(2 * time.Hour)},
		},
		{
			name:    "back when its every window at its reserve has reset",
			work:    []quota.Window{session(0.95, 3*time.Hour), {Key: "7d", Utilization: 0.97, ResetsAt: start.Add(30 * time.Hour)}},
			side:    spent,
			spare:   spent,
			refused: []string{"side", "spare"},
			want:    decision{reason: reasonNoRoom, afresh: true, noRoom: true, reserved: true, back: start.Add(30 * time.Hour)},
		},
		{
			name:    "back at a time unknown",
			work:    []quota.Window{{Key: "5h", Utilization: 0.95}, week},
			side:    spent,
			spare:   spent,
			refused: []string{"side", "spare"},
			want:    decision{reason: reasonNoRoom, afresh: true, noRoom: true, reserved: true},
		},
		{
			name:  "to an account at its limit, for the upstream to say why",
			work:  []quota.Window{session(0.95, 3*time.Hour), week},
			side:  []quota.Window{session(0.95, 2*time.Hour), week},
			spare: spent,
			want:  decision{account: "spare", reason: reasonNoRoom, afresh: true, noRoom: true},
		},
		{
			name:    "to the client's, with every one refused",
			work:    spent,
			side:    spent,
			spare:   spent,
			refused: []string{"work", "side", "spare"},
			want:    decision{account: "work", reason: reasonNoRoom, afresh: true, noRoom: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := situation{
				req: Request{Session: "0b5c6f2e", Model: opus, Client: "work"},
				now: start,
				accounts: view{
					policy: testPolicy,
					now:    start,
					candidates: []score.Candidate{
						{ID: "work", Windows: tt.work, Reserve: 0.1},
						{ID: "side", Windows: tt.side, Reserve: 0.1},
						{ID: "spare", Windows: tt.spare},
					},
					applies: testPolicy.IsShared,
					barred:  tt.refused,
					refused: tt.refused,
				},
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
		// held is what the session's move off its account says held its
		// request back there.
		held Hold
	}{
		{name: "the session's account", current: "side", want: "side"},
		{name: "the client's, for a new session", want: "work"},
		{name: "the client's, when the session's refused the request", current: "side", refused: []string{"side"}, want: "work", held: HeldByRefusal},
		{name: "another, when the client's refused it too", current: "side", refused: []string{"side", "work"}, want: "spare", held: HeldByRefusal},
		{name: "another, for a new session whose client's refused it", refused: []string{"work"}, want: "side"},
		{name: "the client's, when every one refused it", current: "side", refused: []string{"work", "side", "spare"}, want: "work", held: HeldByRefusal},
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
			want := decision{account: tt.want, reason: reasonNoRoom, afresh: true, noRoom: true, held: tt.held}
			if got := decide(s); got != want {
				t.Errorf("decide() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestDecidePassesOverAccountsUnderPressure(t *testing.T) {
	// Each account's session is a tenth used and resets in 3 hours: at half
	// of it an hour, it runs out before then. Work's week resets tomorrow,
	// side's in two days and spare's in five, so work's quota needs using
	// first, then side's.
	windows := map[string][]quota.Window{
		"work":  weekAt(0.5, 24*time.Hour),
		"side":  weekAt(0.5, 48*time.Hour),
		"spare": weekAt(0.5, 5*24*time.Hour),
	}
	const pressing = 0.5
	// atReserve runs a session out at a reserve of a tenth before it resets,
	// but at its limit after.
	const atReserve = 0.28
	spent := []quota.Window{
		{Key: "5h", Utilization: 1, ResetsAt: start.Add(2 * time.Hour), Status: quota.StatusRejected},
		{Key: "7d", Utilization: 0.5, ResetsAt: start.Add(72 * time.Hour)},
	}
	workAndSpare := status.Pin{Accounts: []string{"work", "spare"}, Since: start.Add(-time.Hour)}
	tests := []struct {
		name string
		// unsessioned leaves the request without a session.
		unsessioned bool
		// pin is the session's own, launched with it.
		pin string
		// current is the session's assignment, nil for a new session.
		current *assignment
		global  status.Pin
		// rates are how fast each account's session is being used, an hour.
		rates map[string]float64
		// spent is the account whose session is used up, if any.
		spent string
		// reserving is the account keeping a tenth of every window back, if
		// any.
		reserving string
		want      decision
	}{
		{
			name:  "a new session passes over the best under pressure",
			rates: map[string]float64{"work": pressing},
			want:  decision{account: "side", reason: "new, work under pressure", afresh: true, passedOver: "work"},
		},
		{
			name:  "a new session goes to the best while another is under pressure",
			rates: map[string]float64{"side": pressing},
			want:  decision{account: "work", reason: "new", afresh: true},
		},
		{
			name:  "a new session goes to the best while every account is under pressure",
			rates: map[string]float64{"work": pressing, "side": pressing, "spare": pressing},
			want:  decision{account: "work", reason: "new", afresh: true},
		},
		{
			name:        "a request without a session passes over the best under pressure",
			unsessioned: true,
			rates:       map[string]float64{"work": pressing},
			want:        decision{account: "side", reason: "unsessioned, work under pressure", afresh: true, passedOver: "work"},
		},
		{
			name:    "a session idle past the hour passes over its own account under pressure",
			current: on("work", 2*time.Hour),
			rates:   map[string]float64{"work": pressing},
			want:    decision{account: "side", reason: "rescored after 2h idle, work under pressure", afresh: true, passedOver: "work"},
		},
		{
			name:    "a session whose account has no room passes over the next under pressure",
			current: on("work", 5*time.Minute),
			spent:   "work",
			rates:   map[string]float64{"side": pressing},
			want:    decision{account: "spare", reason: "moved: work has no room, side under pressure", afresh: true, passedOver: "side"},
		},
		{
			name:    "a session stays on its account under pressure while its cache is warm",
			current: on("work", 5*time.Minute),
			rates:   map[string]float64{"work": pressing},
			want:    decision{account: "work", reason: "sticky", sticky: true},
		},
		{
			name:  "a session's own pin holds on its account under pressure",
			pin:   "work",
			rates: map[string]float64{"work": pressing},
			want:  decision{account: "work", reason: "pinned"},
		},
		{
			name:  "a session's pin that yields passes over the best under pressure",
			pin:   "side",
			spent: "side",
			rates: map[string]float64{"work": pressing},
			want:  decision{account: "spare", reason: "pin yields: side has no room, work under pressure", afresh: true, passedOver: "work"},
		},
		{
			name:   "the global pin's best under pressure passes to the pin's next, not beyond the pin",
			global: workAndSpare,
			rates:  map[string]float64{"work": pressing},
			want:   decision{account: "spare", reason: "pinned (global), work under pressure", afresh: true, passedOver: "work"},
		},
		{
			name:   "the global pin's accounts every one under pressure keep to the pin",
			global: workAndSpare,
			rates:  map[string]float64{"work": pressing, "spare": pressing},
			want:   decision{account: "work", reason: "pinned (global)", afresh: true},
		},
		{
			name:      "an account runs out at its reserve under pressure",
			reserving: "work",
			rates:     map[string]float64{"work": atReserve},
			want:      decision{account: "side", reason: "new, work under pressure", afresh: true, passedOver: "work"},
		},
		{
			name:      "an account the global pin names runs out at its limit, its reserve spent",
			global:    workAndSpare,
			reserving: "work",
			rates:     map[string]float64{"work": atReserve},
			want:      decision{account: "work", reason: "pinned (global)", afresh: true},
		},
		{
			name:    "a session the global pin moves passes over the pin's best under pressure",
			current: on("side", 5*time.Minute),
			global:  status.Pin{Accounts: []string{"work", "spare"}, Since: start.Add(-time.Hour), Move: true},
			rates:   map[string]float64{"work": pressing},
			want:    decision{account: "spare", reason: "moved by pin, work under pressure", passedOver: "work"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := Request{Session: "0b5c6f2e", Model: opus, Pin: tt.pin, Client: "work"}
			if tt.unsessioned {
				req.Session = ""
			}
			accounts := view{policy: testPolicy, now: start, applies: testPolicy.IsShared}
			for _, id := range []string{"work", "side", "spare"} {
				c := score.Candidate{ID: id, Windows: windows[id], Rate: tt.rates[id]}
				if id == tt.spent {
					c.Windows = spent
				}
				accounts.candidates = append(accounts.candidates, c)
			}
			s := situation{req: req, now: start, pin: tt.global, accounts: reserving(accounts, tt.reserving, 0.1)}
			if tt.current != nil {
				s.current, s.assigned = *tt.current, true
			}
			got := decide(s)
			// How the account passed over stood is the log's to tell, and its
			// test's; here, that the choice judged it under pressure, at its
			// rate.
			if passed := tt.want.passedOver != ""; got.pressure.Under != passed || passed && got.pressure.Rate != tt.rates[tt.want.passedOver] {
				t.Errorf("decide() judged %s as %+v, want under pressure: %v", tt.want.passedOver, got.pressure, passed)
			}
			if got.pressure = (score.Pressure{}); got != tt.want {
				t.Errorf("decide() = %+v, want %+v", got, tt.want)
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

// reserving is v with the account whose id is given keeping reserve of every
// window back.
func reserving(v view, id string, reserve float64) view {
	v.candidates = slices.Clone(v.candidates)
	for i := range v.candidates {
		if v.candidates[i].ID == id {
			v.candidates[i].Reserve = reserve
		}
	}
	return v
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
