package screen

import (
	"slices"

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

// savedFor is the makings held but those whose token is f's account's, as a
// save for it is known done.
func (h heldTokens) savedFor(f setup.TokenFor) heldTokens {
	return slices.DeleteFunc(slices.Clone(h), func(t heldToken) bool { return sameAccount(t.f, f) })
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

// sameAccount reports whether a and b are tokens for the same account: one
// configured, by its id, or one to add, by its label.
func sameAccount(a, b setup.TokenFor) bool {
	return a.Add == b.Add && a.Account.ID == b.Account.ID && a.Account.Label == b.Account.Label
}

// tokenLeft is how the makings held left setup, once each has ended: each
// unsaved, in the order they began, that saved nothing, nor was stopped,
// with its latest word: how a later save of its token ended, where one
// didn't save it, else how it ended, as its card showed it, or as it ended
// once the screen had gone, to copy. A token made, unread, not saved, or
// asked which account it's for, is given back however the cards have come
// and gone since; any other ending only where its card, or one opened from
// it, is open. A paste or an answer the screen was saving as it went is
// taken as it ended, as gone has it. And interrupted, whether an interrupt
// was held as a token made was saved, as the screen heard it, or as one
// ended once the screen had gone. A failure, which the screen would have
// closed the card at, came once it had gone, so it's logged, as nothing else
// can show it.
func (m Model) tokenLeft() (unsaved []setup.Tokened, interrupted bool) {
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
		switch r := ended.tokened.Result; {
		case r == setup.TokenSaved || r == setup.TokenCancelled || r == setup.TokenInterrupted:
		case held.later != nil:
			unsaved = append(unsaved, *held.later)
		case madeToken(r) || open != nil && open.told.making == held.making:
			unsaved = append(unsaved, ended.tokened)
		}
	}
	return unsaved, interrupted
}

// goneHeld is the makings held, as the paste or the answer the screen was
// saving as it went ended once it had gone: a save of the account's token
// done, the account's dropped; an answer not saved, its making's later word.
func (m Model) goneHeld() heldTokens {
	held := m.held
	if m.paste != nil {
		if ended, ok := m.paste.endedOnceGone(); ok && ended.saved() {
			held = held.savedFor(m.paste.f)
		}
	}
	if m.which != nil {
		if ended, ok := m.which.endedOnceGone(); ok {
			switch {
			case ended.saved():
				held = held.savedFor(m.which.f)
			case m.which.made() && len(ended.tokened.Shown) > 0:
				held = held.unsavedAs(m.which.from.told.making, ended.tokened)
			}
		}
	}
	return held
}

// madeToken reports whether a making that ended as r made a token it didn't
// save: unread, not saved, or held while setup asks which account it's for.
func madeToken(r setup.TokenResult) bool {
	return r == setup.TokenUnread || r == setup.TokenNotSaved || r == setup.TokenAsked
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
