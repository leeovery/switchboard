package status

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

func TestAnAccountsWindowsStandOnTheHoldWorkedOutOnceForThemAll(t *testing.T) {
	now := time.Date(2026, 10, 1, 14, 42, 7, 0, time.UTC)
	spent := Account{
		ID: "work", TokenSet: true, FetchedAt: now.Add(-4 * time.Second),
		Windows: []quota.Window{
			{Key: "5h", Utilization: 1, ResetsAt: now.Add(2 * time.Hour)},
			{Key: "7d", Utilization: 0.3, ResetsAt: now.Add(4 * 24 * time.Hour)},
		},
	}
	states := Document{GeneratedAt: now, Accounts: []Account{spent}}.WindowStatesOf(spent, now, score.Policy{Started: "5h"})
	if got := states.Of("5h").Kind; got != WindowLimited {
		t.Fatalf("a window read spent stands as kind %d, want WindowLimited", got)
	}

	states.stood.held, states.stood.limited = Hold{}, false
	for _, key := range []string{"5h", "7d"} {
		if got := states.Of(key).Kind; got == WindowLimited {
			t.Errorf("%s stands limited by a hold of its own, want it to stand on its account's, worked out once for every window: none", key)
		}
	}
}
