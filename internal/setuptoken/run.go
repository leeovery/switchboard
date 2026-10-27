package setuptoken

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/tokens"
)

const (
	// lookEvery is how often the shim's directory is looked at for the
	// address it's handed.
	lookEvery = 50 * time.Millisecond
	// drainFor bounds the reading of what setup-token drew last, once it has
	// exited: anything it left running can hold its terminal open.
	drainFor = 2 * time.Second
	// readSize is how much of what setup-token draws is read at a time.
	readSize = 32 << 10
)

// run is one run of setup-token, and what Make has seen of it.
type run struct {
	signing Signing
	shim    shim
	screen  *screen
	// master is the pseudo-terminal's master, which setup-token's drawing is
	// read from.
	master *os.File
	// stop stops setup-token, killing its process group, while it runs.
	// group is that group's id, setup-token's process id.
	stop  context.CancelFunc
	group int
	// output carries what setup-token draws, and is closed once it has drawn
	// its last; exited carries how it exited. Each is nil once it's done.
	output <-chan []byte
	exited <-chan error
	// done tells the reading of output to stop.
	done chan struct{}
	wg   sync.WaitGroup

	// ended is the end of Make's context, waited on until the token's made
	// (see cancel), then nil.
	ended <-chan struct{}
	// The timers Make waits on, each nil until it's started, or once it's no
	// longer waited on: looking, for the shim's directory to be looked at;
	// handOver, for setup-token to have handed its address over; graceOver,
	// for it to have exited once it has made the token; drained, for its
	// drawing to have been read once it has exited.
	looking, handOver, graceOver, drained <-chan time.Time
	waits                                 waits

	// made says setup-token has said it made the token, and shown that it
	// has shown it whole since, as token. exit is how it exited, once it has,
	// and stopped says Make stopped it, its grace to exit over, before ctx
	// ended.
	made, shown bool
	exit        error
	stopped     bool

	// over says Make has its answer: the token, or why there's none.
	over  bool
	token tokens.Token
	err   error
}

// begin starts setup-token, the claude at path, in env, as it's made for it
// (see forSetupToken), on a pseudo-terminal, with the shim as its browser.
func begin(ctx context.Context, path string, env environ, s Signing) (*run, error) {
	sh, err := newShim()
	if err != nil {
		return nil, fmt.Errorf("make setup-token's browser: %w", err)
	}
	r := &run{signing: s, shim: sh, screen: newScreen(columns, rows), done: make(chan struct{})}
	if err := r.start(ctx, path, env); err != nil {
		r.close()
		return nil, err
	}
	return r, nil
}

// start starts setup-token, and the reading of what it draws, and the wait
// for it to exit.
func (r *run) start(ctx context.Context, path string, env environ) error {
	master, tty, err := openTerminal(columns, rows)
	if err != nil {
		return fmt.Errorf("open a pseudo-terminal for setup-token: %w", err)
	}
	r.master = master
	runs, stop := context.WithCancel(ctx)
	r.stop = stop
	cmd, err := start(runs, path, []string{claude.Command, claude.SetupTokenCommand}, env.forSetupToken(path, r.shim.browser()), tty)
	// From here setup-token alone holds its terminal open, so what it draws
	// ends once it, and all it started there, are gone.
	_ = tty.Close()
	switch {
	case err != nil && ctx.Err() != nil:
		return cancelled(ctx)
	case err != nil:
		return fmt.Errorf("start claude %s at %s: %w", claude.SetupTokenCommand, path, err)
	}
	r.group = cmd.Process.Pid
	output := make(chan []byte)
	// It holds one, so the wait never waits on Make.
	exited := make(chan error, 1)
	r.output, r.exited = output, exited
	r.wg.Go(func() { r.read(output) })
	r.wg.Go(func() { exited <- cmd.Wait() })
	return nil
}

// read sends output what setup-token draws, a read at a time, and closes it
// once the terminal is closed, at either end, or Make stops it.
func (r *run) read(output chan<- []byte) {
	defer close(output)
	for {
		buf := make([]byte, readSize)
		n, err := r.master.Read(buf)
		if n > 0 {
			select {
			case output <- buf[:n]:
			case <-r.done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// close stops setup-token, should it still be running, all it started on its
// terminal with it, closes its terminal, waits for the reading of it and for
// setup-token's exit, and removes the shim.
func (r *run) close() {
	if r.stop != nil {
		r.stop()
	}
	if r.group != 0 {
		// What setup-token left running goes too, before Make waits on
		// anything, as what's deaf to the hang-up setup-token's exit sent
		// could hold its terminal open. Killing a group whose leader may be
		// gone looks unsafe, but no new process takes the group's id while
		// anything's left in it, and with nothing left there's nothing to
		// kill: the id comes round again only once the system has handed out
		// every other.
		_ = killGroup(r.group)
	}
	close(r.done)
	if r.master != nil {
		_ = r.master.Close()
	}
	r.wg.Wait()
	r.shim.remove()
}

// wait waits on setup-token, as Make says, as long as w says.
func (r *run) wait(ctx context.Context, w waits) (tokens.Token, error) {
	look := time.NewTicker(lookEvery)
	defer look.Stop()
	r.ended, r.looking, r.handOver, r.waits = ctx.Done(), look.C, time.After(w.handOver), w
	for !r.over {
		r.next(ctx)
	}
	if !r.made && r.stoppedByCancel(ctx) {
		// The end of ctx stops setup-token, so its exit, and the end of its
		// drawing, can come to Make first, as when it's busy as ctx ends:
		// what happened is the cancel all the same.
		return tokens.Token{}, cancelled(ctx)
	}
	return r.token, r.err
}

// stoppedByCancel reports whether the end of ctx is what stopped setup-token:
// ctx has ended, and setup-token was killed, but not by Make.
func (r *run) stoppedByCancel(ctx context.Context) bool {
	exit, ok := errors.AsType[*exec.ExitError](r.exit)
	return ctx.Err() != nil && !r.stopped && ok && !exit.Exited()
}

// next takes the next thing to happen.
func (r *run) next(ctx context.Context) {
	select {
	case <-r.ended:
		r.cancel(ctx)
	case drawn, ok := <-r.output:
		r.draw(ctx, drawn, ok)
	case err := <-r.exited:
		r.exitedWith(ctx, err)
	case <-r.looking:
		r.lookForAddress(ctx)
	case <-r.handOver:
		r.fail(notHandedOver(r.screen.drawing().text()))
	case <-r.graceOver:
		r.graceOver, r.stopped = nil, ctx.Err() == nil
		r.stop()
	case <-r.drained:
		r.conclude(ctx)
	}
}

// cancel follows the end of ctx, which stops setup-token. Before the token's
// made, Make fails with its cause, once it has drawn what setup-token drew
// that's ready to be read, as that can say the token's made, drawn as ctx
// ended. Once it's made, Make reads it from
// what setup-token drew all the same, as when setup-token exits by itself: it
// lasts a year on the account, and nobody could see it were Make to drop it.
func (r *run) cancel(ctx context.Context) {
	if !r.made {
		select {
		case drawn, ok := <-r.output:
			r.draw(ctx, drawn, ok)
		default:
		}
	}
	switch {
	case r.over:
	case !r.made:
		r.fail(cancelled(ctx))
	default:
		r.ended = nil
	}
}

// cancelled is Make failing as ctx ends.
func cancelled(ctx context.Context) error {
	return fmt.Errorf("make a setup token: %w", context.Cause(ctx))
}

// draw draws what setup-token drew, and follows what it says: that it has made
// the token, which it's then given grace to exit after, whether it handed its
// address over or not, and the token, kept the first time it's shown whole,
// as a frame drawn after can take it off the screen; or an error it waits to
// retry at, where Make stops. Its drawing over, ok false, and setup-token
// gone, Make concludes.
func (r *run) draw(ctx context.Context, drawn []byte, ok bool) {
	if !ok {
		r.output = nil
		if r.exited == nil {
			r.conclude(ctx)
		}
		return
	}
	r.screen.write(drawn)
	if r.shown {
		return
	}
	d := r.screen.drawing()
	if !r.made && d.made() {
		r.made = true
		r.looking, r.handOver, r.graceOver = nil, nil, time.After(r.waits.grace)
		r.report(Event{Step: StepMade})
	}
	if r.made {
		r.token, r.shown = d.token()
		return
	}
	if message, failed := d.failed(); failed {
		r.fail(failedAt(d.text(), message))
	}
}

// exitedWith notes setup-token's exit, as err says, and concludes once its
// drawing is over, or drainFor after at most.
func (r *run) exitedWith(ctx context.Context, err error) {
	r.exited, r.exit = nil, err
	r.looking, r.handOver = nil, nil
	if r.output == nil {
		r.conclude(ctx)
		return
	}
	r.drained = time.After(drainFor)
}

// lookForAddress looks for the sign-in address setup-token handed the shim,
// and opens it with the Opener, reporting it, and the wait on its approval
// that follows. Something handed over that isn't an address fails.
func (r *run) lookForAddress(ctx context.Context) {
	raw, ok := r.shim.address()
	if !ok {
		return
	}
	r.looking, r.handOver = nil, nil
	address, ok := newAddress(raw)
	if !ok {
		r.fail(notAnAddress(r.screen.drawing().text()))
		return
	}
	opened, err := r.signing.Opener.Open(ctx, address)
	if ctx.Err() != nil {
		return
	}
	r.report(Event{Step: StepAddress, Address: address, Opened: opened, Unopened: err})
	r.report(Event{Step: StepWaiting})
}

// conclude takes the token setup-token showed whole, or fails with what it
// drew last, saying why there's none: for a token made but never shown whole,
// the end of ctx, where that's what stopped setup-token drawing it.
func (r *run) conclude(ctx context.Context) {
	if r.shown {
		r.over = true
		return
	}
	d := r.screen.drawing()
	if r.made {
		var cause error
		if r.stoppedByCancel(ctx) {
			cause = context.Cause(ctx)
		}
		r.fail(unreadable(d.text(), cause))
		return
	}
	if message, failed := d.failed(); failed {
		r.fail(failedAt(d.text(), message))
		return
	}
	r.fail(exitedWithout(d.text(), r.exit))
}

func (r *run) fail(err error) {
	r.over, r.err = true, err
}

func (r *run) report(e Event) {
	if r.signing.Report != nil {
		r.signing.Report(e)
	}
}
