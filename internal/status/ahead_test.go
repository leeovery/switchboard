package status_test

import (
	"math"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// roomAt is the room a window is to have at a time from the moment's Now,
// and whether it's known to draw.
type roomAt struct {
	after time.Duration
	room  float64
	known bool
}

// A window's room ahead falls straight from the room now to where it runs
// out, its floor its cap where its reserve holds, else empty, then keeps
// that floor to its reset; or, where it lasts, straight to the room it
// leaves at its reset. A limit holding it leaves none till it lifts, its cap
// the room the cap leaves till its reset. Past its reset it has all of it.
// Nothing is drawn before now, nor of a window with nothing to go by.
func TestTheRoomAWindowHasAheadByItsProjection(t *testing.T) {
	// The week, 60% used three days in, runs out two days on at its pace;
	// keeping a tenth back, it reaches its cap a day and a half on.
	runningOut := readAt("work", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.6, 4*day))
	reserved := with(runningOut, reserving)
	// The week, 30% used three days in, heads for 70% by its reset.
	lasting := readAt("work", windowIn("5h", 0.1, 4*time.Hour), windowIn("7d", 0.3, 4*day))
	// A limit naming no window holds every window for an hour.
	unnamed := with(runningOut, func(a *status.Account) { a.Limit = status.Limit{Until: built.Add(time.Hour)} })
	// The session, read spent, back at its reset in two hours.
	spent := readAt("work", spentIn("5h", 1, 2*time.Hour), windowIn("7d", 0.3, 4*day))
	// The session, at a 10% reserve's cap, resets in two hours.
	capped := with(readAt("work", windowIn("5h", 0.92, 2*time.Hour), windowIn("7d", 0.3, 4*day)), reserving)
	// The session, at 1.2 an hour lately, 85% used, runs out 7m30s after it
	// was read.
	pressed := statustest.Routed{Account: readAt("work", windowIn("5h", 0.85, 2*time.Hour)), Rate: 1.2}
	// The session reset an hour ago, so lapsed till a request starts it.
	lapsed := readAt("work", windowIn("5h", 0.3, -time.Hour), windowIn("7d", 0.3, 4*day))
	// The session, five minutes in, too new to say where it's heading.
	fresh := readAt("work", windowIn("5h", 0.01, 4*time.Hour+55*time.Minute), windowIn("7d", 0.3, 4*day))
	nothing := []roomAt{{after: -time.Minute}, {}, {after: time.Hour}, {after: 5 * day}}
	tests := []struct {
		name    string
		account statustest.Routed
		pinned  []string
		later   time.Duration
		key     string
		want    []roomAt
	}{
		{name: "running out before its reset", account: statustest.Routed{Account: runningOut}, key: "7d", want: []roomAt{
			{after: -time.Minute},
			{room: 0.4, known: true},
			{after: day, room: 0.2, known: true},
			{after: 2 * day, room: 0, known: true},
			{after: 3 * day, room: 0, known: true},
			{after: 4 * day, room: 0, known: true},
			{after: 4*day + time.Second, room: 1, known: true},
		}},
		{name: "running out at its cap, its reserve holding", account: statustest.Routed{Account: reserved}, key: "7d", want: []roomAt{
			{room: 0.4, known: true},
			{after: 18 * time.Hour, room: 0.25, known: true},
			{after: 36 * time.Hour, room: 0.1, known: true},
			{after: 3 * day, room: 0.1, known: true},
			{after: 4 * day, room: 0.1, known: true},
			{after: 4*day + time.Second, room: 1, known: true},
		}},
		{name: "running out at its limit, a pin spending its reserve", account: statustest.Routed{Account: reserved}, pinned: []string{"work"}, key: "7d", want: []roomAt{
			{room: 0.4, known: true},
			{after: 36 * time.Hour, room: 0.1, known: true},
			{after: 2 * day, room: 0, known: true},
			{after: 3 * day, room: 0, known: true},
		}},
		{name: "lasting to its reset", account: statustest.Routed{Account: lasting}, key: "7d", want: []roomAt{
			{after: -time.Minute},
			{room: 0.7, known: true},
			{after: 2 * day, room: 0.5, known: true},
			{after: 4 * day, room: 0.3, known: true},
			{after: 4*day + time.Second, room: 1, known: true},
		}},
		{name: "held by its limit till its reset", account: statustest.Routed{Account: spent}, key: "5h", want: []roomAt{
			{room: 0, known: true},
			{after: time.Hour, room: 0, known: true},
			{after: 2 * time.Hour, room: 0, known: true},
			{after: 2*time.Hour + time.Second, room: 1, known: true},
		}},
		{name: "held by a limit lifting before its reset", account: statustest.Routed{Account: unnamed}, key: "7d", want: []roomAt{
			{room: 0, known: true},
			{after: time.Hour, room: 0, known: true},
			{after: time.Hour + time.Second, room: 0.4, known: true},
			{after: 4 * day, room: 0.4, known: true},
			{after: 4*day + time.Second, room: 1, known: true},
		}},
		{name: "at its cap", account: statustest.Routed{Account: capped}, key: "5h", want: []roomAt{
			{room: 0.1, known: true},
			{after: time.Hour, room: 0.1, known: true},
			{after: 2 * time.Hour, room: 0.1, known: true},
			{after: 2*time.Hour + time.Second, room: 1, known: true},
		}},
		{name: "from the room now, a minute after it was read", account: pressed, later: time.Minute, key: "5h", want: []roomAt{
			{room: 0.15, known: true},
			{after: 6*time.Minute + 30*time.Second, room: 0, known: true},
			{after: time.Hour, room: 0, known: true},
			{after: 2 * time.Hour, room: 1, known: true},
		}},
		{name: "not started", account: statustest.Routed{Account: lapsed}, key: "5h", want: nothing},
		{name: "too new to say where it's heading", account: statustest.Routed{Account: fresh}, key: "5h", want: nothing},
		{name: "stale", account: statustest.Routed{Account: with(runningOut, func(a *status.Account) { a.Error = "HTTP 529 · Overloaded" })}, key: "7d", want: nothing},
		{name: "its token refused", account: statustest.Routed{Account: with(runningOut, func(a *status.Account) {
			a.Refused = status.Refusal{Until: built.Add(time.Hour), Status: 401}
		})}, key: "7d", want: nothing},
		{name: "without a usable token", account: statustest.Routed{Account: with(runningOut, func(a *status.Account) { a.TokenSet = false })}, key: "7d", want: nothing},
		{name: "nothing read", account: statustest.Routed{Account: with(runningOut, func(a *status.Account) { a.FetchedAt = time.Time{} })}, key: "7d", want: nothing},
		{name: "not reported", account: statustest.Routed{Account: runningOut}, key: "7d_oi", want: nothing},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := built.Add(tt.later)
			a, _ := statustest.Router(policy, tt.pinned, tt.account).WorkedOut(policy).At(now, policy).Account("work")
			for _, w := range tt.want {
				room, known := a.RoomAhead(tt.key, now.Add(w.after))
				if known != w.known || math.Abs(room-w.room) > 1e-9 {
					t.Errorf("RoomAhead(%s, now%+v) = %v, %v; want %v, %v", tt.key, w.after, room, known, w.room, w.known)
				}
			}
		})
	}
}

// At every hard moment, the room ahead agrees with the moment's own
// projection: the room now at Now, its floor where it runs out, at Out.At,
// and the room Projected leaves at its reset where it lasts; all of it past
// its reset; and something to draw just where the window has a course ahead.
func TestTheRoomAheadAgreesWithTheMomentsProjection(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, policy)
			for _, a := range m.Accounts {
				for _, w := range a.Windows {
					checkRoomAhead(t, m, a, w)
				}
			}
		})
	}
}

// checkRoomAhead checks the room ahead of account a's window w, at moment m,
// against the moment's projection of it.
func checkRoomAhead(t *testing.T, m status.Moment, a status.AccountNow, w status.WindowNow) {
	t.Helper()
	runs, lasts := w.Kind == status.WindowRunsOut, w.Kind == status.WindowOpen && w.Projected != nil
	drawn := runs || lasts || w.Kind == status.WindowCapped || w.Kind == status.WindowLimited
	expect := func(at time.Time, want float64, what string) {
		t.Helper()
		if got, ok := a.RoomAhead(w.Key, at); !ok || math.Abs(got-want) > 1e-9 {
			t.Errorf("%s's %s has %v, %v %s; want %v", a.ID, w.Key, got, ok, what, want)
		}
	}
	if _, ok := a.RoomAhead(w.Key, m.Now); ok != drawn {
		t.Fatalf("%s's %s, of kind %d, has its room ahead known %v, want %v", a.ID, w.Key, w.Kind, ok, drawn)
	}
	if runs || lasts {
		expect(m.Now, 1-w.Used, "now")
	}
	if runs {
		expect(w.Out.At, 1-a.Floor, "where it runs out")
		expect(w.Next, 1-a.Floor, "at its reset")
	}
	if lasts {
		expect(w.Next, 1-*w.Projected, "at its reset")
	}
	if drawn && !w.Next.IsZero() && !w.Back.After(w.Next) {
		expect(w.Next.Add(time.Second), 1, "past its reset")
	}
	if _, ok := a.RoomAhead(w.Key, m.Now.Add(-time.Second)); ok {
		t.Errorf("%s's %s has room known before now", a.ID, w.Key)
	}
}
