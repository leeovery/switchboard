package router

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/notify"
)

const (
	// gatherFor is how long a limit's notification waits for the sessions
	// the limit moves, each as its next request comes, to tell of them all
	// at once.
	gatherFor = 5 * time.Second
	// lookEvery is how often the accounts are looked at with no event to
	// prompt it, so a bar lifting or a window resetting with no traffic is
	// noticed.
	lookEvery = 15 * time.Second
	// quietFor is how long after a notification about an account any other
	// about it but a limit's is dropped.
	quietFor = time.Minute
	// queueSize is how many events can wait to be dealt with. Past that, an
	// event is dropped rather than hold the router up.
	queueSize = 256
	// finishWait bounds how long the notifications due as the router stops
	// get to post, so a post that hangs can't hold it up: one still going
	// then is ended.
	finishWait = 3 * time.Second
)

// Notifier posts desktop notifications, readying what it posts through
// first, as notify.Desktop does.
type Notifier interface {
	// Notify posts message, waiting while what it posts through is readied,
	// and returns once it's posted, or once ctx ends, having ended what it
	// ran to post it.
	Notify(ctx context.Context, message string) error
	// Prepare readies what it posts through, until ctx ends.
	Prepare(ctx context.Context)
}

// notifications posts desktop notifications of what befalls the accounts, as
// settings asks: a limit reached, and the sessions it moved; room again; a
// window passing its warning; and any other move; and, whatever settings
// asks, a token's year ending. It hears the router's events without ever
// holding it up: they queue for run's goroutine, which deals with each, looks
// at the accounts afresh after each and every lookEvery, and posts.
type notifications struct {
	settings config.Notifications
	notifier Notifier
	state    *state
	// years holds what's been told of the accounts' tokens' years, which the
	// state file keeps.
	years *yearsTold
	// now reads the router's clock, which the accounts are judged by. The
	// waits notifications time for themselves run on time's own clock.
	now    func() time.Time
	events chan Event

	// Only run's goroutine touches what follows.
	limits  *limitNotices
	lookout *lookout
	// posted holds, by account, when a notification about it last went out.
	posted map[string]time.Time
	// refused is whether the last notification sent wasn't allowed, as
	// notify.ErrNotAllowed says, which is warned of once until one posts.
	refused bool
}

func newNotifications(settings config.Notifications, notifier Notifier, state *state, years *yearsTold, now func() time.Time) *notifications {
	return &notifications{
		settings: settings,
		notifier: notifier,
		state:    state,
		years:    years,
		now:      now,
		events:   make(chan Event, queueSize),
		limits:   newLimitNotices(),
		lookout:  newLookout(settings.Room, settings.Warning),
		posted:   make(map[string]time.Time),
	}
}

// hear queues an event for run to deal with, of the kinds take deals with: a
// limit reached, and a move. It never waits: with the queue full, the event
// is dropped.
func (n *notifications) hear(e Event) {
	switch e.(type) {
	case LimitReached, Moved:
	default:
		return
	}
	select {
	case n.events <- e:
	default:
		logger.Warn("notifications fell behind; event dropped", "event", fmt.Sprintf("%T", e))
	}
}

// prepare readies what the notifications are posted through as the router
// starts, rather than at its first post, until ctx ends.
func (n *notifications) prepare(ctx context.Context) {
	n.notifier.Prepare(ctx)
}

// run deals with the events heard, and looks at the accounts, posting what
// they call for, until ctx ends, when it finishes. Its first look calls for
// no room again or warning, as there's nothing yet to compare the accounts
// with, but tells of how the tokens' years stand. Each post is given ctx,
// so one waiting on what the notifications are posted through to be readied
// waits no longer than run does, and once ctx has ended, as the router
// stops, nothing is posted but finish's.
func (n *notifications) run(ctx context.Context) {
	ticker := time.NewTicker(lookEvery)
	defer ticker.Stop()
	n.lookAfresh(ctx)
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
		case e := <-n.events:
			accounts, now := n.standings()
			n.take(ctx, e, accounts)
			n.look(ctx, accounts, now)
		case <-ticker.C:
			n.lookAfresh(ctx)
		case <-n.wake():
			n.tellDue(ctx)
		}
	}
	n.finish(ctx)
}

// finish deals with the events still queued, as the router stops, ctx having
// ended, and tells of every limit still gathering at once: a limit's
// notification always goes out. Its posts get finishWait between them, under
// a context of their own, and it waits for them to end, so a post still going
// then is ended, and none outlives it.
func (n *notifications) finish(ctx context.Context) {
	posting, cancel := context.WithTimeout(context.WithoutCancel(ctx), finishWait)
	defer cancel()
	n.takeQueued(posting)
	n.tell(posting, n.limits.all())
	if posting.Err() != nil {
		logger.Warn("ended the notifications still posting as the router stopped", "after", finishWait)
	}
}

// takeQueued deals with each event still queued, as take does.
func (n *notifications) takeQueued(ctx context.Context) {
	for {
		select {
		case e := <-n.events:
			accounts, _ := n.standings()
			n.take(ctx, e, accounts)
		default:
			return
		}
	}
}

// standings returns how the accounts stand now, and now.
func (n *notifications) standings() (standings, time.Time) {
	now := n.now()
	return n.state.standings(now), now
}

// take deals with an event: a limit reached starts gathering the sessions it
// moves, and a move is told of on its own unless a limit told of takes it.
func (n *notifications) take(ctx context.Context, e Event, accounts standings) {
	switch e := e.(type) {
	case LimitReached:
		if n.settings.Limits {
			n.limits.reached(e, time.Now())
		}
	case Moved:
		if n.settings.Limits && n.limits.moved(e, time.Now()) {
			return
		}
		if n.settings.Moves {
			n.post(ctx, moveNotice(e, accounts))
		}
	}
}

// look posts what the accounts, as they stand at now, call for since the last
// look: a token's year last, whatever went out just before, as no quiet
// minute holds it back, so it quiets nothing due at the same look. Once ctx
// has ended it looks no more, so a token's year it would post nothing of
// isn't counted told.
func (n *notifications) look(ctx context.Context, accounts standings, now time.Time) {
	if ctx.Err() != nil {
		return
	}
	for _, notice := range n.lookout.look(accounts) {
		n.post(ctx, notice)
	}
	for _, notice := range n.years.due(accounts, now) {
		n.send(ctx, notice)
	}
}

// lookAfresh looks at the accounts as they stand now, as look does.
func (n *notifications) lookAfresh(ctx context.Context) {
	accounts, now := n.standings()
	n.look(ctx, accounts, now)
}

// wake delivers once the first limit gathering is due, or never, while none
// is.
func (n *notifications) wake() <-chan time.Time {
	due, ok := n.limits.next()
	if !ok {
		return nil
	}
	return time.After(time.Until(due))
}

// tellDue tells of the limits done gathering, as tell does, but for once ctx
// has ended, when they're left gathering, for finish to tell of.
func (n *notifications) tellDue(ctx context.Context) {
	if ctx.Err() == nil {
		n.tell(ctx, n.limits.due(time.Now()))
	}
}

// tell posts the notifications of the limits done gathering, whatever went
// out before them: a limit is the most pressing news, and is told of once a
// limit already.
func (n *notifications) tell(ctx context.Context, done []*gathering) {
	now := n.now()
	accounts := n.state.standings(now)
	for _, g := range done {
		n.send(ctx, g.notice(accounts, now))
	}
}

// post posts a notice, unless another about its account went out within
// quietFor.
func (n *notifications) post(ctx context.Context, notice notify.Notice) {
	if last, ok := n.posted[notice.Account]; ok && time.Since(last) < quietFor {
		logger.Debug("notification dropped: too soon after the last about the account",
			"account", notice.Account, "news", notice.News)
		return
	}
	n.send(ctx, notice)
}

// send posts a notice, which once it's out is the latest about its account,
// and notes how that went in the log. Once ctx has ended, as the router
// stops, it posts nothing.
func (n *notifications) send(ctx context.Context, notice notify.Notice) {
	if ctx.Err() != nil {
		logger.Debug("notification dropped: the router is stopping", "account", notice.Account, "news", notice.News)
		return
	}
	err := n.notifier.Notify(ctx, notice.Message)
	switch {
	case err == nil:
		n.refused = false
		n.posted[notice.Account] = time.Now()
		logger.Info("notification", "account", notice.Account, "news", notice.News)
	case !errors.Is(err, notify.ErrNotAllowed):
		logger.Warn("notification failed", "account", notice.Account, "news", notice.News, "error", err)
	case n.refused:
		logger.Debug("notification not allowed", "account", notice.Account, "news", notice.News)
	default:
		n.refused = true
		logger.Warn("macOS won't let Switchboard post notifications: turn them on in System Settings → Notifications → Switchboard, or, where they're on, switchboard service restart registers its app again",
			"account", notice.Account, "news", notice.News)
	}
}
