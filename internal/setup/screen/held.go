package screen

import (
	"slices"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/setup"
)

// heldToken is a making's, held from when it begins till a save of its
// account's token is known done, whatever card is open, so no card's coming
// or going loses a token made, which lasts a year on the account: which
// making it is, by its number; whose token it makes; how the making ended,
// kept, as the screen can go before it hears; and later, how a later save of
// the token made ended, unsaved, as an answer to the card asking which
// account it's for, where there's one that shows the token.
type heldToken struct {
	making int
	f      setup.TokenFor
	kept   chan tokenedMsg
	later  *setup.Tokened
}

// heldTokens are every making the screen has begun whose token isn't known
// saved, in the order they began.
type heldTokens []heldToken

// with is the makings held, and the one given.
func (h heldTokens) with(t heldToken) heldTokens {
	return append(slices.Clone(h), t)
}

// savedFor is the makings held but those whose token a save for f's account,
// known done, saved: for an account to add, the one numbered making alone,
// as two to add can share a label and be different subscriptions; for one
// configured, every one for it, by its id.
func (h heldTokens) savedFor(f setup.TokenFor, making int) heldTokens {
	return slices.DeleteFunc(slices.Clone(h), func(t heldToken) bool {
		if f.Add {
			return t.making == making
		}
		return !t.f.Add && t.f.Account.ID == f.Account.ID
	})
}

// makingOf is the number of from's making, none where there's no card.
func makingOf(from *tokenCard) int {
	if from == nil {
		return 0
	}
	return from.told.making
}

// without is the makings held but the one numbered making.
func (h heldTokens) without(making int) heldTokens {
	return slices.DeleteFunc(slices.Clone(h), func(t heldToken) bool { return t.making == making })
}

// unsavedAs is the makings held, the one numbered making's later save
// having ended as later, unsaved, showing its token.
func (h heldTokens) unsavedAs(making int, later setup.Tokened) heldTokens {
	h = slices.Clone(h)
	for i := range h {
		if h[i].making == making {
			h[i].later = &later
		}
	}
	return h
}

// tokenLeft is how the makings held left setup, once each has ended: each
// unsaved, in the order they began, that saved nothing, nor was stopped,
// with the name of the account it's for, and its latest word: how a later
// save of its token ended, where one didn't save it, else how it ended, as
// its card showed it, or as it ended once the screen had gone, to copy. An
// ending that shows a token is given back however the cards have come and
// gone since; any other only where its card, or one opened from it, is
// open, so a making retried isn't given back once for each try. A paste or
// an answer the screen was saving as it went is taken as it ended, as
// goneHeld has it. And interrupted, whether an interrupt was held as a token
// made was saved, as the screen heard it, or as one ended once the screen
// had gone. A failure, which the screen would have closed the card at, came
// once it had gone, so it's logged, as nothing else can show it.
func (m Model) tokenLeft() (unsaved []TokenLeft, interrupted bool) {
	interrupted = m.interrupted
	open := m.makingCard()
	for _, held := range m.goneHeld() {
		var ended tokenedMsg
		select {
		case ended = <-held.kept:
		default:
			continue
		}
		interrupted = interrupted || ended.tokened.InterruptHeld
		if ended.err != nil {
			logger.Error("making a token failed once setup's screen had gone", held.f.Logged("error", ended.err)...)
			continue
		}
		left := TokenLeft{For: prose.Clean(held.f.Name()), Tokened: ended.tokened}
		switch r := ended.tokened.Result; {
		case r == setup.TokenSaved || r == setup.TokenCancelled || r == setup.TokenInterrupted:
		case held.later != nil:
			left.Tokened = *held.later
			unsaved = append(unsaved, left)
		case showsToken(ended.tokened) || open != nil && open.told.making == held.making:
			unsaved = append(unsaved, left)
		}
	}
	return unsaved, interrupted
}

// showsToken reports whether a making that ended as t made a token it didn't
// save, unread, not saved, or held while setup asks which account it's for,
// and shows it, to copy: setup-token's error, shown too, is no token.
func showsToken(t setup.Tokened) bool {
	r := t.Result
	return (r == setup.TokenUnread || r == setup.TokenNotSaved || r == setup.TokenAsked) && len(t.Shown) > 0
}

// goneHeld is the makings held, as the paste or the answer the screen was
// saving as it went ended once it had gone: a save done, the makings it
// saved dropped, as savedFor has it; an answer not saved, its making's later
// word.
func (m Model) goneHeld() heldTokens {
	held := m.held
	if m.paste != nil {
		if ended, ok := m.paste.endedOnceGone(); ok && ended.saved() {
			held = held.savedFor(m.paste.f, makingOf(m.paste.from))
		}
	}
	if m.which != nil {
		if ended, ok := m.which.endedOnceGone(); ok {
			switch {
			case ended.saved():
				held = held.savedFor(m.which.f, makingOf(m.which.from))
			case m.which.made() && len(ended.tokened.Shown) > 0:
				held = held.unsavedAs(m.which.from.told.making, ended.tokened)
			}
		}
	}
	return held
}

// endedOnceGone is how the paste being checked as the screen went ended once
// it had gone, where it has: a failure is logged, as nothing else can show
// it, without what was pasted.
func (c pasteCard) endedOnceGone() (pastedMsg, bool) {
	if !c.checking {
		return pastedMsg{}, false
	}
	select {
	case ended := <-c.kept:
		if ended.err != nil {
			why := redact.Text(ended.err.Error(), c.token.value())
			logger.Error("pasting a token failed once setup's screen had gone", c.f.Logged("error", why)...)
		}
		return ended, true
	default:
		return pastedMsg{}, false
	}
}

// saved reports whether the paste saved its token.
func (p pastedMsg) saved() bool {
	return p.err == nil && p.tokened.Result == setup.TokenSaved
}

// endedOnceGone is how the answer being saved as the screen went ended once
// it had gone, where it has: a failure is logged, as nothing else can show
// it, without the token.
func (c whichCard) endedOnceGone() (answeredMsg, bool) {
	if !c.saving {
		return answeredMsg{}, false
	}
	select {
	case ended := <-c.kept:
		if ended.err != nil {
			why := redact.Text(ended.err.Error(), c.ended.Shown...)
			logger.Error("saving a token as the user answered failed once setup's screen had gone", c.f.Logged("error", why)...)
		}
		return ended, true
	default:
		return answeredMsg{}, false
	}
}

// saved reports whether the answer saved the token.
func (a answeredMsg) saved() bool {
	return a.err == nil && a.tokened.Result == setup.TokenSaved
}
