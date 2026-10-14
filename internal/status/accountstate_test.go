package status_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

// Every kind of how an account stands with the router is in one state in a
// word: held back from some models' requests alone, or idle, it's open, and
// without a token or unreadable, not read.
func TestEachKindIsInOneStateInAWord(t *testing.T) {
	tests := []struct {
		name string
		kind status.RoutingKind
		want status.AccountState
	}{
		{name: "without a usable token", kind: status.RoutingTokenless, want: status.StateNotRead},
		{name: "a limit holding back every request", kind: status.RoutingLimited, want: status.StateLimit},
		{name: "a limit holding back some models", kind: status.RoutingModelLimited, want: status.StateOpen},
		{name: "its token refused", kind: status.RoutingRefused, want: status.StateRefused},
		{name: "one model refused", kind: status.RoutingModelRefused, want: status.StateOpen},
		{name: "unreadable", kind: status.RoutingUnreadable, want: status.StateNotRead},
		{name: "not read yet", kind: status.RoutingUnread, want: status.StateNotRead},
		{name: "at its cap", kind: status.RoutingCapped, want: status.StateAtCap},
		{name: "at its cap, the global pin naming it", kind: status.RoutingCappedPinned, want: status.StateAtCap},
		{name: "a model's own window at its cap", kind: status.RoutingModelCapped, want: status.StateOpen},
		{name: "under pressure", kind: status.RoutingPressed, want: status.StatePressure},
		{name: "idle", kind: status.RoutingIdle, want: status.StateOpen},
		{name: "open, new sessions coming here", kind: status.RoutingNext, want: status.StateOpen},
		{name: "open", kind: status.RoutingOpen, want: status.StateOpen},
		{name: "no kind", kind: 0, want: 0},
		{name: "a kind past the last", kind: status.RoutingOpen + 1, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.kind.State(); got != tt.want {
				t.Errorf("RoutingKind(%d).State() = %d (%q), want %d (%q)", tt.kind, got, got.Word(), tt.want, tt.want.Word())
			}
		})
	}
}

func TestEachStateInAWordLeadsWithItsGlyph(t *testing.T) {
	tests := []struct {
		state       status.AccountState
		word, glyph string
	}{
		{state: status.StateOpen, word: "open", glyph: "●"},
		{state: status.StatePressure, word: "pressure", glyph: "●"},
		{state: status.StateAtCap, word: "at cap", glyph: "●"},
		{state: status.StateLimit, word: "limit", glyph: "■"},
		{state: status.StateRefused, word: "refused", glyph: "■"},
		{state: status.StateNotRead, word: "not read", glyph: "○"},
	}
	for _, tt := range tests {
		t.Run(tt.word, func(t *testing.T) {
			if got := tt.state.Word(); got != tt.word {
				t.Errorf("AccountState(%d).Word() = %q, want %q", tt.state, got, tt.word)
			}
			if got := tt.state.Glyph(); got != tt.glyph {
				t.Errorf("AccountState(%d).Glyph() = %q, want %q", tt.state, got, tt.glyph)
			}
		})
	}
}

// Each state in a word means what State maps into it: open, among the rest,
// an account a limit, a refusal or its cap in a model's own window holds
// back from some models; at cap, one a pin spends too; and not read, one
// without a usable token, unreadable or not yet read.
func TestEachStateInAWordMeansWhatItsKindsAre(t *testing.T) {
	tests := []struct {
		state status.AccountState
		means string
	}{
		{state: status.StateOpen, means: "new sessions can go to it, though a limit, a refusal or its cap in a model's own window may hold back some models"},
		{state: status.StatePressure, means: "at its recent rate, it runs out before its window resets"},
		{state: status.StateAtCap, means: "at its cap, where its reserve starts: the router passes it over, unless a pin spends it"},
		{state: status.StateLimit, means: "a limit holds it back from every request"},
		{state: status.StateRefused, means: "its token was refused"},
		{state: status.StateNotRead, means: "its usage isn't read: it has no usable token, or its usage couldn't be read or hasn't been yet"},
		{state: 0},
	}
	for _, tt := range tests {
		t.Run(tt.state.Word(), func(t *testing.T) {
			if got := tt.state.Means(); got != tt.means {
				t.Errorf("AccountState(%d).Means() = %q, want %q", tt.state, got, tt.means)
			}
		})
	}
}

// Accounts' states are counted in their order, open, pressure, at cap,
// limit, refused and not read, whatever the accounts' own, those none is in
// left out; and too wide, how many are open, pressure counting as open.
func TestAccountsStatesAreCountedInTheirOrder(t *testing.T) {
	tests := []struct {
		name     string
		kinds    []status.RoutingKind
		want     status.StateCounts
		open, of int
	}{
		{
			name:  "the frame's: one at its cap, one new sessions come to",
			kinds: []status.RoutingKind{status.RoutingCapped, status.RoutingOpen, status.RoutingNext},
			want:  status.StateCounts{{State: status.StateOpen, N: 2}, {State: status.StateAtCap, N: 1}},
			open:  2, of: 3,
		},
		{
			name: "each in its order",
			kinds: []status.RoutingKind{
				status.RoutingUnread, status.RoutingRefused, status.RoutingLimited, status.RoutingCappedPinned, status.RoutingPressed, status.RoutingIdle,
			},
			want: status.StateCounts{
				{State: status.StateOpen, N: 1}, {State: status.StatePressure, N: 1}, {State: status.StateAtCap, N: 1},
				{State: status.StateLimit, N: 1}, {State: status.StateRefused, N: 1}, {State: status.StateNotRead, N: 1},
			},
			open: 2, of: 6,
		},
		{
			name:  "held back from some models' requests alone: open to the rest",
			kinds: []status.RoutingKind{status.RoutingModelLimited, status.RoutingModelRefused, status.RoutingModelCapped},
			want:  status.StateCounts{{State: status.StateOpen, N: 3}},
			open:  3, of: 3,
		},
		{
			name: "the ten accounts' frame: eight of ten open, pressure counting",
			kinds: []status.RoutingKind{
				status.RoutingCapped, status.RoutingOpen, status.RoutingNext, status.RoutingOpen, status.RoutingOpen,
				status.RoutingPressed, status.RoutingLimited, status.RoutingOpen, status.RoutingOpen, status.RoutingOpen,
			},
			want: status.StateCounts{
				{State: status.StateOpen, N: 7}, {State: status.StatePressure, N: 1}, {State: status.StateAtCap, N: 1}, {State: status.StateLimit, N: 1},
			},
			open: 8, of: 10,
		},
		{name: "none", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := status.CountStates(accountsOfKinds(tt.kinds...))
			if !slices.Equal(got, tt.want) {
				t.Errorf("CountStates() = %+v, want %+v", got, tt.want)
			}
			if open, of := got.OpenOf(); open != tt.open || of != tt.of {
				t.Errorf("OpenOf() = %d, %d; want %d of %d", open, of, tt.open, tt.of)
			}
		})
	}
}

// The accounts asked for are counted, those the config has, whatever the
// moment holds: one it lacks yet as not read, and one it holds that isn't
// asked for, not at all.
func TestTheAccountsAskedForAreCountedThoseTheMomentLacksNotRead(t *testing.T) {
	accounts := accountsOfKinds(status.RoutingCapped, status.RoutingOpen, status.RoutingIdle)
	for i, id := range []string{"work", "personal", "side"} {
		accounts[i].ID = id
	}
	m := status.Moment{Accounts: accounts}
	tests := []struct {
		name string
		ids  []string
		want status.StateCounts
	}{
		{name: "one the moment lacks", ids: []string{"work", "personal", "side", "added"}, want: status.StateCounts{
			{State: status.StateOpen, N: 2}, {State: status.StateAtCap, N: 1}, {State: status.StateNotRead, N: 1},
		}},
		{name: "one the moment holds not asked for", ids: []string{"personal", "side"}, want: status.StateCounts{{State: status.StateOpen, N: 2}}},
	}
	for _, tt := range tests {
		if got := m.CountStatesOf(tt.ids); !slices.Equal(got, tt.want) {
			t.Errorf("%s: CountStatesOf(%q) = %+v, want %+v", tt.name, tt.ids, got, tt.want)
		}
	}
	if got := m.StateInAWord("side"); got != status.StateOpen {
		t.Errorf("side, idle, is %v in a word, want open", got)
	}
	if got := m.StateInAWord("added"); got != status.StateNotRead {
		t.Errorf("an account the moment lacks is %v in a word, want not read", got)
	}
}

// accountsOfKinds are accounts at a moment, each standing with the router as
// the kind given.
func accountsOfKinds(kinds ...status.RoutingKind) []status.AccountNow {
	accounts := make([]status.AccountNow, len(kinds))
	for i, k := range kinds {
		accounts[i].Routing.Kind = k
	}
	return accounts
}
