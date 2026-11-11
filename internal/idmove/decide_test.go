package idmove_test

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
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
	for _, tc := range []struct {
		name      string
		configure string
		orgs      func() *organizations
		wantAsked int
	}{
		{"every configured account with its token, the stray never asked about", fixtureConfig, answering, 0},
		{"a configured account without one, the API silent for the stray", fixtureConfig + "\n[[account]]\nid = \"side-1\"\n", func() *organizations {
			o := answering()
			o.silent = map[string]bool{spareToken: true}
			return o
		}, 1},
		{"a configured account without one, the stray's file unreadable", fixtureConfig + "\n[[account]]\nid = \"side-1\"\n", answering, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			writeFile(t, f.config, tc.configure)
			f.token(t, "spare", spareToken)
			if strings.Contains(tc.name, "unreadable") {
				writeFile(t, f.tokens.Path("spare"), "")
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
