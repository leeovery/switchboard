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

// The made frame, the end of output and the exit come from goroutines as ctx
// ends, as they do: each attempt is a race between them.
func TestRunReadsTheTokenMadeAsItsContextEnds(t *testing.T) {
	killed := exitAs(t, true, "/bin/sleep", "60")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for attempt := range 64 {
		output, exited := make(chan []byte), make(chan error, 1)
		r := &run{screen: newScreen(columns, rows), shim: shim{dir: t.TempDir()}, output: output, exited: exited}
		go func() {
			output <- []byte(frame(waitingRows) + madeFrame())
			close(output)
			exited <- killed
		}()

		token, err := r.wait(ctx, waits{grace: time.Minute, handOver: time.Minute})

		if err != nil || token.Reveal() != madeToken {
			t.Fatalf("attempt %d: wait() = %q, %v; want the token made", attempt, token.Reveal(), err)
		}
	}
}

func TestRunTakesTheTokenFromAFrameErasedInTheSameRead(t *testing.T) {
	made := madeRows(" " + madeToken)
	r := &run{screen: newScreen(columns, rows)}

	r.draw(t.Context(), []byte(frame(waitingRows)+frame(made)+eraseLines(len(made)+1)), true)

	if !r.shown || r.token.Reveal() != madeToken {
		t.Errorf("Make has the token %q, shown: %v; want the token, though erased in the same read, from %q", r.token.Reveal(), r.shown, r.screen.drawing().text())
	}
}

func TestRunFailsWithTheFullestDrawingThatSaidTheTokensMade(t *testing.T) {
	broken := madeRows(" " + madeToken[:60] + "\n " + madeToken[60:])
	r := &run{screen: newScreen(columns, rows)}

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
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, shimAddress), []byte("https://claude.example/oauth/authorize\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		looking := make(chan time.Time, 1)
		looking <- time.Now()
		open := &askedOpener{}
		r := &run{screen: newScreen(columns, rows), shim: shim{dir: dir}, signing: Signing{Opener: open}, looking: looking, ended: ctx.Done()}

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
			r := &run{graceOver: graceOver, stop: func() { stops++ }}

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
	defer func() { _, _ = master.Close(), tty.Close() }()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cmd, err := start(ctx, "/bin/sh", []string{"sh", "-c", "/bin/sleep 60 & echo exiting"}, []string{}, tty)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = killGroup(cmd.Process.Pid) }()
	// All of it, as a program's exit waits on what it drew being read.
	if _, err := io.ReadFull(master, make([]byte, len("exiting\r\n"))); err != nil {
		t.Fatal(err)
	}
	// Long enough for it to have exited, unreaped till waitOn.
	time.Sleep(200 * time.Millisecond)
	cancel()

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
