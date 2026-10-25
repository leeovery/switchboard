package ledger

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
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
