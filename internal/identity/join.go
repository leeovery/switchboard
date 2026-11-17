package identity

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/config"
)

// ErrConflict is what joining an accounts record that contradicts this one
// fails with, wrapped.
var ErrConflict = errors.New("the accounts records disagree")

// JoinAs is how a Mac's record is joined: Configured are the accounts this
// Mac configures, each in its pool, and Pools the pools; From is the install
// of the Mac whose record it is, by which an account whose id is held here is
// given another; Own is set where it's this Mac's own record, as its own
// export carries it, whose ids are this Mac's; Labels is set where its labels
// are taken, as they aren't from this Mac's own export, which may be older
// than a label another Mac gave since; and Theirs are the accounts the Mac
// whose record it is configures, each in its pool, as its export's copy of
// its config gives them, known only where TheirsKnown is set, as of a folder
// that holds the copy.
type JoinAs struct {
	Configured  config.Accounts
	Pools       config.Pools
	From        string
	Own         bool
	Labels      bool
	Theirs      config.Accounts
	TheirsKnown bool
	// Members are the accounts the configs of every Mac whose record an
	// import joins, this one's among them, put in each pool, by their ids
	// here, as Joined's Members gives them, each pool by its id in lower
	// case: a pool's id being random, it's one pool on every Mac, placed by
	// every account any of them puts in it. Nil where the record is joined
	// alone, and only its own Mac's config, as Theirs gives it, is gone by.
	Members map[string][]string
	// Before are the pools the record held as the import began.
	Before []PoolEntry
}

// Why a pool of another Mac's is kept apart as a record is joined, as
// Apart's Why gives it: words never changed, as an import's answer gives
// them.
const (
	// ApartSplit is a pool whose accounts, as the other Mac configures them,
	// are in more than one pool here.
	ApartSplit = "split"
	// ApartNoneHere is a pool none of whose accounts, as the other Mac
	// configures them, is configured here.
	ApartNoneHere = "none_here"
	// ApartNoConfig is a pool whose accounts can't be told, as the other
	// Mac's config isn't known, as of a folder exported before an export
	// carried it.
	ApartNoConfig = "no_config"
	// ApartEmpty is a pool the other Mac's config puts no account in.
	ApartEmpty = "empty"
	// ApartNoPoolsHere is a pool whose accounts can't go to a pool here, as
	// this Mac's config has no pools.
	ApartNoPoolsHere = "no_pools_here"
	// ApartBefore is a pool taken in as the pool here holding its accounts
	// that the record kept apart as the import began, as JoinAs' Before gives
	// it, as another Mac's: the days an import before took in, if any, stay
	// under its id, as a line held stands against one alike but for its
	// pool. Whether any were isn't known: an import that failed, or a restore
	// seeding the record, may have put the pool there with none.
	ApartBefore = "imported_before"
)

// Joined is what joining a record did: the accounts it took in under other
// ids, by their ids in lower case, each with its id here; the pools it took
// in as pools here, as Join says, the same; the pools the other Mac
// configures whose history it keeps apart, in the other record's order; and
// the accounts the other Mac's config puts in each pool it configures, by
// their ids here, each pool by its id in lower case, as JoinAs' Members
// takes them, none where that config isn't known.
type Joined struct {
	Accounts map[string]string
	Pools    map[string]string
	Apart    []Apart
	Members  map[string][]string
}

// Apart is a pool another Mac configures whose history joining its record
// keeps apart, read by its id, as a pool of its own here, whole, or, as
// ApartBefore says, as far as an import took it in before: the install of
// that Mac, the pool's id and label there, and Why, as ApartSplit and its kin
// say.
type Apart struct {
	Install, Pool, Label, Why string
}

// Join takes the accounts of another record, as an exported folder carries
// a Mac's, into r, and returns what it did, as Joined says, the ids of those
// it took in under other ids, in lower case, each with its id in r, among
// it. The ids each had before switchboard
// made ids are the Mac's own names, so none is taken, nor matched on. Each
// is matched to r's account of the same id, in this Mac's own record, as as
// says, its ids this Mac's; else of the same organization; else of the same
// id, as sameByID says:
//   - matched, it's that account, which keeps its id, times and removal, and
//     takes its organization where it has none, isn't configured here, and
//     no other account has it, as only a join can tell it, a token or the
//     router's probe telling one configured; and its label where the other
//     Mac configures it, as Configured says, and this one neither
//     configures it nor removed it, labels taken, as as says, so a label
//     changed there arrives; or, where this one removed it named by its id
//     alone, as the router names one that left the config while no router
//     ran, any other label the other record gives it, configured there or
//     not;
//   - matching none, of an id configured here that r doesn't hold, it's that
//     account, added to r with its organization and time added alone, as an
//     account added on another Mac sharing this one's config;
//   - matching none otherwise, it's added as it is, its label and removal
//     kept, but not the pool the other Mac's config had it in, as an
//     account only the other Mac configures or removed there;
//     where its id is held here already, as one of an account of another
//     organization, or one an account had before, under one spelled from
//     the other Mac's install and its id, as Derive spells one, so it's the
//     same however often it's joined.
//
// Its pools are joined once its accounts are, each as poolOf has it: matched
// by id, a pool's id being random, never one of an organization's, where
// it's configured here or this Mac's own; else taken in as the pool here
// holding the accounts put in it, as Members gives them, where they're in
// one; else apart, as joinPool joins it.
//
// It fails, changing nothing, with ErrConflict wrapped where r, joined,
// breaks the record's rules, as check says.
func (r *Record) Join(other Record, as JoinAs) (Joined, error) {
	j := joining{record: r.clone(), as: as, configured: as.Configured.IDs(), staged: other.ids()}
	joined := Joined{Accounts: make(map[string]string), Pools: make(map[string]string)}
	for _, e := range other.Accounts {
		if id := j.join(e); !strings.EqualFold(id, e.ID) {
			joined.Accounts[strings.ToLower(e.ID)] = id
		}
	}
	joined.Members = j.members(other.Pools, joined.Accounts)
	members := as.Members
	if members == nil {
		members = joined.Members
	}
	for _, p := range other.Pools {
		here, why := j.poolOf(p, members)
		if why != "" {
			joined.Apart = append(joined.Apart, Apart{Install: as.From, Pool: p.ID, Label: cmp.Or(p.Label, p.ID), Why: why})
		}
		if here != "" {
			joined.Pools[strings.ToLower(p.ID)] = here
			continue
		}
		j.joinPool(p)
	}
	if err := j.record.check(); err != nil {
		return Joined{}, fmt.Errorf("%w: joined, %w", ErrConflict, err)
	}
	*r = j.record
	return joined, nil
}

// joining is a record as Join takes another's accounts into it, as as says:
// the ids configured, and those of the other record's accounts, which an id
// spelled for one is never.
type joining struct {
	record             Record
	as                 JoinAs
	configured, staged []string
}

// join takes the account e into the record, as Join says, and returns the
// id it has there.
func (j *joining) join(e Entry) string {
	labelled := j.as.Labels && e.Configured
	e.Was, e.Configured, e.Pool = "", false, ""
	if i := j.matching(e); i >= 0 {
		return j.take(i, e, labelled)
	}
	if j.configuredUnheld(e) {
		j.record.Accounts = append(j.record.Accounts, Entry{ID: e.ID, Organization: e.Organization, Added: e.Added})
		return e.ID
	}
	if !j.record.clashes(e.ID, j.configured) {
		j.record.Accounts = append(j.record.Accounts, e)
		return e.ID
	}
	matched := -1
	id := spelledFrom(j.as.From+"/"+strings.ToLower(e.ID), func(id string) bool {
		if i := slices.IndexFunc(j.record.Accounts, func(own Entry) bool { return strings.EqualFold(own.ID, id) }); i >= 0 {
			if sameByID(j.record.Accounts[i], e) {
				matched = i
				return true
			}
			return false
		}
		return !j.record.clashes(id, j.configured) && !containsFold(j.staged, id)
	})
	if matched >= 0 {
		return j.take(matched, e, labelled)
	}
	e.ID = id
	j.record.Accounts = append(j.record.Accounts, e)
	return id
}

// configuredUnheld reports whether e is the account configured here under
// its id that the record doesn't hold, as Join says.
func (j *joining) configuredUnheld(e Entry) bool {
	return containsFold(j.configured, e.ID) && !j.record.clashes(e.ID, nil)
}

// take has the record's account at place i take what it doesn't know of e,
// the same account as another record keeps it: its organization, where it
// has none, isn't configured here, and no other account has it; and its
// label, where labelled says it's to be taken and the account is neither
// configured here nor removed, or where it's removed named by its id alone
// and e names it otherwise. It returns the account's id.
func (j *joining) take(i int, e Entry, labelled bool) string {
	own := &j.record.Accounts[i]
	if containsFold(j.configured, own.ID) {
		return own.ID
	}
	if _, held := j.record.ByOrganization(e.Organization); !held {
		own.organize(e.Organization)
	}
	if takesLabel(own.ID, own.Label, own.Removed.IsZero(), e.ID, e.Label, labelled) {
		own.Label = e.Label
	}
	return own.ID
}

// members are the accounts the other Mac's config, as Theirs gives it, puts
// in each of pools, those of its record, that it configures, by the ids
// accounts gives them here, else their own, as Joined's Members has them:
// none where that config isn't known.
func (j *joining) members(pools []PoolEntry, accounts map[string]string) map[string][]string {
	if !j.as.TheirsKnown {
		return nil
	}
	members := make(map[string][]string)
	for _, p := range pools {
		if !p.Configured {
			continue
		}
		in := []string{}
		for _, a := range j.as.Theirs {
			if config.SamePool(a.Pool, p.ID) {
				in = append(in, cmp.Or(accounts[strings.ToLower(a.ID)], a.ID))
			}
		}
		members[strings.ToLower(p.ID)] = in
	}
	return members
}

// poolOf is the pool here the pool p, as another record keeps it, is taken
// in as, as Join joins it, members being the accounts put in each pool, as
// Members gives them: none where p is configured here, or this Mac's own, as
// PoolEntry's Ours says, which it's matched to by its id; else the pool here
// holding its members, as poolHere finds it, so a Mac taking over from
// another, or restoring a backup, reads the history in the pool it
// configures, whatever id its config gave it, and a pool whose history is
// relayed through a Mac that imported it is placed as its own Mac places it.
// Where that's none, so p is kept apart, it says why, where p is live there,
// as the other Mac's export marks it configured: one removed there, or
// relayed, has no accounts to go by. Where it's one, but the record held p
// as the import began, as an import before kept it apart, it says so, as
// ApartBefore does.
func (j *joining) poolOf(p PoolEntry, members map[string][]string) (here, why string) {
	if own := j.record.pool(p.ID); containsFold(j.as.Pools.IDs(), p.ID) || own != nil && own.Ours() {
		return "", ""
	}
	if in, known := members[strings.ToLower(p.ID)]; known {
		here, why = j.poolHere(in)
	} else {
		why = ApartNoConfig
	}
	switch {
	case !p.Configured:
		why = ""
	case here != "" && slices.ContainsFunc(j.as.Before, func(b PoolEntry) bool { return strings.EqualFold(b.ID, p.ID) }):
		why = ApartBefore
	}
	return here, why
}

// poolHere is the pool here holding every account of in, the accounts put
// in a pool, by their ids here, that this Mac configures: "" where it can't
// be, with why, as ApartSplit and its kin say: this Mac has no pools, in is
// none, none of in is configured here, or they're in several pools here.
func (j *joining) poolHere(in []string) (string, string) {
	if len(j.as.Pools) == 0 {
		return "", ApartNoPoolsHere
	}
	var here []string
	for _, id := range in {
		own, ok := j.as.Configured.ByID(id)
		if ok && !slices.ContainsFunc(here, func(p string) bool { return config.SamePool(p, own.Pool) }) {
			here = append(here, own.Pool)
		}
	}
	switch {
	case len(in) == 0:
		return "", ApartEmpty
	case len(here) == 0:
		return "", ApartNoneHere
	case len(here) == 1:
		return here[0], ""
	}
	return "", ApartSplit
}

// joinPool takes the pool p, as another record keeps it, into the record,
// where it's no pool here, as poolOf says: matched by id, it's the record's
// pool, which keeps its times and removal, and, unless it's this Mac's, as
// PoolEntry's Here says, or configured here, takes p's label as take has an
// account take one; matching none, of an id configured here, it's that pool,
// added with its time added alone, as a pool added on another Mac sharing
// this one's config, unlabelled, as a configured one is, but this Mac's only
// once KeepUp keeps it, as nothing of how this Mac's config writes it is
// known till then; matching none otherwise, it's added as it is, its label
// and removal kept, live, labelled, by its id where it has no label, as only
// the other Mac configures it. What the other Mac marks a pool, its First,
// Configured, Here and Written, is that Mac's, never this one's.
func (j *joining) joinPool(p PoolEntry) {
	labelled := j.as.Labels && p.Configured
	p.First, p.Configured, p.Here, p.Written = false, false, false, PoolWritten{}
	configured := containsFold(j.as.Pools.IDs(), p.ID)
	switch own := j.record.pool(p.ID); {
	case own != nil && !own.Here && !configured:
		if takesLabel(own.ID, own.Label, own.Removed.IsZero(), p.ID, p.Label, labelled) {
			own.Label = p.Label
		}
	case own != nil:
	case configured:
		j.record.Pools = append(j.record.Pools, PoolEntry{ID: p.ID, Added: p.Added})
	default:
		p.Label = cmp.Or(p.Label, p.ID)
		j.record.Pools = append(j.record.Pools, p)
	}
}

// takesLabel reports whether an account or a pool this record keeps, not
// configured here, by its id and label, and whether it's live, takes the
// label another record gives it, by its id there and that label, as Join
// says: where labelled says it's taken and it's live here; or where it's
// removed here named by its id alone, as the router names one that left the
// config while no router ran, knowing no other, and the other names it
// otherwise.
func takesLabel(id, label string, live bool, otherID, otherLabel string, labelled bool) bool {
	namedByID := strings.EqualFold(label, id)
	return otherLabel != "" && (live && labelled || !live && namedByID && !strings.EqualFold(otherLabel, otherID))
}

// matching is the place of the account e is, as Join matches it: -1 where
// it's none.
func (j *joining) matching(e Entry) int {
	sameID := func(own Entry) bool { return strings.EqualFold(own.ID, e.ID) }
	if j.as.Own {
		if i := slices.IndexFunc(j.record.Accounts, sameID); i >= 0 {
			return i
		}
	}
	if e.Organization != "" {
		if i := slices.IndexFunc(j.record.Accounts, func(own Entry) bool { return own.Organization == e.Organization }); i >= 0 {
			return i
		}
	}
	return slices.IndexFunc(j.record.Accounts, func(own Entry) bool { return sameID(own) && sameByID(own, e) })
}

// sameByID reports whether e, of own's id, is own: always, but where both
// organizations are known and differ. Ids are made, never typed, so two
// subscriptions share one only by chance, about one in a million a pair, as
// the design allows ids; and where one's organization isn't known, such two
// are taken for one.
func sameByID(own, e Entry) bool {
	return own.Organization == "" || e.Organization == "" || own.Organization == e.Organization
}

// ids are the record's accounts' ids.
func (r Record) ids() []string {
	ids := make([]string, len(r.Accounts))
	for i, e := range r.Accounts {
		ids[i] = e.ID
	}
	return ids
}
