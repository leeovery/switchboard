package capture

import (
	"maps"
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
	worlds, world := worldsNow(), autoWorld()
	_, read, _ := world.readers()
	windows := views.NewWindows(read, world.accounts, dayStart(worlds.now, today-1), worlds.now)
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

// A world's document stands each account's windows as its records' readings
// last read them, however the world was built, so the two can't tell of
// different readings.
func TestTheWorldsDocumentsAgreeWithTheirRecordsReadings(t *testing.T) {
	worlds := worldsNow()
	for name, world := range map[string]accountWorld{
		"auto": worlds.auto(), "pinned": worlds.pinned(), "routing's limit": worlds.limit(), "the bad hour": worlds.badHour(),
		"ten accounts": worlds.many(10), "work alone": worlds.one(),
		"built of its fields alone": {accounts: firstAccounts(3), history: worlds.auto().history},
	} {
		t.Run(name, func(t *testing.T) {
			doc := world.document(worlds.now)
			_, read, _ := worlds.recordsOf(world).readers(func() time.Time { return worlds.now })
			last := make(map[string]map[string]float64)
			for r := range read.Between(time.Time{}, doc.GeneratedAt.Add(time.Nanosecond)) {
				if last[r.Account] == nil {
					last[r.Account] = make(map[string]float64)
				}
				last[r.Account][r.Key] = r.Utilization
			}
			for _, a := range doc.Accounts {
				standing := make(map[string]float64)
				for _, w := range a.Windows {
					standing[w.Key] = w.Utilization
				}
				if len(last[a.ID]) == 0 || !maps.Equal(standing, last[a.ID]) {
					t.Errorf("%s's windows stand at %v, but its records last read them at %v", a.ID, standing, last[a.ID])
				}
			}
		})
	}
}
