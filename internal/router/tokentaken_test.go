package router_test

import (
	"os"
	"testing"
	"time"
)

func TestTheRoutersDocumentGivesWhenEachTokenWasTaken(t *testing.T) {
	taken := now.Add(-90 * 24 * time.Hour)
	store, files := withTokenFiles(t)
	writeTokenTaken(t, store, "work", workToken, taken)
	// Side's file replaced by something else, its record of the token before,
	// and personal's written by hand, with no record.
	if err := os.WriteFile(store.Path("side"), []byte("test-token-side-renewed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.Path("personal"), []byte(personalToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := testConfig("http://127.0.0.1:1")
	files(&cfg)

	doc := newRouterFrom(t, cfg).Status()
	want := map[string][2]time.Time{"work": {taken, taken.Add(365 * 24 * time.Hour)}}
	for _, a := range doc.Accounts {
		if !a.TokenSet {
			t.Errorf("%s has no usable token: %s", a.ID, a.Error)
		}
		got := [2]time.Time{a.TokenTaken, a.TokenGoodUntil}
		if !got[0].Equal(want[a.ID][0]) || !got[1].Equal(want[a.ID][1]) {
			t.Errorf("%s's token was taken at %v, good until %v; want %v", a.ID, got[0], got[1], want[a.ID])
		}
	}
}
