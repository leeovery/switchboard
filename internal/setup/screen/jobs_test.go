package screen

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// overlap is how long a test gives a job it holds back to start all the
// same, should nothing hold it.
const overlap = 50 * time.Millisecond

// done is a job's outcome.
type done struct{ name string }

func TestAJobsOutcomeComesBackAsAMessage(t *testing.T) {
	js := newJobs(t.Context())
	t.Cleanup(js.wait)
	if got := js.read(func(context.Context) tea.Msg { return done{"read"} })(); got != (done{"read"}) {
		t.Errorf("a read gives %v, want its outcome", got)
	}
	if got := js.write(func(context.Context) tea.Msg { return done{"write"} })(); got != (done{"write"}) {
		t.Errorf("a write gives %v, want its outcome", got)
	}
}

// endedUnheard is a context that ends once end is called, but never says so
// on Done, so a select waiting on it always takes what else comes: only Err
// says it's ended.
type endedUnheard struct {
	context.Context
	ended atomic.Bool
}

func (c *endedUnheard) Done() <-chan struct{} { return nil }

func (c *endedUnheard) Err() error {
	if c.ended.Load() {
		return context.Canceled
	}
	return nil
}

func TestAnOutcomeOnceTheContextIsDoneIsDropped(t *testing.T) {
	ctx := &endedUnheard{Context: t.Context()}
	js := newJobs(ctx)
	t.Cleanup(js.wait)
	answered := func(context.Context) tea.Msg {
		ctx.ended.Store(true)
		return done{"answered as its context ended"}
	}
	if got := js.read(answered)(); got != nil {
		t.Errorf("a job answering once its context is done gives %v, want nothing", got)
	}
}

func TestAWriteWaitingItsTurnAsTheContextEndsNeverRuns(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	js := newJobs(ctx)
	js.writing <- struct{}{}
	var ran atomic.Bool
	go js.write(func(context.Context) tea.Msg {
		ran.Store(true)
		return nil
	})()
	// Time for the write to reach the lock, so the context ends as it waits
	// there: a check before the lock alone would let it run.
	time.Sleep(overlap)
	cancel()
	<-js.writing
	js.wait()
	if ran.Load() {
		t.Error("a write whose context ended as it waited its turn ran, want it never run")
	}
}

func TestAWriteWaitingItsTurnStopsWaitingAsTheContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	js := newJobs(ctx)
	// The turn is taken and never given back, as by a writing action that
	// hangs.
	js.writing <- struct{}{}
	var ran atomic.Bool
	go js.write(func(context.Context) tea.Msg {
		ran.Store(true)
		return nil
	})()
	// Time for the write to reach its turn, so the context ends as it waits
	// there.
	time.Sleep(overlap)
	cancel()

	waited := make(chan struct{})
	go func() {
		js.wait()
		close(waited)
	}()
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("wait waited on a write still waiting its turn once the context ended, want the write to stop waiting")
	}
	if ran.Load() {
		t.Error("a write whose context ended as it waited its turn ran, want it never run")
	}
}

func TestAJobsPanicIsLoggedAndPanickedAgainByItsCommand(t *testing.T) {
	log := logstest.Capture(t)
	js := newJobs(t.Context())
	t.Cleanup(js.wait)
	defer func() {
		p, ok := recover().(jobPanic)
		if !ok || p.value != "a bug" || !strings.Contains(p.String(), "a bug") {
			t.Errorf("the command panicked with %v, want the job's panic, a bug", p)
		}
		if !log.Has("level=ERROR", "panicked", "panic=", "a bug", "stack=") {
			t.Errorf("the log reads\n%s\nwant the job's panic, and where it was", log)
		}
	}()
	js.read(func(context.Context) tea.Msg { panic("a bug") })()
	t.Error("the command returned, want it to panic as the job did")
}

func TestAPanicTakenOnceTheContextHasEndedIsntPanickedAgain(t *testing.T) {
	ctx := &endedUnheard{Context: t.Context()}
	ctx.ended.Store(true)
	js := newJobs(ctx)
	defer func() {
		if p := recover(); p != nil {
			t.Errorf("taking a job's panic once its context has ended panicked with %v, want nothing", p)
		}
	}()
	if got := js.given(jobPanic{value: "a bug"}); got != nil {
		t.Errorf("a job's panic taken once its context has ended gives %v, want nothing", got)
	}
}

func TestAJobsPanicOnceTheContextIsDoneIsLogged(t *testing.T) {
	log := logstest.Capture(t)
	ctx, cancel := context.WithCancel(t.Context())
	js := newJobs(ctx)
	started, panicking := make(chan struct{}), make(chan struct{})
	go js.read(func(context.Context) tea.Msg {
		close(started)
		<-panicking
		panic("a bug")
	})()
	<-started
	cancel()
	close(panicking)
	js.wait()
	if !log.Has("level=ERROR", "panicked", "panic=") {
		t.Errorf("the log reads\n%s\nwant the job's panic", log)
	}
}

func TestAJobWaitedForOnceItsContextIsDoneGivesNothing(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	js := newJobs(ctx)
	t.Cleanup(js.wait)
	release := make(chan struct{})
	given := make(chan tea.Msg)
	go func() {
		given <- js.read(func(context.Context) tea.Msg {
			<-release
			return done{"late"}
		})()
	}()
	cancel()
	if got := <-given; got != nil {
		t.Errorf("a job still running as its context ends gives %v, want nothing", got)
	}
	close(release)
}

func TestWritesRunOneAtATimeAndReadsBesideThem(t *testing.T) {
	js := newJobs(t.Context())
	t.Cleanup(js.wait)
	first, release := make(chan struct{}), make(chan struct{})
	go js.write(func(context.Context) tea.Msg {
		close(first)
		<-release
		return nil
	})()
	<-first

	second := make(chan struct{})
	go js.write(func(context.Context) tea.Msg {
		close(second)
		return nil
	})()
	if got := js.read(func(context.Context) tea.Msg { return done{"read"} })(); got != (done{"read"}) {
		t.Errorf("a read beside a write gives %v, want its outcome while the write runs", got)
	}
	select {
	case <-second:
		t.Fatal("a second write started while the first ran, want it to wait its turn")
	case <-time.After(overlap):
	}
	close(release)
	<-second
}

func TestNoJobStartsOnceWaitHasBegun(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	js := newJobs(ctx)
	cancel()
	js.wait()
	ran := false
	if got := js.read(func(context.Context) tea.Msg { ran = true; return done{"late"} })(); got != nil || ran {
		t.Errorf("a job after wait gives %v, ran %v; want it never started", got, ran)
	}
}

func TestWaitWaitsForEveryJobStillRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	js := newJobs(ctx)
	started, ended := make(chan struct{}), make(chan struct{})
	go js.read(func(ctx context.Context) tea.Msg {
		close(started)
		<-ctx.Done()
		time.Sleep(overlap)
		close(ended)
		return nil
	})()
	<-started
	cancel()
	js.wait()
	select {
	case <-ended:
	default:
		t.Error("wait returned with a job still running, want it to wait for it to end")
	}
}
