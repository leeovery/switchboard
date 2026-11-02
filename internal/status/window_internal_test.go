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
	states := Document{GeneratedAt: now, Accounts: []Account{spent}}.windowStatesOf(spent, now, score.Policy{Started: "5h"})
	kindOf := func(key string) WindowKind {
		w, _ := spent.Window(key)
		kind, _ := states.kindOf(states.judge(w))
		return kind
	}
	if got := kindOf("5h"); got != WindowLimited {
		t.Fatalf("a window read spent stands as kind %d, want WindowLimited", got)
	}

	states.stood.held, states.stood.limited = Hold{}, false
	for _, key := range []string{"5h", "7d"} {
		if got := kindOf(key); got == WindowLimited {
			t.Errorf("%s stands limited by a hold of its own, want it to stand on its account's, worked out once for every window: none", key)
		}
	}
}

// A layout asks a held moment which accounts have a window without working
// the moment out.
func TestAHeldMomentSaysWhichAccountsHaveAWindowWithoutWorkingTheMomentOut(t *testing.T) {
	now := time.Date(2026, 10, 1, 14, 42, 7, 0, time.UTC)
	week := quota.Window{Key: "7d", Utilization: 0.3, ResetsAt: now.Add(4 * 24 * time.Hour)}
	doc := Document{GeneratedAt: now, Accounts: []Account{
		{ID: "work", TokenSet: true, FetchedAt: now, Windows: []quota.Window{{Key: "5h", Utilization: 0.2, ResetsAt: now.Add(time.Hour)}, week}},
		{ID: "ultra", TokenSet: true, FetchedAt: now, Windows: []quota.Window{week}},
	}}
	held := doc.HoldAt(now, score.Policy{Shared: []string{"5h", "7d"}, Started: "5h"})
	worked := 0
	moment := held.moment
	held.moment = func() Moment { worked++; return moment() }

	if !held.Has("5h", "work") || held.Has("5h", "ultra") {
		t.Errorf("Has(5h) = %v of work, %v of ultra; want true, false", held.Has("5h", "work"), held.Has("5h", "ultra"))
	}
	if worked != 0 {
		t.Errorf("asking which accounts have a window worked the moment out %d times, want none", worked)
	}
}
