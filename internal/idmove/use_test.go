package idmove_test

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/status"
)

// adopted is the state file an earlier build's router keeps, started on the
// config another Mac moved: its primary's token record, by its new id, holds
// the old ids' tokens as former ones of its own.
const adopted = `{"version": 1, "sessions": [], "tokens": {"` + workID + `": {"former": [` +
	`{"sha256": "aa", "replaced_at": "2026-11-08T09:00:00Z", "from": "work"}, ` +
	`{"sha256": "bb", "replaced_at": "2026-11-08T09:00:00Z", "from": "personal"}]}}}`

// appendDay appends lines to the plain file of the day with the given date of
// files.
func appendDay(t *testing.T, files *dayfile.Files, date string, lines ...string) {
	t.Helper()
	file, err := os.OpenFile(filepath.Join(files.Dir, files.Prefix+"-"+date+".jsonl"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := file.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// requestLines are the fixture's request ledger's files.
func (f fixture) requestLines() *dayfile.Files {
	return ledger.Files(ledger.Dir(f.state), logs.For("test"))
}

// A second Mac whose earlier build's router ran on the config another Mac
// moved, before this one upgraded, keeps what only follows that config: its
// state file keeps the old ids' token records under the primary's new id,
// and its events and its request lines' reasons may name the new ids. None
// of it is a use of them here, so the token files by the old ids join them.
func TestWhatOnlyFollowsTheConfigIsNoUseOfAnIDAnotherMacMoved(t *testing.T) {
	for _, tc := range []struct {
		name string
		put  func(t *testing.T, f fixture)
	}{
		{name: "its state file keeping the old ids' tokens under the primary's new id"},
		{name: "and an event naming the new ids", put: func(t *testing.T, f fixture) {
			putDay(t, events.Files(ledger.Dir(f.state), logs.For("test")), yesterday, false,
				event(yesterday, 1, status.Event{Kind: status.EventPin, Account: workID, Accounts: []string{workID, personalID}}))
		}},
		{name: "and a request line whose reason names a new id", put: func(t *testing.T, f fixture) {
			appendDay(t, f.requestLines(), today, line(today, "r8", personal, status.ReasonPinYields+workID+" "+status.WhyReserve, "", nil))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			writeFile(t, f.config, movedConfig)
			writeFile(t, filepath.Join(f.state, "state.json"), adopted)
			if tc.put != nil {
				tc.put(t, f)
			}
			f.runBounded(t, answering())

			record := f.record(t)
			for was, want := range map[string]identity.Entry{
				work:     {ID: workID, Organization: workOrganization},
				personal: {ID: personalID, Organization: personalOrganization},
			} {
				if e, ok := record.ByWas(was); !ok || e.ID != want.ID || e.Organization != want.Organization || !e.Removed.IsZero() {
					t.Errorf("%s's token file joined %+v, want %s, its organization's, as the other Mac moved it", was, e, want.ID)
				}
			}
			for _, e := range record.Accounts {
				if e.Removed.IsZero() && e.ID != workID && e.ID != personalID {
					t.Errorf("the record holds %+v, want the accounts the other Mac moved alone", e)
				}
			}
		})
	}
}

// Where a token file the API didn't answer for may join an account another
// Mac moved, the move waits for it, whatever only follows the config names.
func TestTheMoveWaitsForATokenFileThatMayJoinAnAccountAnotherMacMoved(t *testing.T) {
	const copied = "test-token-copy"
	for _, tc := range []struct {
		name      string
		configure string
		state     string
		tokens    map[string]string
		orgs      func() *organizations
	}{
		{name: "the config another Mac moved, the state file naming the primary's new id", configure: movedConfig, state: adopted,
			orgs: func() *organizations {
				o := answering()
				o.silent = map[string]bool{workToken: true}
				return o
			}},
		{name: "the primary alone, the state file naming its new id", configure: "[[account]]\nid = \"" + workID + "\"\nlabel = \"Work\"\nprimary = true\n", state: adopted,
			orgs: func() *organizations {
				o := answering()
				o.silent = map[string]bool{workToken: true}
				return o
			}},
		// Later's token takes the longer id; a copy of it by another name
		// joins nothing, its organization held, so the shorter waits on
		// the token the API didn't answer for.
		{name: "a copy of a configured account's token, the API silent for another",
			configure: fixtureConfig + "\n[[account]]\nid = \"later\"\n\n[[account]]\nid = \"" + clashingID + "\"\n\n[[account]]\nid = \"" + clashingLaterID + "\"\n",
			tokens:    map[string]string{"later": "test-token-later", "copy": copied, "spare": spareToken},
			orgs: func() *organizations {
				o := answering()
				o.of["test-token-later"], o.of[copied] = clashingLater, clashingLater
				o.silent = map[string]bool{spareToken: true}
				return o
			}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			writeFile(t, f.config, tc.configure)
			if tc.state != "" {
				writeFile(t, filepath.Join(f.state, "state.json"), tc.state)
			}
			for id, token := range tc.tokens {
				f.token(t, id, token)
			}
			before := f.snapshot(t)
			ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
			defer cancel()
			if err := f.mover(tc.orgs()).Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("Run() = %v, want it waiting for the API till its context ended", err)
			}
			if got := f.snapshot(t); !maps.Equal(got, before) {
				t.Errorf("waiting on the API, the install changed:\n%s", differences(before, got))
			}
		})
	}
}

// A token file of the organization a configured account's token holds joins
// no account, though its organization spells another's id.
func TestATokenFileOfAnOrganizationAConfiguredAccountsTokenHoldsJoinsNothing(t *testing.T) {
	const later, copied = "test-token-later", "test-token-copy"
	f := newFixture(t)
	writeFile(t, f.config, fixtureConfig+"\n[[account]]\nid = \"later\"\n\n[[account]]\nid = \""+clashingID+"\"\n\n[[account]]\nid = \""+clashingLaterID+"\"\n")
	f.token(t, "later", later)
	f.token(t, "copy", copied)
	o := answering()
	o.of[later], o.of[copied] = clashingLater, clashingLater
	f.runBounded(t, o)

	record := f.record(t)
	if e, ok := record.ByWas("later"); !ok || e.ID != clashingLaterID || e.Organization != clashingLater {
		t.Errorf("later moved onto %+v, want %s, its organization's", e, clashingLaterID)
	}
	if e, ok := record.ByWas("copy"); ok {
		t.Errorf("the copy of later's token joined %+v, want it passed over", e)
	}
	if e, ok := record.ByID(clashingID); !ok || e.Organization != "" {
		t.Errorf("%s moved onto %+v, want its own id, its organization unknown", clashingID, e)
	}
}

// A token file joins no configured account this Mac used, though its
// organization spells the account's id: one by that id here is this Mac's
// own, not one another Mac moved.
func TestATokenFileJoinsNoAccountThisMacUsed(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.config, "[[account]]\nid = \"personal\"\n\n[[account]]\nid = \""+workID+"\"\n")
	appendDay(t, f.requestLines(), today, line(today, "r8", workID, status.ReasonSticky, "", nil))
	f.runBounded(t, answering())

	record := f.record(t)
	if e, ok := record.ByWas(work); !ok || e.ID == workID || e.Removed.IsZero() {
		t.Errorf("work's token file went into the record as %+v, want work removed, joining nothing", e)
	}
	if e, ok := record.ByWas(workID); !ok || e.ID == workID || e.Organization != "" || !identity.WellFormed(e.ID) {
		t.Errorf("%s moved onto %+v, want a random id, as one without a token", workID, e)
	}
}
