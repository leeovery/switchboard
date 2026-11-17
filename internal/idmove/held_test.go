package idmove_test

import (
	"context"
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

	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
)

// heldLives runs the move of the fixture's install as lives does, each
// router holding its request ledger's rounds as the router does: from its
// start on the ids the move renames onto, as Settle reports it, or on a
// Settle that fails, till the move's done. during is told of each step as
// lives' stop is, and of "starting life n" just before the nth router
// settles, given what makes a round of that router's ledger, nil before it
// has one; where it fails, that router stops there.
func (f fixture) heldLives(t *testing.T, during func(step string, round func()) error) {
	t.Helper()
	for life := 1; !idmove.Moved(f.state); life++ {
		if life > 6 {
			t.Fatalf("the move isn't done after %d routers", life-1)
		}
		m := f.mover(answering())
		m.Routing = f.configuredIDs(t)
		if during(fmt.Sprintf("starting life %d", life), nil) != nil {
			continue
		}
		settled, unsettled := m.Settle()
		h := f.historyOf(ledger.Holding(func() bool { return (settled || unsettled != nil) && !idmove.Moved(f.state) }))
		f.live(t, m, h, life, during)
	}
}

// live is the nth life of a router heldLives runs, of the history h: it makes
// a round as it starts, once it has settled, and once its move has run,
// unless it stopped.
func (f fixture) live(t *testing.T, m *idmove.Mover, h idmove.History, life int, during func(step string, round func()) error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	var running sync.WaitGroup
	running.Go(func() { h.Ledger.Run(ctx) })
	defer func() {
		cancel()
		running.Wait()
	}()
	round := func() {
		if err := h.Ledger.RoundNow(ctx); err != nil {
			t.Fatalf("RoundNow() = %v", err)
		}
	}
	round()
	m.StopAfter(func(step string) error { return during(step, round) })
	if during(fmt.Sprintf("settled in life %d", life), round) != nil {
		return
	}
	err := m.RunBeside(ctx, h)
	switch {
	case errors.Is(err, errStopped):
		return
	case err != nil && !errors.Is(err, idmove.ErrAwaitingRestart):
		t.Fatalf("RunBeside() = %v", err)
	}
	round()
}

// ledgerDay is the step the move tells of once it has renamed the request
// ledger's day with the given date, its lines and its summary.
func ledgerDay(date string) string {
	return ledger.Files("", logs.For("test")).Name + " " + date
}

// betweenLinesAndSummary stops the move once it has renamed yesterday's
// lines, before it renames their summary, as a crash between the two does:
// the summary is put back as it was before the move came to the day.
type betweenLinesAndSummary struct {
	t       *testing.T
	f       fixture
	held    string
	stopped bool
}

func (b *betweenLinesAndSummary) during(step string, _ func()) error {
	path := ledger.SummaryFile(ledger.Dir(b.f.state), yesterday)
	switch {
	case b.stopped:
	case step == ledgerDay(longAgo):
		b.held = readFile(b.t, path)
	case step == ledgerDay(yesterday):
		writeFile(b.t, path, b.held)
		b.stopped = true
		return errStopped
	}
	return nil
}

// lateLine files a line under work's new id on yesterday, as a request that
// arrived before midnight and ended once the router started on the new ids
// is filed, as the first router on them has settled, so yesterday's summary
// no longer stands; and, where round is set, has that router make a round.
type lateLine struct {
	t     *testing.T
	f     fixture
	round bool
	filed bool
}

func (l *lateLine) during(step string, round func()) error {
	if l.filed || !strings.HasPrefix(step, "settled in life") || !slices.Contains(l.f.configuredIDs(l.t), workID) {
		return nil
	}
	file, err := os.OpenFile(filepath.Join(ledger.Dir(l.f.state), "requests-"+yesterday+".jsonl"), os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	_, err = file.WriteString(line(yesterday, "r8", workID, status.ReasonNew, "", nil) + "\n")
	if err := errors.Join(err, file.Close()); err != nil {
		return err
	}
	l.filed = true
	if l.round {
		round()
	}
	return nil
}

// acrossMidnight has work's 5-hour window read late on the day before
// yesterday and again in yesterday's first hour, before it resets, at the
// uses given, rejected at a use of 1, and the summaries of both days made
// again: yesterday's rise is from the use it began the day at, and a window
// rejected as the day began reaches no limit in it.
func acrossMidnight(late, early float64) func(t *testing.T, f fixture) {
	return func(t *testing.T, f fixture) {
		t.Helper()
		const dayBefore = "2026-11-06"
		resets := time.Date(2026, 11, 7, 2, 0, 0, 0, time.Local)
		read := func(at time.Time, use float64) string {
			r := readings.Reading{At: at, Account: work, Key: "five_hour", Utilization: use, ResetsAt: resets, Status: quota.StatusAllowed,
				Source: readings.FromAnswer}
			if use >= 1 {
				r.Status = quota.StatusRejected
			}
			return jsonOf(r)
		}
		files := readingsFiles(f.state)
		putDay(t, files, dayBefore, false, read(time.Date(2026, 11, 6, 23, 30, 0, 0, time.Local), late))
		putDay(t, files, yesterday, false, read(time.Date(2026, 11, 7, 0, 30, 0, 0, time.Local), early), reading(yesterday, work, 0.5),
			reading(yesterday, personal, 0.3))
		for _, date := range []string{dayBefore, yesterday} {
			if err := os.Remove(ledger.SummaryFile(ledger.Dir(f.state), date)); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
		}
		f.historyOf().Ledger.SummariseEnded(now)
	}
}

// together makes each of the moments durings make, in turn, at each step.
func together(durings ...func(step string, round func()) error) func(step string, round func()) error {
	return func(step string, round func()) error {
		for _, during := range durings {
			if err := during(step, round); err != nil {
				return err
			}
		}
		return nil
	}
}

// A router holds its rounds from its start on the new ids till the move's
// done, so no round summarises a day the move has renamed only in part: at
// each of the moves's hard moments, each day's every request, rise and limit
// is counted once, under its account's new id.
func TestARouterHoldingItsRoundsThroughTheMoveCountsEachDayOnce(t *testing.T) {
	// Each moment files lateLine's line, so yesterday's summary no longer
	// stands, and a round would summarise the day again: the next router's
	// first, of a move stopped, else one as the line's filed.
	tests := []struct {
		name string
		// lay lays out more of the fixture's history before the move.
		lay func(t *testing.T, f fixture)
		// during makes the moment, as heldLives's during, and reports
		// whether the move came to it.
		during func(t *testing.T, f fixture) (func(step string, round func()) error, func() bool)
	}{
		{
			name: "stopped between a day's lines and its summary",
			during: func(t *testing.T, f fixture) (func(string, func()) error, func() bool) {
				l, b := &lateLine{t: t, f: f}, &betweenLinesAndSummary{t: t, f: f}
				return together(l.during, b.during), func() bool { return l.filed && b.stopped }
			},
		},
		{
			name: "stopped so, the next router starting unable to read the config",
			during: func(t *testing.T, f fixture) (func(string, func()) error, func() bool) {
				l, b := &lateLine{t: t, f: f}, &betweenLinesAndSummary{t: t, f: f}
				broken, mended := false, false
				unreadable := func(step string, _ func()) error {
					switch {
					case b.stopped && !broken && strings.HasPrefix(step, "starting life"):
						broken = true
						chmod(t, f.config, 0)
					case broken && !mended && strings.HasPrefix(step, "settled in life"):
						mended = true
						chmod(t, f.config, 0o600)
					}
					return nil
				}
				return together(l.during, unreadable, b.during), func() bool { return l.filed && b.stopped && mended }
			},
		},
		{
			name: "yesterday summarised again as the move runs, its window read across midnight",
			lay:  acrossMidnight(0.2, 0.4),
			during: func(t *testing.T, f fixture) (func(string, func()) error, func() bool) {
				l := &lateLine{t: t, f: f, round: true}
				return l.during, func() bool { return l.filed }
			},
		},
		{
			name: "yesterday summarised again so, its window rejected across midnight",
			lay:  acrossMidnight(1, 1),
			during: func(t *testing.T, f fixture) (func(string, func()) error, func() bool) {
				l := &lateLine{t: t, f: f, round: true}
				return l.during, func() bool { return l.filed }
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			if tt.lay != nil {
				tt.lay(t, f)
			}
			before := f.tallies(t)
			during, reached := tt.during(t, f)
			f.heldLives(t, during)
			if !reached() {
				t.Fatal("the move never came to the moment the test makes")
			}

			rename := f.renamer(t)
			want := make(map[string]int, len(before))
			for key, n := range before {
				kind, date, account := splitKey(key)
				want[kind+"|"+date+"|"+rename(account)] += n
			}
			for _, kind := range []string{"requests.account", "summary", "summary.pools"} {
				want[kind+"|"+yesterday+"|"+workID]++
			}
			want["summary.input|"+yesterday+"|"+workID] += 10
			want["requests.from|"+yesterday+"|"]++
			if got := f.tallies(t); !maps.Equal(got, want) {
				t.Errorf("once the move's done, the days' totals by account differ from those before it, renamed:\n%s", tallyDifferences(want, got))
			}
		})
	}
}

// chmod sets the mode of the file at path.
func chmod(t *testing.T, path string, mode os.FileMode) {
	t.Helper()
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// tallyDifferences says how got differs from want, tallies as tallies counts
// them, a line a key.
func tallyDifferences(want, got map[string]int) string {
	keys := slices.Concat(slices.Collect(maps.Keys(want)), slices.Collect(maps.Keys(got)))
	slices.Sort(keys)
	var said []string
	for _, key := range slices.Compact(keys) {
		if want[key] != got[key] {
			said = append(said, fmt.Sprintf("%s: want %d, got %d", key, want[key], got[key]))
		}
	}
	return strings.Join(said, "\n")
}
