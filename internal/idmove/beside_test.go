package idmove_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
)

// errStopped is a move stopped, as by a power cut or a crash.
var errStopped = errors.New("stopped")

// historyOf is the fixture's history's files, as a router opens them.
func (f fixture) historyOf() idmove.History {
	return idmove.History{
		Ledger:   ledger.Open(ledger.Dir(f.state), dayfile.Forever, clock, ledger.HistoryIn(readingsFiles(f.state)), fixtureCaps, logs.For("test")),
		Readings: readingsFiles(f.state),
		Events:   events.Files(ledger.Dir(f.state), logs.For("test")),
	}
}

// configuredIDs are the ids of the accounts the fixture's config configures,
// as a router starting on it routes under.
func (f fixture) configuredIDs(t *testing.T) []string {
	t.Helper()
	return f.configured(t).IDs()
}

// configured are the accounts the fixture's config configures.
func (f fixture) configured(t *testing.T) config.Accounts {
	t.Helper()
	draft, err := config.Edit(f.config, nil)
	if err != nil {
		t.Fatal(err)
	}
	return draft.Config().Accounts
}

// lives runs the move of the fixture's install as routers run it, one after
// another until it's done: each starts on the config's ids as they stand,
// with the history's files of its own, settles the move, then runs it beside
// itself, ending where it waits for a restart. stop is called with each
// step's name, and each day's file's, as the move tells them, and with
// "settled in life n" once the nth router has settled it: where it fails,
// that router stops there.
func (f fixture) lives(t *testing.T, stop func(step string) error) {
	t.Helper()
	for life := 1; !idmove.Moved(f.state); life++ {
		if life > 6 {
			t.Fatalf("the move isn't done after %d routers", life-1)
		}
		m := f.mover(answering())
		m.Routing = f.configuredIDs(t)
		m.StopAfter(stop)
		if err := m.Settle(); err != nil {
			t.Fatalf("Settle() = %v", err)
		}
		if stop(fmt.Sprintf("settled in life %d", life)) != nil {
			continue
		}
		err := m.RunBeside(t.Context(), f.historyOf())
		if err != nil && !errors.Is(err, idmove.ErrAwaitingRestart) && !errors.Is(err, errStopped) {
			t.Fatalf("RunBeside() = %v", err)
		}
	}
}

func TestAMoveBesideRoutersStoppedAnywhereCarriesOnToTheSameEnd(t *testing.T) {
	clean := newFixture(t)
	var steps []string
	clean.lives(t, func(step string) error {
		steps = append(steps, step)
		return nil
	})
	want := clean.snapshot(t)
	for _, step := range []string{"config", "settled in life 1", "settled in life 2", "state", "drop"} {
		if !strings.Contains(strings.Join(steps, "|"), step) {
			t.Fatalf("the move beside routers told of %q, want %s among them", steps, step)
		}
	}
	for _, at := range steps {
		t.Run(at, func(t *testing.T) {
			f := newFixture(t)
			stopped := false
			f.lives(t, func(step string) error {
				if step == at && !stopped {
					stopped = true
					return errStopped
				}
				return nil
			})
			if !stopped {
				t.Errorf("the move never reached %s, to stop there", at)
			}
			if got := f.snapshot(t); !maps.Equal(got, want) {
				t.Errorf("stopped after %s and run on, the install differs from one never stopped:\n%s", at, differences(want, got))
			}
		})
	}
}

// A round summarises the days that have ended once their readings are moved
// and before their lines are, so each summary it writes holds an account's
// requests under its old id and its windows under its new: renamed, the two
// merge into one, every total kept.
func TestASummarisingRoundBesideTheMoveKeepsEveryTotal(t *testing.T) {
	f := newFixture(t)
	before := f.tallies(t)
	h := f.historyOf()
	m := f.mover(answering())
	m.History = &h
	summarised := false
	m.StopAfter(func(step string) error {
		if step != h.Readings.Name+" "+today || summarised {
			return nil
		}
		summarised = true
		for _, date := range []string{longAgo, yesterday} {
			if err := os.Remove(ledger.SummaryFile(ledger.Dir(f.state), date)); err != nil {
				return err
			}
		}
		h.Ledger.SummariseEnded(now)
		var s ledger.Summary
		if err := json.Unmarshal([]byte(readFile(t, ledger.SummaryFile(ledger.Dir(f.state), yesterday))), &s); err != nil {
			return err
		}
		var accounts []string
		for _, a := range s.Accounts {
			accounts = append(accounts, a.Account)
		}
		if !slices.Contains(accounts, work) || !slices.Contains(accounts, workID) {
			t.Errorf("summarised as the move ran, yesterday's summary holds %q, want work under both its ids, to merge", accounts)
		}
		return nil
	})
	if err := m.Run(t.Context()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if !summarised {
		t.Fatal("no round summarised as the move ran")
	}

	rename := f.renamer(t)
	want := make(map[string]int, len(before))
	for key, n := range before {
		kind, date, account := splitKey(key)
		want[kind+"|"+date+"|"+rename(account)] += n
	}
	if got := f.tallies(t); !maps.Equal(got, want) {
		t.Errorf("summarised as it was moved, the days' totals by account are\n%v\nwant\n%v", got, want)
	}
}

func TestTheConfigIsWrittenUnderTheAccountsRecordsLock(t *testing.T) {
	f := newFixture(t)
	first := f.mover(answering())
	first.StopAfter(func(step string) error {
		if step == "give" {
			return errStopped
		}
		return nil
	})
	if err := first.Run(t.Context()); !errors.Is(err, errStopped) {
		t.Fatalf("Run() = %v, want it stopped once it gave the token files their new names", err)
	}

	held, release := make(chan struct{}), make(chan struct{})
	var holding sync.WaitGroup
	holding.Go(func() {
		_ = identity.NewFile(f.state).Change(func(*identity.Record, func() error) error {
			close(held)
			<-release
			return nil
		})
	})
	<-held
	moved := make(chan error, 1)
	go func() { moved <- f.mover(answering()).Run(t.Context()) }()
	time.Sleep(100 * time.Millisecond)
	if got := readFile(t, f.config); got != fixtureConfig {
		t.Errorf("while another held the accounts record, the config was written:\n%s", got)
	}
	close(release)
	holding.Wait()
	if err := <-moved; err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if got := readFile(t, f.config); got != movedConfig {
		t.Errorf("once the record was let go of, the config reads\n%s\nwant\n%s", got, movedConfig)
	}
}

func TestHealingWritesTheNewIDsBackInAConfigPutBackToTheOld(t *testing.T) {
	f := newFixture(t)
	// The config is a link into a folder of its own, as one kept in a
	// repository of the user's files.
	kept := filepath.Join(t.TempDir(), "config.toml")
	writeFile(t, kept, fixtureConfig)
	if err := os.Remove(f.config); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(kept, f.config); err != nil {
		t.Fatal(err)
	}
	if healed, err := idmove.Heal(f.config, f.state, nil); err != nil || healed {
		t.Fatalf("Heal() before the move = %t, %v, want nothing healed", healed, err)
	}
	f.moved(t, answering())

	writeFile(t, kept, fixtureConfig)
	healed, err := idmove.Heal(f.config, f.state, nil)
	if err != nil || !healed {
		t.Fatalf("Heal() = %t, %v, want the config healed", healed, err)
	}
	if got := readFile(t, kept); got != movedConfig {
		t.Errorf("healed, the config reads\n%s\nwant\n%s", got, movedConfig)
	}
	if target, err := os.Readlink(f.config); err != nil || target != kept {
		t.Errorf("healed, the config's link leads to %q, %v, want it leading where it did", target, err)
	}
	for _, id := range f.configuredIDs(t) {
		if _, err := f.tokens.Read(id); err != nil {
			t.Errorf("healed, %s's token = %v, want it read", id, err)
		}
	}
	if healed, err := idmove.Heal(f.config, f.state, nil); err != nil || healed {
		t.Errorf("Heal() again = %t, %v, want nothing left to heal", healed, err)
	}
}

// A config merged mid-move, as one kept in a repository of the user's files,
// that names an account by its old id and its new: the move drops the one by
// its old id, giving what it sets to the one by its new id where that sets
// none, so routers restarted on it route by the new ids alone, and the move
// ends.
func TestAConfigMergedMidMoveToNameAnAccountByBothItsIDsKeepsTheOneByItsNew(t *testing.T) {
	f := newFixture(t)
	merged := "[[account]]\nid = \"work\"\nlabel = \"Work\"\nprimary = true\nplan = \"max\"\nreserve = 0.1\n\n" +
		"[[account]]\nid = \"personal\"\n\n[[account]]\nid = \"" + workID + "\"\n"
	f.lives(t, func(step string) error {
		if step == "give" {
			writeFile(t, f.config, merged)
		}
		return nil
	})

	want := config.Accounts{
		{ID: personalID, Label: personal},
		{ID: workID, Label: "Work", Primary: true, Plan: "max", Reserve: 0.1},
	}
	if got := f.configured(t); !slices.Equal(got, want) {
		t.Errorf("moved, the config configures %+v, want %+v", got, want)
	}
	if _, err := f.tokens.Read(workID); err != nil {
		t.Errorf("moved, %s's token = %v, want it read", workID, err)
	}
}

// A config with pools, one fenced to a directory in the home, is healed as
// any other, read in the home directory given: without one, it can't be.
func TestHealingReadsAConfigWithPoolsInTheHomeDirectory(t *testing.T) {
	f := newFixture(t)
	f.moved(t, answering())
	home := func() (string, error) { return "/Users/test", nil }
	pooled := "[[account]]\nid      = \"work\"\nlabel   = \"Work\"\nprimary = true\npool    = \"b2n8\"\n\n" +
		"[[account]]\nid   = \"personal\"\npool = \"b2n8\"\n\n" +
		"[[pool]]\nid = \"b2n8\"\n\n[[pool]]\nid   = \"q7mt\"\ndirs = [\"~/Code/acme\"]\n"
	writeFile(t, f.config, pooled)

	if healed, err := idmove.Heal(f.config, f.state, nil); err == nil || healed {
		t.Errorf("Heal() with no home = %t, %v, want it to fail, the pool's directory in the home not found", healed, err)
	}
	healed, err := idmove.Heal(f.config, f.state, home)
	if err != nil || !healed {
		t.Fatalf("Heal() = %t, %v, want the config healed", healed, err)
	}
	cfg, err := config.Load(f.config, home)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.Accounts.IDs(), []string{workID, personalID}; !slices.Equal(got, want) {
		t.Errorf("healed, the config configures %v, want %v", got, want)
	}
	if got := cfg.Accounts.InPool("b2n8").IDs(); !slices.Equal(got, []string{workID, personalID}) {
		t.Errorf("healed, pool b2n8 holds %v, want both accounts still in it", got)
	}
	if healed, err := idmove.Heal(f.config, f.state, home); err != nil || healed {
		t.Errorf("Heal() again = %t, %v, want nothing left to heal", healed, err)
	}
}

// A config that names an account by its old id and its new, as a merge of
// two of its versions leaves it once the move's done, is healed as the move
// heals one mid-move: the one by its old id dropped, the one by its new id
// keeping what it sets itself.
func TestHealingDropsAnAccountByItsOldIDWhereItsNewIsConfiguredToo(t *testing.T) {
	f := newFixture(t)
	f.moved(t, answering())
	merged := "[[account]]\nid = \"work\"\nlabel = \"Work\"\nprimary = true\nplan = \"max\"\nreserve = 0.1\n\n" +
		"[[account]]\nid = \"personal\"\n\n[[account]]\nid = \"" + workID + "\"\nlabel = \"Work again\"\nplan = \"pro\"\n"
	writeFile(t, f.config, merged)

	healed, err := idmove.Heal(f.config, f.state, nil)
	if err != nil || !healed {
		t.Fatalf("Heal() = %t, %v, want the config healed", healed, err)
	}
	want := config.Accounts{
		{ID: personalID, Label: personal},
		{ID: workID, Label: "Work again", Primary: true, Plan: "pro", Reserve: 0.1},
	}
	if got := f.configured(t); !slices.Equal(got, want) {
		t.Errorf("healed, the config configures %+v, want %+v", got, want)
	}
	if healed, err := idmove.Heal(f.config, f.state, nil); err != nil || healed {
		t.Errorf("Heal() again = %t, %v, want nothing left to heal", healed, err)
	}
}

// The account by its new id is its pool's primary after a heal exactly where
// the one dropped for it was before it, marked or as the first with none
// marked: marked where the dropped one was, or where it doesn't come first;
// left as it is where it's the first already, and where the dropped one
// wasn't the primary, or was another pool's. Another pool's primary keeps
// its mark.
func TestHealingCarriesThePrimaryWithinItsPool(t *testing.T) {
	const pools = "[[pool]]\nid = \"b2n8\"\n\n[[pool]]\nid   = \"q7mt\"\ndirs = [\"~/Code/acme\"]\n"
	// acme is an account in Acme, q7mt, never marked.
	const acme = "m4x9"
	account := func(id, pool string, marked bool) string {
		text := "[[account]]\nid   = \"" + id + "\"\npool = \"" + pool + "\"\n"
		if marked {
			text += "primary = true\n"
		}
		return text + "\n"
	}
	work := func(marked bool) string { return account("work", "q7mt", marked) }
	tests := []struct {
		name string
		// accounts are the config's accounts after personal, Default's
		// primary, marked.
		accounts []string
		// marked are the accounts the healed config marks primary, as it's
		// written, and unmarked those it doesn't.
		marked, unmarked []string
		// primary is Acme's primary, healed.
		primary string
	}{
		{
			name:     "unmarked first, another account before the new id",
			accounts: []string{work(false), account(acme, "q7mt", false), account(workID, "q7mt", false)},
			marked:   []string{personalID, workID}, unmarked: []string{acme}, primary: workID,
		},
		{
			name:     "marked first",
			accounts: []string{work(true), account(workID, "q7mt", false), account(acme, "q7mt", false)},
			marked:   []string{personalID, workID}, unmarked: []string{acme}, primary: workID,
		},
		{
			name:     "unmarked first, the new id right after it",
			accounts: []string{work(false), account(workID, "q7mt", false), account(acme, "q7mt", false)},
			marked:   []string{personalID}, unmarked: []string{workID, acme}, primary: workID,
		},
		{
			name:     "not the primary",
			accounts: []string{account(acme, "q7mt", false), work(false), account(workID, "q7mt", false)},
			marked:   []string{personalID}, unmarked: []string{workID, acme}, primary: acme,
		},
		{
			name:     "marked, the new id in another pool",
			accounts: []string{work(true), account(workID, "b2n8", false), account(acme, "q7mt", false)},
			marked:   []string{personalID}, unmarked: []string{workID, acme}, primary: acme,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			f.moved(t, answering())
			home := func() (string, error) { return "/Users/test", nil }
			writeFile(t, f.config, account("personal", "b2n8", true)+strings.Join(tt.accounts, "")+pools)

			if healed, err := idmove.Heal(f.config, f.state, home); err != nil || !healed {
				t.Fatalf("Heal() = %t, %v, want the config healed", healed, err)
			}
			draft, err := config.Edit(f.config, home)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range tt.marked {
				if !draft.MarksPrimaryOf(id) {
					t.Errorf("healed, the config doesn't mark %s primary, want it marked", id)
				}
			}
			for _, id := range tt.unmarked {
				if draft.MarksPrimaryOf(id) {
					t.Errorf("healed, the config marks %s primary, want it unmarked", id)
				}
			}
			if got, _ := draft.Config().Accounts.PrimaryOf("q7mt"); got.ID != tt.primary {
				t.Errorf("healed, Acme's primary is %s, want %s", got.ID, tt.primary)
			}
			if got, _ := draft.Config().Accounts.PrimaryOf("b2n8"); got.ID != personalID {
				t.Errorf("healed, Default's primary is %s, want %s, as it was", got.ID, personalID)
			}
		})
	}
}

// A compressed day damaged since it was summarised: its lines before the
// damage are moved with the rest, the file itself kept aside, so a round
// that summarises the day again counts each request once, under its new id.
func TestADamagedCompressedDayIsMovedUpToItsDamageItsTotalsCountedOnce(t *testing.T) {
	f := newFixture(t)
	lines := ledger.Files(ledger.Dir(f.state), logs.For("test")).CompressedPath(longAgo)
	whole, err := os.ReadFile(lines)
	if err != nil {
		t.Fatal(err)
	}
	cut := whole[:len(whole)-6]
	writeFile(t, lines, string(cut))
	notGzip := readingsFiles(f.state).CompressedPath("2026-11-01")
	writeFile(t, notGzip, "not gzip, nor ever was\n")
	before := f.tallies(t)
	f.runBounded(t, answering())

	for path, data := range map[string][]byte{lines: cut, notGzip: []byte("not gzip, nor ever was\n")} {
		if got, err := os.ReadFile(path + ".damaged"); err != nil || !bytes.Equal(got, data) {
			t.Errorf("%s set aside holds %q, %v, want it as it was", filepath.Base(path), got, err)
		}
	}
	if got := readFile(t, lines); strings.Contains(got, `"`+work+`"`) || !strings.Contains(got, `"`+workID+`"`) {
		t.Errorf("the damaged day's lines read\n%s\nwant them moved onto the new ids", got)
	}
	rename := f.renamer(t)
	want := make(map[string]int, len(before))
	for key, n := range before {
		kind, date, account := splitKey(key)
		want[kind+"|"+date+"|"+rename(account)] += n
	}
	if got := f.tallies(t); !maps.Equal(got, want) {
		t.Errorf("moved, the days' totals by account are\n%v\nwant\n%v", got, want)
	}
	if err := os.Remove(ledger.SummaryFile(ledger.Dir(f.state), longAgo)); err != nil {
		t.Fatal(err)
	}
	f.historyOf().Ledger.SummariseEnded(now)
	if got := f.tallies(t); !maps.Equal(got, want) {
		t.Errorf("summarised again, the days' totals by account are\n%v\nwant\n%v", got, want)
	}
}
