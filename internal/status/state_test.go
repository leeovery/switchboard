package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// stateNow is when the states are said: Thursday 1 October 2026, 14:42:07,
// an hour east of UTC.
var stateNow = time.Date(2026, 10, 1, 14, 42, 7, 0, time.FixedZone("UTC+1", 60*60))

// at is a time of day on stateNow's day, or on another day of its month.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, 10, day, hour, minute, 0, 0, stateNow.Location())
}

// readNow is an account with a usable token, its windows read just now: the
// session, the week and Fable's week.
func readNow(session, week, fable float64) status.Account {
	return status.Account{
		ID: "work", Label: "work", TokenSet: true, FetchedAt: stateNow.Add(-4 * time.Second).UTC(),
		Windows: []quota.Window{
			{Key: "5h", Label: "Session", Utilization: session, ResetsAt: at(1, 17, 10).UTC()},
			{Key: "7d", Label: "Week", Utilization: week, ResetsAt: at(5, 21, 0).UTC()},
			{Key: "7d_oi", Label: "Fable week", Utilization: fable, ResetsAt: at(5, 21, 0).UTC()},
		},
	}
}

// with is the account a as change leaves it.
func with(a status.Account, change func(*status.Account)) status.Account {
	change(&a)
	return a
}

// pressing has the account's session run out at 16:05 at its last half
// hour's rate, before it resets at 17:10: under pressure.
func pressing(a *status.Account) {
	rate := (1 - a.Windows[0].Utilization) / at(1, 16, 5).Sub(stateNow).Hours()
	since := stateNow.Add(-score.Recent).UTC()
	a.Rates = []status.Rate{{Window: "5h", Rate: rate, Since: since}}
	a.Pressure = status.Pressure{Window: "5h", Rate: rate, Recent: true, Since: since, RunsOut: at(1, 16, 5).UTC(), Under: true}
}

func TestTheStateAnAccountIsIn(t *testing.T) {
	limited := func(windows ...string) func(*status.Account) {
		return func(a *status.Account) { a.Limit = status.Limit{Windows: windows, Until: at(1, 15, 54).UTC()} }
	}
	refusing := func(code int, family string) func(*status.Account) {
		return func(a *status.Account) {
			a.Refused = status.Refusal{Until: at(1, 21, 40).UTC(), Status: code, Family: family}
		}
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
		want   status.State
	}{
		{
			name:    "without a usable token",
			account: with(readNow(0.2, 0.3, 0), func(a *status.Account) { a.TokenSet, a.Error = false, "token missing" }),
			want:    status.State{Condition: status.Tokenless, Says: "no token", Then: "switchboard accounts token work"},
		},
		{
			name:    "its usage unreadable",
			account: status.Account{ID: "work", TokenSet: true, Error: "HTTP 401 ·\x1b[2J Invalid bearer token"},
			want:    status.State{Condition: status.Unreadable, Says: "can't read it", Then: "HTTP 401 · [2J Invalid bearer token"},
		},
		{
			name:    "its last probe failing, its windows as last read",
			account: with(readNow(0.2, 0.3, 0), func(a *status.Account) { a.Error = "HTTP 529 · Overloaded" }),
			want:    status.State{Condition: status.Open, Says: "open"},
		},
		{
			name:    "nothing read yet",
			account: status.Account{ID: "work", TokenSet: true},
			want:    status.State{Condition: status.Unread, Says: "not read yet"},
		},
		{
			name:    "at its session's limit",
			account: with(readNow(1, 0.3, 0), limited("5h")),
			want:    status.State{Condition: status.Limited, Says: "limit reached", Then: "back 15:54, in 1h 11m"},
		},
		{
			name:    "at a limit naming no window",
			account: with(readNow(0.6, 0.3, 0), limited()),
			want:    status.State{Condition: status.Limited, Says: "limit reached", Then: "back 15:54, in 1h 11m"},
		},
		{
			name: "its session read spent, as probing reads it",
			account: with(readNow(1, 0.3, 0), func(a *status.Account) {
				a.Windows[0].Status = quota.StatusRejected
			}),
			want: status.State{Condition: status.Limited, Says: "limit reached", Then: "back 17:10, in 2h 27m"},
		},
		{
			name:    "its token refused",
			account: with(readNow(0.2, 0.3, 0), refusing(401, "")),
			want:    status.State{Condition: status.Limited, Says: "refused (401)", Then: "until 21:40"},
		},
		{
			name:    "a limit before its token refused",
			account: with(with(readNow(1, 0.3, 0), limited("5h")), refusing(401, "")),
			want:    status.State{Condition: status.Limited, Says: "limit reached", Then: "back 15:54, in 1h 11m"},
		},
		{
			name: "at Fable's week's limit",
			account: with(readNow(0.2, 0.3, 1), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: at(5, 21, 0).UTC()}
			}),
			want: status.State{Condition: status.PartlyLimited, Says: "Fable wk limit", Then: "back Mon 21:00 · other models still come here"},
		},
		{
			name:    "its token refused before a model's limit",
			account: with(with(readNow(0.2, 0.3, 1), refusing(401, "")), limited("7d_oi")),
			want:    status.State{Condition: status.Limited, Says: "refused (401)", Then: "until 21:40"},
		},
		{
			name:    "a family's requests refused",
			account: with(readNow(0.2, 0.3, 0), refusing(403, "opus")),
			want:    status.State{Condition: status.PartlyLimited, Says: "refused (403, opus)", Then: "until 21:40 · other models still come here"},
		},
		{
			name:    "at its reserve",
			account: with(readNow(0.2, 0.92, 0), func(a *status.Account) { a.Reserve, a.AtReserve = 0.1, []string{"7d"} }),
			want:    status.State{Condition: status.Reserved, Says: "at its reserve (90%)"},
		},
		{
			name:    "spending its reserve, pinned",
			account: with(readNow(0.2, 0.92, 0), func(a *status.Account) { a.Reserve, a.AtReserve = 0.1, []string{"7d"} }),
			pin:     []string{"work"},
			want:    status.State{Condition: status.Reserved, Says: "spending its reserve (pinned)"},
		},
		{
			name:    "a model's own window at its reserve, other models still coming here",
			account: with(readNow(0.2, 0.3, 0.92), func(a *status.Account) { a.Reserve, a.AtReserve = 0.1, []string{"7d_oi"} }),
			want:    status.State{Condition: status.Reserved, Says: "Fable wk at its reserve (90%)", Then: "other models still come here"},
		},
		{
			name:    "a shared window and a model's own at the reserve: held back from every request",
			account: with(readNow(0.2, 0.92, 0.95), func(a *status.Account) { a.Reserve, a.AtReserve = 0.1, []string{"7d", "7d_oi"} }),
			want:    status.State{Condition: status.Reserved, Says: "at its reserve (90%)"},
		},
		{
			name:    "a model's own window at its reserve, pinned",
			account: with(readNow(0.2, 0.3, 0.92), func(a *status.Account) { a.Reserve, a.AtReserve = 0.1, []string{"7d_oi"} }),
			pin:     []string{"work"},
			want:    status.State{Condition: status.Reserved, Says: "spending its reserve (pinned)"},
		},
		{
			name:    "under pressure, new sessions going to another",
			account: with(readNow(0.58, 0.34, 0.12), pressing),
			best:    "side",
			want:    status.State{Condition: status.Pressed, Says: "under pressure", Then: "new sessions go elsewhere"},
		},
		{
			name:    "under pressure, new sessions still coming here",
			account: with(readNow(0.58, 0.34, 0.12), pressing),
			best:    "work",
			want:    status.State{Condition: status.Pressed, Says: "under pressure", Then: "runs out ~16:05 at this pace"},
		},
		{
			name:    "under pressure, its reserve holding it back first",
			account: with(with(readNow(0.58, 0.34, 0.12), pressing), func(a *status.Account) { a.Reserve = 0.1 }),
			best:    "work",
			want:    status.State{Condition: status.Pressed, Says: "under pressure", Then: "at its reserve ~15:45"},
		},
		{
			name:    "under pressure, its reserve spent by the pin",
			account: with(with(readNow(0.58, 0.34, 0.12), pressing), func(a *status.Account) { a.Reserve = 0.1 }),
			pin:     []string{"work"},
			best:    "work",
			want:    status.State{Condition: status.Pressed, Says: "under pressure", Then: "runs out ~16:05 at this pace"},
		},
		{
			name:    "its session lapsed, the router priming it",
			account: with(readNow(0, 0.05, 0), func(a *status.Account) { a.Lapsed = []string{"5h"} }),
			prime:   []status.Slot{{Account: "side", At: "07:10", Next: at(2, 7, 10).UTC()}, {Account: "work", At: "16:20", Next: at(1, 16, 20).UTC()}},
			want:    status.State{Condition: status.Idle, Says: "idle", Then: "window starts at its prime, 16:20"},
		},
		{
			name:    "its session lapsed, nothing saying when it's primed",
			account: with(readNow(0, 0.05, 0), func(a *status.Account) { a.Lapsed = []string{"5h"} }),
			prime:   []status.Slot{{Account: "work", At: "16:20"}},
			want:    status.State{Condition: status.Idle, Says: "idle", Then: "window starts with its next request"},
		},
		{
			name:    "open, new sessions coming here",
			account: readNow(0.12, 0.33, 0.04),
			others:  []status.Account{{ID: "side"}},
			best:    "work",
			want:    status.State{Condition: status.Open, Says: "open", Then: "new sessions come here"},
		},
		{
			name:    "open, the one account there is",
			account: readNow(0.12, 0.33, 0.04),
			best:    "work",
			want:    status.State{Condition: status.Open, Says: "open"},
		},
		{
			name:    "open, new sessions going to another",
			account: readNow(0.12, 0.33, 0.04),
			others:  []status.Account{{ID: "side"}},
			best:    "side",
			want:    status.State{Condition: status.Open, Says: "open"},
		},
		{
			name:    "open, its week within 10 points of its reserve",
			account: with(readNow(0.31, 0.78, 0.2), func(a *status.Account) { a.Reserve = 0.2 }),
			others:  []status.Account{{ID: "side"}},
			best:    "work",
			want:    status.State{Condition: status.Open, Says: "open", Then: "its week nears its reserve"},
		},
		{
			name:    "open, Fable's week within 10 points of its reserve",
			account: with(readNow(0.31, 0.5, 0.71), func(a *status.Account) { a.Reserve = 0.2 }),
			want:    status.State{Condition: status.Open, Says: "open", Then: "its Fable week nears its reserve"},
		},
		{
			name:    "open, its week further from its reserve",
			account: with(readNow(0.31, 0.69, 0.2), func(a *status.Account) { a.Reserve = 0.2 }),
			want:    status.State{Condition: status.Open, Says: "open"},
		},
		{
			name:    "open, its week near where a reserve the pin spends would start",
			account: with(readNow(0.31, 0.78, 0.2), func(a *status.Account) { a.Reserve = 0.2 }),
			pin:     []string{"work"},
			want:    status.State{Condition: status.Open, Says: "open"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := status.Document{
				Best: tt.best, Pin: status.Pin{Accounts: tt.pin}, Prime: status.Prime{Window: "5h", Slots: tt.prime},
				Accounts: append([]status.Account{tt.account}, tt.others...),
			}
			got := doc.StateOf(tt.account, stateNow, policy)
			if got != tt.want {
				t.Errorf("StateOf() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestAStateInWords(t *testing.T) {
	tests := []struct {
		state status.State
		want  string
	}{
		{state: status.State{Condition: status.Pressed, Says: "under pressure", Then: "new sessions go elsewhere"}, want: "under pressure · new sessions go elsewhere"},
		{state: status.State{Condition: status.Open, Says: "open"}, want: "open"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("String() = %q, want %q", got, tt.want)
		}
	}
}

func TestWhatHoldsAnAccountBackAtItsLimit(t *testing.T) {
	tests := []struct {
		name    string
		account status.Account
		want    status.Hold
		wantOK  bool
	}{
		{
			name: "the limit the router saw, its shared window first",
			account: with(readNow(1, 1, 1), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"7d_oi", "5h"}, Until: at(1, 15, 54).UTC()}
			}),
			want:   status.Hold{Windows: []string{"5h", "7d_oi"}, Until: at(1, 15, 54).UTC(), Every: true},
			wantOK: true,
		},
		{
			name:    "a model's own window alone",
			account: with(readNow(0.2, 0.3, 1), func(a *status.Account) { a.Limit = status.Limit{Windows: []string{"7d_oi"}, Until: at(5, 21, 0).UTC()} }),
			want:    status.Hold{Windows: []string{"7d_oi"}, Until: at(5, 21, 0).UTC()},
			wantOK:  true,
		},
		{
			name:    "a window read spent, without the router",
			account: readNow(0.2, 1.02, 0),
			want:    status.Hold{Windows: []string{"7d"}, Until: at(5, 21, 0).UTC(), Every: true},
			wantOK:  true,
		},
		{
			name: "a limit since lifted",
			account: with(readNow(0.2, 0.3, 0), func(a *status.Account) {
				a.Limit = status.Limit{Windows: []string{"5h"}, Until: stateNow.Add(-time.Minute)}
			}),
		},
		{name: "room in every window", account: readNow(0.2, 0.3, 0)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.account.Held(stateNow, policy)
			if ok != tt.wantOK || !got.Until.Equal(tt.want.Until) || got.Every != tt.want.Every || !slices.Equal(got.Windows, tt.want.Windows) {
				t.Errorf("Held() = %+v, %v, want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestAHoldHoldsTheWindowsItNamesOrEveryWindowNamingNone(t *testing.T) {
	tests := []struct {
		name string
		hold status.Hold
		want map[string]bool
	}{
		{name: "naming some", hold: status.Hold{Windows: []string{"5h", "7d_oi"}, Every: true}, want: map[string]bool{"5h": true, "7d": false, "7d_oi": true}},
		{name: "naming none, as a limit only the overall verdict said", hold: status.Hold{Every: true}, want: map[string]bool{"5h": true, "7d": true, "7d_oi": true}},
	}
	for _, tt := range tests {
		for key, want := range tt.want {
			if got := tt.hold.Holds(key); got != want {
				t.Errorf("%s: Holds(%q) = %v, want %v", tt.name, key, got, want)
			}
		}
	}
}
