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
	readRefused := refusedRead(quota.TokenRefused)
	ended := refusedRead(quota.SubscriptionEnded)
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
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleFailed, Used: 0.97, At: read},
		},
		{
			name:    "its last read failed, at its cap: as last read, before the cap",
			account: with(with(readNow(0.95, 0.3, 0), capped), failing),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleFailed, Used: 0.95, At: read},
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
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleRefused, Used: 0.97, At: read},
		},
		{
			name:    "a stale week that has reset since it was read: stale, and empty, as it runs",
			account: with(readNow(0.2, 0.6, 0), func(a *status.Account) { failing(a); a.Windows[1].ResetsAt = at(1, 9, 0).UTC() }),
			key:     "7d",
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleFailed, At: read},
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
			name:    "a limit naming a model's own week, which holds back that model's requests alone: not this one",
			account: with(readNow(0.2, 0.3, 1), limited("7d_oi")),
			key:     "7d",
			want:    status.WindowNow{Kind: status.WindowOpen, Used: 0.3, At: at(5, 21, 0).UTC()},
		},
		{
			name:    "a limit naming the session, which holds back every request, lifting before the week resets: the week its own",
			account: with(readNow(0.2, 0.3, 0.4), limited("5h")),
			key:     "7d",
			want:    status.WindowNow{Kind: status.WindowOpen, Used: 0.3, At: at(5, 21, 0).UTC()},
		},
		{
			name:    "a limit naming the session till just as the week resets: the week the account's limit",
			account: limitedTo(readNow(1, 0.3, 0.4), "5h"),
			key:     "7d",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.3, At: at(5, 21, 0).UTC()},
		},
		{
			name:    "the week at its cap, which holds back every request: the account's cap, back at the week's reset",
			account: with(readNow(0.2, 0.97, 0.4), capped),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowCapped, Used: 0.2, At: at(5, 21, 0).UTC()},
		},
		{
			name:    "the week at its cap, a limit naming the session lifting first: the session the limit, as its own",
			account: with(with(readNow(0.2, 0.97, 0.4), capped), limited("5h")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.2, At: at(1, 15, 54).UTC()},
		},
		{
			name:    "the week at its cap, a limit naming the session lifting first: the account's cap, the later",
			account: with(with(readNow(0.2, 0.97, 0.4), capped), limited("5h")),
			key:     "7d_oi",
			want:    status.WindowNow{Kind: status.WindowCapped, Used: 0.4, At: at(5, 21, 0).UTC()},
		},
		{
			name:    "the session at its cap, the week read spent, lifting later: the account's limit",
			account: with(readNow(0.97, 1, 0.4), capped),
			key:     "7d_oi",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.4, At: at(5, 21, 0).UTC()},
		},
		{
			name:    "a model's own week at its cap, the session read spent: its own cap, not the account's limit",
			account: with(readNow(1, 0.3, 0.97), capped),
			key:     "7d_oi",
			want:    status.WindowNow{Kind: status.WindowCapped, Used: 0.97, At: at(5, 21, 0).UTC()},
		},
		{
			name: "the session not started, the week at its cap: not started, before the account's cap",
			account: with(with(readNow(0, 0.97, 0), capped), func(a *status.Account) {
				a.Windows[0] = quota.Window{Key: "5h", Label: "Session"}
				a.Lapsed = []string{"5h"}
			}),
			key:  "5h",
			want: status.WindowNow{Kind: status.WindowNotStarted},
		},
		{
			name: "the session not started, the week read spent: not started, as its next reset isn't known",
			account: with(readNow(0, 1, 0), func(a *status.Account) {
				a.Windows[0] = quota.Window{Key: "5h", Label: "Session"}
				a.Lapsed = []string{"5h"}
			}),
			key:  "5h",
			want: status.WindowNow{Kind: status.WindowNotStarted},
		},
		{
			name: "the session not started, the week read spent, its read refused, so the hold's end not known: not started, as it offers no room",
			account: with(with(readNow(0, 1, 0), readRefused), func(a *status.Account) {
				a.Windows[0] = quota.Window{Key: "5h", Label: "Session"}
				a.Lapsed = []string{"5h"}
			}),
			key:  "5h",
			want: status.WindowNow{Kind: status.WindowNotStarted},
		},
		{
			name:    "its token refused, a limit naming the week till after the session resets: stale, as it says why, not the account's limit",
			account: with(with(readNow(0.4, 0.9, 0), refused), limited("7d")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleRefused, Used: 0.4, At: read},
		},
		{
			name:    "its last read refusing its token, a limit naming the week: stale, as it says why, not the account's limit",
			account: with(with(readNow(0.4, 0.9, 0), readRefused), limited("7d")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleRefused, Used: 0.4, At: read},
		},
		{
			name:    "its subscription ended, the week read spent: stale, as it says why, not the account's limit",
			account: with(readNow(0.4, 1, 0.3), ended),
			key:     "7d_oi",
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleLapsed, Used: 0.3, At: read},
		},
		{
			name:    "its last read failed, the week read spent: stale, as last read, not the account's limit",
			account: with(readNow(0.4, 1, 0.3), failing),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleFailed, Used: 0.4, At: read},
		},
		{
			name: "the week at its cap, its reset not known, a limit naming the session: the account's cap, lifting last at no known time",
			account: with(with(with(readNow(0.2, 0.97, 0.4), capped), limited("5h")), func(a *status.Account) {
				a.Windows[1].ResetsAt = time.Time{}
			}),
			key:  "7d_oi",
			want: status.WindowNow{Kind: status.WindowCapped, Used: 0.4},
		},
		{
			name: "the session read spent, its reset not known, the week at its cap: the account's limit, lifting last at no known time",
			account: with(with(readNow(1, 0.97, 0.4), capped), func(a *status.Account) {
				a.Windows[0].ResetsAt = time.Time{}
			}),
			key:  "7d_oi",
			want: status.WindowNow{Kind: status.WindowLimited, Used: 0.4},
		},
		{
			name:    "its token refused, the week at its cap: stale, before the account's cap",
			account: with(with(readNow(0.4, 0.97, 0), capped), refused),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleRefused, Used: 0.4, At: read},
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
			want:    status.WindowNow{Kind: status.WindowStale, Stale: status.StaleRefused, At: read},
		},
		{
			name:    "the session reset since it was read, its last read refusing its token: lapsed, not started, before the failed read",
			account: with(with(readNow(0.97, 0.3, 0), resetSince), readRefused),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowNotStarted},
		},
		{
			name:    "the session reset since it was read, its subscription ended: lapsed, not started, before the failed read",
			account: with(with(readNow(0.97, 0.3, 0), resetSince), ended),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowNotStarted},
		},
		{
			name:    "its token refused, the router's limit naming the window: the limit, before the refusal, back as the later refusal ends",
			account: with(with(readNow(0.9, 0.3, 0), refused), limited("5h")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.9, At: at(1, 21, 40).UTC()},
		},
		{
			name: "its token refused till before the router's limit naming the window lifts: back as the limit lifts",
			account: with(with(readNow(0.9, 0.3, 0), func(a *status.Account) {
				a.Refused = status.Refusal{Until: at(1, 15, 0).UTC(), Status: 401}
			}), limited("5h")),
			key:  "5h",
			want: status.WindowNow{Kind: status.WindowLimited, Used: 0.9, At: at(1, 15, 54).UTC()},
		},
		{
			name:    "its subscription ended, the router's limit naming the window: the limit, back when not known",
			account: with(with(readNow(0.9, 0.3, 0), ended), limited("5h")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.9},
		},
		{
			name:    "its last read refusing its token, the router's limit naming the window: the limit, back when not known",
			account: with(with(readNow(0.9, 0.3, 0), readRefused), limited("5h")),
			key:     "5h",
			want:    status.WindowNow{Kind: status.WindowLimited, Used: 0.9},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{GeneratedAt: stateNow.UTC(), Accounts: []status.Account{tt.account}, Pin: status.Pin{Accounts: tt.pin}}
			got := windowAt(doc, tt.account.ID, tt.key, stateNow, policy)
			if got.Kind != tt.want.Kind || got.Stale != tt.want.Stale || !near(got.Used, tt.want.Used) || !got.At.Equal(tt.want.At) {
				t.Errorf("the window stands as kind %d, stale as %d, %v used, at %v; want kind %d, stale as %d, %v used, at %v",
					got.Kind, got.Stale, got.Used, got.At, tt.want.Kind, tt.want.Stale, tt.want.Used, tt.want.At)
			}
		})
	}
}

// The errors of a probe the API refused, as the probe gives them: an ended
// subscription's, the API's message cut to the 60 characters it keeps, and a
// refused token's.
const (
	endedSubscription = "HTTP 403 · OAuth authentication is currently not allowed for this organ"
	tokenRefused      = "HTTP 401 · Invalid bearer token"
)

// An account a limit or its cap holds back from every request can take
// nothing till it's back: a window it holds back till at least its next
// reset, with room by its reading, offers no allowance, and reads what holds
// the account, back as it is, held with it; a model's own it hasn't used is
// unused all the same, as the hold says nothing of its use. A
// hold that lifts before the window resets leaves the window its own words
// and allowance, as it can be used again once the hold lifts; one lifting
// just as it resets holds it. A limit on the Fable week alone holds back that
// model's requests, so leaves the rest theirs.
func TestAnAccountHeldBackFromEveryRequestOffersNoAllowanceInAWindowItOutlasts(t *testing.T) {
	sessionLimited := func(till time.Time) func(*status.Account) {
		return func(a *status.Account) { a.Limit = status.Limit{Windows: []string{"5h"}, Until: till.UTC()} }
	}
	tests := []struct {
		name    string
		account status.Account
		key     string
		kind    status.WindowKind
		at      time.Time
		held    bool
		unused  bool
	}{
		{
			name: "its week at its limit: the session reads the limit", key: "5h",
			account: limitedTo(readNow(0.3, 1, 0.4), "7d"), kind: status.WindowLimited, at: at(5, 21, 0), held: true,
		},
		{
			name: "its week at its limit, lifting as the Fable week resets: the Fable week reads the limit", key: "7d_oi",
			account: limitedTo(readNow(0.3, 1, 0.4), "7d"), kind: status.WindowLimited, at: at(5, 21, 0), held: true,
		},
		{
			name: "its week at its limit, the Fable week unused: held with it, and unused", key: "7d_oi",
			account: limitedTo(readNow(0.3, 1, 0), "7d"), kind: status.WindowLimited, at: at(5, 21, 0), held: true, unused: true,
		},
		{
			name: "its week at its cap, the Fable week unused: held with it, and unused", key: "7d_oi",
			account: with(readNow(0.3, 0.97, 0), reservingAWentieth), kind: status.WindowCapped, at: at(5, 21, 0), held: true, unused: true,
		},
		{
			name: "its week at its cap: the session reads the cap", key: "5h",
			account: with(readNow(0.3, 0.97, 0.4), reservingAWentieth), kind: status.WindowCapped, at: at(5, 21, 0), held: true,
		},
		{
			name: "its week at its cap, back as the Fable week resets: the Fable week reads the cap", key: "7d_oi",
			account: with(readNow(0.3, 0.97, 0.4), reservingAWentieth), kind: status.WindowCapped, at: at(5, 21, 0), held: true,
		},
		{
			name: "its session at its limit till just as the week resets: the week reads the limit", key: "7d",
			account: with(readNow(1, 0.3, 0.4), sessionLimited(at(5, 21, 0))), kind: status.WindowLimited, at: at(5, 21, 0), held: true,
		},
		{
			name: "its session at its limit till 15:54, before the week resets: the week its own, with its allowance", key: "7d",
			account: with(readNow(0.9, 0.3, 0.4), sessionLimited(at(1, 15, 54))), kind: status.WindowOpen, at: at(5, 21, 0),
		},
		{
			name: "its session at its cap till 17:10, before the week resets: the week its own, with its allowance", key: "7d",
			account: with(readNow(0.97, 0.3, 0.4), reservingAWentieth), kind: status.WindowOpen, at: at(5, 21, 0),
		},
		{
			name: "its Fable week at its limit alone: the session its own, with its allowance", key: "5h",
			account: limitedTo(readNow(0.3, 0.4, 1), "7d_oi"), kind: status.WindowOpen, at: at(1, 17, 10),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{GeneratedAt: stateNow.UTC(), Accounts: []status.Account{tt.account}}
			got := windowAt(doc, tt.account.ID, tt.key, stateNow, policy)
			if got.Kind != tt.kind || !got.At.Equal(tt.at) || got.WithAccount != tt.held || got.Unused != tt.unused {
				t.Errorf("it stands as kind %d at %v, held with its account %v, unused %v; want kind %d at %v, held with it %v, unused %v",
					got.Kind, got.At, got.WithAccount, got.Unused, tt.kind, tt.at, tt.held, tt.unused)
			}
			worked, _ := doc.WorkedOut(policy).Accounts[0].Window(tt.key)
			for _, allowance := range []quota.Allowance{got.Allowance, worked.Allowance} {
				if offered := allowance != (quota.Allowance{}); offered == tt.held {
					t.Errorf("on the moment and worked out, as status --json gives it, its allowances are %+v and %+v; want them offered %v", got.Allowance, worked.Allowance, !tt.held)
				}
			}
		})
	}
}

// limitedTo is the account a, the router's limit naming the windows with the
// given keys till each resets.
func limitedTo(a status.Account, keys ...string) status.Account {
	return with(a, func(a *status.Account) { a.Limit = status.Limit{Windows: keys, Until: at(5, 21, 0).UTC()} })
}

// reservingAWentieth has the account keep a twentieth of each window back.
func reservingAWentieth(a *status.Account) {
	a.Reserve = 0.05
}

// A stale reading says why it's stale, the first that holds of: its
// subscription ended, as the account's refused read says its last read was
// refused its organization's login; its token refused, by the router or by
// its last read refused; and its last read failed otherwise. The error's
// text never says it, whatever it reads, nor a refusal this build doesn't
// know. A refusal that has lifted, the account read clean since, leaves none.
func TestAStaleReadingSaysWhyItsStale(t *testing.T) {
	refused := status.Refusal{Until: at(1, 21, 40).UTC(), Status: 401}
	tests := []struct {
		name        string
		error       string
		refusedRead quota.ReadRefusal
		refused     status.Refusal
		wantKind    status.WindowKind
		wantStale   status.StaleCause
	}{
		{name: "its subscription ended: subscription lapsed", error: endedSubscription, refusedRead: quota.SubscriptionEnded, wantKind: status.WindowStale, wantStale: status.StaleLapsed},
		{
			name: "its subscription ended, its token refused too: subscription lapsed", error: endedSubscription, refusedRead: quota.SubscriptionEnded, refused: refused,
			wantKind: status.WindowStale, wantStale: status.StaleLapsed,
		},
		{name: "its last read's token refused: its token refused", error: tokenRefused, refusedRead: quota.TokenRefused, wantKind: status.WindowStale, wantStale: status.StaleRefused},
		{name: "the router seeing its token refused: its token refused", refused: refused, wantKind: status.WindowStale, wantStale: status.StaleRefused},
		{name: "an error reading as an ended subscription's, no refusal said: failed", error: endedSubscription, wantKind: status.WindowStale, wantStale: status.StaleFailed},
		{name: "an error reading as a refused token's, no refusal said: failed", error: tokenRefused, wantKind: status.WindowStale, wantStale: status.StaleFailed},
		{
			name: "a refusal this build doesn't know: failed", error: "HTTP 403 · Something new", refusedRead: quota.ReadRefusal("something_new"),
			wantKind: status.WindowStale, wantStale: status.StaleFailed,
		},
		{name: "a read answered otherwise: failed", error: "HTTP 529 · Overloaded", wantKind: status.WindowStale, wantStale: status.StaleFailed},
		{name: "a read not answered: failed", error: "timed out after 5s", wantKind: status.WindowStale, wantStale: status.StaleFailed},
		{
			name: "its token refused, the refusal lifted and read clean since: open, not stale", refused: status.Refusal{Until: stateNow.Add(-time.Minute).UTC(), Status: 401},
			wantKind: status.WindowOpen,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := with(readNow(0.2, 0.3, 0), func(a *status.Account) { a.Error, a.RefusedRead, a.Refused = tt.error, tt.refusedRead, tt.refused })
			doc := status.Document{GeneratedAt: stateNow.UTC(), Accounts: []status.Account{a}}
			if got := windowAt(doc, a.ID, "5h", stateNow, policy); got.Kind != tt.wantKind || got.Stale != tt.wantStale {
				t.Errorf("the window stands as kind %d, stale as %d; want kind %d, stale as %d", got.Kind, got.Stale, tt.wantKind, tt.wantStale)
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
