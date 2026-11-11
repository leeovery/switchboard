package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/tokens"
)

// mover is what moves the accounts and their history onto the ids switchboard
// makes, beside the router serve starts from cfg, its config file at path:
// nil where the move's done in the state directory, stateDir.
func (a *app) mover(path, stateDir string, files tokens.Store, cfg router.Config) router.Mover {
	if idmove.Moved(stateDir) {
		return nil
	}
	return &idmove.Mover{
		ConfigPath:    path,
		StateDir:      stateDir,
		Tokens:        files,
		Organizations: organizations{a.prober(cfg.Upstream)},
		Routing:       cfg.Accounts.IDs(),
		Now:           a.Now,
	}
}

// organizations tells the organization of a token as the API's answer to a
// probe made with it names it, as the router reads the accounts' usage.
type organizations struct {
	prober accounts.Prober
}

func (o organizations) Organization(ctx context.Context, token tokens.Token) (string, error) {
	probed, err := o.prober.Probe(ctx, token.Reveal())
	return probed.Organization, err
}

// errHistoryMoving is what a verb that reads the history fails with while the
// history waits on its move onto the accounts' ids.
var errHistoryMoving = errors.New(prose.HistoryMoving + ": try again once it's done")

// historyMoved fails, saying so, where the history in the state directory
// given waits on its move onto the accounts' ids, as idmove.Pending says.
func historyMoved(stateDir string) error {
	if idmove.Pending(stateDir) {
		return expected{errHistoryMoving}
	}
	return nil
}

// awaitMove waits while the history in the state directory given waits on its
// move onto the accounts' ids, as idmove.Pending says, saying so on out once,
// looking again as often as a follow does, until ctx ends: the dashboard
// holds the days it reads for as long as it runs, so it opens once they're
// moved.
func (a *app) awaitMove(ctx context.Context, out io.Writer, stateDir string) error {
	if !idmove.Pending(stateDir) {
		return nil
	}
	if _, err := fmt.Fprintln(out, prose.HistoryMoving+": the dashboard opens once it's done"); err != nil {
		return err
	}
	every := logs.PollEvery(a.FollowEvery)
	for idmove.Pending(stateDir) {
		timer := time.NewTimer(every)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}
