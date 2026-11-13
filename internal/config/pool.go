package config

import (
	"cmp"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
)

// Pool is a set of accounts that sessions are routed across: where it's
// fenced, to one or more directories, a session started inside them is
// routed across its accounts and no others.
type Pool struct {
	// ID names the pool for good, as switchboard made it.
	ID string
	// Label is what it's shown by: the config's, else its id.
	Label string
	// Dirs are the directories it's fenced to, each covering everything
	// beneath it, absolute and clean, one starting ~/ in the home directory:
	// none for the unfenced pool. WrittenDirs are the same, as the config
	// writes them, such as ~/Code/acme, for showing.
	Dirs, WrittenDirs []string
	// Unplaced are the directories it writes that can't be placed, as
	// FencesOf reads a pool of a config that breaks its rules: neither
	// absolute nor starting ~/, or starting ~/ where no home directory is
	// found. A pool Load reads has none.
	Unplaced []string
}

// Fenced reports whether the pool is fenced to directories: else it's the
// unfenced pool, serving every session started outside every fence.
func (p Pool) Fenced() bool {
	return len(p.Dirs) > 0
}

// Covers reports whether one of the pool's directories is dir or holds it,
// both absolute and clean, compared a whole folder name at a time and
// ignoring case. It reads no files: links are resolved by the caller, where
// they're to be.
func (p Pool) Covers(dir string) bool {
	return slices.ContainsFunc(p.Dirs, func(fence string) bool { return within(dir, fence) })
}

// Pools are the configured pools, in the config file's order: none in a
// config from before pools.
type Pools []Pool

// IDs lists the pools' ids.
func (ps Pools) IDs() []string {
	ids := make([]string, len(ps))
	for i, p := range ps {
		ids[i] = p.ID
	}
	return ids
}

// ByID returns the pool with the given id, ids compared ignoring case,
// reporting false where none has it.
func (ps Pools) ByID(id string) (Pool, bool) {
	return ps.find(func(p Pool) bool { return strings.EqualFold(p.ID, id) })
}

// LabelTaken reports whether a pool, but the one with the given id, shows
// label, ignoring case, as Accounts' LabelTaken says of an account's.
func (ps Pools) LabelTaken(id, label string) bool {
	_, taken := ps.Labelled(id, label)
	return taken
}

// Labelled returns the pool, but the one with the given id, that shows
// label, ignoring case, reporting false where none does.
func (ps Pools) Labelled(id, label string) (Pool, bool) {
	shown := prose.Clean(label)
	return ps.find(func(p Pool) bool { return p.ID != id && strings.EqualFold(prose.Clean(p.Label), shown) })
}

// Unfenced returns the pool with no directories, reporting false where every
// pool is fenced. A config has one at most.
func (ps Pools) Unfenced() (Pool, bool) {
	return ps.find(func(p Pool) bool { return !p.Fenced() })
}

// Implicit returns the pool a verb acts on where none is named: the only
// pool, where there's one, else the unfenced pool. It reports false where
// neither is, as where every pool of several is fenced, or none is
// configured.
func (ps Pools) Implicit() (Pool, bool) {
	if len(ps) == 1 {
		return ps[0], true
	}
	return ps.Unfenced()
}

// Asked returns the pool a verb naming the one with the given id, "" for
// none, reads or acts on: the one named, ids compared ignoring case, else
// the one Implicit gives. It reports false where the one named isn't
// configured, or none is named and Implicit gives none.
func (ps Pools) Asked(named string) (Pool, bool) {
	if named == "" {
		return ps.Implicit()
	}
	return ps.ByID(named)
}

// First returns the id of the first pool, where work recorded in no pool is:
// the one with the id noted, as the accounts record notes it, where it's
// among ps, as ps spells its id; else ps's first, as where the one noted has
// been removed, or can't be read; "" where ps holds none. The router and
// every reader of the recorded files go by it alike.
func (ps Pools) First(noted string) string {
	switch p, ok := ps.ByID(noted); {
	case ok:
		return p.ID
	case len(ps) > 0:
		return ps[0].ID
	}
	return ""
}

// find returns the first pool that matches, reporting false where none does.
func (ps Pools) find(match func(Pool) bool) (Pool, bool) {
	i := slices.IndexFunc(ps, match)
	if i < 0 {
		return Pool{}, false
	}
	return ps[i], true
}

// InPool returns the accounts in the pool with the given id, ids compared
// ignoring case, in the config's order.
func (as Accounts) InPool(id string) Accounts {
	var in Accounts
	for _, a := range as {
		if strings.EqualFold(a.Pool, id) {
			in = append(in, a)
		}
	}
	return in
}

// PrimaryOf returns the primary of the pool with the given id: the one of
// its accounts marked primary, else its first. It reports false where the
// pool has no account.
func (as Accounts) PrimaryOf(pool string) (Account, bool) {
	in := as.InPool(pool)
	if len(in) == 0 {
		return Account{}, false
	}
	return in.Primary(), true
}

// PoolOf returns the pool of the account with the given id, reporting false
// where no account has the id, as where the config is from before pools.
func (c *Config) PoolOf(id string) (Pool, bool) {
	a, ok := c.Accounts.ByID(id)
	if !ok {
		return Pool{}, false
	}
	return c.Pools.ByID(a.Pool)
}

// PoolsOf returns the pools the config file at path configures, read
// whatever else in the file is wrong, as ExportFolderOf reads the export's
// folder: as Load reads them, their directories starting ~/ found in the home
// directory homeDir returns. It returns none where the file configures none,
// or there's no file. It fails where the file can't be read as TOML, or its
// pools as Load would read them.
func PoolsOf(path string, homeDir func() (string, error)) (Pools, error) {
	var only struct {
		Pools []filePool `toml:"pool"`
	}
	if err := decodeOnly(path, &only); err != nil {
		return nil, err
	}
	pools, err := readPools(only.Pools, homeDir)
	if err != nil {
		return nil, invalid(path, err)
	}
	return pools, nil
}

// FencesOf returns the pools the config file at path writes, as written,
// for where PoolsOf can't read them, as they break Load's rules: each with
// the directories it writes placed, as PlacedIn places them in the home
// directory homeDir returns, where that can be found, and those it can't
// place Unplaced. They're for telling whether a directory is inside a fence
// alone, as nothing else of them is checked: each directory is a fence. It
// returns none where the file writes none, or there's no file, and fails
// where it can't be read as TOML.
func FencesOf(path string, homeDir func() (string, error)) (Pools, error) {
	var only struct {
		Pools []filePool `toml:"pool"`
	}
	if err := decodeOnly(path, &only); err != nil {
		return nil, err
	}
	home, err := findHome(homeDir)
	if err != nil {
		home = ""
	}
	var pools Pools
	for _, w := range only.Pools {
		pools = append(pools, Pool{ID: w.ID, Label: w.shownAs(), WrittenDirs: w.Dirs, Unplaced: w.Dirs}.PlacedIn(home))
	}
	return pools, nil
}

// PlacedIn returns p with those of its Unplaced directories that can be
// placed among its Dirs, home being the home directory, "" where none is
// found: an absolute one, cleaned, and one starting ~/, in home, where home
// is absolute, as findHome holds it to. Those that can't be stay Unplaced.
func (p Pool) PlacedIn(home string) Pool {
	unplaced := p.Unplaced
	p.Dirs, p.Unplaced = slices.Clone(p.Dirs), nil
	for _, dir := range unplaced {
		switch {
		case filepath.IsAbs(dir):
			p.Dirs = append(p.Dirs, filepath.Clean(dir))
		case strings.HasPrefix(dir, homePrefix) && filepath.IsAbs(home):
			p.Dirs = append(p.Dirs, expandIn(home, dir))
		default:
			p.Unplaced = append(p.Unplaced, dir)
		}
	}
	return p
}

// filePool is a [[pool]] table as it's written.
type filePool struct {
	ID    string   `toml:"id"`
	Label string   `toml:"label"`
	Dirs  []string `toml:"dirs"`
}

// shownAs is the label the pool is shown by: its own, else its id.
func (p filePool) shownAs() string {
	return cmp.Or(p.Label, p.ID)
}

// name identifies the pool in errors: by its id, else by its position.
func (p filePool) name(i int) string {
	if !p.named() {
		return fmt.Sprintf("pool #%d", i+1)
	}
	return poolNamed(p.ID)
}

// named reports whether the pool's id can name it in errors, as an
// account's can.
func (p filePool) named() bool {
	return p.ID != "" && !redact.HoldsToken(p.ID)
}

// poolNamed names the pool with the given id in errors, as in pool "q7mt",
// hiding anything in the id that looks like a token.
func poolNamed(id string) string {
	return fmt.Sprintf("pool %q", redact.Text(id))
}

// fence is one of a pool's directories, as the rules compare it with the
// others: the pool's place in the config, its name in errors, and the
// directory as it's written, and as it's found, absolute and clean.
type fence struct {
	pool               int
	name, written, dir string
}

// readPools returns the pools as they're written, ids, labels and
// directories checked, a directory starting ~/ found in the home directory
// homeDir returns, or every problem with them together: ids unique,
// ignoring case; one pool unfenced at most; and no directory the same as,
// inside or around another, its own pool's or another's.
func readPools(written []filePool, homeDir func() (string, error)) (Pools, error) {
	if len(written) == 0 {
		return nil, nil
	}
	var errs []error
	pools := make(Pools, len(written))
	var fences []fence
	for i, w := range written {
		pools[i] = Pool{ID: w.ID, Label: w.shownAs(), WrittenDirs: w.Dirs}
		own, err := w.check(i)
		fences, errs = append(fences, own...), append(errs, err)
	}
	errs = append(errs, uniquePoolIDs(written), oneUnfenced(written))
	if err := find(fences, homeDir); err != nil {
		return nil, errors.Join(append(errs, err)...)
	}
	errs = append(errs, noOverlap(fences))
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	for _, f := range fences {
		pools[f.pool].Dirs = append(pools[f.pool].Dirs, f.dir)
	}
	return pools, nil
}

// ReadPools returns the pools given as a config that writes them reads them,
// as Load reads a file's: checked, each shown by its label, else its id, and
// a directory starting ~/ found in the home directory homeDir returns. It
// fails as Load would, saying why.
func ReadPools(pools []NewPool, homeDir func() (string, error)) (Pools, error) {
	written := make([]filePool, len(pools))
	for i, p := range pools {
		written[i] = filePool(p)
	}
	return readPools(written, homeDir)
}

// check reports what's wrong with the pool at index i of the file, its id,
// label and directories, and returns its fences, of the directories it can
// be fenced to, as checkFence says, yet to be found.
func (p filePool) check(i int) ([]fence, error) {
	name := p.name(i)
	errs := []error{checkID(poolKind, name, p.ID), checkLabel(poolKind, name, p.Label)}
	var fences []fence
	for _, dir := range p.Dirs {
		if err := checkFence(name, dir); err != nil {
			errs = append(errs, err)
			continue
		}
		fences = append(fences, fence{pool: i, name: name, written: dir})
	}
	return fences, errors.Join(errs...)
}

// checkFence fails, saying why, unless dir is a directory the pool name
// names can be fenced to: an absolute path, or one starting ~/, the home
// directory's, holding nothing shaped like a token, as export.dir is.
func checkFence(name, dir string) error {
	switch {
	case redact.HoldsToken(dir):
		return fmt.Errorf("%s: %w", name, wrongValue("dirs", dir, "holds something shaped like a token, which switchboard never writes: give another directory"))
	case !filepath.IsAbs(dir) && !strings.HasPrefix(dir, homePrefix):
		return fmt.Errorf("%s: %w", name, wrongValue("dirs", dir, "must be an absolute path, or one starting ~/, the home directory's, such as ~/Code/acme"))
	}
	return nil
}

// find finds each fence's directory, as expandIn finds it, asking homeDir
// for the home directory only where one starts ~/, and failing once, for
// them all, where it can't be found.
func find(fences []fence, homeDir func() (string, error)) error {
	var home string
	if slices.ContainsFunc(fences, func(f fence) bool { return strings.HasPrefix(f.written, homePrefix) }) {
		var err error
		if home, err = findHome(homeDir); err != nil {
			return fmt.Errorf("find the home directory the pools' dirs are in: %w", err)
		}
	}
	for i := range fences {
		fences[i].dir = expandIn(home, fences[i].written)
	}
	return nil
}

// uniquePoolIDs reports each pool whose id another's is, ignoring case, as
// commands name a pool by its id in any case: an id given again, once, as
// checkAccounts reports an account's.
func uniquePoolIDs(pools []filePool) error {
	var errs []error
	uses := make(map[string]int, len(pools))
	// firsts is the first id named in each case, by the id in lower case.
	firsts := make(map[string]string, len(pools))
	for _, p := range pools {
		uses[p.ID]++
		if !p.named() || uses[p.ID] > 2 {
			continue
		}
		folded := strings.ToLower(p.ID)
		first, seen := firsts[folded]
		switch {
		case uses[p.ID] == 2:
			errs = append(errs, fmt.Errorf("duplicate pool id %q", p.ID))
		case seen:
			errs = append(errs, fmt.Errorf("pool ids %q and %q differ only in case, which commands take for the same pool", first, p.ID))
		default:
			firsts[folded] = p.ID
		}
	}
	return errors.Join(errs...)
}

// oneUnfenced reports more than one pool with no directories, naming each:
// a session started outside every fence would have two pools to be in.
func oneUnfenced(pools []filePool) error {
	var unfenced []string
	for i, p := range pools {
		if len(p.Dirs) == 0 {
			unfenced = append(unfenced, p.name(i))
		}
	}
	if len(unfenced) < 2 {
		return nil
	}
	return fmt.Errorf("%s have no dirs: only one pool can be unfenced, as a session started outside every fence would have two pools to be in", prose.List(unfenced))
}

// noOverlap reports each two fences whose directories overlap, as within
// compares them: the same, or one inside the other, whether they're one
// pool's or two.
func noOverlap(fences []fence) error {
	var errs []error
	for i, a := range fences {
		for _, b := range fences[i+1:] {
			if err := overlap(a, b); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// overlap fails, saying how, where a's directory and b's overlap.
func overlap(a, b fence) error {
	var how string
	switch inA, inB := within(b.dir, a.dir), within(a.dir, b.dir); {
	case inA && inB:
		how = "is the same as"
	case inA:
		how = "holds"
	case inB:
		how = "is inside"
	default:
		return nil
	}
	if a.pool == b.pool {
		return fmt.Errorf("%s: dirs %q %s %q: a pool's directories mustn't overlap", a.name, a.written, how, b.written)
	}
	return fmt.Errorf("%s's dirs %q %s %s's %q: no directory may be the same as, inside or around another pool's", a.name, a.written, how, b.name, b.written)
}

// within reports whether path is dir or inside it, both absolute and clean,
// compared a whole folder name at a time, so /Code/acme doesn't hold
// /Code/acme-api, and ignoring case, as macOS compares them. It's lexical
// alone: it reads no files.
func within(path, dir string) bool {
	in, fence := folders(path), folders(dir)
	return len(fence) <= len(in) && slices.EqualFunc(fence, in[:len(fence)], strings.EqualFold)
}

// folders returns the folder names of path, from the root.
func folders(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == filepath.Separator })
}

// idsApart reports each pool whose id is an account's, ignoring case: a
// pool's id and an account's are never one, as the accounts record keeps
// them apart.
func idsApart(accounts []fileAccount, pools []filePool) error {
	var errs []error
	for i, p := range pools {
		if !p.named() {
			continue
		}
		if j := slices.IndexFunc(accounts, func(a fileAccount) bool { return a.named() && strings.EqualFold(a.ID, p.ID) }); j >= 0 {
			errs = append(errs, fmt.Errorf("%s: id is %s's, ignoring case: a pool's id is never an account's", p.name(i), accounts[j].name(j)))
		}
	}
	return errors.Join(errs...)
}

// checkMembership reports each account that names no configured pool where
// the config has pools, each naming one where it has none, as a config from
// before pools has: pools names each pool by its id.
func checkMembership(accounts []fileAccount, pools []filePool) error {
	var errs []error
	for i, a := range accounts {
		if err := a.checkPool(a.name(i), pools); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// checkPool fails, saying why, where the account name names names no pool
// among those configured, where there are any, or names one where there
// aren't.
func (a fileAccount) checkPool(name string, pools []filePool) error {
	switch {
	case len(pools) == 0 && a.Pool != "":
		return fmt.Errorf("%s: %w", name, wrongValue("pool", a.Pool, "isn't configured, as the config has no [[pool]] table"))
	case len(pools) == 0:
		return nil
	case a.Pool == "":
		return fmt.Errorf("%s: pool is required, as the config has pools: give the id of the one it's in", name)
	case configuredPool(pools, a.Pool) < 0:
		return fmt.Errorf("%s: %w", name, wrongValue("pool", a.Pool, "isn't configured: give the id of a [[pool]]"))
	}
	return nil
}

// configuredPool returns where the pool with the given id is among pools,
// ids compared ignoring case: -1 where none has it.
func configuredPool(pools []filePool, id string) int {
	return slices.IndexFunc(pools, func(p filePool) bool { return strings.EqualFold(p.ID, id) })
}
