package idmove_test

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
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
	"github.com/leeovery/switchboard/internal/readings"
)

// errStopped is a move stopped, as by a power cut or a crash.
var errStopped = errors.New("stopped")

// historyOf is the fixture's history's files, as a router opens them.
func (f fixture) historyOf() idmove.History {
	return idmove.History{
		Ledger:   ledger.Open(ledger.Dir(f.state), dayfile.Forever, clock, ledger.HistoryIn(readingsFiles(f.state)), ledger.Caps{}, logs.For("test")),
		Readings: readingsFiles(f.state),
		Events:   events.Files(ledger.Dir(f.state), logs.For("test")),
	}
}

// configuredIDs are the ids of the accounts the fixture's config configures,
// as a router starting on it routes under.
func (f fixture) configuredIDs(t *testing.T) []string {
	t.Helper()
	draft, err := config.Edit(f.config)
	if err != nil {
		t.Fatal(err)
	}
	return draft.Config().Accounts.IDs()
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
			if got := f.snapshot(t); !maps.Equal(got, want) {
				t.Errorf("stopped after %s and run on, the install differs from one never stopped:\n%s", at, differences(want, got))
			}
		})
	}
}

func TestASummarisingRoundBesideTheMoveKeepsEveryTotal(t *testing.T) {
	f := newFixture(t)
	before := f.tallies(t)
	h := f.historyOf()
	done := make(chan struct{})
	var rounds sync.WaitGroup
	rounds.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
				h.Ledger.SummariseEnded(now)
			}
		}
	})
	m := f.mover(answering())
	m.History = &h
	err := m.Run(t.Context())
	close(done)
	rounds.Wait()
	if err != nil {
		t.Fatalf("Run() = %v", err)
	}
	h.Ledger.SummariseEnded(now)

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
	if healed, err := idmove.Heal(f.config, f.state); err != nil || healed {
		t.Fatalf("Heal() before the move = %t, %v, want nothing healed", healed, err)
	}
	f.moved(t, answering())

	writeFile(t, kept, fixtureConfig)
	healed, err := idmove.Heal(f.config, f.state)
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
	if healed, err := idmove.Heal(f.config, f.state); err != nil || healed {
		t.Errorf("Heal() again = %t, %v, want nothing left to heal", healed, err)
	}
}

func TestADamagedCompressedDayIsLeftAsItWasAndTheRestMoved(t *testing.T) {
	f := newFixture(t)
	damaged := map[string][]byte{
		readingsFiles(f.state).CompressedPath("2026-11-01"):                              []byte("not gzip, nor ever was\n"),
		ledger.Files(ledger.Dir(f.state), logs.For("test")).CompressedPath("2026-11-01"): nil,
	}
	for path, data := range damaged {
		writeFile(t, path, string(data))
	}
	f.runBounded(t, answering())

	for path, data := range damaged {
		if got, err := os.ReadFile(path); err != nil || string(got) != string(data) {
			t.Errorf("%s holds %q, %v, want it left as it was, %q", filepath.Base(path), got, err, data)
		}
	}
	if got := readFile(t, filepath.Join(readings.Dir(f.state), "readings-"+today+".jsonl")); !strings.Contains(got, `"`+workID+`"`) {
		t.Errorf("today's readings weren't moved:\n%s", got)
	}
}
