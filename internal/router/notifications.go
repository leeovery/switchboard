package router

import (
	"context"
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
	// get to post, so a notifier that hangs can't hold it up.
	finishWait = 3 * time.Second
)

// Notifier posts a desktop notification.
type Notifier interface {
	Notify(message string) error
}

// notifications posts desktop notifications of what befalls the accounts, as
// settings asks: a limit reached, and the sessions it moved; room again; a
// window passing its warning; and any other move. It hears the router's
// events without ever holding it up: they queue for run's goroutine, which
// deals with each, looks at the accounts afresh after each and every
// lookEvery, and posts.
type notifications struct {
	settings config.Notifications
	notifier Notifier
	state    *state
	// now reads the router's clock, which the accounts are judged by. The
	// waits notifications time for themselves run on time's own clock.
	now    func() time.Time
	events chan Event

	// Only run's goroutine touches what follows, and finish's once run hands
	// it over as it ends.
	limits  *limitNotices
	lookout *lookout
	// posted holds, by account, when a notification about it last went out.
	posted map[string]time.Time
}

func newNotifications(settings config.Notifications, notifier Notifier, state *state, now func() time.Time) *notifications {
	return &notifications{
		settings: settings,
		notifier: notifier,
		state:    state,
		now:      now,
		events:   make(chan Event, queueSize),
		limits:   newLimitNotices(),
		lookout:  newLookout(settings.Room, settings.Warning),
		posted:   make(map[string]time.Time),
	}
}

// hear queues an event for run to deal with. It never waits: with the queue
// full, the event is dropped.
func (n *notifications) hear(e Event) {
	select {
	case n.events <- e:
	default:
		logger.Warn("notifications fell behind; event dropped", "event", fmt.Sprintf("%T", e))
	}
}

// run deals with the events heard, and looks at the accounts, posting what
// they call for, until ctx ends, when it finishes. Its first look calls for
// nothing, as there's nothing yet to compare the accounts with.
func (n *notifications) run(ctx context.Context) {
	ticker := time.NewTicker(lookEvery)
	defer ticker.Stop()
	n.look(n.standings())
	for {
		select {
		case <-ctx.Done():
			n.finish()
			return
		case e := <-n.events:
			accounts := n.standings()
			n.take(e, accounts)
			n.look(accounts)
		case <-ticker.C:
			n.look(n.standings())
		case <-n.wake():
			n.tell(n.limits.due(time.Now()))
		}
	}
}

// finish deals with the events still queued, as the router stops, and tells
// of every limit still gathering at once: a limit's notification always goes
// out. It waits finishWait for them at most, so a notifier that hangs can't
// hold the router up.
func (n *notifications) finish() {
	done := make(chan struct{})
	go func() {
		defer close(done)
		n.takeQueued()
		n.tell(n.limits.all())
	}()
	timeout := time.NewTimer(finishWait)
	defer timeout.Stop()
	select {
	case <-done:
	case <-timeout.C:
		logger.Warn("stopped waiting for notifications to post", "after", finishWait)
	}
}

// takeQueued deals with each event still queued, as take does.
func (n *notifications) takeQueued() {
	for {
		select {
		case e := <-n.events:
			n.take(e, n.standings())
		default:
			return
		}
	}
}

// standings returns how the accounts stand now.
func (n *notifications) standings() standings {
	return n.state.standings(n.now())
}

// take deals with an event: a limit reached starts gathering the sessions it
// moves, and a move is told of on its own unless a limit gathering takes it.
func (n *notifications) take(e Event, accounts standings) {
	switch e := e.(type) {
	case LimitReached:
		if n.settings.Limits {
			n.limits.reached(e, time.Now())
		}
	case Moved:
		if !n.limits.moved(e) && n.settings.Moves {
			n.post(moveNotice(e, accounts))
		}
	}
}

// look posts what the accounts, as they stand, call for since the last look.
func (n *notifications) look(accounts standings) {
	for _, notice := range n.lookout.look(accounts) {
		n.post(notice)
	}
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

// tell posts the notifications of the limits done gathering, whatever went
// out before them: a limit is the most pressing news, and is told of once a
// limit already.
func (n *notifications) tell(done []*gathering) {
	now := n.now()
	accounts := n.state.standings(now)
	for _, g := range done {
		n.send(g.notice(accounts, now))
	}
}

// post posts a notice, unless another about its account went out within
// quietFor.
func (n *notifications) post(notice notify.Notice) {
	if last, ok := n.posted[notice.Account]; ok && time.Since(last) < quietFor {
		logger.Debug("notification dropped: too soon after the last about the account",
			"account", notice.Account, "news", notice.News)
		return
	}
	n.send(notice)
}

// send posts a notice as the latest about its account, and notes how that
// went in the log.
func (n *notifications) send(notice notify.Notice) {
	n.posted[notice.Account] = time.Now()
	if err := n.notifier.Notify(notice.Message); err != nil {
		logger.Warn("notification failed", "account", notice.Account, "news", notice.News, "error", err)
		return
	}
	logger.Info("notification", "account", notice.Account, "news", notice.News)
}
