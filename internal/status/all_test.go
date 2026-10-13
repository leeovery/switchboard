package status_test

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// allWindow is what's wanted of a window of every account taken together.
type allWindow struct {
	key        string
	of         int
	used, room float64
	// pace is the mean even pace wanted, or none where it's negative.
	pace  float64
	stale bool
}

// String says what's wanted of the window, as allWindowOf says what it is.
func (w allWindow) String() string {
	return fmt.Sprintf("{%s of %d: used %.4f, room %.4f, pace %.4f, stale %v}", w.key, w.of, w.used, w.room, w.pace, w.stale)
}

// allWindowOf is the window as allWindow says it, its pace negative where it
// has none.
func allWindowOf(w status.AllWindowNow) allWindow {
	pace := -1.0
	if w.Pace != nil {
		pace = *w.Pace
	}
	return allWindow{key: w.Key, of: w.Of, used: w.Used, room: w.Room, pace: pace, stale: w.Stale}
}

// near reports whether the windows are alike, their figures within a
// rounding of each other.
func (w allWindow) near(o allWindow) bool {
	within := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	return w.key == o.key && w.of == o.of && w.stale == o.stale && within(w.used, o.used) && within(w.room, o.room) && within(w.pace, o.pace)
}

// all accounts takes every account's windows together, whatever the global
// pin names or holds any back: a window's use the mean of those whose use of
// it is known, a stale or refused account's as last read among them, its
// room what each has left to its limit, summed, and its pace the mean of
// those running; an account without the window, a usable token, or a read
// counts in none of them; and with one account there's nothing to take.
func TestAllAccountsTakesEveryAccountsWindowsTogether(t *testing.T) {
	failed := func(a *status.Account) { a.Error = "HTTP 529 · Overloaded" }
	refused := func(a *status.Account) { a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 401} }
	limited := func(a *status.Account) {
		a.Limit = status.Limit{ID: 4, Windows: []string{"5h"}, Until: built.Add(time.Hour)}
	}
	tokenless := func(a *status.Account) { a.TokenSet = false }
	fable := func(used float64) func(*status.Account) {
		return func(a *status.Account) { a.Windows = append(a.Windows, windowIn("7d_oi", used, 3*day)) }
	}
	routed := func(pinned []string, accounts ...status.Account) status.Document {
		var all []statustest.Routed
		for _, a := range accounts {
			all = append(all, statustest.Routed{Account: a})
		}
		return statustest.Router(policy, pinned, all...)
	}
	tests := []struct {
		name string
		doc  status.Document
		now  time.Time
		want []allWindow
	}{
		{
			name: "every account read: Compare's week",
			doc: routed(nil,
				readAt("work", windowIn("7d", 0.34, 4*day)), readAt("personal", windowIn("7d", 0.66, 5*day)), readAt("side", windowIn("7d", 0.78, 6*day)),
			),
			now:  built.Add(time.Minute),
			want: []allWindow{{key: "7d", of: 3, used: 0.5933333333333334, room: 1.22, pace: 2.0 / 7}},
		},
		{
			name: "one keeping a reserve: its room to its limit, not its cap, as the pool's is",
			doc:  routed(nil, with(readAt("work", windowIn("5h", 0.4, 2*time.Hour)), reserving), readAt("side", windowIn("5h", 0.1, 4*time.Hour))),
			now:  built.Add(time.Minute),
			want: []allWindow{{key: "5h", of: 2, used: 0.25, room: 1.5, pace: 0.4}},
		},
		{
			name: "one at its limit",
			doc:  routed(nil, with(readAt("work", spentIn("5h", 1, time.Hour)), limited), readAt("side", windowIn("5h", 0.1, 4*time.Hour))),
			now:  built.Add(time.Minute),
			want: []allWindow{{key: "5h", of: 2, used: 0.55, room: 0.9, pace: 0.5}},
		},
		{
			name: "a window reset since read, not running till a request starts it",
			doc:  routed(nil, readAt("work", windowIn("5h", 0.8, time.Hour)), readAt("side", windowIn("5h", 0.3, 3*time.Hour))),
			now:  built.Add(time.Hour + 5*time.Second),
			want: []allWindow{{key: "5h", of: 2, used: 0.15, room: 1.7, pace: 0.4}},
		},
		{
			name: "one stale",
			doc:  routed(nil, with(readAt("work", windowIn("5h", 0.4, 2*time.Hour)), failed), readAt("side", windowIn("5h", 0.1, 4*time.Hour))),
			now:  built.Add(time.Minute),
			want: []allWindow{{key: "5h", of: 2, used: 0.25, room: 1.5, pace: 0.4}},
		},
		{
			name: "every one stale",
			doc: routed(nil,
				with(readAt("work", windowIn("5h", 0.4, 2*time.Hour)), failed), with(readAt("side", windowIn("5h", 0.1, 4*time.Hour)), failed),
			),
			now:  built.Add(time.Minute),
			want: []allWindow{{key: "5h", of: 2, used: 0.25, room: 1.5, pace: 0.4, stale: true}},
		},
		{
			name: "one refused",
			doc:  routed(nil, with(readAt("work", windowIn("5h", 0.4, 2*time.Hour)), refused), readAt("side", windowIn("5h", 0.1, 4*time.Hour))),
			now:  built.Add(time.Minute),
			want: []allWindow{{key: "5h", of: 2, used: 0.25, room: 1.5, pace: 0.4}},
		},
		{
			name: "Fable's week on two of three",
			doc: routed(nil,
				with(readAt("work", windowIn("7d", 0.3, 3*day)), fable(0.2)), with(readAt("personal", windowIn("7d", 0.5, 3*day)), fable(0.6)),
				readAt("side", windowIn("7d", 0.1, 3*day)),
			),
			now: built.Add(time.Minute),
			want: []allWindow{
				{key: "7d", of: 3, used: 0.3, room: 2.1, pace: 4.0 / 7},
				{key: "7d_oi", of: 2, used: 0.4, room: 1.2, pace: 4.0 / 7},
			},
		},
		{
			name: "one never read",
			doc:  routed(nil, status.Account{ID: "work", Label: "work", TokenSet: true}, readAt("side", windowIn("5h", 0.1, 4*time.Hour))),
			now:  built.Add(time.Minute),
			want: []allWindow{{key: "5h", of: 1, used: 0.1, room: 0.9, pace: 0.2}},
		},
		{
			name: "one without a usable token, read before: its Fable week, none's known, left out",
			doc: routed(nil,
				with(with(readAt("work", windowIn("5h", 0.6, 2*time.Hour)), fable(0.3)), tokenless), readAt("side", windowIn("5h", 0.1, 4*time.Hour)),
			),
			now:  built.Add(time.Minute),
			want: []allWindow{{key: "5h", of: 1, used: 0.1, room: 0.9, pace: 0.2}},
		},
		{
			name: "the global pin naming one: every account still",
			doc:  routed([]string{"side"}, readAt("work", windowIn("5h", 0.4, 2*time.Hour)), readAt("side", windowIn("5h", 0.1, 4*time.Hour))),
			now:  built.Add(time.Minute),
			want: []allWindow{{key: "5h", of: 2, used: 0.25, room: 1.5, pace: 0.4}},
		},
		{
			name: "one account: nothing to take together",
			doc:  routed(nil, readAt("work", windowIn("5h", 0.4, 2*time.Hour), windowIn("7d", 0.5, 2*day))),
			now:  built.Add(time.Minute),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.doc.WorkedOut(policy).At(tt.now, policy).All().Windows
			if len(got) != len(tt.want) {
				t.Fatalf("All() has %d windows, %v; want %d, %v", len(got), got, len(tt.want), tt.want)
			}
			for i, w := range got {
				if g := allWindowOf(w); !g.near(tt.want[i]) {
					t.Errorf("All()'s window %d is %v, want %v", i, g, tt.want[i])
				}
			}
		})
	}
}
