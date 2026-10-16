package status_test

import (
	"math"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// At Now, the room RoomThen gives each account's window, of the readings of
// it as its document read it, is the room the moment has the pool count it
// as having, at every hard moment, but of an account without a usable token:
// the moment counts its windows as read, and Then, as Runway draws it, none.
func TestRoomThenAtNowIsTheRoomThePoolCountsAsTheMomentHasIt(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, policy)
			for i, a := range m.Accounts {
				if !a.TokenSet {
					continue
				}
				for _, w := range a.Windows {
					got, _ := m.RoomThen(a, status.Index(readingsOf(hm.Doc.Accounts[i])), m.Told(a.ID), w.Key, m.Now)
					if math.Abs(got-w.Room) > 1e-9 {
						t.Errorf("RoomThen(%s, %s, Now) = %v, want %v, as the moment has the pool count it", a.ID, w.Key, got, w.Room)
					}
				}
			}
		})
	}
}

// RoomThen is the room an account had left in a window at a moment gone, as
// Then judges how it stood then: an account idle before its window started
// has its whole room till then, its last window long reset; a window read
// spent, or rejected, has none; a hold of every request takes the room of
// the window a request starts, the session, from when it began, but not the
// week's; a refused token takes every window's; one not usable now, too;
// and where nothing was read by then, it isn't known. A model's own window
// counts by its own reading.
func TestTheRoomAnAccountHadLeftInAWindowAtAMomentGone(t *testing.T) {
	// Work keeps a reserve of a tenth, so runs out at 0.9 of each window.
	// Idle since Monday, its 5-hour window then reset at 16:00, its week read
	// then at 0.3; its session now started at 13:00, read at 13:30.
	monday := readOf(at(1, 13, 0).AddDate(0, 0, -3), "5h", 0.6, at(1, 16, 0).AddDate(0, 0, -3))
	week := readOf(at(1, 13, 0).AddDate(0, 0, -3), "7d", 0.3, at(5, 21, 0))
	session := readOf(at(1, 13, 30), "5h", 0.2, at(1, 18, 0))
	idle := []readings.Reading{monday, week, session}
	// Its session read at its cap from 14:10, and rejected at 0.95 at 14:30.
	capped := append(idle[:3:3], readOf(at(1, 14, 10), "5h", 0.92, at(1, 18, 0)))
	rejectedAt := append(capped[:4:4], rejected(readOf(at(1, 14, 30), "5h", 0.95, at(1, 18, 0))))
	// Its week read spent at 14:20.
	weekSpent := append(idle[:3:3], readOf(at(1, 14, 20), "7d", 1, at(5, 21, 0)))
	fable := readOf(at(1, 13, 30), "7d_oi", 0.25, at(5, 21, 0))
	refusal := status.Span{Kind: status.WasRefused, From: at(1, 14, 0), Until: at(1, 15, 0)}

	reserved := workAt(func(d *status.Document) { d.Accounts[0].Reserve = 0.1 })
	tokenless := workAt(func(d *status.Document) { d.Accounts[0] = status.Account{ID: "work", Label: "work"} })
	tests := []struct {
		name   string
		moment status.Moment
		read   []readings.Reading
		told   []status.Span
		key    string
		t      time.Time
		want   float64
		known  bool
	}{
		{name: "idle before its session, its last long reset", moment: reserved, read: idle, key: "5h", t: at(1, 11, 0), want: 0.9, known: true},
		{name: "its session started", moment: reserved, read: idle, key: "5h", t: at(1, 14, 0), want: 0.7, known: true},
		{name: "idle, its week as last read", moment: reserved, read: idle, key: "7d", t: at(1, 11, 0), want: 0.6, known: true},
		{name: "at its cap, its session", moment: reserved, read: capped, key: "5h", t: at(1, 14, 15), want: 0, known: true},
		{name: "at its cap, its week", moment: reserved, read: capped, key: "7d", t: at(1, 14, 15), want: 0.6, known: true},
		{name: "before its cap", moment: reserved, read: capped, key: "5h", t: at(1, 14, 5), want: 0.7, known: true},
		{name: "its session read rejected short of its limit", moment: reserved, read: rejectedAt, key: "5h", t: at(1, 14, 35), want: 0, known: true},
		{name: "its week read spent, its session held by it", moment: reserved, read: weekSpent, key: "5h", t: at(1, 14, 25), want: 0, known: true},
		{name: "its week read spent", moment: reserved, read: weekSpent, key: "7d", t: at(1, 14, 25), want: 0, known: true},
		{name: "its token refused", moment: reserved, read: idle, told: []status.Span{refusal}, key: "7d", t: at(1, 14, 30), want: 0, known: true},
		{name: "its token no longer usable", moment: tokenless, read: idle, key: "5h", t: at(1, 14, 0), want: 0, known: true},
		{name: "nothing read by then", moment: reserved, read: idle, key: "5h", t: at(1, 9, 0).AddDate(0, 0, -3), want: 0, known: false},
		{name: "a model's own week", moment: reserved, read: append(idle[:3:3], fable), key: "7d_oi", t: at(1, 14, 0), want: 0.65, known: true},
		{name: "a model's own week not read", moment: reserved, read: idle, key: "7d_oi", t: at(1, 14, 0), want: 0, known: true},
	}
	for _, tt := range tests {
		a := tt.moment.Accounts[0]
		got, known := tt.moment.RoomThen(a, status.Index(tt.read), tt.told, tt.key, tt.t)
		if math.Abs(got-tt.want) > 1e-9 || known != tt.known {
			t.Errorf("%s: RoomThen(%s, %s) = %v, %v, want %v, %v", tt.name, tt.key, onDay(tt.t), got, known, tt.want, tt.known)
		}
	}
}
