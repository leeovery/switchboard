package screen

import (
	"context"
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
// has begun before it starts.
func (js *jobs) read(j job) tea.Cmd {
	return func() tea.Msg {
		outcome, started := js.start(j)
		if !started {
			return nil
		}
		select {
		case msg := <-outcome:
			if js.ctx.Err() != nil {
				return nil
			}
			return msg
		case <-js.ctx.Done():
			return nil
		}
	}
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
// channel it returns, reporting false where wait has begun.
func (js *jobs) start(j job) (<-chan tea.Msg, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	if js.stopped {
		return nil, false
	}
	outcome := make(chan tea.Msg, 1)
	js.running.Go(func() { outcome <- j(js.ctx) })
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
