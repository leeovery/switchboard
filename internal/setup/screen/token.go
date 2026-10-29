package screen

import (
	"context"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setuptoken"
)

// tokenCard is the token card as the screen holds it: whose token it makes,
// and the accounts it's compared with; how its making tells what comes of
// it, and what stops it; the last step the making told, and the step that
// had the sign-in address ready, once told, with when the card heard it,
// which the wait counts from; whether esc has asked the making to stop; and
// how it ended, where it saved nothing.
type tokenCard struct {
	f        setup.TokenFor
	told     makingTold
	stop     context.CancelFunc
	step     setup.MakingStep
	address  setup.Making
	since    time.Time
	stopping bool
	unsaved  *setup.Unsaved
}

// stage is where the card's making has got, as the card tells it: ended,
// where it saved nothing; the token made, however esc came after; being
// stopped, as esc asked; waiting for the sign-in's approval, once the
// address is ready; else starting.
func (c tokenCard) stage() dashboard.TokenStage {
	switch {
	case c.unsaved != nil:
		return dashboard.TokenUnsaved
	case c.step >= setup.TokenMade:
		return dashboard.TokenChecking
	case c.stopping:
		return dashboard.TokenStopping
	case c.step >= setup.AddressReady:
		return dashboard.TokenWaiting
	}
	return dashboard.TokenStarting
}

// unopened reports whether the sign-in address didn't open, so the card
// shows it to paste, and c copies it, as nothing can open it again.
func (c tokenCard) unopened() bool {
	return c.address.Opened == setuptoken.NotOpened
}

// drawn is the card as dashboard.Setup draws it at now: waiting, how long
// it's waited, how the address opened, as setup words it, made safe to show,
// and, where it didn't open, the address, to paste; ended, how it ended.
func (c tokenCard) drawn(now time.Time) *dashboard.SetupTokenCard {
	d := &dashboard.SetupTokenCard{ID: c.f.Account.ID, Stage: c.stage()}
	switch d.Stage {
	case dashboard.TokenWaiting:
		d.Waited, d.Opening, d.Private = now.Sub(c.since), prose.Clean(c.address.Opening()), c.address.Opened == setuptoken.OpenedPrivately
		if d.Unopened = c.unopened(); d.Unopened {
			d.Address = c.address.Address.Reveal()
		}
	case dashboard.TokenUnsaved:
		d.Unsaved = *c.unsaved
	}
	return d
}

// makingTold is how a token's making tells the screen what comes of it: its
// number; each step, as MakeToken reports it, held as they come, so the
// making never waits on the screen to hear one; and how it ended, its job's
// outcome, or its panic, on ended for the screen, and kept for Run, which
// takes it once the screen has gone, as the screen can go before it hears.
type makingTold struct {
	making int
	steps  chan setup.Making
	ended  chan tea.Msg
	kept   chan tokenedMsg
}

// makingMsg is a step the making numbered making told.
type makingMsg struct {
	making int
	at     setup.Making
}

// tokenedMsg is how the making numbered making ended, as MakeToken answered.
type tokenedMsg struct {
	making  int
	tokened setup.Tokened
	err     error
}

// next is what the making tells next, once it tells it: each step it told
// before it ended, in turn, then how it ended; nothing once ctx is done.
func (t makingTold) next(ctx context.Context) tea.Msg {
	select {
	case at := <-t.steps:
		return makingMsg{making: t.making, at: at}
	case end := <-t.ended:
		// Every step was told before the making ended, so one still held is
		// heard first, how it ended put back to be heard next.
		select {
		case at := <-t.steps:
			t.ended <- end
			return makingMsg{making: t.making, at: at}
		default:
			return end
		}
	case <-ctx.Done():
		return nil
	}
}

// making is m with the token card open, making the token f says with
// MakeToken, under a context of its own, which esc ends, its spinner
// moving; and the command that begins the making, as a writing action on a
// goroutine the runner waits for, and listens for what it tells.
func (m Model) making(f setup.TokenFor) (Model, tea.Cmd) {
	m.makings++
	m.spins++
	told := makingTold{making: m.makings, steps: make(chan setup.Making, int(setup.CheckingToken)), ended: make(chan tea.Msg, 1), kept: make(chan tokenedMsg, 1)}
	ctx, stop := context.WithCancel(m.jobs.ctx)
	m.token = &tokenCard{f: f, told: told, stop: stop}
	begin := m.jobs.begin(makeToken(m.jobs, m.steps, ctx, stop, f, told), told.ended)
	return m, tea.Batch(begin, m.listen(), m.armSpin())
}

// makeToken is the job that makes the token f says with MakeToken, as a
// writing action of js's, under ctx, the making's own, which stop ends once
// it's done: each step it reports is told on told's steps, so long as the
// runner's context lasts, and how it ended is its outcome, kept for Run as
// well. Where ctx ends as it waits its turn, it's cancelled, nothing made.
func makeToken(js *jobs, steps Steps, ctx context.Context, stop context.CancelFunc, f setup.TokenFor, told makingTold) job {
	return func(runs context.Context) tea.Msg {
		defer stop()
		ended := tokenedMsg{making: told.making, tokened: setup.Tokened{Result: setup.TokenCancelled}}
		if done, ok := js.turn(ctx); ok {
			defer done()
			ended.tokened, ended.err = steps.MakeToken(ctx, f, func(at setup.Making) {
				select {
				case told.steps <- at:
				case <-runs.Done():
				}
			})
		}
		told.kept <- ended
		return ended
	}
}

// tokenLeft is how the token card's making left setup, once it has ended:
// unsaved, how it ended, where the screen went with the card open, and it
// saved nothing, nor was stopped, as the card showed it, or as it ended once
// the screen had gone, as unread, to copy, nil otherwise; and interrupted,
// whether an interrupt was held as a token made was saved, as the screen
// heard it, or as it ended once the screen had gone. A failure, which the
// screen would have closed the card at, came once it had gone, so it's
// logged, as nothing else can show it.
func (m Model) tokenLeft() (unsaved *setup.Tokened, interrupted bool) {
	if m.token == nil {
		return nil, m.interrupted
	}
	var ended tokenedMsg
	select {
	case ended = <-m.token.told.kept:
	default:
		return nil, m.interrupted
	}
	interrupted = m.interrupted || ended.tokened.InterruptHeld
	if ended.err != nil {
		logger.Error("making a token failed once setup's screen had gone", "account", m.token.f.Account.ID, "error", ended.err)
		return nil, interrupted
	}
	switch ended.tokened.Result {
	case setup.TokenSaved, setup.TokenCancelled, setup.TokenInterrupted:
		return nil, interrupted
	}
	return &ended.tokened, interrupted
}

// listen is the command that waits for what the token card's making tells
// next, as Await has it run.
func (m Model) listen() tea.Cmd {
	told := m.token.told
	return m.await(m.jobs.read(told.next))
}

// madeStep takes in a step the card's making told, and listens for the next:
// the address ready, how it opened, from now, as the wait counts; any
// other, where it's got. A step of a making not the card's is dropped.
func (m Model) madeStep(msg makingMsg) (Model, tea.Cmd) {
	if m.token == nil || m.token.told.making != msg.making {
		return m, nil
	}
	card := *m.token
	card.step = msg.at.Step
	if msg.at.Step == setup.AddressReady {
		card.address, card.since = msg.at, m.now()
	}
	m.token = &card
	return m, m.listen()
}

// tokened takes in how the card's making ended, as ended has it, but where
// an interrupt was held as its token was saved: then the screen leaves, as
// the interrupt asked. How a making not the card's ended is dropped.
func (m Model) tokened(msg tokenedMsg) (Model, tea.Cmd) {
	card := m.token
	if card == nil || card.told.making != msg.making {
		return m, nil
	}
	m, cmd := m.ended(*card, msg)
	if msg.tokened.InterruptHeld {
		m.interrupted = true
		return m, tea.Quit
	}
	return m, cmd
}

// ended takes in how card's making ended: saved, the card closed and the
// accounts read again, the row chosen moving to the account once they're
// read; failing, the step failed, not saved, the card closed; stopped, the
// card closed, nothing saved; else, saving nothing, the card says why, as
// setup words it, made safe to show, the accounts read again where the
// token made wasn't saved, as its save may have changed them.
func (m Model) ended(card tokenCard, msg tokenedMsg) (Model, tea.Cmd) {
	switch r := msg.tokened.Result; {
	case msg.err != nil || r == setup.TokenSaved:
		m.token = nil
		if msg.err == nil {
			m.madeFor = card.f.Account.ID
		}
		return m.changed(msg.err)
	case r == setup.TokenCancelled || r == setup.TokenInterrupted:
		m.token = nil
		return m, nil
	}
	unsaved := cleanUnsaved(msg.tokened.Unsaved())
	card.unsaved = &unsaved
	m.token = &card
	if msg.tokened.Result == setup.TokenNotSaved {
		return m.askAgain()
	}
	return m, nil
}

// cleanUnsaved is u made safe to show: what setup-token showed keeping its
// spacing, as the token in it is to be copied.
func cleanUnsaved(u setup.Unsaved) setup.Unsaved {
	u.Headline, u.Says, u.Then = prose.Clean(u.Headline), prose.Clean(u.Says), prose.Clean(u.Then)
	u.Shown = slices.Clone(u.Shown)
	for i, line := range u.Shown {
		u.Shown[i] = prose.Visible(line)
	}
	return u
}

// tokenPressed acts on a key at the token card, as its stage has it: esc,
// starting or waiting, stops the making, which saves nothing unless the
// token's made first, and once it ended, closes the card; o, waiting,
// opens the address again, where it opened, and c copies it where it
// didn't; and enter, once it ended, tries again, where it does. p, pasting
// a token, does nothing yet; no key stops a making being stopped, nor one
// whose token is made, which is saved however setup's left.
func (m Model) tokenPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	card := *m.token
	stage := card.stage()
	switch key.String() {
	case "esc":
		switch stage {
		case dashboard.TokenStarting, dashboard.TokenWaiting:
			card.stopping = true
			card.stop()
			m.token = &card
		case dashboard.TokenUnsaved:
			m.token = nil
		}
	case "o":
		if stage == dashboard.TokenWaiting && !card.unopened() {
			return m, m.reopen(card.address)
		}
	case "c":
		if stage == dashboard.TokenWaiting && card.unopened() {
			return m, copied(m.page().Copies())
		}
	case "enter":
		if stage == dashboard.TokenUnsaved && card.unsaved.Again {
			return m.making(card.f)
		}
	}
	return m, nil
}

// reopenedMsg is what Reopen answered, opening the sign-in address again.
type reopenedMsg struct {
	err error
}

// reopen is the command that opens the sign-in address at its AddressReady
// again, with Reopen.
func (m Model) reopen(at setup.Making) tea.Cmd {
	steps := m.steps
	return m.jobs.read(func(ctx context.Context) tea.Msg {
		return reopenedMsg{err: steps.Reopen(ctx, at)}
	})
}

// reopened takes in what Reopen answered, noting why in the log where it
// couldn't open the address again: the card goes on waiting, as it does.
func (m Model) reopened(msg reopenedMsg) Model {
	if msg.err != nil {
		logger.Warn("couldn't open the sign-in address again", "error", msg.err)
	}
	return m
}
