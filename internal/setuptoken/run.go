package setuptoken

import (
	"context"
	"fmt"
	"os"
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

	// The timers Make waits on, each nil until it's started, or once it's no
	// longer waited on: looking, for the shim's directory to be looked at;
	// handOver, for setup-token to have handed its address over; graceOver,
	// for it to have exited once it has made the token; drained, for its
	// drawing to have been read once it has exited.
	looking, handOver, graceOver, drained <-chan time.Time
	waits                                 waits

	// made says setup-token has said it made the token, and exit is how it
	// exited, once it has.
	made bool
	exit error

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
	ctx, r.stop = context.WithCancel(ctx)
	cmd, err := start(ctx, path, []string{claude.Command, claude.SetupTokenCommand}, env.forSetupToken(path, r.shim.browser()), tty)
	// From here setup-token alone holds its terminal open, so what it draws
	// ends once it, and all it started there, are gone.
	_ = tty.Close()
	if err != nil {
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
	close(r.done)
	if r.master != nil {
		_ = r.master.Close()
	}
	r.wg.Wait()
	if r.group != 0 {
		// What setup-token left running, deaf to the hang-up its exit sent,
		// goes too. Killing a group whose leader is gone looks unsafe, but
		// no new process takes the group's id while anything's left in it,
		// and with nothing left there's nothing to kill: the id comes round
		// again only once the system has handed out every other.
		_ = killGroup(r.group)
	}
	r.shim.remove()
}

// wait waits on setup-token, as Make says, as long as w says.
func (r *run) wait(ctx context.Context, w waits) (tokens.Token, error) {
	look := time.NewTicker(lookEvery)
	defer look.Stop()
	r.looking, r.handOver, r.waits = look.C, time.After(w.handOver), w
	for !r.over {
		r.next(ctx)
	}
	return r.token, r.err
}

// next takes the next thing to happen.
func (r *run) next(ctx context.Context) {
	select {
	case <-ctx.Done():
		r.fail(fmt.Errorf("make a setup token: %w", context.Cause(ctx)))
	case drawn, ok := <-r.output:
		r.draw(drawn, ok)
	case err := <-r.exited:
		r.exitedWith(err)
	case <-r.looking:
		r.lookForAddress(ctx)
	case <-r.handOver:
		r.fail(notHandedOver(r.screen.drawing().text()))
	case <-r.graceOver:
		r.graceOver = nil
		r.stop()
	case <-r.drained:
		r.conclude()
	}
}

// draw draws what setup-token drew, and follows what it says: that it has made
// the token, which it's then given grace to exit after, whether it handed its
// address over or not, or an error it waits to retry at, where Make stops. Its
// drawing over, ok false, and setup-token gone, Make reads the token from it.
func (r *run) draw(drawn []byte, ok bool) {
	if !ok {
		r.output = nil
		if r.exited == nil {
			r.conclude()
		}
		return
	}
	r.screen.write(drawn)
	if r.made {
		return
	}
	d := r.screen.drawing()
	if d.made() {
		r.made = true
		r.looking, r.handOver, r.graceOver = nil, nil, time.After(r.waits.grace)
		r.report(Event{Step: StepMade})
		return
	}
	if message, failed := d.failed(); failed {
		r.fail(failedAt(d.text(), message))
	}
}

// exitedWith notes setup-token's exit, as err says, and reads the token from
// its drawing, once that's over, or drainFor at most.
func (r *run) exitedWith(err error) {
	r.exited, r.exit = nil, err
	r.looking, r.handOver = nil, nil
	logger.Debug("setup-token exited", "status", err)
	if r.output == nil {
		r.conclude()
		return
	}
	r.drained = time.After(drainFor)
}

// lookForAddress looks for the sign-in address setup-token handed the shim,
// and opens it with the Opener, reporting it, and the wait on its approval
// that follows. One that isn't an address fails, as setup-token not handing
// one over does.
func (r *run) lookForAddress(ctx context.Context) {
	raw, ok := r.shim.address()
	if !ok {
		return
	}
	r.looking, r.handOver = nil, nil
	address, ok := newAddress(raw)
	if !ok {
		r.fail(notHandedOver(r.screen.drawing().text()))
		return
	}
	opened, err := r.signing.Opener.Open(ctx, address)
	if ctx.Err() != nil {
		return
	}
	r.report(Event{Step: StepAddress, Address: address, Opened: opened, Unopened: err})
	r.report(Event{Step: StepWaiting})
}

// conclude takes the token from what setup-token drew last, or fails with
// what it drew, saying why there's none.
func (r *run) conclude() {
	d := r.screen.drawing()
	if token, ok := d.token(); ok {
		r.over, r.token = true, token
		return
	}
	if d.made() {
		r.fail(unreadable(d.text()))
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
