package identity_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"io"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// recordWithPools is an accounts record's file holding an account and its
// pools, as written.
const recordWithPools = `{
  "version": 1,
  "accounts": [
    {
      "id": "b6s7",
      "organization": "test-org-work",
      "added": "2026-09-01T12:00:00Z"
    }
  ],
  "pools": [
    {
      "id": "b2n8",
      "added": "2026-09-01T12:00:00Z",
      "first": true
    },
    {
      "id": "q7mt",
      "label": "Acme",
      "added": "2026-09-01T12:00:00Z",
      "removed": "2026-10-20T07:45:00Z"
    }
  ]
}
`

// The record keeps its pools: each one's id, its label once removed, when
// it was added and removed, in UTC to the second, and whether it's the
// first; and it reads back as it was written.
func TestTheRecordsPoolsAreWrittenAndReadBack(t *testing.T) {
	cet := time.FixedZone("CET", 60*60)
	pools := []identity.PoolEntry{
		{ID: "b2n8", Added: time.Date(2026, 9, 1, 13, 0, 0, 999, cet), First: true},
		{ID: "q7mt", Label: "Acme", Added: added, Removed: removed},
	}
	f := identity.NewFile(t.TempDir())
	err := f.Update(func(r *identity.Record) error {
		r.Accounts = []identity.Entry{{ID: "b6s7", Organization: "test-org-work", Added: added}}
		r.Pools = pools
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, f); got != recordWithPools {
		t.Errorf("the record's file holds\n%s\nwant\n%s", got, recordWithPools)
	}
	got, err := f.Read()
	if err != nil {
		t.Fatal(err)
	}
	stamped := slices.Clone(pools)
	stamped[0].Added = added
	if !slices.EqualFunc(got.Pools, stamped, identity.PoolEntry.Equal) {
		t.Errorf("Read() pools = %+v, want %+v", got.Pools, stamped)
	}
}

// A record from before pools reads as it did, holding none, and is written
// as it was, without pools.
func TestARecordWithoutPoolsReadsAndIsWrittenAsBefore(t *testing.T) {
	f := identity.NewFile(t.TempDir())
	writeFile(t, f, recordOfTwo, 0o600)
	got, err := f.Read()
	if err != nil || got.Pools != nil || !slices.Equal(ids(got.Accounts), []string{"b6s7", "m2x7"}) {
		t.Fatalf("Read() = %+v, %v; want its two accounts and no pools", got, err)
	}
	if err := f.Update(func(r *identity.Record) error { r.Accounts[0].Organization = "test-org-other"; return nil }); err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(recordOfTwo, "test-org-work", "test-org-other", 1); readFile(t, f) != want {
		t.Errorf("the record's file holds\n%s\nwant\n%s", readFile(t, f), want)
	}
}

// A record whose pools aren't ones switchboard keeps is damaged, and left as
// it was.
func TestARecordWithDamagedPoolsIsAnErrorAndLeftAsItWas(t *testing.T) {
	const account = `{"id": "b6s7", "added": "2026-09-01T12:00:00Z"}`
	tests := []struct {
		name, pools string
	}{
		{name: "a pool without an id", pools: `[{"added": "2026-09-01T12:00:00Z"}]`},
		{name: "a pool's id that isn't one switchboard makes", pools: `[{"id": "../acme", "added": "2026-09-01T12:00:00Z"}]`},
		{name: "a pool added at no time", pools: `[{"id": "q7mt"}]`},
		{name: "a pool removed without a label", pools: `[{"id": "q7mt", "added": "2026-09-01T12:00:00Z", "removed": "2026-10-20T07:45:00Z"}]`},
		{name: "two pools with one id", pools: `[{"id": "q7mt", "added": "2026-09-01T12:00:00Z"}, {"id": "q7mt", "added": "2026-09-01T12:00:00Z"}]`},
		{name: "two first pools", pools: `[{"id": "b2n8", "added": "2026-09-01T12:00:00Z", "first": true}, {"id": "q7mt", "added": "2026-09-01T12:00:00Z", "first": true}]`},
		{name: "pools not a list", pools: `{"id": "q7mt"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := identity.NewFile(t.TempDir())
			data := `{"version": 1, "accounts": [` + account + `], "pools": ` + tt.pools + `}`
			writeFile(t, f, data, 0o600)
			if got, err := f.Read(); !errors.Is(err, identity.ErrDamaged) {
				t.Errorf("Read() = %+v, %v; want an error matching ErrDamaged", got, err)
			}
			checkUnchanged(t, f, data)
		})
	}
}

// A change of the pools alone is written, and one that fails once it's
// committed puts the pools back as they were.
func TestAChangeOfThePoolsIsWrittenAndPutBack(t *testing.T) {
	failed := errors.New("the config's save failed")
	pool := identity.PoolEntry{ID: "m5r1", Added: added}
	t.Run("written", func(t *testing.T) {
		f := identity.NewFile(t.TempDir())
		writeFile(t, f, recordWithPools, 0o600)
		if err := f.Change(func(r *identity.Record, _ func() error) error { r.Pools = append(r.Pools, pool); return nil }); err != nil {
			t.Fatal(err)
		}
		if got, err := f.Read(); err != nil || len(got.Pools) != 3 || got.Pools[2].ID != "m5r1" {
			t.Errorf("Read() = %+v, %v; want the pool added", got.Pools, err)
		}
	})
	t.Run("put back", func(t *testing.T) {
		f := identity.NewFile(t.TempDir())
		writeFile(t, f, recordWithPools, 0o600)
		err := f.Change(func(r *identity.Record, commit func() error) error {
			r.Pools = append(r.Pools, pool)
			return errors.Join(commit(), failed)
		})
		if !errors.Is(err, failed) {
			t.Errorf("Change() = %v, want the change's error", err)
		}
		if got := readFile(t, f); got != recordWithPools {
			t.Errorf("the record's file holds\n%s\nwant it as it was", got)
		}
	})
}

// Joining another Mac's record keeps this one's pools.
func TestJoiningARecordKeepsItsPools(t *testing.T) {
	record := thisMacs()
	record.Pools = []identity.PoolEntry{{ID: "b2n8", Added: added, First: true}}
	got, _ := joined(t, record, joinAs, identity.Entry{ID: "n4p8", Organization: "test-org-new", Added: added})
	if !slices.EqualFunc(got.Pools, record.Pools, identity.PoolEntry.Equal) {
		t.Errorf("joined, the record's pools are %+v, want %+v", got.Pools, record.Pools)
	}
}

// Each pool is found by its id, in any case, and none for an id no pool has.
func TestAPoolByItsIDInTheRecord(t *testing.T) {
	record := identity.Record{Pools: []identity.PoolEntry{{ID: "b2n8", Added: added}, {ID: "q7mt", Label: "Acme", Added: added, Removed: removed}}}
	tests := []struct {
		id, want string
	}{
		{id: "q7mt", want: "q7mt"},
		{id: "Q7MT", want: "q7mt"},
		{id: "z9z9"},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got, ok := record.PoolByID(tt.id)
			if got.ID != tt.want || ok != (tt.want != "") {
				t.Errorf("PoolByID(%q) = %q, %v; want %q", tt.id, got.ID, ok, tt.want)
			}
		})
	}
}

func TestNewPoolIDsAreShapedAsIDsAre(t *testing.T) {
	seeded := rand.NewChaCha8([32]byte{})
	ids := make([]string, 2000)
	for i := range ids {
		ids[i] = newPoolID(t, identity.Record{}, nil, seeded)
	}
	checkShaped(t, ids)
}

// A pool's id is never one the record holds, an account's, one an account
// had before switchboard made ids, or a pool's, nor one the config gives an
// account or a pool, in any case, nor the word pin takes for routing: it's
// drawn again.
func TestNewPoolIDsAreNeverOnesHeld(t *testing.T) {
	reserved := sha256.Sum256([]byte(reservedOrganization))
	first, second, third := draw(1), draw(2), draw(3)
	firstID := newPoolID(t, identity.Record{}, nil, bytes.NewReader(first))
	secondID := newPoolID(t, identity.Record{}, nil, bytes.NewReader(second))
	thirdID := newPoolID(t, identity.Record{}, nil, bytes.NewReader(third))
	tests := []struct {
		name       string
		record     identity.Record
		configured []string
		draws      [][]byte
		want       string
	}{
		{name: "held by none", draws: [][]byte{first, second}, want: firstID},
		{name: "an account's", record: identity.Record{Accounts: []identity.Entry{{ID: firstID}}}, draws: [][]byte{first, second}, want: secondID},
		{name: "an account's before switchboard made ids, in another case", record: identity.Record{Accounts: []identity.Entry{{ID: "m2x7", Was: strings.ToUpper(firstID)}}}, draws: [][]byte{first, second}, want: secondID},
		{name: "a pool's", record: identity.Record{Pools: []identity.PoolEntry{{ID: firstID}}}, draws: [][]byte{first, second}, want: secondID},
		{name: "a removed pool's, in another case", record: identity.Record{Pools: []identity.PoolEntry{{ID: strings.ToUpper(firstID), Label: "Acme", Removed: removed}}}, draws: [][]byte{first, second}, want: secondID},
		{name: "a pool's and an account's in turn", record: identity.Record{Accounts: []identity.Entry{{ID: secondID}}, Pools: []identity.PoolEntry{{ID: firstID}}}, draws: [][]byte{first, second, third}, want: thirdID},
		{name: "one the config gives, the record not holding it", configured: []string{"work", strings.ToUpper(firstID)}, draws: [][]byte{first, second}, want: secondID},
		{name: "the word pin takes for routing", draws: [][]byte{reserved[:], second}, want: secondID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newPoolID(t, tt.record, tt.configured, bytes.NewReader(slices.Concat(tt.draws...))); got != tt.want {
				t.Errorf("NewPoolID() = %q, want %q", got, tt.want)
			}
		})
	}
}

// NewPoolID fails where what it draws from does, and where it draws only
// held ones.
func TestNewPoolIDFailsWhereItsDrawDoes(t *testing.T) {
	held := identity.Record{Pools: []identity.PoolEntry{{ID: newPoolID(t, identity.Record{}, nil, bytes.NewReader(draw(1)))}}}
	tests := []struct {
		name   string
		record identity.Record
		rand   io.Reader
	}{
		{name: "nothing to draw", rand: bytes.NewReader(nil)},
		{name: "drawing a held one for good", record: held, rand: endless(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if id, err := tt.record.NewPoolID(tt.rand, nil); err == nil {
				t.Errorf("NewPoolID() = %q, want an error", id)
			}
		})
	}
}

// The record keeps up with the pools configured, ids found in any case: one
// it lacks is added, but one whose id isn't one switchboard makes; one
// removed, or kept as only another Mac configures it, is configured again;
// and one that has left the config is marked removed, with the label it last
// had there, else its id. A config from before pools, which has none, leaves
// them as they are, as does one a heal is owed.
func TestTheRecordKeepsUpWithThePoolsConfigured(t *testing.T) {
	later := removed.Add(24 * time.Hour)
	record := func() identity.Record {
		return identity.Record{
			Accounts: []identity.Entry{{ID: "b6s7", Added: added, Was: "work"}},
			Pools: []identity.PoolEntry{
				{ID: "b2n8", Added: added, First: true},
				{ID: "q7mt", Label: "Acme", Added: added, Removed: removed},
				{ID: "m5r1", Label: "Side", Added: added},
				{ID: "h4w2", Added: added},
			},
		}
	}
	at := func(i int, p identity.PoolEntry) []identity.PoolEntry {
		pools := record().Pools
		pools[i] = p
		return pools
	}
	labelled := func(id string) string { return map[string]string{"h4w2": "Client"}[id] }
	// The pools as a config writes them without labels or directories, so
	// the record keeps none written of them.
	pools := func(ids ...string) config.Pools {
		ps := make(config.Pools, len(ids))
		for i, id := range ids {
			ps[i] = config.Pool{ID: id, Label: id}
		}
		return ps
	}
	tests := []struct {
		name     string
		accounts []string
		pools    config.Pools
		labelOf  func(string) string
		want     []identity.PoolEntry
		kept     identity.PoolsKept
		unhealed []string
	}{
		{name: "in step", pools: pools("B2N8", "h4w2"), want: record().Pools},
		{
			name: "a pool the record lacks", pools: pools("b2n8", "h4w2", "n4p8"),
			want: append(record().Pools, identity.PoolEntry{ID: "n4p8", Added: later}),
			kept: identity.PoolsKept{Added: []string{"n4p8"}},
		},
		{
			name: "one removed, configured again", pools: pools("b2n8", "h4w2", "Q7MT"),
			want: at(1, identity.PoolEntry{ID: "q7mt", Added: added}),
			kept: identity.PoolsKept{Returned: []string{"q7mt"}},
		},
		{
			name: "one only another Mac configures, configured here", pools: pools("b2n8", "h4w2", "m5r1"),
			want: at(2, identity.PoolEntry{ID: "m5r1", Added: added}),
			kept: identity.PoolsKept{Returned: []string{"m5r1"}},
		},
		{
			name: "one that left the config", pools: pools("b2n8"), labelOf: labelled,
			want: at(3, identity.PoolEntry{ID: "h4w2", Label: "Client", Added: added, Removed: later}),
			kept: identity.PoolsKept{Removed: []string{"h4w2"}},
		},
		{
			name: "one that left the config, its label not known", pools: pools("b2n8"),
			want: at(3, identity.PoolEntry{ID: "h4w2", Label: "h4w2", Added: added, Removed: later}),
			kept: identity.PoolsKept{Removed: []string{"h4w2"}},
		},
		{
			name: "one whose id isn't one switchboard makes", pools: pools("b2n8", "h4w2", "9abc"),
			want: record().Pools, kept: identity.PoolsKept{Clashed: []string{"9abc"}},
		},
		{name: "a config from before pools", want: record().Pools},
		{name: "a heal owed", accounts: []string{"WORK"}, pools: pools("b2n8"), want: record().Pools, unhealed: []string{"WORK"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := record()
			accounts := config.Accounts{{ID: "b6s7"}}
			if tt.accounts != nil {
				accounts = config.Accounts{{ID: tt.accounts[0]}}
			}
			kept := r.KeepUp(identity.Followed{Accounts: accounts, Pools: tt.pools, PoolLabel: tt.labelOf}, later)
			if !slices.EqualFunc(r.Pools, tt.want, identity.PoolEntry.Equal) {
				t.Errorf("the record's pools are\n%+v\nwant\n%+v", r.Pools, tt.want)
			}
			if want := (identity.Kept{Pools: tt.kept, Unhealed: tt.unhealed}); !reflect.DeepEqual(kept, want) {
				t.Errorf("kept %+v, want %+v", kept, want)
			}
		})
	}
}

// The record keeps each pool configured as the config writes it, its label
// and its directories, and each account configured in the pool the config
// has it in, kept once either is removed; a removed pool whose label the
// config's follower doesn't know is labelled as the record kept it written;
// and a config from before pools, put back, leaves them all as they were,
// for a heal to write back.
func TestTheRecordKeepsHowThePoolsAreWrittenAndEachAccountsPool(t *testing.T) {
	later := removed.Add(24 * time.Hour)
	acme := config.Pool{ID: "q7mt", Label: "Acme", Dirs: []string{"/Users/test/Code/acme"}, WrittenDirs: []string{"~/Code/acme"}}
	first := config.Pool{ID: "b2n8", Label: "b2n8"}
	accounts := config.Accounts{{ID: "b6s7", Pool: "q7mt"}, {ID: "m2x7", Pool: "b2n8"}}
	r := identity.Record{
		Accounts: []identity.Entry{{ID: "b6s7", Added: added}, {ID: "m2x7", Added: added}},
		Pools:    []identity.PoolEntry{{ID: "b2n8", Added: added, First: true}},
	}

	r.KeepUp(identity.Followed{Accounts: accounts, Pools: config.Pools{first, acme}}, later)
	written := []identity.PoolEntry{
		{ID: "b2n8", Added: added, First: true},
		{ID: "q7mt", Added: later, Written: identity.PoolWritten{Label: "Acme", Dirs: []string{"~/Code/acme"}}},
	}
	if !slices.EqualFunc(r.Pools, written, identity.PoolEntry.Equal) {
		t.Errorf("the record's pools are\n%+v\nwant each as the config writes it\n%+v", r.Pools, written)
	}
	if got := []string{r.Accounts[0].Pool, r.Accounts[1].Pool}; !slices.Equal(got, []string{"q7mt", "b2n8"}) {
		t.Errorf("the record has the accounts in the pools %v, want those the config has them in", got)
	}

	r.KeepUp(identity.Followed{Accounts: accounts}, later)
	if !slices.EqualFunc(r.Pools, written, identity.PoolEntry.Equal) || r.Accounts[0].Pool != "q7mt" {
		t.Errorf("a config from before pools put back, the record holds %+v and %+v, want them as they were", r.Pools, r.Accounts)
	}

	r.KeepUp(identity.Followed{Accounts: accounts[1:], Pools: config.Pools{first}}, later)
	gone := written[1]
	gone.Label, gone.Removed = "Acme", later
	if !slices.EqualFunc(r.Pools, []identity.PoolEntry{written[0], gone}, identity.PoolEntry.Equal) {
		t.Errorf("Acme left the config, the record's pools are %+v, want it removed, labelled as the record kept it written, its directories kept", r.Pools)
	}
	if e := r.Accounts[0]; e.Removed.IsZero() || e.Pool != "q7mt" {
		t.Errorf("b6s7 left the config, the record holds %+v, want it removed, its pool kept", e)
	}
}

// Joining another Mac's record takes no pool its config had an account in,
// nor how its config writes a pool: they're that Mac's config's.
func TestJoiningARecordTakesNoneOfHowTheOtherMacsConfigHasThePools(t *testing.T) {
	other := identity.Record{
		Accounts: []identity.Entry{{ID: "n4p8", Organization: "test-org-new", Added: added, Pool: "c1nt"}},
		Pools:    []identity.PoolEntry{{ID: "c1nt", Label: "Client", Added: added, Written: identity.PoolWritten{Label: "Client", Dirs: []string{"~/Code/client"}}}},
	}
	r := thisMacs()
	if _, err := r.Join(other, joinAs); err != nil {
		t.Fatal(err)
	}
	if e, ok := r.ByID("n4p8"); !ok || e.Pool != "" {
		t.Errorf("joined, the record holds %+v, want n4p8 in no pool of this Mac's", e)
	}
	if p, ok := r.PoolByID("c1nt"); !ok || !p.Equal(identity.PoolEntry{ID: "c1nt", Label: "Client", Added: added}) {
		t.Errorf("joined, the record holds %+v, want Client as the other Mac keeps it, but not as its config writes it", p)
	}
}

// An account's id and a pool's are never one: an account's derived or
// random id is never a pool's, and the record keeping up with the config
// leaves out an account configured under a pool's id, and a pool under an
// account's.
func TestAccountsAndPoolsNeverShareAnID(t *testing.T) {
	derived := identity.Record{}.Derive("test-org-work", nil)
	pooled := identity.Record{Pools: []identity.PoolEntry{{ID: strings.ToUpper(derived), Added: added}}}
	if got := pooled.Derive("test-org-work", nil); strings.EqualFold(got, derived) || len(got) != len(derived)+1 {
		t.Errorf("Derive() with a pool holding %q = %q, want it a character longer", derived, got)
	}

	first := draw(1)
	firstID := random(t, identity.Record{}, nil, bytes.NewReader(first))
	held := identity.Record{Pools: []identity.PoolEntry{{ID: firstID, Added: added}}}
	if got := random(t, held, nil, bytes.NewReader(slices.Concat(first, draw(2)))); got == firstID {
		t.Errorf("Random() with a pool holding %q = %q, want another", firstID, got)
	}

	r := identity.Record{
		Accounts: []identity.Entry{{ID: "b6s7", Added: added}},
		Pools:    []identity.PoolEntry{{ID: "b2n8", Added: added}},
	}
	kept := r.KeepUp(identity.Followed{
		Accounts: config.Accounts{{ID: "b6s7"}, {ID: "b2n8"}},
		Pools:    config.Pools{{ID: "b2n8"}, {ID: "b6s7"}},
	}, removed)
	want := identity.Kept{Clashed: []string{"b2n8"}, Pools: identity.PoolsKept{Clashed: []string{"b6s7"}}}
	if !reflect.DeepEqual(kept, want) {
		t.Errorf("kept %+v, want %+v", kept, want)
	}
	if len(r.Accounts) != 1 || len(r.Pools) != 1 {
		t.Errorf("the record holds %+v and %+v, want each as it was", r.Accounts, r.Pools)
	}
}

// newPoolID returns the pool's id the record gives, drawn from rand, the
// config's ids being configured.
func newPoolID(t *testing.T, record identity.Record, configured []string, rand io.Reader) string {
	t.Helper()
	id, err := record.NewPoolID(rand, configured)
	if err != nil {
		t.Fatal(err)
	}
	return id
}
