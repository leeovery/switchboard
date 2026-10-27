// Package setuptoken makes an account's setup token: it runs Claude Code's
// claude setup-token on a pseudo-terminal of its own, has the sign-in address
// setup-token would open handed to it, opens it as its caller chooses, and
// takes the token from what setup-token draws once it says it's made. It
// draws nothing itself: the caller words each step Make reports.
package setuptoken

import (
	"cmp"
	"context"
	"errors"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/tokens"
)

var logger = logs.For("setuptoken")

// The pseudo-terminal setup-token draws on: wide enough that the token is
// drawn whole, on a row of its own.
const (
	columns = 1000
	rows    = 50
)

const (
	// exitGrace is how long setup-token is given, once it says it has made
	// the token, from the last it draws till it has shown the token whole,
	// to exit by itself, as it does half a second later, before it's stopped.
	exitGrace = 5 * time.Second
	// handOverWithin bounds the wait for setup-token to hand its sign-in
	// address over, which it does as it starts, a moment after it's run,
	// seconds on a machine hard at work.
	handOverWithin = 30 * time.Second
)

// waits are how long Make waits on setup-token: grace to exit once it says it
// has made the token, handOver to hand its sign-in address over, and drain to
// exit once it's stopped, and to finish drawing once it has exited.
type waits struct {
	grace, handOver, drain time.Duration
}

// orDefaults returns the waits, exitGrace, handOverWithin and drainFor for
// those that are 0.
func (w waits) orDefaults() waits {
	return waits{grace: cmp.Or(w.grace, exitGrace), handOver: cmp.Or(w.handOver, handOverWithin), drain: cmp.Or(w.drain, drainFor)}
}

// What Make fails with, given no way to tell switchboard's own claude link
// from the real claude, or to open the sign-in address.
var (
	errNoExecutable = errors.New("make a setup token: the Maker has no Executable")
	errNoOpener     = errors.New("make a setup token: the Signing has no Opener")
)

// Maker makes setup tokens with the real claude: the one claude.Find finds,
// never switchboard's own claude link.
type Maker struct {
	// Environ is the environment setup-token starts from, as os.Environ gives
	// it, and whose PATH claude is looked for on.
	Environ []string
	// InstallPaths are where Claude Code's installers put claude, tried in
	// order when it isn't on PATH.
	InstallPaths []string
	// Executable returns the path of this switchboard binary, as
	// os.Executable does: a claude that leads to it is switchboard's link.
	Executable func() (string, error)
	// waits are how long Make waits on setup-token, the defaults where 0.
	waits waits
}

// Signing is what a token's making takes of its caller.
type Signing struct {
	// Opener opens the sign-in address: in the browser as it is, or in a
	// private window.
	Opener Opener
	// Report hears each step, in order, on a goroutine of its own, so it
	// never holds up Make's reading of what setup-token draws, and all of
	// them before Make returns: nil hears nothing.
	Report func(Event)
}

// Make runs claude setup-token, in the user's own environment, as Claude Code
// starts on its own login, and returns the token it makes. It opens the
// sign-in address with s's Opener, reports it, opened or for the caller to
// show, then waits on the sign-in's approval in the browser, then reports the
// token made, and returns it once setup-token exits, as it does by itself.
//
// Where setup-token gives no token Make can read, it fails with a
// *ScreenError, whose Screen is what setup-token last drew, for the person to
// see, or, where it said it made a token, the fullest drawing that said so,
// to copy the token from. So it does where setup-token shows an error it
// would wait to retry at, where Make stops it, and where it doesn't hand its
// sign-in address over within handOverWithin. When ctx ends before the
// token's made, setup-token is stopped, and Make fails with ctx's cause,
// where that's what stopped it, and not its own exit first. Once it's made,
// it lasts a year on the account, so Make reads on till setup-token shows it
// whole, then stops it and returns it, or till its grace is over, and then
// fails with a *ScreenError, which is ctx's cause too. Whichever way Make
// returns, setup-token is gone, all it started on its terminal with it.
func (m Maker) Make(ctx context.Context, s Signing) (tokens.Token, error) {
	switch {
	case ctx.Err() != nil:
		return tokens.Token{}, cancelled(ctx)
	case m.Executable == nil:
		return tokens.Token{}, errNoExecutable
	case s.Opener == nil:
		return tokens.Token{}, errNoOpener
	}
	env := ownEnviron(m.Environ)
	path, err := claude.Find(env.get("PATH"), m.InstallPaths, m.Executable)
	if err != nil {
		return tokens.Token{}, err
	}
	r, err := begin(ctx, path, env, s)
	if err != nil {
		return tokens.Token{}, err
	}
	defer r.close()
	logger.Info("making a setup token", "claude", path)
	return r.wait(ctx, m.waits.orDefaults())
}
