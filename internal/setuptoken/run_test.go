//go:build darwin

package setuptoken

import (
	"context"
	"testing"
	"time"
)

// The made frame and the end of ctx are ready at once: each attempt is a draw
// between them.
func TestRunReadsTheTokenMadeAsItsContextEnds(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	for attempt := range 64 {
		output := make(chan []byte, 1)
		output <- []byte(madeFrame(-1))
		r := &run{screen: newScreen(columns, rows), output: output, ended: ctx.Done()}

		r.next(ctx)

		if r.over || r.token.Reveal() != madeToken {
			t.Fatalf("attempt %d: Make has its answer: %v, token %q, err %v; want it reading on, the token made", attempt, r.over, r.token.Reveal(), r.err)
		}
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
			r := &run{graceOver: graceOver, stop: func() { stops++ }}

			r.next(tt.ctx)

			if stops != 1 || r.stopped != tt.want {
				t.Errorf("its grace over, Make stopped setup-token %d times, saying it stopped it: %v; want once, saying %v", stops, r.stopped, tt.want)
			}
		})
	}
}

func TestRunNamesTheCancelOnlyWhereItStoppedSetupToken(t *testing.T) {
	killed := exitOf(t, true, "/bin/sleep", "60")
	failed := exitOf(t, false, "/bin/sh", "-c", "exit 1")
	ended, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name    string
		ctx     context.Context
		stopped bool
		exit    error
		want    bool
	}{
		{name: "killed, its context ended", ctx: ended, exit: killed, want: true},
		{name: "killed, its context running", ctx: t.Context(), exit: killed},
		{name: "killed by Make, before its context ended", ctx: ended, stopped: true, exit: killed},
		{name: "failed by itself", ctx: ended, exit: failed},
		{name: "exited by itself", ctx: ended},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &run{stopped: tt.stopped, exit: tt.exit}

			if got := r.stoppedByCancel(tt.ctx); got != tt.want {
				t.Errorf("stoppedByCancel() = %v, want %v", got, tt.want)
			}
		})
	}
}

// exitOf returns how the program run as argv, on a terminal of its own,
// exits, killed as start's cancel kills it, where kill says.
func exitOf(t *testing.T, kill bool, argv ...string) error {
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
	err = cmd.Wait()
	if err == nil {
		t.Fatalf("%q exited cleanly, want it failing", argv)
	}
	return err
}
