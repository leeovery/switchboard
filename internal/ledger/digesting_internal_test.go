package ledger

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/logs"
)

// runDigesting runs a digesting of a ledger of no days, in the test's
// synctest bubble, returning it, how many passes it has made, and what ends
// its run, done closed as the run returns.
func runDigesting(t *testing.T) (d *digesting, passes *atomic.Int32, cancel context.CancelFunc, done <-chan struct{}) {
	t.Helper()
	passes = new(atomic.Int32)
	d = newDigesting(nil, digestDays(t), func() time.Time {
		passes.Add(1)
		return time.Now()
	})
	ctx, cancel := context.WithCancel(t.Context())
	ended := make(chan struct{})
	go func() {
		d.run(ctx)
		close(ended)
	}()
	return d, passes, cancel, ended
}

// Run's digesting makes no pass before its first kick, which the writer's
// first round gives as it ends, however long that round takes; then one at
// each kick.
func TestRunsDigestingPassesFirstAtItsFirstKick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		d, passes, cancel, done := runDigesting(t)
		defer func() {
			cancel()
			<-done
		}()

		time.Sleep(time.Hour)
		synctest.Wait()
		if n := passes.Load(); n != 0 {
			t.Errorf("before a kick, %d passes were made, want none", n)
		}
		for kicks := range int32(2) {
			d.kick()
			synctest.Wait()
			if n := passes.Load(); n != kicks+1 {
				t.Errorf("after %d kicks, %d passes were made, want %d", kicks+1, n, kicks+1)
			}
		}
	})
}

// A round kicks the digesting only once it leaves no day to summarise again
// for a later one, so no pass finds a day whose summary is still of an older
// version, which it can't tell the day's lines against.
func TestARoundKicksTheDigestingOnceItLeavesNoDayForALaterOne(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local)
	for back := 1; back <= summariseAtOnce+1; back++ {
		day := at.AddDate(0, 0, -back)
		date := dayfile.DateOf(day)
		// A day of a request, summarised under version 5.
		files := map[string]string{
			filepath.Join(dir, "requests-"+date+".jsonl"): `{"at":"` + day.UTC().Format(time.RFC3339) + `","request":"1","kind":"message","status":200}`,
			SummaryFile(dir, date):                        `{"version":5,"day":"` + date + `","lines":1}`,
		}
		for path, data := range files {
			if err := os.WriteFile(path, []byte(data+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	l := Open(dir, 400*24*time.Hour, func() time.Time { return at }, &asking{}, Caps{}, logs.For("router"))
	l.digesting = newDigesting(nil, l.days, func() time.Time { return at })

	for _, more := range []bool{true, false} {
		got := l.SummariseSome(at)
		if kicked := len(l.digesting.kicks) == 1; got != more || kicked == more {
			t.Errorf("SummariseSome() = %t, the digesting kicked %t; want %t, kicked %t: once no day is left for a later round", got, kicked, more, !more)
		}
	}
}

// Run's digesting ends as its context does, though no kick ever came, as
// where Run's context ends during the writer's first round.
func TestRunsDigestingEndsWithItsContextBeforeItsFirstKick(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		_, passes, cancel, done := runDigesting(t)
		synctest.Wait()

		cancel()
		synctest.Wait()
		select {
		case <-done:
		default:
			t.Fatal("the digesting waits on with its context ended, want it ended")
		}
		if n := passes.Load(); n != 0 {
			t.Errorf("%d passes were made, want none", n)
		}
	})
}
