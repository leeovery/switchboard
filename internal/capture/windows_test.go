package capture

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/views"
)

// readingOf is the reading of the account's window read at the time given,
// as Windows gives the readings history.
func readingOf(t *testing.T, w views.Windows, account, key string, at time.Time) views.WindowReading {
	t.Helper()
	i := slices.IndexFunc(w.Windows, func(r views.WindowReadings) bool { return r.Account == account && r.Window == key })
	if i < 0 {
		t.Fatalf("no readings of %s's %s", account, key)
	}
	readings := w.Windows[i].Readings
	j := slices.IndexFunc(readings, func(r views.WindowReading) bool { return r.At.Equal(at) })
	if j < 0 {
		t.Fatalf("no reading of %s's %s at %s", account, key, at)
	}
	return readings[j]
}

func TestTheReadingsAreTheWindowsTheFramesTellOf(t *testing.T) {
	worlds := worldsNow()
	world := worlds.auto()
	windows := views.NewWindows(world.history, world.accounts, dayStart(worlds.now, today-1), worlds.now)
	at := func(d, hour, minute, second int) time.Time { return dayAt(worlds.now, d, hour, minute, second).UTC() }
	tests := []struct {
		name         string
		account, key string
		at           time.Time
		use          float64
		status       quota.Status
	}{
		{"work at its cap at 14:58 yesterday", "work", sessionKey, at(today-1, 14, 58, 0), 0.95, quota.StatusAllowed},
		{"work at its limit at 15:02 yesterday", "work", sessionKey, at(today-1, 15, 2, 0), 1, quota.StatusRejected},
		{"personal at its limit at 19:12:40 yesterday", "personal", sessionKey, at(today-1, 19, 12, 40), 1, quota.StatusRejected},
		{"work at its cap at 14:31:03 today", "work", sessionKey, at(today, 14, 31, 3), 0.95, quota.StatusAllowed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := readingOf(t, windows, tt.account, tt.key, tt.at)
			if r.Utilization != tt.use || r.Status != tt.status {
				t.Errorf("the reading is %v, %s, want %v, %s", r.Utilization, r.Status, tt.use, tt.status)
			}
		})
	}
}

func TestTheWorldsDocumentsAgreeWithTheirReadings(t *testing.T) {
	worlds := worldsNow()
	for name, world := range map[string]accountWorld{
		"auto": worlds.auto(), "pinned": worlds.pinned(), "routing's limit": worlds.limit(), "the bad hour": worlds.badHour(),
		"ten accounts": worlds.many(10), "work alone": worlds.one(),
	} {
		t.Run(name, func(t *testing.T) {
			doc := world.document(worlds.now)
			for _, a := range doc.Accounts {
				for _, w := range a.Windows {
					var last float64
					for r := range world.history.Between(time.Time{}, doc.GeneratedAt.Add(time.Nanosecond)) {
						if r.Account == a.ID && r.Key == w.Key {
							last = r.Utilization
						}
					}
					if last != w.Utilization {
						t.Errorf("%s's %s stands at %v, but was last read at %v", a.ID, w.Key, w.Utilization, last)
					}
				}
			}
		})
	}
}
