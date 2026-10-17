package status_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

func TestAWindowStandsAsTheFirstOfItsStatesThatHolds(t *testing.T) {
	limited := func(windows ...string) func(*status.Account) {
		return func(a *status.Account) { a.Limit = status.Limit{Windows: windows, Until: at(1, 15, 54).UTC()} }
	}
	capped := func(a *status.Account) { a.Reserve = 0.05 }
	failing := func(a *status.Account) { a.Error = "HTTP 529 · Overloaded" }
	refused := func(a *status.Account) { a.Refused = status.Refusal{Until: at(1, 21, 40).UTC(), Status: 401} }
	read := stateNow.Add(-4 * time.Second).UTC()
	tests := []struct {
		name    string
		account status.Account
		key     string
		pin     []string
		want    status.WindowNow
	}{
		{
			name:    "no usable token, before all else",
			account: with(with(readNow(1, 0.3, 0), limited("5h")), func(a *status.Account) { a.TokenSet = false }),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowTokenless},
		},
		{
			name:    "nothing read of the account",
			account: with(status.Account{ID: "work", TokenSet: true}, limited("5h")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowUnread},
		},
		{
			name:    "a window the account hasn't reported",
			account: with(readNow(0.2, 0.3, 0), func(a *status.Account) { a.Windows = a.Windows[:2] }),
			key:     "7d_oi",
			want:    status.WindowNow{Kind: status.WindowUnread},
		},
		{
			name:    "its last read failed: as last read",
			account: with(readNow(0.97, 0.3, 0), failing),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowStale, Used: 0.97, At: read},
		},
		{
			name:    "its last read failed, at its cap: as last read, before the cap",
			account: with(with(readNow(0.95, 0.3, 0), capped), failing),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowStale, Used: 0.95, At: read},
		},
		{
			name:    "its last read failed, a limit holding the window: the limit, said even where its last read failed",
			account: with(with(readNow(1, 0.3, 0), limited("5h")), failing),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 1, At: at(1, 17, 10).UTC()},
		},
		{
			name:    "its token refused: as last read",
			account: with(readNow(0.97, 0.3, 0), refused),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowStale, Used: 0.97, At: read},
		},
		{
			name:    "a stale week that has reset since it was read: stale, and empty, as it runs",
			account: with(readNow(0.2, 0.6, 0), func(a *status.Account) { failing(a); a.Windows[1].ResetsAt = at(1, 9, 0).UTC() }),
			key:     "7d",
			want:    status.WindowNow{Kind: status.WindowStale, At: read},
		},
		{
			name:    "read spent, at its cap too: the limit, back at its reset",
			account: with(readNow(1, 0.3, 0), capped),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 1, At: at(1, 17, 10).UTC()},
		},
		{
			name:    "the router's limit naming it, under its cap: back as the limit lifts",
			account: with(readNow(0.9, 0.3, 0), limited("5h")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.9, At: at(1, 15, 54).UTC()},
		},
		{
			name:    "the router's limit naming it, at its cap too: the limit",
			account: with(with(readNow(0.95, 0.3, 0), capped), limited("5h")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.95, At: at(1, 15, 54).UTC()},
		},
		{
			name:    "a limit naming no window holds every window",
			account: with(readNow(0.2, 0.3, 0), limited()),
			key:     "7d",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.3, At: at(1, 15, 54).UTC()},
		},
		{
			name:    "a limit naming another window",
			account: with(readNow(0.2, 0.3, 0), limited("5h")),
			key:     "7d",
			want:    status.WindowNow{Kind: status.WindowOpen, Used: 0.3, At: at(5, 21, 0).UTC()},
		},
		{
			name:    "at its cap, though it runs out: back at its reset",
			account: with(with(readNow(0.95, 0.3, 0), capped), pressing),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowCapped, Used: 0.95, At: at(1, 17, 10).UTC()},
		},
		{
			name:    "at its cap, which the pin spends: it runs out, at its limit",
			account: with(with(readNow(0.95, 0.3, 0), capped), pressing),
			key:     "5h",
			pin:     []string{"work"},
			want:    status.WindowNow{Kind: status.WindowRunsOut, Used: 0.95, At: at(1, 16, 5).UTC()},
		},
		{
			name:    "running out before it resets",
			account: with(readNow(0.58, 0.3, 0), pressing),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowRunsOut, Used: 0.58, At: at(1, 16, 5).UTC()},
		},
		{
			name: "lapsed, not started",
			account: with(readNow(0, 0.3, 0), func(a *status.Account) {
				a.Windows[0] = quota.Window{Key: "5h", Label: "Session"}
				a.Lapsed = []string{"5h"}
			}),
			key:  "5h",
			want: status.WindowNow{Kind: status.WindowNotStarted},
		},
		{
			name:    "open, its allowance to its reset",
			account: readNow(0.2, 0.3, 0),
			key:     "7d",
			want:    status.WindowNow{Kind: status.WindowOpen, Used: 0.3, At: at(5, 21, 0).UTC()},
		},
		{
			name:    "a week reset since it was read: empty, its allowance to its next reset",
			account: with(readNow(0.2, 0.3, 0), func(a *status.Account) { a.Windows[1].ResetsAt = at(1, 14, 0).UTC() }),
			key:     "7d",
			want:    status.WindowNow{Kind: status.WindowOpen, At: at(8, 14, 0).UTC()},
		},
		{
			name:    "the session reset since it was read, nothing noting it lapsed: lapsed at now, not started",
			account: with(readNow(0.2, 0.3, 0), resetSince),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowNotStarted},
		},
		{
			name:    "the session reset since it was read, the router's limit naming it: the limit, not lapsed",
			account: with(with(readNow(0.97, 0.3, 0), resetSince), limited("5h")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, At: at(1, 15, 54).UTC()},
		},
		{
			name:    "the session reset since it was read, its last read failed: lapsed, not started, before the failed read",
			account: with(with(readNow(0.97, 0.3, 0), resetSince), failing),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowNotStarted},
		},
		{
			name: "the session noted lapsed, its probe failed: not started, before the failed read",
			account: with(readNow(0, 0.3, 0), func(a *status.Account) {
				failing(a)
				a.Windows[0] = quota.Window{Key: "5h", Label: "Session"}
				a.Lapsed = []string{"5h"}
			}),
			key:  "5h",
			want: status.WindowNow{Kind: status.WindowNotStarted},
		},
		{
			name:    "the session reset since it was read, its token refused: stale, before not started, and empty",
			account: with(with(readNow(0.97, 0.3, 0), resetSince), refused),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowStale, At: read},
		},
		{
			name:    "its token refused, the router's limit naming the window: the limit, before the refusal",
			account: with(with(readNow(0.9, 0.3, 0), refused), limited("5h")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.9, At: at(1, 15, 54).UTC()},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{GeneratedAt: stateNow.UTC(), Accounts: []status.Account{tt.account}, Pin: status.Pin{Accounts: tt.pin}}
			got := windowAt(doc, tt.account.ID, tt.key, stateNow, policy)
			if got.Kind != tt.want.Kind || !near(got.Used, tt.want.Used) || !got.At.Equal(tt.want.At) {
				t.Errorf("the window stands as kind %d, %v used, at %v; want kind %d, %v used, at %v", got.Kind, got.Used, got.At, tt.want.Kind, tt.want.Used, tt.want.At)
			}
		})
	}
}

// A window's even pace and its allowance stand as of when the document was
// read, as its projection does, so they stay put while the document does:
// drawn a minute after, they're the document's own, as it worked them out.
func TestAWindowsPaceAndAllowanceAreAsTheDocumentWorkedThemOut(t *testing.T) {
	a := readNow(0.2, 0.3, 0.1)
	doc := status.Document{GeneratedAt: stateNow.Add(-time.Minute).UTC(), Accounts: []status.Account{a}}.WorkedOut(policy)
	for _, w := range doc.Accounts[0].Windows {
		got := windowAt(doc, a.ID, w.Key, stateNow, policy)
		if got.Pace == nil || w.Pace == nil || !near(*got.Pace, *w.Pace) {
			t.Errorf("%s's even pace is %s, want %s, the document's", w.Key, figureOf(got.Pace), figureOf(w.Pace))
		}
		if got.Kind != status.WindowOpen || got.Allowance != w.Allowance {
			t.Errorf("%s stands as kind %d, its allowance %+v; want open, its allowance %+v, the document's", w.Key, got.Kind, got.Allowance, w.Allowance)
		}
	}
}

// A window that has reset since the document was read stands in a period
// that started after then: its allowance is spread from that period's start,
// never over more than the window's length, however old the document is.
func TestTheAllowanceOfAWindowResetSinceTheDocumentWasReadSpansTheWindowAtMost(t *testing.T) {
	reset := at(1, 15, 0).UTC()
	now := reset.Add(30 * time.Second)
	// running names no window a request starts, so a session runs on past its
	// reset, as a week does, where policy has it lapse.
	running := score.Policy{Shared: policy.Shared}
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.6, ResetsAt: reset}
	week := quota.Window{Key: "7d", Label: "Week", Utilization: 0.6, ResetsAt: reset}
	tests := []struct {
		name   string
		window quota.Window
		read   time.Time
		policy score.Policy
		want   quota.Allowance
	}{
		{name: "a session read a minute before its reset", window: session, read: reset.Add(-time.Minute), policy: running, want: quota.Allowance{Share: 1.0 / 5, Per: quota.PerHour}},
		{name: "a session of a document three hours old", window: session, read: reset.Add(-3 * time.Hour), policy: running, want: quota.Allowance{Share: 1.0 / 5, Per: quota.PerHour}},
		{name: "a week read a minute before its reset", window: week, read: reset.Add(-time.Minute), policy: policy, want: quota.Allowance{Share: 1.0 / 7, Per: quota.PerDay}},
		{name: "a week of a document three hours old", window: week, read: reset.Add(-3 * time.Hour), policy: policy, want: quota.Allowance{Share: 1.0 / 7, Per: quota.PerDay}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := status.Account{ID: "work", Label: "work", TokenSet: true, FetchedAt: tt.read, Windows: []quota.Window{tt.window}}
			doc := status.Document{GeneratedAt: tt.read, Accounts: []status.Account{a}}
			got := windowAt(doc, a.ID, tt.window.Key, now, tt.policy)
			if got.Kind != status.WindowOpen || !near(got.Allowance.Share, tt.want.Share) || got.Allowance.Per != tt.want.Per {
				t.Errorf("the window stands as kind %d, its allowance %+v; want open, its allowance %+v, the whole of it over its length", got.Kind, got.Allowance, tt.want)
			}
		})
	}
}

// windowAt is how the window with the given key of doc's account with the
// given id stands at now, as policy judges it, as the moment has it.
func windowAt(doc status.Document, id, key string, now time.Time, policy score.Policy) status.WindowNow {
	a, _ := doc.At(now, policy).Account(id)
	w, _ := a.Window(key)
	return w
}

// resetSince has the account's session, read running, reset at 14:00, before
// stateNow, with nothing read since.
func resetSince(a *status.Account) {
	a.Windows[0].ResetsAt = at(1, 14, 0).UTC()
}

// near reports whether two shares are equal but for rounding.
func near(a, b float64) bool {
	return a-b < 1e-9 && b-a < 1e-9
}
