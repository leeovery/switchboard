package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// Detail's head says how an account stands in ROUTING's kind and first
// words, then where new sessions go, never when room returns: a limit
// holding back every request, or its cap, sends them elsewhere; a model's
// limit, cap or refusal, its cap the pin spends, and its token refused say
// what ROUTING says; under pressure, and open, say they come here where ▲
// marks it, and nothing of them where it doesn't; and the rest say what
// ROUTING says.
func TestDetailsHeadSaysWhereNewSessionsGo(t *testing.T) {
	limited := func(windows ...string) func(*status.Account) {
		return func(a *status.Account) { a.Limit = status.Limit{Windows: windows, Until: at(1, 15, 54).UTC()} }
	}
	fableLimited := func(a *status.Account) {
		a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: at(5, 21, 0).UTC()}
	}
	refusing := func(code int, family string) func(*status.Account) {
		return func(a *status.Account) {
			a.Refused = status.Refusal{Until: at(1, 21, 40).UTC(), Status: code, Family: family}
		}
	}
	capped := func(reserve float64) func(*status.Account) {
		return func(a *status.Account) { a.Reserve = reserve }
	}
	lapsed := func(a *status.Account) { a.Lapsed = []string{"5h"} }
	side := []status.Account{{ID: "side", Label: "side", TokenSet: true}}
	tests := []struct {
		name    string
		account status.Account
		// others are the document's other accounts, best the account new
		// sessions go to, which ▲ marks, and pin the accounts the global pin
		// names.
		others []status.Account
		best   string
		pin    []string
		want   string
	}{
		{name: "a limit holding back every request", account: with(readNow(0.6, 0.3, 0), limited()), others: side, best: "side", want: "limit reached · new sessions go elsewhere"},
		{
			name: "a limit holding back every request, the global pin naming it", account: with(readNow(0.6, 0.3, 0), limited()), others: side, best: "side", pin: []string{"work"},
			want: "limit reached · new sessions go elsewhere",
		},
		{name: "a limit holding back every request, the one account there is", account: with(readNow(0.6, 0.3, 0), limited()), best: "work", want: "limit reached"},
		{name: "at its cap", account: with(readNow(0.95, 0.34, 0.12), capped(0.05)), others: side, best: "side", want: "at its cap · new sessions go elsewhere"},
		{name: "at its cap, the one account there is", account: with(readNow(0.95, 0.34, 0.12), capped(0.05)), best: "work", want: "at its cap"},
		{
			name: "at its cap, the global pin naming it", account: with(readNow(0.95, 0.34, 0.12), capped(0.05)), others: side, best: "work", pin: []string{"work"},
			want: "at its cap · pinned, so new sessions still come here",
		},
		{name: "a model's limit", account: with(readNow(0.2, 0.3, 1), fableLimited), others: side, best: "work", want: "Fable wk limit · other models still come here"},
		{name: "a model's cap", account: with(readNow(0.2, 0.3, 0.92), capped(0.1)), others: side, best: "work", want: "Fable wk at its cap · other models still come here"},
		{
			name: "a model's cap, the global pin naming it", account: with(readNow(0.2, 0.3, 0.92), capped(0.1)), others: side, best: "work", pin: []string{"work"},
			want: "Fable wk at its cap · pinned, so new sessions still come here",
		},
		{name: "its token refused", account: with(readNow(0.2, 0.3, 0), refusing(401, "")), others: side, best: "side", want: "refused (401) · its token needs renewing"},
		{name: "one model refused", account: with(readNow(0.2, 0.3, 0), refusing(403, "opus")), others: side, best: "work", want: "refused (403, opus) · other models still come here"},
		{name: "under pressure, new sessions coming here", account: with(readNow(0.58, 0.34, 0.12), pressing), others: side, best: "work", want: "under pressure · new sessions still come here"},
		{name: "under pressure, the one account there is", account: with(readNow(0.58, 0.34, 0.12), pressing), best: "work", want: "under pressure · new sessions still come here"},
		{name: "under pressure, new sessions going elsewhere", account: with(readNow(0.58, 0.34, 0.12), pressing), others: side, best: "side", want: "under pressure"},
		{name: "open, new sessions coming here", account: readNow(0.12, 0.33, 0.04), others: side, best: "work", want: "open · new sessions come here"},
		{name: "open, the one account there is", account: readNow(0.12, 0.33, 0.04), best: "work", want: "open · new sessions come here"},
		{name: "open, new sessions going elsewhere", account: readNow(0.12, 0.33, 0.04), others: side, best: "side", want: "open"},
		{name: "idle", account: with(readNow(0, 0.05, 0), lapsed), others: side, best: "work", want: "idle · window starts with its next request"},
		{name: "not read yet", account: status.Account{ID: "work", Label: "work", TokenSet: true}, others: side, best: "side", want: "not read yet"},
		{
			name: "unreadable", account: status.Account{ID: "work", Label: "work", TokenSet: true, Error: "timed out"}, others: side, best: "side",
			want: "can't read it · timed out",
		},
		{name: "without a usable token", account: status.Account{ID: "work", Label: "work"}, others: side, best: "side", want: "no token · switchboard accounts token work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{
				GeneratedAt: stateNow.UTC(), Source: status.SourceRouter, Best: tt.best, Pin: status.Pin{Accounts: tt.pin},
				Accounts: append([]status.Account{tt.account}, tt.others...),
			}
			a, _ := doc.At(stateNow, policy).Account("work")
			head := a.Headed()
			if got := head.String(); got != tt.want {
				t.Errorf("Headed() says %q, want %q; ROUTING says %q", got, tt.want, a.Routing)
			}
			if head.Kind != a.Routing.Kind || !slices.Equal(head.Short, []string{a.Routing.Says}) || !head.Back.IsZero() {
				t.Errorf("Headed() = %+v, want ROUTING's kind, %d, its first words alone short, %q, and no time to be back", head, a.Routing.Kind, a.Routing.Says)
			}
		})
	}
}

// Detail's head follows ▲ and the account's state from look to look: under
// pressure where new sessions go to it, then idle once its session resets,
// before it's read again; and of another read, where they go elsewhere.
func TestDetailsHeadFollowsTheAccountFromLookToLook(t *testing.T) {
	reset := at(1, 17, 10)
	a := with(readNow(0.58, 0.34, 0.12), pressing).AsOf(policy, stateNow)
	others := []status.Account{{ID: "side", Label: "side", TokenSet: true}}
	tests := []struct {
		name string
		best string
		now  time.Time
		want string
	}{
		{name: "new sessions coming here, its session running", best: "work", now: reset.Add(-time.Second), want: "under pressure · new sessions still come here"},
		{name: "new sessions coming here, its session reset", best: "work", now: reset.Add(time.Second), want: "idle · window starts with its next request"},
		{name: "new sessions going elsewhere, its session running", best: "side", now: reset.Add(-time.Second), want: "under pressure"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{GeneratedAt: stateNow.UTC(), Source: status.SourceRouter, Best: tt.best, Accounts: append([]status.Account{a}, others...)}
			work, _ := doc.At(tt.now, policy).Account("work")
			if got := work.Headed().String(); got != tt.want {
				t.Errorf("at %v, Headed() says %q, want %q", tt.now, got, tt.want)
			}
		})
	}
}
