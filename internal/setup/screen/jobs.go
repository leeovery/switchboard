package screen

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"

	tea "charm.land/bubbletea/v2"
)

// job is long work of setup's, done under ctx, its outcome a message for the
// model to take in.
type job func(ctx context.Context) tea.Msg

// jobs runs setup's long work off Update, each job on a goroutine of its
// own, its outcome coming back as a message over a channel, until wait has
// waited for every one still running. Setup's writing actions run one at a
// time, as setup keeps what they've changed unguarded; its reads may run
// beside them.
type jobs struct {
	// ctx is what every job runs under: once it's done, a job's outcome is
	// dropped.
	ctx context.Context
	// mu guards stopped, set once wait has begun, after which no job starts.
	mu      sync.Mutex
	stopped bool
	running sync.WaitGroup
	// writing holds setup's writing actions to one at a time: the write
	// running holds its one place. It's a channel, not a mutex, so a write
	// waiting its turn can stop waiting once the context is done.
	writing chan struct{}
}

// newJobs is jobs run under ctx.
func newJobs(ctx context.Context) *jobs {
	return &jobs{ctx: ctx, writing: make(chan struct{}, 1)}
}

// read is a command that runs j, beside whatever else runs, and waits for
// its outcome, giving it unless the jobs' context is done by then, or wait
// has begun before it starts. Where j panicked, the command panics again,
// as Bubble Tea recovers from a command's panic, putting the terminal back,
// as it can't from a goroutine of the jobs' own.
func (js *jobs) read(j job) tea.Cmd {
	return func() tea.Msg {
		outcome := make(chan tea.Msg, 1)
		if !js.start(j, outcome) {
			return nil
		}
		select {
		case msg := <-outcome:
			return js.given(msg)
		case <-js.ctx.Done():
			return nil
		}
	}
}

// given is a job's outcome, as read gives it: nothing once the jobs' context
// is done; a job's panic, while the context lasts, its panic again, for
// Bubble Tea to recover from.
func (js *jobs) given(msg tea.Msg) tea.Msg {
	switch p, panicked := msg.(jobPanic); {
	case js.ctx.Err() != nil:
		return nil
	case panicked:
		panic(p)
	}
	return msg
}

// write is a command that runs j as a writing action, as held has it, as
// read runs it.
func (js *jobs) write(j job) tea.Cmd {
	return js.read(js.held(j))
}

// held is j run once no other writing action runs, as turn has it: not at
// all where the jobs' context is done before its turn.
func (js *jobs) held(j job) job {
	return func(ctx context.Context) tea.Msg {
		done, ok := js.turn(ctx)
		if !ok {
			return nil
		}
		defer done()
		return j(ctx)
	}
}

// turn waits till no other writing action runs, then takes the writing
// actions' one place, which done gives back, while ctx, the jobs' own or one
// of theirs, lasts: where it's done before its turn, it stops waiting,
// reporting false, holding nothing, so a writing action that hangs holds up
// no other once ctx is done.
func (js *jobs) turn(ctx context.Context) (done func(), ok bool) {
	select {
	case js.writing <- struct{}{}:
	case <-ctx.Done():
		return nil, false
	}
	if ctx.Err() != nil {
		<-js.writing
		return nil, false
	}
	return func() { <-js.writing }, true
}

// begin is a command that starts j, beside whatever else runs, as read
// does, but waits for nothing, giving nothing back itself: j's outcome, or
// its panic, is given on ended, which holds one, for whatever listens for
// it. Nothing is, where wait has begun before it starts.
func (js *jobs) begin(j job, ended chan<- tea.Msg) tea.Cmd {
	return func() tea.Msg {
		js.start(j, ended)
		return nil
	}
}

// writes reports whether a writing action runs now.
func (js *jobs) writes() bool {
	return len(js.writing) > 0
}

// start runs j on a goroutine wait waits for, its outcome given on
// outcome, which must hold it, reporting false where wait has begun. A
// panic of j's is logged as it's recovered from, whatever comes of it
// after, as Bubble Tea writes one it recovers from where the screen hides
// it, and it's j's outcome.
func (js *jobs) start(j job, outcome chan<- tea.Msg) bool {
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.stopped {
		return false
	}
	js.running.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				p := jobPanic{value: r, stack: debug.Stack()}
				logger.Error("a job of setup's panicked", "panic", fmt.Sprint(p.value), "stack", string(p.stack))
				outcome <- p
			}
		}()
		outcome <- j(js.ctx)
	})
	return true
}

// wait waits for every job still running to end, starting none after: the
// jobs' context is to be done first, for them to end.
func (js *jobs) wait() {
	js.mu.Lock()
	js.stopped = true
	js.mu.Unlock()
	js.running.Wait()
}

// jobPanic is a job's panic, and the stack it panicked at, which Bubble Tea
// writes out, should it be panicked with again.
type jobPanic struct {
	value any
	stack []byte
}

// String is the panic, and where it was, as Bubble Tea writes a panic it
// recovers from.
func (p jobPanic) String() string {
	return fmt.Sprintf("%v\n\n%s", p.value, p.stack)
}
