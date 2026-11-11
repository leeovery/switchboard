package identity_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"math/rand/v2"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/identity"
)

// shaped is what an id is at its shortest: four characters, lowercase
// letters and digits, a letter first.
var shaped = regexp.MustCompile(`^[a-z][a-z0-9]{3}$`)

// reservedOrganization derives, at its shortest, the word pin takes for
// routing: found by searching, as one organization in about a million does.
const reservedOrganization = "test-org-131599"

// The id an organization derives is the same on every Mac and every build:
// these are pinned, as a change would give every account a new id.
func TestDeriveIsTheSameEverywhere(t *testing.T) {
	tests := []struct {
		organization, want string
	}{
		{organization: "test-org-work", want: "b6s7"},
		{organization: "test-org-personal", want: "qgr2"},
		{organization: "00000000-0000-4000-8000-000000000001", want: "pd2u"},
	}
	for _, tt := range tests {
		t.Run(tt.organization, func(t *testing.T) {
			if got := (identity.Record{}).Derive(tt.organization, nil); got != tt.want {
				t.Errorf("Derive(%q) = %q, want %q", tt.organization, got, tt.want)
			}
		})
	}
}

// Every id derived is four characters, lowercase letters and digits, a
// letter first, any letter, and any of the 36 after it; and the same derived
// again.
func TestDerivedIDsAreShapedAsIDsAre(t *testing.T) {
	var ids []string
	for n := range 2000 {
		organization := fmt.Sprintf("test-org-%d", n)
		id := identity.Record{}.Derive(organization, nil)
		if again := (identity.Record{}).Derive(organization, nil); again != id {
			t.Fatalf("Derive(%q) = %q, then %q", organization, id, again)
		}
		ids = append(ids, id)
	}
	checkShaped(t, ids)
}

// An id the record holds for another account, or a configured account has,
// lengthens the one derived by a character, spelled on from the same hash, as
// many times as it takes; the one the record has for the organization itself
// is its own, whatever it is.
func TestDeriveLengthensAnIDHeldByAnother(t *testing.T) {
	const work = "test-org-work"
	tests := []struct {
		name         string
		organization string
		held         []identity.Entry
		configured   []string
		want         string
	}{
		{
			name:         "held by none",
			organization: work,
			held:         []identity.Entry{{ID: "qgr2", Organization: "test-org-personal"}},
			want:         "b6s7",
		},
		{
			name:         "held by another organization",
			organization: work,
			held:         []identity.Entry{{ID: "b6s7", Organization: "test-org-personal"}},
			want:         "b6s7e",
		},
		{
			name:         "held by an account whose organization isn't known",
			organization: work,
			held:         []identity.Entry{{ID: "b6s7"}},
			want:         "b6s7e",
		},
		{
			name:         "held by another, and the id a character longer too",
			organization: work,
			held:         []identity.Entry{{ID: "b6s7", Organization: "test-org-personal"}, {ID: "b6s7e"}},
			want:         "b6s7eh",
		},
		{
			name:         "an account's id before switchboard made ids",
			organization: work,
			held:         []identity.Entry{{ID: "m2x7", Was: "b6s7"}},
			want:         "b6s7e",
		},
		{
			name:         "an account's id before switchboard made ids, in another case",
			organization: work,
			held:         []identity.Entry{{ID: "m2x7", Was: "B6S7"}},
			want:         "b6s7e",
		},
		{
			name:         "held by the organization itself",
			organization: work,
			held:         []identity.Entry{{ID: "b6s7", Organization: work}},
			want:         "b6s7",
		},
		{
			name:         "held a character longer by the organization itself, its shortest by another",
			organization: work,
			held:         []identity.Entry{{ID: "b6s7", Organization: "test-org-personal"}, {ID: "b6s7e", Organization: work}},
			want:         "b6s7e",
		},
		{
			name:         "held a character longer by the organization itself, nothing holding its shortest",
			organization: work,
			held:         []identity.Entry{{ID: "b6s7e", Organization: work}},
			want:         "b6s7e",
		},
		{
			name:         "the organization's random id, given before its organization was known",
			organization: work,
			held:         []identity.Entry{{ID: "xk2m", Organization: work}},
			want:         "xk2m",
		},
		{
			name:         "the organization's, a configured account having it too",
			organization: work,
			held:         []identity.Entry{{ID: "b6s7", Organization: work}},
			configured:   []string{"b6s7"},
			want:         "b6s7",
		},
		{
			name:         "a configured account's the record doesn't hold",
			organization: work,
			configured:   []string{"qgr2", "b6s7"},
			want:         "b6s7e",
		},
		{
			name:         "a configured account's, in another case",
			organization: work,
			configured:   []string{"B6S7"},
			want:         "b6s7e",
		},
		{
			name:         "a configured account's, and the record's for another a character longer",
			organization: work,
			held:         []identity.Entry{{ID: "b6s7e", Organization: "test-org-personal"}},
			configured:   []string{"b6s7"},
			want:         "b6s7eh",
		},
		{
			name:         "the word pin takes for routing",
			organization: reservedOrganization,
			want:         "auto3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (identity.Record{Accounts: tt.held}).Derive(tt.organization, tt.configured); got != tt.want {
				t.Errorf("Derive(%q) = %q, want %q", tt.organization, got, tt.want)
			}
		})
	}
}

// Every random id is shaped as a derived one is.
func TestRandomIDsAreShapedAsIDsAre(t *testing.T) {
	seeded := rand.NewChaCha8([32]byte{})
	ids := make([]string, 2000)
	for i := range ids {
		ids[i] = random(t, identity.Record{}, nil, seeded)
	}
	checkShaped(t, ids)
}

// A random id is never one the record holds, as an id or as one an account
// had before switchboard made ids, nor one a configured account has, in any
// case, nor the word pin takes for routing: it's drawn again.
func TestRandomIDsAreNeverOnesHeld(t *testing.T) {
	reserved := sha256.Sum256([]byte(reservedOrganization))
	first, second, third := draw(1), draw(2), draw(3)
	firstID, secondID := random(t, identity.Record{}, nil, bytes.NewReader(first)), random(t, identity.Record{}, nil, bytes.NewReader(second))
	thirdID := random(t, identity.Record{}, nil, bytes.NewReader(third))
	tests := []struct {
		name       string
		held       []identity.Entry
		configured []string
		draws      [][]byte
		want       string
	}{
		{name: "held by none", draws: [][]byte{first, second}, want: firstID},
		{name: "an account's", held: []identity.Entry{{ID: firstID, Organization: "test-org-work"}}, draws: [][]byte{first, second}, want: secondID},
		{name: "an account's whose organization isn't known", held: []identity.Entry{{ID: firstID}}, draws: [][]byte{first, second}, want: secondID},
		{name: "an account's before switchboard made ids", held: []identity.Entry{{ID: "m2x7", Was: firstID}}, draws: [][]byte{first, second}, want: secondID},
		{
			name:  "an account's before switchboard made ids, in another case",
			held:  []identity.Entry{{ID: "m2x7", Was: strings.ToUpper(firstID)}},
			draws: [][]byte{first, second},
			want:  secondID,
		},
		{name: "two drawn in turn", held: []identity.Entry{{ID: firstID}, {ID: secondID}}, draws: [][]byte{first, second, third}, want: thirdID},
		{name: "the word pin takes for routing", draws: [][]byte{reserved[:], second}, want: secondID},
		{name: "a configured account's the record doesn't hold", configured: []string{"qgr2", firstID}, draws: [][]byte{first, second}, want: secondID},
		{name: "a configured account's, in another case", configured: []string{strings.ToUpper(firstID)}, draws: [][]byte{first, second}, want: secondID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := random(t, identity.Record{Accounts: tt.held}, tt.configured, bytes.NewReader(slices.Concat(tt.draws...)))
			if got != tt.want {
				t.Errorf("Random() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Random fails where what it draws from does, as when it runs out before
// drawing an id that isn't held, and where it draws only held ones, giving up
// rather than drawing for good.
func TestRandomFailsWhereItsDrawDoes(t *testing.T) {
	first := draw(1)
	held := identity.Record{Accounts: []identity.Entry{{ID: random(t, identity.Record{}, nil, bytes.NewReader(first))}}}
	tests := []struct {
		name   string
		record identity.Record
		rand   io.Reader
	}{
		{name: "nothing to draw", rand: bytes.NewReader(nil)},
		{name: "too little", rand: bytes.NewReader(first[:8])},
		{name: "run out past one held", record: held, rand: bytes.NewReader(first)},
		{name: "drawing a held one for good", record: held, rand: endless(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := tt.record.Random(tt.rand, nil)
			if id != "" || err == nil {
				t.Errorf("Random() = %q, %v; want no id, and why", id, err)
			}
		})
	}
}

// draw is a draw of random bytes, as many as Random takes for an id, each
// the byte given.
func draw(b byte) []byte {
	return bytes.Repeat([]byte{b}, sha256.Size)
}

// endless draws the byte it is, for good.
type endless byte

func (e endless) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(e)
	}
	return len(p), nil
}

// random returns the random id the record gives, drawn from rand, the
// configured accounts' ids being configured.
func random(t *testing.T, record identity.Record, configured []string, rand io.Reader) string {
	t.Helper()
	id, err := record.Random(rand, configured)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// checkShaped fails the test unless every id is shaped as an id is, and,
// between them, every letter starts one, and every character follows.
func checkShaped(t *testing.T, ids []string) {
	t.Helper()
	firsts, rests := map[rune]bool{}, map[rune]bool{}
	for _, id := range ids {
		if !shaped.MatchString(id) {
			t.Fatalf("id %q isn't four lowercase letters and digits, a letter first", id)
		}
		for i, c := range id {
			if i == 0 {
				firsts[c] = true
			} else {
				rests[c] = true
			}
		}
	}
	if len(firsts) != 26 || len(rests) != 36 {
		t.Errorf("%d ids start with %d characters, and %d follow; want every letter, and every letter and digit", len(ids), len(firsts), len(rests))
	}
}
