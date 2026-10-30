package screen

import (
	"context"
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
// which the wait counts from; whether esc, or p, has asked the making to
// stop, and whether p, for the paste card to open once it has; and how it
// ended, where it saved nothing, and whether with its token made, given
// back to copy, unread or not saved.
type tokenCard struct {
	f                 setup.TokenFor
	told              makingTold
	stop              context.CancelFunc
	step              setup.MakingStep
	address           setup.Making
	since             time.Time
	stopping, pasting bool
	unsaved           *setup.Unsaved
	madeUnsaved       bool
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

// stopped is the card once its making is asked to stop, which saves nothing
// unless the token's made first; and, where pasting, for the paste card to
// open in its place once it has stopped.
func (c tokenCard) stopped(pasting bool) *tokenCard {
	c.stopping, c.pasting = true, pasting
	c.stop()
	return &c
}

// notSaved is the card as it tells of its token made once a paste of it
// couldn't be saved, as why says: as setup tells a token made that isn't
// saved, what the card showed under it, the token, or setup-token's lines,
// to copy again.
func (c tokenCard) notSaved(why string) *tokenCard {
	unsaved := setup.Tokened{Result: setup.TokenNotSaved, Why: why, Shown: c.unsaved.Shown}.Unsaved().Clean()
	c.unsaved = &unsaved
	return &c
}

// backTo is the card the paste card opened from it goes back to, on esc,
// and leaves setup as, so a token made but unsaved is there to copy again:
// none where the making ended otherwise.
func (c tokenCard) backTo() *tokenCard {
	if !c.madeUnsaved {
		return nil
	}
	return &c
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
// logged, as nothing else can show it. Where the paste card is open, the
// card is the one it was opened from, as the paste card goes back to it,
// none where the paste it was checking as the screen went saved its token.
func (m Model) tokenLeft() (unsaved *setup.Tokened, interrupted bool) {
	card := m.token
	if m.paste != nil {
		card = m.paste.from
		if m.paste.savedOnceGone() {
			card = nil
		}
	}
	if card == nil {
		return nil, m.interrupted
	}
	var ended tokenedMsg
	select {
	case ended = <-card.told.kept:
	default:
		return nil, m.interrupted
	}
	interrupted = m.interrupted || ended.tokened.InterruptHeld
	if ended.err != nil {
		logger.Error("making a token failed once setup's screen had gone", "account", card.f.Account.ID, "error", ended.err)
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

// ended takes in how card's making ended: saved, or failing, as tokenSaved
// has it, the card closed; stopped, the card closed, nothing saved, the
// paste card opening in its place where p stopped it; else, saving nothing,
// the card says why, as setup words it, made safe to show, the accounts
// read again where the token made wasn't saved, as its save may have
// changed them.
func (m Model) ended(card tokenCard, msg tokenedMsg) (Model, tea.Cmd) {
	switch r := msg.tokened.Result; {
	case msg.err != nil || r == setup.TokenSaved:
		m.token = nil
		return m.tokenSaved(card.f.Account.ID, msg.err)
	case r == setup.TokenCancelled && card.pasting:
		return m.pasting(card.f, nil), nil
	case r == setup.TokenCancelled || r == setup.TokenInterrupted:
		m.token = nil
		return m, nil
	}
	unsaved := msg.tokened.Unsaved().Clean()
	card.unsaved = &unsaved
	card.madeUnsaved = msg.tokened.Result == setup.TokenUnread || msg.tokened.Result == setup.TokenNotSaved
	m.token = &card
	if msg.tokened.Result == setup.TokenNotSaved {
		return m.askAgain()
	}
	return m, nil
}

// tokenPressed acts on a key at the token card, as its stage has it: esc,
// starting or waiting, stops the making, which saves nothing unless the
// token's made first, and once it ended, closes the card; p, starting or
// waiting, stops it so, then opens the paste card once it has stopped, and
// once it ended, opens the paste card at once, which goes back to this card
// as backTo has it; o, waiting, opens the
// address again, where it opened, and c copies it where it didn't; and
// enter, once it ended, tries again, where it does. No key stops a making
// being stopped, nor one whose token is made, which is saved however
// setup's left.
func (m Model) tokenPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	card := *m.token
	stage := card.stage()
	switch key.String() {
	case "esc":
		switch stage {
		case dashboard.TokenStarting, dashboard.TokenWaiting:
			m.token = card.stopped(false)
		case dashboard.TokenUnsaved:
			m.token = nil
		}
	case "p":
		switch stage {
		case dashboard.TokenStarting, dashboard.TokenWaiting:
			m.token = card.stopped(true)
		case dashboard.TokenUnsaved:
			return m.pasting(card.f, card.backTo()), nil
		}
	case "o":
		if stage == dashboard.TokenWaiting && !card.unopened() {
			return m, m.reopen(card.address)
		}
	case "c":
		if stage == dashboard.TokenWaiting && card.unopened() {
			return m, copied(m.page().Copies("c"))
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
