package setuptoken

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/tokens"
)

const (
	// lookEvery is how often the shim's directory is looked at for the
	// address it's handed.
	lookEvery = 50 * time.Millisecond
	// drainFor bounds the wait on setup-token's exit once Make stops it, and
	// on the end of its drawing once it has exited, each a moment as a rule.
	drainFor = 2 * time.Second
	// readSize is how much of what setup-token draws is read at a time.
	readSize = 32 << 10
	// readsAtDrain bounds the reads Make draws once the drain is over (see
	// drawWaiting), so what draws as fast as Make reads can't hold it there.
	readsAtDrain = 4
	// reportsHeld is how many events Make can report before the Signing's
	// Report hears the first: Make reports each step once (see report).
	reportsHeld = int(StepMade)
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
	// reports carries what Make reports to the Signing's Report; reported is
	// the last step it carried.
	reports  chan Event
	reported Step
	wg       sync.WaitGroup

	// ended is the end of Make's context, waited on until it comes (see
	// cancel), then nil.
	ended <-chan struct{}
	// The timers Make waits on, each nil until it's started, or once it's no
	// longer waited on: looking, for the shim's directory to be looked at;
	// handOver, for setup-token to have handed its address over; graceOver,
	// for it to have exited once it has made the token; drained, for it to
	// have exited, and its drawing to have been read, once Make has stopped
	// it, or it has exited.
	looking, handOver, graceOver, drained <-chan time.Time
	waits                                 waits

	// made says setup-token has said it made the token, and shown that it
	// has shown it whole since, as token. madeScreen is the fullest drawing
	// that said it's made (see drawnSinceMade), madeDrawn as full.
	made, shown bool
	madeScreen  string
	madeDrawn   int
	// exit is how setup-token exited, once it has: nil where it exited 0.
	// stopped says Make stopped it, its grace over, before ctx ended.
	exit    error
	stopped bool

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
	r := &run{signing: s, shim: sh, screen: newScreen(columns, rows), done: make(chan struct{}), reports: make(chan Event, reportsHeld)}
	r.wg.Go(r.tell)
	if err := r.start(ctx, path, env); err != nil {
		r.close()
		return nil, err
	}
	return r, nil
}

// start starts setup-token, while ctx lasts, and the reading of what it
// draws, and the wait for it to exit. The end of ctx doesn't kill it by
// itself: Make stops it, as cancel says.
func (r *run) start(ctx context.Context, path string, env environ) error {
	if ctx.Err() != nil {
		return cancelled(ctx)
	}
	master, tty, err := openTerminal(columns, rows)
	if err != nil {
		return fmt.Errorf("open a pseudo-terminal for setup-token: %w", err)
	}
	r.master = master
	runs, stop := context.WithCancel(context.WithoutCancel(ctx))
	r.stop = stop
	cmd, err := start(runs, path, []string{claude.Command, claude.SetupTokenCommand}, env.forSetupToken(path, r.shim.browser()), tty)
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
	r.wg.Go(func() { exited <- exitOf(cmd) })
	return nil
}

// exitOf waits on cmd's exit, then kills what it left running in its group,
// so nothing of it holds its terminal open, and returns how it exited (see
// waitOn). The group's id stays its own while anything's left in it, as
// close says.
func exitOf(cmd *exec.Cmd) error {
	err := waitOn(cmd)
	_ = killGroup(cmd.Process.Pid)
	return err
}

// waitOn waits on cmd's exit, and returns how it exited: nil where it exited
// 0, though Wait has that the end of its context, or the Cancel that followed
// it failing, where that came as it exited by itself.
func waitOn(cmd *exec.Cmd) error {
	err := cmd.Wait()
	if state := cmd.ProcessState; state != nil && state.Success() {
		return nil
	}
	return err
}

// read sends output what setup-token draws, a read at a time, and closes it
// once the terminal is closed, at either end, or Make stops it.
func (r *run) read(output chan<- []byte) {
	defer close(output)
	buf := make([]byte, readSize)
	for {
		n, err := r.master.Read(buf)
		if n > 0 {
			select {
			case output <- bytes.Clone(buf[:n]):
			case <-r.done:
				return
			}
		}
		if err != nil {
			return
		}
	}
}

// tell has the Signing's Report hear each event Make reports, in order.
func (r *run) tell() {
	for e := range r.reports {
		if r.signing.Report != nil {
			r.signing.Report(e)
		}
	}
}

// close stops setup-token, should it still be running, all it started on its
// terminal with it, closes its terminal, waits for the reading of it, for
// setup-token's exit, and for the Signing's Report to have heard all Make
// reported, and removes the shim.
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
	close(r.reports)
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
		return tokens.Token{}, cancelled(ctx)
	}
	return r.token, r.err
}

// next takes the next thing to happen.
func (r *run) next(ctx context.Context) {
	select {
	case <-r.ended:
		r.cancel()
	case drawn, ok := <-r.output:
		r.draw(ctx, drawn, ok)
	case err := <-r.exited:
		r.exitedWith(ctx, err)
	case <-r.looking:
		r.lookForAddress(ctx)
	case <-r.handOver:
		r.judge(ctx, notHandedOver(r.screen.drawing().text()))
	case <-r.graceOver:
		r.stopped = ctx.Err() == nil
		r.halt()
	case <-r.drained:
		r.drawWaiting(ctx)
		if !r.over {
			r.conclude(ctx)
		}
	}
}

// drawWaiting draws what's been read of setup-token's drawing and waits on
// Make as its drain ends, readsAtDrain reads at most: the select the drain
// won could have taken that instead, and it can hold the rest of a token.
func (r *run) drawWaiting(ctx context.Context) {
	for range readsAtDrain {
		if r.output == nil || r.over {
			return
		}
		select {
		case drawn, ok := <-r.output:
			r.draw(ctx, drawn, ok)
		default:
			return
		}
	}
}

// cancel follows the end of ctx. Before the token's made, Make stops
// setup-token, and waits on its exit, to tell whether the cancel stopped it,
// or it failed by itself first. Once it's made, it lasts a year on the
// account, and nobody could see it were Make to drop it, so Make reads on
// till it's shown whole, and only then stops setup-token, or till its grace
// is over, which no longer runs on from what it draws (see pace).
func (r *run) cancel() {
	r.ended = nil
	if !r.made || r.shown {
		r.halt()
	}
}

// halt stops setup-token, and waits on its exit, and the end of its drawing,
// as long as the waits' drain at most.
func (r *run) halt() {
	r.stop()
	r.looking, r.handOver, r.graceOver = nil, nil, nil
	if r.drained == nil {
		r.drained = time.After(r.waits.drain)
	}
}

// cancelled is Make failing as ctx ends.
func cancelled(ctx context.Context) error {
	return fmt.Errorf("make a setup token: %w", context.Cause(ctx))
}

// draw draws what setup-token drew, and follows what its screen says before
// each control sequence in it, and after it all, until the token's shown
// whole: a frame and what erases it can come in one read. Its drawing over,
// ok false, and setup-token gone, Make concludes.
func (r *run) draw(ctx context.Context, drawn []byte, ok bool) {
	if !ok {
		r.output = nil
		if r.exited == nil {
			r.conclude(ctx)
		}
		return
	}
	wasShown := r.shown
	for len(drawn) > 0 && !r.shown && !r.over {
		piece := len(drawn)
		if at := bytes.IndexByte(drawn[1:], ansi.ESC); at >= 0 {
			piece = at + 1
		}
		r.screen.write(drawn[:piece])
		drawn = drawn[piece:]
		r.follow(ctx)
	}
	r.screen.write(drawn)
	r.pace(ctx, wasShown)
}

// pace sets setup-token's grace as it draws, once it has made the token,
// wasShown saying it had shown the token whole before. While ctx lasts, the
// grace runs afresh from each thing it draws, up to the token shown whole, and
// on from there however it draws on, so a setup-token that never stops
// drawing is stopped all the same. Once ctx has ended, what it draws gives it
// no more grace, and it's stopped as soon as it has shown the token.
func (r *run) pace(ctx context.Context, wasShown bool) {
	switch {
	case !r.made || r.over || r.drained != nil || wasShown:
	case ctx.Err() == nil:
		r.graceOver = time.After(r.waits.grace)
	case r.shown:
		r.halt()
	}
}

// follow follows what setup-token's screen says: that it has made the token,
// whether it handed its address over or not; and the token, kept the first
// time it's shown whole, and the fullest drawing that said it's made, for a
// failure to show; or an error it waits to retry at, where Make stops.
func (r *run) follow(ctx context.Context) {
	d := r.screen.drawing()
	if !r.made && d.made() {
		r.made = true
		r.looking, r.handOver = nil, nil
		if r.drained == nil {
			r.graceOver = time.After(r.waits.grace)
		}
		r.report(Event{Step: StepMade})
	}
	if !r.made {
		if d.failed() {
			r.judge(ctx, failedAt(d.text()))
		}
		return
	}
	if drawn := d.drawnSinceMade(); drawn >= r.madeDrawn {
		r.madeScreen, r.madeDrawn = d.text(), drawn
	}
	r.token, r.shown = d.token(false)
}

// exitedWith notes setup-token's exit, as err says, and concludes once its
// drawing is over, or the waits' drain after at most.
func (r *run) exitedWith(ctx context.Context, err error) {
	r.exited, r.exit = nil, err
	r.looking, r.handOver, r.graceOver = nil, nil, nil
	if r.output == nil {
		r.conclude(ctx)
		return
	}
	if r.drained == nil {
		r.drained = time.After(r.waits.drain)
	}
}

// lookForAddress looks for the sign-in address setup-token handed the shim,
// while ctx lasts, and opens it with the Opener, reporting it, and the wait
// on its approval that follows. Something handed over that isn't an address
// fails.
func (r *run) lookForAddress(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	raw, ok := r.shim.address()
	if !ok {
		return
	}
	r.looking, r.handOver = nil, nil
	address, ok := newAddress(raw)
	if !ok {
		r.judge(ctx, notAnAddress(r.screen.drawing().text()))
		return
	}
	opened, err := r.signing.Opener.Open(ctx, address)
	if ctx.Err() != nil {
		return
	}
	r.report(Event{Step: StepAddress, Address: address, Opened: opened, Unopened: err})
	r.report(Event{Step: StepWaiting})
}

// conclude takes the token setup-token showed whole, or, where it exited 0 by
// itself, and its drawing has ended, the token its screen shows last, as
// nothing's left to draw; or fails, saying why there's none: for a token made,
// with the fullest drawing that said so, and what stopped setup-token (see
// stoppedBy); otherwise, with what it drew last.
func (r *run) conclude(ctx context.Context) {
	if r.made && !r.shown && r.drewAll() {
		r.token, r.shown = r.screen.drawing().token(true)
	}
	d := r.screen.drawing()
	switch {
	case r.shown:
		r.over = true
	case r.made:
		r.fail(unreadable(r.madeScreen, r.stoppedBy(ctx)))
	case d.failed():
		r.judge(ctx, failedAt(d.text()))
	case r.exited != nil:
		// Still running the waits' drain after ctx ended, which stops it.
		r.fail(cancelled(ctx))
	default:
		r.fail(exitedWithout(d.text(), r.exit))
	}
}

// drewAll reports whether setup-token has exited 0 by itself, and what it drew
// has been read to its end, the terminal closed: not where the drain ends
// first, as the last of it could be on its way yet.
func (r *run) drewAll() bool {
	return r.output == nil && r.exited == nil && r.exit == nil
}

// judge fails with err, a failure Make judged from what setup-token drew or
// how long it took, or with the cancel, where ctx has ended by then: the two
// can come at once, and the end of ctx stops setup-token either way.
func (r *run) judge(ctx context.Context, err error) {
	if ctx.Err() != nil {
		err = cancelled(ctx)
	}
	r.fail(err)
}

// stoppedBy returns what stopped setup-token, for a failure to carry: the
// end of ctx, where that killed it, or it's still running after it; nothing,
// where Make killed it; or how it exited by itself, nil for 0.
func (r *run) stoppedBy(ctx context.Context) error {
	switch {
	case r.stoppedByCancel(ctx), ctx.Err() != nil && r.exited != nil:
		return context.Cause(ctx)
	case r.stopped && killed(r.exit):
		return nil
	}
	return r.exit
}

// stoppedByCancel reports whether the end of ctx is what stopped setup-token:
// ctx has ended, and setup-token was killed, but not by Make, its grace over.
func (r *run) stoppedByCancel(ctx context.Context) bool {
	return ctx.Err() != nil && !r.stopped && killed(r.exit)
}

// killed reports whether exit, how setup-token exited, is its being killed.
func killed(exit error) bool {
	status, ok := errors.AsType[*exec.ExitError](exit)
	return ok && !status.Exited()
}

func (r *run) fail(err error) {
	r.over, r.err = true, err
}

// report has the Signing's Report hear e, off Make's goroutine, so a Report
// that takes its time never holds up the reading of what setup-token draws.
// Each step is reported once, and in order: one reported already, or coming
// before one that was, isn't, so reports never holds more than reportsHeld.
func (r *run) report(e Event) {
	if e.Step <= r.reported {
		return
	}
	r.reported = e.Step
	r.reports <- e
}
