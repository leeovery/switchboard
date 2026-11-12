package idmove_test

import (
	"bytes"
	"encoding/json"
	"errors"
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
			f.pooled(t)
			if got := f.snapshot(t); !maps.Equal(got, want) {
				t.Errorf("stopped after %s and run again, the install differs from one never stopped:\n%s", stop, differences(want, got))
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
		if in := a.InPool(first); !slicesOfModelsEqual(in.Models, a.Models) || a.Pools != nil {
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
		// removed is the label the record keeps of the first pool, as
		// removed: "" where it's configured.
		removed string
		want    string
	}{
		{name: "the first pool configured, labelled Default", want: "Default"},
		{name: "the first pool removed, labelled as the record knew it", removed: "Everything", want: "Everything"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newIDsFixture(t)
			unpooled := readFile(t, f.config)
			first := f.pooled(t)
			if tt.removed != "" {
				if err := identity.NewFile(f.state).Update(func(r *identity.Record) error {
					r.Pools[0].Label, r.Pools[0].Removed = tt.removed, now
					return nil
				}); err != nil {
					t.Fatal(err)
				}
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
