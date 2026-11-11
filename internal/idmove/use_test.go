package idmove_test

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		{name: "and a request line moved from a new id, going out on none", put: func(t *testing.T, f fixture) {
			appendDay(t, f.requestLines(), today, line(today, "r8", "", status.ReasonNoRoom, workID, nil))
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
		{name: "the primary alone, the state file naming its new id", configure: primaryAlone, state: adopted,
			orgs: func() *organizations {
				o := answering()
				o.silent = map[string]bool{workToken: true}
				return o
			}},
		// Work's token file may be the former id of the account kept.
		{name: "the primary alone, its token given here, the API silent for work's", configure: primaryAlone,
			tokens: map[string]string{workID: "test-token-given"},
			orgs: func() *organizations {
				o := answering()
				o.of["test-token-given"] = workOrganization
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
// own, not one another Mac moved. Its use is a request going out on it or
// passing it over, a reading of its windows, or a day's summary naming it,
// as one does whose day's lines are gone.
func TestATokenFileJoinsNoAccountThisMacUsed(t *testing.T) {
	for _, tc := range []struct {
		name string
		use  func(t *testing.T, f fixture)
	}{
		{name: "a request going out on it", use: func(t *testing.T, f fixture) {
			appendDay(t, f.requestLines(), today, line(today, "r8", workID, status.ReasonSticky, "", nil))
		}},
		{name: "a request passing it over alone", use: func(t *testing.T, f fixture) {
			appendDay(t, f.requestLines(), today, line(today, "r8", personal, status.ReasonSticky, "", []ledger.Tried{{Account: workID, Why: status.WhyLimit}}))
		}},
		{name: "a reading of its windows alone", use: func(t *testing.T, f fixture) {
			appendDay(t, readingsFiles(f.state), today, reading(today, workID, 0.1))
		}},
		{name: "a day's summary alone, its lines gone", use: func(t *testing.T, f fixture) {
			const pruned = "2026-11-01"
			appendDay(t, f.requestLines(), pruned, line(pruned, "r8", workID, status.ReasonSticky, "", nil))
			ledger.Open(ledger.Dir(f.state), dayfile.Forever, clock, ledger.HistoryIn(readingsFiles(f.state)), fixtureCaps, logs.For("test")).SummariseEnded(now)
			for _, path := range []string{filepath.Join(f.requestLines().Dir, "requests-"+pruned+".jsonl"), f.requestLines().CompressedPath(pruned)} {
				if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
					t.Fatal(err)
				}
			}
			if summary := readFile(t, ledger.SummaryFile(ledger.Dir(f.state), pruned)); !strings.Contains(summary, `"`+workID+`"`) {
				t.Fatalf("the day's summary reads %s, want it naming %s", summary, workID)
			}
		}},
		{name: "a request going out on it, in another case", use: func(t *testing.T, f fixture) {
			appendDay(t, f.requestLines(), today, line(today, "r8", strings.ToUpper(workID), status.ReasonSticky, "", nil))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			writeFile(t, f.config, "[[account]]\nid = \"personal\"\n\n[[account]]\nid = \""+workID+"\"\n")
			tc.use(t, f)
			f.runBounded(t, answering())

			record := f.record(t)
			if e, ok := record.ByWas(work); !ok || e.ID == workID || e.Removed.IsZero() {
				t.Errorf("work's token file went into the record as %+v, want work removed, joining nothing", e)
			}
			if e, ok := record.ByWas(workID); !ok || e.ID == workID || e.Organization != "" || !identity.WellFormed(e.ID) {
				t.Errorf("%s moved onto %+v, want a random id, as one without a token", workID, e)
			}
		})
	}
}

// primaryAlone is the config another Mac moved, of its primary alone.
const primaryAlone = "[[account]]\nid = \"" + workID + "\"\nlabel = \"Work\"\nprimary = true\n"

// A second Mac whose user gave an account another Mac moved a token before
// this one moved, as the shared config shows it with none, keeps its id, as
// its token's organization spells it, however this Mac used it since: the
// config isn't rewritten to an id the other Mac doesn't have. The token file
// by its old id, of the same organization, is its former id, as on the other
// Mac, so its history moves under the account's id, its token file left
// beside the account's own. Personal's joins the account another Mac moved
// it onto, where it's configured; where it isn't, no configured account
// holds its organization, so it's an account removed before ids were made.
func TestAnAccountAnotherMacMovedGivenATokenHereKeepsItsIDAndItsHistory(t *testing.T) {
	const given = "test-token-given"
	for _, tc := range []struct {
		name      string
		configure string
		// record are the accounts the record holds already.
		record []identity.Entry
		use    func(t *testing.T, f fixture)
		// personalJoins is set where personal's token file joins the
		// account another Mac moved it onto.
		personalJoins bool
	}{
		{name: "both accounts configured, unused here", configure: movedConfig, personalJoins: true},
		{name: "both accounts configured, used here", configure: movedConfig, personalJoins: true, use: func(t *testing.T, f fixture) {
			appendDay(t, f.requestLines(), today, line(today, "r8", workID, status.ReasonSticky, "", nil))
			appendDay(t, readingsFiles(f.state), today, reading(today, workID, 0.1))
		}},
		{name: "the primary alone", configure: primaryAlone},
		{name: "the primary alone, the record holding it already", configure: primaryAlone,
			record: []identity.Entry{{ID: workID, Organization: workOrganization, Added: now}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			writeFile(t, f.config, tc.configure)
			if tc.record != nil {
				putRecord(t, f.state, tc.record)
			}
			f.token(t, workID, given)
			if tc.use != nil {
				tc.use(t, f)
			}
			o := answering()
			o.of[given] = workOrganization
			m := f.mover(o)
			m.Routing = f.configuredIDs(t)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			if err := m.Run(ctx); err != nil {
				t.Fatalf("Run() beside a router on the config's ids = %v, want the move done", err)
			}

			if got := readFile(t, f.config); got != tc.configure {
				t.Errorf("the config reads\n%s\nwant it as the other Mac moved it\n%s", got, tc.configure)
			}
			record := f.record(t)
			if e, ok := record.ByWas(work); !ok || e.ID != workID || e.Organization != workOrganization || !e.Removed.IsZero() {
				t.Errorf("work's token file went into the record as %+v, want %s's former id, as the other Mac has it", e, workID)
			}
			for _, e := range record.Accounts {
				if e.ID != workID && strings.EqualFold(e.Was, work) {
					t.Errorf("the record holds %+v, want work as %s alone", e, workID)
				}
			}
			wantTokens := []string{workID, workID + ".taken", personal, personal + ".taken", work, work + ".taken"}
			if e, ok := record.ByWas(personal); tc.personalJoins {
				wantTokens = []string{workID, workID + ".taken", personalID, personalID + ".taken", work, work + ".taken"}
				if !ok || e.ID != personalID || e.Organization != personalOrganization || !e.Removed.IsZero() {
					t.Errorf("personal's token file joined %+v, want %s, as the other Mac moved it", e, personalID)
				}
			} else if !ok || !identity.WellFormed(e.ID) || e.Organization != "" || e.Removed.IsZero() {
				t.Errorf("personal's token file went into the record as %+v, want personal removed, under a random id, its organization unknown", e)
			}
			if got := readFile(t, filepath.Join(f.requestLines().Dir, "requests-"+today+".jsonl")); strings.Contains(got, `"`+work+`"`) || !strings.Contains(got, `"`+workID+`"`) {
				t.Errorf("today's request lines read\n%s\nwant work's under %s", got, workID)
			}
			if got := tokenNames(t, f); !slices.Equal(got, wantTokens) {
				t.Errorf("the tokens directory holds %q, want %q", got, wantTokens)
			}
		})
	}
}

// An account removed before ids were made that the history spells in two
// cases is one account, as ids are told apart ignoring case: added on the
// first day either spelling tells of, and labelled as the history first
// spells it, its earliest day's readings read first.
func TestAnAccountTheHistorySpellsInTwoCasesIsRemovedOnce(t *testing.T) {
	const earlier = "2026-11-02"
	f := newFixture(t)
	spelled := strings.ToUpper(gone)
	appendDay(t, readingsFiles(f.state), earlier, reading(earlier, spelled, 0.4))
	f.runBounded(t, answering())

	var removed []identity.Entry
	for _, e := range f.record(t).Accounts {
		if strings.EqualFold(e.Was, gone) {
			removed = append(removed, e)
		}
	}
	added, _, _ := dayfile.Day(earlier)
	if len(removed) != 1 || removed[0].Label != spelled || !removed[0].Added.Equal(added) || removed[0].Removed.IsZero() {
		t.Errorf("the record holds %+v of %s, want one account, removed, labelled %q, added at %s", removed, gone, spelled, added)
	}
}

// A Mac's own move, of its configured accounts' tokens alone, asks of each of
// them once, and of nothing else.
func TestAMoveOfAMacsOwnAccountsAsksOfTheirTokensAlone(t *testing.T) {
	f := newFixture(t)
	o := answering()
	f.runBounded(t, o)

	if o.asked != 2 || o.askedFor[workToken] != 1 || o.askedFor[personalToken] != 1 {
		t.Errorf("the API was asked %d times, %v, want once for each configured account's token", o.asked, o.askedFor)
	}
}

// A token file whose organization a removed account alone holds is no
// account's former id, as only a configured account takes one: the history
// naming it, it's an account removed before ids were made, apart.
func TestATokenFileOfARemovedAccountsOrganizationIsNoFormerID(t *testing.T) {
	const given, side, sideToken, sideOrganization, removedID = "test-token-given", "side", "test-token-side", "test-org-side", "k3f9"
	f := newFixture(t)
	writeFile(t, f.config, primaryAlone)
	putRecord(t, f.state, []identity.Entry{{ID: removedID, Organization: sideOrganization, Label: side, Added: now.Add(-48 * time.Hour), Removed: now.Add(-24 * time.Hour)}})
	f.token(t, workID, given)
	f.token(t, side, sideToken)
	appendDay(t, f.requestLines(), today, line(today, "r8", side, status.ReasonSticky, "", nil))
	o := answering()
	o.of[given], o.of[sideToken] = workOrganization, sideOrganization
	f.runBounded(t, o)

	record := f.record(t)
	if e, ok := record.ByID(removedID); !ok || e.Was != "" {
		t.Errorf("the removed account is %+v, want it with no former id", e)
	}
	if e, ok := record.ByWas(side); !ok || e.ID == removedID || e.Organization != "" || e.Removed.IsZero() {
		t.Errorf("side's token file went into the record as %+v, want side removed, under a random id, apart", e)
	}
}
