package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

func TestHowAnAccountStandsWithTheRouter(t *testing.T) {
	limited := func(windows ...string) func(*status.Account) {
		return func(a *status.Account) { a.Limit = status.Limit{Windows: windows, Until: at(1, 15, 54).UTC()} }
	}
	refusing := func(code int, family string) func(*status.Account) {
		return func(a *status.Account) {
			a.Refused = status.Refusal{Until: at(1, 21, 40).UTC(), Status: code, Family: family}
		}
	}
	failing := func(a *status.Account) { a.Error = "timed out" }
	capped := func(reserve float64) func(*status.Account) {
		return func(a *status.Account) { a.Reserve = reserve }
	}
	fableLimited := func(a *status.Account) {
		a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: at(5, 21, 0).UTC()}
	}
	tests := []struct {
		name    string
		account status.Account
		// others are the document's other accounts, best the account new
		// sessions go to, and pin the accounts the global pin names.
		others []status.Account
		best   string
		pin    []string
		prime  []status.Slot
		want   status.Routing
	}{
		{
			name:    "without a usable token",
			account: with(readNow(0.2, 0.3, 0), func(a *status.Account) { a.TokenSet, a.Error = false, "token missing" }),
			want:    status.Routing{Kind: status.RoutingTokenless, Says: "no token", Clauses: []string{"switchboard accounts token work"}, Short: []string{"no token"}},
		},
		{
			name:    "at a limit holding back every request, its last read failing: the limit",
			account: with(with(readNow(1, 0.3, 0), limited("5h")), failing),
			want:    status.Routing{Kind: status.RoutingLimited, Says: "limit reached", Clauses: []string{"back 17:10"}, Short: []string{"limit reached", "17:10"}},
		},
		{
			name:    "at a limit naming no window",
			account: with(readNow(0.6, 0.3, 0), limited()),
			want:    status.Routing{Kind: status.RoutingLimited, Says: "limit reached", Clauses: []string{"back 15:54"}, Short: []string{"limit reached", "15:54"}},
		},
		{
			name: "its session read spent with no reset known: no time to be back",
			account: with(readNow(1, 0.3, 0), func(a *status.Account) {
				a.Windows[0] = quota.Window{Key: "5h", Label: "Session", Utilization: 1, Status: quota.StatusRejected}
			}),
			want: status.Routing{Kind: status.RoutingLimited, Says: "limit reached", Short: []string{"limit reached"}},
		},
		{
			name:    "nothing read yet, but the limit the router saw it reach",
			account: with(status.Account{ID: "work", TokenSet: true}, limited("5h")),
			want:    status.Routing{Kind: status.RoutingLimited, Says: "limit reached", Clauses: []string{"back 15:54"}, Short: []string{"limit reached", "15:54"}},
		},
		{
			name:    "a limit before its token refused",
			account: with(with(readNow(0.98, 0.3, 0), limited("5h")), refusing(401, "")),
			want:    status.Routing{Kind: status.RoutingLimited, Says: "limit reached", Clauses: []string{"back 15:54"}, Short: []string{"limit reached", "15:54"}},
		},
		{
			name:    "at Fable's week's limit",
			account: with(readNow(0.2, 0.3, 1), fableLimited),
			want: status.Routing{
				Kind: status.RoutingModelLimited, Says: "Fable wk limit", Clauses: []string{"back Mon 21:00", "other models still come here"}, Short: []string{"Fable wk limit"},
			},
		},
		{
			name:    "at Fable's week's limit, its last read failing: the limit",
			account: with(with(readNow(0.2, 0.3, 1), fableLimited), failing),
			want: status.Routing{
				Kind: status.RoutingModelLimited, Says: "Fable wk limit", Clauses: []string{"back Mon 21:00", "other models still come here"}, Short: []string{"Fable wk limit"},
			},
		},
		{
			name:    "its token refused, its last read failing: the refusal",
			account: with(with(readNow(0.2, 0.3, 0), refusing(401, "")), failing),
			want:    status.Routing{Kind: status.RoutingRefused, Says: "refused (401)", Clauses: []string{"its token needs renewing"}, Short: []string{"refused (401)"}},
		},
		{
			name:    "its token refused before a model's limit, as no model comes here",
			account: with(with(readNow(0.2, 0.3, 1), refusing(401, "")), fableLimited),
			want:    status.Routing{Kind: status.RoutingRefused, Says: "refused (401)", Clauses: []string{"its token needs renewing"}, Short: []string{"refused (401)"}},
		},
		{
			name:    "at Fable's week's limit, a shared window at its cap holding back the rest: the cap",
			account: with(with(readNow(0.2, 0.92, 1), fableLimited), capped(0.1)),
			want: status.Routing{
				Kind: status.RoutingCapped, Says: "at its cap", Clauses: []string{"back Mon 21:00", "new sessions go elsewhere"}, Short: []string{"at its cap", "Mon 21:00"},
			},
		},
		{
			name:    "at Fable's week's limit, a shared window at a cap the pin spends: the limit",
			account: with(with(readNow(0.2, 0.92, 1), fableLimited), capped(0.1)),
			pin:     []string{"work"},
			want: status.Routing{
				Kind: status.RoutingModelLimited, Says: "Fable wk limit", Clauses: []string{"back Mon 21:00", "other models still come here"}, Short: []string{"Fable wk limit"},
			},
		},
		{
			name:    "a family's requests refused",
			account: with(readNow(0.2, 0.3, 0), refusing(403, "opus")),
			want: status.Routing{
				Kind: status.RoutingModelRefused, Says: "refused (403, opus)", Clauses: []string{"other models still come here"}, Short: []string{"refused (403)"},
			},
		},
		{
			name:    "a family's requests refused, its session at its cap holding back the rest: the cap",
			account: with(with(readNow(0.92, 0.3, 0), refusing(403, "opus")), capped(0.1)),
			want: status.Routing{
				Kind: status.RoutingCapped, Says: "at its cap", Clauses: []string{"back 17:10", "new sessions go elsewhere"}, Short: []string{"at its cap", "17:10"},
			},
		},
		{
			name:    "its usage unreadable, nothing read before",
			account: status.Account{ID: "work", TokenSet: true, Error: "HTTP 401 ·\x1b[2J Invalid bearer token"},
			want: status.Routing{
				Kind: status.RoutingUnreadable, Says: "can't read it", Clauses: []string{"HTTP 401 · [2J Invalid bearer token"}, Short: []string{"can't read it"},
			},
		},
		{
			name:    "its last read failing, its windows as last read saying how it stands",
			account: with(readNow(0.2, 0.3, 0), failing),
			want:    status.Routing{Kind: status.RoutingOpen, Says: "open", Short: []string{"open"}},
		},
		{
			name:    "nothing read yet",
			account: status.Account{ID: "work", TokenSet: true},
			want:    status.Routing{Kind: status.RoutingUnread, Says: "not read yet", Short: []string{"not read yet"}},
		},
		{
			name:    "at its cap in its session",
			account: with(readNow(0.95, 0.34, 0.12), capped(0.05)),
			others:  []status.Account{{ID: "side"}},
			best:    "side",
			want: status.Routing{
				Kind: status.RoutingCapped, Says: "at its cap", Clauses: []string{"back 17:10", "new sessions go elsewhere"}, Short: []string{"at its cap", "17:10"},
			},
		},
		{
			name:    "at its cap in its session and its week: back as the last of them resets",
			account: with(readNow(0.95, 0.96, 0.12), capped(0.05)),
			want: status.Routing{
				Kind: status.RoutingCapped, Says: "at its cap", Clauses: []string{"back Mon 21:00", "new sessions go elsewhere"}, Short: []string{"at its cap", "Mon 21:00"},
			},
		},
		{
			name:    "at its cap, the global pin naming it",
			account: with(readNow(0.95, 0.34, 0.12), capped(0.05)),
			pin:     []string{"work"},
			want: status.Routing{
				Kind: status.RoutingCappedPinned, Says: "at its cap", Clauses: []string{"pinned, so new sessions still come here"}, Short: []string{"at its cap", "pinned"},
			},
		},
		{
			name:    "a model's own window at its cap, other models still coming here",
			account: with(readNow(0.2, 0.3, 0.92), capped(0.1)),
			want: status.Routing{
				Kind: status.RoutingCapped, Says: "Fable wk at its cap", Clauses: []string{"back Mon 21:00", "other models still come here"}, Short: []string{"Fable wk at its cap", "Mon 21:00"},
			},
		},
		{
			name:    "at its cap in a window that has reset since: no longer",
			account: with(readNow(0.95, 0.34, 0.12), func(a *status.Account) { a.Reserve, a.Windows[0].ResetsAt = 0.05, stateNow.Add(-5*time.Minute).UTC() }),
			want:    status.Routing{Kind: status.RoutingOpen, Says: "open", Short: []string{"open"}},
		},
		{
			name:    "under pressure",
			account: with(readNow(0.58, 0.34, 0.12), pressing),
			others:  []status.Account{{ID: "side"}},
			best:    "side",
			want:    status.Routing{Kind: status.RoutingPressed, Says: "under pressure", Clauses: []string{"out ~16:05"}, Short: []string{"under pressure"}},
		},
		{
			name:    "under pressure, new sessions still coming here",
			account: with(readNow(0.58, 0.34, 0.12), pressing),
			others:  []status.Account{{ID: "side"}},
			best:    "work",
			want:    status.Routing{Kind: status.RoutingPressed, Says: "under pressure", Clauses: []string{"out ~16:05"}, Short: []string{"under pressure"}},
		},
		{
			name:    "under pressure, its cap holding it back first: out where its cap starts",
			account: with(with(readNow(0.58, 0.34, 0.12), pressing), capped(0.1)),
			want:    status.Routing{Kind: status.RoutingPressed, Says: "under pressure", Clauses: []string{"out ~15:45"}, Short: []string{"under pressure"}},
		},
		{
			name:    "its session lapsed, the router priming it",
			account: with(readNow(0, 0.05, 0), func(a *status.Account) { a.Lapsed = []string{"5h"} }),
			prime:   []status.Slot{{Account: "side", At: "07:10", Next: at(2, 7, 10).UTC()}, {Account: "work", At: "16:20", Next: at(1, 16, 20).UTC()}},
			want:    status.Routing{Kind: status.RoutingIdle, Says: "idle", Clauses: []string{"window starts at its prime, 16:20"}, Short: []string{"idle"}},
		},
		{
			name:    "its session lapsed, nothing saying when it's primed",
			account: with(readNow(0, 0.05, 0), func(a *status.Account) { a.Lapsed = []string{"5h"} }),
			want:    status.Routing{Kind: status.RoutingIdle, Says: "idle", Clauses: []string{"window starts with its next request"}, Short: []string{"idle"}},
		},
		{
			name:    "open, new sessions coming here",
			account: readNow(0.12, 0.33, 0.04),
			others:  []status.Account{{ID: "side"}},
			best:    "work",
			want:    status.Routing{Kind: status.RoutingNext, Says: "open", Clauses: []string{"new sessions come here"}, Short: []string{"open", "next"}},
		},
		{
			name:    "open, the one account there is",
			account: readNow(0.12, 0.33, 0.04),
			best:    "work",
			want:    status.Routing{Kind: status.RoutingOpen, Says: "open", Short: []string{"open"}},
		},
		{
			name:    "open, new sessions going to another",
			account: readNow(0.12, 0.33, 0.04),
			others:  []status.Account{{ID: "side"}},
			best:    "side",
			want:    status.Routing{Kind: status.RoutingOpen, Says: "open", Short: []string{"open"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{
				GeneratedAt: stateNow.UTC(), Best: tt.best, Pin: status.Pin{Accounts: tt.pin}, Prime: status.Prime{Window: "5h", Slots: tt.prime},
				Accounts: append([]status.Account{tt.account}, tt.others...),
			}
			got := doc.RoutingOf(tt.account, stateNow, policy)
			if got.Kind != tt.want.Kind || got.Says != tt.want.Says || !slices.Equal(got.Clauses, tt.want.Clauses) || !slices.Equal(got.Short, tt.want.Short) {
				t.Errorf("RoutingOf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestROUTINGAndAnAccountsStateJudgeItsCapAlike(t *testing.T) {
	reset := at(1, 17, 10)
	tests := []struct {
		name string
		// session is how much of the session is used, built when the
		// document was, and now when the account is judged.
		session    float64
		built, now time.Time
		capped     bool
	}{
		{name: "short of its reserve", session: 0.85, built: stateNow, now: stateNow},
		{name: "at its reserve", session: 0.9, built: stateNow, now: stateNow, capped: true},
		{name: "past it", session: 0.95, built: stateNow, now: stateNow, capped: true},
		{name: "a moment before the session resets", session: 0.95, built: reset.Add(-10 * time.Second), now: reset.Add(-time.Second), capped: true},
		{name: "a moment after it resets, before it's read again", session: 0.95, built: reset.Add(-10 * time.Second), now: reset.Add(time.Second)},
	}
	for _, tt := range tests {
		for _, noted := range []bool{true, false} {
			a := with(readNow(tt.session, 0.3, 0), func(a *status.Account) { a.Reserve = 0.1 })
			if noted {
				a = a.AsOf(policy, tt.built)
			}
			doc := status.Document{GeneratedAt: tt.built.UTC(), Source: status.SourceRouter, Accounts: []status.Account{a}}
			routing, state := doc.RoutingOf(a, tt.now, policy).Kind, doc.StateOf(a, tt.now, policy).Condition
			want := [2]bool{tt.capped, !tt.capped}
			got := [2]bool{routing == status.RoutingCapped && state == status.Reserved, routing == status.RoutingOpen && state == status.Open}
			if got != want {
				t.Errorf("%s, the router noting its reserve as built %t: ROUTING says %d, its state %d; want both capped %t", tt.name, noted, routing, state, tt.capped)
			}
		}
	}
}

func TestAnAccountAtItsCapIsOpenOnTheTickItsWindowResets(t *testing.T) {
	built := at(1, 17, 9)
	a := with(readNow(0.95, 0.3, 0), func(a *status.Account) { a.Reserve = 0.1 }).AsOf(policy, built)
	doc := status.Document{GeneratedAt: built.UTC(), Source: status.SourceRouter, Accounts: []status.Account{a}}
	// A moment after its session resets at 17:10, before it's read again.
	if got := doc.RoutingOf(a, at(1, 17, 10).Add(time.Second), policy); got.Kind != status.RoutingOpen || len(got.Clauses) > 0 {
		t.Errorf("RoutingOf() = %+v, want open, no time to be back", got)
	}
}

func TestEachKindLeadsWithItsGlyph(t *testing.T) {
	tests := map[status.RoutingKind]string{
		status.RoutingTokenless:    "✕",
		status.RoutingLimited:      "■",
		status.RoutingModelLimited: "■",
		status.RoutingRefused:      "■",
		status.RoutingModelRefused: "■",
		status.RoutingUnreadable:   "!",
		status.RoutingUnread:       "○",
		status.RoutingCapped:       "●",
		status.RoutingCappedPinned: "●",
		status.RoutingPressed:      "●",
		status.RoutingIdle:         "○",
		status.RoutingNext:         "●",
		status.RoutingOpen:         "●",
	}
	for kind, want := range tests {
		if got := kind.Glyph(); got != want {
			t.Errorf("kind %d leads with %q, want %q", kind, got, want)
		}
	}
}

func TestAnAccountsStandingInWords(t *testing.T) {
	tests := []struct {
		routing status.Routing
		want    string
	}{
		{routing: status.Routing{Says: "at its cap", Clauses: []string{"back 17:10", "new sessions go elsewhere"}}, want: "at its cap · back 17:10 · new sessions go elsewhere"},
		{routing: status.Routing{Says: "open"}, want: "open"},
	}
	for _, tt := range tests {
		if got := tt.routing.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}
