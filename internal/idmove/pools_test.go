package idmove_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/status"
)

// newIDsFixture returns the fixture's install moved onto the accounts' ids,
// as one upgrading from the release before pools finds it: its days
// summarised by a release before pools, yesterday's of version 6, its lines
// kept, and the long-done day's of version 6 too, its lines gone since.
func newIDsFixture(t *testing.T) fixture {
	t.Helper()
	f := newFixture(t)
	f.moved(t, answering())
	dir := ledger.Dir(f.state)
	for _, date := range []string{longAgo, yesterday} {
		path := ledger.SummaryFile(dir, date)
		writeFile(t, path, strings.Replace(readFile(t, path), `"version":7`, `"version":6`, 1))
	}
	if err := os.Remove(ledger.Files(dir, logs.For("test")).CompressedPath(longAgo)); err != nil {
		t.Fatal(err)
	}
	return f
}

// poolMover is the move to pools of the fixture's install, by the fixture's
// clock, drawing the first pool's id from a source that draws the same each
// time.
func (f fixture) poolMover() *idmove.PoolMover {
	return &idmove.PoolMover{ConfigPath: f.config, StateDir: f.state, Now: clock, Random: rand.NewChaCha8([32]byte{9})}
}

// pooled runs the move to pools of the fixture's install to its end,
// failing the test where it fails, and returns the first pool's id.
func (f fixture) pooled(t *testing.T) string {
	t.Helper()
	if err := f.poolMover().Run(t.Context()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	return f.firstPool(t)
}

// firstPool is the id of the first pool the fixture's accounts record notes.
func (f fixture) firstPool(t *testing.T) string {
	t.Helper()
	first, ok := f.record(t).FirstPool()
	if !ok {
		t.Fatal("the accounts record notes no first pool")
	}
	return first.ID
}

// withPool is the config text given as config's AddPool writes the pool
// with the given id in it, labelled as given, in one edit.
func withPool(t *testing.T, text, id, label string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, path, text)
	draft, err := config.Edit(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := draft.AddPool(config.NewPool{ID: id, Label: label}); err != nil {
		t.Fatal(err)
	}
	if err := draft.Save(); err != nil {
		t.Fatal(err)
	}
	return readFile(t, path)
}

func TestTheMoveToPoolsMakesTheFirstPoolEveryAccountInIt(t *testing.T) {
	f := newIDsFixture(t)
	before := readFile(t, f.config)
	first := f.pooled(t)

	if !identity.WellFormed(first) {
		t.Errorf("the first pool's id is %q, want one switchboard makes", first)
	}
	if got, want := readFile(t, f.config), withPool(t, before, first, "Default"); got != want {
		t.Errorf("the config reads\n%s\nwant it as one edit adding the first pool writes it\n%s", got, want)
	}
	cfg, err := config.Load(f.config, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range cfg.Accounts {
		if a.Pool != first {
			t.Errorf("%s is in pool %q, want the first, %q", a.ID, a.Pool, first)
		}
	}
	if !idmove.PoolsMoved(f.state) || idmove.PoolsPending(f.state) {
		t.Error("the move to pools isn't marked done")
	}
}

// configWithPools is the fixture's config moved onto the ids, as another Mac
// sharing it gave it pools: Acme first, the unfenced one after it.
const configWithPools = `[[account]]
id      = "b6s7"
label   = "Work"
primary = true
pool    = "q7mt"

[[account]]
id = "qgr2"
label = "personal"
pool  = "b2n8"

[[pool]]
id    = "q7mt"
label = "Acme"
dirs  = ["/tmp/acme"]

[[pool]]
id    = "b2n8"
label = "Default"
`

func TestTheMoveToPoolsTakesTheFirstOfAConfigsPoolsLeavingItAsItIs(t *testing.T) {
	f := newIDsFixture(t)
	writeFile(t, f.config, configWithPools)
	if first := f.pooled(t); first != "q7mt" {
		t.Errorf("the first pool is %q, want the config's first, q7mt", first)
	}
	if got := readFile(t, f.config); got != configWithPools {
		t.Errorf("the config reads\n%s\nwant it as it was", got)
	}
	if p, ok := f.record(t).PoolByID("q7mt"); !ok || !p.First || !p.Removed.IsZero() || p.Label != "" {
		t.Errorf("the record has q7mt as %+v, want it first, configured", p)
	}
}

func TestTheMoveToPoolsStoppedAfterAnyStepCarriesOnToTheSameEnd(t *testing.T) {
	clean := newIDsFixture(t)
	var steps []string
	m := clean.poolMover()
	m.StopAfter(func(step string) error {
		steps = append(steps, step)
		return nil
	})
	if err := m.Run(t.Context()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	want := clean.snapshot(t)
	for _, step := range []string{"pools first", "pools config", "pools history", "pools request ledger " + yesterday, "pools mark"} {
		if !slices.Contains(steps, step) {
			t.Fatalf("the move told of %q, want %q among them", steps, step)
		}
	}
	for _, stop := range steps {
		t.Run(stop, func(t *testing.T) {
			f := newIDsFixture(t)
			before := f.tallies(t)
			stopped := f.poolMover()
			stopped.StopAfter(func(step string) error {
				if step == stop {
					return errStopped
				}
				return nil
			})
			if err := stopped.Run(t.Context()); !errors.Is(err, errStopped) {
				t.Fatalf("Run() = %v, want it stopped after %s", err, stop)
			}
			if got := f.tallies(t); !maps.Equal(got, before) {
				t.Errorf("stopped after %s, the days' totals by account and pool are\n%v\nwant\n%v", stop, got, before)
			}
			f.pooled(t)
			if got := f.snapshot(t); !maps.Equal(got, want) {
				t.Errorf("stopped after %s and run again, the install differs from one never stopped:\n%s", stop, differences(want, got))
			}
			if got := f.tallies(t); !maps.Equal(got, before) {
				t.Errorf("stopped after %s and run again, the days' totals by account and pool are\n%v\nwant\n%v", stop, got, before)
			}
		})
	}
}

// pooledLines are the pools the lines of the files given name, by
// date, "" for a line naming none, and each line's kind as of, the
// key a line is told apart by.
func pooledLines(t *testing.T, files *dayfile.Files, of func(line []byte) string) map[string]string {
	t.Helper()
	pools := make(map[string]string)
	dates, err := files.All()
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range dates {
		_, _, err := dayfile.ReadDay(files, date, func(l []byte) ([]byte, bool) { return l, true }, func(l []byte) bool {
			var line struct {
				Pool string `json:"pool"`
			}
			if err := json.Unmarshal(l, &line); err != nil {
				t.Fatal(err)
			}
			pools[date+" "+of(l)] = line.Pool
			return true
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return pools
}

func TestEveryRequestLineAndPinEventAndSummaryNamesTheFirstPoolMoved(t *testing.T) {
	f := newIDsFixture(t)
	readings := readFile(t, plainPath(readingsFiles(f.state), today))
	first := f.pooled(t)

	dir := ledger.Dir(f.state)
	requests := pooledLines(t, ledger.Files(dir, logs.For("test")), func(l []byte) string {
		var line ledger.Line
		_ = json.Unmarshal(l, &line)
		return line.Request
	})
	if len(requests) != 4 {
		t.Fatalf("the request lines read are %v, want yesterday's and today's four", requests)
	}
	for line, pool := range requests {
		if pool != first {
			t.Errorf("request line %s names pool %q, want the first, %q", line, pool, first)
		}
	}
	pins := pooledLines(t, events.Files(dir, logs.For("test")), func(l []byte) string {
		var e status.Event
		_ = json.Unmarshal(l, &e)
		return e.Kind
	})
	want := map[string]string{longAgo + " " + status.EventLimit: "", longAgo + " " + status.EventMoved: "", today + " " + status.EventPin: first}
	if !maps.Equal(pins, want) {
		t.Errorf("the events name the pools %v, want %v: a pin's the first, every other's none", pins, want)
	}
	for _, date := range []string{longAgo, yesterday} {
		var s ledger.Summary
		if err := json.Unmarshal([]byte(readFile(t, ledger.SummaryFile(dir, date))), &s); err != nil {
			t.Fatal(err)
		}
		for _, a := range s.Accounts {
			if busy := len(a.Models) > 0; busy && (a.Pool != first || a.Pools != nil) || !busy && a.Pool != "" {
				t.Errorf("the summary of %s has %q's day in pool %q, %v, want its work in the first, %q, and none of no work", date, a.Account, a.Pool, a.Pools, first)
			}
		}
	}
	if got := readFile(t, plainPath(readingsFiles(f.state), today)); got != readings {
		t.Errorf("the readings history moved to pools reads\n%s\nwant it as it was, an account's windows its own", got)
	}
}

// Every day's totals by account are the same moved to pools, and the same
// again once a round summarises the days whose summaries are of an older
// version, every account's work in the first pool, its pools adding up to
// its whole.
func TestEveryDaysTotalsByAccountAreTheSameMovedToPools(t *testing.T) {
	f := newIDsFixture(t)
	before := f.tallies(t)
	first := f.pooled(t)
	if got := f.tallies(t); !maps.Equal(got, before) {
		t.Errorf("moved to pools, the days' totals by account are\n%v\nwant\n%v", got, before)
	}

	f.historyOf().Ledger.SummariseEnded(now)
	if got := f.tallies(t); !maps.Equal(got, before) {
		t.Errorf("summarised again, the days' totals by account are\n%v\nwant\n%v", got, before)
	}
	var s ledger.Summary
	if err := json.Unmarshal([]byte(readFile(t, ledger.SummaryFile(ledger.Dir(f.state), yesterday))), &s); err != nil {
		t.Fatal(err)
	}
	if s.Version != 7 {
		t.Errorf("yesterday's summary is of version %d, want it summarised again from its lines, as version 7", s.Version)
	}
	for _, a := range s.Accounts {
		if in := a.InPool(first, ""); !slicesOfModelsEqual(in.Models, a.Models) || a.Pools != nil {
			t.Errorf("yesterday's day of %q is %+v, want its whole work in the first pool alone", a.Account, a)
		}
	}
}

// slicesOfModelsEqual reports whether two accounts' days' models count
// alike.
func slicesOfModelsEqual(a, b []ledger.ModelDay) bool {
	return slices.EqualFunc(a, b, func(x, y ledger.ModelDay) bool { return x.Model == y.Model && x.Requests() == y.Requests() })
}

func TestAHistoryMovedToPoolsReadsTheSameMovedAgain(t *testing.T) {
	f := newIDsFixture(t)
	first := f.pooled(t)
	moved := f.snapshot(t)

	h := f.historyOf()
	for _, date := range []string{yesterday, today} {
		if err := h.Ledger.RenameDay(date, func(l []byte) []byte { return l }, ledger.Renaming{Unpooled: first}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(f.state, "pools.moved")); err != nil {
		t.Fatal(err)
	}
	f.pooled(t)
	if got := f.snapshot(t); !maps.Equal(got, moved) {
		t.Errorf("moved to pools again, the install differs from one moved once:\n%s", differences(moved, got))
	}
}

// A compressed day damaged since it was summarised is moved up to its
// damage, the file itself kept aside, as the move onto the ids keeps one.
func TestADamagedCompressedDayIsMovedToPoolsUpToItsDamage(t *testing.T) {
	f := newIDsFixture(t)
	files := events.Files(ledger.Dir(f.state), logs.For("test"))
	path := files.CompressedPath(longAgo)
	whole, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cut := whole[:len(whole)-6]
	writeFile(t, path, string(cut))
	f.pooled(t)

	if got, err := os.ReadFile(path + ".damaged"); err != nil || !bytes.Equal(got, cut) {
		t.Errorf("the damaged file set aside holds %q, %v, want it as it was", got, err)
	}
	if got := readFile(t, path); !strings.Contains(got, `"kind":"limit"`) {
		t.Errorf("the damaged day's events read\n%s\nwant its lines before the damage kept", got)
	}
}

// A day summarised by a release before pools since the move, as a downgrade
// leaves it, all its work in no pool while its lines name the first, is the
// first pool's work summarised again by a ledger with FirstPool: the first
// pool the accounts record notes.
func TestFirstPoolGivesADaySummarisedBeforePoolsSinceTheMoveTheRecordsFirstPool(t *testing.T) {
	f := newIDsFixture(t)
	first := f.pooled(t)
	dir := ledger.Dir(f.state)
	path := ledger.SummaryFile(dir, yesterday)
	downgraded := strings.ReplaceAll(strings.Replace(readFile(t, path), `"version":7`, `"version":6`, 1), `"pool":"`+first+`",`, "")
	if strings.Contains(downgraded, first) {
		t.Fatalf("yesterday's summary, as a release before pools writes it, still names the first pool:\n%s", downgraded)
	}
	writeFile(t, path, downgraded)

	ledger.Open(dir, dayfile.Forever, clock, ledger.HistoryIn(readingsFiles(f.state)), fixtureCaps, logs.For("test"), idmove.FirstPool(f.state)).SummariseEnded(now)
	var s ledger.Summary
	if err := json.Unmarshal([]byte(readFile(t, path)), &s); err != nil {
		t.Fatal(err)
	}
	for _, a := range s.Accounts {
		if in := a.InPool(first, ""); len(a.Models) > 0 && (!slicesOfModelsEqual(in.Models, a.Models) || a.Pools != nil) {
			t.Errorf("summarised again, yesterday's day of %q is %+v, want its whole work in the first pool alone", a.Account, a)
		}
	}
}

func TestTheMoveToPoolsWaitsOnTheMoveOntoTheIDs(t *testing.T) {
	f := newFixture(t)
	before := f.snapshot(t)
	if err := f.poolMover().Run(t.Context()); err == nil {
		t.Fatal("Run() before the move onto the ids = nil, want it refused")
	}
	if got := f.snapshot(t); !maps.Equal(got, before) {
		t.Errorf("refused, the install changed:\n%s", differences(before, got))
	}
}

// Beside a router that doesn't route in pools, the move writes the first
// pool in the config and waits for the router to take it up before it
// touches the history, so no line the router files after a day is moved
// names none.
func TestTheMoveToPoolsBesideARouterWaitsForItToTakeThePoolUp(t *testing.T) {
	f := newIDsFixture(t)
	before := f.tallies(t)
	lines := readFile(t, plainPath(ledger.Files(ledger.Dir(f.state), logs.For("test")), today))
	m := f.poolMover()
	if err := m.RunBeside(t.Context(), f.historyOf(), func() bool { return false }); !errors.Is(err, idmove.ErrAwaitingPools) {
		t.Fatalf("RunBeside() = %v, want it waiting for the router to take the pool up", err)
	}
	if cfg, err := config.Load(f.config, nil); err != nil || len(cfg.Pools) != 1 {
		t.Errorf("the config holds the pools %v, %v; want the first", cfg, err)
	}
	if got := readFile(t, plainPath(ledger.Files(ledger.Dir(f.state), logs.For("test")), today)); got != lines || idmove.PoolsMoved(f.state) {
		t.Errorf("waiting, the move changed today's lines to\n%s\nor was marked done; want them as they were", got)
	}

	if err := f.poolMover().RunBeside(t.Context(), f.historyOf(), func() bool { return true }); err != nil {
		t.Fatalf("RunBeside() once the router routes in pools = %v", err)
	}
	if !idmove.PoolsMoved(f.state) || !maps.Equal(f.tallies(t), before) {
		t.Error("once the router took the pool up, the move isn't done, every total kept")
	}
}

func TestTheMoveToPoolsIsPendingWhileThereIsAHistoryToMove(t *testing.T) {
	f := newIDsFixture(t)
	if !idmove.PoolsPending(f.state) || idmove.PoolsMoved(f.state) {
		t.Error("before the move, the history isn't pending its move to pools")
	}
	f.pooled(t)
	if idmove.PoolsPending(f.state) {
		t.Error("moved, the history is still pending its move to pools")
	}
	if idmove.PoolsPending(t.TempDir()) {
		t.Error("a state directory with no history is pending the move to pools, want none to move")
	}
}

func TestHealingGivesAConfigWithoutPoolsTheFirstPoolAgain(t *testing.T) {
	tests := []struct {
		name string
		// unlabelled takes the first pool's label out of the config before
		// it's put back.
		unlabelled bool
		want       string
	}{
		{name: "the first pool configured, labelled Default", want: "Default"},
		{name: "the first pool configured, its label taken out", unlabelled: true, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newIDsFixture(t)
			unpooled := readFile(t, f.config)
			first := f.pooled(t)
			if tt.unlabelled {
				edited(t, f.config, func(d *config.Draft) error { return d.SetPoolLabel(first, "") })
				f.keptUp(t)
			}
			writeFile(t, f.config, unpooled)

			healed, err := idmove.HealPools(f.config, f.state, nil)
			if err != nil || !healed {
				t.Fatalf("HealPools() = %v, %v; want the config healed", healed, err)
			}
			if got, want := readFile(t, f.config), withPool(t, unpooled, first, tt.want); got != want {
				t.Errorf("the config healed reads\n%s\nwant\n%s", got, want)
			}
			if healed, err := idmove.HealPools(f.config, f.state, nil); err != nil || healed {
				t.Errorf("HealPools() of a config with pools = %v, %v; want it left as it is", healed, err)
			}
		})
	}
}

// testHome is the home directory the tests read a config in, whose pools'
// directories start ~/.
func testHome() (string, error) { return "/Users/test", nil }

// clientPool is a client's pool, fenced to a directory in the home.
var clientPool = config.NewPool{ID: "c1nt", Label: "Client", Dirs: []string{"~/Code/client"}}

// keptUp keeps the fixture's accounts record in step with its config, as
// the router does as it follows it.
func (f fixture) keptUp(t *testing.T) {
	t.Helper()
	cfg, err := config.Load(f.config, testHome)
	if err != nil {
		t.Fatal(err)
	}
	if err := identity.NewFile(f.state).Update(func(r *identity.Record) error {
		r.KeepUp(identity.Followed{Accounts: cfg.Accounts, Pools: cfg.Pools}, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// edited edits the config at path as each edit given does, saving it once.
func edited(t *testing.T, path string, edits ...func(d *config.Draft) error) {
	t.Helper()
	draft, err := config.Edit(path, testHome)
	if err != nil {
		t.Fatal(err)
	}
	for _, edit := range edits {
		if err := edit(draft); err != nil {
			t.Fatal(err)
		}
	}
	if err := draft.Save(); err != nil {
		t.Fatal(err)
	}
}

// poolsIn are the pools the config at path makes, each its id, label and
// directories as written, in its order, and the pool each account is in, by
// its id.
func poolsIn(t *testing.T, path string) ([]string, map[string]string) {
	t.Helper()
	cfg, err := config.Load(path, testHome)
	if err != nil {
		t.Fatal(err)
	}
	var pools []string
	for _, p := range cfg.Pools {
		pools = append(pools, p.ID+" "+p.Label+" "+strings.Join(p.WrittenDirs, ","))
	}
	members := make(map[string]string)
	for _, a := range cfg.Accounts {
		members[a.ID] = a.Pool
	}
	return pools, members
}

// withAccount is a config's text with an account the accounts record has
// in no pool added.
func withAccount(text string) string {
	return text + "\n[[account]]\nid = \"n4p8\"\n"
}

// A config from before pools put back once the move's done, as a pull in the
// repository it's kept in puts one back, is healed with the pools the record
// keeps as the config last wrote them, in its order, their labels and
// directories, as written, with them, and each account in the pool it was
// in: so the client's pool stays fenced, its account in it. An account the
// record has in no pool goes to the first.
func TestHealingWritesThePoolsBackAsTheConfigLastHadThem(t *testing.T) {
	f := newIDsFixture(t)
	unpooled := readFile(t, f.config)
	first := f.pooled(t)
	moved := f.configuredIDs(t)[1]
	edited(t, f.config,
		func(d *config.Draft) error { return d.SetPoolLabel(first, "Everything") },
		func(d *config.Draft) error { return d.AddPool(clientPool) },
		func(d *config.Draft) error { return d.SetPool(moved, clientPool.ID) },
	)
	f.keptUp(t)
	wantPools, wantMembers := poolsIn(t, f.config)
	wantMembers["n4p8"] = first

	writeFile(t, f.config, withAccount(unpooled))
	if healed, err := idmove.HealPools(f.config, f.state, testHome); err != nil || !healed {
		t.Fatalf("HealPools() = %v, %v; want the config healed", healed, err)
	}
	gotPools, gotMembers := poolsIn(t, f.config)
	if !slices.Equal(gotPools, wantPools) {
		t.Errorf("healed, the config's pools are %q, want them as it last had them, %q", gotPools, wantPools)
	}
	if !maps.Equal(gotMembers, wantMembers) {
		t.Errorf("healed, the accounts are in the pools %v, want %v: each in the pool it was in, the one the record has in none in the first", gotMembers, wantMembers)
	}
}

// inClient moves every account the draft configures into the client's pool.
func inClient(d *config.Draft) error {
	for _, id := range d.Config().Accounts.IDs() {
		if err := d.SetPool(id, clientPool.ID); err != nil {
			return err
		}
	}
	return nil
}

// clientsAndRest has every account in the client's pool, the first pool
// removed, and the rest's pool, fenced to the directories given, added.
func clientsAndRest(first string, restDirs []string) []func(d *config.Draft) error {
	return []func(d *config.Draft) error{
		func(d *config.Draft) error { return d.AddPool(clientPool) },
		inClient,
		func(d *config.Draft) error { return d.RemovePool(first) },
		func(d *config.Draft) error {
			return d.AddPool(config.NewPool{ID: "z4z4", Label: "Rest", Dirs: restDirs})
		},
	}
}

// An account the record has in no pool it keeps goes to the unfenced pool
// written back, never a fenced one, as a client's is.
func TestHealingPutsAnAccountInNoPoolInTheUnfencedPool(t *testing.T) {
	f := newIDsFixture(t)
	unpooled := readFile(t, f.config)
	edited(t, f.config, clientsAndRest(f.pooled(t), nil)...)
	f.keptUp(t)

	writeFile(t, f.config, withAccount(unpooled))
	if healed, err := idmove.HealPools(f.config, f.state, testHome); err != nil || !healed {
		t.Fatalf("HealPools() = %v, %v; want the config healed", healed, err)
	}
	pools, members := poolsIn(t, f.config)
	if want := []string{"c1nt Client ~/Code/client", "z4z4 Rest "}; !slices.Equal(pools, want) || members["n4p8"] != "z4z4" {
		t.Errorf("healed, the config holds the pools %q, n4p8 in %q; want %q, n4p8 in Rest, the unfenced one", pools, members["n4p8"], want)
	}
}

// Where every pool the record keeps is fenced, an account it has in none of
// them has no pool to go to, never the first's, so the config is left as it
// is: whether the first is among them or removed.
func TestHealingFailsWhereAnAccountInNoPoolHasNoUnfencedPoolToGoTo(t *testing.T) {
	tests := []struct {
		name  string
		edits func(first string) []func(d *config.Draft) error
	}{
		{
			name: "the first among them",
			edits: func(first string) []func(d *config.Draft) error {
				return []func(d *config.Draft) error{func(d *config.Draft) error { return d.SetPoolDirs(first, []string{"~/Code"}) }}
			},
		},
		{
			name: "the first removed",
			edits: func(first string) []func(d *config.Draft) error {
				return clientsAndRest(first, []string{"~/Code/rest"})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newIDsFixture(t)
			unpooled := withAccount(readFile(t, f.config))
			edited(t, f.config, tt.edits(f.pooled(t))...)
			f.keptUp(t)

			writeFile(t, f.config, unpooled)
			if healed, err := idmove.HealPools(f.config, f.state, testHome); err == nil || healed {
				t.Errorf("HealPools() = %v, %v; want it to fail", healed, err)
			}
			if got := readFile(t, f.config); got != unpooled {
				t.Errorf("the config reads\n%s\nwant it as it was\n%s", got, unpooled)
			}
		})
	}
}

// The pools a config naming none is to have, as heal writes them and a
// router routes within them till it has: those the record keeps as this
// Mac's, as last written, in its order; each account in the pool the record
// has it in, by its id now or the one it had before ids; one it has in none
// in the unfenced pool, else in none, never the first's.
func TestThePoolsKeptAreThoseTheRecordKeepsAsThisMacs(t *testing.T) {
	first := identity.PoolEntry{ID: "b2n8", Added: now, First: true, Here: true, Written: identity.PoolWritten{Label: "Default", Dirs: []string{"~/Work"}}}
	client := identity.PoolEntry{ID: "c1nt", Added: now, Here: true, Written: identity.PoolWritten{Label: "Client", Dirs: []string{"~/Code/client"}}}
	gone := identity.PoolEntry{ID: "g0n3", Label: "Gone", Added: now, Removed: now, Written: identity.PoolWritten{Label: "Gone"}}
	theirs := identity.PoolEntry{ID: "t4r5", Label: "Theirs", Added: now}
	rest := identity.PoolEntry{ID: "z4z4", Added: now, Here: true, Written: identity.PoolWritten{Label: "Rest"}}
	accounts := []identity.Entry{
		{ID: "b6s7", Was: "work", Added: now, Pool: "b2n8"},
		{ID: "qgr2", Added: now, Pool: "C1NT"},
		{ID: "m2k5", Added: now, Pool: "g0n3"},
	}
	ids := []string{"work", "qgr2", "m2k5", "n4p8"}
	tests := []struct {
		name      string
		pools     []identity.PoolEntry
		wantPools []string
		wantIn    map[string]string
	}{
		{
			name:      "every pool fenced",
			pools:     []identity.PoolEntry{first, client, gone, theirs},
			wantPools: []string{"b2n8 Default ~/Work", "c1nt Client ~/Code/client"},
			wantIn:    map[string]string{"work": "b2n8", "qgr2": "c1nt", "m2k5": "", "n4p8": ""},
		},
		{
			name:      "one unfenced",
			pools:     []identity.PoolEntry{first, client, gone, theirs, rest},
			wantPools: []string{"b2n8 Default ~/Work", "c1nt Client ~/Code/client", "z4z4 Rest "},
			wantIn:    map[string]string{"work": "b2n8", "qgr2": "c1nt", "m2k5": "z4z4", "n4p8": "z4z4"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept, err := idmove.PoolsKept(identity.Record{Accounts: accounts, Pools: tt.pools}, ids)
			if err != nil {
				t.Fatal(err)
			}
			var pools []string
			for _, p := range kept.Pools {
				pools = append(pools, p.ID+" "+p.Label+" "+strings.Join(p.Dirs, ","))
			}
			if !slices.Equal(pools, tt.wantPools) || !maps.Equal(kept.In, tt.wantIn) {
				t.Errorf("PoolsKept() = %q, %v; want %q, %v", pools, kept.In, tt.wantPools, tt.wantIn)
			}
		})
	}
	if _, err := idmove.PoolsKept(identity.Record{Accounts: accounts, Pools: []identity.PoolEntry{client}}, ids); err == nil {
		t.Error("PoolsKept() of a record noting no first pool succeeded, want it to fail")
	}
}

// Healed, each pool's primary is the one the config last had it mark, in
// place of any a config from before pools marks.
func TestHealingMarksEachPoolsPrimaryAsTheConfigLastHadIt(t *testing.T) {
	f := newIDsFixture(t)
	unpooled := readFile(t, f.config)
	first := f.pooled(t)
	work, personal := f.configuredIDs(t)[0], f.configuredIDs(t)[1]
	edited(t, f.config,
		func(d *config.Draft) error { return d.SetPrimary(personal) },
		func(d *config.Draft) error { return d.AddPool(clientPool) },
		func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "n4p8", Pool: clientPool.ID}) },
		func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "m2k5", Pool: clientPool.ID}) },
		func(d *config.Draft) error { return d.SetPrimary("m2k5") },
	)
	f.keptUp(t)

	writeFile(t, f.config, withAccount(unpooled)+"\n[[account]]\nid = \"m2k5\"\n")
	if healed, err := idmove.HealPools(f.config, f.state, testHome); err != nil || !healed {
		t.Fatalf("HealPools() = %v, %v; want the config healed", healed, err)
	}
	want := map[string]string{
		work:     first + " primary false",
		personal: first + " primary true",
		"n4p8":   clientPool.ID + " primary false",
		"m2k5":   clientPool.ID + " primary true",
	}
	if got := primariesIn(t, f.config); !maps.Equal(got, want) {
		t.Errorf("healed, the accounts are %v, want %v", got, want)
	}
}

// primariesIn are the pool each account the config at path configures is
// in, and whether it's its pool's primary, by its id.
func primariesIn(t *testing.T, path string) map[string]string {
	t.Helper()
	cfg, err := config.Load(path, testHome)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string)
	for _, a := range cfg.Accounts {
		got[a.ID] = fmt.Sprintf("%s primary %v", a.Pool, a.Primary)
	}
	return got
}

// A pool the config last had no account in had no primary, so healed, the
// accounts it's given have none marked, a mark a config from before pools
// gives one taken off, and its first stands.
func TestHealingTakesTheMarksOffAPoolTheConfigLastHadNoAccountIn(t *testing.T) {
	f := newIDsFixture(t)
	unpooled := readFile(t, f.config)
	first := f.pooled(t)
	work, personal := f.configuredIDs(t)[0], f.configuredIDs(t)[1]
	edited(t, f.config, func(d *config.Draft) error { return d.AddPool(clientPool) }, inClient)
	f.keptUp(t)

	writeFile(t, f.config, unpooled)
	edited(t, f.config,
		func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "n4p8"}) },
		func(d *config.Draft) error { return d.AddAccount(config.NewAccount{ID: "m2k5", Primary: true}) },
	)
	if healed, err := idmove.HealPools(f.config, f.state, testHome); err != nil || !healed {
		t.Fatalf("HealPools() = %v, %v; want the config healed", healed, err)
	}
	want := map[string]string{
		work:     clientPool.ID + " primary true",
		personal: clientPool.ID + " primary false",
		"n4p8":   first + " primary true",
		"m2k5":   first + " primary false",
	}
	if got := primariesIn(t, f.config); !maps.Equal(got, want) {
		t.Errorf("healed, the accounts are %v, want %v", got, want)
	}
}

// configWithEverything is the fixture's config moved onto the ids, in one
// pool, unfenced, labelled Everything.
const configWithEverything = `[[account]]
id    = "b6s7"
label = "Work"
pool  = "b2n8"

[[account]]
id    = "qgr2"
label = "personal"
pool  = "b2n8"

[[pool]]
id    = "b2n8"
label = "Everything"
`

// The first pool of a config with pools, as the move notes it, is healed as
// that config wrote it, before the record is ever kept in step with it.
func TestHealingBeforeTheRecordIsKeptInStepWritesTheFirstPoolAsTheConfigHadIt(t *testing.T) {
	f := newIDsFixture(t)
	unpooled := readFile(t, f.config)
	writeFile(t, f.config, configWithEverything)
	f.pooled(t)

	writeFile(t, f.config, unpooled)
	if healed, err := idmove.HealPools(f.config, f.state, testHome); err != nil || !healed {
		t.Fatalf("HealPools() = %v, %v; want the config healed", healed, err)
	}
	if pools, _ := poolsIn(t, f.config); !slices.Equal(pools, []string{"b2n8 Everything "}) {
		t.Errorf("healed, the config holds the pools %q, want the first labelled Everything, as the config had it", pools)
	}
}

// A config put back from before both moves, its accounts named by the ids
// they had before switchboard made ids, is healed with each account in the
// pool the record has it in by its id now, as a router looking at its config
// heals it before its restart writes the ids back: so the ids written back
// later find each account in its pool still.
func TestHealingAConfigFromBeforeBothMovesPutsEachAccountInItsPool(t *testing.T) {
	f := newIDsFixture(t)
	first := f.pooled(t)
	personalID := f.configuredIDs(t)[1]
	edited(t, f.config,
		func(d *config.Draft) error { return d.AddPool(clientPool) },
		func(d *config.Draft) error { return d.SetPool(personalID, clientPool.ID) },
	)
	f.keptUp(t)

	writeFile(t, f.config, fixtureConfig)
	if healed, err := idmove.HealPools(f.config, f.state, testHome); err != nil || !healed {
		t.Fatalf("HealPools() = %v, %v; want the config healed", healed, err)
	}
	if _, members := poolsIn(t, f.config); members[work] != first || members[personal] != clientPool.ID {
		t.Errorf("healed, the accounts are in the pools %v, want %s in the first, %s in Client", members, work, personal)
	}
	if _, err := idmove.Heal(f.config, f.state, testHome); err != nil {
		t.Fatal(err)
	}
	if _, members := poolsIn(t, f.config); members[personalID] != clientPool.ID {
		t.Errorf("given its ids back, the config has the accounts in the pools %v, want %s in Client", members, personalID)
	}
}

func TestHealingWaitsForTheMoveToPools(t *testing.T) {
	f := newIDsFixture(t)
	before := readFile(t, f.config)
	if healed, err := idmove.HealPools(f.config, f.state, nil); err != nil || healed || readFile(t, f.config) != before {
		t.Errorf("HealPools() before the move = %v, %v; want the config left as it is", healed, err)
	}
}

// plainPath is where files keep the plain file of the day with the given
// date.
func plainPath(files *dayfile.Files, date string) string {
	return filepath.Join(files.Dir, files.Prefix+"-"+date+".jsonl")
}
