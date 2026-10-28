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
	// writing holds setup's writing actions to one at a time.
	writing sync.Mutex
}

// newJobs is jobs run under ctx.
func newJobs(ctx context.Context) *jobs {
	return &jobs{ctx: ctx}
}

// read is a command that runs j, beside whatever else runs, and waits for
// its outcome, giving it unless the jobs' context is done by then, or wait
// has begun before it starts. Where j panicked, the command panics again,
// as Bubble Tea recovers from a command's panic, putting the terminal back,
// as it can't from a goroutine of the jobs' own.
func (js *jobs) read(j job) tea.Cmd {
	return func() tea.Msg {
		outcome, started := js.start(j)
		if !started {
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
// is done; a job's panic, its panic again, but once the context is done,
// with nothing left to recover from it, logged.
func (js *jobs) given(msg tea.Msg) tea.Msg {
	p, panicked := msg.(jobPanic)
	switch {
	case panicked && js.ctx.Err() == nil:
		panic(p)
	case panicked:
		p.log()
	case js.ctx.Err() == nil:
		return msg
	}
	return nil
}

// write is a command that runs j once no other writing action runs, as read
// runs it: none where the jobs' context is done before its turn.
func (js *jobs) write(j job) tea.Cmd {
	return js.read(func(ctx context.Context) tea.Msg {
		js.writing.Lock()
		defer js.writing.Unlock()
		if ctx.Err() != nil {
			return nil
		}
		return j(ctx)
	})
}

// start runs j on a goroutine wait waits for, its outcome given on the
// channel it returns, reporting false where wait has begun. A panic of j's
// is its outcome, but once the jobs' context is done, when no command waits
// to panic with it again, it's logged.
func (js *jobs) start(j job) (<-chan tea.Msg, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.stopped {
		return nil, false
	}
	outcome := make(chan tea.Msg, 1)
	js.running.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				p := jobPanic{value: r, stack: debug.Stack()}
				if js.ctx.Err() != nil {
					p.log()
					return
				}
				outcome <- p
			}
		}()
		outcome <- j(js.ctx)
	})
	return outcome, true
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

// log notes the panic in the log, as nothing is left to recover from it.
func (p jobPanic) log() {
	logger.Error("a job of setup's panicked as setup's screen stopped", "panic", fmt.Sprint(p.value), "stack", string(p.stack))
}
