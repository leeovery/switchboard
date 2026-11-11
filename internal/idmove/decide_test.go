package idmove_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// spareToken is the token of a token file of an account neither configured
// nor known.
const spareToken = "test-token-spare"

// runBounded runs the move of the fixture's install to its end, failing the
// test where it fails, or hasn't ended in a few seconds, as a move that
// waits forever would.
func (f fixture) runBounded(t *testing.T, o idmove.Organizations) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := f.mover(o).Run(ctx); err != nil {
		t.Fatalf("Run() = %v, want the move done", err)
	}
}

func TestAStrayTokenFileHoldsTheMoveUpNowhere(t *testing.T) {
	silentForTheStray := func() *organizations {
		o := answering()
		o.silent = map[string]bool{spareToken: true}
		return o
	}
	for _, tc := range []struct {
		name      string
		configure string
		orgs      func() *organizations
		// spoil, where it's set, is the token file spoilt, emptied.
		spoil     string
		wantAsked int
		// wantTaken are the ids the tokens of accounts take, by the
		// accounts' old ids, as their organizations spell them.
		wantTaken map[string]string
	}{
		{name: "every configured account with its token, the stray never asked about", configure: fixtureConfig, orgs: answering},
		{name: "a configured account without one, the API silent for the stray", configure: fixtureConfig + "\n[[account]]\nid = \"side-1\"\n", orgs: silentForTheStray, wantAsked: 1},
		{name: "a configured account without one, the stray's file unreadable", configure: fixtureConfig + "\n[[account]]\nid = \"side-1\"\n", orgs: answering, spoil: "spare"},
		// Work's id is one switchboard could make, but the history here names
		// it, so it's no account another Mac moved.
		{name: "a configured account without one, by an id the history names, the API silent for the stray", configure: fixtureConfig, orgs: silentForTheStray, spoil: work, wantAsked: 1},
		{name: "the accounts another Mac moved, each joined by a token file the API answered for, the API silent for the stray", configure: movedConfig, orgs: silentForTheStray, wantAsked: 1},
		// Work's token takes the id the config names it by too, as its
		// organization spells it.
		{name: "an account configured by the id another Mac moved it onto too, its token here answered for, the API silent for the stray", configure: fixtureConfig + "\n[[account]]\nid = \"" + workID + "\"\n", orgs: silentForTheStray, wantAsked: 1,
			wantTaken: map[string]string{work: workID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			writeFile(t, f.config, tc.configure)
			f.token(t, "spare", spareToken)
			if tc.spoil != "" {
				writeFile(t, f.tokens.Path(tc.spoil), "")
			}
			o := tc.orgs()
			f.runBounded(t, o)

			if got := o.askedFor[spareToken]; got != tc.wantAsked {
				t.Errorf("the API was asked about the stray token %d times, want %d", got, tc.wantAsked)
			}
			if _, ok := f.record(t).ByWas("spare"); ok {
				t.Error("the stray token file's account went into the record, want it passed over")
			}
			if _, err := os.Lstat(f.tokens.Path("spare")); err != nil {
				t.Errorf("the stray token file = %v, want it left as it was", err)
			}
			if e, ok := f.record(t).ByWas(personal); !ok || e.ID != personalID {
				t.Errorf("personal moved onto %+v, want %s", e, personalID)
			}
			for was, id := range tc.wantTaken {
				if e, ok := f.record(t).ByWas(was); !ok || e.ID != id || e.Organization == "" {
					t.Errorf("%s moved onto %+v, want %s, which its token's organization spells", was, e, id)
				}
			}
		})
	}
}

func TestAnUnusableTokenFileMovesItsAccountAsOneWithoutAToken(t *testing.T) {
	for _, tc := range []struct {
		name  string
		spoil func(t *testing.T, path string)
	}{
		{"empty", func(t *testing.T, path string) { writeFile(t, path, "") }},
		{"holding more than a token", func(t *testing.T, path string) { writeFile(t, path, personalToken+" "+workToken+"\n") }},
		{"a link leading nowhere", func(t *testing.T, path string) {
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("nowhere", path); err != nil {
				t.Fatal(err)
			}
		}},
		{"others able to read it", func(t *testing.T, path string) {
			if err := os.Chmod(path, 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			tc.spoil(t, f.tokens.Path(personal))
			o := answering()
			f.runBounded(t, o)

			e, ok := f.record(t).ByWas(personal)
			if !ok || e.ID == personalID || e.Organization != "" || !identity.WellFormed(e.ID) {
				t.Errorf("personal moved onto %+v, want a random id, its organization unknown, as one without a token", e)
			}
			if _, err := os.Lstat(f.tokens.Path(e.ID)); err != nil {
				t.Errorf("personal's token file by its new id = %v, want it moved there, as it was, to be put right", err)
			}
			if o.askedFor[personalToken] != 0 {
				t.Errorf("the API was asked about personal's token %d times, want none, its file unusable", o.askedFor[personalToken])
			}
		})
	}
}

func TestATokenFileUnreadableForAMomentIsLookedAtAgain(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.tokens.Path(personal), "")
	m := f.mover(answering())
	looked := 0
	m.OnLookingAgain(func(id string) {
		looked++
		if id == personal {
			writeFile(t, f.tokens.Path(personal), personalToken+"\n")
		}
	})
	if err := m.Run(t.Context()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if e, ok := f.record(t).ByWas(personal); !ok || e.ID != personalID || e.Organization != personalOrganization {
		t.Errorf("personal moved onto %+v, want %s, its token read as it was looked at again", e, personalID)
	}
	if looked != 1 {
		t.Errorf("the move looked again at a token %d times, want once, at personal's alone", looked)
	}
}

func TestARefusalCountsOnlyWhereItRepeats(t *testing.T) {
	f := newFixture(t)
	o := answering()
	o.refused = map[string]int{personalToken: 1}
	f.runBounded(t, o)
	if e, ok := f.record(t).ByWas(personal); !ok || e.ID != personalID || e.Organization != personalOrganization {
		t.Errorf("personal, refused once then answered, moved onto %+v, want %s, its organization's", e, personalID)
	}
	if got := o.askedFor[personalToken]; got != 2 {
		t.Errorf("the API was asked about personal's token %d times, want twice", got)
	}
}

func TestALabelMadeOfAnOldIDIsOneNoOtherAccountShows(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.config, `[[account]]
id      = "work"
label   = "PERSONAL"
primary = true

[[account]]
id = "personal"

[[account]]
id    = "qgr3"
label = "personal 2"
`)
	f.runBounded(t, answering())
	want := `[[account]]
id      = "b6s7"
label   = "PERSONAL"
primary = true

[[account]]
id = "qgr2"
label = "personal 3"

[[account]]
id    = "qgr3"
label = "personal 2"
`
	if got := readFile(t, f.config); got != want {
		t.Errorf("the config reads\n%s\nwant\n%s", got, want)
	}
}

// A second Mac upgrading with the config the first moved, its token files
// still by the old ids, and starting before its network's up, waits to
// join them, rather than take its own accounts for removed ones.
func TestASecondMacStartingBeforeItsNetworkWaitsToJoinTheAccountsAnotherMoved(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.config, movedConfig)
	before := f.snapshot(t)
	silent := answering()
	silent.silent = map[string]bool{workToken: true, personalToken: true}
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := f.mover(silent).Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() = %v, want it waiting for the API till its context ended", err)
	}
	if got := f.snapshot(t); !maps.Equal(got, before) {
		t.Errorf("waiting on the API, the install changed:\n%s", differences(before, got))
	}
	err := f.mover(silent).RunBeside(t.Context(), f.historyOf())
	if u, ok := errors.AsType[*idmove.Unanswered](err); !ok || u.Account != personal || !strings.Contains(u.Why, "timed out") {
		t.Errorf("RunBeside() = %v, want it unanswered, naming the token file it waits on and why", err)
	}

	f.runBounded(t, answering())
	for was, id := range map[string]string{work: workID, personal: personalID} {
		if e, ok := f.record(t).ByWas(was); !ok || e.ID != id || !e.Removed.IsZero() {
			t.Errorf("once the API answered, %s moved onto %+v, want %s, joined", was, e, id)
		}
	}
}

// A config merged with another Mac's that moved first, before this one moves,
// names an account by its old id and the new one the other Mac moved it onto:
// the account's token here takes the other Mac's id, which its organization
// spells, so the two Macs agree on it, and the config keeps it alone.
func TestAConfigMergedBeforeTheMoveToNameAnAccountByBothItsIDsHasItTakeTheOtherMacs(t *testing.T) {
	const (
		old   = "[[account]]\nid = \"work\"\nlabel = \"Work\"\nprimary = true\nplan = \"max\"\nreserve = 0.1\n\n"
		other = "[[account]]\nid = \"personal\"\n\n"
		moved = "[[account]]\nid = \"" + workID + "\"\n\n"
		// As the other Mac's move wrote it, the plan and the reserve changed
		// there since.
		movedThere = "[[account]]\nid = \"" + workID + "\"\nlabel = \"Work\"\nplan = \"ultra\"\nreserve = 0.2\n\n"
	)
	wantWork := config.Account{ID: workID, Label: "Work", Primary: true, Plan: "max", Reserve: 0.1}
	wantPersonal := config.Account{ID: personalID, Label: personal}
	for _, tc := range []struct {
		name, configure string
		want            config.Accounts
	}{
		{name: "the new id after the old", configure: old + other + moved, want: config.Accounts{wantPersonal, wantWork}},
		{name: "the new id before the old", configure: moved + old + other, want: config.Accounts{wantWork, wantPersonal}},
		{name: "the new id as the other Mac wrote it", configure: old + other + movedThere,
			want: config.Accounts{wantPersonal, {ID: workID, Label: "Work", Primary: true, Plan: "ultra", Reserve: 0.2}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			writeFile(t, f.config, tc.configure)
			f.runBounded(t, answering())

			if got := f.configured(t); !slices.Equal(got, tc.want) {
				t.Errorf("moved, the config configures %+v, want %+v", got, tc.want)
			}
			record := f.record(t)
			if e, ok := record.ByID(workID); !ok || e.Organization != workOrganization || e.Was != work {
				t.Errorf("the record holds %s as %+v, want work's, its organization known", workID, e)
			}
			var live []string
			for _, e := range record.Accounts {
				if e.Removed.IsZero() {
					live = append(live, e.ID)
				}
			}
			if want := []string{workID, personalID}; !slices.Equal(slices.Sorted(slices.Values(live)), slices.Sorted(slices.Values(want))) {
				t.Errorf("the record holds the accounts %q, want %q, each on its organization's id, once", live, want)
			}
			if _, err := f.tokens.Read(workID); err != nil {
				t.Errorf("moved, %s's token = %v, want it read", workID, err)
			}
		})
	}
}

// Two organizations whose ids clash at four characters: another Mac moved
// the first's account onto its four, and the second's, those held, onto its
// five, which start with them.
const (
	clashing, clashingLater     = "test-org-clash-17", "test-org-clash-616"
	clashingID, clashingLaterID = "q1ie", "q1iew"
)

// A config merged with another Mac's, that names two accounts whose
// organizations' ids clash by their old ids and the new: each account's token
// takes the longest id its organization spells, so neither takes the other's,
// whichever the config names first.
func TestAnAccountTakesTheLongestIDItsOrganizationSpellsOfThoseAnotherMacMoved(t *testing.T) {
	const (
		first  = "[[account]]\nid = \"first\"\n\n"
		later  = "[[account]]\nid = \"later\"\n\n"
		moved  = "[[account]]\nid = \"" + clashingID + "\"\n\n[[account]]\nid = \"" + clashingLaterID + "\"\n"
		tokenA = "test-token-first"
		tokenB = "test-token-later"
	)
	for _, tc := range []struct{ name, configure string }{
		{"the first's old id first", fixtureConfig + "\n" + first + later + moved},
		{"the later's old id first", fixtureConfig + "\n" + later + first + moved},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			writeFile(t, f.config, tc.configure)
			f.token(t, "first", tokenA)
			f.token(t, "later", tokenB)
			o := answering()
			o.of[tokenA], o.of[tokenB] = clashing, clashingLater
			f.runBounded(t, o)

			record := f.record(t)
			for was, want := range map[string]identity.Entry{
				"first": {ID: clashingID, Organization: clashing},
				"later": {ID: clashingLaterID, Organization: clashingLater},
			} {
				if e, ok := record.ByWas(was); !ok || e.ID != want.ID || e.Organization != want.Organization {
					t.Errorf("%s moved onto %+v, want %s, its organization's, as the other Mac moved it", was, e, want.ID)
				}
			}
			if got, want := f.configuredIDs(t), []string{workID, personalID, clashingID, clashingLaterID}; !slices.Equal(got, want) {
				t.Errorf("moved, the config configures %q, want %q", got, want)
			}
		})
	}
}

// A second Mac upgrading with the config another Mac moved, which names two
// accounts whose organizations' ids clash, its token files still by their
// old ids: each token file joins the longest id its organization spells, so
// neither joins the other's, though the later's file is the first asked.
func TestATokenFileJoinsTheLongestIDItsOrganizationSpellsOfThoseAnotherMacMoved(t *testing.T) {
	const tokenA, tokenB = "test-token-alpha", "test-token-beta"
	f := newFixture(t)
	writeFile(t, f.config, movedConfig+"\n[[account]]\nid = \""+clashingID+"\"\n\n[[account]]\nid = \""+clashingLaterID+"\"\n")
	f.token(t, "alpha", tokenA)
	f.token(t, "beta", tokenB)
	o := answering()
	o.of[tokenA], o.of[tokenB] = clashingLater, clashing
	f.runBounded(t, o)

	record := f.record(t)
	for was, want := range map[string]identity.Entry{
		"alpha":  {ID: clashingLaterID, Organization: clashingLater},
		"beta":   {ID: clashingID, Organization: clashing},
		work:     {ID: workID, Organization: workOrganization},
		personal: {ID: personalID, Organization: personalOrganization},
	} {
		if e, ok := record.ByWas(was); !ok || e.ID != want.ID || e.Organization != want.Organization {
			t.Errorf("%s's token file joined %+v, want %s, its organization's, as the other Mac moved it", was, e, want.ID)
		}
	}
}

func TestTheMoveAsksTheAPIAgainAfterWaitsThatDoubleToHalfItsLasts(t *testing.T) {
	for _, tc := range []struct {
		name  string
		mover idmove.Mover
		want  []time.Duration
	}{
		{name: "as it comes", want: []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 5 * time.Minute, 5 * time.Minute}},
		{name: "a shorter Lasts", mover: idmove.Mover{Retry: time.Second, Lasts: 10 * time.Second}, want: []time.Duration{time.Second, 2 * time.Second, 4 * time.Second, 5 * time.Second, 5 * time.Second}},
		{name: "a first wait past half Lasts", mover: idmove.Mover{Retry: time.Minute, Lasts: time.Minute}, want: []time.Duration{30 * time.Second, 30 * time.Second}},
		{name: "a Lasts too short to halve", mover: idmove.Mover{Lasts: time.Nanosecond}, want: []time.Duration{time.Millisecond, time.Millisecond}},
		{name: "a Retry below none", mover: idmove.Mover{Retry: -time.Second}, want: []time.Duration{time.Millisecond, 2 * time.Millisecond}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var waits []time.Duration
			for wait := time.Duration(0); len(waits) < len(tc.want); {
				wait = tc.mover.AskAgain(wait)
				waits = append(waits, wait)
			}
			if !slices.Equal(waits, tc.want) {
				t.Errorf("the move waits %v to ask the API again, want %v", waits, tc.want)
			}
		})
	}
}

func TestARefusalCountsOnlyOnceItHasLasted(t *testing.T) {
	f := newFixture(t)
	o := answering()
	o.refused = map[string]int{personalToken: always}
	m := f.mover(o)
	m.ClockedBy(time.Now)
	m.Lasts = 300 * time.Millisecond
	began := time.Now()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	if err := m.Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() = %v, want it waiting while the refusal hasn't lasted", err)
	}
	if _, ok := f.record(t).ByWas(personal); ok {
		t.Fatal("personal moved on a refusal that hadn't lasted")
	}
	lasted, cancel := context.WithTimeout(t.Context(), 4*m.Lasts)
	defer cancel()
	if err := m.Run(lasted); err != nil {
		t.Fatalf("Run() = %v, want the move done once the refusal had lasted", err)
	}
	if e, ok := f.record(t).ByWas(personal); !ok || e.Organization != "" || e.ID == personalID || time.Since(began) < m.Lasts {
		t.Errorf("personal moved onto %+v after %v, want a random id once the refusal had lasted %v", e, time.Since(began), m.Lasts)
	}
}

// A refusal has lasted only where it's heard throughout: each refusal heard
// within Lasts of the one before, and no answer naming the organization nor a
// token file that can't be read between, though the API not answering may be.
func TestARefusalHasLastedOnlyWhereHeardThroughout(t *testing.T) {
	const (
		refuses  = "refuses"
		names    = "names"
		silent   = "silent"
		unusable = "refuses, then can't be read, then refuses"
	)
	type attempt struct {
		at     time.Duration
		token  string
		counts bool
	}
	for _, tc := range []struct {
		name     string
		attempts []attempt
	}{
		{name: "heard throughout", attempts: []attempt{{0, refuses, false}, {5 * time.Minute, refuses, false}, {10 * time.Minute, refuses, true}}},
		{name: "heard again only past Lasts", attempts: []attempt{{0, refuses, false}, {11 * time.Minute, refuses, false}, {21 * time.Minute, refuses, true}}},
		{name: "the API not answering between", attempts: []attempt{{0, refuses, false}, {5 * time.Minute, silent, false}, {9 * time.Minute, refuses, false}, {10 * time.Minute, refuses, true}}},
		{name: "an answer naming the organization between", attempts: []attempt{{0, refuses, false}, {5 * time.Minute, names, false}, {9 * time.Minute, refuses, false}, {18 * time.Minute, refuses, false}, {19 * time.Minute, refuses, true}}},
		{name: "a token file that can't be read between looks", attempts: []attempt{{0, refuses, false}, {5 * time.Minute, unusable, false}, {14 * time.Minute, refuses, false}, {15 * time.Minute, refuses, true}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			o := answering()
			var at time.Time
			m := f.mover(o)
			m.Lasts = 10 * time.Minute
			m.ClockedBy(func() time.Time { return at })
			var between []string
			m.OnLookingAgain(func(id string) {
				if id == personal && len(between) > 0 {
					writeFile(t, f.tokens.Path(personal), between[0])
					between = between[1:]
				}
			})

			for _, a := range tc.attempts {
				at = now.Add(a.at)
				// Work's token goes unanswered while personal's names its
				// organization, so the move waits on it, rather than end there.
				o.silent = map[string]bool{personalToken: a.token == silent, workToken: a.token == names}
				o.refused = map[string]int{personalToken: always}
				if a.token == names {
					o.refused = nil
				}
				if a.token == unusable {
					between = []string{"", personalToken + "\n"}
				}

				err := m.RunBeside(t.Context(), f.historyOf())
				e, moved := f.record(t).ByWas(personal)
				switch {
				case a.counts && (err != nil || !moved || e.Organization != "" || e.ID == personalID):
					t.Fatalf("at %v, %s: RunBeside() = %v, personal moved onto %+v, want a random id, the refusal having lasted", a.at, a.token, err, e)
				case !a.counts && (!errors.Is(err, idmove.ErrUnanswered) || moved):
					t.Fatalf("at %v, %s: RunBeside() = %v, personal moved onto %+v, want the move waiting, the refusal not having lasted", a.at, a.token, err, e)
				}
			}
		})
	}
}

func TestLooksOfKindsThatDisagreeCountOnlyOnceTwoAgree(t *testing.T) {
	const unusable, refused = "unusable", "refused"
	for _, tc := range []struct {
		looks     []string
		wantAsked int
		wantLog   string
	}{
		{looks: []string{unusable, refused, unusable}, wantAsked: 1},
		{looks: []string{unusable, refused, refused}, wantAsked: 2, wantLog: "have named no organization for a while"},
		{looks: []string{refused, unusable, unusable}, wantAsked: 1, wantLog: "looked at twice, so the move takes it as no token"},
	} {
		t.Run(strings.Join(tc.looks, ", "), func(t *testing.T) {
			log := logstest.Capture(t)
			f := newFixture(t)
			o := answering()
			o.refused = map[string]int{personalToken: always}
			put := func(state string) {
				content := personalToken + "\n"
				if state == unusable {
					content = ""
				}
				writeFile(t, f.tokens.Path(personal), content)
			}
			put(tc.looks[0])
			m := f.mover(o)
			looked := 0
			m.OnLookingAgain(func(id string) {
				if id == personal {
					looked++
					put(tc.looks[looked])
				}
			})

			err := m.RunBeside(t.Context(), f.historyOf())
			if got := o.askedFor[personalToken]; got != tc.wantAsked {
				t.Errorf("the API was asked about personal's token %d times, want %d", got, tc.wantAsked)
			}
			if tc.wantLog == "" {
				if u, ok := errors.AsType[*idmove.Unanswered](err); !ok || u.Account != personal {
					t.Errorf("RunBeside() = %v, want personal unanswered, no two looks agreeing", err)
				}
				return
			}
			if err != nil || !log.Has(tc.wantLog, "account="+personal) {
				t.Errorf("RunBeside() = %v, log reads\n%s\nwant the move done, personal counted as %q says", err, log, tc.wantLog)
			}
		})
	}
}

func TestAnAnswerNamingNoOrganizationCountsAsARefusalDoesButAnAskToAskAgainNever(t *testing.T) {
	for _, tc := range []struct {
		status int
		counts bool
	}{
		{status: 400, counts: true},
		{status: 403, counts: true},
		{status: 404, counts: true},
		{status: 200, counts: true},
		{status: 408},
		{status: 425},
		{status: 429},
		{status: 500},
		{status: 529},
		// No answer of the API's own, as where a proxy between answered.
		{status: 0},
	} {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			f := newFixture(t)
			o := answering()
			o.failing = map[string]error{personalToken: answeredError{status: tc.status}}

			waiting := f.mover(o)
			waiting.Lasts = time.Hour
			err := waiting.RunBeside(t.Context(), f.historyOf())
			if u, ok := errors.AsType[*idmove.Unanswered](err); !ok || u.Account != personal || !strings.Contains(u.Why, fmt.Sprintf("HTTP %d", tc.status)) {
				t.Fatalf("RunBeside() = %v, want personal unanswered, saying why", err)
			}

			ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
			defer cancel()
			err = f.mover(o).Run(ctx)
			e, moved := f.record(t).ByWas(personal)
			switch {
			case tc.counts && (err != nil || !moved || e.Organization != "" || e.ID == personalID):
				t.Errorf("Run() = %v, personal moved onto %+v, want a random id, the answer counted once it lasted", err, e)
			case !tc.counts && (!errors.Is(err, context.DeadlineExceeded) || moved):
				t.Errorf("Run() = %v, personal moved onto %+v, want it waiting on the API, never counting its failure", err, e)
			}
		})
	}
}
