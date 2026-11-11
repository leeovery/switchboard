package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
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

// organizations tells the organization of a token as the API's answers to a
// probe made with it name it, as the router reads the accounts' usage: an
// answer that names one is an answer, whether the probe read usage or not.
type organizations struct {
	prober accounts.Prober
}

func (o organizations) Organization(ctx context.Context, token tokens.Token) (string, error) {
	probed, err := o.prober.Probe(ctx, token.Reveal())
	if probed.Organization != "" {
		return probed.Organization, nil
	}
	return "", err
}

// moveAskFor is how long a command that finds the history waiting on its
// move onto the accounts' ids waits for the router to say how it stands.
const moveAskFor = 2 * time.Second

// historyMoved fails where the history in the state directory given waits on
// its move onto the accounts' ids, as idmove.Pending says, saying what holds
// it up, as moveSaid says it.
func historyMoved(stateDir string) error {
	if !idmove.Pending(stateDir) {
		return nil
	}
	said := moveSaid(context.Background(), stateDir)
	// The move may be done by the time the router's said how it stands.
	if !idmove.Pending(stateDir) {
		return nil
	}
	return expected{fmt.Errorf("%s; try again once it's done", said)}
}

// moveSaid says how the move onto the accounts' ids stands, as the router in
// the state directory given tells it, asked for moveAskFor at most, as
// status.MovingAsked and status.Moving's Said have it.
func moveSaid(ctx context.Context, stateDir string) string {
	ctx, cancel := context.WithTimeout(ctx, moveAskFor)
	defer cancel()
	moving, err := router.NewClient(router.SocketPath(stateDir)).Moving(ctx)
	return status.MovingAsked(moving, err, errors.Is(err, router.ErrNotRunning)).Said()
}

// awaitMove waits while the history in the state directory given waits on its
// move onto the accounts' ids, as idmove.Pending says, saying what holds it
// up on out, as moveSaid says it, and again whenever that changes, looking
// again as often as a follow does, until ctx ends: the dashboard holds the
// days it reads for as long as it runs, so it opens once they're moved.
func (a *app) awaitMove(ctx context.Context, out io.Writer, stateDir string) error {
	every := logs.PollEvery(a.FollowEvery)
	for told := ""; idmove.Pending(stateDir); {
		if said := moveSaid(ctx, stateDir); said != told {
			if _, err := fmt.Fprintln(out, said+"; the dashboard opens once it's done"); err != nil {
				return err
			}
			told = said
		}
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

// heal writes the accounts' new ids back in the config file at path, where
// the move onto them is done in the state directory given and the config
// names any by the id it had before, as idmove.Heal does: as a router starts,
// before it reads the config, so it starts on the ids the token files are
// named by. Where it can't, which is logged, the router starts all the same,
// the accounts' tokens not found by the old ids.
func heal(path, stateDir string) {
	healed, err := idmove.Heal(path, stateDir)
	switch {
	case err != nil:
		logger.Warn("couldn't write the accounts' new ids back in the config, which names them by those they had before", "path", path, "error", err)
	case healed:
		logger.Info("wrote the accounts' new ids back in the config, which named them by those they had before", "path", path)
	}
}

// formerly gives the id of the account, not removed, that had the id given
// before switchboard made ids, as the accounts record in the state directory
// given has it as the router starts: none where the record can't be read,
// which is logged.
func formerly(stateDir string) func(id string) (string, bool) {
	record, err := identity.NewFile(stateDir).Read()
	if err != nil {
		logger.Warn("can't read the accounts record, so a session pinned to an account by the id it had before ids were made isn't", "error", err)
	}
	return func(id string) (string, bool) {
		e, ok := record.ByWas(id)
		return e.ID, ok && e.Removed.IsZero()
	}
}
