//go:build darwin

package setuptoken

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// eraseLines erases the count lines up from the cursor's, as log-update erases
// its last frame.
func eraseLines(count int) string {
	return strings.Repeat("\x1b[2K\x1b[1A", count-1) + "\x1b[2K\x1b[G"
}

// newTestRun returns a run of no setup-token, as begin makes one, but for
// stop, which it stops setup-token with, and w, how long it waits.
func newTestRun(t *testing.T, stop func(), w waits) *run {
	return &run{screen: newScreen(columns, rows), shim: shim{dir: t.TempDir()}, reports: make(chan Event, reportsHeld), stop: stop, waits: w}
}

// waitsLong are waits no test waits out.
var waitsLong = waits{grace: time.Minute, handOver: time.Minute, drain: time.Minute}

// conclusion takes what happens next to r till Make has its answer, failing
// the test where it has none within ten seconds.
func conclusion(t *testing.T, ctx context.Context, r *run) {
	t.Helper()
	over := make(chan struct{})
	go func() {
		defer close(over)
		for !r.over {
			r.next(ctx)
		}
	}()
	select {
	case <-over:
	case <-time.After(10 * time.Second):
		t.Fatal("Make has no answer after ten seconds, want it bounded")
	}
}

// The made frame, the end of output and the exit come from goroutines as ctx
// ends, as they do: each attempt is a race between them.
func TestRunReadsTheTokenMadeAsItsContextEnds(t *testing.T) {
	killed := exitAs(t, true, "/bin/sleep", "60")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for attempt := range 64 {
		output, exited := make(chan []byte), make(chan error, 1)
		r := newTestRun(t, func() {}, waitsLong)
		r.output, r.exited = output, exited
		go func() {
			output <- []byte(frame(waitingRows) + madeFrame())
			close(output)
			exited <- killed
		}()

		token, err := r.wait(ctx, waitsLong)

		if err != nil || token.Reveal() != madeToken {
			t.Fatalf("attempt %d: wait() = %q, %v; want the token made", attempt, token.Reveal(), err)
		}
	}
}

func TestRunTakesTheTokenFromAFrameErasedInTheSameRead(t *testing.T) {
	made := madeRows(" " + madeToken)
	r := newTestRun(t, func() {}, waitsLong)

	r.draw(t.Context(), []byte(frame(waitingRows)+frame(made)+eraseLines(len(made)+1)), true)

	if !r.shown || r.token.Reveal() != madeToken {
		t.Errorf("Make has the token %q, shown: %v; want the token, though erased in the same read, from %q", r.token.Reveal(), r.shown, r.screen.drawing().text())
	}
}

func TestRunFailsWithTheFullestDrawingThatSaidTheTokensMade(t *testing.T) {
	broken := madeRows(" " + madeToken[:60] + "\n " + madeToken[60:])
	r := newTestRun(t, func() {}, waitsLong)

	r.draw(t.Context(), []byte(frame(waitingRows)+frame(broken)+eraseLines(len(broken)+1)+" Done\r\n"), true)
	r.conclude(t.Context())

	screen, ok := errors.AsType[*ScreenError](r.err)
	if !ok {
		t.Fatalf("Make fails with %v, want a *ScreenError", r.err)
	}
	for _, want := range []string{madeRows("")[0], madeToken[:60] + "\n " + madeToken[60:], "Store this token securely"} {
		if !strings.Contains(screen.Screen(), want) {
			t.Errorf("the error's Screen() = %q, want the made frame, erased since, holding %q", screen.Screen(), want)
		}
	}
}

func TestRunAsksTheOpenerNothingOnceItsContextHasEnded(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for attempt := range 64 {
		open := &askedOpener{}
		r := newTestRun(t, func() {}, waitsLong)
		if err := os.WriteFile(filepath.Join(r.shim.dir, shimAddress), []byte("https://claude.example/oauth/authorize\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		looking := make(chan time.Time, 1)
		looking <- time.Now()
		r.signing, r.looking, r.ended = Signing{Opener: open}, looking, ctx.Done()

		r.next(ctx)

		if open.asked > 0 {
			t.Fatalf("attempt %d: the Opener was asked %d times, want none once ctx has ended", attempt, open.asked)
		}
	}
}

// askedOpener is an Opener that opens nothing, counting the times it's asked.
type askedOpener struct{ asked int }

func (o *askedOpener) Open(context.Context, Address) (Opened, error) {
	o.asked++
	return NotOpened, nil
}

// The end of ctx comes with what Make judges a failure by, ready in the same
// moment: each attempt is a draw between them.
func TestRunNamesTheCancelThatComesWithAFailureItJudges(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name string
		with func(r *run)
	}{
		{name: "the hand-over's wait over", with: func(r *run) {
			handOver := make(chan time.Time, 1)
			handOver <- time.Now()
			r.handOver = handOver
		}},
		{name: "an error drawn", with: func(r *run) {
			output := make(chan []byte, 1)
			output <- []byte(" OAuth error: Request failed with status code 400\r\n\r\n Press Enter to retry.\r\n")
			r.output = output
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for attempt := range 32 {
				r := newTestRun(t, func() {}, waits{grace: time.Minute, handOver: time.Minute, drain: 10 * time.Millisecond})
				r.exited, r.ended = make(chan error), ctx.Done()
				tt.with(r)

				conclusion(t, ctx, r)

				if !errors.Is(r.err, context.Canceled) {
					t.Fatalf("attempt %d: Make fails with %v, want the cancel", attempt, r.err)
				}
			}
		})
	}
}

// What setup-token does stopped is waited on the waits' drain at most: here
// it outlives its stop.
func TestRunWaitsOnSetupTokenOnceItsStoppedForAWhileAtMost(t *testing.T) {
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name      string
		ctx       context.Context
		with      func(r *run)
		wantToken bool
	}{
		{name: "stopped by Make, its grace over", ctx: t.Context(), with: func(r *run) {
			graceOver := make(chan time.Time, 1)
			graceOver <- time.Now()
			r.graceOver = graceOver
		}},
		{name: "stopped as its context ends, the token shown", ctx: ended, wantToken: true, with: func(r *run) {
			r.draw(ended, []byte(madeFrame()), true)
			r.ended = ended.Done()
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRun(t, func() {}, waits{grace: time.Minute, handOver: time.Minute, drain: 10 * time.Millisecond})
			r.made, r.exited = true, make(chan error)
			tt.with(r)

			conclusion(t, tt.ctx, r)

			if got := r.token.Reveal() == madeToken; got != tt.wantToken {
				t.Errorf("Make has the token %q (%v), want it: %v", r.token.Reveal(), r.err, tt.wantToken)
			}
		})
	}
}

func TestRunStopsSetupTokenItselfOnlyBeforeItsContextEnds(t *testing.T) {
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name string
		ctx  context.Context
		want bool
	}{
		{name: "its context running", ctx: t.Context(), want: true},
		{name: "its context ended", ctx: ended},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graceOver := make(chan time.Time, 1)
			graceOver <- time.Now()
			stops := 0
			r := newTestRun(t, func() { stops++ }, waitsLong)
			r.graceOver = graceOver

			r.next(tt.ctx)

			if stops != 1 || r.stopped != tt.want {
				t.Errorf("its grace over, Make stopped setup-token %d times, saying it stopped it: %v; want once, saying %v", stops, r.stopped, tt.want)
			}
		})
	}
}

func TestRunNamesWhatStoppedSetupToken(t *testing.T) {
	killed := exitAs(t, true, "/bin/sleep", "60")
	failed := exitAs(t, false, "/bin/sh", "-c", "exit 1")
	cause := errors.New("the person stopped it")
	ended, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	tests := []struct {
		name    string
		ctx     context.Context
		stopped bool
		running bool
		exit    error
		want    error
		// wantCancel is whether it's the cancel that stopped it.
		wantCancel bool
	}{
		{name: "killed, its context ended", ctx: ended, exit: killed, want: cause, wantCancel: true},
		{name: "running yet, its context ended", ctx: ended, running: true, want: cause},
		{name: "killed, its context running", ctx: t.Context(), exit: killed, want: killed},
		{name: "killed by Make, before its context ended", ctx: ended, stopped: true, exit: killed},
		{name: "failed by itself", ctx: ended, exit: failed, want: failed},
		{name: "exited 0 by itself", ctx: ended},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &run{stopped: tt.stopped, exit: tt.exit}
			if tt.running {
				r.exited = make(chan error)
			}

			if got := r.stoppedBy(tt.ctx); !errors.Is(got, tt.want) {
				t.Errorf("stoppedBy() = %v, want %v", got, tt.want)
			}
			if got := r.stoppedByCancel(tt.ctx); got != tt.wantCancel {
				t.Errorf("stoppedByCancel() = %v, want %v", got, tt.wantCancel)
			}
		})
	}
}

// Once a program has exited, the end of its context has its Cancel kill what
// it left running in its group, and Wait has that the cancel, though it
// exited 0.
func TestWaitOnTakesAnExitOf0AsItIsThoughItsContextEndsAsItExits(t *testing.T) {
	master, tty, err := openTerminal(columns, rows)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = master.Close() }()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd, err := start(ctx, "/bin/sh", []string{"sh", "-c", "/bin/sleep 60 & echo exiting"}, []string{}, tty)
	_ = tty.Close()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = killGroup(cmd.Process.Pid) }()
	kill, cancelled := cmd.Cancel, make(chan struct{})
	cmd.Cancel = func() error {
		defer close(cancelled)
		return kill()
	}
	// The terminal's drawing ends as the program, which leads its session,
	// exits, though what it left running holds it open.
	_, _ = io.Copy(io.Discard, master)
	cancel()
	<-cancelled

	if err := waitOn(cmd); err != nil {
		t.Errorf("waitOn() = %v, want nothing: it exited 0", err)
	}
}

// exitAs returns how the program run as argv, on a terminal of its own,
// exits, killed as start's cancel kills it, where kill says.
func exitAs(t *testing.T, kill bool, argv ...string) error {
	t.Helper()
	master, tty, err := openTerminal(columns, rows)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = master.Close(), tty.Close() }()
	cmd, err := start(t.Context(), argv[0], argv, []string{}, tty)
	if err != nil {
		t.Fatal(err)
	}
	if kill {
		if err := killGroup(cmd.Process.Pid); err != nil {
			t.Fatal(err)
		}
	}
	err = waitOn(cmd)
	if err == nil {
		t.Fatalf("%q exited cleanly, want it failing", argv)
	}
	return err
}
